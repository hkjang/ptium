package keys

import (
	"strings"
	"testing"
)

// The screen that grants a scope used to keep its own list of them, and it had
// drifted: templates:read — which seven routes require and every default key
// carries — could not be granted from the product at all. Both the offered list
// and the validator now read the one catalogue, so neither can drift again.
func TestEveryScopeTheServerAcceptsIsOffered(t *testing.T) {
	offered := map[string]bool{}
	for _, scope := range Scopes(true) {
		offered[scope.ID] = true
		if strings.TrimSpace(scope.Grants) == "" {
			t.Errorf("scope %q is offered without saying what it grants", scope.ID)
		}
	}
	for scope := range userScopes {
		if !offered[scope] {
			t.Errorf("scope %q is accepted but not offered", scope)
		}
	}
	for scope := range adminScopes {
		if !offered[scope] {
			t.Errorf("admin scope %q is accepted but not offered", scope)
		}
	}
	for scope := range offered {
		if err := ValidateScopes([]string{scope}, true); err != nil {
			t.Errorf("scope %q is offered but refused: %v", scope, err)
		}
	}
	// The default a key gets when nobody chooses is one of the offered ones.
	for _, scope := range []string{"presentations:read", "presentations:write", "templates:read", "mcp:use"} {
		if !offered[scope] {
			t.Errorf("the default scope %q is not offered", scope)
		}
	}
}

// An owner who is not an administrator is not offered — or given — the scopes
// only an administrator may hold.
func TestAnOrdinaryOwnerIsOfferedNoAdminScopes(t *testing.T) {
	for _, scope := range Scopes(false) {
		if scope.Admin {
			t.Errorf("%q was offered to an ordinary owner", scope.ID)
		}
	}
	if err := ValidateScopes([]string{"admin:users"}, false); err == nil {
		t.Error("an ordinary owner was allowed an administrator scope")
	}
	if len(Scopes(true)) <= len(Scopes(false)) {
		t.Error("an administrator is offered no more than anybody else")
	}
}
