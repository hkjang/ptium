package generation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// The author of a deck is told what kind of thing went wrong and who to ask.
// What they must never be told is where the model lives: the failure the
// provider hands back carries the address and port of an internal service, and
// anyone with an account could read it off the screen.
func TestAFailedGenerationTellsTheAuthorSomethingUseful(t *testing.T) {
	leaky := fmt.Errorf(`AI request failed: %w`,
		errors.New(`Post "http://10.0.4.19:11300/v1/chat/completions": dial tcp 10.0.4.19:11300: connect: connection refused`))
	message := AuthorMessage(leaky, "ko")
	for _, secret := range []string{"10.0.4.19", "11300", "chat/completions", "dial tcp"} {
		if strings.Contains(message, secret) {
			t.Fatalf("the author is shown %q: %s", secret, message)
		}
	}
	if !strings.Contains(message, "연결하지 못했습니다") {
		t.Fatalf("the author is not told the service could not be reached: %s", message)
	}

	cases := []struct {
		cause error
		says  string
	}{
		{rejectedRequest{status: 401, message: "invalid api key"}, "API 키"},
		{rejectedRequest{status: 429, message: "rate limited"}, "잠시 뒤"},
		{rejectedRequest{status: 503, message: "upstream down"}, "오류를 돌려주었습니다"},
		{context.DeadlineExceeded, "제한 시간"},
		{errors.New("AI provider returned invalid JSON (status 200)"), "읽을 수 없는 답"},
		{errors.New("no presentation template is available: gone"), "템플릿"},
		{errors.New("something nobody has classified"), "다시 시도해 주세요"},
	}
	for _, test := range cases {
		if got := AuthorMessage(test.cause, "ko"); !strings.Contains(got, test.says) {
			t.Errorf("%v was written as %q, expected it to mention %q", test.cause, got, test.says)
		}
	}

	// And in the language the deck was asked for.
	if english := AuthorMessage(rejectedRequest{status: 401}, "en"); !strings.Contains(english, "API key") {
		t.Fatalf("the English message reads %q", english)
	}
	if japanese := AuthorMessage(context.DeadlineExceeded, "ja"); !strings.Contains(japanese, "制限時間") {
		t.Fatalf("the Japanese message reads %q", japanese)
	}
}
