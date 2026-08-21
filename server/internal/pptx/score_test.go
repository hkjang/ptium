package pptx

import "testing"

// A score answers the question a list of findings does not: is this ready. It
// is measured from the same findings, so it can always be traced back to them.
func TestADeckScoresWhatWasMeasured(t *testing.T) {
	clean := ScoreDeck(nil, 9)
	if clean.Total != 100 || clean.Weakest != 0 {
		t.Fatalf("a deck with nothing wrong scores %d (weakest %d)", clean.Total, clean.Weakest)
	}

	// One slide drawn wrong, in a deck of nine.
	scored := ScoreDeck([]Finding{
		{Slide: 3, Kind: FindingOverflow, Detail: "…"},
		{Slide: 7, Kind: FindingNotes, Advisory: true, Detail: "…"},
	}, 9)
	if scored.Total >= 100 {
		t.Errorf("a deck with a defect still scores %d", scored.Total)
	}
	if scored.Weakest != 3 {
		t.Errorf("the slide to open first is %d, want the one drawn wrong", scored.Weakest)
	}
	byKey := map[string]DimensionScore{}
	for _, dimension := range scored.Dimensions {
		byKey[dimension.Key] = dimension
	}
	if byKey[DimensionReadability].Score >= byKey[DimensionVisual].Score {
		t.Errorf("the overflow did not land on readability: %+v", scored.Dimensions)
	}
	if byKey[DimensionStructure].Counted != 1 || byKey[DimensionReadability].Counted != 1 {
		t.Errorf("a dimension does not say what it counted: %+v", scored.Dimensions)
	}
	if len(scored.Slides) != 9 {
		t.Errorf("the deck scored %d of its 9 slides", len(scored.Slides))
	}

	// The same fault spread over more slides costs more.
	one := ScoreDeck([]Finding{{Slide: 1, Kind: FindingDensity, Advisory: true}}, 10)
	many := ScoreDeck([]Finding{
		{Slide: 1, Kind: FindingDensity, Advisory: true}, {Slide: 2, Kind: FindingDensity, Advisory: true},
		{Slide: 3, Kind: FindingDensity, Advisory: true}, {Slide: 4, Kind: FindingDensity, Advisory: true},
		{Slide: 5, Kind: FindingDensity, Advisory: true},
	}, 10)
	if !(many.Total < one.Total) {
		t.Errorf("five crowded slides (%d) should score below one (%d)", many.Total, one.Total)
	}
}
