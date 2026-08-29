package docs

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// A spreadsheet keeps a date as a count of days and a share as a fraction, so a
// schedule read as written says "45678" where the sheet on screen says a day,
// and a split says "0.68" where it says 68%. Both are the point of the cell.
func TestANumberIsWrittenTheWayItsFormatMeansIt(t *testing.T) {
	formats := readCellFormats([]byte(`<styleSheet>
		<numFmts><numFmt numFmtId="164" formatCode="0.0%"/><numFmt numFmtId="165" formatCode="yyyy&quot;년&quot; mm&quot;월&quot;"/>
		<numFmt numFmtId="166" formatCode="0&quot;%&quot;"/></numFmts>
		<cellXfs>
			<xf numFmtId="0"/><xf numFmtId="14"/><xf numFmtId="9"/>
			<xf numFmtId="164"/><xf numFmtId="165"/><xf numFmtId="166"/>
		</cellXfs></styleSheet>`))
	for _, one := range []struct {
		what          string
		style, stored string
		want          string
		changed       bool
	}{
		{"a plain number is a number", "0", "128", "128", false},
		{"a built-in date", "1", "45678", "2025-01-21", true},
		{"a built-in per cent", "2", "0.68", "68%", true},
		{"a per cent somebody wrote themselves", "3", "0.325", "32.5%", true},
		{"a date somebody wrote in Korean", "4", "45678", "2025-01-21", true},
		// The quoted text in 0"%" is a per cent sign printed after a number, not
		// a per cent format: 68 stays 68.
		{"a quoted sign is not a format", "5", "68", "68", false},
		{"a cell with no style at all", "", "45678", "45678", false},
		{"words are left alone", "1", "착수", "착수", false},
	} {
		got, changed := formats.written(one.style, one.stored)
		if got != one.want || changed != one.changed {
			t.Errorf("%s: written(%q, %q) = (%q, %v), want (%q, %v)",
				one.what, one.style, one.stored, got, changed, one.want, one.changed)
		}
	}
}

// The day the count runs from.
//
// The format keeps a leap day in 1900 that never happened. Counting from two
// days before 1900-01-01 puts every date from March 1900 onwards where the
// sheet shows it, which is every date anybody imports; the two months before
// that come out a day earlier than Excel draws them, and no reader agrees with
// Excel there without repeating the mistake.
func TestTheDayTheCountRunsFrom(t *testing.T) {
	formats := readCellFormats([]byte(`<styleSheet><cellXfs><xf numFmtId="14"/></cellXfs></styleSheet>`))
	for stored, want := range map[string]string{
		"1": "1899-12-31", // the count's own first day
		// Excel draws day 60 as 1900-02-29, a day that did not exist.
		"60":    "1900-02-28",
		"61":    "1900-03-01", // from here on the two agree
		"45678": "2025-01-21",
		"45900": "2025-08-31",
	} {
		if got, _ := formats.written("0", stored); got != want {
			t.Errorf("day %s came out as %s, want %s", stored, got, want)
		}
	}
}

// And the rule has to be reached. Checking written() on its own passes whether
// or not the reader ever calls it, so the spreadsheet is read whole.
func TestASpreadsheetShowsItsDatesAndPercentages(t *testing.T) {
	document, err := Read("실적.xlsx", spreadsheet(t))
	if err != nil {
		t.Fatalf("reading the spreadsheet: %v", err)
	}
	for _, want := range []string{"2025-01-21", "68%"} {
		if !strings.Contains(document.Source, want) {
			t.Errorf("the deck does not say %q:\n%s", want, document.Source)
		}
	}
	for _, stored := range []string{"45678", "0.68"} {
		if strings.Contains(document.Source, stored) {
			t.Errorf("the deck still says what the cell stored (%q) rather than what it means:\n%s",
				stored, document.Source)
		}
	}
	// A number with no format of its own is a number and stays one.
	if !strings.Contains(document.Source, "128") {
		t.Errorf("a plain number did not survive:\n%s", document.Source)
	}
}

// spreadsheet writes the smallest workbook that carries a date, a percentage
// and a plain number.
func spreadsheet(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	archive := zip.NewWriter(&out)
	add := func(name, body string) {
		part, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	add("[Content_Types].xml", `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`+
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`+
		`<Default Extension="xml" ContentType="application/xml"/></Types>`)
	add("_rels/.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`)
	add("xl/workbook.xml", `<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" `+
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`+
		`<sheets><sheet name="실적" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`)
	add("xl/styles.xml", `<?xml version="1.0"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`+
		`<cellXfs><xf numFmtId="0"/><xf numFmtId="14"/><xf numFmtId="9"/></cellXfs></styleSheet>`)
	add("xl/worksheets/sheet1.xml", `<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`+
		`<row r="1"><c r="A1" t="inlineStr"><is><t>항목</t></is></c><c r="B1" t="inlineStr"><is><t>값</t></is></c></row>`+
		`<row r="2"><c r="A2" t="inlineStr"><is><t>착수</t></is></c><c r="B2" s="1"><v>45678</v></c></row>`+
		`<row r="3"><c r="A3" t="inlineStr"><is><t>국내</t></is></c><c r="B3" s="2"><v>0.68</v></c></row>`+
		`<row r="4"><c r="A4" t="inlineStr"><is><t>매출</t></is></c><c r="B4"><v>128</v></c></row>`+
		`</sheetData></worksheet>`)
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
