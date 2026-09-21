package docs

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/pptx"
)

func TestSheetRowReferencesReachTheDeck(t *testing.T) {
	for _, table := range []bool{false, true} {
		for _, test := range []struct {
			name       string
			references []string
			cells      []int
			last       int
		}{
			{"omitted rows", []string{"1", "5", "9"}, []int{1, 5, 9}, 9},
			{"consecutive rows", []string{"1", "2", "3"}, []int{1, 2, 3}, 3},
			{"missing references", []string{"", "", ""}, []int{1, 5, 9}, 3},
			{"trimmed reference", []string{"1", "5", " 9 "}, []int{1, 5, 9}, 9},
			{"last worksheet row", []string{"1", "5", "1048576"}, []int{1, 5, 1048576}, 1048576},
			{"zero", []string{"1", "5", "0"}, []int{1, 5, 9}, 3},
			{"negative", []string{"1", "5", "-9"}, []int{1, 5, 9}, 3},
			{"text", []string{"1", "5", "nine"}, []int{1, 5, 9}, 3},
			{"overflow", []string{"1", "5", "999999999999999999999999999"}, []int{1, 5, 9}, 3},
			{"past worksheet edge", []string{"1", "5", "1048577"}, []int{1, 5, 9}, 3},
		} {
			t.Run(fmt.Sprintf("%s/table=%v", test.name, table), func(t *testing.T) {
				var rows strings.Builder
				want := [][]string{{"분기", "매출", "비고"}, {"1분기", "1180", "확정"}, {"2분기", "1240", "작업"}}
				for i, reference := range test.references {
					rows.WriteString(`<row`)
					if reference != "" {
						fmt.Fprintf(&rows, ` r="%s"`, reference)
					}
					rows.WriteString(`>`)
					rows.WriteString(textCell(fmt.Sprintf("A%d", test.cells[i]), want[i][0]))
					if i == 0 {
						rows.WriteString(textCell("B1", want[i][1]))
					} else {
						rows.WriteString(numberCell(fmt.Sprintf("B%d", test.cells[i]), want[i][1]))
					}
					if table {
						rows.WriteString(textCell(fmt.Sprintf("C%d", test.cells[i]), want[i][2]))
					}
					rows.WriteString(`</row>`)
				}
				document, err := Read("실적.xlsx", concealedBook(t, "", rows.String()))
				if err != nil {
					t.Fatal(err)
				}
				if len(document.Warnings) != 0 {
					t.Errorf("warnings: %v", document.Warnings)
				}
				parsed := deck.ParseSource(document.Source)
				if len(parsed.Slides) != 2 {
					t.Fatalf("slides = %d, want cover and data", len(parsed.Slides))
				}
				column := "B"
				if table {
					column = "C"
				}
				checkRowSlide(t, parsed.Slides[1], fmt.Sprintf("분기 실적!A1:%s%d", column, test.last), want, table)
			})
		}
	}
}

func TestSparseHiddenRowsKeepTheirCoordinatesOnEverySlide(t *testing.T) {
	for _, table := range []bool{false, true} {
		for _, count := range []int{1, 17} {
			t.Run(fmt.Sprintf("table=%v/rows=%d", table, count), func(t *testing.T) {
				cols := `<cols><col min="2" max="2" hidden="1"/></cols>`
				rows := `<row r="1">` + textCell("A1", "분기") + textCell("B1", "코드") + textCell("C1", "매출")
				header := []string{"분기", "매출"}
				if table {
					rows += textCell("D1", "비고")
					header = append(header, "비고")
				}
				rows += `</row><row r="5" hidden="1">` + textCell("A5", "숨김") + numberCell("C5", "999") + `</row><row r="7"/>`
				var body [][]string
				for i := 0; i < count; i++ {
					r := 9 + i*4
					label, value := fmt.Sprintf("분기%d", i+1), fmt.Sprint(100+i)
					rows += fmt.Sprintf(`<row r="%d">`, r) + textCell(fmt.Sprintf("A%d", r), label) + numberCell(fmt.Sprintf("C%d", r), value)
					row := []string{label, value}
					if table {
						rows += textCell(fmt.Sprintf("D%d", r), "확정")
						row = append(row, "확정")
					}
					rows += `</row>`
					body = append(body, row)
				}
				// Trailing empty and hidden rows must not extend the citation or add a warning count.
				rows += `<row r="1000"/><row r="1001" hidden="1"/>`
				document, err := Read("실적.xlsx", concealedBook(t, cols, rows))
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(document.Warnings, []string{"분기 실적의 숨긴 행 1개와 열 1개는 가져오지 않았습니다"}) {
					t.Errorf("warnings = %v", document.Warnings)
				}
				parsed := deck.ParseSource(document.Source)
				if len(parsed.Slides) != 1+(count+7)/8 {
					t.Fatalf("slides = %d", len(parsed.Slides))
				}
				column := "C"
				if table {
					column = "D"
				}
				for i, slide := range parsed.Slides[1:] {
					end := min((i+1)*8, count)
					want := append([][]string{header}, body[i*8:end]...)
					checkRowSlide(t, slide, fmt.Sprintf("분기 실적!A1:%s%d", column, 9+(end-1)*4), want, table)
				}
			})
		}
	}
}

// Inspect the public ZIP reader's parsed citations and data together, so a
// coordinate fix cannot silently change labels, values, order or pagination.
func checkRowSlide(t *testing.T, slide deck.SourceSlide, locator string, want [][]string, table bool) {
	t.Helper()
	if len(slide.Sources) != 1 || slide.Sources[0].Locator != locator {
		t.Errorf("sources = %+v, want %s", slide.Sources, locator)
	}
	if len(slide.Blocks) != 1 {
		t.Fatalf("blocks = %+v", slide.Blocks)
	}
	block := slide.Blocks[0]
	if table {
		if block.Kind != pptx.BlockTable || !reflect.DeepEqual(block.Rows, want) {
			t.Errorf("table = %+v, want %#v", block, want)
		}
		return
	}
	if block.Kind != pptx.BlockColumns || len(block.Items) != len(want)-1 {
		t.Fatalf("chart = %+v", block)
	}
	for i, item := range block.Items {
		if item.Label != want[i+1][0] || item.Number == nil || fmt.Sprint(*item.Number) != want[i+1][1] {
			t.Errorf("item %d = %+v, want %v", i, item, want[i+1])
		}
	}
}
