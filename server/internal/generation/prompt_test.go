package generation

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/pptx"
)

// The brief must name the same ceiling the measurement enforces. It said "three
// to five per slide" while the measurement counted points per region and
// reported at seven, so a model could follow the brief and still be marked
// down.
func TestTheBriefNamesTheDensityCeiling(t *testing.T) {
	if pptx.MaximumPoints != 6 {
		t.Fatalf("the brief spells the ceiling out in words; update it for %d", pptx.MaximumPoints)
	}
	for _, wanted := range []string{"never more than six", "in one region"} {
		if !strings.Contains(sourceSystemPrompt, wanted) {
			t.Fatalf("the writing brief does not say %q", wanted)
		}
	}
}
