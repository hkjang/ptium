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

// A citation is drawn on the slide like any other line of text, and read back
// as one it became a point: a deck came in arguing "출처: 내부 자료 2026". It is
// also repeated under the notes on the way out, so reading both gives the deck
// the same source twice.
func TestAnImportedCitationIsACitation(t *testing.T) {
	_, pkg, manifest := buildTemplate(t, "plum-rail")
	content, _ := manifest.Layout(manifest.DefaultLayout)
	rendered, err := Render(pkg, manifest, Deck{Title: "출처", Language: "ko", Slides: []Slide{
		{LayoutID: content.ID, Notes: "말할 내용은 [문서](https://example.com/x)에 있습니다",
			Fields:  map[string][]Paragraph{SlotTitle: {{Text: "근거"}}, SlotBody: {{Text: "요점"}}},
			Sources: []Citation{{Title: "내부 자료 2026"}}},
	}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	stored, err := Open(rendered)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	read := ReadDeck(stored)
	if len(read.Slides) != 1 {
		t.Fatalf("read %d slides, want 1", len(read.Slides))
	}
	slide := read.Slides[0]
	if len(slide.Sources) != 1 || slide.Sources[0] != "내부 자료 2026" {
		t.Errorf("the citation came back as %v", slide.Sources)
	}
	for _, line := range slide.Bullets {
		if strings.Contains(line.Text, "출처") {
			t.Errorf("the citation was read as a point: %q", line.Text)
		}
	}
	if strings.Contains(slide.Notes, "내부 자료 2026") {
		t.Errorf("the citation is in the notes as well: %q", slide.Notes)
	}
	if !strings.Contains(slide.Notes, "말할 내용") {
		t.Errorf("the author's own notes were lost: %q", slide.Notes)
	}
	// A note is where an author puts the address of the thing they will be asked
	// about, and the notes part keeps its own relationships.
	if !strings.Contains(slide.Notes, "https://example.com/x") {
		t.Errorf("a link in the notes lost its address: %q", slide.Notes)
	}
}

// Emphasis brought in from a PowerPoint file has to come back out as emphasis.
//
// The one that was wrong: a run's own text usually carries the space that
// separates it from the next one — "이 부분은 굵게 " — and the importer wrapped
// that space inside the marks. The reader refuses a mark that closes on a
// space, by the rule every markdown reader applies, so nothing turned them back
// into runs: the slide drew "**이 부분은 굵게 ***이 부분은 기울임 *" as
// characters, and the exported file carried no bold at all.
func TestEmphasisComesBackAsEmphasis(t *testing.T) {
	written := markedUpRun("이 부분은 굵게 ", "1", "", "") +
		markedUpRun("이 부분은 기울임 ", "", "1", "") +
		markedUpRun("이 부분은 그냥", "", "", "")
	if strings.Contains(written, "굵게 **") {
		t.Fatalf("the closing mark sits after a space: %q", written)
	}
	runs := SplitRuns(written)
	var bold, italic, plain []string
	for _, run := range runs {
		switch {
		case run.Bold:
			bold = append(bold, run.Text)
		case run.Italic:
			italic = append(italic, run.Text)
		default:
			plain = append(plain, run.Text)
		}
	}
	if len(bold) != 1 || !strings.Contains(bold[0], "굵게") {
		t.Errorf("the bold run came back as %q from %q", bold, written)
	}
	if len(italic) != 1 || !strings.Contains(italic[0], "기울임") {
		t.Errorf("the italic run came back as %q from %q", italic, written)
	}
	// And no mark is left standing as a character anybody would read.
	for _, run := range runs {
		if strings.Contains(run.Text, "*") {
			t.Errorf("a mark is drawn as a character: %q", run.Text)
		}
	}
	if strings.Join(append(append(bold, italic...), plain...), "") == "" {
		t.Error("the words themselves did not survive")
	}
}

// The spaces that separated the runs are still there: emphasis must not glue
// the words together.
func TestTheSpacesBetweenRunsSurvive(t *testing.T) {
	written := markedUpRun("앞 ", "1", "", "") + markedUpRun("뒤", "", "", "")
	whole := ""
	for _, run := range SplitRuns(written) {
		whole += run.Text
	}
	if whole != "앞 뒤" {
		t.Fatalf("the words came back as %q, want %q", whole, "앞 뒤")
	}
}

// A run that is only a space is not something to mark up.
func TestASpaceIsNotEmphasised(t *testing.T) {
	if got := markedUpRun("   ", "1", "1", ""); got != "   " {
		t.Fatalf("a run of spaces was marked up as %q", got)
	}
}
