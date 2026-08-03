package keys

import (
	"crypto/sha256"
	"testing"
)

func TestEqualHash(t *testing.T) {
	a := sha256.Sum256([]byte("a"))
	b := sha256.Sum256([]byte("b"))
	if !equalHash(a[:], a[:]) {
		t.Fatal("same hash should match")
	}
	if equalHash(a[:], b[:]) || equalHash(a[:1], b[:]) {
		t.Fatal("different hashes must not match")
	}
}

func TestValidateScopesRejectsUnknownAndAdminEscalation(t *testing.T) {
	if ValidateScopes([]string{"presentations:read"}, false) != nil {
		t.Fatal("known user scope rejected")
	}
	if ValidateScopes([]string{"unknown"}, true) == nil {
		t.Fatal("unknown scope accepted")
	}
	if ValidateScopes([]string{"admin:settings"}, false) == nil {
		t.Fatal("non-admin received admin scope")
	}
	if ValidateScopes([]string{"admin:settings"}, true) != nil {
		t.Fatal("admin scope rejected for admin")
	}
}

func TestTokenPrefixAllowsURLSafeSecretUnderscores(t *testing.T) {
	prefix, ok := tokenPrefix("ptium_012345abcdef_secret_with_under_scores")
	if !ok || prefix != "012345abcdef" {
		t.Fatalf("token prefix parsing failed: prefix=%q ok=%v", prefix, ok)
	}
	for _, invalid := range []string{"", "ptium_short_secret", "other_012345abcdef_secret", "ptium_012345abcdef_"} {
		if _, ok := tokenPrefix(invalid); ok {
			t.Fatalf("invalid token accepted: %q", invalid)
		}
	}
}
