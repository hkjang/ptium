package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type oidcTestProvider struct {
	server *httptest.Server
	mu     sync.RWMutex
	keyID  string
	key    *rsa.PrivateKey
	loads  int
}

func newOIDCTestProvider(t *testing.T, keyID string, key *rsa.PrivateKey) *oidcTestProvider {
	t.Helper()
	provider := &oidcTestProvider{keyID: keyID, key: key}
	provider.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(writer).Encode(DiscoveryDocument{
				Issuer:                provider.server.URL,
				JWKSURI:               provider.server.URL + "/jwks",
				AuthorizationEndpoint: provider.server.URL + "/authorize",
				TokenEndpoint:         provider.server.URL + "/token",
			})
		case "/jwks":
			provider.mu.Lock()
			provider.loads++
			keyID, publicKey := provider.keyID, &provider.key.PublicKey
			provider.mu.Unlock()
			_ = json.NewEncoder(writer).Encode(map[string]any{"keys": []any{rsaJWK(keyID, publicKey)}})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(provider.server.Close)
	return provider
}

func (provider *oidcTestProvider) rotate(keyID string, key *rsa.PrivateKey) {
	provider.mu.Lock()
	provider.keyID, provider.key = keyID, key
	provider.mu.Unlock()
}

func (provider *oidcTestProvider) loadCount() int {
	provider.mu.RLock()
	defer provider.mu.RUnlock()
	return provider.loads
}

func TestOIDCAuthenticatorDiscoveryClaimsAndKeyRotation(t *testing.T) {
	keyOne, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider := newOIDCTestProvider(t, "key-1", keyOne)
	now := time.Date(2026, time.August, 1, 7, 0, 0, 0, time.UTC)
	authenticator, err := NewOIDCAuthenticator(t.Context(), OIDCConfig{
		Issuer:        provider.server.URL,
		ClientID:      "ptium-web",
		Audiences:     []string{"ptium-api"},
		AllowHTTP:     true,
		JWKSCacheTTL:  time.Hour,
		ClockSkew:     time.Second,
		MaxTokenBytes: 8192,
	}, WithOIDCClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewOIDCAuthenticator() error = %v", err)
	}
	if authenticator.Discovery().AuthorizationEndpoint == "" {
		t.Fatal("validated discovery metadata was not retained")
	}

	claims := map[string]any{
		"iss":   provider.server.URL,
		"sub":   "keycloak-user-1",
		"email": "alice@example.com",
		"name":  "Alice",
		"aud":   []string{"account", "ptium-api"},
		"scope": "openid profile presentations:read",
		"iat":   now.Add(-time.Minute).Unix(),
		"exp":   now.Add(time.Hour).Unix(),
		"realm_access": map[string]any{
			"roles": []string{"user", "ptium-admin"},
		},
		"resource_access": map[string]any{
			"ptium-web": map[string]any{"roles": []string{"presentation-editor"}},
		},
	}
	principal, err := authenticateJWT(t, authenticator, signRS256(t, keyOne, "key-1", claims))
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.Subject != "keycloak-user-1" || principal.Email != "alice@example.com" {
		t.Fatalf("principal identity = %#v", principal)
	}
	if !principal.HasAllRoles("user", "ptium-admin", "presentation-editor") {
		t.Fatalf("roles = %v", principal.Roles)
	}
	if !principal.HasAllScopes("openid", "profile", "presentations:read") {
		t.Fatalf("scopes = %v", principal.Scopes)
	}

	badAudience := cloneClaims(claims)
	badAudience["aud"] = "some-other-api"
	if _, err := authenticateJWT(t, authenticator, signRS256(t, keyOne, "key-1", badAudience)); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("bad-audience error = %v", err)
	}

	expired := cloneClaims(claims)
	expired["exp"] = now.Add(-time.Minute).Unix()
	if _, err := authenticateJWT(t, authenticator, signRS256(t, keyOne, "key-1", expired)); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expired-token error = %v", err)
	}

	keyTwo, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider.rotate("key-2", keyTwo)
	principal, err = authenticateJWT(t, authenticator, signRS256(t, keyTwo, "key-2", claims))
	if err != nil {
		t.Fatalf("Authenticate() after key rotation error = %v", err)
	}
	if principal.Subject != "keycloak-user-1" {
		t.Fatalf("rotated-key principal = %#v", principal)
	}
	if provider.loadCount() < 2 {
		t.Fatalf("JWKS loads = %d, want initial load plus rotation refresh", provider.loadCount())
	}
}

func TestOIDCAuthenticatorAllowsUnconfiguredAudience(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider := newOIDCTestProvider(t, "key", key)
	now := time.Now().UTC().Truncate(time.Second)
	authenticator, err := NewOIDCAuthenticator(t.Context(), OIDCConfig{
		Issuer:    provider.server.URL,
		ClientID:  "ptium-web",
		AllowHTTP: true,
	}, WithOIDCClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewOIDCAuthenticator() error = %v", err)
	}
	claims := map[string]any{
		"iss": provider.server.URL,
		"sub": "user",
		"aud": "keycloak-default-account-audience",
		"exp": now.Add(time.Hour).Unix(),
	}
	if _, err := authenticateJWT(t, authenticator, signRS256(t, key, "key", claims)); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
}

func authenticateJWT(t *testing.T, authenticator *OIDCAuthenticator, token string) (*Principal, error) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://ptium.test/api/v1/me", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	return authenticator.Authenticate(request.Context(), request)
}

func signRS256(t *testing.T, key *rsa.PrivateKey, keyID string, claims map[string]any) string {
	t.Helper()
	headerBytes, err := json.Marshal(map[string]any{"alg": "RS256", "kid": keyID, "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	claimBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	header := base64.RawURLEncoding.EncodeToString(headerBytes)
	payload := base64.RawURLEncoding.EncodeToString(claimBytes)
	signingInput := header + "." + payload
	hasher := crypto.SHA256.New()
	_, _ = hasher.Write([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hasher.Sum(nil))
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func rsaJWK(keyID string, key *rsa.PublicKey) map[string]any {
	exponent := big.NewInt(int64(key.E)).Bytes()
	return map[string]any{
		"kty": "RSA",
		"kid": keyID,
		"use": "sig",
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(exponent),
	}
}

func cloneClaims(claims map[string]any) map[string]any {
	clone := make(map[string]any, len(claims))
	for key, value := range claims {
		clone[key] = value
	}
	return clone
}
