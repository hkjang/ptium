package httpapi

import (
	"testing"

	"github.com/hkjang/ptium/server/internal/db"
)

// Which settings are secrets is decided in one place — what this product ships
// — and read in another, where a value is hidden or shown. The two disagreed:
// the name rule read "api_key" inside "security.api_key_grace" and made a
// rotation overlap a secret, so an administrator who saved that field never saw
// its value again.
//
// Nothing running would say so. A hidden setting looks exactly like a setting
// meant to be hidden.
func TestEverySettingIsAsSecretAsItIsShipped(t *testing.T) {
	shipped := db.ShippedSettingKeys()
	if len(shipped) < 20 {
		t.Fatalf("only %d settings were read, so this is checking almost nothing", len(shipped))
	}
	secrets := 0
	for _, key := range shipped {
		declared, ok := db.SettingIsSecret(key)
		if !ok {
			t.Errorf("%s is shipped but its sensitivity cannot be read", key)
			continue
		}
		if declared {
			secrets++
		}
		if got := sensitiveSettingKey(key); got != declared {
			t.Errorf("%s: the screen treats it as secret=%v, the product ships it as secret=%v",
				key, got, declared)
		}
	}
	// And the check is reading something with both answers in it.
	if secrets == 0 || secrets == len(shipped) {
		t.Errorf("%d of %d settings are shipped as secrets, so agreement proves nothing", secrets, len(shipped))
	}
}

// A key this deployment has that the product has never heard of still falls to
// the name, which is all there is to go on.
func TestAnUnshippedKeyIsStillReadByItsName(t *testing.T) {
	for key, want := range map[string]bool{
		"nonsense.client_secret": true,
		"nonsense.password":      true,
		"nonsense.thing.secret":  true,
		"nonsense.slide_count":   false,
	} {
		if got := sensitiveSettingKey(key); got != want {
			t.Errorf("%s: secret=%v, want %v", key, got, want)
		}
	}
}
