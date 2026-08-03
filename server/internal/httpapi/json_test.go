package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONRejectsTrailingContent(t *testing.T) {
	for _, body := range []string{`{"name":"ok"} {}`, `{"name":"ok"} trailing`} {
		request := httptest.NewRequest("POST", "/", strings.NewReader(body))
		response := httptest.NewRecorder()
		var target struct {
			Name string `json:"name"`
		}
		if decodeJSON(response, request, &target) {
			t.Fatalf("decodeJSON accepted trailing content %q", body)
		}
		if response.Code != 400 {
			t.Fatalf("status = %d, want 400", response.Code)
		}
	}
}

func TestDecodeJSONAcceptsOneValue(t *testing.T) {
	request := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"ok"}`))
	response := httptest.NewRecorder()
	var target struct {
		Name string `json:"name"`
	}
	if !decodeJSON(response, request, &target) || target.Name != "ok" {
		t.Fatalf("valid JSON was rejected: status=%d body=%s", response.Code, response.Body.String())
	}
}
