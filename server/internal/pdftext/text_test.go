package pdftext

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// build assembles a minimal PDF out of numbered objects. Nothing here writes a
// cross-reference table, because the reader deliberately does not read one:
// the files people upload are routinely repaired, appended to, or truncated,
// and the objects themselves are the only part that can be trusted.
func build(objects ...string) []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n")
	for index, body := range objects {
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", index+1, body)
	}
	out.WriteString("trailer<</Root 1 0 R>>\n%%EOF\n")
	return out.Bytes()
}

func streamed(dictionary, content string) string {
	return fmt.Sprintf("<<%s /Length %d>>\nstream\n%s\nendstream", dictionary, len(content), content)
}

func onePage(fontDict, content string) []byte {
	return build(
		"<</Type /Catalog /Pages 2 0 R>>",
		"<</Type /Pages /Kids [3 0 R] /Count 1>>",
		"<</Type /Page /Parent 2 0 R /Resources <</Font <</F1 5 0 R>>>> /Contents 4 0 R>>",
		streamed("", content),
		fontDict,
	)
}

func linesOf(t *testing.T, data []byte) []string {
	t.Helper()
	read, err := Read(data)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(read.Pages) != 1 {
		t.Fatalf("expected one page, got %d", len(read.Pages))
	}
	return read.Pages[0].Lines
}

// A page is glyphs at coordinates. What shares a baseline is a line, whoever
// drew it and however many instructions it took.
func TestWordsOnOneBaselineAreOneLine(t *testing.T) {
	content := `BT /F1 12 Tf 1 0 0 1 72 700 Tm (Quarterly) Tj ET
BT /F1 12 Tf 1 0 0 1 130 700 Tm (review) Tj ET
BT /F1 12 Tf 1 0 0 1 72 680 Tm (Second line) Tj ET`
	lines := linesOf(t, onePage("<</Type /Font /Subtype /Type1 /BaseFont /Helvetica>>", content))
	want := []string{"Quarterly review", "Second line"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Errorf("lines = %#v, want %#v", lines, want)
	}
}

// A generator is free to place every letter itself, and several do. One letter
// per line is not what the page says.
func TestLetterByLetterPlacementIsStillAWord(t *testing.T) {
	var content strings.Builder
	for index, letter := range []string{"R", "E", "P", "O", "R", "T"} {
		fmt.Fprintf(&content, "BT /F1 10 Tf 1 0 0 1 %d 700 Tm (%s) Tj ET\n", 72+index*5, letter)
	}
	lines := linesOf(t, onePage("<</Type /Font /Subtype /Type1 /BaseFont /Helvetica>>", content.String()))
	if len(lines) != 1 || strings.ReplaceAll(lines[0], " ", "") != "REPORT" {
		t.Errorf("lines = %#v, want one line reading REPORT", lines)
	}
}

// Two blocks of text side by side on one baseline are two things, not one long
// sentence.
func TestAWideGapOnOneBaselineIsNotASpace(t *testing.T) {
	content := `BT /F1 12 Tf 1 0 0 1 72 700 Tm (left column) Tj ET
BT /F1 12 Tf 1 0 0 1 400 700 Tm (right column) Tj ET`
	lines := linesOf(t, onePage("<</Type /Font /Subtype /Type1 /BaseFont /Helvetica>>", content))
	if len(lines) != 2 {
		t.Errorf("lines = %#v, want the two columns apart", lines)
	}
}

// koreanCMap writes a ToUnicode map in one block of the given size. Real files
// write twelve thousand entries in a single block, which is a hundred and
// twenty times what the specification suggests.
func koreanCMap(entries map[uint32]rune, padTo int) string {
	var body strings.Builder
	total := len(entries) + padTo
	fmt.Fprintf(&body, "/CIDInit /ProcSet findresource begin\nbegincmap\n%d beginbfchar\n", total)
	for code, said := range entries {
		fmt.Fprintf(&body, "<%04X> <%04X>\n", code, said)
	}
	for index := 0; index < padTo; index++ {
		fmt.Fprintf(&body, "<%04X> <%04X>\n", 0x8000+index, 0x4E00+index)
	}
	body.WriteString("endbfchar\nendcmap\nend\n")
	return body.String()
}

func koreanPage(t *testing.T, shown string, padTo int) []string {
	t.Helper()
	cmap := koreanCMap(map[uint32]rune{1: '보', 2: '고', 3: '서'}, padTo)
	data := build(
		"<</Type /Catalog /Pages 2 0 R>>",
		"<</Type /Pages /Kids [3 0 R] /Count 1>>",
		"<</Type /Page /Parent 2 0 R /Resources <</Font <</F1 5 0 R>>>> /Contents 4 0 R>>",
		streamed("", "BT /F1 14 Tf 1 0 0 1 72 700 Tm <"+shown+"> Tj ET"),
		"<</Type /Font /Subtype /Type0 /BaseFont /Noto /Encoding /Identity-H /DescendantFonts [7 0 R] /ToUnicode 6 0 R>>",
		streamed("", cmap),
		"<</Type /Font /Subtype /CIDFontType0 /BaseFont /Noto>>",
	)
	return linesOf(t, data)
}

// The map is the whole of reading Korean out of a PDF, and it has to be read to
// its end. Keeping only the tail of a block is how a reader covers a tenth of a
// page and drops the rest without saying so.
func TestAWholeCMapBlockIsRead(t *testing.T) {
	for _, padding := range []int{0, 400, 12000} {
		lines := koreanPage(t, "000100020003", padding)
		if len(lines) != 1 || lines[0] != "보고서" {
			t.Errorf("with %d extra entries lines = %#v, want [보고서]", padding, lines)
		}
	}
}

