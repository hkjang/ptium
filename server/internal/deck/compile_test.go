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

// A deck's last slide is usually its ask, and closing layouts are built for
// that: a title and a line under it. A table or a plotted trend on the last
// slide is not an ask — it is the argument still running — and putting it on a
// closing layout flattens the component into stray text under the title.
func TestALastSlideCarryingAComponentIsNotAClosingSlide(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 2026 성장 전략\n@cover\n> 로드맵\n\n# 핵심 진단\n- 성장률 둔화\n\n" +
		"# 월별 처리량\n::line 월별 처리량\n- 월 | 1월, 2월, 3월, 4월\n- 전환 전 | 120, 118, 121, 119\n" +
		"- 전환 후 | 120, 132, 148, 165\n::\n"
	result := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	if len(result.Slides) != 3 {
		t.Fatalf("compiled %d slides", len(result.Slides))
	}
	last := Decode(result.Slides[2].Content)
	if len(last.Blocks) == 0 {
		t.Fatalf("the trend was not drawn as a component: %+v", last)
	}
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "written as text") || strings.Contains(warning, "plain text") {
			t.Fatalf("the last slide lost its component: %s", warning)
		}
	}

	// An author who says the slide is a closing page still gets one.
	said := Compile(ParseSource("# 시작\n@cover\n\n# 본론\n- 한 줄\n\n# 다음 단계\n@closing\n- 승인 요청\n"),
		manifest, CompileOptions{Language: "ko"})
	if len(said.Slides) != 3 {
		t.Fatalf("compiled %d slides", len(said.Slides))
	}
	if said.Slides[2].Layout != pptx.RoleClosing {
		t.Fatalf("the closing slide landed on %q", said.Slides[2].Layout)
	}
}

// "!source 통계청 | 2026 소비 동향 | 표 3" names the office that published the
// figure. A marker is "1" or "*" — a mark the claim carries — and Korean,
// Japanese and Chinese institutions are named in two and three characters, so
// reading the first field as a marker by its length alone threw away the one
// word the audience asked for.
func TestAShortKoreanPublisherIsTheSourceNotAMarker(t *testing.T) {
	parsed := ParseSource("# 핵심 진단\n- 이탈 고객의 62%가 온보딩에서 발생\n" +
		"!source 통계청 | 2026 소비 동향 | 표 3\n")
	if len(parsed.Slides) != 1 || len(parsed.Slides[0].Sources) != 1 {
		t.Fatalf("parsed %+v", parsed.Slides)
	}
	source := parsed.Slides[0].Sources[0]
	if source.Title != "통계청" {
		t.Fatalf("the publisher is not the source's name: %+v", source)
	}
	if source.Marker != "" {
		t.Fatalf("the publisher was read as a marker: %+v", source)
	}
	if source.Locator != "2026 소비 동향, 표 3" {
		t.Fatalf("the rest of the citation reads %q", source.Locator)
	}

	// A real mark is still a mark, in either alphabet.
	for _, mark := range []string{"1", "a", "*", "†", "가"} {
		marked := ParseSource("# 제목\n!source " + mark + " | 통계청 2026 소비 동향\n")
		if len(marked.Slides) != 1 || len(marked.Slides[0].Sources) != 1 {
			t.Fatalf("%q: parsed %+v", mark, marked.Slides)
		}
		if got := marked.Slides[0].Sources[0]; got.Marker != mark || got.Title != "통계청 2026 소비 동향" {
			t.Fatalf("%q was not read as a marker: %+v", mark, got)
		}
	}
}
