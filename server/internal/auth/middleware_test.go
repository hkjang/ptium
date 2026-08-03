package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticationAndAuthorizationMiddleware(t *testing.T) {
	authenticator := AuthenticatorFunc(func(context.Context, *http.Request) (*Principal, error) {
		return &Principal{Subject: "user-1", Roles: []string{"ptium-admin"}}, nil
	})
	called := false
	handler := Middleware(authenticator)(RequireAdmin()(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		called = true
		principal, ok := PrincipalFromContext(request.Context())
		if !ok || principal.Subject != "user-1" {
			t.Fatalf("principal = %#v, ok = %v", principal, ok)
		}
		writer.WriteHeader(http.StatusNoContent)
	})))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	if response.Code != http.StatusNoContent || !called {
		t.Fatalf("status = %d, called = %v", response.Code, called)
	}
}

func TestAuthenticationMiddlewareHidesCredentialDetails(t *testing.T) {
	authenticator := AuthenticatorFunc(func(context.Context, *http.Request) (*Principal, error) {
		return nil, errors.Join(ErrInvalidCredentials, errors.New("sensitive verifier detail"))
	})
	handler := Middleware(authenticator)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not run")
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", response.Code)
	}
	if body := response.Body.String(); body == "" || containsString(body, "sensitive") {
		t.Fatalf("response leaked verifier detail: %q", body)
	}
}

func TestRequireAnyScope(t *testing.T) {
	base := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	handler := RequireAnyScope("mcp:use")(base)

	request := httptest.NewRequest("POST", "/mcp", nil)
	request = request.WithContext(WithPrincipal(request.Context(), &Principal{
		Subject: "api-client",
		Scopes:  []string{"mcp:use"},
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("authorized status = %d", response.Code)
	}

	request = httptest.NewRequest("POST", "/mcp", nil)
	request = request.WithContext(WithPrincipal(request.Context(), &Principal{
		Subject: "api-client",
		Scopes:  []string{"presentations:read"},
	}))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("forbidden status = %d", response.Code)
	}
}

func containsString(value, substring string) bool {
	for index := 0; index+len(substring) <= len(value); index++ {
		if value[index:index+len(substring)] == substring {
			return true
		}
	}
	return false
}