// A composite font with no map draws glyph numbers. There is no honest way to
// turn those into letters, so the page says nothing rather than something made
// up.
func TestGlyphNumbersWithNoMapSayNothing(t *testing.T) {
	data := build(
		"<</Type /Catalog /Pages 2 0 R>>",
		"<</Type /Pages /Kids [3 0 R] /Count 1>>",
		"<</Type /Page /Parent 2 0 R /Resources <</Font <</F1 5 0 R>>>> /Contents 4 0 R>>",
		streamed("", "BT /F1 14 Tf 1 0 0 1 72 700 Tm <0A0B0C0D> Tj ET"),
		"<</Type /Font /Subtype /Type0 /BaseFont /Noto /Encoding /Identity-H /DescendantFonts [6 0 R]>>",
		"<</Type /Font /Subtype /CIDFontType2 /BaseFont /Noto>>",
	)
	if lines := linesOf(t, data); len(lines) != 0 {
		t.Errorf("lines = %#v, want nothing invented", lines)
	}
}

// Some generators write Korean as UTF-16 while telling the file the font is
// Latin. Read as the file claims, the page says Ö¨¬üÈÇ; read as it was written,
// it says what its author typed.
func TestKoreanWrittenAsUTF16UnderALatinFontIsRead(t *testing.T) {
	// "프로젝트 현황" — two bytes for each Korean character, one for the space.
	content := "BT /F1 24 Tf 1 0 0 1 72 700 Tm <d504b85cc81dd2b820d604d669> Tj ET"
	lines := linesOf(t, onePage("<</Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding>>", content))
	if len(lines) != 1 || lines[0] != "프로젝트 현황" {
		t.Errorf("lines = %#v, want [프로젝트 현황]", lines)
	}
}

// Latin text under a Latin font stays Latin: the rule above must not reach text
// it was not written for.
func TestLatinTextIsNotReadAsKorean(t *testing.T) {
	content := "BT /F1 12 Tf 1 0 0 1 72 700 Tm (Generated 2025) Tj ET"
	lines := linesOf(t, onePage("<</Type /Font /Subtype /Type1 /BaseFont /Helvetica>>", content))
	if len(lines) != 1 || lines[0] != "Generated 2025" {
		t.Errorf("lines = %#v, want [Generated 2025]", lines)
	}
}

// A Korean subset font maps its codes to jamo rather than to syllables, and a
// slide carrying loose jamo reads as gibberish in PowerPoint.
func TestJamoAreJoinedIntoSyllables(t *testing.T) {
	cases := map[string]string{
		"\u1112\u1161\u11ab\u1100\u1173\u11af": "한글",           // lead, vowel, tail
		"\u1112\u1161":                         "하",            // no tail
		"\ubcf4\uace0\uc11c":                   "보고서",          // already syllables
		"\u1161\u1112":                         "\u1161\u1112", // a vowel before a lead is not a syllable
		"":                                     "",
	}
	for given, want := range cases {
		if got := composeHangul(given); got != want {
			t.Errorf("composeHangul(%q) = %q, want %q", given, got, want)
		}
	}
}

// The same, through the reader: a font whose map hands back jamo.
func TestAPageOfJamoReadsAsKorean(t *testing.T) {
	cmap := "/CIDInit /ProcSet findresource begin\nbegincmap\n5 beginbfchar\n" +
		"<0001> <1112>\n<0002> <1161>\n<0003> <11AB>\n<0004> <1100>\n<0005> <1173>\nendbfchar\nendcmap\nend\n"
	data := build(
		"<</Type /Catalog /Pages 2 0 R>>",
		"<</Type /Pages /Kids [3 0 R] /Count 1>>",
		"<</Type /Page /Parent 2 0 R /Resources <</Font <</F1 5 0 R>>>> /Contents 4 0 R>>",
		streamed("", "BT /F1 14 Tf 1 0 0 1 72 700 Tm <000100020003> Tj ET"),
		"<</Type /Font /Subtype /Type0 /BaseFont /Noto /Encoding /Identity-H /DescendantFonts [7 0 R] /ToUnicode 6 0 R>>",
		streamed("", cmap),
		"<</Type /Font /Subtype /CIDFontType0 /BaseFont /Noto>>",
	)
	if lines := linesOf(t, data); len(lines) != 1 || lines[0] != "한" {
		t.Errorf("lines = %#v, want [한]", lines)
	}
}

// A page drawn as one picture has no text in it, and the reader says so by
// answering with a page that has no lines rather than by failing.
func TestAPageOfPicturesHasNoLines(t *testing.T) {
	data := build(
		"<</Type /Catalog /Pages 2 0 R>>",
		"<</Type /Pages /Kids [3 0 R] /Count 1>>",
		"<</Type /Page /Parent 2 0 R /Resources <</XObject <</Im1 5 0 R>>>> /Contents 4 0 R>>",
		streamed("", "q 612 0 0 792 0 0 cm /Im1 Do Q"),
		streamed("/Type /XObject /Subtype /Image", "binary"),
	)
	read, err := Read(data)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(read.Pages) != 1 || len(read.Pages[0].Lines) != 0 {
		t.Errorf("pages = %#v, want one page with no lines", read.Pages)
	}
}

// What is not a PDF is refused, rather than read as an empty one.
func TestSomethingElseIsRefused(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("PK\x03\x04 a zip"), []byte("%PDF")} {
		if _, err := Read(data); err == nil {
			t.Errorf("Read(%q) accepted a file that is not a PDF", data)
		}
	}
}
