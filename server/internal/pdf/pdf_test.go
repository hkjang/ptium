package pdf

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestTheBuiltinFontCanDrawKorean(t *testing.T) {
	font, err := BuiltinFont()
	if err != nil {
		t.Fatalf("the embedded font did not parse: %v", err)
	}
	for _, character := range []rune{'가', '힣', '매', '출', 'A', '1', '%', '·', '—'} {
		glyph, ok := font.Glyph(character)
		if !ok {
			t.Errorf("the font cannot draw %q", character)
			continue
		}
		if width := font.Width(glyph); width <= 0 || width > 2000 {
			t.Errorf("%q is %d wide, which is not a width", character, width)
		}
	}
	// A Hangul syllable is wider than a Latin letter in every Korean face.
	hangul, _ := font.Glyph('매')
	latin, _ := font.Glyph('i')
	if font.Width(hangul) <= font.Width(latin) {
		t.Errorf("한글 %d, latin %d — the widths are not being read", font.Width(hangul), font.Width(latin))
	}
	if !strings.Contains(FontLicense(), "SIL OPEN FONT LICENSE") {
		t.Errorf("the font ships without its licence")
	}
}

func TestAPageIsWrittenAsAPDF(t *testing.T) {
	font, err := BuiltinFont()
	if err != nil {
		t.Fatal(err)
	}
	document := New(720, 405, "분기 보고", font)
	page := document.AddPage()
	page.Rect(0, 0, 720, 405, "FFFFFF")
	page.Rect(40, 30, 60, 4, "2563EB")
	page.Text(40, 80, 28, "15181D", "매출이 12% 늘었습니다", true, false)
	page.Text(40, 120, 14, "15181D", "근거는 부록에 있습니다", false, false)
	page.Link(40, 108, 90, 16, "https://example.com/a", 0)
	out := document.Bytes()

	if !bytes.HasPrefix(out, []byte("%PDF-1.7")) || !bytes.HasSuffix(out, []byte("%%EOF\n")) {
		t.Fatalf("that is not a PDF: %q … %q", out[:16], out[len(out)-16:])
	}
	for _, wanted := range []string{"/Type /Catalog", "/Type /Pages", "/Type /Page ", "/Subtype /Type0",
		"/Encoding /Identity-H", "/FontFile2", "/ToUnicode", "/Subtype /Link", "/URI"} {
		if !bytes.Contains(out, []byte(wanted)) {
			t.Errorf("the file does not carry %s", wanted)
		}
	}
	// The cross-reference table has to point at what it says it does, or the
	// file opens as damaged in every reader.
	start := bytes.LastIndex(out, []byte("startxref"))
	if start < 0 {
		t.Fatal("no startxref")
	}
	if !bytes.Contains(out[start:], []byte("%%EOF")) {
		t.Error("the trailer is not the end of the file")
	}
}

// Every glyph the document drew has to be in the width array and in the map
// that says what it means, or a reader shows the right shapes and copies out
// the wrong text.
func TestWhatWasDrawnIsWhatIsDescribed(t *testing.T) {
	font, err := BuiltinFont()
	if err != nil {
		t.Fatal(err)
	}
	document := New(720, 405, "폭", font)
	page := document.AddPage()
	page.Text(10, 20, 12, "000000", "매출", false, false)
	out := string(document.Bytes())
	for _, character := range []rune{'매', '출'} {
		glyph, _ := font.Glyph(character)
		if !strings.Contains(document.widthArray(), itoa(int(glyph))+" [") {
			t.Errorf("%q is drawn and has no width", character)
		}
		if !strings.Contains(string(document.toUnicodeCMap()), hex4(int(glyph))) {
			t.Errorf("%q is drawn and nothing says what it means", character)
		}
	}
	if strings.Count(out, "/FontFile2") != 1 {
		t.Errorf("the font is embedded %d times", strings.Count(out, "/FontFile2"))
	}
}

func itoa(value int) string { return strconv.Itoa(value) }
func hex4(value int) string { return fmt.Sprintf("<%04X>", value) }
