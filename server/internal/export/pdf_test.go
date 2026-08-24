package export

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pdf"
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

// The handout is a different document from the deck: the slide at the top of
// the page, and what the presenter meant to say under it.
func TestAHandoutCarriesTheNotes(t *testing.T) {
	data, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	source := "# 실적\n@content\n- 매출이 늘었습니다\n!notes 숫자를 읽지 말고 흐름만 말합니다\n\n" +
		"# 노트 없는 장\n@content\n- 요점만\n"
	compiled := deck.Compile(deck.ParseSource(source), manifest, deck.CompileOptions{Language: "ko"})
	presentation := model.Presentation{Title: "유인물", Language: "ko", Slides: compiled.Slides}
	handout, err := PDF(presentation, Options{TemplateData: data, Manifest: manifest, WithNotes: true})
	if err != nil {
		t.Fatalf("handout: %v", err)
	}
	plain, err := PDF(presentation, Options{TemplateData: data, Manifest: manifest})
	if err != nil {
		t.Fatalf("deck: %v", err)
	}
	if pages := strings.Count(string(handout), "/Type /Page "); pages != 2 {
		t.Errorf("a two-slide handout has %d pages", pages)
	}
	// Both are the same deck at the same page size; only what is on the page
	// differs.
	if !strings.Contains(string(handout), "/MediaBox [0 0 960.000 540.000]") ||
		!strings.Contains(string(plain), "/MediaBox [0 0 960.000 540.000]") {
		t.Error("the handout is not the deck's own page size")
	}
	if len(handout) <= len(plain) {
		t.Errorf("the handout (%d) is not longer than the deck (%d), so it carries nothing extra",
			len(handout), len(plain))
	}
}

// The notes are written the way every other line of the deck is. A handout that
// prints [보기](https://…) is the markup reaching a reader — the one thing the
// drawing of a slide never does — and it is also a link nobody can follow.
func TestAHandoutDrawsTheNotesRatherThanTheirMarkup(t *testing.T) {
	data, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	source := "# 근거\n@content\n- 매출이 늘었습니다\n" +
		"!notes 근거는 [분기 보고서](https://reports.example.com/q3)에 있습니다. **꼭** 확인하세요.\n" +
		"!source 매출 | 사내 결산\n"
	compiled := deck.Compile(deck.ParseSource(source), manifest, deck.CompileOptions{Language: "ko"})
	file, err := PDF(model.Presentation{Title: "유인물", Language: "ko", Slides: compiled.Slides},
		Options{TemplateData: data, Manifest: manifest, WithNotes: true})
	if err != nil {
		t.Fatalf("handout: %v", err)
	}
	body := string(file)
	if strings.Contains(body, "/URI (") == false {
		t.Error("the link in the notes is not one the reader can follow")
	}
	if !strings.Contains(body, "reports.example.com/q3") {
		t.Error("the address the notes point at is not in the file")
	}
	// The words, and the citation the notes page of the pptx carries as well.
	for _, wanted := range []string{"분기 보고서", "사내 결산"} {
		if !containsDrawnText(t, file, wanted) {
			t.Errorf("the handout does not draw %q", wanted)
		}
	}
	if containsDrawnText(t, file, "](") || containsDrawnText(t, file, "**") {
		t.Error("the handout prints the markup")
	}
}

// containsDrawnText says whether the drawn text of a PDF holds a string. The
// glyphs are numbers in the file, so what is searched is the map that says what
// each one means, plus the order they were drawn in.
func containsDrawnText(t *testing.T, file []byte, wanted string) bool {
	t.Helper()
	font, err := pdf.BuiltinFont()
	if err != nil {
		t.Fatal(err)
	}
	var hexed strings.Builder
	for _, character := range wanted {
		glyph, ok := font.Glyph(character)
		if !ok {
			return false
		}
		fmt.Fprintf(&hexed, "%04X", glyph)
	}
	return strings.Contains(drawnText(file), hexed.String())
}

// drawnText is every page's drawing, uncompressed. What a PDF holds is a
// deflated stream per page, so searching the file itself finds only what was
// never compressed.
func drawnText(file []byte) string {
	var out strings.Builder
	rest := file
	for {
		at := bytes.Index(rest, []byte("stream\n"))
		if at < 0 {
			return out.String()
		}
		rest = rest[at+len("stream\n"):]
		end := bytes.Index(rest, []byte("\nendstream"))
		if end < 0 {
			return out.String()
		}
		reader, err := zlib.NewReader(bytes.NewReader(rest[:end]))
		if err == nil {
			if inflated, err := io.ReadAll(reader); err == nil {
				out.Write(inflated)
			}
			reader.Close()
		}
		// Past the end of this stream, not into the word "endstream" — where the
		// next search would find "stream" again and inflate from the wrong byte.
		rest = rest[end+len("\nendstream"):]
	}
}
