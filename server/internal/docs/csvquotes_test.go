package docs

import (
	"strings"
	"testing"
)

// One quote nobody meant as a quote used to refuse the whole file.
//
// A price list writes 15" 모니터, an export writes a width as 21", and the
// strict CSV reader stops at that character and returns nothing: five hundred
// rows refused for one of them, and the person is shown an English parser
// message about a column. The file is read now, with the quote taken as the
// character it is.
func TestAQuoteInAnUnquotedFieldDoesNotRefuseTheFile(t *testing.T) {
	document, err := Read("가격.csv", []byte("제품,가격\n15\" 모니터,120000\n27\" 모니터,300000\n"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, line := range []string{"::columns 가격", `- 15" 모니터 | 120000`, `- 27" 모니터 | 300000`} {
		if !strings.Contains(document.Source, line) {
			t.Errorf("the deck is missing %q:\n%s", line, document.Source)
		}
	}
}

// And it says which line it forgave, in case what is there was meant to be a
// quoted field after all.
func TestAForgivenQuoteIsSaidWithItsLine(t *testing.T) {
	document, err := Read("가격.csv", []byte("제품,가격\n모니터,120000\n27\" 모니터,300000\n"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	warned := strings.Join(document.Warnings, "\n")
	for _, part := range []string{"가격.csv", "3번째 줄", "따옴표"} {
		if !strings.Contains(warned, part) {
			t.Errorf("the warning is missing %q: %v", part, document.Warnings)
		}
	}
}

// A quote that never closes runs to the end of the file. What it swallows is
// not what somebody wrote, but a deck with the file's own header and a warning
// naming the line the quote was opened on is more use than a refusal.
func TestAQuoteThatNeverClosesIsReadRatherThanRefused(t *testing.T) {
	document, err := Read("가격.csv", []byte("제품,가격\n\"모니터,120000\n키보드,30000\n"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(document.Source, "::table 제품") {
		t.Errorf("the deck has no table:\n%s", document.Source)
	}
	if !strings.Contains(strings.Join(document.Warnings, "\n"), "2번째 줄") {
		t.Errorf("the warning does not name the line the quote was opened on: %v", document.Warnings)
	}
}

// A tab-separated file is read the same way.
func TestAQuoteInATabSeparatedFileIsForgiven(t *testing.T) {
	document, err := Read("가격.tsv", []byte("제품\t가격\n15\" 모니터\t120000\n27\" 모니터\t300000\n"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(document.Source, `- 15" 모니터 | 120000`) {
		t.Errorf("the deck is missing the row:\n%s", document.Source)
	}
}

// What the strict reader accepts is still read strictly: a quoted field holds
// its separator and its doubled quotes, and nothing is warned about.
func TestAProperlyQuotedFileIsReadWithoutAWarning(t *testing.T) {
	document, err := Read("실적.csv", []byte(
		"구분,내용\n\"1분기, 상반기\",\"그는 \"\"좋다\"\" 고 했다\"\n2분기,계획대로\n"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, line := range []string{"- 1분기, 상반기 | 그는 \"좋다\" 고 했다", "- 2분기 | 계획대로"} {
		if !strings.Contains(document.Source, line) {
			t.Errorf("the deck is missing %q:\n%s", line, document.Source)
		}
	}
	if len(document.Warnings) != 0 {
		t.Errorf("a file with nothing wrong with it was warned about: %v", document.Warnings)
	}
}

// The rows a forgiving read produces are the file's own rows, in order, with
// the quote where the file put it.
func TestForgivingAQuoteKeepsTheOtherRowsIntact(t *testing.T) {
	rows, forgiven, err := separatedRows("가,나\n1,2\n3\",4\n5,6\n", ',')
	if err != nil {
		t.Fatalf("rows: %v", err)
	}
	if forgiven != 3 {
		t.Errorf("forgave line %d, not the line the quote is on", forgiven)
	}
	expected := [][]string{{"가", "나"}, {"1", "2"}, {`3"`, "4"}, {"5", "6"}}
	if len(rows) != len(expected) {
		t.Fatalf("read %d rows, wanted %d: %q", len(rows), len(expected), rows)
	}
	for index, row := range expected {
		if strings.Join(rows[index], "|") != strings.Join(row, "|") {
			t.Errorf("row %d is %q, wanted %q", index+1, rows[index], row)
		}
	}
}
