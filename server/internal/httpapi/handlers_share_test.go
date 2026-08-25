package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/hkjang/ptium/server/internal/model"
)

// A slide marked skipped is one the author took out of the talk: PowerPoint
// hides it, the handout leaves it out — and a link handed to somebody else was
// drawing it, title and points, to anyone who held the link.
func TestASharedLinkShowsOnlyTheSlidesInTheShow(t *testing.T) {
	skipped, err := json.Marshal(map[string]any{"type": "template", "skipped": true})
	if err != nil {
		t.Fatal(err)
	}
	shownContent, err := json.Marshal(map[string]any{"type": "template"})
	if err != nil {
		t.Fatal(err)
	}
	presentation := model.Presentation{Slides: []model.Slide{
		{ID: "1", Position: 1, Title: "보이는 장", Content: shownContent},
		{ID: "2", Position: 2, Title: "숨긴 장", Content: skipped},
		{ID: "3", Position: 3, Title: "세 번째 장", Content: shownContent},
	}}
	shown := shownSlides(presentation)
	if len(shown) != 2 {
		t.Fatalf("a link shows %d slides, want 2", len(shown))
	}
	for _, slide := range shown {
		if slide.Title == "숨긴 장" {
			t.Errorf("a slide the author took out of the show is on the link")
		}
	}
	// The link counts what it shows, and draws the slide that number stands for.
	if shown[1].Position != 3 {
		t.Errorf("the link's second slide draws position %d, want 3", shown[1].Position)
	}
	if slideIsSkipped(presentation.Slides[0]) || !slideIsSkipped(presentation.Slides[1]) {
		t.Errorf("the skipped mark is read wrong")
	}
	// A slide with no content at all is not hidden by accident.
	if slideIsSkipped(model.Slide{}) {
		t.Errorf("a slide with no content was treated as skipped")
	}
}
