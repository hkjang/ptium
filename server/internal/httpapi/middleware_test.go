package httpapi

import (
	"context"
	tlspkg "crypto/tls"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hkjang/ptium/server/internal/auth"
)

func TestAllowScopeOnlyRestrictsAPIKeys(t *testing.T) {
	oidc := auth.WithPrincipal(context.Background(), &auth.Principal{Subject: "user", AuthMethod: "oidc"})
	if !allowScope(oidc, "presentations:write") {
		t.Fatal("OIDC identity should use normal user authorization")
	}
	apiKey := auth.WithPrincipal(context.Background(), &auth.Principal{Subject: "api", AuthMethod: "api_key", Scopes: []string{"presentations:read", "mcp:use"}})
	if !allowScope(apiKey, "presentations:read") || allowScope(apiKey, "presentations:write") {
		t.Fatal("API key scopes were not enforced")
	}
}

func TestSessionRenewalExtendsACookieBeforeItLapses(t *testing.T) {
	issuer, err := auth.NewSessionIssuer("a-deployment-secret-of-sufficient-length", 2*time.Hour)
	if err != nil {
		t.Fatalf("NewSessionIssuer: %v", err)
	}
	server := &Server{sessions: issuer, logger: slog.New(slog.DiscardHandler)}
	handler := server.sessionRenewalMiddleware(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))

	renewedCookie := func(state auth.SessionState) *http.Cookie {
		principal := &auth.Principal{Subject: "session:user-1", AuthMethod: "session",
			Claims: map[string]any{auth.SessionStateClaim: state}}
		request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request.WithContext(auth.WithPrincipal(request.Context(), principal)))
		for _, cookie := range recorder.Result().Cookies() {
			if cookie.Name == auth.SessionCookieName {
				return cookie
			}
		}
		return nil
	}

	fresh := auth.SessionState{UserID: "user-1", Epoch: 5, FromCookie: true, ExpiresAt: time.Now().Add(110 * time.Minute)}
	if cookie := renewedCookie(fresh); cookie != nil {
		t.Fatalf("a fresh session must not be reissued on every request, got %+v", cookie)
	}

	expiring := auth.SessionState{UserID: "user-1", Epoch: 5, FromCookie: true, ExpiresAt: time.Now().Add(20 * time.Minute)}
	cookie := renewedCookie(expiring)
	if cookie == nil {
		t.Fatal("a session past half its life must be extended")
	}
	claims, err := issuer.Parse(cookie.Value)
	if err != nil {
		t.Fatalf("the renewed cookie does not verify: %v", err)
	}
	// The epoch is carried over, so a password change still retires the session.
	if claims.UserID != "user-1" || claims.Epoch != 5 {
		t.Fatalf("claims = %+v", claims)
	}
	if time.Until(time.Unix(claims.ExpiresAt, 0)) < 110*time.Minute {
		t.Fatalf("the renewed session expires too soon: %s", time.Unix(claims.ExpiresAt, 0))
	}

	// A bearer token is not a cookie; renewing it would be pointless.
	header := auth.SessionState{UserID: "user-1", Epoch: 5, FromCookie: false, ExpiresAt: time.Now().Add(time.Minute)}
	if cookie := renewedCookie(header); cookie != nil {
		t.Fatalf("a header session must not receive a cookie, got %+v", cookie)
	}
}

func TestSecureRequestFollowsTheProxyProtocol(t *testing.T) {
	cases := map[string]bool{"": false, "http": false, "https": true, "HTTPS": true, "https, http": true, "http, https": false}
	for forwarded, want := range cases {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		if forwarded != "" {
			request.Header.Set("X-Forwarded-Proto", forwarded)
		}
		if got := secureRequest(request); got != want {
			t.Fatalf("secureRequest(X-Forwarded-Proto: %q) = %v, want %v", forwarded, got, want)
		}
	}
	tls := httptest.NewRequest(http.MethodPost, "https://ptium.example.com/api/v1/auth/login", nil)
	tls.TLS = &tlspkg.ConnectionState{}
	if !secureRequest(tls) {
		t.Fatal("a direct TLS request must be secure")
	}
}
