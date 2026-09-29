package docs

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
)

// The name can already say that this is a continuation before it is uploaded.
// Read the file and then the generated source, so both the file-name fallback
// and the workbook's own sheet name reach the titles a deck actually uses.
func TestASheetNamesItsContinuationOnlyOnce(t *testing.T) {
	for _, extension := range []string{"csv", "tsv", "xlsx"} {
		for _, heading := range []string{"분기 실적", "분기 실적 (계속)"} {
			t.Run(extension+"/"+heading, func(t *testing.T) {
				separator := ","
				if extension == "tsv" {
					separator = "\t"
				}
				text := "지역" + separator + "상태\n"
				rows := `<row r="1">` + textCell("A1", "지역") + textCell("B1", "상태") + `</row>`
				for index := 1; index <= 20; index++ {
					label := fmt.Sprintf("지역%d", index)
					text += label + separator + "확정\n"
					rows += fmt.Sprintf(`<row r="%d">`, index+1) +
						textCell(fmt.Sprintf("A%d", index+1), label) +
						textCell(fmt.Sprintf("B%d", index+1), "확정") + `</row>`
				}
				filename, data := heading+"."+extension, []byte(text)
				if extension == "xlsx" {
					filename = "보고서.xlsx"
					data = packWorkbook([][2]string{
						{"xl/workbook.xml", `<workbook><sheets><sheet name="` + heading + `" id="rId1"/></sheets></workbook>`},
						{"xl/_rels/workbook.xml.rels", `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`},
						{"xl/worksheets/sheet1.xml", `<worksheet><sheetData>` + rows + `</sheetData></worksheet>`},
					})
				}
				document, err := Read(filename, data)
				if err != nil {
					t.Fatal(err)
				}
				slides := deck.ParseSource(document.Source).Slides
				if len(slides) != 4 {
					t.Fatalf("got %d slides, want a cover and three table slides", len(slides))
				}
				var titles []string
				for _, slide := range slides[1:] {
					titles = append(titles, slide.Title)
				}
				want := []string{heading, "분기 실적 (계속)", "분기 실적 (계속)"}
				if !reflect.DeepEqual(titles, want) {
					t.Errorf("titles = %q, want %q", titles, want)
				}
				// Naming a continuation must not rename the place it came from.
				if extension == "xlsx" {
					for _, slide := range slides[1:] {
						if len(slide.Sources) != 1 || !strings.HasPrefix(slide.Sources[0].Locator, heading+"!") {
							t.Errorf("lost the sheet's own name in its citation: %+v", slide.Sources)
						}
					}
				}
			})
		}
	}
}
