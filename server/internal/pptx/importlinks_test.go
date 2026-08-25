package pptx

import (
	"strings"
	"testing"
)

// A deck read back from a file keeps what its runs said, not only what they
// spelled. An address is the one part of a link that cannot be typed again from
// looking at the slide, and it was dropped: the words of the link came across
// and the link did not.
func TestAnImportedDeckKeepsItsLinksAndEmphasis(t *testing.T) {
	_, pkg, manifest := buildTemplate(t, "plum-rail")
	content, _ := manifest.Layout(manifest.DefaultLayout)
	deck := Deck{Title: "링크", Language: "ko", Slides: []Slide{
		{LayoutID: content.ID, Fields: map[string][]Paragraph{
			SlotTitle: {{Text: "자료"}},
			SlotBody: {
				{Text: "자료는 [계획서](https://example.com/plan)에 있습니다"},
				{Text: "**전환 대상** 42개와 *검토 중* 3개"},
			},
		}, Notes: "말할 내용"},
		{LayoutID: content.ID, Fields: map[string][]Paragraph{SlotTitle: {{Text: "두 번째 장"}}}},
	}}
	rendered, err := Render(pkg, manifest, deck)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	stored, err := Open(rendered)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	read := ReadDeck(stored)
	if len(read.Slides) == 0 {
		t.Fatal("nothing was read back")
	}
	said := ""
	for _, line := range read.Slides[0].Bullets {
		said += line.Text + "\n"
	}
	for _, want := range []string{"[계획서](https://example.com/plan)", "**전환 대상**", "*검토 중*"} {
		if !strings.Contains(said, want) {
			t.Errorf("the import lost %q; it read:\n%s", want, said)
		}
	}
}

// A deck is read the way it is read on the wall: down the page, then across.
// The file stores shapes in drawing order — every text box, then the pictures,
// then the frames, and within each whatever order they were last touched in —
// so a deck written by hand came back with its argument out of order, which is
// the one thing an import is for.
func TestAnImportedSlideIsReadDownThePage(t *testing.T) {
	_, pkg, manifest := buildTemplate(t, "plum-rail")
	content, _ := manifest.Layout(manifest.DefaultLayout)
	rendered, err := Render(pkg, manifest, Deck{Title: "순서", Language: "ko", Slides: []Slide{
		{LayoutID: content.ID, Fields: map[string][]Paragraph{SlotTitle: {{Text: "순서"}}}},
	}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	stored, err := Open(rendered)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Three plain text boxes, written bottom-first, as a hand-edited deck has
	// them. The two at the top are a row a hair out of alignment.
	stored.SetText("ppt/slides/slide1.xml", `<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree>
	<p:sp><p:nvSpPr><p:cNvPr id="2" name="TextBox 2"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="800000" y="4000000"/><a:ext cx="3000000" cy="600000"/></a:xfrm></p:spPr><p:txBody><a:p><a:r><a:t>세 번째</a:t></a:r></a:p></p:txBody></p:sp>
	<p:sp><p:nvSpPr><p:cNvPr id="3" name="TextBox 3"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="4200000" y="1210000"/><a:ext cx="3000000" cy="600000"/></a:xfrm></p:spPr><p:txBody><a:p><a:r><a:t>두 번째</a:t></a:r></a:p></p:txBody></p:sp>
	<p:sp><p:nvSpPr><p:cNvPr id="4" name="TextBox 4"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="800000" y="1200000"/><a:ext cx="3000000" cy="600000"/></a:xfrm></p:spPr><p:txBody><a:p><a:r><a:t>첫 번째</a:t></a:r></a:p></p:txBody></p:sp>
	</p:spTree></p:cSld></p:sld>`)
	read := ReadDeck(stored)
	if len(read.Slides) != 1 {
		t.Fatalf("read %d slides, want 1", len(read.Slides))
	}
	// The first line becomes the title when the slide has no title placeholder.
	said := []string{read.Slides[0].Title}
	for _, line := range read.Slides[0].Bullets {
		said = append(said, line.Text)
	}
	want := "첫 번째 두 번째 세 번째"
	if strings.Join(said, " ") != want {
		t.Errorf("read %q, want %q", strings.Join(said, " "), want)
	}
}

// A slide the author took out of the show stays out of it. Carrying it in as an
// ordinary slide is not losing something — it is putting something back in
// front of a room that somebody decided a room should not see.
func TestAHiddenSlideIsStillHiddenAfterImport(t *testing.T) {
	_, pkg, manifest := buildTemplate(t, "plum-rail")
	content, _ := manifest.Layout(manifest.DefaultLayout)
	rendered, err := Render(pkg, manifest, Deck{Title: "숨김", Language: "ko", Slides: []Slide{
		{LayoutID: content.ID, Fields: map[string][]Paragraph{
			SlotTitle: {{Text: "보이는 장"}}, SlotBody: {{Text: "요점"}}}, Notes: "말할 내용"},
		{LayoutID: content.ID, Skipped: true, Fields: map[string][]Paragraph{
			SlotTitle: {{Text: "숨긴 장"}}, SlotBody: {{Text: "아직 공유하지 않을 내용"}}}, Notes: "말할 내용"},
	}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	stored, err := Open(rendered)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	read := ReadDeck(stored)
	if len(read.Slides) != 2 {
		t.Fatalf("read %d slides, want 2", len(read.Slides))
	}
	if read.Slides[0].Hidden {
		t.Errorf("the first slide is part of the show and was read as hidden")
	}
	if !read.Slides[1].Hidden {
		t.Errorf("a slide written with show=\"0\" was read as part of the show")
	}
}
