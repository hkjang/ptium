package deck

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/pptx"
)

// A model asked for a trend writes the values it has, and they are often not
// numbers: "Q3 | 1시간". Plotting is impossible, but the rows are still labelled
// figures — dropping them into prose loses the slide's design.
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
