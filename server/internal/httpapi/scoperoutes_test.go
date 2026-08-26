package httpapi

import (
	"os"
	"regexp"
	"testing"

	"github.com/hkjang/ptium/server/internal/keys"
)

// Every scope a route requires has to be one a key can be given. The screen
// that grants them reads the catalogue, so a route guarded by a scope missing
// from it is a door no API key can ever open — which is what templates:read
// was: seven routes required it and nothing could grant it.
func TestEveryScopeARouteRequiresCanBeGranted(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read the routing table: %v", err)
	}
	grantable := map[string]bool{}
	for _, scope := range keys.Scopes(true) {
		grantable[scope.ID] = true
	}
	required := regexp.MustCompile(`requireScope\("([^"]+)"`)
	seen := map[string]bool{}
	for _, match := range required.FindAllStringSubmatch(string(source), -1) {
		seen[match[1]] = true
	}
	if len(seen) < 5 {
		t.Fatalf("only %d scopes found in the routing table; the reading is wrong", len(seen))
	}
	for scope := range seen {
		if !grantable[scope] {
			t.Errorf("routes require %q and no key can be given it", scope)
		}
	}
}
