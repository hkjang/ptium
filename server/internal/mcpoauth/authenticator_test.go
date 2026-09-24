package mcpoauth

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hkjang/ptium/server/internal/auth"
)

// A fake identity provider: a real key pair, discovery and JWKS served the
// way Keycloak serves them, and tokens signed with the key — so what is
// checked is the checking, not a stand-in for it.
type fakeIdP struct {
	server *httptest.Server
	key    *rsa.PrivateKey
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	idp := &fakeIdP{key: key}
	idp.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"issuer": idp.server.URL, "jwks_uri": idp.server.URL + "/jwks",
				"authorization_endpoint": idp.server.URL + "/authorize", "token_endpoint": idp.server.URL + "/token",
			})
		case "/jwks":
			exponent := big.NewInt(int64(key.E)).Bytes()
			_ = json.NewEncoder(writer).Encode(map[string]any{"keys": []any{map[string]any{
				"kty": "RSA", "kid": "k1", "use": "sig", "alg": "RS256",
				"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(exponent),
			}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(idp.server.Close)
	return idp
}

// accessToken is what Keycloak 26 mints for a client: typ=Bearer, the
// client in azp, and aud=["account"] — never the client id.
func (idp *fakeIdP) accessToken(t *testing.T, client string, more map[string]any) string {
	t.Helper()
	claims := map[string]any{
		"iss": idp.server.URL, "sub": "sub-hong", "typ": "Bearer", "azp": client,
		"aud": []string{"account"}, "exp": time.Now().Add(5 * time.Minute).Unix(),
		"iat": time.Now().Add(-time.Second).Unix(), "scope": "openid profile email",
		"preferred_username": "hong",
	}
	for key, value := range more {
		if value == nil {
			delete(claims, key)
		} else {
			claims[key] = value
		}
	}
	return idp.sign(t, "RS256", claims)
}

func (idp *fakeIdP) sign(t *testing.T, alg string, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]any{"alg": alg, "kid": "k1", "typ": "JWT"})
	payload, _ := json.Marshal(claims)
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	hasher := crypto.SHA256.New()
	_, _ = hasher.Write([]byte(signingInput))
	var signature []byte
	if alg == "HS256" {
		signature = hasher.Sum(nil)
	} else {
		var err error
		if signature, err = rsa.SignPKCS1v15(rand.Reader, idp.key, crypto.SHA256, hasher.Sum(nil)); err != nil {
			t.Fatal(err)
		}
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

type fixture struct {
	idp      *fakeIdP
	policy   Policy
	accounts map[string]Account
	// keys is what the key authenticator behind the SSO door saw and said.
	keysSaw []string
	chain   auth.Authenticator
	// logs is what the SSO door wrote for the operator.
	logs bytes.Buffer
}

const resource = "https://slides.corp.example/mcp"

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{idp: newFakeIdP(t), policy: Policy{Enabled: true, Resource: resource, Audiences: []string{"claude-mcp"}, Scopes: []string{"presentations:read", "templates:read"}}}
	f.accounts = map[string]Account{"sub-hong": {ID: "u-hong", Email: "hong@example.com", Name: "홍길동", Roles: []string{"user"}}}
	provider, err := auth.NewOIDCAuthenticator(t.Context(), auth.OIDCConfig{Issuer: f.idp.server.URL, ClientID: "ptium-web", AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	sso := &Authenticator{
		Provider: provider,
		Policy:   func(context.Context) Policy { return f.policy },
		Accounts: AccountsFunc(func(_ context.Context, subject string) (Account, error) {
			account, ok := f.accounts[subject]
			if !ok {
				return Account{}, ErrNoAccount
			}
			return account, nil
		}),
		Logger: slog.New(slog.NewTextHandler(&f.logs, nil)),
	}
	keys := auth.NewAPIKeyAuthenticator(auth.APIKeyVerifierFunc(func(_ context.Context, key string) (*auth.Principal, error) {
		f.keysSaw = append(f.keysSaw, key)
		if key == "ptium_abc_good" {
			return &auth.Principal{Subject: "api-key:u-hong", Scopes: []string{"mcp:use"}}, nil
		}
		return nil, auth.ErrInvalidCredentials
	}))
	f.chain = auth.CompositeAuthenticator{Authenticators: []auth.Authenticator{sso, keys}}
	return f
}

func (f *fixture) authenticate(t *testing.T, bearer string) (*auth.Principal, error) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, resource, nil)
	request.Header.Set("Authorization", "Bearer "+bearer)
	return f.chain.Authenticate(request.Context(), request)
}

func refusalSaying(t *testing.T, err error, want string) {
	t.Helper()
	var refusal *auth.Refusal
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v, want a refusal", err)
	}
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("a refusal must be invalid credentials to the middleware: %v", err)
	}
	if !strings.Contains(refusal.Message, want) {
		t.Fatalf("refusal says %q, want it to say %q", refusal.Message, want)
	}
	if refusal.Cause == nil || refusal.Error() == refusal.Message {
		t.Fatalf("a refusal must carry its cause for the log, got %q", refusal.Error())
	}
}

