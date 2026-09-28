package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/auth"
)

// A token-shaped string with the given claims. The signature is never checked
// here — only the provider that issued it checks it, when it is handed back.
func idTokenWith(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	body, _ := json.Marshal(claims)
	return header + "." + base64.RawURLEncoding.EncodeToString(body) + ".c2ln"
}

const testIssuer = "https://sso.example.com/realms/ptium"

func TestAnIDTokenIsKeptOnlyWhenItIsThisClientsForThisPerson(t *testing.T) {
	good := idTokenWith(map[string]any{"iss": testIssuer, "aud": "ptium", "sub": "u-1"})
	if !idTokenFor(good, testIssuer, "ptium", "u-1") {
		t.Fatal("this client's token for this person was refused")
	}
	// Keycloak writes aud as a list when a token has more than one audience.
	if !idTokenFor(idTokenWith(map[string]any{"iss": testIssuer + "/", "aud": []string{"account", "ptium"}, "sub": "u-1"}),
		testIssuer, "ptium", "u-1") {
		t.Error("a token naming this client among several audiences was refused")
	}
	for name, token := range map[string]string{
		"another issuer":  idTokenWith(map[string]any{"iss": "https://evil.example.com", "aud": "ptium", "sub": "u-1"}),
		"another client":  idTokenWith(map[string]any{"iss": testIssuer, "aud": "other", "sub": "u-1"}),
		"another person":  idTokenWith(map[string]any{"iss": testIssuer, "aud": "ptium", "sub": "u-2"}),
		"not a token":     "not-a-token",
		"too big to keep": idTokenWith(map[string]any{"iss": testIssuer, "aud": "ptium", "sub": "u-1", "pad": strings.Repeat("x", 4000)}),
	} {
		if idTokenFor(token, testIssuer, "ptium", "u-1") {
			t.Errorf("%s was kept", name)
		}
	}
}

// Measured against Keycloak 26: asked to sign somebody out with no
// id_token_hint, it stops on a page of its own asking "Do you want to log
// out?". Whoever closes that tab stays signed in at the provider, and the next
// tab they open is signed straight back in.
func TestSignOutHandsBackTheProvidersAddressWithTheHint(t *testing.T) {
	server := &Server{authPublic: AuthPublicConfig{
		OIDCEnabled: true, Issuer: testIssuer, ClientID: "ptium",
		EndSessionEndpoint: testIssuer + "/protocol/openid-connect/logout",
	}}
	hint := idTokenWith(map[string]any{"iss": testIssuer, "aud": "ptium", "sub": "u-1"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout",
		strings.NewReader(`{"postLogoutRedirectUri":"https://ptium.example.com/login"}`))
	request.AddCookie(&http.Cookie{Name: auth.IDTokenHintCookieName, Value: hint})
	recorder := httptest.NewRecorder()
	server.signOut(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var answer struct {
		Data struct {
			EndSessionURL string `json:"endSessionUrl"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	address, err := url.Parse(answer.Data.EndSessionURL)
	if err != nil {
		t.Fatal(err)
	}
	query := address.Query()
	if query.Get("id_token_hint") != hint {
		t.Errorf("the provider is not told whose session to end: %s", answer.Data.EndSessionURL)
	}
	if query.Get("client_id") != "ptium" || query.Get("post_logout_redirect_uri") != "https://ptium.example.com/login" {
		t.Errorf("sign-out address is %s", answer.Data.EndSessionURL)
	}
	// Both cookies go, so the hint cannot outlive the session it belongs to.
	cleared := map[string]bool{}
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.MaxAge < 0 {
			cleared[cookie.Name] = true
		}
	}
	if !cleared[auth.SessionCookieName] || !cleared[auth.IDTokenHintCookieName] {
		t.Errorf("cookies cleared: %v", cleared)
	}
}

// A client that asks nothing is answered as it always was: scripts and older
// workspaces post to this endpoint with no body and expect no body back.
func TestSignOutWithNoBodyIsAnsweredAsBefore(t *testing.T) {
	server := &Server{authPublic: AuthPublicConfig{OIDCEnabled: true, ClientID: "ptium",
		EndSessionEndpoint: testIssuer + "/protocol/openid-connect/logout"}}
	recorder := httptest.NewRecorder()
	server.signOut(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil))
	if recorder.Code != http.StatusNoContent || recorder.Body.Len() != 0 {
		t.Errorf("status %d, body %q", recorder.Code, recorder.Body.String())
	}
	// And with no provider there is nowhere to send anybody.
	plain := &Server{}
	recorder = httptest.NewRecorder()
	plain.signOut(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout",
		strings.NewReader(`{"postLogoutRedirectUri":"https://ptium.example.com/login"}`)))
	if recorder.Code != http.StatusNoContent {
		t.Errorf("a deployment with no provider answered %d", recorder.Code)
	}
}
