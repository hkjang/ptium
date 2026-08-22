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

// The brief must ask for the same locator the citation check enforces: one the
// brief itself gives, or none.
func TestTheBriefSaysWhereALocatorComesFrom(t *testing.T) {
	for _, wanted := range []string{
		`The part of !source after "|" is where in the source it is`,
		"If the brief does not say where, write the\n  name alone.",
	} {
		if !strings.Contains(sourceSystemPrompt, wanted) {
			t.Fatalf("the writing brief does not say %q", wanted)
		}
	}
}
