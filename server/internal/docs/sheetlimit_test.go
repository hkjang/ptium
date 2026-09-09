package docs

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// A deck holds so many slides, and a workbook can hold more sheets than that.
// What it used to say when it ran out — "시트가 많아 앞 30개만 가져왔습니다" — named
// none of the sheets it left behind, and counted in slides while saying
// sheets: a workbook whose sheets each fill three slides said it had taken the
// first thirty of its eleven.

// countedBook writes a workbook of numbered sheets, each with the same number
// of body rows under a header.
func countedBook(t *testing.T, sheets, bodyRows int) []byte {
	t.Helper()
	return countedBookOf(t, repeated(sheets, bodyRows))
}

// repeated is a run of sheets of the same length.
func repeated(count, rows int) []int {
	each := make([]int, count)
	for index := range each {
		each[index] = rows
	}
	return each
}

// countedBookOf writes a workbook whose sheets are as long as it is told, so a
// sheet with nothing on it can sit among sheets that have something.
func countedBookOf(t *testing.T, bodyRows []int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	add := func(name, body string) {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	var index, relationships strings.Builder
	for number, rows := range bodyRows {
		id := "rId" + strconv.Itoa(number+1)
		part := "worksheets/sheet" + strconv.Itoa(number+1) + ".xml"
		index.WriteString(`<sheet name="` + sheetNumbered(number+1) + `" r:id="` + id + `"/>`)
		relationships.WriteString(`<Relationship Id="` + id + `" Target="` + part + `"/>`)
		add("xl/"+part, `<worksheet><sheetData>`+countedRows(rows)+`</sheetData></worksheet>`)
	}
	add("xl/workbook.xml", `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`+
		`<sheets>`+index.String()+`</sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<Relationships>`+relationships.String()+`</Relationships>`)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// sheetNumbered names a sheet so that no name is the beginning of another:
// "시트3" inside "시트30" would answer a question nobody asked.
func sheetNumbered(number int) string {
	return fmt.Sprintf("시트%02d", number)
}

// countedRows writes a header and a labelled figure per row, which is a sheet
// a slide is made of.
func countedRows(body int) string {
	if body == 0 {
		return ""
	}
	rows := `<row r="1"><c r="A1" t="inlineStr"><is><t>항목</t></is></c>` +
		`<c r="B1" t="inlineStr"><is><t>금액</t></is></c></row>`
	for line := 0; line < body; line++ {
		at := strconv.Itoa(line + 2)
		rows += `<row r="` + at + `"><c r="A` + at + `" t="inlineStr"><is><t>` + at + `번</t></is></c>` +
			`<c r="B` + at + `"><v>` + strconv.Itoa(100+line) + `</v></c></row>`
	}
	return rows
}

// slidesIn counts the slides a deck has, which is how many cite where they
// came from. The cover cites nothing.
func slidesIn(source string) int {
	return strings.Count(source, "!source ")
}

func TestASheetLeftOutOfAFullDeckIsNamed(t *testing.T) {
	for _, test := range []struct {
		name    string
		book    []byte
		slides  int
		want    []string
		missing []string
	}{
		{
			// One slide per sheet: the deck fills at the thirtieth sheet, and
			// the four after it are the four that are not in it.
			name:   "more sheets than a deck holds",
			book:   countedBook(t, 34, 2),
			slides: maximumSlides,
			want:   []string{sheetNumbered(31), sheetNumbered(32), sheetNumbered(33), sheetNumbered(34)},
			// The number in the old warning was how many slides a deck holds,
			// said as though it were how many sheets were read.
			missing: []string{"앞 30개", sheetNumbered(30)},
		},
		{
			// Three slides per sheet: the deck fills at the tenth sheet, and
			// saying it took the first thirty sheets of twelve is a sentence
			// about a workbook nobody uploaded.
			name:    "sheets that each fill three slides",
			book:    countedBook(t, 12, 24),
			slides:  maximumSlides,
			want:    []string{sheetNumbered(11), sheetNumbered(12)},
			missing: []string{"앞 30개", sheetNumbered(10)},
		},
		{
			// Past a handful the names are the workbook's table of contents
			// written into one line, so what is left is how many.
			name:    "more sheets left out than a warning can name",
			book:    countedBook(t, 40, 2),
			slides:  maximumSlides,
			want:    []string{sheetNumbered(31), sheetNumbered(35), "외 5개"},
			missing: []string{sheetNumbered(36), sheetNumbered(40)},
		},
		{
			// An empty sheet was never going to be a slide, so it is not one of
			// the sheets that were left out.
			name:    "an empty sheet past the limit was not left out",
			book:    countedBookOf(t, append(repeated(maximumSlides, 2), 0, 2)),
			slides:  maximumSlides,
			want:    []string{sheetNumbered(32)},
			missing: []string{sheetNumbered(31)},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			document, err := Read("실적.xlsx", test.book)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if got := slidesIn(document.Source); got != test.slides {
				t.Errorf("the deck has %d slides, want %d", got, test.slides)
			}
			warnings := strings.Join(document.Warnings, "\n")
			for _, name := range test.want {
				if !strings.Contains(warnings, name) {
					t.Errorf("the warnings do not say %q: %q", name, warnings)
				}
			}
			for _, name := range test.missing {
				if strings.Contains(warnings, name) {
					t.Errorf("the warnings say %q, which was not left out: %q", name, warnings)
				}
			}
			// A sheet the deck says it left out is not in the deck, and the
			// last sheet it did take is.
			if strings.Contains(document.Source, "# "+sheetNumbered(31)) {
				t.Errorf("the deck has a sheet it says it left out:\n%s", document.Source)
			}
		})
	}
}

func TestAWorkbookThatFitsWarnsAboutNothing(t *testing.T) {
	document, err := Read("실적.xlsx", countedBook(t, maximumSlides, 2))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := slidesIn(document.Source); got != maximumSlides {
		t.Errorf("the deck has %d slides, want %d", got, maximumSlides)
	}
	if len(document.Warnings) > 0 {
		t.Errorf("every sheet was taken, but the deck warns: %q", document.Warnings)
	}
}

func TestSheetsAreNamedWhileNamingThemHelps(t *testing.T) {
	for _, test := range []struct {
		names []string
		want  string
	}{
		{nil, ""},
		{[]string{"코드표"}, "코드표"},
		{[]string{"코드표", "설정"}, "코드표, 설정"},
		{[]string{"1", "2", "3", "4", "5"}, "1, 2, 3, 4, 5"},
		{[]string{"1", "2", "3", "4", "5", "6"}, "1, 2, 3, 4, 5 외 1개"},
		{[]string{"1", "2", "3", "4", "5", "6", "7", "8"}, "1, 2, 3, 4, 5 외 3개"},
	} {
		if got := sheetsNamed(test.names); got != test.want {
			t.Errorf("sheetsNamed(%q) = %q, want %q", test.names, got, test.want)
		}
	}
}
