package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/auth"
	"github.com/hkjang/ptium/server/internal/mcpoauth"
)

// refusingAuthenticator answers every request the way an SSO door refuses a
// token, or with no credentials at all when none came.
type refusingAuthenticator struct{}

func (refusingAuthenticator) Authenticate(_ context.Context, request *http.Request) (*auth.Principal, error) {
	if request.Header.Get("Authorization") == "" {
		return nil, auth.ErrNoCredentials
	}
	return nil, &auth.Refusal{Cause: errors.New("sso token refused: audience"), Message: "The token was not issued for this server (aud=[account], azp=\"weekly\")."}
}

func ssoServer(policy mcpoauth.Policy, oidc bool) *Server {
	server := &Server{
		logger:        slog.New(slog.DiscardHandler),
		authenticator: refusingAuthenticator{},
		mcpHandler:    http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) }),
		readMCPOAuth:  func(context.Context) mcpoauth.Policy { return policy },
	}
	if oidc {
		server.authPublic = AuthPublicConfig{OIDCEnabled: true, Issuer: "https://keycloak.corp.example/realms/corp"}
	}
	return server
}

const ssoResource = "https://slides.corp.example/mcp"

func TestTheMetadataDocumentIsBareJSONReadableByAnyoneAndAbsentWhileOff(t *testing.T) {
	on := mcpoauth.Policy{Enabled: true, Resource: ssoResource, Audiences: []string{"claude-mcp"}, Scopes: []string{"presentations:read"}}
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		handler := ssoServer(on, true).Handler()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Origin", "https://claude.ai")
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s answered %d: %s", path, recorder.Code, recorder.Body)
		}
		var document map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &document); err != nil {
			t.Fatal(err)
		}
		if _, enveloped := document["data"]; enveloped {
			t.Fatalf("%s is wrapped in the product's envelope: %s", path, recorder.Body)
		}
		if document["resource"] != ssoResource || document["resource_name"] != "Ptium MCP" {
			t.Fatalf("%s document = %s", path, recorder.Body)
		}
		servers, _ := document["authorization_servers"].([]any)
		if len(servers) != 1 || servers[0] != "https://keycloak.corp.example/realms/corp" {
			t.Fatalf("authorization_servers = %v", document["authorization_servers"])
		}
		methods, _ := document["bearer_methods_supported"].([]any)
		if len(methods) != 1 || methods[0] != "header" {
			t.Fatalf("bearer_methods_supported = %v", document["bearer_methods_supported"])
		}
		scopes, _ := document["scopes_supported"].([]any)
		if len(scopes) != 1 || scopes[0] != "presentations:read" {
			t.Fatalf("scopes_supported = %v", document["scopes_supported"])
		}
		if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Fatalf("Access-Control-Allow-Origin = %q", got)
		}
		if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "" {
			t.Fatalf("a public document says credentials %q", got)
		}
		if got := recorder.Header().Get("Content-Security-Policy"); got != apiPolicy {
			t.Fatalf("a data path carries %q", got)
		}
	}
	// Off, or on with no provider to point at: 404. Metadata that points at
	// a provider whose tokens are then refused is a login loop.
	for name, server := range map[string]*Server{
		"switched off":            ssoServer(mcpoauth.Policy{Enabled: false, Resource: ssoResource}, true),
		"no OIDC running":         ssoServer(on, false),
		"no resource identifier":  ssoServer(mcpoauth.Policy{Enabled: true}, true),
		"the MCP endpoint absent": func() *Server { s := ssoServer(on, true); s.mcpHandler = nil; return s }(),
	} {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/mcp", nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s: answered %d", name, recorder.Code)
		}
	}
}

func TestA401AtMCPPointsAtTheMetadataAndAREST401DoesNot(t *testing.T) {
	on := mcpoauth.Policy{Enabled: true, Resource: ssoResource, Scopes: []string{"presentations:read"}}
	handler := ssoServer(on, true).Handler()

	// No credentials: the invitation, without error=.
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}")))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("/mcp answered %d", recorder.Code)
	}
	want := `Bearer realm="ptium-mcp", resource_metadata="https://slides.corp.example/.well-known/oauth-protected-resource/mcp"`
	if got := recorder.Header().Get("WWW-Authenticate"); got != want {
		t.Fatalf("WWW-Authenticate = %q, want %q", got, want)
	}

	// A refused token: error="invalid_token", and the refusal's sentence in
	// the body — the operator reads which claim was wrong from it.
	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	request.Header.Set("Authorization", "Bearer a.b.c")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("/mcp with a token answered %d", recorder.Code)
	}
	if got := recorder.Header().Get("WWW-Authenticate"); got != want+`, error="invalid_token"` {
		t.Fatalf("WWW-Authenticate = %q", got)
	}
	if !strings.Contains(recorder.Body.String(), `azp=\"weekly\"`) || !strings.Contains(recorder.Body.String(), "authentication_required") {
		t.Fatalf("body = %s", recorder.Body)
	}

	// The REST API's 401 is exactly what it was.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if recorder.Code != http.StatusUnauthorized || recorder.Header().Get("WWW-Authenticate") != `Bearer realm="ptium"` {
		t.Fatalf("/api/v1/me: %d %q", recorder.Code, recorder.Header().Get("WWW-Authenticate"))
	}
	if strings.Contains(recorder.Body.String(), "weekly") {
		t.Fatalf("a REST refusal carried the SSO sentence: %s", recorder.Body)
	}

	// Switched off, /mcp's 401 is exactly what it was too.
	recorder = httptest.NewRecorder()
	ssoServer(mcpoauth.Policy{Resource: ssoResource}, true).Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}")))
	if recorder.Code != http.StatusUnauthorized || recorder.Header().Get("WWW-Authenticate") != `Bearer realm="ptium-mcp"` {
		t.Fatalf("/mcp off: %d %q", recorder.Code, recorder.Header().Get("WWW-Authenticate"))
	}
}

