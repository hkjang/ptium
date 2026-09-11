package pdftext

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The shapes real files arrive in.
//
// Every one of these read as a page with nothing on it, and a page with
// nothing on it is reported to the person as "this PDF has no text, it looks
// like a scan". They were reports, spreadsheets and letters the whole time.

// builtPDF writes the smallest file that holds the given objects.
func builtPDF(objects map[int][]byte) []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n")
	places := map[int]int{}
	numbers := make([]int, 0, len(objects))
	for number := range objects {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	for _, number := range numbers {
		places[number] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n", number)
		out.Write(objects[number])
		out.WriteString("\nendobj\n")
	}
	start := out.Len()
	top := numbers[len(numbers)-1] + 1
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", top)
	for number := 1; number < top; number++ {
		fmt.Fprintf(&out, "%010d 00000 n \n", places[number])
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", top, start)
	return out.Bytes()
}

func streamObject(entries string, body []byte) []byte {
	return append([]byte(fmt.Sprintf("<< %s /Length %d >>\nstream\n", entries, len(body))),
		append(body, []byte("\nendstream")...)...)
}

const helvetica = `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>`

// onePage builds a file whose single page draws the given content stream.
func pageDrawing(t *testing.T, filters string, body []byte) []byte {
	t.Helper()
	resources := "/Font << /F1 4 0 R >>"
	objects := map[int][]byte{
		1: []byte(`<< /Type /Catalog /Pages 2 0 R >>`),
		2: []byte(`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`),
		3: []byte(fmt.Sprintf(`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] `+
			`/Resources << %s >> /Contents 5 0 R >>`, resources)),
		4: []byte(helvetica),
		5: streamObject(filters, body),
	}
	return builtPDF(objects)
}

func flate(t *testing.T, data []byte) []byte {
	t.Helper()
	var packed bytes.Buffer
	writer := zlib.NewWriter(&packed)
	writer.Write(data)
	writer.Close()
	return packed.Bytes()
}

func ascii85(data []byte) []byte {
	var out bytes.Buffer
	for at := 0; at < len(data); at += 4 {
		group := make([]byte, 4)
		held := copy(group, data[at:])
		value := uint32(group[0])<<24 | uint32(group[1])<<16 | uint32(group[2])<<8 | uint32(group[3])
		var digits [5]byte
		for place := 4; place >= 0; place-- {
			digits[place] = byte(value%85) + '!'
			value /= 85
		}
		out.Write(digits[:held+1])
	}
	out.WriteString("~>")
	return out.Bytes()
}

func said(t *testing.T, file []byte) []string {
	t.Helper()
	read, err := Read(file)
	if err != nil {
		t.Fatalf("reading the file: %v", err)
	}
	lines := []string{}
	for _, page := range read.Pages {
		lines = append(lines, page.Lines...)
	}
	return lines
}

// A stream is often wrapped twice, so that the file stays printable characters
// all the way through. Undoing only the Flate half left the page empty.
func TestAStreamWrappedTwiceIsStillRead(t *testing.T) {
	drawn := []byte(`BT /F1 12 Tf 50 700 Td (Wrapped twice over) Tj ET`)
	for _, one := range []struct {
		what   string
		filter string
		body   []byte
	}{
		{"ASCII85 over Flate", "/Filter [/ASCII85Decode /FlateDecode]", ascii85(flate(t, drawn))},
		{"ASCII85 alone", "/Filter /ASCII85Decode", ascii85(drawn)},
		{"hexadecimal", "/Filter /ASCIIHexDecode", append([]byte(hex.EncodeToString(drawn)), '>')},
		{"Flate alone", "/Filter /FlateDecode", flate(t, drawn)},
		{"nothing at all", "", drawn},
	} {
		lines := said(t, pageDrawing(t, one.filter, one.body))
		if len(lines) != 1 || lines[0] != "Wrapped twice over" {
			t.Errorf("%s: the page says %q", one.what, lines)
		}
	}
}

// A generator is free to put a header, a footer or a whole letterhead in a form
// and draw it with one Do. Reading only the page's own stream left that text
// out with nothing said about it — and a header drawn last belongs at the top.
func TestTextInsideAFormIsReadWhereItSits(t *testing.T) {
	page := []byte("BT /F1 14 Tf 50 700 Td (Body of the page) Tj ET\nq 1 0 0 1 0 0 cm /Fx1 Do Q")
	inner := []byte("BT /F1 12 Tf 50 780 Td (Letterhead above it) Tj ET")
	file := builtPDF(map[int][]byte{
		1: []byte(`<< /Type /Catalog /Pages 2 0 R >>`),
		2: []byte(`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`),
		3: []byte(`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources ` +
			`<< /Font << /F1 4 0 R >> /XObject << /Fx1 6 0 R >> >> /Contents 5 0 R >>`),
		4: []byte(helvetica),
		5: streamObject("", page),
		6: streamObject(`/Type /XObject /Subtype /Form /BBox [0 0 595 842] `+
			`/Resources << /Font << /F1 4 0 R >> >>`, inner),
	})
	lines := said(t, file)
	want := []string{"Letterhead above it", "Body of the page"}
	if len(lines) != 2 || lines[0] != want[0] || lines[1] != want[1] {
		t.Errorf("the page says %q, want %q", lines, want)
	}
}

// A subset font numbers its glyphs to suit itself. Printing a code it does not
// name as if the byte were ASCII turned its space — which had landed on 0x24 —
// into the dollar sign in "매출$이$늘었습니다".
func TestACodeAFontDoesNotNameIsNotPrintedAsItsByte(t *testing.T) {
	entries := map[byte]rune{0x25: '매', 0x26: '출', 0x27: '이', 0x28: '늘', 0x29: '었', 0x2A: '습', 0x2B: '니', 0x2C: '다'}
	codes := []byte{0x25, 0x26, 0x24, 0x27, 0x24, 0x28, 0x29, 0x2A, 0x2B, 0x2C}
	var chars strings.Builder
	order := []int{}
	for code := range entries {
		order = append(order, int(code))
	}
	sort.Ints(order)
	for _, code := range order {
		fmt.Fprintf(&chars, "<%02X> <%04X>\n", code, entries[byte(code)])
	}
	cmap := "/CIDInit /ProcSet findresource begin 12 dict begin begincmap\n" +
		"1 begincodespacerange <00> <FF> endcodespacerange\n" +
		fmt.Sprintf("%d beginbfchar\n%sendbfchar\nendcmap end end", len(entries), chars.String())
	file := builtPDF(map[int][]byte{
		1: []byte(`<< /Type /Catalog /Pages 2 0 R >>`),
		2: []byte(`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`),
		3: []byte(`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] ` +
			`/Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>`),
		4: []byte(`<< /Type /Font /Subtype /TrueType /BaseFont /AAAAAA+Subset ` +
			`/FirstChar 0 /LastChar 255 /ToUnicode 6 0 R >>`),
		5: streamObject("", []byte("BT /F1 12 Tf 50 700 Td <"+hex.EncodeToString(codes)+"> Tj ET")),
		6: streamObject("", []byte(cmap)),
	})
	lines := said(t, file)
	if len(lines) != 1 || lines[0] != "매출 이 늘었습니다" {
		t.Errorf("the page says %q, want %q", lines, "매출 이 늘었습니다")
	}
	for _, line := range lines {
		if strings.ContainsAny(line, "$#@") {
			t.Errorf("a byte was printed as a letter nobody typed: %q", line)
		}
	}
}

// A font may renumber its glyphs and say so in a /Differences list, which is
// the font's own word on what each byte draws.
func TestAFontsOwnEncodingIsRead(t *testing.T) {
	file := builtPDF(map[int][]byte{
		1: []byte(`<< /Type /Catalog /Pages 2 0 R >>`),
		2: []byte(`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`),
		3: []byte(`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] ` +
			`/Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>`),
		4: []byte(`<< /Type /Font /Subtype /Type1 /BaseFont /AAAAAA+Renumbered /Encoding 6 0 R >>`),
		5: streamObject("", []byte("BT /F1 12 Tf 50 700 Td <0102030405> Tj ET")),
		6: []byte(`<< /Type /Encoding /Differences [1 /R /a /i /space /n] >>`),
	})
	if lines := said(t, file); len(lines) != 1 || lines[0] != "Rai n" {
		t.Errorf("the page says %q, want %q", lines, "Rai n")
	}
}

// The usual office setting is an owner password and no user password: the file
// opens for a reader and only says what they may not do with it. Its streams
// are encrypted regardless.
func TestAProtectedFileAnybodyCanOpenIsRead(t *testing.T) {
	// Written by a generator rather than by hand: the key is derived from what
	// the file says about itself, so a handmade one would only test the maths
	// against itself.
	file, err := os.ReadFile(filepath.Join("testdata", "protected.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	lines := said(t, file)
	if len(lines) == 0 {
		t.Fatal("a file anybody can open came back with nothing on it")
	}
	joined := strings.Join(lines, " ")
	if !strings.Contains(joined, "Protected") || !strings.Contains(joined, "24") {
		t.Errorf("the protected report says %q", lines)
	}
}

// And a file that really does want a password is a different thing from one
// with nothing in it. Telling somebody their locked report is a scanned image
// sends them to fix the wrong thing.
func TestAFileThatWantsAPasswordSaysSo(t *testing.T) {
	file, err := os.ReadFile(filepath.Join("testdata", "locked.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	read, err := Read(file)
	if err != nil {
		t.Fatalf("reading the file: %v", err)
	}
	if !read.Locked {
		t.Error("a file that wants a password was not reported as one")
	}
	for _, page := range read.Pages {
		if len(page.Lines) > 0 {
			t.Errorf("a locked file gave up text: %q", page.Lines)
		}
	}
}

// glyphText answers the names an /Encoding uses, including the ones a font
// spells out in hexadecimal.
func TestGlyphNamesAreRead(t *testing.T) {
	for name, want := range map[string]string{
		"space": " ", "A": "A", "a": "a", "zero": "0", "bullet": "•",
		"quoteright": "’", "endash": "–", "uni0041": "A", "u00C5": "Å",
		"a.sc": "a", "adieresis": "ä",
	} {
		if got, ok := glyphText(name); !ok || got != want {
			t.Errorf("glyphText(%q) = %q %v, want %q", name, got, ok, want)
		}
	}
	for _, name := range []string{"", ".notdef", "g43", "cid7"} {
		if got, ok := glyphText(name); ok {
			t.Errorf("glyphText(%q) invented %q", name, got)
		}
	}
}
