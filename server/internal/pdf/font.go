package pdf

import (
	"bytes"
	"fmt"
	"strings"
)

// writeFont puts the font in the document: the file itself, what it is, and
// which glyph stands for which character.
//
// The whole font is embedded rather than the part the deck uses. Cutting a
// TrueType file down means rebuilding its outlines and every table that points
// into them, and a wrong cut is a PDF that opens with the words missing —
// which is worse than a file that is four megabytes larger. It is embedded
// once per document, not once per page.
func (d *Document) writeFont(object func(string) int, stream func(string, []byte) int) int {
	font := d.font
	file := stream(fmt.Sprintf("/Filter /FlateDecode /Length1 %d", len(font.Data)), deflate(font.Data))
	scale := func(value int) int { return value * 1000 / font.UnitsPerEm }
	descriptor := object(fmt.Sprintf(
		"<< /Type /FontDescriptor /FontName /%s /Flags 4 /FontBBox [%d %d %d %d] /ItalicAngle %d "+
			"/Ascent %d /Descent %d /CapHeight %d /StemV 80 /FontFile2 %d 0 R >>",
		fontName, scale(font.BBox[0]), scale(font.BBox[1]), scale(font.BBox[2]), scale(font.BBox[3]),
		font.ItalicAngle, scale(font.Ascent), scale(font.Descent), scale(font.CapHeight), file))
	toUnicode := stream("/Filter /FlateDecode", deflate(d.toUnicodeCMap()))
	descendant := object(fmt.Sprintf(
		"<< /Type /Font /Subtype /CIDFontType2 /BaseFont /%s "+
			"/CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> "+
			"/FontDescriptor %d 0 R /DW 1000 /W [%s] /CIDToGIDMap /Identity >>",
		fontName, descriptor, d.widthArray()))
	return object(fmt.Sprintf(
		"<< /Type /Font /Subtype /Type0 /BaseFont /%s /Encoding /Identity-H "+
			"/DescendantFonts [%d 0 R] /ToUnicode %d 0 R >>", fontName, descendant, toUnicode))
}

const fontName = "NanumBarunGothic"

// widthArray states the width of every glyph the document drew. A glyph nobody
// used costs nothing to leave out, and there are seventeen thousand of them.
func (d *Document) widthArray() string {
	var out strings.Builder
	for _, glyph := range sortedGlyphs(d.used) {
		fmt.Fprintf(&out, "%d [%d] ", glyph, d.font.Width(glyph))
	}
	return strings.TrimSpace(out.String())
}

// toUnicodeCMap is what makes the text in the file selectable and searchable:
// without it a reader has glyph numbers and no idea what they say, and copying
// a line out of the PDF gives nonsense.
func (d *Document) toUnicodeCMap() []byte {
	var out bytes.Buffer
	out.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n" +
		"/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n" +
		"/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")
	glyphs := sortedGlyphs(d.used)
	for start := 0; start < len(glyphs); start += 100 {
		end := min(start+100, len(glyphs))
		fmt.Fprintf(&out, "%d beginbfchar\n", end-start)
		for _, glyph := range glyphs[start:end] {
			character := d.used[glyph]
			if character > 0xFFFF {
				character -= 0x10000
				fmt.Fprintf(&out, "<%04X> <%04X%04X>\n", glyph,
					0xD800+(character>>10), 0xDC00+(character&0x3FF))
				continue
			}
			fmt.Fprintf(&out, "<%04X> <%04X>\n", glyph, character)
		}
		out.WriteString("endbfchar\n")
	}
	out.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	return out.Bytes()
}
