package httpapi

import (
	"encoding/json"
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

// A generated deck is mostly components. The canvas editor knows about text and
// sends back the fields it edited, so replacing content wholesale erased every
// KPI row, timeline and image on the first keystroke.
func TestUpdateKeepsDrawingsAnEditDoesNotMention(t *testing.T) {
	stored := json.RawMessage(`{"type":"template","fields":{"title":[{"text":"현황"}]},` +
		`"blocks":{"body2":{"kind":"kpi","items":[{"label":"전환 대상","value":"42개"}]}},` +
		`"images":{"body3":{"assetId":"a1","name":"로고"}},` +
		`"elements":[{"id":"free-1","kind":"text","x":10,"y":10,"width":20,"height":10,"text":"주석"}]}`)
	edited := json.RawMessage(`{"type":"template","fields":{"title":[{"text":"현황 정리"}]},"bullets":["요점"]}`)

	merged := preserveDrawing(edited, stored)
	var content struct {
		Fields map[string][]struct{ Text string } `json:"fields"`
		Blocks map[string]struct {
			Kind  string `json:"kind"`
			Items []struct {
				Label string `json:"label"`
				Value string `json:"value"`
			} `json:"items"`
		} `json:"blocks"`
		Images map[string]struct {
			AssetID string `json:"assetId"`
		} `json:"images"`
		Elements []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(merged, &content); err != nil {
		t.Fatalf("merged content is not valid JSON: %v", err)
	}
	// The edit is kept.
	if len(content.Fields["title"]) != 1 || content.Fields["title"][0].Text != "현황 정리" {
		t.Fatalf("the edited title should win: %s", merged)
	}
	// The drawings survive it.
	block, ok := content.Blocks["body2"]
	if !ok || block.Kind != "kpi" || len(block.Items) != 1 || block.Items[0].Value != "42개" {
		t.Fatalf("the component should survive an edit that ignores it: %s", merged)
	}
	if content.Images["body3"].AssetID != "a1" {
		t.Fatalf("the image should survive too: %s", merged)
	}
	if len(content.Elements) != 1 || content.Elements[0].ID != "free-1" || content.Elements[0].Text != "주석" {
		t.Fatalf("the freeform element should survive too: %s", merged)
	}
}

// Deleting has to stay possible: an explicit empty object clears the slot.
func TestUpdateClearsDrawingsWhenAskedExplicitly(t *testing.T) {
	stored := json.RawMessage(`{"blocks":{"body":{"kind":"kpi"}},"images":{"body2":{"assetId":"a1"}},"elements":[{"id":"e1"}]}`)
	for _, edited := range []string{
		`{"fields":{},"blocks":{},"images":{},"elements":[]}`,
		`{"fields":{},"blocks":null,"images":null,"elements":null}`,
	} {
		merged := preserveDrawing(json.RawMessage(edited), stored)
		if strings.Contains(string(merged), "kpi") || strings.Contains(string(merged), "a1") || strings.Contains(string(merged), "e1") {
			t.Fatalf("%s should clear the drawings, got %s", edited, merged)
		}
	}
}
