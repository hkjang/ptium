package pptx

import (
	"fmt"
	"strings"
	"testing"
)

// A table's rows are a fixed height — that rhythm is what makes it read as a
// table — and the cells were written at the body size whatever they held. A
// real deck's table of platforms and what each is used for wrapped one line
// past its row and was drawn over the row underneath; the measurement called it
// what it was: two lines of the table overlap.
func TestATableCellIsWrittenToFitItsRow(t *testing.T) {
	_, design, _ := testDesign(t, "plum-rail")
	frame := Frame{X: 914400, Y: 914400, Width: 4 * 914400, Height: 2 * 914400}
	block := Block{Kind: BlockTable,
		Columns: []string{"플랫폼", "활용 방안", "비고"},
		Rows: [][]string{
			{"E-GENE", "LCNC 이용한 AI 서비스 개발", "엔티티 / 폼 / 리스트 / 릴레이션 등 개발"},
			{"CHEETAH", "AI 모델 Proxy", "GPU A100"},
		}}
	_, rowHeight := design.tableRhythm(frame, len(block.Rows))
	size, cells := design.tableCells(frame, block.Columns, block.Rows, rowHeight)
	if size >= design.Body {
		t.Errorf("a table whose cells do not fit was written at the full body size (%d)", size)
	}
	room := rowHeight - design.Unit/2
	for index, column := range frame.Columns(len(block.Columns), design.Unit) {
		for _, row := range cells {
			if lines := cellLines(row[index], size, column.Width); lines*lineHeightFor(size) > room {
				t.Errorf("cell %q still draws %d lines in room for %d", row[index], lines,
					room/lineHeightFor(size))
			}
		}
	}
}

// Some cells cannot be made to fit at any size a person can read: a week's work
// listed in one box. The rest of the product answers that by cutting the line
// and marking it, which the reader can see. Drawing it over the row underneath
// is the one answer that hides it.
func TestACellNoSizeCanHoldIsCutAndMarked(t *testing.T) {
	_, design, _ := testDesign(t, "plum-rail")
	frame := Frame{X: 914400, Y: 914400, Width: 3 * 914400, Height: 2 * 914400}
	long := "ITSM 운영 및 개선 서버 자원 신규 신청서 개선 완료 ERP 영업지원 통합 관리 체계 구축 진행 " +
		"부서보안담당자 신청서 결재라인 변경 완료 ALM 계정 관리 개선 진행 SBOM 체계 구축 연동 분석 설계"
	columns := []string{"구분", "내용"}
	rows := [][]string{{"1주차", long}}
	_, rowHeight := design.tableRhythm(frame, len(rows))
	size, cells := design.tableCells(frame, columns, rows, rowHeight)
	drawn := cells[0][1]
	if !strings.HasSuffix(drawn, "…") {
		t.Errorf("a cell no size can hold was drawn whole: %q", drawn)
	}
	room := rowHeight - design.Unit/2
	width := frame.Columns(len(columns), design.Unit)[1].Width
	if lines := cellLines(drawn, size, width); lines*lineHeightFor(size) > room {
		t.Errorf("the cut cell still draws %d lines past its row", lines)
	}
	if !strings.HasPrefix(drawn, "ITSM 운영") {
		t.Errorf("the cut cell no longer starts with what it said: %q", drawn)
	}
}

// A table stops at the bottom of its region and caps its columns. Whatever is
// past that is on no slide, and until now nothing said so: a twelve-row table
// drew eight rows and the quality panel called the deck clean.
func TestATableSaysWhatItCouldNotDraw(t *testing.T) {
	_, design, layout := testDesign(t, "plum-rail")
	frame := bodyFrame(layout)
	placeholder, _ := layout.Slot(SlotBody)
	rows := make([][]string, 0, 12)
	for index := 1; index <= 12; index++ {
		rows = append(rows, []string{fmt.Sprintf("항목%d", index), fmt.Sprintf("값%d", index)})
	}
	block := Block{Kind: BlockTable, Columns: []string{"구분", "값"}, Rows: rows}
	said := ""
	for _, finding := range inspectComponent(placeholder, frame, block, design, 12192000, 6858000) {
		if finding.Kind == FindingTrimmed {
			said = finding.Detail
		}
	}
	if !strings.Contains(said, "8 of its 12 rows") {
		t.Errorf("a twelve-row table reported %q", said)
	}
	// What the file carries is what the preview draws.
	component := RenderBlock(design, frame, block)
	if component.Table == nil || len(component.Table.Rows) != component.RowsDrawn {
		t.Errorf("the exported table holds %d rows of the %d drawn",
			len(component.Table.Rows), component.RowsDrawn)
	}
	// A table that fits says nothing.
	small := Block{Kind: BlockTable, Columns: []string{"항목", "2026"}, Rows: [][]string{{"인건비", "4.2"}}}
	for _, finding := range inspectComponent(placeholder, frame, small, design, 12192000, 6858000) {
		if finding.Kind == FindingTrimmed {
			t.Errorf("a table that fits was reported as trimmed: %q", finding.Detail)
		}
	}
}