func TestATokenIssuedForThisServerOpensTheRegisteredAccount(t *testing.T) {
	f := newFixture(t)
	principal, err := f.authenticate(t, f.idp.accessToken(t, "claude-mcp", nil))
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.AuthMethod != AuthMethod || principal.Claims["ptium_user_id"] != "u-hong" || principal.Subject != "mcp-oauth:u-hong" {
		t.Fatalf("principal = %+v", principal)
	}
	// The scopes are the administrator's, plus the endpoint itself; the
	// token's "openid profile email" names none of this product's vocabulary.
	if !principal.HasAllScopes("mcp:use", "presentations:read", "templates:read") || principal.HasScope("presentations:write") {
		t.Fatalf("scopes = %v", principal.Scopes)
	}
	// Roles come from the account, not the token.
	if principal.HasRole("ptium-admin") {
		t.Fatalf("roles = %v", principal.Roles)
	}
	if len(f.keysSaw) != 0 {
		t.Fatalf("a JWT reached the key authenticator: %v", f.keysSaw)
	}
}

func TestTheAudienceRuleIsTheResourceOrTheAdministratorsList(t *testing.T) {
	f := newFixture(t)
	// The formal path: an Audience mapper puts the resource in aud.
	if _, err := f.authenticate(t, f.idp.accessToken(t, "some-other-client", map[string]any{"aud": []string{"account", resource}})); err != nil {
		t.Fatalf("resource in aud: %v", err)
	}
	// aud as a single string works the same.
	if _, err := f.authenticate(t, f.idp.accessToken(t, "some-other-client", map[string]any{"aud": resource})); err != nil {
		t.Fatalf("resource as a bare aud: %v", err)
	}
	// Another application's token: refused, and the refusal says what was
	// seen and what to write where.
	_, err := f.authenticate(t, f.idp.accessToken(t, "weekly-web", nil))
	refusalSaying(t, err, `aud=[account], azp="weekly-web"`)
	refusalSaying(t, err, `add "weekly-web" to mcp.oauth.audience`)
	refusalSaying(t, err, resource)
	// The administrator writes that client id down: through, with no mapper.
	f.policy.Audiences = append(f.policy.Audiences, "weekly-web")
	if _, err := f.authenticate(t, f.idp.accessToken(t, "weekly-web", nil)); err != nil {
		t.Fatalf("listed azp: %v", err)
	}
	// The web sign-in's own client is not accepted by being the web client.
	f.policy.Audiences = nil
	_, err = f.authenticate(t, f.idp.accessToken(t, "ptium-web", nil))
	refusalSaying(t, err, "not issued for this server")
}

func TestTheChecksTheStandardNames(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		name  string
		token string
		says  string
	}{
		{"expired", f.idp.accessToken(t, "claude-mcp", map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}), "signature, issuer or validity"},
		{"not yet valid", f.idp.accessToken(t, "claude-mcp", map[string]any{"nbf": time.Now().Add(time.Hour).Unix()}), "signature, issuer or validity"},
		{"another issuer", f.idp.accessToken(t, "claude-mcp", map[string]any{"iss": "https://other.example/realms/x"}), "signature, issuer or validity"},
		{"HS256", f.idp.sign(t, "HS256", map[string]any{"iss": f.idp.server.URL, "sub": "sub-hong", "azp": "claude-mcp", "exp": time.Now().Add(time.Hour).Unix()}), "signature, issuer or validity"},
		{"an ID token", f.idp.accessToken(t, "claude-mcp", map[string]any{"typ": "ID"}), "ID token"},
		{"a bound token", f.idp.accessToken(t, "claude-mcp", map[string]any{"cnf": map[string]any{"jkt": "x"}}), "cnf"},
		{"no subject", f.idp.accessToken(t, "claude-mcp", map[string]any{"sub": nil}), "sub"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.authenticate(t, tc.token)
			refusalSaying(t, err, tc.says)
		})
	}
	// A token signed by a key the provider never published.
	other := newFakeIdP(t)
	_, err := f.authenticate(t, other.sign(t, "RS256", map[string]any{"iss": f.idp.server.URL, "sub": "sub-hong", "azp": "claude-mcp", "exp": time.Now().Add(time.Hour).Unix()}))
	refusalSaying(t, err, "signature, issuer or validity")
}

func TestNobodyIsRegisteredByAToken(t *testing.T) {
	f := newFixture(t)
	_, err := f.authenticate(t, f.idp.accessToken(t, "claude-mcp", map[string]any{"sub": "sub-nobody"}))
	refusalSaying(t, err, "Sign in to the web once first")
	if _, registered := f.accounts["sub-nobody"]; registered {
		t.Fatal("a token registered an account")
	}
	f.accounts["sub-off"] = Account{ID: "u-off", Disabled: true}
	_, err = f.authenticate(t, f.idp.accessToken(t, "claude-mcp", map[string]any{"sub": "sub-off"}))
	refusalSaying(t, err, "disabled")
	// A role claim in the token raises nobody.
	principal, err := f.authenticate(t, f.idp.accessToken(t, "claude-mcp", map[string]any{"realm_access": map[string]any{"roles": []string{"ptium-admin"}}}))
	if err != nil || principal.HasRole("ptium-admin") {
		t.Fatalf("principal = %+v, err = %v", principal, err)
	}
}

