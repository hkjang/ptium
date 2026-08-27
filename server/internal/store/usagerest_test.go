package store

import "testing"

// The usage lists are the busiest few, and a list of the busiest few read as a
// full accounting: eight people shown against four and a half thousand decks,
// adding to less than the headline said, with nothing on the screen to say
// where the difference went.
func TestWhatAUsageListLeavesOut(t *testing.T) {
	listed := []UsageCount{{Name: "가", Count: 2134}, {Name: "나", Count: 2059}, {Name: "다", Count: 6}}
	if got := leftOutOf(4484, listed); got != 285 {
		t.Fatalf("4484 decks against 4199 listed left %d out, want 285", got)
	}
}

func TestAListThatNamesEverythingLeavesNothingOut(t *testing.T) {
	listed := []UsageCount{{Name: "가", Count: 40}, {Name: "나", Count: 12}}
	if got := leftOutOf(52, listed); got != 0 {
		t.Fatalf("a complete list left %d out, want 0", got)
	}
	if got := leftOutOf(0, nil); got != 0 {
		t.Fatalf("an empty window left %d out, want 0", got)
	}
}

// The lists and the totals are counted over the same window, so this cannot go
// negative — but a screen saying "그 밖의 -3" would be worse than saying nothing,
// so it does not.
func TestALeftOutCountIsNeverNegative(t *testing.T) {
	if got := leftOutOf(10, []UsageCount{{Count: 40}}); got != 0 {
		t.Fatalf("a list larger than its total left %d out, want 0", got)
	}
}
