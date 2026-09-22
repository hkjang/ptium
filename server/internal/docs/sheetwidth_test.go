package docs

import (
	"archive/zip"
	"bytes"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// widthBook writes a one-sheet workbook from cells given as the reference each
// one carries and the text in it, so a test can write a reference no sheet
// could hold. Kept as real xlsx bytes for the public reader, the same as the
// other sheet tests here.
func widthBook(t *testing.T, columns string, rows [][][2]string) []byte {
	t.Helper()
	var sheet strings.Builder
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	sheet.WriteString(columns)
	sheet.WriteString(`<sheetData>`)
	for index, row := range rows {
		fmt.Fprintf(&sheet, `<row r="%d">`, index+1)
		for _, cell := range row {
			fmt.Fprintf(&sheet, `<c r="%s" t="inlineStr"><is><t>%s</t></is></c>`, cell[0], cell[1])
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range map[string]string{
		"xl/workbook.xml":            `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="분기 실적" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   sheet.String(),
	} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// widthTableRows is every table row the deck was given, in order.
func widthTableRows(parsed deck.Source) [][]string {
	var rows [][]string
	for _, slide := range parsed.Slides {
		for _, block := range slide.Blocks {
			if block.Kind == pptx.BlockTable {
				rows = append(rows, block.Rows...)
			}
		}
	}
	return rows
}

func TestColumnLettersRunToTheFarEdgeOfTheSheet(t *testing.T) {
	for _, item := range []struct {
		index int
		want  string
	}{
		{-3, "A"},
		{0, "A"},
		{25, "Z"},
		{26, "AA"},
		{51, "AZ"},
		{52, "BA"},
		{701, "ZZ"},
		{702, "AAA"},
		{703, "AAB"},
		{16383, "XFD"},
	} {
		if got := columnLetter(item.index); got != item.want {
			t.Errorf("columnLetter(%d) = %q, want %q", item.index, got, item.want)
		}
	}
}

func TestColumnOfStopsAtTheEdgeOfTheSheet(t *testing.T) {
	for _, item := range []struct {
		reference string
		want      int
	}{
		{"A1", 0},
		{"BC12", 54},
		{"ZZ9", 701},
		{"AAA9", 702},
		// The last column a sheet has, and the first one it does not.
		{"XFD1", 16383},
		{"XFE1", -1},
		{"AAAAA1", -1},
		// Longer than an int can count to, had it kept counting.
		{"AAAAAAAAAAAAAAAAAAAA1", -1},
		{"1", -1},
		{"", -1},
	} {
		if got := columnOf(item.reference); got != item.want {
			t.Errorf("columnOf(%q) = %d, want %d", item.reference, got, item.want)
		}
	}
}

// A reference to a column the sheet does not have is a reference that names no
// column, and a cell with no reference is the next one along. Dropping it would
// lose what was written in it without saying so.
func TestACellPastTheEdgeOfTheSheetLandsAfterTheCellBeforeIt(t *testing.T) {
	for _, reference := range []string{"XFE", "AAAAA", "AAAAAAAAAA"} {
		t.Run(reference, func(t *testing.T) {
			document, err := Read("실적.xlsx", widthBook(t, "", [][][2]string{
				{{"A1", "지역"}, {"B1", "매출"}, {"C1", "비고"}},
				{{"A2", "서울"}, {reference + "2", "1200"}, {"C2", "검토"}},
				{{"A3", "부산"}, {"B3", "980"}, {"C3", "완료"}},
			}))
			if err != nil {
				t.Fatal(err)
			}
			if len(document.Warnings) != 0 {
				t.Errorf("warnings: %v", document.Warnings)
			}
			want := [][]string{
				{"지역", "매출", "비고"},
				{"서울", "1200", "검토"},
				{"부산", "980", "완료"},
			}
			if got := widthTableRows(deck.ParseSource(document.Source)); !reflect.DeepEqual(got, want) {
				t.Errorf("table rows = %#v, want %#v", got, want)
			}
			wantSource := "!source 실적.xlsx | 분기 실적!A1:C3\n"
			if !strings.Contains(document.Source, wantSource) {
				t.Errorf("missing source %q:\n%s", wantSource, document.Source)
			}
		})
	}
}

// One cell is enough to ask for a row of twelve million columns, and every row
// of the sheet is then made that wide. A sheet of nine cells is a sheet of nine
// cells however its references are written.
func TestASheetOfNineCellsStaysASheetOfNineCells(t *testing.T) {
	book := widthBook(t, "", [][][2]string{
		{{"A1", "지역"}, {"B1", "매출"}, {"C1", "비고"}},
		{{"A2", "서울"}, {"AAAAA2", "1200"}, {"C2", "검토"}},
		{{"A3", "부산"}, {"B3", "980"}, {"C3", "완료"}},
	})
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if _, err := Read("실적.xlsx", book); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	// Well under the seven megabytes one such row costs, and far above the
	// hundreds of kilobytes reading nine cells takes.
	const ceiling = 2 << 20
	if grew := after.TotalAlloc - before.TotalAlloc; grew > ceiling {
		t.Errorf("reading nine cells allocated %d bytes, want under %d", grew, ceiling)
	}
}

// Past ZZ a column is named with three letters, and the arithmetic that stops
// at two wrote "[A" into the source a slide cites.
func TestACitationNamesAColumnPastTwoLetters(t *testing.T) {
	// A through ZY are folded away, so the two columns the sheet shows are ZZ
	// and AAA — exactly where two letters run out.
	document, err := Read("실적.xlsx", widthBook(t, `<cols><col min="1" max="701" hidden="1"/></cols>`, [][][2]string{
		{{"ZZ1", "지역"}, {"AAA1", "매출"}},
		{{"ZZ2", "서울"}, {"AAA2", "1200"}},
		{{"ZZ3", "부산"}, {"AAA3", "980"}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	// The folded columns hold nothing, so there is nothing to say was left out.
	if len(document.Warnings) != 0 {
		t.Errorf("warnings: %v", document.Warnings)
	}
	wantSource := "!source 실적.xlsx | 분기 실적!A1:AAA3\n"
	if !strings.Contains(document.Source, wantSource) {
		t.Errorf("missing source %q:\n%s", wantSource, document.Source)
	}
	if strings.ContainsAny(document.Source, "[]") {
		t.Errorf("source names a column with a character no column has:\n%s", document.Source)
	}
	parsed := deck.ParseSource(document.Source)
	var locators []string
	for _, slide := range parsed.Slides {
		for _, citation := range slide.Sources {
			locators = append(locators, citation.Locator)
		}
	}
	if want := []string{"분기 실적!A1:AAA3"}; !reflect.DeepEqual(locators, want) {
		t.Errorf("cited locators = %#v, want %#v", locators, want)
	}
}
