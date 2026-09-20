package docs

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// A sheet hides rows and columns the way a workbook hides sheets, and for the
// same reasons: the filter that took last year's rows out of view, the group
// somebody folded up, the column of codes a formula looks up in. What is on
// the screen is the table; what was hidden was never meant to be in it.

// concealedBook writes a workbook of one sheet, "분기 실적", with whatever the
// sheet says about its columns ahead of its rows.
func concealedBook(t *testing.T, cols, rows string) []byte {
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
		`<sheets><sheet name="분기 실적" r:id="rId1"/></sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`)
	add("xl/worksheets/sheet1.xml", `<worksheet>`+cols+`<sheetData>`+rows+`</sheetData></worksheet>`)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// textCell is an inline-string cell.
func textCell(reference, value string) string {
	return `<c r="` + reference + `" t="inlineStr"><is><t>` + value + `</t></is></c>`
}

// numberCell is a number cell.
func numberCell(reference, value string) string {
	return `<c r="` + reference + `"><v>` + value + `</v></c>`
}

func TestAHiddenRowIsNotInTheTable(t *testing.T) {
	// A three-column sheet, so it is a table, with the rows a filter took out
	// of view hidden: one the way Excel writes it, one the way a hand writes
	// it. The hidden row with nothing in it was never on the screen either
	// way, so it is not one of the rows the warning counts.
	rows := `<row r="1">` + textCell("A1", "분기") + textCell("B1", "매출") + textCell("C1", "비고") + `</row>` +
		`<row r="2">` + textCell("A2", "1분기") + numberCell("B2", "1180") + textCell("C2", "확정") + `</row>` +
		`<row r="3" hidden="1">` + textCell("A3", "2분기") + numberCell("B3", "1240") + textCell("C3", "작업") + `</row>` +
		`<row r="4">` + textCell("A4", "3분기") + numberCell("B4", "1310") + textCell("C4", "확정") + `</row>` +
		`<row r="5" hidden="true">` + textCell("A5", "4분기") + numberCell("B5", "1290") + textCell("C5", "작업") + `</row>` +
		`<row r="6" hidden="1"/>` +
		`<row r="7">` + textCell("A7", "합계") + numberCell("B7", "2490") + textCell("C7", "") + `</row>`
	document, err := Read("실적.xlsx", concealedBook(t, "", rows))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, line := range []string{"- 2분기 |", "- 4분기 |"} {
		if strings.Contains(document.Source, line) {
			t.Errorf("the table has %q, which the sheet hides:\n%s", line, document.Source)
		}
	}
	shown := []string{"- 분기 | 매출 | 비고", "- 1분기 | 1180 | 확정", "- 3분기 | 1310 | 확정", "- 합계 | 2490 |"}
	at := -1
	for _, line := range shown {
		index := strings.Index(document.Source, line)
		if index < 0 {
			t.Errorf("the table is missing %q:\n%s", line, document.Source)
			continue
		}
		if index < at {
			t.Errorf("%q is out of order:\n%s", line, document.Source)
		}
		at = index
	}
	warnings := strings.Join(document.Warnings, "\n")
	if !strings.Contains(warnings, "분기 실적의 숨긴 행 2개는 가져오지 않았습니다") {
		t.Errorf("the warnings do not say two rows were hidden: %q", warnings)
	}
	if strings.Contains(warnings, "열") {
		t.Errorf("no column was hidden, but the warnings mention one: %q", warnings)
	}
}

func TestAHiddenColumnIsCutOutOfEveryRow(t *testing.T) {
	// Columns B and C are folded away. Left as blanks, a hidden column in the
	// middle of the sheet stays in the table as an empty column, since only
	// the trailing ones are trimmed; cut out, A and D sit next to each other
	// the way they do on the screen.
	cols := `<cols><col min="1" max="1" width="12"/><col min="2" max="3" hidden="1"/></cols>`
	rows := `<row r="1">` + textCell("A1", "분기") + textCell("B1", "코드") + numberCell("C1", "1") + textCell("D1", "비고") + `</row>` +
		`<row r="2">` + textCell("A2", "1분기") + textCell("B2", "Q1") + numberCell("C2", "10") + textCell("D2", "확정") + `</row>` +
		`<row r="3">` + textCell("A3", "2분기") + textCell("B3", "Q2") + numberCell("C3", "20") + textCell("D3", "작업") + `</row>`
	document, err := Read("실적.xlsx", concealedBook(t, cols, rows))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, line := range []string{"- 분기 | 비고\n", "- 1분기 | 확정\n", "- 2분기 | 작업\n"} {
		if !strings.Contains(document.Source, line) {
			t.Errorf("the table is missing %q:\n%s", line, document.Source)
		}
	}
	for _, word := range []string{"코드", "Q1", "| 10 |"} {
		if strings.Contains(document.Source, word) {
			t.Errorf("the table has %q, which the sheet hides:\n%s", word, document.Source)
		}
	}
	if !strings.Contains(document.Source, "!A1:B3") {
		t.Errorf("the source range does not count the two columns that are shown:\n%s", document.Source)
	}
	warnings := strings.Join(document.Warnings, "\n")
	if !strings.Contains(warnings, "분기 실적의 숨긴 열 2개는 가져오지 않았습니다") {
		t.Errorf("the warnings do not say two columns were hidden: %q", warnings)
	}
}

