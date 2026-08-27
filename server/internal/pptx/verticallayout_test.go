package pptx

import (
	"strings"
	"testing"
)

// A writer choosing a layout is told which ones set their text vertically.
//
// A layout for traditional CJK typesetting holds more lines than any other,
// which is exactly why anything choosing on capacity lands in one. Automatic
// selection has ranked them last for a while; a model picking a layout by name
// from the summary was never told at all, and a deck of ordinary Korean bullets
// came back in one — its body measured 3.11cm past the slide edge and 55% on
// top of its own title.
func TestTheLayoutSummarySaysWhichLayoutsAreVertical(t *testing.T) {
	manifest := Manifest{
		AspectRatio: "16:9",
		Layouts: []Layout{{
			ID: "vertical-text", Name: "VERTICAL", Role: "content", Type: "vertTx",
			Placeholders: []Placeholder{
				{Slot: SlotTitle, Type: "title", Kind: "text", MaxChars: 40, MaxLines: 1},
				{Slot: "body", Type: "body", Kind: "text", MaxChars: 400, MaxLines: 20, Vertical: true},
			},
		}, {
			ID: "object", Name: "OBJECT", Role: "content", Type: "obj",
			Placeholders: []Placeholder{
				{Slot: SlotTitle, Type: "title", Kind: "text", MaxChars: 40, MaxLines: 1},
				{Slot: "body", Type: "body", Kind: "text", MaxChars: 300, MaxLines: 8},
			},
		}},
	}
	summary := manifest.SummaryFor("ko", 0)
	for _, line := range strings.Split(strings.TrimSpace(summary), "\n") {
		vertical := strings.Contains(line, "id=vertical-text")
		said := strings.Contains(line, "vertical CJK")
		if vertical != said {
			t.Errorf("the summary line does not say what this layout is:\n  %s", line)
		}
	}
}

// A placeholder marked vertical is enough, whatever the layout calls itself:
// a template can lay its text sideways in an ordinary object layout.
func TestAVerticalPlaceholderMakesTheLayoutVertical(t *testing.T) {
	sideways := Layout{ID: "odd", Type: "obj", Placeholders: []Placeholder{
		{Slot: "body", Type: "body", Kind: "text", Vertical: true},
	}}
	if !verticalLayout(sideways) {
		t.Error("a layout whose body is set vertically was not recognised")
	}
	upright := Layout{ID: "plain", Type: "obj", Placeholders: []Placeholder{
		{Slot: "body", Type: "body", Kind: "text"},
	}}
	if verticalLayout(upright) {
		t.Error("an ordinary layout was called vertical")
	}
	// A picture placeholder that happens to carry the flag is not text.
	picture := Layout{ID: "pic", Type: "obj", Placeholders: []Placeholder{
		{Slot: "picture", Type: "pic", Kind: "picture", Vertical: true},
	}}
	if verticalLayout(picture) {
		t.Error("a picture placeholder made the layout read as vertical text")
	}
}
