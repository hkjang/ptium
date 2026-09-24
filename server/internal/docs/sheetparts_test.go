package docs

import (
	"archive/zip"
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// packWorkbook writes an .xlsx out of the parts given, in the order given.
func packWorkbook(parts [][2]string) []byte {
	buffer := &bytes.Buffer{}
	writer := zip.NewWriter(buffer)
	for _, part := range parts {
		file, _ := writer.Create(part[0])
		file.Write([]byte(part[1]))
	}
	writer.Close()
	return buffer.Bytes()
}

// A part that deflates to almost nothing, which is what every part of a
// workbook the reader does not want is allowed to be.
func compressible(size int) string {
	return strings.Repeat("A", size)
}

// The artwork, the printer settings, the theme and the chain a recalculation
// walks are in the file and are not what a deck is made of. Unpacked with the
// rest of the archive and held until the last sheet had been read, they cost
// what the file says they cost only once it is open: a third of a megabyte of
// upload unpacked to a third of a gigabyte for a deck of one table, and the
// per-part limit never saw it because no one part was over the limit.
func TestAWorkbookDoesNotUnpackThePartsADeckIsNotMadeOf(t *testing.T) {
	const junk, size = 40, 8 << 20
	parts := [][2]string{
		{"[Content_Types].xml", `<Types/>`},
		{"xl/_rels/workbook.xml.rels",
			`<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`},
		{"xl/workbook.xml", `<workbook><sheets><sheet name="매출" sheetId="1" id="rId1"/></sheets></workbook>`},
		{"xl/worksheets/sheet1.xml", `<worksheet><sheetData>` +
			`<row r="1"><c r="A1" t="inlineStr"><is><t>도시</t></is></c><c r="B1" t="inlineStr"><is><t>매출</t></is></c></row>` +
			`<row r="2"><c r="A2" t="inlineStr"><is><t>서울</t></is></c><c r="B2"><v>1200</v></c></row>` +
			`</sheetData></worksheet>`},
		{"xl/calcChain.xml", compressible(size)},
		{"xl/theme/theme1.xml", compressible(size)},
		{"xl/printerSettings/printerSettings1.bin", compressible(size)},
	}
	for i := 0; i < junk; i++ {
		parts = append(parts, [2]string{fmt.Sprintf("xl/media/image%d.png", i), compressible(size)})
	}
	data := packWorkbook(parts)
	if len(data) > 1<<20 {
		t.Fatalf("the crafted upload got large enough to be the bound itself: %d bytes", len(data))
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	document, err := Read("우리회사.xlsx", data)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("upload %d bytes holding %d MB of parts, allocated %.0f MB",
		len(data), (junk+3)*size>>20, float64(allocated)/(1<<20))
	if allocated > 64<<20 {
		t.Errorf("a %d byte upload allocated %.0f MB for a deck of one table",
			len(data), float64(allocated)/(1<<20))
	}

	// The sheet itself still has to come across whole, or the reader saved the
	// memory by reading nothing.
	if !strings.Contains(document.Source, "서울") || !strings.Contains(document.Source, "1200") {
		t.Errorf("the sheet did not come across: %q", document.Source)
	}
}

// A hidden sheet is passed over, and its part was unpacked before anything
// looked at whether it would be: the code table a formula looks up in is
// often the largest sheet in the workbook.
func TestAHiddenSheetIsPassedOverWithoutBeingUnpacked(t *testing.T) {
	const size = 64 << 20
	data := packWorkbook([][2]string{
		{"xl/_rels/workbook.xml.rels", `<Relationships>` +
			`<Relationship Id="rId1" Target="worksheets/sheet1.xml"/>` +
			`<Relationship Id="rId2" Target="worksheets/sheet2.xml"/></Relationships>`},
		{"xl/workbook.xml", `<workbook><sheets>` +
			`<sheet name="매출" sheetId="1" id="rId1"/>` +
			`<sheet name="코드" sheetId="2" id="rId2" state="hidden"/>` +
			`</sheets></workbook>`},
		{"xl/worksheets/sheet1.xml", `<worksheet><sheetData>` +
			`<row r="1"><c r="A1" t="inlineStr"><is><t>도시</t></is></c><c r="B1" t="inlineStr"><is><t>매출</t></is></c></row>` +
			`<row r="2"><c r="A2" t="inlineStr"><is><t>서울</t></is></c><c r="B2"><v>1200</v></c></row>` +
			`</sheetData></worksheet>`},
		{"xl/worksheets/sheet2.xml", `<worksheet><sheetData><row r="1">` +
			`<c r="A1" t="inlineStr"><is><t>` + compressible(size) + `</t></is></c></row></sheetData></worksheet>`},
	})

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	document, err := Read("우리회사.xlsx", data)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 32<<20 {
		t.Errorf("a hidden sheet of %d MB cost %.0f MB to pass over",
			size>>20, float64(allocated)/(1<<20))
	}
	// Passing over it without unpacking it is still passing over it, and that
	// is said the same way.
	said := false
	for _, warning := range document.Warnings {
		if strings.Contains(warning, "숨겨진 시트") && strings.Contains(warning, "코드") {
			said = true
		}
	}
	if !said {
		t.Errorf("the hidden sheet was not named: %v", document.Warnings)
	}
}

// Unpacking a part by name is unpacking it at the name the workbook gave it.
// A sheet's relationship may name its part relative to xl/, already under xl/,
// or from the root of the package with a leading slash, and all three are the
// same part.
func TestASheetIsFoundAtEveryNameItsRelationshipCanGiveIt(t *testing.T) {
	for _, target := range []string{"worksheets/sheet1.xml", "xl/worksheets/sheet1.xml", "/xl/worksheets/sheet1.xml"} {
		t.Run(target, func(t *testing.T) {
			data := packWorkbook([][2]string{
				{"xl/_rels/workbook.xml.rels",
					`<Relationships><Relationship Id="rId1" Target="` + target + `"/></Relationships>`},
				{"xl/workbook.xml", `<workbook><sheets><sheet name="매출" sheetId="1" id="rId1"/></sheets></workbook>`},
				// The shared string table and the styles are named parts too:
				// the label comes out of the one and the per cent out of the
				// other, so a workbook that reads them from the wrong place
				// comes back as a table of bare numbers.
				{"xl/sharedStrings.xml", `<sst><si><t>도시</t></si><si><t>점유율</t></si><si><t>서울</t></si></sst>`},
				{"xl/styles.xml", `<styleSheet><cellXfs count="2">` +
					`<xf numFmtId="0"/><xf numFmtId="9" applyNumberFormat="1"/>` +
					`</cellXfs></styleSheet>`},
				{"xl/worksheets/sheet1.xml", `<worksheet><sheetData>` +
					`<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row>` +
					`<row r="2"><c r="A2" t="s"><v>2</v></c><c r="B2" s="1"><v>0.42</v></c></row>` +
					`</sheetData></worksheet>`},
			})
			document, err := Read("우리회사.xlsx", data)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			// A two-column sheet is drawn rather than tabled, so what proves
			// the parts were found is the label and the figure: the series
			// name and the row label out of the shared string table, and the
			// per cent out of the styles.
			for _, want := range []string{"점유율", "서울", "42%"} {
				if !strings.Contains(document.Source, want) {
					t.Errorf("%q is missing from the deck: %q", want, document.Source)
				}
			}
		})
	}
}
