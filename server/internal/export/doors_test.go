package export

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// One deck, every door.
//
// The defect this guards against has been found four times in this product,
// each time in a new place: a line of the deck is drawn correctly everywhere it
// was already drawn, and literally in the place somebody just added. The
// brackets of a link on the wall, the stars of a marked word in a handout, an
// address that disappears from the speaker notes. Each fix was to route the new
// drawing through the same splitter as the old ones; none of them stopped the
// next door from being added without it.
//
// So the rule is stated once, over every door at once: what the deck says is
// drawn, and what the author typed to say it is not.
func TestEveryDoorDrawsTheSameDeck(t *testing.T) {
	data, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	source := "# 3분기 실적\n@cover\n> 매출 12% 성장\n\n" +
		"# 근거\n@content\n" +
		"- 자세한 내용은 [분기 보고서](https://reports.example.com/q3)에 있습니다\n" +
		"- **비용은 그대로**이고 *가정은 인건비 동결*입니다\n" +
		"- 근거는 [부록](#3)에 있습니다\n" +
		"!notes 숫자를 읽지 말고 [흐름](https://example.com/flow)만 말합니다\n" +
		"!source 매출 | 사내 결산\n\n" +
		"# 부록\n@content\n- 산출 근거\n"
	compiled := deck.Compile(deck.ParseSource(source), manifest, deck.CompileOptions{Language: "ko"})
	presentation := model.Presentation{Title: "네 개의 문", Language: "ko", Slides: compiled.Slides}

	// The words a reader sees on the slide itself, wherever they are looking at
	// it, and the words that are only ever on the presenter's own page.
	drawn := []string{"분기 보고서", "비용은 그대로", "가정은 인건비 동결", "부록"}
	spoken := []string{"흐름", "사내 결산"}
	// What was typed to put them there. None of it is ever on a screen or a page.
	typed := []string{"](", "**", "https://reports.example.com/q3에", "*가정"}

	pptxFile, err := PPTX(presentation, Options{TemplateData: data, Manifest: manifest})
	if err != nil {
		t.Fatalf("pptx: %v", err)
	}
	pdfFile, err := PDF(presentation, Options{TemplateData: data, Manifest: manifest})
	if err != nil {
		t.Fatalf("pdf: %v", err)
	}
	handout, err := PDF(presentation, Options{TemplateData: data, Manifest: manifest, WithNotes: true})
	if err != nil {
		t.Fatalf("handout: %v", err)
	}
	var drawing strings.Builder
	for position := range presentation.Slides {
		markup, err := PreviewSVG(presentation, manifest, position+1, 960, nil, nil)
		if err != nil {
			t.Fatalf("preview %d: %v", position+1, err)
		}
		drawing.WriteString(markup)
	}

	doors := []struct {
		name string
		// notes says whether this door shows the presenter's own page as well
		// as the slide.
		notes bool
		holds func(string) bool
	}{
		{"the preview", false, func(word string) bool { return strings.Contains(svgText(drawing.String()), word) }},
		{"the exported pptx", true, func(word string) bool { return strings.Contains(pptxText(t, pptxFile), word) }},
		{"the PDF", false, func(word string) bool { return containsDrawnText(t, pdfFile, word) }},
		{"the handout", true, func(word string) bool { return containsDrawnText(t, handout, word) }},
	}
	for _, door := range doors {
		wanted := drawn
		if door.notes {
			wanted = append(append([]string{}, drawn...), spoken...)
		}
		for _, word := range wanted {
			if !door.holds(word) {
				t.Errorf("%s does not draw %q", door.name, word)
			}
		}
		for _, markup := range typed {
			if door.holds(markup) {
				t.Errorf("%s draws %q, which is markup and not words", door.name, markup)
			}
		}
	}
}

// svgText is the words a drawing draws, without its tags.
func svgText(drawing string) string {
	var out strings.Builder
	inside := false
	for _, character := range drawing {
		switch {
		case character == '<':
			inside = true
			out.WriteByte('\n')
		case character == '>':
			inside = false
		case !inside:
			out.WriteRune(character)
		}
	}
	return out.String()
}

// pptxText is the words an exported file draws: the text of every run, on every
// slide and on every notes page.
func pptxText(t *testing.T, file []byte) string {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	for _, entry := range archive.File {
		if !strings.HasPrefix(entry.Name, "ppt/slides/slide") && !strings.HasPrefix(entry.Name, "ppt/notesSlides/") {
			continue
		}
		opened, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(opened)
		opened.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, part := range strings.Split(string(body), "<a:t>")[1:] {
			if end := strings.Index(part, "</a:t>"); end >= 0 {
				out.WriteString(part[:end])
				out.WriteByte('\n')
			}
		}
	}
	return out.String()
}
