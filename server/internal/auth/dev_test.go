package auth

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDevAuthenticatorDynamicIdentity(t *testing.T) {
	secret := strings.Repeat("a-secure-development-secret-", 2)
	authenticator, err := NewDevAuthenticator(DevConfig{
		Enabled: true,
		Secret:  secret,
		Principal: Principal{
			Subject: "dev:default@example.com",
			Roles:   []string{"user", "ptium-admin"},
		},
	})
	if err != nil {
		t.Fatalf("NewDevAuthenticator() error = %v", err)
	}
	request := httptest.NewRequest("GET", "http://ptium.test/api/v1/me", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	request.Header.Set("X-Ptium-Dev-Secret", secret)
	request.Header.Set("Authorization", "Bearer dev:alice@example.com:ptium-admin,user")

	principal, err := authenticator.Authenticate(request.Context(), request)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.Email != "alice@example.com" || !principal.HasAllRoles("user", "ptium-admin") {
		t.Fatalf("principal = %#v", principal)
	}
}

func TestDevAuthenticatorSecurityChecks(t *testing.T) {
	secret := strings.Repeat("x", 32)
	authenticator, err := NewDevAuthenticator(DevConfig{Enabled: true, Secret: secret})
	if err != nil {
		t.Fatalf("NewDevAuthenticator() error = %v", err)
	}

	tests := []struct {
		name       string
		remote     string
		secret     string
		bearer     string
		wantNoCred bool
	}{
		{name: "missing secret", remote: "127.0.0.1:1", wantNoCred: true},
		{name: "wrong secret", remote: "127.0.0.1:1", secret: strings.Repeat("z", 32)},
		{name: "remote client", remote: "203.0.113.2:1", secret: secret},
		{name: "role escalation", remote: "127.0.0.1:1", secret: secret, bearer: "Bearer dev:user@example.com:super-admin"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "http://ptium.test/", nil)
			request.RemoteAddr = test.remote
			if test.secret != "" {
				request.Header.Set("X-Ptium-Dev-Secret", test.secret)
			}
			if test.bearer != "" {
				request.Header.Set("Authorization", test.bearer)
			}
			_, err := authenticator.Authenticate(request.Context(), request)
			if test.wantNoCred {
				if !errors.Is(err, ErrNoCredentials) {
					t.Fatalf("error = %v, want ErrNoCredentials", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("error = %v, want ErrInvalidCredentials", err)
			}
		})
	}
}
