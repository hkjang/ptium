package store

import "testing"

// The same fault on two decks is one fault.
//
// A 5xx is recorded with the path it happened on, and a path carries the deck's
// id. The normaliser replaced runs of eight hex characters and runs of four
// digits, and a UUID's middle groups are four characters of mixed hex — so
// every deck opened its own incident group and the error centre showed one
// configuration as five separate problems.
func TestOneFaultOnManyDecksIsOneGroup(t *testing.T) {
	first := normalizeError("HTTP 503 on POST /api/v1/presentations/21bfa589-1147-43cc-97a2-2087d8fb4620/slides/1/revise")
	second := normalizeError("HTTP 503 on POST /api/v1/presentations/9a497bde-83e2-45ac-a733-9b34bfae3d85/slides/1/revise")
	if first != second {
		t.Errorf("two decks fingerprint apart:\n  %q\n  %q", first, second)
	}
	// And a message that differs in what actually happened still differs.
	other := normalizeError("HTTP 500 on POST /api/v1/presentations/9a497bde-83e2-45ac-a733-9b34bfae3d85/export")
	if other == first {
		t.Error("two different faults were folded into one group")
	}
}
