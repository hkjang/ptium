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

// The example the brief ends with used to be Korean whatever language was
// asked for, and a model writing English copied what it saw: an English deck
// came back with "::kpi 규모" as a caption, and the example's own citation
// turned up as an invented source in decks that had nothing to do with it.
func TestTheExampleIsInTheDecksLanguage(t *testing.T) {
	korean := exampleDeck("ko")
	if !strings.Contains(korean, "::kpi 규모") {
		t.Fatalf("the Korean example is not Korean: %s", korean)
	}
	for _, language := range []string{"en", "en-GB", "ja", "zh"} {
		example := exampleDeck(language)
		if strings.Contains(example, "규모") || strings.Contains(example, "전환 대상") {
			t.Fatalf("the %s example carries Korean: %s", language, example)
		}
		if !strings.Contains(example, "::kpi Scope") {
			t.Fatalf("the %s example lost its component: %s", language, example)
		}
		if !strings.Contains(example, "Write yours in the requested") {
			t.Fatalf("the %s example does not say it is not the language to write in", language)
		}
	}
	// Both examples teach the same language, so both carry every construct.
	for _, example := range []string{korean, exampleDeck("en")} {
		for _, construct := range []string{"# ", "@cover", "> ", "!notes ", "!source ", "::kpi ", "\n::\n"} {
			if !strings.Contains(example, construct) {
				t.Fatalf("an example does not show %q", construct)
			}
		}
	}
	// The rules themselves no longer carry the example.
	if strings.Contains(sourceSystemPrompt, "전환은 지금") {
		t.Fatal("the example is still nailed into the rules")
	}
}