// A table heading is one line and is not wrapped, so a long one is painted
// across the table and off the slide. A report's table whose header cell held a
// paragraph drew two metres past the region it was given.
func TestATableHeadingIsDrawnInsideItsColumn(t *testing.T) {
	_, design, layout := testDesign(t, "plum-rail")
	frame := bodyFrame(layout)
	placeholder, _ := layout.Slot(SlotBody)
	paragraph := "프로젝트 상세 내용 - OpenAI 호환 프록시 제공과 스트리밍 중계, 사용자별 토큰과 비용 집계, " +
		"민감정보 마스킹, 라우팅 정책과 감사 로그까지 한 곳에서 다룹니다"
	block := Block{Kind: BlockTable, Columns: []string{paragraph, "비고"},
		Rows: [][]string{{"주요 기능", "SSE 중계"}}}
	for _, finding := range inspectComponent(placeholder, frame, block, design, 12192000, 6858000) {
		if finding.Kind == FindingOverflow {
			t.Errorf("a long heading %s", finding.Detail)
		}
	}
	// An ordinary heading is left as it was written.
	plain := Block{Kind: BlockTable, Columns: []string{"항목", "2026"}, Rows: [][]string{{"인건비", "4.2"}}}
	component := RenderBlock(design, frame, plain)
	drawn := ""
	for _, primitive := range component.Primitives {
		if primitive.Kind == shapeText && len(primitive.Lines) > 0 && primitive.Lines[0].Text == "항목" {
			drawn = primitive.Lines[0].Text
		}
	}
	if drawn != "항목" {
		t.Errorf("an ordinary heading was drawn as %q", drawn)
	}
}

// A figure gives way to its card down to the size of its label and no further.
// What will still not fit used to be painted across the tile beside it: a brief
// that answers "인력" with a sentence — "12 staff redeployed" — is not a figure,
// and drawing it whole put it through the neighbouring number.
func TestAKPIFigureIsCutRatherThanPaintedAcrossTheNextTile(t *testing.T) {
	_, design, layout := testDesign(t, "plum-rail")
	// Half a region wide: three tiles in the room a comparison slide gives its
	// figures, which is where a live model put a sentence in a figure's place.
	wide := bodyFrame(layout)
	frame := Frame{X: wide.X, Y: wide.Y, Width: wide.Width / 2, Height: wide.Height / 2}
	placeholder, _ := layout.Slot(SlotBody)
	block := Block{Kind: BlockKPI, Items: []Item{
		{Label: "Throughput", Value: "+34%"},
		{Label: "Error rate", Value: "0.3%"},
		{Label: "Labor model", Value: "12 staff redeployed"},
	}}
	for _, finding := range inspectComponent(placeholder, frame, block, design, 12192000, 6858000) {
		if finding.Kind == FindingOverflow {
			t.Errorf("a sentence in a figure's place %s", finding.Detail)
		}
	}
	// And it says what it cut.
	cut := ""
	for _, finding := range inspectComponent(placeholder, frame, block, design, 12192000, 6858000) {
		if finding.Kind == FindingTrimmed {
			cut = finding.Detail
		}
	}
	if !strings.Contains(cut, "cut") {
		t.Errorf("a cut figure was not reported: %q", cut)
	}
	// An ordinary row of figures is left alone.
	plain := Block{Kind: BlockKPI, Items: []Item{
		{Label: "전환 대상", Value: "42개"}, {Label: "절감", Value: "18억"},
	}}
	for _, finding := range inspectComponent(placeholder, frame, plain, design, 12192000, 6858000) {
		if finding.Kind == FindingTrimmed || finding.Kind == FindingOverflow {
			t.Errorf("an ordinary figure row was reported: %s %s", finding.Kind, finding.Detail)
		}
	}
}

