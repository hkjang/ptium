package httpapi

import (
	"testing"

	"github.com/hkjang/ptium/server/internal/store"
)

// A deck imported before its required fields were filled could be looked at and
// never changed: every edit validated the row as it stood and failed on what
// was already missing. What is missing is filled from the deployment's
// defaults, and what the deck says is left alone.
func TestADeckMissingItsRequiredFieldsIsStillEditable(t *testing.T) {
	defaults := store.PresentationInput{Theme: "slate-classic", Language: "ko",
		Audience: "general", Tone: "professional", SlideCount: 8}
	imported := store.PresentationInput{Title: "가져온 덱", Language: "ja", SlideCount: 4}
	fillMissingFrom(&imported, defaults)
	if message := validatePresentationEditInput(imported); message != "" {
		t.Fatalf("an imported deck cannot be edited: %s", message)
	}
	if imported.Language != "ja" {
		t.Fatalf("the deck's own language was overwritten: %q", imported.Language)
	}
	if imported.SlideCount != 4 {
		t.Fatalf("the deck's own slide count was overwritten: %d", imported.SlideCount)
	}
	if imported.Theme != "slate-classic" || imported.Audience != "general" || imported.Tone != "professional" {
		t.Fatalf("the missing fields were not filled: %+v", imported)
	}
}
