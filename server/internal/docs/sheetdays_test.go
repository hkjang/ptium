package docs

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// A workbook says which day it counts its days from, and one written by Excel
// for the Macintosh counts from 1904. Read as if every workbook counted from
// 1900, every date in one came out four years and a day early: a contract dated
// 2025-01-21 arrived in the deck as 2021-01-20, with nothing to say so.

// dayStyles: a date, a date with a time on it, and a per cent, in the order the
// styles are numbered.
const dayStyles = `<styleSheet><cellXfs>` +
	`<xf numFmtId="14"/><xf numFmtId="22"/><xf numFmtId="9"/>` +
	`</cellXfs></styleSheet>`

// dayBook wraps one sheet, its styles and the workbook's own properties in the
// smallest workbook that holds them.
func dayBook(t *testing.T, properties, styles, sheet string) []byte {
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
		properties+`<sheets><sheet name="일정" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`)
	add("xl/styles.xml", styles)
	add("xl/worksheets/sheet1.xml", `<worksheet><sheetData>`+sheet+`</sheetData></worksheet>`)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestADayIsWrittenInTheSystemTheWorkbookCountsBy(t *testing.T) {
	macintosh := readCellFormats([]byte(dayStyles), true)
	for _, one := range []struct {
		what          string
		style, stored string
		want          string
		changed       bool
	}{
		// 44216 is 2025-01-21 counted from 1904, and 2021-01-20 counted from
		// 1900: the same stored number, four years and a day apart.
		{"a date counted from 1904", "0", "44216", "2025-01-21", true},
		{"a date and a time counted from 1904", "1", "44216.5625", "2025-01-21 13:30", true},
		// The count from 1904 has no phantom leap day in it, so it runs plainly
		// from its own first day, which is 0 rather than 1.
		{"the first day of the count", "0", "0", "1904-01-01", true},
		{"the day after it", "0", "1", "1904-01-02", true},
		{"the last day the count reaches", "0", "2957003", "9999-12-31", true},
		{"a day past the last one is left as it is", "0", "2957004", "2957004", false},
		{"a day counted backwards is left as it is", "0", "-1", "-1", false},
		// What is not a day is not counted from anywhere.
		{"a per cent is a per cent either way", "2", "0.68", "68%", true},
		{"words are left alone", "0", "미정", "미정", false},
	} {
		got, changed := macintosh.written(one.style, one.stored)
		if got != one.want || changed != one.changed {
			t.Errorf("%s: written(%q, %q) = (%q, %v), want (%q, %v)",
				one.what, one.style, one.stored, got, changed, one.want, one.changed)
		}
	}
}

// The ordinary workbook is unmoved by any of this.
func TestADayCountedFrom1900IsStillCountedFrom1900(t *testing.T) {
	ordinary := readCellFormats([]byte(dayStyles), false)
	for _, one := range []struct {
		what          string
		style, stored string
		want          string
	}{
		{"a date", "0", "45678", "2025-01-21"},
		{"the same number a 1904 workbook would call 2025-01-21", "0", "44216", "2021-01-20"},
		{"a date and a time", "1", "45678.5625", "2025-01-21 13:30"},
		// The day is taken out of the count after it is rounded to the second, so
		// a moment a hair before midnight is the next day rather than
		// twenty-four o'clock on this one.
		{"a moment a hair before midnight", "1", "45678.9999999", "2025-01-22 00:00"},
		{"the count's own first day", "0", "1", "1900-01-01"},
		{"nothing is day zero", "0", "0", "0"},
	} {
		if got, _ := ordinary.written(one.style, one.stored); got != one.want {
			t.Errorf("%s: written(%q, %q) = %q, want %q",
				one.what, one.style, one.stored, got, one.want)
		}
	}
}

func TestWhenAWorkbookSaysItCountsFrom1904(t *testing.T) {
	for value, want := range map[string]bool{
		"1":     true,
		"true":  true,
		"True":  true,
		" 1 ":   true,
		"0":     false,
		"false": false,
		"":      false, // a workbook that says nothing counts from 1900
	} {
		if got := counts1904(value); got != want {
			t.Errorf("counts1904(%q) = %v, want %v", value, got, want)
		}
	}
}

// And the rule has to be reached: a schedule from a Macintosh workbook, read
// whole, says the days the sheet shows.
func TestASpreadsheetCountedFrom1904ShowsItsDays(t *testing.T) {
	sheet := `<row r="1"><c r="A1" t="inlineStr"><is><t>항목</t></is></c><c r="B1" t="inlineStr"><is><t>납기</t></is></c></row>` +
		`<row r="2"><c r="A2" t="inlineStr"><is><t>계약</t></is></c><c r="B2" s="0"><v>44216</v></c></row>` +
		`<row r="3"><c r="A3" t="inlineStr"><is><t>착수</t></is></c><c r="B3" s="0"><v>44229</v></c></row>`
	document, err := Read("일정.xlsx", dayBook(t, `<workbookPr date1904="1"/>`, dayStyles, sheet))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, line := range []string{"- 계약 | 2025-01-21", "- 착수 | 2025-02-03"} {
		if !strings.Contains(document.Source, line) {
			t.Errorf("the deck is missing %q:\n%s", line, document.Source)
		}
	}
	// The days the same numbers stand for in a workbook counting from 1900.
	for _, early := range []string{"2021-01-20", "2021-02-02"} {
		if strings.Contains(document.Source, early) {
			t.Errorf("the deck is four years early (%q):\n%s", early, document.Source)
		}
	}
	// And a workbook that says nothing about it counts from 1900 as it always
	// did, with the same sheet in it.
	ordinary, err := Read("일정.xlsx", dayBook(t, "", dayStyles, sheet))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(ordinary.Source, "- 계약 | 2021-01-20") {
		t.Errorf("a workbook counting from 1900 did not keep counting from 1900:\n%s", ordinary.Source)
	}
}
