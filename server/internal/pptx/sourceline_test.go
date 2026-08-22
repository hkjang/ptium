package pptx

import (
	"regexp"
	"strings"
	"testing"
)

// A figure whose source is only in the speaker notes reads, to everyone who is
// forwarded the file, as a figure with no source. The line goes at the foot of
// the slide, in the band the design already keeps for the page number.
func TestASlideThatCitesSomethingSaysSoOnItsFace(t *testing.T) {
	_, pkg, manifest := buildTemplate(t, "plum-rail")
	layout, _ := manifest.Layout(manifest.DefaultLayout)
	deck := Deck{Title: "출처", Language: "ko", Slides: []Slide{
		{LayoutID: manifest.TitleLayout, Fields: map[string][]Paragraph{SlotTitle: {{Text: "표지"}}}},
		{LayoutID: layout.ID, Fields: map[string][]Paragraph{
			SlotTitle: {{Text: "핵심 진단"}}, SlotBody: {{Text: "이탈 고객의 62%가 온보딩에서 발생"}}},
			Sources: []Citation{{Title: "통계청", Locator: "2026 소비 동향, 표 3"}}},
	}}
	rendered, err := Render(pkg, manifest, deck)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	result, err := Open(rendered)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	slide, _ := result.Text("ppt/slides/slide2.xml")
	if !strings.Contains(slide, "출처: 통계청, 2026 소비 동향, 표 3") {
		t.Fatalf("the slide does not say where its figure came from:\n%s", slide)
	}
	// The page number is still there, and the two do not sit on top of each other.
	if !strings.Contains(slide, `type="slidenum"`) {
		t.Fatalf("the source line displaced the page number:\n%s", slide)
	}
	note := slideSourceNote(layout, deck.Slides[1], "ko")
	if note == nil {
		t.Fatal("no source note was placed")
	}
	if number := layout.SlideNumber; note.X+note.Width > number.X {
		t.Fatalf("the source line runs into the page number: ends at %d, number starts at %d",
			note.X+note.Width, number.X)
	}
	// And the notes page keeps the full list, since the line at the foot is a
	// summary of it rather than a replacement for it.
	notes, _ := result.Text("ppt/notesSlides/notesSlide1.xml")
	if !strings.Contains(notes, "통계청") {
		t.Fatalf("the notes page lost the source:\n%s", notes)
	}
	// The screen shows what the file will.
	svg := PreviewSVG(manifest, layout, deck.Slides[1], PreviewOptions{Width: 640, Language: "ko"})
	if !strings.Contains(svg, "출처: 통계청") {
		t.Fatalf("the preview does not draw the source line:\n%s", svg)
	}
}

// A slide with nothing to cite says nothing, and a design whose content runs
// through the foot of the page keeps its design.
func TestTheSourceLineStaysAwayWhenThereIsNoRoomForIt(t *testing.T) {
	_, _, manifest := buildTemplate(t, "plum-rail")
	layout, _ := manifest.Layout(manifest.DefaultLayout)
	plain := Slide{LayoutID: layout.ID, Fields: map[string][]Paragraph{SlotTitle: {{Text: "제목"}}}}
	if note := slideSourceNote(layout, plain, "ko"); note != nil {
		t.Fatalf("a slide with no sources drew one: %+v", note)
	}
	cited := plain
	cited.Sources = []Citation{{Title: "통계청"}}
	if note := slideSourceNote(layout, cited, "ko"); note == nil {
		t.Fatal("a cited slide drew nothing")
	}
	// A picture pinned across the foot of the page takes the room.
	covered := cited
	covered.Frames = map[string]Frame{SlotTitle: {X: 0, Y: 0, Width: 12192000, Height: 6858000}}
	if note := slideSourceNote(layout, covered, "ko"); note != nil {
		t.Fatalf("the line was drawn over the slide's own content: %+v", note)
	}
	// A template with no page number has no band anyone can rely on.
	bare := layout
	bare.SlideNumber = nil
	if note := slideSourceNote(bare, cited, "ko"); note != nil {
		t.Fatalf("the line was placed on a template that has no foot: %+v", note)
	}
}

// Several sources are numbered the way the notes page numbers them, and a line
// too long for its band is cut rather than run under the page number — what it
// drops is still listed in full on the notes page.
func TestManySourcesAreNumberedAndKeptToOneLine(t *testing.T) {
	line := sourceLine([]Citation{{Title: "통계청"}, {Marker: "*", Title: "내부 조사"}}, "ko")
	if line != "출처: 1. 통계청  *. 내부 조사" {
		t.Fatalf("the line reads %q", line)
	}
	if english := sourceLine([]Citation{{Title: "Statistics Korea"}}, "en"); english != "Source: Statistics Korea" {
		t.Fatalf("the English line reads %q", english)
	}
	long := strings.Repeat("아주 긴 출처 이름 ", 20)
	fitted := fitOneLine("출처: "+long, 1100, 3000000)
	if !strings.HasSuffix(fitted, "…") {
		t.Fatalf("a line too long for its band was not cut: %q", fitted)
	}
	if textWidth(fitted, 1100) > 3000000 {
		t.Fatalf("the cut line is still too wide: %q", fitted)
	}
}

// The preview used to assume PowerPoint's default text padding for every
// template. The shipped designs set it to zero, so every line on screen sat a
// tenth of an inch to the right of where the exported file puts it — visible
// the moment anything else on the slide is aligned to the same edge, which is
// what the source line at the foot of the page is.
func TestThePreviewStartsTextWhereTheFileDoes(t *testing.T) {
	_, _, manifest := buildTemplate(t, "plum-rail")
	layout, _ := manifest.Layout(manifest.DefaultLayout)
	body, ok := layout.Slot(SlotBody)
	if !ok {
		t.Fatal("the content layout has no body")
	}
	if body.TextInset() != 0 {
		t.Fatalf("the shipped design sets lIns=0; the manifest read %d", body.TextInset())
	}
	slide := Slide{LayoutID: layout.ID, Number: 2,
		Fields:  map[string][]Paragraph{SlotTitle: {{Text: "핵심 진단"}}, SlotBody: {{Text: "한 줄"}}},
		Sources: []Citation{{Title: "통계청"}}}
	svg := PreviewSVG(manifest, layout, slide, PreviewOptions{Width: 1280, Language: "ko"})
	starts := regexp.MustCompile(`<text x="([0-9.]+)"[^>]*>(?:<tspan[^>]*>)?(?:출처|• 한 줄|핵심)`).
		FindAllStringSubmatch(svg, -1)
	if len(starts) < 3 {
		t.Fatalf("expected the title, the body and the source line:\n%s", svg)
	}
	for _, start := range starts[1:] {
		if start[1] != starts[0][1] {
			t.Fatalf("the slide's lines start at different edges: %q and %q", starts[0][1], start[1])
		}
	}

	// A template that says nothing keeps PowerPoint's own default.
	quiet := Placeholder{}
	if quiet.TextInset() != DefaultTextInset {
		t.Fatalf("an unmeasured placeholder reads as %d", quiet.TextInset())
	}
}
