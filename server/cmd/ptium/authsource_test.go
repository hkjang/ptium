package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// storedSettings is a settings service with nothing but a map behind it.
type storedSettings map[string]any

func (s storedSettings) Get(ctx context.Context, key string, target any) error {
	value, ok := s[key]
	if !ok {
		return errors.New("no such setting")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

// An administrator can type the OIDC client secret into the workspace rather
// than into a deployment manifest. What they type is used when the environment
// does not already say otherwise — which is the rule for the issuer and the
// client id, and has to be the same rule for the secret, or a confidential
// client works only for whoever can edit the pod spec.
func TestTheWorkspaceCanHoldTheOIDCClientSecret(t *testing.T) {
	stored := storedSettings{
		"auth.oidc.issuer_url":    "https://sso.example.com/realms/company",
		"auth.oidc.client_id":     "ptium-web",
		"auth.oidc.client_secret": "stored-secret",
		"auth.oidc.admin_roles":   []string{"ptium-admin"},
	}
	source := databaseAuthSource(context.Background(), stored)
	for key, wanted := range map[string]string{
		"OIDC_ISSUER_URL":    "https://sso.example.com/realms/company",
		"OIDC_CLIENT_ID":     "ptium-web",
		"OIDC_CLIENT_SECRET": "stored-secret",
		"AUTH_ADMIN_ROLES":   "ptium-admin",
	} {
		if got, ok := source.Lookup(key); !ok || got != wanted {
			t.Errorf("%s = %q (%v), want %q", key, got, ok, wanted)
		}
	}

	// The environment is how a deployment pins a secret, and it keeps winning.
	t.Setenv("OIDC_CLIENT_SECRET", "from-the-pod-spec")
	source = databaseAuthSource(context.Background(), stored)
	if got, _ := source.Lookup("OIDC_CLIENT_SECRET"); got != "from-the-pod-spec" {
		t.Errorf("with the environment set, the secret is %q", got)
	}
}

// A secret nobody stored is no secret: a public client must not end up sending
// an empty client_secret to the provider.
func TestNoStoredSecretLeavesTheClientPublic(t *testing.T) {
	source := databaseAuthSource(context.Background(), storedSettings{
		"auth.oidc.issuer_url": "https://sso.example.com/realms/company",
		"auth.oidc.client_id":  "ptium-web",
	})
	if got, ok := source.Lookup("OIDC_CLIENT_SECRET"); ok && got != "" {
		t.Errorf("a client with no stored secret got %q", got)
	}
}
