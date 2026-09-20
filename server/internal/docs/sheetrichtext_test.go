package docs

import (
	"archive/zip"
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// The same cells can be stored directly, as inline runs, or as shared runs.
// Keep the ZIP and the public reader in the test so all three reach the deck.
func richTextBook(t *testing.T, storage string, table, hidden bool) []byte {
	t.Helper()
	rows := [][][]string{
		{{"지", "역"}, {"국내", " 매출"}, {"비", "고"}},
		{{"  R&", "D  "}, {" 1,", "200 "}, {"  국내", " 매출 & 성장  "}},
		{{"부", "산"}, {"9", "80"}, {"검토", " 완료"}},
	}
	var sheet, shared strings.Builder
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	if hidden {
		sheet.WriteString(`<cols><col min="2" max="2" hidden="1"/></cols>`)
	}
	sheet.WriteString(`<sheetData>`)
	index := 0
	for r, row := range rows {
		fmt.Fprintf(&sheet, `<row r="%d"`, r+1)
		if hidden && r == 1 {
			sheet.WriteString(` hidden="1"`)
		}
		sheet.WriteString(`>`)
		for c, pieces := range row {
			if !table && c == 2 {
				continue
			}
			if hidden && c == 1 {
				fmt.Fprintf(&sheet, `<c r="%c%d" t="inlineStr"><is><t>숨김</t></is></c>`, 'B', r+1)
			}
			col := c
			if hidden && c > 0 {
				col++
			}
			var runs strings.Builder
			for _, piece := range pieces {
				piece = strings.ReplaceAll(piece, "&", "&amp;")
				runs.WriteString(`<r><rPr><b/><color rgb="FFFF0000"/></rPr><t xml:space="preserve">` + piece + `</t></r>`)
			}
			runs.WriteString(`<rPh sb="0" eb="1"><t>발음안내</t></rPh>`)
			switch storage {
			case "plain":
				value := strings.ReplaceAll(strings.Join(pieces, ""), "&", "&amp;")
				fmt.Fprintf(&sheet, `<c r="%c%d" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, 'A'+col, r+1, value)
			case "inline":
				fmt.Fprintf(&sheet, `<c r="%c%d" t="inlineStr"><is>%s</is></c>`, 'A'+col, r+1, runs.String())
			case "shared":
				fmt.Fprintf(&sheet, `<c r="%c%d" t="s"><v>%d</v></c>`, 'A'+col, r+1, index)
				shared.WriteString(`<si>` + runs.String() + `</si>`)
				index++
			}
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range map[string]string{
		"xl/workbook.xml":            `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="분기 실적" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   sheet.String(),
		"xl/sharedStrings.xml":       `<sst>` + shared.String() + `</sst>`,
	} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestRichTextCellsKeepTheirTextThroughTheDeck(t *testing.T) {
	for _, table := range []bool{false, true} {
		for _, hidden := range []bool{false, true} {
			t.Run(fmt.Sprintf("table=%v/hidden=%v", table, hidden), func(t *testing.T) {
				var plain Document
				for _, storage := range []string{"plain", "shared", "inline"} {
					t.Run(storage, func(t *testing.T) {
						document, err := Read("실적.xlsx", richTextBook(t, storage, table, hidden))
						if err != nil {
							t.Fatal(err)
						}
						if storage == "plain" {
							plain = document
						} else if document.Source != plain.Source || !reflect.DeepEqual(document.Warnings, plain.Warnings) {
							t.Errorf("storage changed source or warnings:\n%+v\nwant:\n%+v", document, plain)
						}
						lastColumn := 'B'
						if table {
							lastColumn++
						}
						if hidden {
							lastColumn++
						}
						wantSource := fmt.Sprintf("!source 실적.xlsx | 분기 실적!A1:%c3\n", lastColumn)
						if !strings.Contains(document.Source, wantSource) {
							t.Errorf("missing source %q:\n%s", wantSource, document.Source)
						}
						if hidden {
							if len(document.Warnings) != 1 || !strings.Contains(document.Warnings[0], "숨긴 행 1개와 열 1개") {
								t.Errorf("hidden warnings: %v", document.Warnings)
							}
						} else if len(document.Warnings) != 0 {
							t.Errorf("warnings: %v", document.Warnings)
						}
						for _, absent := range []string{"발음안내", "FFFF0000", "숨김"} {
							if strings.Contains(document.Source, absent) {
								t.Errorf("unwanted text %q:\n%s", absent, document.Source)
							}
						}
						parsed := deck.ParseSource(document.Source)
						if !table {
							if !strings.Contains(document.Source, "::columns 국내 매출\n") {
								t.Errorf("missing chart heading:\n%s", document.Source)
							}
							want := map[string]float64{"부산": 980}
							if !hidden {
								want["R&D"] = 1200
							}
							for label, number := range want {
								item, ok := chartItem(parsed, label)
								if !ok || item.Number == nil || *item.Number != number {
									t.Errorf("chart item %q = %+v, found %v; want %v", label, item, ok, number)
								}
							}
						} else {
							want := [][]string{{"지역", "국내 매출", "비고"}}
							if !hidden {
								want = append(want, []string{"R&D", "1,200", "국내 매출 & 성장"})
							}
							want = append(want, []string{"부산", "980", "검토 완료"})
							var got [][]string
							for _, slide := range parsed.Slides {
								for _, block := range slide.Blocks {
									if block.Kind == pptx.BlockTable {
										got = append(got, block.Rows...)
									}
								}
							}
							if !reflect.DeepEqual(got, want) {
								t.Errorf("table rows = %#v, want %#v", got, want)
							}
						}
					})
				}
			})
		}
	}
}
