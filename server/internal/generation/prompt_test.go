package generation

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/model"
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

// The substitution that puts a registered slide into a deck matches by title,
// and a model left to itself titles the company introduction something of its
// own — so against a real model the slide library almost never fired. The
// brief now offers the names.
func TestTheBriefOffersTheSlidesAlreadyMade(t *testing.T) {
	request := writingRequest{
		Presentation: model.Presentation{Title: "사업 계획", Language: "ko", RequestedSlideCount: 6},
		Template:     Template{Manifest: pptx.Manifest{}},
		Registered:   []string{"회사 소개", "보안 아키텍처"},
	}
	prompt := sourceUserPrompt(request)
	for _, wanted := range []string{"already made and agreed", "- 회사 소개", "- 보안 아키텍처", "write its title exactly"} {
		if !strings.Contains(prompt, wanted) {
			t.Fatalf("the brief does not say %q", wanted)
		}
	}
	// The brief names one of them, so it is not a suggestion.
	request.Presentation.Prompt = "회사 소개와 2026년 사업 계획을 임원에게 보고"
	insisted := sourceUserPrompt(request)
	if !strings.Contains(insisted, "names these by name, so the deck must contain them") {
		t.Fatalf("the brief names a registered slide and the deck was not told to include it")
	}
	if strings.Count(insisted, "- 보안 아키텍처") != 1 {
		t.Fatal("a slide the brief does not name was insisted on")
	}

	// A workspace with no library says nothing about one.
	request.Registered = nil
	if strings.Contains(sourceUserPrompt(request), "already made and agreed") {
		t.Fatal("a deck with no library was told about one")
	}
}
