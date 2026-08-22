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

// "성장 채널 | 위축 채널" is how a comparison slide names its two sides when the
// points below are one list. The bar is the same separator every component uses
// for a row; printed whole it landed as a stray line above the left column and
// left the right one unnamed.
func TestALeadWrittenAsTwoSidesNamesBothColumns(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 채널별 실적 분화\n@two\n> 성장 채널 | 위축 채널\n" +
		"- 직영은 468억으로 11.4% 성장\n- 온라인은 212억으로 28.5% 성장\n" +
		"- 대리점은 287억으로 7.4% 감소\n- 대리점 마진율 하락이 이어짐\n"
	result := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	content := Decode(result.Slides[0].Content)
	regions := map[string]string{}
	for slot, paragraphs := range content.Fields {
		for _, paragraph := range paragraphs {
			regions[slot] += paragraph.Text + "\n"
		}
	}
	var growth, decline string
	for slot, text := range regions {
		if slot == pptx.SlotTitle {
			continue
		}
		if strings.Contains(text, "성장 채널") {
			growth = text
		}
		if strings.Contains(text, "위축 채널") {
			decline = text
		}
	}
	if growth == "" || decline == "" {
		t.Fatalf("the two sides did not become two columns: %+v", regions)
	}
	if growth == decline {
		t.Fatalf("both names landed in one region: %+v", regions)
	}
	if !strings.Contains(growth, "직영은 468억") || !strings.Contains(decline, "대리점 마진율") {
		t.Fatalf("the points did not follow their side: %+v", regions)
	}
	// The whole lead must not also appear as one line anywhere.
	for _, text := range regions {
		if strings.Contains(text, "성장 채널 | 위축 채널") {
			t.Fatalf("the lead was printed whole: %+v", regions)
		}
	}

	// A sentence that merely contains a bar is not two column names.
	sentence := Compile(ParseSource("# 비교\n@two\n> 매출과 비용을 같은 기준으로 | 분기별로 나누어 자세히 살펴봅니다\n"+
		"- 한 줄\n- 다른 줄\n"), manifest, CompileOptions{Language: "ko"})
	whole := strings.TrimSpace(sentence.Slides[0].Subtitle)
	for _, paragraphs := range Decode(sentence.Slides[0].Content).Fields {
		for _, paragraph := range paragraphs {
			if strings.Contains(paragraph.Text, "매출과 비용을 같은 기준으로 | 분기별로") {
				whole = paragraph.Text
			}
		}
	}
	if whole == "" {
		t.Fatalf("a sentence with a bar in it was cut in half: %+v", Decode(sentence.Slides[0].Content))
	}
}

// On a slide the author called a comparison, the points are the things being
// compared — peers. Read as a table, the first of them became the header row:
// one option in the body and the other one masquerading as column titles.
func TestComparisonRowsAreNotReadAsATableHeader(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 현행 유지와 이중화 비교\n@comparison\n> 두 가지 시나리오를 비교\n" +
		"- 현행 유지 | 400억 원 손실 위험 | 단일 장애점 존재\n" +
		"- 이중화 구축 | 2.4억 원 추가 비용 | 99.95% 가용성 확보\n"
	result := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	content := Decode(result.Slides[0].Content)
	if len(content.Blocks) == 0 {
		t.Fatalf("the rows did not become a component: %+v", content)
	}
	for _, block := range content.Blocks {
		if block.Kind != pptx.BlockComparison {
			t.Fatalf("the rows were drawn as %q", block.Kind)
		}
		if len(block.Columns) > 0 {
			t.Fatalf("one of the options became the header row: %+v", block.Columns)
		}
		said := ""
		for _, row := range block.Rows {
			said += strings.Join(row, " ") + "\n"
		}
		for _, wanted := range []string{"현행 유지", "400억 원 손실 위험", "이중화 구축", "99.95% 가용성 확보"} {
			if !strings.Contains(said, wanted) {
				t.Fatalf("the comparison lost %q:\n%s", wanted, said)
			}
		}
	}

	// A table the author actually wrote still has its header.
	table := Compile(ParseSource("# 연간 비용\n::table 연간 비용\n- 항목 | 2026 | 2027\n- 인건비 | 4.2억 | 3.4억\n::\n"),
		manifest, CompileOptions{Language: "ko"})
	for _, block := range Decode(table.Slides[0].Content).Blocks {
		if block.Kind != pptx.BlockTable || len(block.Columns) == 0 {
			t.Fatalf("an explicit table lost its header: %+v", block)
		}
	}
}

