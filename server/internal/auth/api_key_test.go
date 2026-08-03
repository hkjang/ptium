package auth

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
)

func TestAPIKeyAuthenticatorBearerHook(t *testing.T) {
	verifier := APIKeyVerifierFunc(func(_ context.Context, key string) (*Principal, error) {
		if key != "ptium_test_key" {
			return nil, ErrInvalidCredentials
		}
		return &Principal{Subject: "service-1", Scopes: []string{"presentations:write"}}, nil
	})
	authenticator := APIKeyAuthenticator{Verifier: verifier, AllowBearer: true}
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("Authorization", "Bearer ptium_test_key")
	principal, err := authenticator.Authenticate(request.Context(), request)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.AuthMethod != "api_key" || principal.Subject != "service-1" {
		t.Fatalf("principal = %#v", principal)
	}
	if !principal.HasScope("presentations:write") {
		t.Fatalf("scopes = %v", principal.Scopes)
	}

	request = httptest.NewRequest("GET", "/", nil)
	request.Header.Set("Authorization", "Bearer wrong")
	if _, err := authenticator.Authenticate(request.Context(), request); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("error = %v, want ErrInvalidCredentials", err)
	}
}
