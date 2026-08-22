package deck

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/model"
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

// A closing page carries the deck's ask. On a layout with no body region its
// points share the subtitle with the lead — and replacing the field instead of
// adding to it dropped the lead off the one slide that cannot afford to lose it.
func TestAClosingPageKeepsItsLeadAndItsPoints(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 시작\n@cover\n\n# 본론\n- 한 줄\n\n# 다음 단계\n@closing\n> 결정과 실행을 분리해 요청합니다.\n" +
		"- 오늘 요청하는 결정 한 가지\n- 결정 후 30일 안에 진행할 일\n"
	result := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	built := Build(model.Presentation{Title: "확인", Language: "ko", Slides: result.Slides}, manifest, "")
	closing := built.Slides[len(built.Slides)-1]
	said := ""
	for _, paragraphs := range closing.Fields {
		for _, paragraph := range paragraphs {
			said += paragraph.Text + "\n"
		}
	}
	for _, wanted := range []string{"결정과 실행을 분리해 요청합니다.", "오늘 요청하는 결정 한 가지", "결정 후 30일 안에 진행할 일"} {
		if !strings.Contains(said, wanted) {
			t.Fatalf("the closing page lost %q:\n%s", wanted, said)
		}
	}
}

// Two columns are written the way anyone describes two sides of something: a
// heading, its points, another heading, its points. Both headings used to be
// glued into one sentence over the left column, leaving the right one bare.
func TestASlideWrittenAsTwoColumnsGetsTwoColumns(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 성장과 위축의 원인 분석\n@two\n" +
		"> 시장 환경 변화\n- 온라인 구매 선호도가 증가\n- 모바일 비중 확대\n" +
		"> 내부 채널 갈등\n- 가격 정책 불일치\n- 대리점 디지털 역량 부족\n"
	result := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	if len(result.Slides) != 1 {
		t.Fatalf("compiled %d slides", len(result.Slides))
	}
	content := Decode(result.Slides[0].Content)
	regions := map[string][]string{}
	for slot, paragraphs := range content.Fields {
		for _, paragraph := range paragraphs {
			regions[slot] = append(regions[slot], paragraph.Text)
		}
	}
	var left, right []string
	for slot, lines := range regions {
		if slot == pptx.SlotTitle {
			continue
		}
		if strings.Contains(strings.Join(lines, " "), "시장 환경 변화") {
			left = lines
		}
		if strings.Contains(strings.Join(lines, " "), "내부 채널 갈등") {
			right = lines
		}
	}
	if len(left) == 0 || len(right) == 0 {
		t.Fatalf("the two headings did not end up over their own points: %+v", regions)
	}
	if strings.Contains(strings.Join(left, " "), "내부 채널 갈등") {
		t.Fatalf("both headings landed in one region: %+v", regions)
	}
	// Each column keeps its own points.
	if !strings.Contains(strings.Join(left, " "), "모바일 비중 확대") {
		t.Fatalf("the left column lost its points: %+v", left)
	}
	if !strings.Contains(strings.Join(right, " "), "대리점 디지털 역량 부족") {
		t.Fatalf("the right column lost its points: %+v", right)
	}

	// A layout with one region gets one list rather than two crammed headings.
	single := Compile(ParseSource("# 한 칸\n@layout 제목-및-내용\n"+
		"> 왼쪽\n- 한 줄\n> 오른쪽\n- 다른 줄\n"), manifest, CompileOptions{Language: "ko"})
	said := ""
	for _, paragraphs := range Decode(single.Slides[0].Content).Fields {
		for _, paragraph := range paragraphs {
			said += paragraph.Text + "\n"
		}
	}
	for _, wanted := range []string{"왼쪽", "한 줄", "오른쪽", "다른 줄"} {
		if !strings.Contains(said, wanted) {
			t.Fatalf("a one-region layout lost %q:\n%s", wanted, said)
		}
	}
}

// And a two-column slide comes back out as it was written. Everything in this
// language is written, compiled and written again; a heading that survives the
// compile but not the writing is a slow deletion.
func TestTwoColumnsSurviveTheRoundTrip(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 성장과 위축의 원인 분석\n@two\n" +
		"> 시장 환경 변화\n- 온라인 구매 선호도가 증가\n- 모바일 비중 확대\n" +
		"> 내부 채널 갈등\n- 가격 정책 불일치\n- 디지털 역량 부족\n"
	result := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	written := Format(model.Presentation{Title: "채널", Language: "ko", Slides: result.Slides}, manifest)
	for _, line := range []string{
		"> 시장 환경 변화", "- 온라인 구매 선호도가 증가", "- 모바일 비중 확대",
		"> 내부 채널 갈등", "- 가격 정책 불일치", "- 디지털 역량 부족",
	} {
		if !strings.Contains(written, line) {
			t.Fatalf("the round trip lost %q:\n%s", line, written)
		}
	}
	// Written again, it compiles to the same two columns.
	again := Compile(ParseSource(written), manifest, CompileOptions{Language: "ko"})
	if len(again.Slides) != len(result.Slides) {
		t.Fatalf("the deck changed size: %d then %d", len(result.Slides), len(again.Slides))
	}
	rewritten := Format(model.Presentation{Title: "채널", Language: "ko", Slides: again.Slides}, manifest)
	if strings.TrimSpace(rewritten) != strings.TrimSpace(written) {
		t.Fatalf("a second round trip changed the deck:\n%s\n---\n%s", written, rewritten)
	}
}