func TestHidingAHelperColumnLeavesTheSheetAChart(t *testing.T) {
	// A label column, a column of figures, and beside them the helper column
	// a formula works from, hidden so nobody sees it. What is on the screen is
	// the two-column sheet a person would draw as a chart; with the helper
	// column read in it is three columns wide, and a table. Whether the sheet
	// becomes a chart or a table is decided on what is left after hiding, and
	// nothing else about that decision moves.
	rows := `<row r="1">` + textCell("A1", "분기") + textCell("B1", "매출") + textCell("C1", "계수") + `</row>` +
		`<row r="2">` + textCell("A2", "1분기") + numberCell("B2", "1180") + numberCell("C2", "0.9") + `</row>` +
		`<row r="3">` + textCell("A3", "2분기") + numberCell("B3", "1240") + numberCell("C3", "1.1") + `</row>`
	for _, test := range []struct {
		name    string
		cols    string
		want    string
		missing string
		warning string
	}{
		{
			name:    "with the helper column shown",
			want:    "::table 분기\n",
			missing: "::columns",
		},
		{
			name:    "with the helper column hidden",
			cols:    `<cols><col min="3" max="3" hidden="1"/></cols>`,
			want:    "::columns 매출\n- 1분기 | 1180\n- 2분기 | 1240\n",
			missing: "::table",
			warning: "분기 실적의 숨긴 열 1개는 가져오지 않았습니다",
		},
		{
			// A range of columns the sheet says nothing about, in the ways a
			// sheet says nothing: no hidden attribute, hidden switched off, a
			// range with no bounds. None of it hides anything.
			name: "with column settings that hide nothing",
			cols: `<cols><col min="1" max="3" width="14" customWidth="1"/><col min="3" max="3" hidden="0"/><col hidden="1"/></cols>`,
			want: "::table 분기\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			document, err := Read("실적.xlsx", concealedBook(t, test.cols, rows))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !strings.Contains(document.Source, test.want) {
				t.Errorf("the deck is missing %q:\n%s", test.want, document.Source)
			}
			if test.missing != "" && strings.Contains(document.Source, test.missing) {
				t.Errorf("the deck has %q:\n%s", test.missing, document.Source)
			}
			warnings := strings.Join(document.Warnings, "\n")
			if test.warning == "" {
				if warnings != "" {
					t.Errorf("nothing was hidden, but the deck warns: %s", warnings)
				}
				return
			}
			if !strings.Contains(warnings, test.warning) {
				t.Errorf("the warnings do not say what was hidden: %q", warnings)
			}
		})
	}
}

func TestHiddenRowsAndColumnsAreCountedInOneLine(t *testing.T) {
	// Both at once is one warning, not two, and it counts only what was on
	// the screen: a hidden column whose only cell is in a hidden row was
	// already counted with the row.
	cols := `<cols><col min="3" max="3" hidden="1"/><col min="5" max="16384" hidden="1"/></cols>`
	rows := `<row r="1">` + textCell("A1", "분기") + textCell("B1", "매출") + textCell("C1", "계수") + textCell("D1", "비고") + `</row>` +
		`<row r="2">` + textCell("A2", "1분기") + numberCell("B2", "1180") + numberCell("C2", "0.9") + textCell("D2", "확정") + `</row>` +
		`<row r="3" hidden="1">` + textCell("A3", "2분기") + numberCell("B3", "1240") + numberCell("C3", "1.1") + textCell("D3", "작업") + `</row>`
	document, err := Read("실적.xlsx", concealedBook(t, cols, rows))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(document.Source, "- 1분기 | 1180 | 확정\n") {
		t.Errorf("the table does not keep A, B and D together:\n%s", document.Source)
	}
	if len(document.Warnings) != 1 {
		t.Fatalf("want one warning, got %d: %q", len(document.Warnings), document.Warnings)
	}
	if document.Warnings[0] != "분기 실적의 숨긴 행 1개와 열 1개는 가져오지 않았습니다" {
		t.Errorf("the warning does not count one row and one column: %q", document.Warnings[0])
	}
}

func TestASheetHiddenRowByRowIsNotASlide(t *testing.T) {
	// Every row hidden is a sheet with nothing on the screen, which is the
	// same as an empty sheet: not a slide, and when it is the only sheet, the
	// workbook has no table to read.
	rows := `<row r="1" hidden="1">` + textCell("A1", "분기") + textCell("B1", "매출") + `</row>` +
		`<row r="2" hidden="1">` + textCell("A2", "1분기") + numberCell("B2", "1180") + `</row>` +
		`<row r="3" hidden="1">` + textCell("A3", "2분기") + numberCell("B3", "1240") + `</row>`
	_, err := Read("실적.xlsx", concealedBook(t, "", rows))
	if err == nil {
		t.Fatal("a sheet of hidden rows was read as a deck")
	}
	if err.Error() != "이 통합 문서에는 읽을 표가 없습니다" {
		t.Errorf("unexpected message: %v", err)
	}
}
