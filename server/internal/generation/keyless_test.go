package generation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A model on a closed network is reached without a key.
//
// An empty ai.api_key was read as "this deployment has no provider" — the one
// thing it cannot mean where this product is meant to run. A site that had
// named its host and its model, and could see both on the admin screen, was
// told "이 배포는 AI 제공자가 설정되어 있지 않습니다" when it asked for another
// draft, and its generated decks came from the offline writer without saying
// so. The provider check said the same: "API 키가 비어 있어 제공자에 요청하지
// 않습니다" — it refused on the host's behalf rather than asking it.
func TestAModelWithNoKeyIsStillAModel(t *testing.T) {
	var sawAuthorization, called = "", 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		called++
		sawAuthorization = request.Header.Get("Authorization")
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": "# 현황\n- 짧게\n"}}},
		})
	}))
	defer server.Close()

	generator := New(testSettings{
		"ai.provider": "openai-compatible", "ai.base_url": server.URL, "ai.model": "test-model",
	})
	drafted, err := generator.ReviseSlide(context.Background(), Revision{Source: "# 현황\n- 원문\n"})
	if err != nil {
		t.Fatalf("a keyless provider was refused: %v", err)
	}
	if called != 1 {
		t.Fatalf("the model was asked %d times", called)
	}
	if strings.TrimSpace(drafted) == "" {
		t.Error("the draft came back empty")
	}
	// "Bearer " with nothing after it is a malformed credential, not an absent one.
	if sawAuthorization != "" {
		t.Errorf("a request with no key still carried Authorization %q", sawAuthorization)
	}

	// And a deployment that does have a key still sends it.
	keyed := New(testSettings{
		"ai.provider": "openai-compatible", "ai.base_url": server.URL, "ai.model": "test-model",
		"ai.api_key": "secret-token",
	})
	if _, err := keyed.ReviseSlide(context.Background(), Revision{Source: "# 현황\n- 원문\n"}); err != nil {
		t.Fatalf("ReviseSlide with a key: %v", err)
	}
	if sawAuthorization != "Bearer secret-token" {
		t.Errorf("the key was not sent: Authorization %q", sawAuthorization)
	}

	// A deployment that chose the offline writer is still left alone.
	off := New(testSettings{"ai.provider": "fallback", "ai.base_url": server.URL, "ai.model": "test-model"})
	if _, err := off.ReviseSlide(context.Background(), Revision{Source: "# 현황\n"}); err != ErrProviderUnavailable {
		t.Errorf("a deployment set to the offline writer answered %v", err)
	}
}
