package docs

import (
	"archive/zip"
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// craftWorkbook writes a one-sheet .xlsx whose every row holds a cell in
// column A and another at the reference given, which is what decides how wide
// each row is laid out.
func craftWorkbook(rows int, farColumn string) []byte {
	var sheet strings.Builder
	sheet.WriteString(`<worksheet><sheetData>`)
	for r := 1; r <= rows; r++ {
		fmt.Fprintf(&sheet, `<row r="%d"><c r="A%d" t="inlineStr"><is><t>서울</t></is></c>`, r, r)
		if farColumn != "" {
			fmt.Fprintf(&sheet, `<c r="%s%d" t="inlineStr"><is><t>1200</t></is></c>`, farColumn, r)
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)

	buffer := &bytes.Buffer{}
	writer := zip.NewWriter(buffer)
	add := func(name, body string) {
		file, _ := writer.Create(name)
		file.Write([]byte(body))
	}
	add("[Content_Types].xml", `<Types/>`)
	add("xl/_rels/workbook.xml.rels", `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`)
	add("xl/workbook.xml", `<workbook><sheets><sheet name="표" sheetId="1" id="rId1"/></sheets></workbook>`)
	add("xl/worksheets/sheet1.xml", sheet.String())
	writer.Close()
	return buffer.Bytes()
}

// A sheet whose every row ends at the far edge is laid out the whole way
// across on every one of them. Bounding the column alone left a four hundred
// kilobyte upload reaching eleven gigabytes, so the cells read are counted.
func TestASheetOfFarEdgesDoesNotCostMoreThanASheet(t *testing.T) {
	data := craftWorkbook(60000, "XFD")
	if len(data) > 1<<20 {
		t.Fatalf("the crafted upload got large enough to be the bound itself: %d bytes", len(data))
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	document, err := Read("probe.xlsx", data)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatalf("the workbook should still be read, in part: %v", err)
	}

	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("upload %d bytes, allocated %.0f MB", len(data), float64(allocated)/(1<<20))
	if allocated > 1<<30 {
		t.Errorf("reading a %d byte upload allocated %.1f GB", len(data), float64(allocated)/(1<<30))
	}

	// What was not read has to be said, or a deck made of the first rows of a
	// sheet looks like a deck made of the sheet.
	said := false
	for _, warning := range document.Warnings {
		if strings.Contains(warning, "읽지 않았습니다") {
			said = true
			t.Logf("warning: %s", warning)
		}
	}
	if !said {
		t.Errorf("rows went unread and nothing said so: %v", document.Warnings)
	}
}

// The budget is spent on width, so an ordinary narrow sheet of the same many
// rows is still read whole and says nothing about rows it left out.
func TestANarrowSheetOfManyRowsIsStillReadWhole(t *testing.T) {
	document, err := Read("narrow.xlsx", craftWorkbook(20000, "B"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, warning := range document.Warnings {
		if strings.Contains(warning, "읽지 않았습니다") {
			t.Errorf("a narrow sheet was cut short: %s", warning)
		}
	}
}
