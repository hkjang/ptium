package export

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

func printedDeck(t *testing.T, source string) ([]byte, pptx.Manifest) {
	t.Helper()
	data, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	compiled := deck.Compile(deck.ParseSource(source), manifest, deck.CompileOptions{Language: "ko"})
	file, err := PDF(model.Presentation{Title: "인쇄", Language: "ko", Slides: compiled.Slides},
		Options{TemplateData: data, Manifest: manifest})
	if err != nil {
		t.Fatalf("pdf: %v", err)
	}
	return file, manifest
}

func TestADeckIsPutOnPaper(t *testing.T) {
	file, manifest := printedDeck(t, "# 분기 실적\n@cover\n> 3분기 요약\n\n# 근거\n@content\n"+
		"- 매출이 12% 늘었습니다\n- 비용은 그대로입니다\n")
	if !strings.HasPrefix(string(file[:8]), "%PDF-1.7") {
		t.Fatalf("that is not a PDF: %q", file[:16])
	}
	body := string(file)
	if strings.Count(body, "/Type /Page ") != 2 {
		t.Errorf("a two-slide deck made %d pages", strings.Count(body, "/Type /Page "))
	}
	// The page is the deck's own shape, in points.
	width := float64(manifest.SlideWidth) / 12700
	if !strings.Contains(body, "/MediaBox [0 0 "+trimZeros(width)) {
		t.Errorf("the page is not the deck's size (%.0f points): %s", width, mediaBox(body))
	}
	if !strings.Contains(body, "/Subtype /Type0") || !strings.Contains(body, "/FontFile2") {
		t.Error("the file does not carry the font its text is set in")
	}
}

// A skipped slide is kept out of the talk, and the handout is what the room is
// given.
func TestASkippedSlideIsNotPrinted(t *testing.T) {
	file, _ := printedDeck(t, "# 본론\n@content\n- 첫 줄\n\n# 부록\n@content\n!skip\n- 물어보면 보여 줄 표\n")
	if pages := strings.Count(string(file), "/Type /Page "); pages != 1 {
		t.Errorf("a deck of one shown slide printed %d pages", pages)
	}
}

func mediaBox(body string) string {
	at := strings.Index(body, "/MediaBox")
	if at < 0 {
		return "no MediaBox"
	}
	return body[at:min(at+40, len(body))]
}

// trimZeros writes a page size the way the file does.
func trimZeros(value float64) string { return strconv.FormatFloat(value, 'f', 3, 64) }

// A deck whose every slide is skipped has nothing to print. That is a deck the
// person can fix — unskip one — and saying so is the difference between a 409
// somebody acts on and a 500 they report as a bug.
func TestADeckWithNothingToPrintSaysSo(t *testing.T) {
	data, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	compiled := deck.Compile(deck.ParseSource("# 부록\n@content\n!skip\n- 물어보면 보여 줄 표\n"),
		manifest, deck.CompileOptions{Language: "ko"})
	_, err = PDF(model.Presentation{Title: "전부 건너뜀", Language: "ko", Slides: compiled.Slides},
		Options{TemplateData: data, Manifest: manifest})
	if !errors.Is(err, ErrNothingToPrint) {
		t.Fatalf("expected the deck to say it has nothing to print, got %v", err)
	}
}