func TestSwitchedOffATokenIsRefusedLikeABadKeyAndKeysAreUntouched(t *testing.T) {
	f := newFixture(t)
	f.policy.Enabled = false
	_, err := f.authenticate(t, f.idp.accessToken(t, "claude-mcp", nil))
	refusalSaying(t, err, "does not take SSO access tokens")
	// Off is the administrator's choice, not a misconfiguration: nothing to
	// warn about.
	const cannotTake = "MCP SSO is switched on but cannot take tokens"
	if strings.Contains(f.logs.String(), cannotTake) {
		t.Fatalf("switched off warned the operator:\n%s", f.logs.String())
	}
	// Switched on but with nothing to name: the same answer to the client,
	// and the reason — what Active says — is warned about, since the
	// administrator flipped a switch that does nothing.
	f.policy = Policy{Enabled: true}
	if active, reason := f.policy.Active(f.idp.server.URL); active || !strings.Contains(reason, "resource identifier") {
		t.Fatalf("Active() = %v, %q", active, reason)
	}
	_, err = f.authenticate(t, f.idp.accessToken(t, "claude-mcp", nil))
	refusalSaying(t, err, "does not take SSO access tokens")
	if !strings.Contains(err.Error(), "sso token refused: no resource identifier") {
		t.Fatalf("cause = %v", err)
	}
	if logs := f.logs.String(); !strings.Contains(logs, cannotTake) || !strings.Contains(logs, "resource identifier") {
		t.Fatalf("switched on with no resource did not warn the operator:\n%s", logs)
	}
	// Keys go to the key authenticator exactly as before, on or off.
	for _, enabled := range []bool{false, true} {
		f.policy = Policy{Enabled: enabled, Resource: resource}
		f.keysSaw = nil
		if principal, err := f.authenticate(t, "ptium_abc_good"); err != nil || principal.Subject != "api-key:u-hong" {
			t.Fatalf("enabled=%v: key principal = %+v, err = %v", enabled, principal, err)
		}
		if _, err := f.authenticate(t, "ptium_abc_bad"); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("enabled=%v: bad key err = %v", enabled, err)
		}
		// Neither a key nor a token: the key authenticator's answer, no new words.
		_, err := f.authenticate(t, "what.is-this")
		var refusal *auth.Refusal
		if !errors.Is(err, auth.ErrInvalidCredentials) || errors.As(err, &refusal) {
			t.Fatalf("enabled=%v: junk err = %v", enabled, err)
		}
		if len(f.keysSaw) != 3 {
			t.Fatalf("enabled=%v: keys saw %v", enabled, f.keysSaw)
		}
	}
}

func TestScopesNarrowToWhatTheTokenCarriesWhenItSpeaksTheVocabulary(t *testing.T) {
	f := newFixture(t)
	principal, err := f.authenticate(t, f.idp.accessToken(t, "claude-mcp", map[string]any{"scope": "openid presentations:read"}))
	if err != nil {
		t.Fatal(err)
	}
	if !principal.HasAllScopes("mcp:use", "presentations:read") || principal.HasScope("templates:read") {
		t.Fatalf("scopes = %v", principal.Scopes)
	}
	// Never wider than the administrator's list, whatever the token says.
	principal, err = f.authenticate(t, f.idp.accessToken(t, "claude-mcp", map[string]any{"scope": "presentations:read presentations:write admin:settings"}))
	if err != nil {
		t.Fatal(err)
	}
	if principal.HasAnyScope("presentations:write", "admin:settings") {
		t.Fatalf("scopes = %v", principal.Scopes)
	}
}

// The web sign-in's authenticator guards the REST API. A token minted for the
// MCP client names that client in azp, and does not open the API.
func TestAnMCPClientsTokenDoesNotOpenTheRESTAPI(t *testing.T) {
	f := newFixture(t)
	web, err := auth.NewOIDCAuthenticator(t.Context(), auth.OIDCConfig{Issuer: f.idp.server.URL, ClientID: "ptium-web", AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "https://slides.corp.example/api/v1/me", nil)
	request.Header.Set("Authorization", "Bearer "+f.idp.accessToken(t, "claude-mcp", nil))
	if _, err := web.Authenticate(request.Context(), request); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("REST with an MCP client's token: err = %v", err)
	}
	// The web client's own token still does, as it always has.
	request.Header.Set("Authorization", "Bearer "+f.idp.accessToken(t, "ptium-web", nil))
	if _, err := web.Authenticate(request.Context(), request); err != nil {
		t.Fatalf("REST with the web client's token: %v", err)
	}
}
