package docs

import (
	"strings"
	"testing"
)

// A workbook saved as "Strict Open XML Spreadsheet" — the shape an archive or a
// public body asks for — writes a date as the date itself rather than as a
// count of days: t="d" and "2025-01-21T13:30:00". Read as a count, it is not a
// number at all, so it went through untouched and the deck showed a delivery
// date with a T in the middle of it, whatever format the cell carried.

// momentStyles: a date, a date with a time on it, General, a time, a time with
// seconds, a per cent, a spelt-out date and time with seconds, and a length of
// time — in the order the styles are numbered.
const momentStyles = `<styleSheet>` +
	`<numFmts><numFmt numFmtId="164" formatCode="yyyy-mm-dd hh:mm:ss"/></numFmts>` +
	`<cellXfs>` +
	`<xf numFmtId="14"/><xf numFmtId="22"/><xf numFmtId="0"/><xf numFmtId="18"/>` +
	`<xf numFmtId="19"/><xf numFmtId="9"/><xf numFmtId="164"/><xf numFmtId="46"/>` +
	`</cellXfs></styleSheet>`

func TestAMomentIsWrittenTheWayTheFormatShowsIt(t *testing.T) {
	formats := readCellFormats([]byte(momentStyles), false)
	for _, one := range []struct {
		what          string
		style, stored string
		want          string
		changed       bool
	}{
		// The format says how much of the moment the sheet shows, the same as
		// it does for a count of days.
		{"a date keeps its day", "0", "2025-01-21T13:30:00", "2025-01-21", true},
		{"a date and a time keep both", "1", "2025-01-21T13:30:00", "2025-01-21 13:30", true},
		{"a spelt-out format that says seconds keeps them", "6", "2025-01-21T13:30:45", "2025-01-21 13:30:45", true},
		{"a time keeps the clock alone", "3", "2025-01-21T13:30:45", "13:30", true},
		{"a time with seconds keeps them", "4", "2025-01-21T13:30:45", "13:30:45", true},
		// A date format on a cell that stores no clock is still a date.
		{"a day on its own", "0", "2025-01-21", "2025-01-21", true},
		{"a day on its own asked for its clock", "1", "2025-01-21", "2025-01-21 00:00", true},
		// Told nothing about the cell, write what the moment itself holds.
		{"General keeps the day when the clock is midnight", "2", "2025-01-21T00:00:00", "2025-01-21", true},
		{"General keeps the clock when there is one", "2", "2025-01-21T13:30:00", "2025-01-21 13:30", true},
		{"General keeps the seconds when there are some", "2", "2025-01-21T13:30:45", "2025-01-21 13:30:45", true},
		{"a cell with no style at all", "", "2025-01-21T13:30:00", "2025-01-21 13:30", true},
		// A per cent and a length of time are not what a moment is; the moment
		// is written as the moment rather than forced through either.
		{"a per cent format on a moment", "5", "2025-01-21T13:30:00", "2025-01-21 13:30", true},
		{"a length-of-time format on a moment", "7", "2025-01-21T13:30:00", "2025-01-21 13:30", true},
		// A style number past the end of the table is no style.
		{"a style nobody wrote down", "99", "2025-01-21", "2025-01-21", true},
		// What is not a moment is left exactly as it is.
		{"words", "0", "미정", "미정", false},
		{"an empty cell", "0", "", "", false},
		{"a count of days in a cell that says it holds a date", "0", "45678", "45678", false},
		{"a day nobody finished writing", "0", "2025-01", "2025-01", false},
		{"a day that is not a day", "0", "2025-02-30T00:00:00", "2025-02-30T00:00:00", false},
	} {
		got, changed := formats.moment(one.style, one.stored)
		if got != one.want || changed != one.changed {
			t.Errorf("%s: moment(%q, %q) = (%q, %v), want (%q, %v)",
				one.what, one.style, one.stored, got, changed, one.want, one.changed)
		}
	}
}

func TestAMomentIsReadInTheShapesAWorkbookWritesIt(t *testing.T) {
	for _, one := range []struct {
		what, stored string
		want         string // the clock as it was written down, or "" for unread
	}{
		{"a day and a clock", "2025-01-21T13:30:00", "2025-01-21 13:30:00"},
		{"a day and a clock without seconds", "2025-01-21T13:30", "2025-01-21 13:30:00"},
		{"a day on its own", "2025-01-21", "2025-01-21 00:00:00"},
		{"a fraction of a second", "2025-01-21T13:30:00.5", "2025-01-21 13:30:00"},
		// The zone it is kept at is not converted away: 13:30 in Seoul stays
		// 13:30, rather than becoming 04:30 in the deck.
		{"a clock kept at an offset", "2025-01-21T13:30:00+09:00", "2025-01-21 13:30:00"},
		{"a clock kept at Z", "2025-01-21T13:30:00Z", "2025-01-21 13:30:00"},
		{"space around it", "  2025-01-21T13:30:00  ", "2025-01-21 13:30:00"},
		{"nothing", "", ""},
		{"words", "미정", ""},
		{"a count of days", "45678", ""},
	} {
		moment, ok := isoMoment(one.stored)
		got := ""
		if ok {
			got = moment.Format("2006-01-02 15:04:05")
		}
		if got != one.want {
			t.Errorf("%s: isoMoment(%q) = %q, want %q", one.what, one.stored, got, one.want)
		}
	}
}

// End to end: the strict schema's dates reach the slide as days.
func TestAStrictWorkbooksDatesReachTheSlideAsDays(t *testing.T) {
	book := dayBook(t, "", momentStyles,
		`<row r="1"><c r="A1" t="inlineStr"><is><t>납기</t></is></c>`+
			`<c r="B1" t="inlineStr"><is><t>회의</t></is></c></row>`+
			`<row r="2"><c r="A2" s="0" t="d"><v>2025-01-21T13:30:00</v></c>`+
			`<c r="B2" s="1" t="d"><v>2025-01-21T13:30:00</v></c></row>`+
			`<row r="3"><c r="A3" s="0" t="d"><v>2025-02-03</v></c>`+
			`<c r="B3" s="3" t="d"><v>2025-02-03T09:00:00</v></c></row>`)
	document, err := readWorkbook("일정.xlsx", book)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"2025-01-21", "2025-01-21 13:30", "2025-02-03", "09:00"} {
		if !strings.Contains(document.Source, want) {
			t.Errorf("the deck does not say %q:\n%s", want, document.Source)
		}
	}
	// The characters the cell stores are what nobody should be reading.
	if strings.Contains(document.Source, "T13:30") || strings.Contains(document.Source, "T09:00") {
		t.Errorf("the deck still shows the stored characters:\n%s", document.Source)
	}
}
