package settings

import (
	"encoding/json"
	"testing"
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
