package deck

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/pptx"
)

// Reading a deck's source twice gives the same document.
//
// The slots holding pictures were collected by walking a map, and a map walks
// in a different order every time. A slide with two pictures wrote its two
// ::image lines either way round, so a deck's own source applied back to it
// swapped the pictures between their regions — measured on a real imported
// deck, image15 and image16 changing places on every second save.
func TestASlidesPicturesAreWrittenInTheSameOrderEveryTime(t *testing.T) {
	manifest := pptx.Manifest{
		AspectRatio: "16:9", SlideWidth: 12192000, SlideHeight: 6858000,
		Layouts: []pptx.Layout{{
			ID: "캡션-있는-그림", Name: "PICTURE WITH CAPTION", Role: pptx.RoleContent,
			Placeholders: []pptx.Placeholder{
				{Slot: pptx.SlotTitle, Type: "title", Kind: "text", MaxChars: 40, MaxLines: 1},
				{Slot: "sidePicture", Type: "pic", Kind: "picture", X: 6000000, Y: 1000000, Width: 5000000, Height: 4000000},
				{Slot: "insetPicture", Type: "pic", Kind: "picture", X: 500000, Y: 1000000, Width: 5000000, Height: 4000000},
			},
		}},
	}
	content := Content{LayoutID: "캡션-있는-그림"}
	content.SetImage("sidePicture", ContentImage{AssetID: "aaa", Name: "image15.png"})
	content.SetImage("insetPicture", ContentImage{AssetID: "bbb", Name: "image16.png"})

	first := ""
	for attempt := range 40 {
		written := strings.Join(bodySlotOrder(manifest.Layouts[0], true, content), ",")
		if attempt == 0 {
			first = written
			continue
		}
		if written != first {
			t.Fatalf("the same slide wrote its pictures in two different orders:\n  %q\n  %q", first, written)
		}
	}
	// And the order is the one the template puts them in: the inset sits to the
	// left of the side picture, so it is written first.
	if !strings.HasPrefix(first, "insetPicture") {
		t.Errorf("the pictures are not in the order the template places them: %q", first)
	}
}

// A deck's own source, applied back to it, leaves the pictures where they were.
//
// The writer walked the slide's regions in reading order and the compiler hands
// images out picture-region first, so the two disagreed the moment a slide held
// a picture beside its prose: the first ::image line, written from the prose
// region, was read back into the picture region and the two exchanged places.
// Two of the twelve real decks measured against this product did it on every
// save.
func TestApplyingASlidesOwnSourceLeavesItsPicturesWhereTheyWere(t *testing.T) {
	layout := pptx.Layout{
		ID: "그림-과-글", Name: "PICTURE AND TEXT", Role: pptx.RoleContent,
		Placeholders: []pptx.Placeholder{
			{Slot: pptx.SlotTitle, Type: "title", Kind: "text", MaxChars: 40, MaxLines: 1},
			{Slot: pptx.SlotBody, Type: "body", Kind: "text", X: 500000, Y: 1000000, Width: 5000000, Height: 4000000, MaxChars: 300, MaxLines: 8},
			{Slot: "picture", Type: "pic", Kind: "picture", X: 6000000, Y: 1000000, Width: 5000000, Height: 4000000},
		},
	}
	content := Content{LayoutID: layout.ID}
	content.SetImage(pptx.SlotBody, ContentImage{AssetID: "in-the-prose", Name: "image15.png"})
	content.SetImage("picture", ContentImage{AssetID: "in-the-picture-region", Name: "image16.png"})

	// The order the source hands them back is the order the compiler takes them
	// in: the picture region first.
	queue := picturesInAssignmentOrder(layout, true, content)
	if len(queue.images) != 2 {
		t.Fatalf("the slide carries two pictures, queued %d", len(queue.images))
	}
	if queue.images[0].Name != "image16.png" {
		t.Errorf("the first ::image line is %q; the compiler reads the first one into the picture region",
			queue.images[0].Name)
	}
	if queue.images[1].Name != "image15.png" {
		t.Errorf("the second ::image line is %q, want the prose region's picture", queue.images[1].Name)
	}
}
