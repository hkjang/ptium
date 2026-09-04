package docs

import (
	"strings"
	"testing"
)

// A spreadsheet keeps a time of day as the part of a day that has gone by, so a
// meeting at half past one is stored 0.5625 and a shift of a day and a half is
// stored 1.5. Read as the numbers they are, an agenda came back as a column of
// fractions between 0 and 1, and a timesheet's total said "1.5" where the sheet
// said "36:00" — and a date with a time on it lost the time it was there for.

// timeStyles are the clock formats, in the order the styles below are numbered.
const timeStyles = `<styleSheet><numFmts>
	<numFmt numFmtId="164" formatCode="h:mm"/>
	<numFmt numFmtId="165" formatCode="h:mm:ss"/>
	<numFmt numFmtId="166" formatCode="h:mm AM/PM"/>
	<numFmt numFmtId="167" formatCode="yyyy-mm-dd hh:mm"/>
	<numFmt numFmtId="168" formatCode="[h]:mm:ss"/>
	<numFmt numFmtId="169" formatCode="[mm]:ss"/>
	<numFmt numFmtId="170" formatCode="yyyy-mm-dd"/>
	<numFmt numFmtId="171" formatCode="0.00&quot;h&quot;"/>
	<numFmt numFmtId="172" formatCode="[Red]h:mm"/>
</numFmts><cellXfs>
	<xf numFmtId="164"/><xf numFmtId="165"/><xf numFmtId="166"/><xf numFmtId="167"/>
	<xf numFmtId="168"/><xf numFmtId="169"/><xf numFmtId="170"/><xf numFmtId="171"/>
	<xf numFmtId="172"/>
	<xf numFmtId="20"/><xf numFmtId="21"/><xf numFmtId="22"/><xf numFmtId="46"/><xf numFmtId="0"/>
</cellXfs></styleSheet>`

func TestATimeIsWrittenAsTheClockTheSheetShows(t *testing.T) {
	formats := readCellFormats([]byte(timeStyles))
	for _, one := range []struct {
		what          string
		style, stored string
		want          string
		changed       bool
	}{
		{"a time of day", "0", "0.5625", "13:30", true},
		{"a time of day with its seconds", "1", "0.5625", "13:30:00", true},
		// The clock is written the one way, as a date is: what a format asks for
		// is how to print it, and the deck says the moment.
		{"a time written for the afternoon is still the clock", "2", "0.5625", "13:30", true},
		{"a date with a time on it keeps both", "3", "45678.5625", "2025-01-21 13:30", true},
		// A length of time runs past the end of a day. Read as a time of day, a
		// day and a half of machine time came back as midday.
		{"a length of time is counted whole", "4", "1.5", "36:00:00", true},
		{"a length of minutes is counted whole", "5", "0.0625", "1:30:00", true},
		{"midnight is a time", "0", "0", "00:00", true},
		{"a time is rounded to the second the sheet shows", "1", "0.24999999", "06:00:00", true},
		// The rest of the sheet is read the way it was.
		{"a date with no time on it is a date", "6", "45678.5625", "2025-01-21", true},
		{"an hour written in quotation marks is a number", "7", "7.5", "7.5", false},
		{"a clock in red is still a clock", "8", "0.5625", "13:30", true},
		{"a built-in time", "9", "0.5625", "13:30", true},
		{"a built-in time with seconds", "10", "0.5625", "13:30:00", true},
		{"a built-in date-time", "11", "45678.5625", "2025-01-21 13:30", true},
		{"a built-in duration", "12", "1.5", "36:00:00", true},
		{"a number with no format is a number", "13", "0.5625", "0.5625", false},
		{"words under a clock format are words", "0", "미정", "미정", false},
		// A time cannot be counted backwards, and nothing past the last day the
		// count reaches is a moment either; both are left as they stand.
		{"a negative time is left as it is", "0", "-0.5", "-0.5", false},
		{"a count past the last day is left as it is", "3", "3000000", "3000000", false},
	} {
		got, changed := formats.written(one.style, one.stored)
		if got != one.want || changed != one.changed {
			t.Errorf("%s: written(%q, %q) = (%q, %v), want (%q, %v)",
				one.what, one.style, one.stored, got, changed, one.want, one.changed)
		}
	}
}

