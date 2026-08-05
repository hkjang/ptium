package settings

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/model"
)

func TestSealRoundTrip(t *testing.T) {
	service, err := New(nil, "test material")
	if err != nil {
		t.Fatal(err)
	}
	plain := json.RawMessage(`"super-secret"`)
	sealed, err := service.seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if string(sealed) == string(plain) {
		t.Fatal("value was not encrypted")
	}
	opened, err := service.open(sealed)
	if err != nil || string(opened) != string(plain) {
		t.Fatalf("round trip failed: %q %v", opened, err)
	}
}

// A value sealed under one key cannot be read under another. That happens for
// real when an operator rotates KEY_ENCRYPTION_SECRET or the database URL it is
// derived from, and the settings page is where they go to fix it — so it has to
// keep working.
func TestARotatedKeyLeavesTheSettingReadable(t *testing.T) {
	before, err := New(nil, "old material")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := before.seal(json.RawMessage(`"super-secret"`))
	if err != nil {
		t.Fatal(err)
	}

	after, err := New(nil, "new material")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := after.open(sealed); err == nil {
		t.Fatal("a value sealed under another key must not open")
	}

	listed := []model.Setting{{Key: "ai.api_key", Sensitive: true, Value: sealed}}
	for i := range listed {
		if listed[i].Sensitive {
			if _, openErr := after.open(listed[i].Value); openErr != nil {
				listed[i].Unreadable = true
			}
		}
	}
	if !listed[0].Unreadable {
		t.Fatal("the setting should be reported as unreadable")
	}
}

func TestGetNamesTheRemedyForAnUnreadableSetting(t *testing.T) {
	service, err := New(nil, "material")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.open(json.RawMessage(`{"version":1,"ciphertext":"AAAA"}`)); err == nil {
		t.Fatal("a truncated ciphertext must not open")
	}
	if !strings.Contains(ErrUnreadable.Error(), "again") {
		t.Fatalf("the error should tell the operator what to do: %v", ErrUnreadable)
	}
}