// A table cell was written as one piece of text, so a word the author marked
// bold and an address the author wrote as a link were drawn as the characters
// they are typed with: the exported file's header read **1분기** and
// [근거](https://…) while the preview drew the words alone. The author checks
// the preview, sees a clean table, and sends a file with the markup on the wall.
func TestATableCellDrawsItsMarksRatherThanPrintingThem(t *testing.T) {
	_, design, _ := testDesign(t, "plum-rail")
	frame := Frame{X: 914400, Y: 914400, Width: 6 * 914400, Height: 3 * 914400}
	block := Block{Kind: BlockTable,
		Columns: []string{"구분", "**1분기**", "[근거](https://example.invalid/e)"},
		Rows: [][]string{
			{"인프라", "1억 2천", "[계약서](https://example.invalid/contract)"},
			{"인건비", "**3억**", "정상"},
		}}
	part := design.tablePart(frame, block, 0)
	if part == nil {
		t.Fatal("the table drew nothing")
	}
	links := &linkTable{}
	markup := part.drawingML(7, "", links, "ko-KR")
	for _, mark := range []string{"**", "](", "https://example.invalid"} {
		if strings.Contains(markup, "<a:t>"+mark) || strings.Contains(markup, mark+"</a:t>") {
			t.Errorf("the exported table prints %q where a reader can see it", mark)
		}
	}
	if strings.Count(markup, "hlinkClick") != 2 {
		t.Errorf("the two addresses written in cells became %d links, want 2",
			strings.Count(markup, "hlinkClick"))
	}
	for _, word := range []string{"1분기", "근거", "계약서", "3억", "인프라"} {
		if !strings.Contains(markup, "<a:t>"+word+"</a:t>") {
			t.Errorf("the cell %q is not in the exported table", word)
		}
	}
	// What is read aloud is the words, never the marks.
	if said := describeTable(part, block, "ko"); strings.Contains(said, "**") || strings.Contains(said, "](") {
		t.Errorf("the table is read aloud as %q", said)
	}
}

// A cell the design had to cut keeps only its words: half a link is not a link,
// and a cut that lands inside the address would export characters no reader can
// follow.
func TestACutTableCellKeepsItsWordsAndNotHalfALink(t *testing.T) {
	_, design, _ := testDesign(t, "plum-rail")
	frame := Frame{X: 914400, Y: 914400, Width: 2 * 914400, Height: 914400}
	long := "[" + strings.Repeat("근거가 되는 아주 긴 문서 이름 ", 12) + "](https://example.invalid/very/long)"
	block := Block{Kind: BlockTable, Columns: []string{"구분", "비고"},
		Rows: [][]string{{"인프라", long}, {"인건비", "정상"}}}
	part := design.tablePart(frame, block, 0)
	if part == nil {
		t.Fatal("the table drew nothing")
	}
	markup := part.drawingML(7, "", &linkTable{}, "ko-KR")
	if strings.Contains(markup, "https://example.invalid") {
		t.Error("a cut cell exported the address as text on the slide")
	}
}

// A link written in a cell was a live link in the exported .pptx and nothing at
// all in the PDF of the same deck, because the drawing carried the words alone:
// no link colour, no underline, and nothing for the printed page to turn into
// something a reader can click.
func TestATableCellsLinkIsDrawnAsALink(t *testing.T) {
	_, design, _ := testDesign(t, "plum-rail")
	frame := Frame{X: 914400, Y: 914400, Width: 6 * 914400, Height: 3 * 914400}
	block := Block{Kind: BlockTable, Columns: []string{"항목", "근거"},
		Rows: [][]string{
			{"인프라", "[계약서](https://example.invalid/contract)"},
			{"인건비", "정상"},
		}}
	primitives, drawn := design.layoutTable(frame, block)
	if drawn != 2 {
		t.Fatalf("the table drew %d rows, want 2", drawn)
	}
	component := Component{Primitives: primitives}
	svg := component.SVG(1.0/9525, "0563C1", 0)
	if !strings.Contains(svg, `<a href="https://example.invalid/contract"`) {
		t.Error("the cell's link is not a link in the drawing, so the printed page has nothing to click")
	}
	if !strings.Contains(svg, "underline") {
		t.Error("the cell's link is not drawn as one, so a reader cannot see it is a link")
	}
	// The words are drawn, the markup is not.
	if strings.Contains(svg, "](") || strings.Contains(svg, "[계약서]") {
		t.Errorf("the drawing prints the markup: %s", svg)
	}
	if !strings.Contains(svg, "계약서") || !strings.Contains(svg, "정상") {
		t.Error("a cell's own words did not survive")
	}
}
