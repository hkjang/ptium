package auth

import (
	"strings"
	"testing"
)

func TestLoadBootstrapConfigOIDCAndKeycloakAliases(t *testing.T) {
	t.Run("generic issuer with optional audience", func(t *testing.T) {
		config, err := LoadBootstrapConfig(MapSource{
			"OIDC_ISSUER_URL": "https://id.example.com/realms/ptium/",
			"OIDC_CLIENT_ID":  "ptium-web",
		})
		if err != nil {
			t.Fatalf("LoadBootstrapConfig() error = %v", err)
		}
		if got, want := config.OIDC.Issuer, "https://id.example.com/realms/ptium"; got != want {
			t.Fatalf("issuer = %q, want %q", got, want)
		}
		if len(config.OIDC.Audiences) != 0 {
			t.Fatalf("audiences = %v, want optional/empty", config.OIDC.Audiences)
		}
	})

	t.Run("keycloak base URL and realm", func(t *testing.T) {
		config, err := LoadBootstrapConfig(MapSource{
			"KEYCLOAK_URL":   "https://sso.example.com/",
			"KEYCLOAK_REALM": "Ptium Team",
			"OIDC_CLIENT_ID": "ptium-api",
			"OIDC_AUDIENCE":  "ptium-api,ptium-mcp",
		})
		if err != nil {
			t.Fatalf("LoadBootstrapConfig() error = %v", err)
		}
		if got, want := config.OIDC.Issuer, "https://sso.example.com/realms/Ptium%20Team"; got != want {
			t.Fatalf("issuer = %q, want %q", got, want)
		}
		if got := strings.Join(config.OIDC.Audiences, ","); got != "ptium-api,ptium-mcp" {
			t.Fatalf("audiences = %q", got)
		}
	})
}

func TestLoadBootstrapConfigDevAuthRequiresStrongSecret(t *testing.T) {
	_, err := LoadBootstrapConfig(MapSource{
		"DEV_AUTH_ENABLED": "true",
		"DEV_AUTH_SECRET":  "short",
	})
	if err == nil || !strings.Contains(err.Error(), "at least 32") {
		t.Fatalf("error = %v, want minimum-secret error", err)
	}

	config, err := LoadBootstrapConfig(MapSource{
		"DEV_AUTH_ENABLED": "true",
		"DEV_AUTH_SECRET":  strings.Repeat("s", 32),
		"DEV_AUTH_EMAIL":   "dev@example.com",
	})
	if err != nil {
		t.Fatalf("LoadBootstrapConfig() error = %v", err)
	}
	if config.Dev.Header != "X-Ptium-Dev-Secret" {
		t.Fatalf("header = %q", config.Dev.Header)
	}
	if config.Dev.Principal.Subject != "dev:dev@example.com" {
		t.Fatalf("subject = %q", config.Dev.Principal.Subject)
	}
}

func TestLoadBootstrapConfigOIDCAdminRoleAlias(t *testing.T) {
	config, err := LoadBootstrapConfig(MapSource{"OIDC_ADMIN_ROLES": "realm-admin,ptium-owner"})
	if err != nil {
		t.Fatalf("LoadBootstrapConfig() error = %v", err)
	}
	if got := strings.Join(config.AdminRoles, ","); got != "ptium-owner,realm-admin" {
		t.Fatalf("admin roles = %q", got)
	}
}
