package docs

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// What a cell holds is in its type, and only one of the types is a number. A
// logical cell stores TRUE as 1, and a formula cell stores the text or the
// error it ended in. Reading all three as figures turned a checklist into a
// column of 0s and 1s — and, where the cell carried a format, into days and
// per cents of them.

// kindBook wraps one sheet and its styles in the smallest workbook that holds
// them. A sheet with no styles at all passes "" for them.
func kindBook(t *testing.T, styles, sheet string) []byte {
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
	add("xl/workbook.xml", `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`+
		`<sheets><sheet name="점검표" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`)
	if styles != "" {
		add("xl/styles.xml", styles)
	}
	add("xl/worksheets/sheet1.xml", `<worksheet><sheetData>`+sheet+`</sheetData></worksheet>`)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// Style 1 is a date and style 2 a per cent, which is what a stored 1 or 0 must
// not be read as when the cell is a logical one.
const kindStyles = `<styleSheet><cellXfs><xf numFmtId="0"/><xf numFmtId="14"/><xf numFmtId="9"/></cellXfs></styleSheet>`

func TestACellIsReadAsWhatItsTypeSaysItIs(t *testing.T) {
	for _, test := range []struct {
		name       string
		styles     string
		sheet      string
		want       []string
		wantAbsent []string
	}{
		{
			// A checklist: the sheet on screen says TRUE and FALSE, and the
			// deck used to say 1 and 0 — and to draw them as a bar chart,
			// because a column of 1s and 0s is a column of figures.
			name: "a logical cell is the word the sheet shows",
			sheet: `<row r="1"><c r="A1" t="inlineStr"><is><t>항목</t></is></c><c r="B1" t="inlineStr"><is><t>완료</t></is></c></row>` +
				`<row r="2"><c r="A2" t="inlineStr"><is><t>설계</t></is></c><c r="B2" t="b"><v>1</v></c></row>` +
				`<row r="3"><c r="A3" t="inlineStr"><is><t>개발</t></is></c><c r="B3" t="b"><v>0</v></c></row>`,
			want:       []string{"::table 항목", "- 설계 | TRUE", "- 개발 | FALSE"},
			wantAbsent: []string{"::columns 완료", "- 설계 | 1"},
		},
		{
			// The same cells with a format on them. A 1 under a date format is
			// a day and a 0 under a per cent is "0%", and a logical cell is
			// neither: it came back as "1899-12-31" and "0%".
			name:   "a logical cell with a format on it is still the word",
			styles: kindStyles,
			sheet: `<row r="1"><c r="A1" t="inlineStr"><is><t>항목</t></is></c><c r="B1" t="inlineStr"><is><t>완료</t></is></c></row>` +
				`<row r="2"><c r="A2" t="inlineStr"><is><t>설계</t></is></c><c r="B2" s="1" t="b"><v>1</v></c></row>` +
				`<row r="3"><c r="A3" t="inlineStr"><is><t>개발</t></is></c><c r="B3" s="2" t="b"><v>0</v></c></row>`,
			want:       []string{"- 설계 | TRUE", "- 개발 | FALSE"},
			wantAbsent: []string{"1899-12-31", "0%"},
		},
		{
			// A formula that returned text. The characters happen to read as a
			// number, and the cell happens to carry a date format, but what the
			// formula returned is the characters.
			name:   "a formula's text is not a day",
			styles: kindStyles,
			sheet: `<row r="1"><c r="A1" t="inlineStr"><is><t>항목</t></is></c><c r="B1" t="inlineStr"><is><t>코드</t></is></c></row>` +
				`<row r="2"><c r="A2" t="inlineStr"><is><t>설계</t></is></c><c r="B2" s="1" t="str"><v>45678</v></c></row>`,
			want:       []string{"- 설계 | 45678"},
			wantAbsent: []string{"2025-01-21"},
		},
		{
			// A formula that ended in an error says so, rather than being read
			// as a figure of some kind.
			name:   "a formula's error is said as it is",
			styles: kindStyles,
			sheet: `<row r="1"><c r="A1" t="inlineStr"><is><t>항목</t></is></c><c r="B1" t="inlineStr"><is><t>비율</t></is></c></row>` +
				`<row r="2"><c r="A2" t="inlineStr"><is><t>설계</t></is></c><c r="B2" s="2" t="e"><v>#DIV/0!</v></c></row>`,
			want: []string{"- 설계 | #DIV/0!"},
		},
		{
			// A logical cell holding neither of the two things it can hold is
			// left as it is rather than guessed at.
			name: "a logical cell holding something else is left alone",
			sheet: `<row r="1"><c r="A1" t="inlineStr"><is><t>항목</t></is></c><c r="B1" t="inlineStr"><is><t>완료</t></is></c></row>` +
				`<row r="2"><c r="A2" t="inlineStr"><is><t>설계</t></is></c><c r="B2" t="b"><v>참</v></c></row>`,
			want: []string{"- 설계 | 참"},
		},
		{
			// The numbers a sheet is mostly made of are untouched: a figure is
			// still a figure, and a formatted one is still what its format says.
			name:   "ordinary numbers are read as before",
			styles: kindStyles,
			sheet: `<row r="1"><c r="A1" t="inlineStr"><is><t>분기</t></is></c><c r="B1" t="inlineStr"><is><t>매출</t></is></c></row>` +
				`<row r="2"><c r="A2" t="inlineStr"><is><t>1분기</t></is></c><c r="B2"><v>1180</v></c></row>` +
				`<row r="3"><c r="A3" t="inlineStr"><is><t>2분기</t></is></c><c r="B3"><v>1240</v></c></row>`,
			want: []string{"::columns 매출", "- 1분기 | 1180", "- 2분기 | 1240"},
		},
		{
			// And a formatted number still is: the date a schedule keeps is a
			// day, not the count of days it stores.
			name:   "a formatted number is still what its format means",
			styles: kindStyles,
			sheet: `<row r="1"><c r="A1" t="inlineStr"><is><t>항목</t></is></c><c r="B1" t="inlineStr"><is><t>착수일</t></is></c></row>` +
				`<row r="2"><c r="A2" t="inlineStr"><is><t>설계</t></is></c><c r="B2" s="1"><v>45678</v></c></row>`,
			want: []string{"- 설계 | 2025-01-21"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			document, err := Read("점검표.xlsx", kindBook(t, test.styles, test.sheet))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			for _, line := range test.want {
				if !strings.Contains(document.Source, line) {
					t.Errorf("the deck is missing %q:\n%s", line, document.Source)
				}
			}
			for _, line := range test.wantAbsent {
				if strings.Contains(document.Source, line) {
					t.Errorf("the deck still says %q:\n%s", line, document.Source)
				}
			}
		})
	}
}

func TestALogicalCellIsWrittenAsAWord(t *testing.T) {
	for _, test := range []struct{ stored, want string }{
		{"1", "TRUE"},
		{"0", "FALSE"},
		{" 1 ", "TRUE"},
		{"", ""},
		{"2", "2"},
		{"TRUE", "TRUE"},
	} {
		if got := truthOf(test.stored); got != test.want {
			t.Errorf("truthOf(%q) = %q, want %q", test.stored, got, test.want)
		}
	}
}
