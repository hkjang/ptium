package config

import "testing"

func TestLoadRequiresDSN(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PTIUM_DATABASE_DSN", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing DSN error")
	}
}

func TestLoadMinimal(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("DEV_AUTH_ENABLED", "false")
	t.Setenv("OIDC_ISSUER_URL", "")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != ":8080" || c.DatabaseURL != "postgres://example" {
		t.Fatalf("unexpected config: %#v", c)
	}
}
