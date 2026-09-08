package docs

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// A workbook says which of its sheets are hidden, and the ones that are hidden
// are the ones nobody meant to show: the code table a formula looks up in, the
// settings a macro reads, last year's working copy. Read as if they were
// visible, they became slides in the middle of the deck.

// hiddenSheet is one sheet of a workbook: its name, its state, and its rows.
type hiddenSheet struct {
	name  string
	state string
	rows  string
}

// A small sheet, enough to be a slide of its own.
const twoQuarters = `<row r="1"><c r="A1" t="inlineStr"><is><t>분기</t></is></c><c r="B1" t="inlineStr"><is><t>매출</t></is></c></row>` +
	`<row r="2"><c r="A2" t="inlineStr"><is><t>1분기</t></is></c><c r="B2"><v>1180</v></c></row>` +
	`<row r="3"><c r="A3" t="inlineStr"><is><t>2분기</t></is></c><c r="B3"><v>1240</v></c></row>`

// hiddenBook writes a workbook of several sheets, each with its own state.
func hiddenBook(t *testing.T, sheets []hiddenSheet) []byte {
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
	var index, relationships strings.Builder
	for number, sheet := range sheets {
		state := ""
		if sheet.state != "" {
			state = ` state="` + sheet.state + `"`
		}
		id := "rId" + string(rune('1'+number))
		part := "worksheets/sheet" + string(rune('1'+number)) + ".xml"
		index.WriteString(`<sheet name="` + sheet.name + `"` + state + ` r:id="` + id + `"/>`)
		relationships.WriteString(`<Relationship Id="` + id + `" Target="` + part + `"/>`)
		add("xl/"+part, `<worksheet><sheetData>`+sheet.rows+`</sheetData></worksheet>`)
	}
	add("xl/workbook.xml", `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`+
		`<sheets>`+index.String()+`</sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<Relationships>`+relationships.String()+`</Relationships>`)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestAHiddenSheetIsNotASlide(t *testing.T) {
	for _, test := range []struct {
		name    string
		sheets  []hiddenSheet
		want    []string
		missing []string
		warning string
	}{
		{
			// The ordinary shape: one sheet to show, one code table behind it.
			name: "a hidden sheet beside a visible one",
			sheets: []hiddenSheet{
				{name: "분기 실적", rows: twoQuarters},
				{name: "코드표", state: "hidden", rows: twoQuarters},
			},
			want:    []string{"# 분기 실적"},
			missing: []string{"# 코드표"},
			warning: "코드표",
		},
		{
			// veryHidden is the one only a macro can put back, so it is further
			// from being meant for a deck, not nearer.
			name: "a very hidden sheet",
			sheets: []hiddenSheet{
				{name: "분기 실적", rows: twoQuarters},
				{name: "설정", state: "veryHidden", rows: twoQuarters},
			},
			want:    []string{"# 분기 실적"},
			missing: []string{"# 설정"},
			warning: "설정",
		},
		{
			// The state a sheet writes when it is not hidden, and the state a
			// sheet writes by writing nothing.
			name: "sheets that are not hidden",
			sheets: []hiddenSheet{
				{name: "분기 실적", state: "visible", rows: twoQuarters},
				{name: "부문별", rows: twoQuarters},
			},
			want: []string{"# 분기 실적", "# 부문별"},
		},
		{
			// The attribute is written by hand as often as by Excel.
			name: "a hidden sheet spelled in capitals",
			sheets: []hiddenSheet{
				{name: "분기 실적", rows: twoQuarters},
				{name: "코드표", state: "Hidden", rows: twoQuarters},
			},
			want:    []string{"# 분기 실적"},
			missing: []string{"# 코드표"},
			warning: "코드표",
		},
		{
			// More than one, and the warning names them all rather than counting.
			name: "two hidden sheets",
			sheets: []hiddenSheet{
				{name: "분기 실적", rows: twoQuarters},
				{name: "코드표", state: "hidden", rows: twoQuarters},
				{name: "설정", state: "veryHidden", rows: twoQuarters},
			},
			want:    []string{"# 분기 실적"},
			missing: []string{"# 코드표", "# 설정"},
			warning: "코드표, 설정",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			document, err := Read("실적.xlsx", hiddenBook(t, test.sheets))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			for _, line := range test.want {
				if !strings.Contains(document.Source, line) {
					t.Errorf("the deck is missing %q:\n%s", line, document.Source)
				}
			}
			for _, line := range test.missing {
				if strings.Contains(document.Source, line) {
					t.Errorf("the deck has %q, which the workbook hides:\n%s", line, document.Source)
				}
			}
			warnings := strings.Join(document.Warnings, "\n")
			if test.warning == "" {
				if warnings != "" {
					t.Errorf("nothing was hidden, but the deck warns: %s", warnings)
				}
				return
			}
			if !strings.Contains(warnings, test.warning) {
				t.Errorf("the warnings do not name %q: %q", test.warning, warnings)
			}
		})
	}
}

func TestAWorkbookHiddenAllTheWayThroughSaysSo(t *testing.T) {
	// "이 통합 문서에는 읽을 표가 없습니다" sends someone back to a file whose sheets
	// are full of figures. What is wrong with it is that they are hidden.
	_, err := Read("실적.xlsx", hiddenBook(t, []hiddenSheet{
		{name: "코드표", state: "hidden", rows: twoQuarters},
		{name: "설정", state: "veryHidden", rows: twoQuarters},
	}))
	if err == nil {
		t.Fatal("a workbook of hidden sheets was read as a deck")
	}
	if !strings.Contains(err.Error(), "숨겨") {
		t.Errorf("the message does not say the sheets are hidden: %v", err)
	}
}

func TestASheetIsHiddenBySaying(t *testing.T) {
	for _, test := range []struct {
		state string
		want  bool
	}{
		{"", false},
		{"visible", false},
		{"hidden", true},
		{"veryHidden", true},
		{"VeryHidden", true},
		{" hidden ", true},
		{"HIDDEN", true},
		{"shown", false},
	} {
		if got := sheetHidden(test.state); got != test.want {
			t.Errorf("sheetHidden(%q) = %v, want %v", test.state, got, test.want)
		}
	}
}
