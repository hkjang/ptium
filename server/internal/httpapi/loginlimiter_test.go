package httpapi

import (
	"testing"
	"time"
)

// The console recorded who signed in and never recorded anybody failing to, so
// a run of guesses against an administrator's account showed up nowhere an
// administrator looks. Writing down each attempt is not the answer: this
// limiter is in-process precisely so that a failed sign-in is not a database
// write, and anybody who can reach the sign-in page could then fill the audit
// table without an account.
//
// So one row per run, at the point it goes into backoff.
func TestTheLimiterSaysWhenGuessingCrossesIntoBackoff(t *testing.T) {
	limiter := newLoginLimiter()
	held := time.Now()
	limiter.now = func() time.Time { return held }

	crossings := 0
	for attempt := 1; attempt <= 20; attempt++ {
		if limiter.fail("10.0.0.1") {
			crossings++
			if attempt != limiter.threshold+1 {
				t.Errorf("the crossing was reported on attempt %d, want %d", attempt, limiter.threshold+1)
			}
		}
	}
	if crossings != 1 {
		t.Errorf("twenty guesses reported %d crossings, want exactly one", crossings)
	}

	// A different client is its own run.
	if !limiterCrossesAt(newLoginLimiter(), "10.0.0.2") {
		t.Error("another client never reported a crossing of its own")
	}

	// And a valid sign-in clears the run, so the next one is reported again.
	limiter.succeed("10.0.0.1")
	again := 0
	for attempt := 1; attempt <= 8; attempt++ {
		if limiter.fail("10.0.0.1") {
			again++
		}
	}
	if again != 1 {
		t.Errorf("after a valid sign-in the next run reported %d crossings, want one", again)
	}

	// A limiter that is not there, and a request with no address, report
	// nothing rather than panicking on the sign-in path.
	var absent *loginLimiter
	if absent.fail("10.0.0.3") || newLoginLimiter().fail("") {
		t.Error("a crossing was reported where there is nothing to cross")
	}
}

func limiterCrossesAt(limiter *loginLimiter, client string) bool {
	for attempt := 1; attempt <= 10; attempt++ {
		if limiter.fail(client) {
			return true
		}
	}
	return false
}