// kindOfFormat on its own, because what a format code is read as is where the
// rule above starts.
func TestWhatAFormatCodeIsRead(t *testing.T) {
	for _, one := range []struct {
		id, code string
		want     numberKind
	}{
		{"0", "h:mm", numberKind{what: "time"}},
		{"0", "h:mm:ss", numberKind{what: "time", seconds: true}},
		{"0", "yyyy-mm-dd hh:mm", numberKind{what: "datetime"}},
		{"0", "yyyy-mm-dd", numberKind{what: "date"}},
		{"0", "[h]:mm:ss", numberKind{what: "elapsed", seconds: true}},
		{"0", "[mm]:ss", numberKind{what: "elapsed", seconds: true}},
		// A minute is written with the letter a month is, and on its own it
		// says neither: "mmm" is a month and stays a plain number here, as it
		// was before there were times at all.
		{"0", "mmm", numberKind{}},
		// A colour is not a unit, so an amount in red is not a length of time.
		{"0", "[Red]#,##0", numberKind{}},
		{"0", "0.0%", numberKind{what: "percent"}},
		{"0", `0.00"h"`, numberKind{}},
		{"20", "", numberKind{what: "time"}},
		{"22", "", numberKind{what: "datetime"}},
		{"45", "", numberKind{what: "elapsed", seconds: true}},
	} {
		if got := kindOfFormat(one.id, one.code); got != one.want {
			t.Errorf("kindOfFormat(%q, %q) = %+v, want %+v", one.id, one.code, got, one.want)
		}
	}
}

func TestOnlyAUnitStandsInBracketsAsALengthOfTime(t *testing.T) {
	for code, want := range map[string]bool{
		"[h]:mm:ss":     true,
		"[hh]:mm":       true,
		"[mm]:ss":       true,
		"[s]":           true,
		"h:mm":          false,
		"[Red]h:mm":     false,
		"[$-409]h:mm":   false,
		"[>=1000]#,##0": false,
		`"[h]"h:mm`:     false, // in quotation marks it is text, not a unit
		"[h:mm":         false, // a bracket nobody closed is not a unit
	} {
		if got := elapsedUnit(code); got != want {
			t.Errorf("elapsedUnit(%q) = %v, want %v", code, got, want)
		}
	}
}

// And the rule has to be reached: an agenda read whole says the clock.
func TestASpreadsheetShowsItsTimes(t *testing.T) {
	document, err := Read("점검표.xlsx", kindBook(t, timeStyles,
		`<row r="1"><c r="A1" t="inlineStr"><is><t>순서</t></is></c><c r="B1" t="inlineStr"><is><t>시각</t></is></c>`+
			`<c r="C1" t="inlineStr"><is><t>소요</t></is></c></row>`+
			`<row r="2"><c r="A2" t="inlineStr"><is><t>개회</t></is></c><c r="B2" s="0"><v>0.5625</v></c>`+
			`<c r="C2" s="4"><v>0.0208333333</v></c></row>`+
			`<row r="3"><c r="A3" t="inlineStr"><is><t>보고</t></is></c><c r="B3" s="0"><v>0.6041666667</v></c>`+
			`<c r="C3" s="4"><v>1.5</v></c></row>`))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, line := range []string{"- 개회 | 13:30 | 0:30:00", "- 보고 | 14:30 | 36:00:00"} {
		if !strings.Contains(document.Source, line) {
			t.Errorf("the deck is missing %q:\n%s", line, document.Source)
		}
	}
	for _, stored := range []string{"0.5625", "0.0208", "| 1.5"} {
		if strings.Contains(document.Source, stored) {
			t.Errorf("the deck still says what the cell stored (%q) rather than the clock:\n%s",
				stored, document.Source)
		}
	}
}
