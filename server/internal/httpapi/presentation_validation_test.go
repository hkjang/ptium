package httpapi

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/store"
)

func TestExistingPresentationEditUsesAbsoluteSlideLimit(t *testing.T) {
	input := store.PresentationInput{
		Title: "Existing deck", Theme: "modern", Language: "ko", Audience: "general",
		Tone: "professional", SlideCount: 20,
	}
	if message := validatePresentationEditInput(input); message != "" {
		t.Fatalf("existing deck edit was blocked after a lower configured generation limit: %s", message)
	}
	if message := validatePresentationInput(input, 10); !strings.Contains(message, "between 1 and 10") {
		t.Fatalf("new generation validation did not enforce configured limit: %q", message)
	}
}

func TestExistingPresentationEditRetainsAbsoluteMaximum(t *testing.T) {
	input := store.PresentationInput{
		Title: "Existing deck", Theme: "modern", Language: "ko", Audience: "general",
		Tone: "professional", SlideCount: 51,
	}
	if message := validatePresentationEditInput(input); !strings.Contains(message, "between 1 and 50") {
		t.Fatalf("absolute edit limit was not enforced: %q", message)
	}
}
