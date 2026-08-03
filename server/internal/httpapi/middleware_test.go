package httpapi

import (
	"context"
	"testing"

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