// The definitions are listed to authors and to the model by name — raci,
// checklist, matrix — so writing the name as the fence is what anyone does with
// a list of names. It used to be an unknown component, and the rows fell
// through to the slide as stray bullets.
func TestAGridCanBeNamedDirectly(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 준비 상태\n::checklist 이관 준비\n- 항목 | 상태\n- 요건 정의 | 완료\n- 이중화 구현 | 진행\n- 최종 검수 | 미착수\n::\n"
	parsed := ParseSource(source)
	if len(parsed.Warnings) != 0 {
		t.Fatalf("naming a grid warned: %v", parsed.Warnings)
	}
	if len(parsed.Slides) != 1 || len(parsed.Slides[0].Blocks) != 1 {
		t.Fatalf("parsed %+v", parsed.Slides)
	}
	block := parsed.Slides[0].Blocks[0]
	if block.Kind != pptx.BlockGrid || block.Definition != "checklist" {
		t.Fatalf("the fence did not name the definition: %+v", block)
	}
	if block.Caption != "이관 준비" {
		t.Fatalf("the caption reads %q", block.Caption)
	}
	result := Compile(parsed, manifest, CompileOptions{Language: "ko"})
	drawn := false
	for _, drawnBlock := range Decode(result.Slides[0].Content).Blocks {
		if drawnBlock.Kind == pptx.BlockGrid && drawnBlock.Grid != nil && drawnBlock.Grid.Name == "checklist" {
			drawn = true
		}
	}
	if !drawn {
		t.Fatalf("the checklist was not drawn as a grid: %+v", Decode(result.Slides[0].Content))
	}

	// A name nobody has defined is still an unknown component, said plainly.
	unknown := ParseSource("# 제목\n::flowchart\n- 한 줄\n::\n")
	if len(unknown.Warnings) == 0 || !strings.Contains(unknown.Warnings[0], "flowchart") {
		t.Fatalf("an unknown fence was not reported: %v", unknown.Warnings)
	}
}

// A model writing a comparison puts each side's name on its own line, before
// any point: "> 투자" then "> 유지" then the points to share out. Joined with a
// space, the left column was headed "투자 유지" — a heading that says nothing —
// and the right column was headed nothing at all. This is the deck the model
// actually wrote, cut to size.
func TestTwoHeadingsOnTheirOwnLinesNameTheTwoColumns(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 투자 대비 위험 비용\n@comparison\n> 투자\n> 유지\n" +
		"- 12억 원 초기 비용\n- 안정성 확보 및 신뢰도 상승\n" +
		"- 추가 투자 없이 현재 상태 유지\n- 장애 리스크 지속 및 확대\n"
	result := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	content := Decode(result.Slides[0].Content)
	regions := map[string]string{}
	for slot, paragraphs := range content.Fields {
		if slot == pptx.SlotTitle {
			continue
		}
		for _, paragraph := range paragraphs {
			regions[slot] += paragraph.Text + "\n"
		}
	}
	var invest, hold string
	for _, text := range regions {
		if strings.Contains(text, "투자\n") {
			invest = text
		}
		if strings.Contains(text, "유지\n") {
			hold = text
		}
	}
	if invest == "" || hold == "" || invest == hold {
		t.Fatalf("the two headings did not become two columns: %+v", regions)
	}
	if !strings.Contains(invest, "12억 원 초기 비용") || !strings.Contains(hold, "장애 리스크 지속") {
		t.Fatalf("the points did not follow their side: %+v", regions)
	}
	for _, text := range regions {
		if strings.Contains(text, "투자 유지") {
			t.Fatalf("the two headings were glued into one: %+v", regions)
		}
	}

	// Two lines of a sentence are still one lead. Only a pair of names is a pair
	// of columns.
	sentence := ParseSource("# 비교\n@two\n> 매출과 비용을 같은 기준으로 나누어\n" +
		"> 분기별로 자세히 살펴봅니다\n- 한 줄\n- 다른 줄\n")
	if lead := sentence.Slides[0].Lead; strings.Contains(lead, "|") {
		t.Fatalf("a two-line sentence was read as two columns: %q", lead)
	}
}

