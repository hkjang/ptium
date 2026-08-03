package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// MiddlewareOptions allows the server's error-management subsystem to observe
// authentication failures without exposing sensitive verification details to
// clients.
type MiddlewareOptions struct {
	Realm      string
	OnError    func(context.Context, error)
	WriteError func(http.ResponseWriter, *http.Request, int, string)
}

// AuthenticationMiddleware verifies a request and attaches its Principal.
func AuthenticationMiddleware(authenticator Authenticator, options MiddlewareOptions) func(http.Handler) http.Handler {
	realm := strings.TrimSpace(options.Realm)
	if realm == "" {
		realm = "ptium"
	}
	writeError := options.WriteError
	if writeError == nil {
		writeError = writeJSONError
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if authenticator == nil {
				err := errors.New("authentication middleware has no authenticator")
				if options.OnError != nil {
					options.OnError(request.Context(), err)
				}
				writeError(writer, request, http.StatusServiceUnavailable, "authentication_unavailable")
				return
			}

			principal, err := authenticator.Authenticate(request.Context(), request)
			if err != nil {
				if options.OnError != nil {
					options.OnError(request.Context(), err)
				}
				if errors.Is(err, ErrNoCredentials) || errors.Is(err, ErrInvalidCredentials) {
					writer.Header().Set("WWW-Authenticate", `Bearer realm="`+escapeRealm(realm)+`"`)
					writeError(writer, request, http.StatusUnauthorized, "authentication_required")
					return
				}
				writeError(writer, request, http.StatusServiceUnavailable, "authentication_unavailable")
				return
			}

			next.ServeHTTP(writer, request.WithContext(WithPrincipal(request.Context(), principal)))
		})
	}
}

// Middleware is the common zero-option form of AuthenticationMiddleware.
func Middleware(authenticator Authenticator) func(http.Handler) http.Handler {
	return AuthenticationMiddleware(authenticator, MiddlewareOptions{})
}

// RequireAuthenticated rejects requests that have not passed authentication.
func RequireAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := PrincipalFromContext(request.Context()); !ok {
			writeJSONError(writer, request, http.StatusUnauthorized, "authentication_required")
			return
		}
		next.ServeHTTP(writer, request)
	})
}

// RequireAnyRole authorizes a principal that has at least one requested role.
func RequireAnyRole(roles ...string) func(http.Handler) http.Handler {
	required := normalizedRoles(roles)
	return authorize(func(principal *Principal) bool { return principal.HasAnyRole(required...) })
}

// RequireAllRoles authorizes a principal that has every requested role.
func RequireAllRoles(roles ...string) func(http.Handler) http.Handler {
	required := normalizedRoles(roles)
	return authorize(func(principal *Principal) bool { return principal.HasAllRoles(required...) })
}

// RequireAnyScope authorizes API and MCP callers that have at least one
// requested scope. OIDC scope claims and API-key scopes use the same check.
func RequireAnyScope(scopes ...string) func(http.Handler) http.Handler {
	required := normalizedRoles(scopes)
	return authorize(func(principal *Principal) bool { return principal.HasAnyScope(required...) })
}

// RequireAllScopes authorizes callers that have every requested scope.
func RequireAllScopes(scopes ...string) func(http.Handler) http.Handler {
	required := normalizedRoles(scopes)
	return authorize(func(principal *Principal) bool { return principal.HasAllScopes(required...) })
}

// RequireAdmin authorizes configured administrator roles. Calling it without
// roles uses the conventional Ptium role names.
func RequireAdmin(adminRoles ...string) func(http.Handler) http.Handler {
	if len(adminRoles) == 0 {
		adminRoles = []string{"admin", "ptium-admin"}
	}
	return RequireAnyRole(adminRoles...)
}

// IsAdmin provides the non-HTTP equivalent for service-layer checks.
func IsAdmin(ctx context.Context, adminRoles ...string) bool {
	principal, ok := PrincipalFromContext(ctx)
	if !ok {
		return false
	}
	if len(adminRoles) == 0 {
		adminRoles = []string{"admin", "ptium-admin"}
	}
	return principal.HasAnyRole(adminRoles...)
}

func authorize(allowed func(*Principal) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			principal, ok := PrincipalFromContext(request.Context())
			if !ok {
				writeJSONError(writer, request, http.StatusUnauthorized, "authentication_required")
				return
			}
			if allowed == nil || !allowed(principal) {
				writeJSONError(writer, request, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(writer, request)
		})
	}
}

func writeJSONError(writer http.ResponseWriter, _ *http.Request, status int, code string) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"error": map[string]string{"code": code},
	})
}

func escapeRealm(realm string) string {
	return strings.NewReplacer("\\", "", `"`, "").Replace(realm)
}
