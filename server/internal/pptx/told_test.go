package pptx

import (
	"os"
	"strings"
	"testing"
)

// What the measurement can say has to be what the API says it can say.
//
// The schema listed seven kinds while the server had twelve, so a client
// validating against it would have refused a perfectly good answer — and the
// five it did not know about were the ones about the writing rather than the
// drawing, which are the findings a person acts on.
func TestEveryFindingKindIsInTheSchema(t *testing.T) {
	schema, err := os.ReadFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("openapi.yaml: %v", err)
	}
	said := string(schema)
	start := strings.Index(said, "enum: [overflow")
	if start < 0 {
		t.Fatal("the schema no longer lists the finding kinds where this test looks")
	}
	listed := said[start : start+strings.Index(said[start:], "]")]
	for _, kind := range []string{
		FindingOverflow, FindingTrimmed, FindingOutside, FindingCollision, FindingContrast,
		FindingOrphan, FindingDensity, FindingNotes, FindingRepeat, FindingLink,
		FindingSource, FindingEcho, FindingUnfinished,
	} {
		if !strings.Contains(listed, kind) {
			t.Errorf("the measurement reports %q and the schema does not list it", kind)
		}
	}
}