// A model writing a contents slide put the first item on the lead line and the
// rest below it as points. Drawn as written, "1." sits above the list without a
// bullet — and on a layout with a subtitle region it lands in the subtitle,
// separated from "2." to "5." altogether. The numbering breaks on the one slide
// whose whole job is the numbering.
func TestTheFirstItemOfAListIsNotALead(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 목차\n@content\n> 1. 시장 성장과 시스템 리스크\n- 2. 장애 현황 및 영향 분석\n" +
		"- 3. 이중화 투자 계획\n- 4. 투자 대비 위험 비용 비교\n- 5. 승인 요청 및 일정\n"
	content := Decode(Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"}).Slides[0].Content)
	items := 0
	for slot, paragraphs := range content.Fields {
		if slot == pptx.SlotTitle {
			continue
		}
		for _, paragraph := range paragraphs {
			items++
			if paragraph.Lead {
				t.Fatalf("%q was drawn as a lead among its own list", paragraph.Text)
			}
		}
	}
	if items != 5 {
		t.Fatalf("the list has %d items, wanted all 5 together: %+v", items, content.Fields)
	}

	// A lead that is not the first of a list still leads. "1인당 매출" is not an
	// item number, and neither is a lead whose points do not continue it.
	for _, kept := range []string{
		"# 성장의 근거\n@content\n> 세 가지로 좁혀 말씀드립니다\n- 채널이 늘었습니다\n- 단가가 올랐습니다\n",
		"# 목표\n@content\n> 1. 배경\n- 시장이 커졌습니다\n- 경쟁이 늘었습니다\n",
	} {
		compiled := Decode(Compile(ParseSource(kept), manifest, CompileOptions{Language: "ko"}).Slides[0].Content)
		lead := false
		for slot, paragraphs := range compiled.Fields {
			if slot == pptx.SlotTitle {
				continue
			}
			for _, paragraph := range paragraphs {
				lead = lead || paragraph.Lead
			}
		}
		if !lead && len(compiled.Fields) < 3 {
			t.Fatalf("a real lead was demoted into the list: %+v", compiled.Fields)
		}
	}
}

func TestItemNumberReadsOnlyItemNumbers(t *testing.T) {
	cases := map[string]int{
		"1. 시장 성장": 1, "2) 장애 현황": 2, "③ 투자 계획": 3, "12. 마지막 항목": 12,
		"1.5배 늘었다": 0, "2026년 상반기": 0, "12억 원 요청": 0, "": 0, "성장": 0, "1.": 0,
	}
	for text, want := range cases {
		if got := itemNumber(text); got != want {
			t.Fatalf("itemNumber(%q) = %d, wanted %d", text, got, want)
		}
	}
}

// A model wrote the name of its table inside the fence, on the line above the
// column headings. The first row of a table is its headings, so the table came
// out with one column headed "준비 상태", "항목 | 상태" as its first row of data,
// and the name printed twice — once as the caption and once as that lone
// column. One cell where the rows below have more is a caption, not a header.
func TestACaptionInsideTheFenceIsNotTheHeaderRow(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 조직 준비 상태\n@content\n::table\n- 준비 상태\n- 항목 | 상태\n" +
		"- 핵심 인력 교육 | 완료\n- 변경 관리 프로세스 | 진행\n::\n"
	content := Decode(Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"}).Slides[0].Content)
	var block pptx.Block
	for _, candidate := range content.Blocks {
		block = candidate
	}
	if got := strings.Join(block.Columns, " | "); got != "항목 | 상태" {
		t.Fatalf("the header row is %q, wanted the real headings", got)
	}
	if block.Caption != "준비 상태" {
		t.Fatalf("the caption is %q, wanted the line the author wrote", block.Caption)
	}
	if len(block.Rows) != 2 {
		t.Fatalf("the table has %d rows, wanted its 2 items: %+v", len(block.Rows), block.Rows)
	}

	// A caption the fence already carries wins, and the lone line stays a row
	// when the rows below it are lone lines too.
	named := "# 목록\n@content\n::table 이미 있는 이름\n- 준비 상태\n- 항목 | 상태\n- 교육 | 완료\n::\n"
	for _, candidate := range Decode(Compile(ParseSource(named), manifest, CompileOptions{Language: "ko"}).Slides[0].Content).Blocks {
		if candidate.Caption != "이미 있는 이름" {
			t.Fatalf("the fence's own caption was replaced: %q", candidate.Caption)
		}
	}
}

// The same comparison written without "> " in front of its two names: the
// slide's first bullet read "현재 | 자동화" and neither column was named.
func TestAPairedFirstPointNamesTheColumns(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 자동화 도입 전후 비교\n@comparison\n- 현재 | 자동화\n" +
		"- 현재: 0.8% 오배송, 인력 비용 증가\n- 자동화: 0.1% 목표, 인력 30% 절감\n- 처리 속도 2배 향상 기대\n"
	content := Decode(Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"}).Slides[0].Content)
	regions := map[string]string{}
	for slot, paragraphs := range content.Fields {
		if slot == pptx.SlotTitle {
			continue
		}
		for _, paragraph := range paragraphs {
			regions[slot] += paragraph.Text + "\n"
		}
	}
	var now, after string
	for _, text := range regions {
		if strings.HasPrefix(text, "현재\n") {
			now = text
		}
		if strings.HasPrefix(text, "자동화\n") {
			after = text
		}
	}
	if now == "" || after == "" || now == after {
		t.Fatalf("the two names did not become two columns: %+v", regions)
	}
	for _, text := range regions {
		if strings.Contains(text, "현재 | 자동화") {
			t.Fatalf("the pair was drawn as a point: %+v", regions)
		}
	}

	// A point that merely contains a bar is still a point: a component row, or a
	// sentence, keeps its place in the list.
	sentence := "# 비교\n@comparison\n- 매출과 비용을 같은 기준으로 | 분기별로 나누어 자세히 살펴봅니다\n" +
		"- 한 줄\n- 다른 줄\n"
	kept := Decode(Compile(ParseSource(sentence), manifest, CompileOptions{Language: "ko"}).Slides[0].Content)
	found := false
	for slot, paragraphs := range kept.Fields {
		if slot == pptx.SlotTitle {
			continue
		}
		for _, paragraph := range paragraphs {
			found = found || strings.Contains(paragraph.Text, "매출과 비용을 같은 기준으로")
		}
	}
	if !found {
		t.Fatalf("a sentence with a bar was taken for a pair of column names: %+v", kept.Fields)
	}
}

// A hero is one number, and layoutHero draws items[0] — so a hero written with
// two figures lost one silently. The model was told a hero is one number and
// wrote two anyway; a row of indicators keeps both, which is what the author
// asked for.
func TestAHeroWithTwoFiguresKeepsBoth(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 투자 비용 및 회수 기간\n@content\n::hero 투자\n- 총 투자액 | 24억 원\n- 회수 기간 | 3년\n::\n"
	result := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	content := Decode(result.Slides[0].Content)
	var block pptx.Block
	for _, candidate := range content.Blocks {
		block = candidate
	}
	if block.Kind != pptx.BlockKPI {
		t.Fatalf("the block is %q, wanted a row that holds both figures", block.Kind)
	}
	if len(block.Items) != 2 {
		t.Fatalf("the block holds %d figures, wanted 2: %+v", len(block.Items), block.Items)
	}
	said := false
	for _, warning := range result.Warnings {
		said = said || strings.Contains(warning, "a hero draws one figure")
	}
	if !said {
		t.Fatalf("nothing said the component was changed: %v", result.Warnings)
	}

	// One figure is still a hero.
	one := Compile(ParseSource("# 투자\n@content\n::hero 투자\n- 총 투자액 | 24억 원\n::\n"),
		manifest, CompileOptions{Language: "ko"})
	for _, candidate := range Decode(one.Slides[0].Content).Blocks {
		if candidate.Kind != pptx.BlockHero {
			t.Fatalf("a single figure was not drawn as a hero: %q", candidate.Kind)
		}
	}
}

// The points of a comparison often say which side they are on. Split down the
// middle, "자동화: 0.1% 목표" landed under the heading "현재" — a slide arguing
// for the opposite of what it says.
func TestPointsThatNameTheirSideGoToThatSide(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 자동화 도입 전후 비교\n@comparison\n> 현재 | 자동화\n" +
		"- 현재: 0.8% 오배송, 인력 비용 증가\n- 자동화: 0.1% 목표, 인력 30% 절감\n- 처리 속도 2배 향상 기대\n"
	content := Decode(Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"}).Slides[0].Content)
	regions := map[string][]string{}
	for slot, paragraphs := range content.Fields {
		if slot == pptx.SlotTitle {
			continue
		}
		for _, paragraph := range paragraphs {
			regions[slot] = append(regions[slot], paragraph.Text)
		}
	}
	var now, after []string
	for _, lines := range regions {
		if lines[0] == "현재" {
			now = lines
		}
		if lines[0] == "자동화" {
			after = lines
		}
	}
	if len(now) == 0 || len(after) == 0 {
		t.Fatalf("the sides were not named: %+v", regions)
	}
	if strings.Join(now, " ") != "현재 0.8% 오배송, 인력 비용 증가" {
		t.Fatalf("the current side holds %q", now)
	}
	// The unprefixed point follows the one before it, which is about automation.
	if len(after) != 3 || !strings.Contains(after[2], "처리 속도 2배") {
		t.Fatalf("the automation side holds %q", after)
	}
	for _, lines := range regions {
		for _, line := range lines {
			if strings.HasPrefix(line, "현재:") || strings.HasPrefix(line, "자동화:") {
				t.Fatalf("the side's name is repeated inside its own column: %q", line)
			}
		}
	}

	// Only one side naming itself says nothing about the other, so the even split
	// stands.
	half := "# 비교\n@comparison\n> 현재 | 자동화\n- 현재: 느립니다\n- 빨라집니다\n- 정확해집니다\n- 싸집니다\n"
	compiled := Decode(Compile(ParseSource(half), manifest, CompileOptions{Language: "ko"}).Slides[0].Content)
	for slot, paragraphs := range compiled.Fields {
		if slot == pptx.SlotTitle {
			continue
		}
		if len(paragraphs) > 1 && paragraphs[0].Text == "현재" && len(paragraphs) != 3 {
			t.Fatalf("the split changed with only one side named: %+v", compiled.Fields)
		}
	}
}

// A model wrote a whole slide on its lead line: three points separated by
// slashes, drawn as one run-on sentence across the top of an otherwise empty
// slide. It is a list, so it is drawn as one.
func TestASlideWrittenOnItsLeadLineBecomesItsPoints(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 현재 물류 운영 현황 및 한계\n@content\n" +
		"> 3개 창고, 일 12,000건 처리 중 / 오배송률 0.8% 유지 / 인력 의존도 높고 확장성 부족\n"
	content := Decode(Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"}).Slides[0].Content)
	var lines []string
	for slot, paragraphs := range content.Fields {
		if slot == pptx.SlotTitle {
			continue
		}
		for _, paragraph := range paragraphs {
			lines = append(lines, paragraph.Text)
		}
	}
	if len(lines) != 3 {
		t.Fatalf("the slide has %d lines, wanted its 3 points: %q", len(lines), lines)
	}
	for _, line := range lines {
		if strings.Contains(line, " / ") {
			t.Fatalf("the line was left whole: %q", line)
		}
	}

	// A cover's subtitle is one statement about the deck, not a list.
	cover := Decode(Compile(ParseSource("# 물류 자동화 도입 보고\n@cover\n"+
		"> 2026년도 운영 효율화 방안 / 보고자: Ptium QA 팀\n"),
		manifest, CompileOptions{Language: "ko"}).Slides[0].Content)
	whole := false
	for _, paragraphs := range cover.Fields {
		for _, paragraph := range paragraphs {
			whole = whole || strings.Contains(paragraph.Text, " / ")
		}
	}
	if !whole {
		t.Fatalf("a cover's subtitle was split into points: %+v", cover.Fields)
	}

	// A lead above points is still a lead: the slide has more than the one line.
	kept := Decode(Compile(ParseSource("# 성장의 근거\n@content\n> 채널 / 단가 두 가지로 좁힙니다\n"+
		"- 채널이 늘었습니다\n- 단가가 올랐습니다\n"), manifest, CompileOptions{Language: "ko"}).Slides[0].Content)
	lead := false
	for slot, paragraphs := range kept.Fields {
		if slot == pptx.SlotTitle {
			continue
		}
		for _, paragraph := range paragraphs {
			lead = lead || strings.Contains(paragraph.Text, "채널 / 단가")
		}
	}
	if !lead {
		t.Fatalf("a lead with points below it was split: %+v", kept.Fields)
	}
}
