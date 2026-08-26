package httpapi

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/settings"
)

// A schema that states a range the server does not enforce is a promise to
// every client that reads it. This API documented `ai.timeout_seconds` as
// 10–3600 and `generation.repair_passes` as 0–10 while storing 99999 and 500
// and answering 200 — and then honouring neither. Every setting with a bound
// must appear in the schema, and every bound the schema states must be one the
// server actually refuses outside of.
func TestEverySettingWithABoundIsDocumented(t *testing.T) {
	t.Parallel()
	schema, err := os.ReadFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	written := string(schema)
	for key := range settings.Numbers {
		if !strings.Contains(written, key) {
			t.Errorf("%s is honoured in a range the schema never states", key)
		}
	}
	for key := range settings.Flags {
		if !strings.Contains(written, key) {
			t.Errorf("%s is read as a flag the schema never states", key)
		}
	}
	for key := range settings.Words {
		if !strings.Contains(written, key) {
			t.Errorf("%s is honoured at a few words the schema never states", key)
		}
	}
	// And the server refuses what the bounds say it will not honour.
	for key, bounds := range settings.Numbers {
		for _, outside := range []int{bounds.Low - 1, bounds.High + 1} {
			if err := validateSettingValue(key, jsonNumber(outside)); err == nil {
				t.Errorf("%s stored %d, which it does not honour", key, outside)
			}
		}
		if err := validateSettingValue(key, jsonNumber(bounds.Low)); err != nil {
			t.Errorf("%s refused %d, which it honours: %v", key, bounds.Low, err)
		}
	}
	for key := range settings.Flags {
		if err := validateSettingValue(key, []byte(`"yes"`)); err == nil {
			t.Errorf("%s stored a word where it reads a flag", key)
		}
		if err := validateSettingValue(key, []byte(`true`)); err != nil {
			t.Errorf("%s refused true: %v", key, err)
		}
	}
	for key, words := range settings.Words {
		if err := validateSettingValue(key, []byte(`"something-nobody-implements"`)); err == nil {
			t.Errorf("%s stored a word it does not act on", key)
		}
		if err := validateSettingValue(key, []byte(`"`+words[0]+`"`)); err != nil {
			t.Errorf("%s refused its own value %q: %v", key, words[0], err)
		}
	}
}

func jsonNumber(value int) []byte { return []byte(strconv.Itoa(value)) }