// The client is told only the refusal's sentence; the cause — which check
// the token failed — goes to the log under the request id the client got
// back, so the operator can join the two.
func TestARefusedTokensCauseIsLoggedUnderTheRequestIDTheClientGot(t *testing.T) {
	var lines strings.Builder
	server := ssoServer(mcpoauth.Policy{Enabled: true, Resource: ssoResource}, true)
	server.logger = slog.New(slog.NewTextHandler(&lines, nil))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	request.Header.Set("Authorization", "Bearer a.b.c")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("/mcp answered %d: %s", recorder.Code, recorder.Body)
	}
	var body struct {
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.RequestID == "" {
		t.Fatalf("no requestId in %s", recorder.Body)
	}
	// The request log carries the id too; what is pinned is the refusal's
	// own line carrying it, beside the cause.
	var refused string
	for _, line := range strings.Split(lines.String(), "\n") {
		if strings.Contains(line, `msg="authentication failed"`) {
			refused = line
		}
	}
	if refused == "" {
		t.Fatalf("no authentication failed line in:\n%s", lines.String())
	}
	for _, want := range []string{"request_id=" + body.RequestID, "path=/mcp", `error="sso token refused: audience"`} {
		if !strings.Contains(refused, want) {
			t.Errorf("the refusal's log line lacks %s: %s", want, refused)
		}
	}
	// The cause is for the log alone: the client saw the sentence, not it.
	if strings.Contains(recorder.Body.String(), "sso token refused") {
		t.Fatalf("the cause reached the client: %s", recorder.Body)
	}
}

// An SSO principal passes the same scope gate a key does: what the
// administrator granted opens, what they did not stays shut, and nothing
// about the token being from the identity provider widens it.
func TestAnSSOPrincipalIsBoundedByItsScopesLikeAKey(t *testing.T) {
	sso := auth.WithPrincipal(context.Background(), &auth.Principal{Subject: "mcp-oauth:u", AuthMethod: mcpoauth.AuthMethod, Scopes: []string{"mcp:use", "presentations:read"}})
	if !allowScope(sso, "mcp:use") || !allowScope(sso, "presentations:read") {
		t.Fatal("a granted scope was refused")
	}
	if allowScope(sso, "presentations:write") || allowScope(sso, "admin:settings") {
		t.Fatal("an ungranted scope was allowed")
	}
	recorder := httptest.NewRecorder()
	requireScope("presentations:write", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("reached") })).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/mcp", nil).WithContext(sso))
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "mcp.oauth.scopes") {
		t.Fatalf("%d %s", recorder.Code, recorder.Body)
	}
}

func TestTheAdministratorsSwitchIsRefusedWhenItWouldDoNothing(t *testing.T) {
	values := func(pairs ...string) map[string]json.RawMessage {
		result := map[string]json.RawMessage{}
		for index := 0; index+1 < len(pairs); index += 2 {
			result[pairs[index]] = json.RawMessage(pairs[index+1])
		}
		return result
	}
	if err := mcpoauth.FromValues(values("mcp.oauth.enabled", "true"), "").Validate("https://kc/realms/x"); err == nil || !strings.Contains(err.Error(), "resource identifier") {
		t.Fatalf("no resource: %v", err)
	}
	if err := mcpoauth.FromValues(values("mcp.oauth.enabled", "true"), "https://slides.corp.example").Validate(""); err == nil || !strings.Contains(err.Error(), "issuer") {
		t.Fatalf("no issuer: %v", err)
	}
	if err := mcpoauth.FromValues(values("mcp.oauth.enabled", "true"), "https://slides.corp.example").Validate("https://kc/realms/x"); err != nil {
		t.Fatalf("PUBLIC_BASE_URL builds the identifier: %v", err)
	}
	if err := mcpoauth.FromValues(values("mcp.oauth.enabled", "false"), "").Validate(""); err != nil {
		t.Fatalf("off needs nothing: %v", err)
	}
	for value, ok := range map[string]bool{
		`""`: true, `"https://slides.corp.example/mcp"`: true, `"http://localhost:8080/mcp"`: true,
		`"http://slides.corp.example/mcp"`: false, `"https://slides.corp.example/"`: false, `"https://slides.corp.example/mcp?x=1"`: false,
		`"https://user:pw@slides.corp.example/mcp"`: false, `"slides"`: false,
	} {
		if err := validateSettingValue(mcpoauth.SettingResource, json.RawMessage(value)); (err == nil) != ok {
			t.Errorf("resource %s: err = %v, want ok=%v", value, err, ok)
		}
	}
	for value, ok := range map[string]bool{
		`"presentations:read templates:read"`: true, `""`: false, `"admin:settings"`: false, `"nothing:like-this"`: false,
	} {
		if err := validateSettingValue(mcpoauth.SettingScopes, json.RawMessage(value)); (err == nil) != ok {
			t.Errorf("scopes %s: err = %v, want ok=%v", value, err, ok)
		}
	}
	for value, ok := range map[string]bool{
		`"claude-mcp cursor"`: true, `""`: true, `"has space?"`: true, `"한글"`: false,
	} {
		if err := validateSettingValue(mcpoauth.SettingAudience, json.RawMessage(value)); (err == nil) != ok {
			t.Errorf("audience %s: err = %v, want ok=%v", value, err, ok)
		}
	}
}
