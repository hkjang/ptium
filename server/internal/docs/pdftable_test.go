package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A table in a PDF is not a table. It is cells drawn at coordinates, and the
// reader keeps a wide gap between two of them apart because that is what tells
// a column from a space — so every cell of a four-column table arrived as its
// own bullet, and a deck made from a report full of figures was a list of
// loose words.
func TestARowOfCellsIsReadAsATable(t *testing.T) {
	rows := [][]string{
		{"항목", "작년", "올해"},
		{"매출", "103억", "128억"},
		{"고객", "264곳", "312곳"},
	}
	table, end := tableAt(rows, 0)
	if table == nil || end != 3 || len(table) != 3 {
		t.Fatalf("three rows of three came back as %v (to %d)", table, end)
	}
	if table[1][2] != "128억" {
		t.Errorf("the table reads %v", table)
	}
}

func TestWhatIsNotATableIsLeftAlone(t *testing.T) {
	for _, one := range []struct {
		what string
		rows [][]string
	}{
		{"a line of prose", [][]string{{"한 줄짜리 문장입니다"}, {"또 한 줄"}}},
		{"one row beside a figure", [][]string{{"그림 1", "매출 추이"}, {"다음 문단입니다"}}},
		// A page set in two columns has two pieces on every baseline, and it is
		// prose rather than a table. The pieces are what tell them apart: a
		// table holds labels and figures, a column holds sentences.
		{"a page in two columns", [][]string{
			{strings.Repeat("가", 60), strings.Repeat("나", 60)},
			{strings.Repeat("다", 60), strings.Repeat("라", 60)},
		}},
	} {
		if table, _ := tableAt(one.rows, 0); table != nil {
			t.Errorf("%s was read as a table: %v", one.what, table)
		}
	}
}

// And the whole way through: a page whose cells sit on shared baselines comes
// out of the import as a table the deck can draw.
func TestAPDFTableReachesTheDeck(t *testing.T) {
	file, err := os.ReadFile(filepath.Join("testdata", "table-report.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	document, err := readPDF("보고.pdf", file)
	if err != nil {
		t.Fatalf("reading the report: %v", err)
	}
	if !strings.Contains(document.Source, "::table") {
		t.Errorf("the deck holds no table:\n%s", document.Source)
	}
	for _, want := range []string{"항목 | 작년 | 올해", "매출 | 103억 | 128억"} {
		if !strings.Contains(document.Source, want) {
			t.Errorf("the deck does not say %q:\n%s", want, document.Source)
		}
	}
}
