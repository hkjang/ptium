package pptx

import (
	"strings"
	"testing"
)

// A deck Ptium exported carries the text it was written from, so importing it
// gives the author their components back rather than a reading of the drawing.
func TestAnExportedDeckCarriesItsOwnSource(t *testing.T) {
	template, err := BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	source := "# 이행 계획 & 비용\n::steps\n- 준비 | 범위 확정 <2월>\n- 이행 | 이관\n- 안정화 | 점검\n::\n" +
		"!source 통계청 | 2026 소비 동향 | 표 3\n"
	rendered, err := RenderBytes(template, Deck{Title: "왕복", Language: "ko", Source: source,
		Slides: []Slide{{LayoutID: "제목-및-내용", Fields: map[string][]Paragraph{SlotTitle: {{Text: "이행 계획 & 비용"}}}}}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	pkg, err := Open(rendered)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	read, ok := DeckSource(pkg)
	if !ok {
		t.Fatal("the exported file does not carry its source")
	}
	if read != strings.TrimSpace(source) {
		t.Fatalf("the source came back changed:\n%q\nwant\n%q", read, strings.TrimSpace(source))
	}
	// The part hangs off the presentation, so an editor that rewrites the file
	// has a reason to keep it.
	related := false
	for _, relationship := range pkg.Relationships("ppt/presentation.xml") {
		if strings.HasSuffix(relationship.Target, "ptiumSource.xml") {
			related = true
		}
	}
	if !related {
		t.Fatal("the source part is an orphan")
	}

	// A deck with no source of its own carries no part, and a file from anywhere
	// else reads as one without.
	plain, err := RenderBytes(template, Deck{Title: "왕복", Language: "ko",
		Slides: []Slide{{LayoutID: "제목-및-내용", Fields: map[string][]Paragraph{SlotTitle: {{Text: "제목"}}}}}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	other, err := Open(plain)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, ok := DeckSource(other); ok {
		t.Fatal("a deck with no source carries one anyway")
	}
}

// Someone who opens the export in PowerPoint, fixes a number and sends it back
// has made the embedded source out of date. Restoring from it would throw their
// edit away without saying so, so a file whose slides no longer match is read
// from its shapes like anyone else's.
func TestAFileEditedElsewhereIsNotRestoredFromItsOldSource(t *testing.T) {
	template, err := BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	rendered, err := RenderBytes(template, Deck{Title: "왕복", Language: "ko",
		Source: "# 분기 실적\n- 매출 1,240억\n",
		Slides: []Slide{{LayoutID: "제목-및-내용", Fields: map[string][]Paragraph{
			SlotTitle: {{Text: "분기 실적"}}, SlotBody: {{Text: "매출 1,240억"}}}}}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	pkg, err := Open(rendered)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, ok := DeckSource(pkg); !ok {
		t.Fatal("the untouched file does not carry its source")
	}

	// The number is corrected in PowerPoint and the file comes back.
	slide, _ := pkg.Text("ppt/slides/slide1.xml")
	pkg.SetText("ppt/slides/slide1.xml", strings.Replace(slide, "매출 1,240억", "매출 1,310억", 1))
	if _, ok := DeckSource(pkg); ok {
		t.Fatal("an edited file was restored from the source it no longer matches")
	}
}

// The same, for the pane the speaker actually writes in.
//
// Fingerprinting only the slides let a note edited in PowerPoint through: every
// word on the slides still matched, so the source was restored whole and what
// the speaker had written to say was thrown away — with the import saying it
// had taken the file "as it was".
func TestANoteEditedElsewhereIsNotRestoredFromItsOldSource(t *testing.T) {
	template, err := BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	rendered, err := RenderBytes(template, Deck{Title: "노트 왕복", Language: "ko",
		Source: "# 분기 실적\n!notes 예산 질문이 나오면 이 장에서 답합니다\n- 매출 1,240억\n",
		Slides: []Slide{{LayoutID: "제목-및-내용", Notes: "예산 질문이 나오면 이 장에서 답합니다",
			Fields: map[string][]Paragraph{
				SlotTitle: {{Text: "분기 실적"}}, SlotBody: {{Text: "매출 1,240억"}}}}}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	pkg, err := Open(rendered)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, ok := DeckSource(pkg); !ok {
		t.Fatal("the untouched file does not carry its source")
	}

	// The note is rewritten in PowerPoint. Not a word on the slides changes.
	notes, ok := pkg.Text("ppt/notesSlides/notesSlide1.xml")
	if !ok {
		t.Fatal("the rendered file carries no notes to edit")
	}
	if !strings.Contains(notes, "예산 질문") {
		t.Fatalf("the note is not in the notes slide: %.200s", notes)
	}
	pkg.SetText("ppt/notesSlides/notesSlide1.xml",
		strings.Replace(notes, "예산 질문이 나오면 이 장에서 답합니다", "가격 질문이 나오면 다음 장으로 넘기세요", 1))
	if _, ok := DeckSource(pkg); ok {
		t.Fatal("a file whose notes were edited was restored from the source it no longer matches")
	}
}
