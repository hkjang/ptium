package export

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pdf"
	"github.com/hkjang/ptium/server/internal/pdftext"
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
	// Longer is not "carries the notes". Read both files back and look.
	if said := printedWords(t, handout); !strings.Contains(said, "숫자를 읽지 말고 흐름만 말합니다") {
		t.Errorf("the handout does not have the note on it:\n%s", said)
	}
	if said := printedWords(t, plain); strings.Contains(said, "숫자를 읽지 말고") {
		t.Errorf("the deck has the speaker's note printed on the slide:\n%s", said)
	}
}

// printedWords is what a printed deck says, read back with the same reader the
// product hands other people's PDFs to.
func printedWords(t *testing.T, file []byte) string {
	t.Helper()
	read, err := pdftext.Read(file)
	if err != nil {
		t.Fatalf("reading back what we printed: %v", err)
	}
	var said strings.Builder
	for _, page := range read.Pages {
		for _, line := range page.Lines {
			said.WriteString(line)
			said.WriteString("\n")
		}
	}
	return said.String()
}

// A printed deck is not a picture of a deck: the words on it are words.
//
// A PDF whose glyphs carry no map from their codes back to characters draws
// exactly the same page and cannot be searched, copied out, or read aloud by
// anything — and nothing about the file looks wrong until somebody tries. The
// check is to read it back: this is the reader that refuses to guess, so what
// it finds is really in the file.
func TestAPrintedDeckCanBeReadBack(t *testing.T) {
	file, _ := printedDeck(t, "# 분기 실적\n@cover\n> 3분기 요약\n\n# 근거\n@content\n"+
		"- 매출이 12% 늘었습니다\n- 비용은 그대로입니다\n")
	said := printedWords(t, file)
	for _, want := range []string{"분기 실적", "3분기 요약", "근거", "매출이 12% 늘었습니다", "비용은 그대로입니다"} {
		if !strings.Contains(said, want) {
			t.Errorf("what was printed does not say %q:\n%s", want, said)
		}
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

// The built-in face covers Korean, Latin and kana. A deck written in Japanese
// 新字体 or simplified Chinese reaches past it, and those characters are drawn
// as blank space — so the export says which ones, rather than letting the
// author find out in print.
func TestThePDFSaysWhatItCouldNotDraw(t *testing.T) {
	data, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	printed := func(source, language string) []rune {
		t.Helper()
		compiled := deck.Compile(deck.ParseSource(source), manifest, deck.CompileOptions{Language: language})
		_, missing, err := PDFWithMissing(
			model.Presentation{Title: "인쇄", Language: language, Slides: compiled.Slides},
			Options{TemplateData: data, Manifest: manifest})
		if err != nil {
			t.Fatalf("pdf: %v", err)
		}
		return missing
	}
	missing := printed("# 四半期実績\n- 売上と直販の実績\n- 业绩と销售额\n", "ja")
	if len(missing) == 0 {
		t.Fatal("the deck holds characters the face has no glyph for, and none were reported")
	}
	for _, character := range []rune{'実', '売', '业'} {
		if !strings.ContainsRune(string(missing), character) {
			t.Errorf("%q prints as a blank and was not reported: %q", character, string(missing))
		}
	}
	if none := printed("# 물류센터 자동화\n- 도입 승인 요청\n", "ko"); len(none) > 0 {
		t.Errorf("a Korean deck reported %q", string(none))
	}
}

// A picture in an exported PDF is for paper, not for a monitor.
//
// The one that was wrong: when previews started embedding a picture at the size
// they draw it at, the PDF was drawn through the same renderer and quietly took
// a screen's budget with it. The picture in an exported deck went from 1400x933
// to 1024x683 — a quarter of its detail — and nothing said so. The guard is on
// the exported file rather than on the arithmetic, because the arithmetic was
// right and the export was the caller that forgot to say what it was for.
func TestAPictureInAnExportedPDFIsDrawnForPaper(t *testing.T) {
	data, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	photo := testPhotograph(2400, 1600)
	images := func(string) (pptx.Picture, bool) {
		return pptx.Picture{Data: photo, ContentType: "image/jpeg", Width: 2400, Height: 1600}, true
	}
	source := ""
	for _, name := range []string{"일", "이", "삼", "사", "오", "육"} {
		source += "# " + name + "번 장\n- 요점\n::image 사진\n\n"
	}
	compiled := deck.Compile(deck.ParseSource(source), manifest, deck.CompileOptions{
		Language: "ko",
		ResolveImage: func(reference string) (deck.ContentImage, bool) {
			return deck.ContentImage{AssetID: "photo", Name: reference}, true
		},
	})
	presentation := model.Presentation{Title: "사진", Language: "ko", Slides: compiled.Slides}
	file, err := PDF(presentation, Options{TemplateData: data, Manifest: manifest, Images: images})
	if err != nil {
		t.Fatalf("pdf: %v", err)
	}
	widths := regexp.MustCompile(`/Subtype\s*/Image[\s\S]*?/Width\s+(\d+)`).FindAllSubmatch(file, -1)
	if len(widths) == 0 {
		t.Fatal("the exported PDF carries no picture at all")
	}
	widest := 0
	for _, found := range widths {
		if value, _ := strconv.Atoi(string(found[1])); value > widest {
			widest = value
		}
	}
	// A full-bleed photograph on a printed page is worth every pixel the
	// renderer is willing to embed. A screen's budget would land near a
	// thousand, which is what the regression looked like.
	if widest < 1200 {
		t.Fatalf("the widest picture in the exported PDF is %dpx; a printed page is owed more than a screen", widest)
	}
}

// A photograph, in the sense that matters: large, and not compressible to
// nothing.
func testPhotograph(width, height int) []byte {
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	source := rand.New(rand.NewSource(11))
	for y := 0; y < height; y += 4 {
		for x := 0; x < width; x += 4 {
			shade := color.RGBA{uint8(source.Intn(256)), uint8(source.Intn(256)), uint8(source.Intn(256)), 255}
			for dy := 0; dy < 4 && y+dy < height; dy++ {
				for dx := 0; dx < 4 && x+dx < width; dx++ {
					picture.Set(x+dx, y+dy, shade)
				}
			}
		}
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, picture, &jpeg.Options{Quality: 88}); err != nil {
		panic(err)
	}
	return buffer.Bytes()
}

// The handout leaves out the slides the author is skipping, so the deck's third
// slide is not the third page. A link written [3장](#3) was handed to the paper
// as the number alone, and clicking it turned to the page after the one it
// names — one further on for every skipped slide before it.
func TestAJumpOnPaperNamesThePageItIsPrintedOn(t *testing.T) {
	file, _ := printedDeck(t, "# 첫 장\n@content\n- 세 번째 장 [3장 참고](#3)\n- 건너뛴 장 [2장](#2)\n- 없는 장 [9장](#9)\n\n"+
		"# 둘째 장\n@content\n!skip\n- 뺀 장\n\n# 셋째 장\n@content\n- 여기가 3장\n\n# 넷째 장\n@content\n- 4장\n")
	body := string(file)
	if pages := strings.Count(body, "/Type /Page "); pages != 3 {
		t.Fatalf("a deck of three shown slides printed %d pages", pages)
	}
	// /Dest [1 /Fit] is the second page, which is where the deck's third slide
	// is printed. The jump naming the skipped slide lands there too — the next
	// page there is — and the jump past the end of the deck names no page.
	destinations := regexp.MustCompile(`/Dest \[(\d+) /Fit\]`).FindAllStringSubmatch(body, -1)
	var landed []string
	for _, one := range destinations {
		landed = append(landed, one[1])
	}
	if len(landed) != 2 || landed[0] != "1" || landed[1] != "1" {
		t.Errorf("the jumps landed on pages %v, want both on the second page (index 1)", landed)
	}
	// A link that names neither a page nor an address is not written at all.
	if strings.Contains(body, "/URI ()") {
		t.Error("a jump past the end of the deck was written as a link to nowhere")
	}
}
