package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testIssuer(t *testing.T, ttl time.Duration) *SessionIssuer {
	t.Helper()
	issuer, err := NewSessionIssuer("a-deployment-secret-of-sufficient-length", ttl)
	if err != nil {
		t.Fatalf("NewSessionIssuer: %v", err)
	}
	return issuer
}

func TestNewSessionIssuerRejectsWeakInput(t *testing.T) {
	if _, err := NewSessionIssuer("short", time.Hour); err == nil {
		t.Fatal("a short secret must be rejected")
	}
	if _, err := NewSessionIssuer("a-deployment-secret-of-sufficient-length", 400*24*time.Hour); err == nil {
		t.Fatal("an excessive lifetime must be rejected")
	}
	issuer, err := NewSessionIssuer("a-deployment-secret-of-sufficient-length", 0)
	if err != nil {
		t.Fatalf("a zero lifetime should fall back to the default: %v", err)
	}
	if issuer.Lifetime() != defaultSessionTTL {
		t.Fatalf("lifetime = %s, want %s", issuer.Lifetime(), defaultSessionTTL)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	issuer := testIssuer(t, time.Hour)
	token, expiresAt, err := issuer.Issue("user-1", 1234)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if !strings.HasPrefix(token, SessionTokenPrefix) {
		t.Fatalf("token %q is not marked as a session token", token)
	}
	if time.Until(expiresAt) > time.Hour+time.Minute || time.Until(expiresAt) < 59*time.Minute {
		t.Fatalf("expiry %s is not about an hour away", expiresAt)
	}
	claims, err := issuer.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.UserID != "user-1" || claims.Epoch != 1234 {
		t.Fatalf("claims = %+v", claims)
	}
	if _, _, err := issuer.Issue("", 0); err == nil {
		t.Fatal("an empty user id must be rejected")
	}
}

func TestSessionParseRejectsTamperingAndExpiry(t *testing.T) {
	issuer := testIssuer(t, time.Hour)
	token, _, _ := issuer.Issue("user-1", 7)

	// A different secret must not verify.
	other := testIssuer(t, time.Hour)
	other.key = append([]byte("different"), other.key...)
	if _, err := other.Parse(token); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("a token signed elsewhere must be rejected, got %v", err)
	}

	// Editing the payload must invalidate the signature.
	body := strings.TrimPrefix(token, SessionTokenPrefix)
	encoded, signature, _ := strings.Cut(body, ".")
	forged := SessionTokenPrefix + encoded[:len(encoded)-2] + "AA." + signature
	if _, err := issuer.Parse(forged); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("a tampered payload must be rejected, got %v", err)
	}

	for _, malformed := range []string{"", "nonsense", SessionTokenPrefix, SessionTokenPrefix + "onlypayload",
		SessionTokenPrefix + "." + signature, "ptium_looks_like_an_api_key"} {
		if _, err := issuer.Parse(malformed); err == nil {
			t.Fatalf("%q must not parse", malformed)
		}
	}

	expired := testIssuer(t, time.Hour)
	expired.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	stale, _, _ := expired.Issue("user-1", 7)
	if _, err := issuer.Parse(stale); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("an expired token must be rejected, got %v", err)
	}
}

func TestSessionAuthenticatorOnlyClaimsItsOwnTokens(t *testing.T) {
	issuer := testIssuer(t, time.Hour)
	resolved := 0
	authenticator := SessionAuthenticator{
		Issuer: issuer,
		Resolver: SessionResolverFunc(func(_ context.Context, claims SessionClaims) (*Principal, error) {
			resolved++
			if claims.Epoch != 42 {
				// This is the revocation check: a stale epoch is not a session.
				return nil, ErrInvalidCredentials
			}
			return &Principal{Subject: "session:" + claims.UserID, AuthMethod: "password"}, nil
		}),
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	token, _, _ := issuer.Issue("user-1", 42)
	request.Header.Set("Authorization", "Bearer "+token)
	principal, err := authenticator.Authenticate(context.Background(), request)
	if err != nil || principal == nil || principal.AuthMethod != "password" {
		t.Fatalf("Authenticate() = %+v, %v", principal, err)
	}

	// An API key must pass straight through to the next authenticator.
	apiKeyRequest := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	apiKeyRequest.Header.Set("Authorization", "Bearer ptium_abcdef123456_secret")
	if _, err := authenticator.Authenticate(context.Background(), apiKeyRequest); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("an API key must not be claimed by the session authenticator, got %v", err)
	}
	if resolved != 1 {
		t.Fatalf("the resolver ran %d times, want once", resolved)
	}

	// A token whose account has since changed its password is rejected.
	staleEpoch, _, _ := issuer.Issue("user-1", 41)
	staleRequest := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	staleRequest.Header.Set("Authorization", "Bearer "+staleEpoch)
	if _, err := authenticator.Authenticate(context.Background(), staleRequest); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("a token from before a password change must be rejected, got %v", err)
	}

	unconfigured := SessionAuthenticator{}
	if _, err := unconfigured.Authenticate(context.Background(), request); err == nil {
		t.Fatal("an unconfigured authenticator must error")
	}
}

func TestOIDCConfigRequiresClientIDWithSecret(t *testing.T) {
	config := OIDCConfig{
		Issuer: "https://sso.example.com/realms/company", ClientSecret: "a-secret",
		AllowedAlgorithms: []string{"RS256"}, HTTPTimeout: time.Second,
		JWKSCacheTTL: time.Minute, MaxTokenBytes: 4096,
	}
	if err := config.Validate(); err == nil {
		t.Fatal("a client secret without a client id must be rejected")
	}
	config.ClientID = "ptium-web"
	if err := config.Validate(); err != nil {
		t.Fatalf("a confidential client must validate: %v", err)
	}
}

func TestLoadBootstrapConfigReadsClientSecret(t *testing.T) {
	config, err := LoadBootstrapConfig(MapSource{
		"OIDC_ISSUER_URL":    "https://sso.example.com/realms/company",
		"OIDC_CLIENT_ID":     "ptium-web",
		"OIDC_CLIENT_SECRET": "confidential-value",
	})
	if err != nil {
		t.Fatalf("LoadBootstrapConfig: %v", err)
	}
	if config.OIDC.ClientSecret != "confidential-value" {
		t.Fatalf("client secret = %q", config.OIDC.ClientSecret)
	}
}
