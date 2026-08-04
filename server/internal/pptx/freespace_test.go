package pptx

import (
	"strings"
	"testing"
)

func TestLargestEmptyRectangleFindsTheOpenColumn(t *testing.T) {
	occupied := make([]bool, freeSpaceGridX*freeSpaceGridY)
	// A sidebar down the left third and a bar across the bottom two rows.
	for y := 0; y < freeSpaceGridY; y++ {
		for x := 0; x < freeSpaceGridX/3; x++ {
			occupied[y*freeSpaceGridX+x] = true
		}
	}
	for y := freeSpaceGridY - 2; y < freeSpaceGridY; y++ {
		for x := 0; x < freeSpaceGridX; x++ {
			occupied[y*freeSpaceGridX+x] = true
		}
	}
	x, y, width, height, ok := largestEmptyRectangle(occupied)
	if !ok {
		t.Fatal("an open region should have been found")
	}
	if x != freeSpaceGridX/3 || y != 0 {
		t.Fatalf("region starts at %d,%d; want %d,0", x, y, freeSpaceGridX/3)
	}
	if width != freeSpaceGridX-freeSpaceGridX/3 || height != freeSpaceGridY-2 {
		t.Fatalf("region is %dx%d; want %dx%d", width, height, freeSpaceGridX-freeSpaceGridX/3, freeSpaceGridY-2)
	}

	// A fully covered slide has nowhere to write.
	for index := range occupied {
		occupied[index] = true
	}
	if _, _, _, _, ok := largestEmptyRectangle(occupied); ok {
		t.Fatal("a fully covered layout must report no free region")
	}
}

func brandedLayout() Layout {
	return Layout{
		ID: "cover", Name: "메인배경", Role: RoleBlank,
		Fill: Background{Fill: "0B1B33"},
		Artwork: []Artwork{
			// A full-bleed photograph is a backdrop, not an obstacle.
			{Kind: "picture", X: 0, Y: 0, Width: 12192000, Height: 6858000, Image: "ppt/media/image1.png", Average: "10203A"},
			// A logo strip along the bottom is an obstacle.
			{Kind: "picture", X: 0, Y: 6000000, Width: 12192000, Height: 858000, Image: "ppt/media/image2.png", Average: "FFFFFF"},
		},
	}
}

func TestSynthesizeSlotsWritesIntoTheFreeArea(t *testing.T) {
	theme := Theme{Colors: map[string]string{"lt1": "FFFFFF", "dk1": "000000"}, MajorLatin: "Arial", MinorLatin: "Arial"}
	layout := brandedLayout()
	slots := synthesizeSlots(layout, theme, 12192000, 6858000, 1400, 1200)
	if len(slots) < 2 {
		t.Fatalf("expected a title and a second region, got %d", len(slots))
	}
	title, body := slots[0], slots[1]
	if title.Slot != SlotTitle || !title.Synthetic || !title.AcceptsText() {
		t.Fatalf("title = %+v", title)
	}
	// The logo strip must be left alone.
	if title.Y+title.Height > 6000000 || body.Y+body.Height > 6000000 {
		t.Fatalf("a region overlaps the logo strip: title %d+%d, body %d+%d",
			title.Y, title.Height, body.Y, body.Height)
	}
	// White type over a dark photograph; the master's 14pt title is ignored
	// because it describes a placeholder this layout does not have.
	if title.Color != "FFFFFF" {
		t.Fatalf("title colour = %s, want white over a dark backdrop", title.Color)
	}
	if title.FontSize < 3000 {
		t.Fatalf("a cover title of %d hundredths of a point is too small", title.FontSize)
	}
	if title.MaxChars <= 0 || title.MaxLines <= 0 {
		t.Fatalf("a synthetic region needs a text budget: %+v", title)
	}
	// A layout that already has a writable slot is left alone.
	withSlots := layout
	withSlots.Placeholders = []Placeholder{{Slot: SlotTitle, Kind: "text"}}
	if extra := synthesizeSlots(withSlots, theme, 12192000, 6858000, 4000, 1800); extra != nil {
		t.Fatalf("a layout with its own placeholders must not be composed: %+v", extra)
	}
}

func TestSynthesizeSlotsKeepsALightBackdropDark(t *testing.T) {
	theme := Theme{Colors: map[string]string{"lt1": "FFFFFF", "dk1": "1A1A1A"}}
	layout := Layout{ID: "light", Name: "본문", Fill: Background{Fill: "FFFFFF"}}
	slots := synthesizeSlots(layout, theme, 12192000, 6858000, 0, 0)
	if len(slots) == 0 {
		t.Fatal("an empty layout is all free space")
	}
	if slots[0].Color != "1A1A1A" {
		t.Fatalf("ink = %s, want the theme's dark colour on white", slots[0].Color)
	}
}

func TestComposedShapeIsAPlainTextBox(t *testing.T) {
	placeholder := Placeholder{
		Slot: SlotTitle, Kind: "text", Type: "title", Synthetic: true, Name: "Ptium title",
		X: 100, Y: 200, Width: 3000000, Height: 800000, FontSize: 3600, Bold: true,
		Color: "FFFFFF", Font: "Pretendard", MaxChars: 40, MaxLines: 2, LineEm: 20,
	}
	xml := placeholderShapeXML(2, placeholder, []Paragraph{{Text: "전환 로드맵"}}, "ko-KR")
	for _, want := range []string{`txBox="1"`, `<a:off x="100" y="200"/>`, `sz="3600"`, `b="1"`,
		`<a:srgbClr val="FFFFFF"/>`, `typeface="Pretendard"`, `<a:buNone/>`, "전환 로드맵"} {
		if !strings.Contains(xml, want) {
			t.Fatalf("composed shape is missing %q:\n%s", want, xml)
		}
	}
	// It must not claim to be a placeholder: there is none to inherit from.
	if strings.Contains(xml, "<p:ph") {
		t.Fatalf("a composed shape must not reference a placeholder:\n%s", xml)
	}
	// A real placeholder keeps inheriting, so it carries no explicit styling.
	real := placeholder
	real.Synthetic = false
	inherited := placeholderShapeXML(2, real, []Paragraph{{Text: "전환 로드맵"}}, "ko-KR")
	if !strings.Contains(inherited, "<p:ph") || strings.Contains(inherited, "sz=\"3600\"") {
		t.Fatalf("a template placeholder must inherit its styling:\n%s", inherited)
	}
}
