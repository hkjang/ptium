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
