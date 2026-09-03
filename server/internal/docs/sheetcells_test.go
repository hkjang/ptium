package docs

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// Where a cell sits is in its reference, and the reference is optional. A
// writer that streams a sheet out row by row leaves it off, and the cells of a
// row are then simply one after another. Reading them all as column A kept only
// the last of each row.

// sheetBook wraps one sheet's XML in the smallest workbook that can hold it.
func sheetBook(t *testing.T, sheet string) []byte {
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
		`<sheets><sheet name="분기 실적" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`)
	add("xl/worksheets/sheet1.xml", `<worksheet><sheetData>`+sheet+`</sheetData></worksheet>`)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func sheetSource(t *testing.T, sheet string) string {
	t.Helper()
	document, err := Read("실적.xlsx", sheetBook(t, sheet))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return document.Source
}

func TestACellWithoutAReferenceIsTheNextOneAlong(t *testing.T) {
	for _, test := range []struct {
		name  string
		sheet string
		want  []string
	}{
		{
			// The whole sheet written without references: two columns, and the
			// labels used to be lost under the figures.
			name: "a sheet written without any references",
			sheet: `<row><c t="inlineStr"><is><t>분기</t></is></c><c t="inlineStr"><is><t>매출</t></is></c></row>` +
				`<row><c t="inlineStr"><is><t>1분기</t></is></c><c><v>1180</v></c></row>` +
				`<row><c t="inlineStr"><is><t>2분기</t></is></c><c><v>1240</v></c></row>`,
			want: []string{"::columns 매출", "- 1분기 | 1180", "- 2분기 | 1240", "분기 실적!A1:B3"},
		},
		{
			// Three columns without references stay three columns.
			name: "a table written without any references",
			sheet: `<row><c t="inlineStr"><is><t>분기</t></is></c><c t="inlineStr"><is><t>매출</t></is></c><c t="inlineStr"><is><t>비고</t></is></c></row>` +
				`<row><c t="inlineStr"><is><t>1분기</t></is></c><c><v>1180</v></c><c t="inlineStr"><is><t>호조</t></is></c></row>`,
			want: []string{"::table 분기", "- 분기 | 매출 | 비고", "- 1분기 | 1180 | 호조"},
		},
		{
			// A reference where there is one, the next place along where there
			// is not.
			name: "a row that only references some of its cells",
			sheet: `<row r="1"><c r="A1" t="inlineStr"><is><t>분기</t></is></c><c t="inlineStr"><is><t>매출</t></is></c></row>` +
				`<row r="2"><c r="A2" t="inlineStr"><is><t>1분기</t></is></c><c><v>1180</v></c></row>`,
			want: []string{"::columns 매출", "- 1분기 | 1180"},
		},
		{
			// A reference that jumps a column is still obeyed, and what comes
			// after it carries on from there rather than from the start.
			name: "a reference that skips a column",
			sheet: `<row r="1"><c r="A1" t="inlineStr"><is><t>분기</t></is></c><c r="C1" t="inlineStr"><is><t>매출</t></is></c><c t="inlineStr"><is><t>비고</t></is></c></row>` +
				`<row r="2"><c r="A2" t="inlineStr"><is><t>1분기</t></is></c><c r="C2"><v>1180</v></c><c t="inlineStr"><is><t>호조</t></is></c></row>`,
			want: []string{"::table 분기", "- 분기 |  | 매출 | 비고", "- 1분기 |  | 1180 | 호조"},
		},
		{
			// A reference nobody can read is not a reason to stack the row into
			// one cell either.
			name: "a reference with no column in it",
			sheet: `<row><c r="1" t="inlineStr"><is><t>분기</t></is></c><c r="1" t="inlineStr"><is><t>매출</t></is></c></row>` +
				`<row><c r="2" t="inlineStr"><is><t>1분기</t></is></c><c r="2"><v>1180</v></c></row>`,
			want: []string{"::columns 매출", "- 1분기 | 1180"},
		},
		{
			// The references every ordinary export writes are still what says
			// where a cell goes, including the gap a missing cell leaves.
			name: "references are still obeyed on their own",
			sheet: `<row r="1"><c r="A1" t="inlineStr"><is><t>분기</t></is></c><c r="B1" t="inlineStr"><is><t>매출</t></is></c><c r="C1" t="inlineStr"><is><t>비고</t></is></c></row>` +
				`<row r="2"><c r="A2" t="inlineStr"><is><t>1분기</t></is></c><c r="C2" t="inlineStr"><is><t>호조</t></is></c></row>`,
			want: []string{"- 분기 | 매출 | 비고", "- 1분기 |  | 호조"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := sheetSource(t, test.sheet)
			for _, line := range test.want {
				if !strings.Contains(source, line) {
					t.Errorf("the deck is missing %q:\n%s", line, source)
				}
			}
		})
	}
}

func TestAColumnIsReadOutOfAReference(t *testing.T) {
	for _, test := range []struct {
		reference string
		want      int
	}{
		{"A1", 0},
		{"b2", 1},
		{"Z100", 25},
		{"AA1", 26},
		{"BC12", 54},
		{"", -1},
		{"12", -1},
		{"  C7  ", 2},
	} {
		if got := columnOf(test.reference); got != test.want {
			t.Errorf("columnOf(%q) = %d, want %d", test.reference, got, test.want)
		}
	}
}
