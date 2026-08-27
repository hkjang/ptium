package httpapi

import (
	"strconv"
	"strings"
	"testing"
)

// A deck shorter than what was asked for says so, in the language it is being
// written in. The same cap on an imported file and on applied source both
// announce themselves; the author of a new deck was the one person not told.
func TestTheCappedSlidesNoteSaysBothNumbers(t *testing.T) {
	for _, language := range []string{"ko", "", "ja", "zh", "en"} {
		note := cappedSlidesNote(10, 5, language)
		if !strings.Contains(note, "10") {
			t.Errorf("[%s] the note does not say what was asked for: %q", language, note)
		}
		if strings.Count(note, "5") < 1 {
			t.Errorf("[%s] the note does not say what is allowed: %q", language, note)
		}
		if strings.TrimSpace(note) == "" {
			t.Errorf("[%s] the note is empty", language)
		}
	}
}

// The numbers are the deployment's, not a rounded story about them.
func TestTheCappedSlidesNoteCarriesTheRealNumbers(t *testing.T) {
	note := cappedSlidesNote(37, 12, "ko")
	for _, want := range []string{"37", "12"} {
		if !strings.Contains(note, want) {
			t.Fatalf("the note %q does not carry %s", note, want)
		}
	}
	if strings.Contains(note, strconv.Itoa(50)) {
		t.Fatalf("the note %q carries a number nobody asked about", note)
	}
}

// An English deck is told in English rather than in the workspace's own
// language, because the note sits beside the deck's own words.
func TestAnEnglishDeckIsToldInEnglish(t *testing.T) {
	if note := cappedSlidesNote(9, 5, "en"); !strings.Contains(note, "slides") {
		t.Fatalf("an English deck was told %q", note)
	}
	if note := cappedSlidesNote(9, 5, "ko"); !strings.Contains(note, "장") {
		t.Fatalf("a Korean deck was told %q", note)
	}
}

// The MCP refusal says how many were asked for, how many are allowed, and where
// the number came from.
//
// "slideCount exceeds the configured generation limit" named a parameter the
// caller may never have passed — the length can be read out of the brief — and
// gave neither number, so an agent could not tell what to ask for instead.
func TestTheMCPRefusalSaysWhereTheNumberCameFrom(t *testing.T) {
	fromBrief := tooManySlidesAsked(60, 50, true)
	for _, want := range []string{"60", "50", "prompt"} {
		if !strings.Contains(fromBrief, want) {
			t.Errorf("a length read from the brief was refused with %q, missing %q", fromBrief, want)
		}
	}
	if strings.HasPrefix(fromBrief, "slideCount") {
		t.Errorf("a length read from the brief blamed slideCount: %q", fromBrief)
	}
	passed := tooManySlidesAsked(30, 5, false)
	for _, want := range []string{"30", "5", "slideCount"} {
		if !strings.Contains(passed, want) {
			t.Errorf("a passed slideCount was refused with %q, missing %q", passed, want)
		}
	}
	if strings.Contains(passed, "prompt") {
		t.Errorf("a passed slideCount was blamed on the prompt: %q", passed)
	}
}
