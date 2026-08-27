package pptx

import "testing"

// Every kind of finding counts against some dimension.
//
// unfinished and twiceTitled were weighted at twelve — the costliest advisories
// here, because the heading is the line the room reads before anything else —
// and were in no dimension, so they never moved the score. A deck whose cover
// stopped mid-sentence measured 100 with the finding printed next to it.
func TestEveryFindingCountsSomewhere(t *testing.T) {
	kinds := []string{
		FindingOverflow, FindingTrimmed, FindingOutside, FindingCollision, FindingContrast,
		FindingOrphan, FindingDensity, FindingNotes, FindingRepeat, FindingLink,
		FindingSource, FindingEcho, FindingUnfinished, FindingTwiceTitled, FindingStale,
	}
	for _, kind := range kinds {
		if dimensionOf[kind] == "" {
			t.Errorf("a %q finding is weighted at %d and counts against no dimension, so it never moves the score",
				kind, weightOf(Finding{Kind: kind, Advisory: true}))
		}
	}
}

// And a cut heading actually costs the deck something.
func TestACutHeadingCostsTheDeck(t *testing.T) {
	clean := ScoreDeck(nil, 8)
	marked := ScoreDeck([]Finding{{Slide: 1, Kind: FindingUnfinished, Advisory: true,
		Detail: "the heading stops in the middle of what it was saying"}}, 8)
	if marked.Total >= clean.Total {
		t.Errorf("a deck with a cut heading scored %d against a clean deck's %d", marked.Total, clean.Total)
	}
}
