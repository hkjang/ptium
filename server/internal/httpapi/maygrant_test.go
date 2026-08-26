package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/auth"
)

func askingAs(method string, principal *auth.Principal) *http.Request {
	request := httptest.NewRequest(method, "/api/v1/api-keys", strings.NewReader("{}"))
	if principal == nil {
		return request
	}
	return request.WithContext(auth.WithPrincipal(request.Context(), principal))
}

// Scoping a key is a promise that the machine holding it cannot do more than it
// was given. A key with api_keys:manage could break that promise by itself: it
// could issue itself another key — or widen the one it held — up to everything
// its owner may do, which on an administrator's account meant admin:users.
func TestAKeyCannotGrantWhatItDoesNotHold(t *testing.T) {
	server := &Server{}
	held := &auth.Principal{Subject: "someone", AuthMethod: "api_key",
		Scopes: []string{"api_keys:manage", "presentations:read"}}

	writer := httptest.NewRecorder()
	if server.mayGrant(writer, askingAs("POST", held), []string{"presentations:write"}) {
		t.Error("a read-only key was allowed to grant writing")
	}
	if writer.Code != http.StatusForbidden {
		t.Errorf("granting more than it holds answered %d", writer.Code)
	}
	var body struct {
		Error struct {
			Message string `json:"message"`
			Details struct {
				Scope string `json:"scope"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(writer.Body.Bytes(), &body); err != nil {
		t.Fatalf("the refusal is not readable: %v", err)
	}
	if body.Error.Details.Scope != "presentations:write" {
		t.Errorf("the refusal names %q", body.Error.Details.Scope)
	}

	// What it holds, it may pass on — and it may always narrow.
	if !server.mayGrant(httptest.NewRecorder(), askingAs("POST", held),
		[]string{"presentations:read", "api_keys:manage"}) {
		t.Error("a key was refused the scopes it holds")
	}
	if !server.mayGrant(httptest.NewRecorder(), askingAs("PATCH", held), []string{"presentations:read"}) {
		t.Error("a key was refused a narrower set than it holds")
	}
}

// A person signing in with their own account grants whatever they are entitled
// to: it is their account, and the key screen is where they say so.
func TestAPersonGrantsWhateverTheirAccountAllows(t *testing.T) {
	server := &Server{}
	session := &auth.Principal{Subject: "someone", AuthMethod: "session"}
	if !server.mayGrant(httptest.NewRecorder(), askingAs("POST", session), []string{"admin:users"}) {
		t.Error("a signed-in administrator was refused by the key-to-key rule")
	}
	if !server.mayGrant(httptest.NewRecorder(), askingAs("POST", nil), []string{"presentations:write"}) {
		t.Error("a request with no principal was judged by the key-to-key rule")
	}
}

// Asking for nothing used to mean the deployment's defaults, and the defaults
// include writing: a read-only key could not name presentations:write, but it
// could leave the field out and be handed it. What a new key will carry is what
// the rule has to judge.
func TestAKeyThatNamesNoScopesGetsItsOwn(t *testing.T) {
	server := &Server{}
	held := &auth.Principal{Subject: "someone", AuthMethod: "api_key",
		Scopes: []string{"api_keys:manage", "presentations:read"}}
	granted := server.scopesToGrant(askingAs("POST", held), nil)
	if strings.Join(granted, " ") != "api_keys:manage presentations:read" {
		t.Errorf("a key naming no scopes would hand on %v", granted)
	}
	if !server.mayGrant(httptest.NewRecorder(), askingAs("POST", held), granted) {
		t.Error("a key was refused the scopes it holds")
	}
	// A person gets the deployment's defaults, as before.
	session := &auth.Principal{Subject: "someone", AuthMethod: "session"}
	if defaults := server.scopesToGrant(askingAs("POST", session), nil); len(defaults) != 4 {
		t.Errorf("a person naming no scopes gets %v", defaults)
	}
	// And what was named is what is judged.
	if named := server.scopesToGrant(askingAs("POST", held), []string{"templates:read"}); len(named) != 1 {
		t.Errorf("named scopes were replaced by %v", named)
	}
}

// Rotating a key hands its replacement's secret to whoever asked, with the old
// key's scopes on it. A key that may not hold a scope may not be handed a key
// that does: a narrow key with api_keys:manage could rotate its owner's
// administrator key and read the new secret out of the answer.
func TestAKeyCannotRotateAKeyWiderThanItself(t *testing.T) {
	server := &Server{}
	held := &auth.Principal{Subject: "someone", AuthMethod: "api_key",
		Scopes: []string{"api_keys:manage", "presentations:read"}}
	if server.mayGrant(httptest.NewRecorder(), askingAs("POST", held), []string{"admin:users", "presentations:read"}) {
		t.Error("a narrow key was allowed to be handed an administrator key")
	}
	if !server.mayGrant(httptest.NewRecorder(), askingAs("POST", held), []string{"presentations:read"}) {
		t.Error("a key was refused a rotation of something narrower than itself")
	}
	session := &auth.Principal{Subject: "someone", AuthMethod: "session"}
	if !server.mayGrant(httptest.NewRecorder(), askingAs("POST", session), []string{"admin:users"}) {
		t.Error("a person was refused a rotation of their own key")
	}
}
