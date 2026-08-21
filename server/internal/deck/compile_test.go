package deck

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/pptx"
)

// A model asked for a trend writes the values it has, and they are often not
// numbers: "Q3 | 1시간". Plotting is impossible, but the rows are still labelled
// figures — dropping them into prose loses the slide's design.
// A live model asked for a column chart of phrases — "현재 조직 | 개발/운영 분리" —
// and the slide drew an empty rectangle where the argument should have been. A
// chart is drawn from numbers; words go in something that holds words.
func TestAChartOfWordsBecomesATable(t *testing.T) {
	manifest := testManifest()
	source := "# 조직 개편\n::columns 전후 비교\n- 현재 조직 | 개발/운영 분리\n- 현재 대응 | 수동 장애 대응\n- 현재 문제 | 대응 지연 및 오류\n::\n"
	compiled := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	content := Decode(compiled.Slides[0].Content)
	var drawn pptx.Block
	for _, block := range content.Blocks {
		drawn = block
	}
	if drawn.Kind == pptx.BlockColumns {
		t.Fatalf("a chart with nothing to plot was kept as a chart: %+v", drawn)
	}
	if drawn.Kind != pptx.BlockTable {
		t.Fatalf("the words were drawn as %q; a table holds them", drawn.Kind)
	}
	if len(drawn.Rows) != 3 {
		t.Fatalf("the table lost rows: %+v", drawn.Rows)
	}
	if drawn.Rows[0][0] != "현재 조직" || drawn.Rows[0][1] != "개발/운영 분리" {
		t.Fatalf("the table's first row reads %v", drawn.Rows[0])
	}
	// And the deck says what it did.
	said := false
	for _, warning := range compiled.Warnings {
		if strings.Contains(warning, "numeric") {
			said = true
		}
	}
	if !said {
		t.Fatalf("nothing explained the change: %v", compiled.Warnings)
	}
}

func TestCompileDrawsAnUnplottableChartAsFigures(t *testing.T) {
	manifest := testManifest()
	source := ParseSource("# 분기별 처리 속도\n@content\n> 지표 예상치입니다.\n::line 처리 속도\n" +
		"- Q3 | 1시간\n- Q4 | 15분\n- 2027Q1 | 5분\n::\n")
	compiled := Compile(source, manifest, CompileOptions{Language: "ko"})
	if len(compiled.Slides) != 1 {
		t.Fatalf("slides = %d", len(compiled.Slides))
	}
	content := Decode(compiled.Slides[0].Content)
	if len(content.Blocks) != 1 {
		t.Fatalf("the rows should still be drawn: %+v", content)
	}
	for slot, block := range content.Blocks {
		if block.Kind != pptx.BlockKPI {
			t.Fatalf("%s = %+v", slot, block)
		}
		if len(block.Items) != 3 || block.Items[0].Value != "1시간" {
			t.Fatalf("items = %+v", block.Items)
		}
	}
	// The downgrade is reported rather than silent.
	found := false
	for _, warning := range compiled.Warnings {
		if strings.Contains(warning, "numeric") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the downgrade should be reported: %v", compiled.Warnings)
	}
}
