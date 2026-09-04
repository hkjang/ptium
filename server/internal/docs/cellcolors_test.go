package docs

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// What a format code says in square brackets is how to print the number, not
// what kind of number it is: a colour, a condition, a locale, a currency sign.
//
// Read as part of the number's shape, "[Red]" has a "d" in it and "#,##0;
// [Red]-#,##0" — what the Currency dialog writes when negatives are shown in
// red — made every amount on the sheet a day in 1903.
func TestABracketInAFormatSaysHowToPrintNotWhatToRead(t *testing.T) {
	formats := readCellFormats([]byte(`<styleSheet><numFmts>
		<numFmt numFmtId="164" formatCode="#,##0;[Red]-#,##0"/>
		<numFmt numFmtId="165" formatCode="&quot;₩&quot;#,##0.00_);[Red](&quot;₩&quot;#,##0.00)"/>
		<numFmt numFmtId="166" formatCode="[&gt;=1000]#,##0,&quot;천&quot;;#,##0"/>
		<numFmt numFmtId="167" formatCode="[$-409]mmm\-yy"/>
		<numFmt numFmtId="168" formatCode="[$-ko-KR]yyyy&quot;년&quot; m&quot;월&quot; d&quot;일&quot;"/>
		<numFmt numFmtId="169" formatCode="[Red]0.0%"/>
		<numFmt numFmtId="170" formatCode="[&quot;일&quot;#,##0"/>
	</numFmts><cellXfs>
		<xf numFmtId="164"/><xf numFmtId="165"/><xf numFmtId="166"/><xf numFmtId="167"/>
		<xf numFmtId="168"/><xf numFmtId="169"/><xf numFmtId="170"/>
	</cellXfs></styleSheet>`))
	for _, one := range []struct {
		what          string
		style, stored string
		want          string
		changed       bool
	}{
		{"negatives in red are still money", "0", "1200", "1200", false},
		{"an amount in red brackets is still money", "1", "1200", "1200", false},
		{"a condition is not a day", "2", "1200", "1200", false},
		{"a locale in front of a date is still a date", "3", "45678", "2025-01-21", true},
		{"a Korean date keeps its date", "4", "45678", "2025-01-21", true},
		{"a per cent in red is still a per cent", "5", "0.325", "32.5%", true},
		// A bracket nobody closed is read the way the rest of the code is.
		{"an unclosed bracket is left as it is", "6", "1200", "1200", false},
	} {
		got, changed := formats.written(one.style, one.stored)
		if got != one.want || changed != one.changed {
			t.Errorf("%s: written(%q, %q) = (%q, %v), want (%q, %v)",
				one.what, one.style, one.stored, got, changed, one.want, one.changed)
		}
	}
}

// spokenPartsOf on its own, because the rule above rests on it.
func TestOnlyTheNumbersOwnPartsAreSpoken(t *testing.T) {
	for code, want := range map[string]string{
		`#,##0;[Red]-#,##0`:   `#,##0;-#,##0`,
		`[$-409]mmm\-yy`:      `mmmyy`,
		`[Red][>=100]0.00`:    `0.00`,
		`0"%"`:                `0`,
		`[h]:mm:ss`:           `:mm:ss`,
		`_-[$₩-412]* #,##0_-`: `_-* #,##0_-`,
		`[Red`:                `[Red`,
		``:                    ``,
	} {
		if got := spokenPartsOf(code); got != want {
			t.Errorf("spokenPartsOf(%q) = %q, want %q", code, got, want)
		}
	}
}

// And the rule has to be reached: a sheet of money read whole stays money, and
// is drawn as the chart two columns of figures are.
func TestASheetOfMoneyInRedIsStillMoney(t *testing.T) {
	document, err := Read("매출.xlsx", moneySpreadsheet(t))
	if err != nil {
		t.Fatalf("reading the spreadsheet: %v", err)
	}
	for _, want := range []string{"1200", "-350"} {
		if !strings.Contains(document.Source, want) {
			t.Errorf("the deck does not say %q:\n%s", want, document.Source)
		}
	}
	if strings.Contains(document.Source, "1903-") {
		t.Errorf("an amount was read as a day:\n%s", document.Source)
	}
	if !strings.Contains(document.Source, "::columns") {
		t.Errorf("two columns of figures were not drawn as a chart:\n%s", document.Source)
	}
}

// moneySpreadsheet writes a workbook whose amounts are formatted the way the
// Currency dialog formats them when negatives are shown in red.
func moneySpreadsheet(t *testing.T) []byte {
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
		`<sheets><sheet name="매출" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`)
	add("xl/styles.xml", `<?xml version="1.0"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`+
		`<numFmts><numFmt numFmtId="164" formatCode="#,##0;[Red]-#,##0"/></numFmts>`+
		`<cellXfs><xf numFmtId="0"/><xf numFmtId="164"/></cellXfs></styleSheet>`)
	add("xl/worksheets/sheet1.xml", `<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`+
		`<row r="1"><c r="A1" t="inlineStr"><is><t>항목</t></is></c><c r="B1" t="inlineStr"><is><t>금액</t></is></c></row>`+
		`<row r="2"><c r="A2" t="inlineStr"><is><t>국내</t></is></c><c r="B2" s="1"><v>1200</v></c></row>`+
		`<row r="3"><c r="A3" t="inlineStr"><is><t>해외</t></is></c><c r="B3" s="1"><v>-350</v></c></row>`+
		`</sheetData></worksheet>`)
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
