package auth

import (
	"context"
	"sort"
	"strings"
)

// Principal is the authenticated identity made available to HTTP handlers.
// Claims is populated for OIDC identities and must be treated as read-only.
type Principal struct {
	Subject    string
	Email      string
	Name       string
	Issuer     string
	Roles      []string
	Scopes     []string
	AuthMethod string
	Claims     map[string]any
}

// HasScope reports whether the principal has an exact API/MCP scope.
func (p *Principal) HasScope(scope string) bool {
	if p == nil || scope == "" {
		return false
	}
	for _, candidate := range p.Scopes {
		if candidate == scope {
			return true
		}
	}
	return false
}

// HasAnyScope reports whether the principal has at least one requested scope.
func (p *Principal) HasAnyScope(scopes ...string) bool {
	for _, scope := range scopes {
		if p.HasScope(scope) {
			return true
		}
	}
	return false
}

// HasAllScopes reports whether the principal has every requested scope.
func (p *Principal) HasAllScopes(scopes ...string) bool {
	if p == nil || len(scopes) == 0 {
		return false
	}
	for _, scope := range scopes {
		if !p.HasScope(scope) {
			return false
		}
	}
	return true
}

// HasRole reports whether the principal has role. Role comparison is exact and
// case-sensitive, matching OIDC/Keycloak role semantics.
func (p *Principal) HasRole(role string) bool {
	if p == nil || role == "" {
		return false
	}
	for _, candidate := range p.Roles {
		if candidate == role {
			return true
		}
	}
	return false
}

// HasAnyRole reports whether the principal has at least one of roles.
func (p *Principal) HasAnyRole(roles ...string) bool {
	for _, role := range roles {
		if p.HasRole(role) {
			return true
		}
	}
	return false
}

// HasAllRoles reports whether the principal has every role. An empty role list
// is intentionally not considered authorization.
func (p *Principal) HasAllRoles(roles ...string) bool {
	if p == nil || len(roles) == 0 {
		return false
	}
	for _, role := range roles {
		if !p.HasRole(role) {
			return false
		}
	}
	return true
}

// Clone returns a copy that handlers can safely enrich without changing the
// authenticator's cached data.
func (p *Principal) Clone() *Principal {
	if p == nil {
		return nil
	}
	clone := *p
	clone.Roles = append([]string(nil), p.Roles...)
	clone.Scopes = append([]string(nil), p.Scopes...)
	if p.Claims != nil {
		clone.Claims = make(map[string]any, len(p.Claims))
		for key, value := range p.Claims {
			clone.Claims[key] = value
		}
	}
	return &clone
}

type principalContextKey struct{}

// WithPrincipal attaches principal to ctx. A nil principal is ignored.
func WithPrincipal(ctx context.Context, principal *Principal) context.Context {
	if principal == nil {
		return ctx
	}
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// PrincipalFromContext returns the request identity, if authentication ran.
func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(*Principal)
	return principal, ok && principal != nil && principal.Subject != ""
}

func normalizedRoles(roles ...[]string) []string {
	seen := make(map[string]struct{})
	for _, list := range roles {
		for _, role := range list {
			role = strings.TrimSpace(role)
			if role != "" {
				seen[role] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for role := range seen {
		result = append(result, role)
	}
	sort.Strings(result)
	return result
}
