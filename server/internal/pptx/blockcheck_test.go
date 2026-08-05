package pptx

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestWriteBlockPreviews is a visual aid: set PTIUM_BLOCK_PREVIEW to a
// directory to dump one SVG per component for inspection.
func TestWriteBlockPreviews(t *testing.T) {
	directory := os.Getenv("PTIUM_BLOCK_PREVIEW")
	if directory == "" {
		t.Skip("set PTIUM_BLOCK_PREVIEW to render component previews")
	}
	data, err := BuiltinTemplate(os.Getenv("PTIUM_BLOCK_PALETTE"))
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	layout, _ := manifest.Layout(manifest.DefaultLayout)
	number := func(v float64) *float64 { return &v }

	blocks := map[string]Block{
		"kpi": {Kind: BlockKPI, Items: []Item{
			{Label: "월 활성 사용자", Number: number(128400), Delta: "+12.4%", Trend: "up"},
			{Label: "이탈률", Value: "3.2%", Delta: "-0.8pp", Trend: "up"},
			{Label: "온보딩 완료", Value: "62%", Delta: "+5pp", Trend: "up"},
		}},
		"hero": {Kind: BlockHero, Items: []Item{
			{Label: "온보딩 단계에서 발생하는 이탈 비중", Number: number(62), Detail: "직전 분기 54%에서 8포인트 상승"},
		}, Unit: "%"},
		"steps": {Kind: BlockSteps, Items: []Item{
			{Label: "계측", Detail: "문제 구간에 지표를 심습니다"},
			{Label: "검증", Detail: "2주 단위로 개선안을 A/B로 확인"},
			{Label: "확장", Detail: "성공 패턴을 전체 퍼널로 적용"},
		}, Emphasis: 2},
		"timeline": {Kind: BlockTimeline, Items: []Item{
			{Value: "1월", Label: "기준선 확보", Detail: "계측 완료"},
			{Value: "3월", Label: "파일럿", Detail: "단일 채널"},
			{Value: "6월", Label: "확장", Detail: "전 채널"},
			{Value: "9월", Label: "정착", Detail: "운영 이관"},
		}},
		"comparison": {Kind: BlockComparison, Items: []Item{
			{Label: "현행 유지", Value: "위험 높음", Bullets: []string{"이탈이 계속 누적됩니다", "선택지가 줄어듭니다"}},
			{Label: "단계적 전환", Value: "권고", Bullets: []string{"2주 단위로 되돌릴 수 있습니다", "예산 재배분으로 시작 가능"}},
		}},
		"columns": {Kind: BlockColumns, Heading: "채널별 이탈률", Unit: "%", Items: []Item{
			{Label: "검색", Number: number(18)}, {Label: "추천", Number: number(24)},
			{Label: "직접", Number: number(11)}, {Label: "이메일", Number: number(31)},
			{Label: "제휴", Number: number(9)},
		}, Emphasis: 4},
		"bars": {Kind: BlockBars, Heading: "이탈 원인", Unit: "%", Items: []Item{
			{Label: "온보딩 단계 이해 부족", Number: number(42)},
			{Label: "가격 정책 오해", Number: number(24)},
			{Label: "기능 탐색 실패", Number: number(19)},
			{Label: "기타", Number: number(15)},
		}},
		"line": {Kind: BlockLine, Heading: "분기별 유지율", Unit: "%",
			Labels: []string{"1Q", "2Q", "3Q", "4Q", "1Q"},
			Series: []Series{
				{Name: "신규 세그먼트", Points: []float64{62, 66, 71, 78, 84}},
				{Name: "기존 채널", Points: []float64{58, 56, 53, 51, 49}},
			}},
		"share": {Kind: BlockShare, Heading: "매출 구성", Unit: "%", Items: []Item{
			{Label: "구독", Number: number(52)}, {Label: "라이선스", Number: number(28)},
			{Label: "전문 서비스", Number: number(20)},
		}},
		"meter": {Kind: BlockMeter, Heading: "목표 달성률", Unit: "%", Items: []Item{
			{Label: "매출", Number: number(78)}, {Label: "신규 고객", Number: number(46)},
			{Label: "유지율", Number: number(91)},
		}},
		"table": {Kind: BlockTable, Heading: "단계별 계획", Columns: []string{"단계", "기간", "책임", "지표"},
			Rows: [][]string{
				{"계측", "1개월", "데이터팀", "기준선"},
				{"파일럿", "2개월", "성장팀", "전환율"},
				{"확장", "3개월", "전사", "유지율"},
			}},
		"quote":   {Kind: BlockQuote, Caption: "성패는 실행 속도가 아니라 실행 순서에서 갈립니다", Attribute: "2026 전략 리뷰"},
		"callout": {Kind: BlockCallout, Caption: "이번 분기에 결정해야 하는 것은 예산이 아니라 담당자입니다"},
	}
	for name, block := range blocks {
		slide := Slide{LayoutID: layout.ID, Fields: map[string][]Paragraph{SlotTitle: {{Text: "컴포넌트: " + name}}},
			Blocks: map[string]Block{SlotBody: block}}
		svg := PreviewSVG(manifest, layout, slide, PreviewOptions{Width: 1000})
		if err := os.WriteFile(fmt.Sprintf("%s/%s.svg", directory, name), []byte(svg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// The inspector measured the box a component gave its text, not the text. That is
// how a slide whose heading was drawn on top of its own first row was reported
// clean, so the measurement is now of what is drawn.
func TestInspectComponentMeasuresDrawnText(t *testing.T) {
	manifest := Manifest{SlideWidth: 12192000, SlideHeight: 6858000}
	design := NewDesign(manifest)
	region := Placeholder{Slot: "body", X: 1000000, Y: 2000000, Width: 2200000, Height: 2600000, Kind: "text"}
	frame := Frame{X: region.X, Y: region.Y, Width: region.Width, Height: region.Height}
	block := Block{
		Kind: BlockComparison,
		// Longer than the three lines a heading is allowed to reserve.
		Heading: "클라우드 네이티브 아키텍처로 전환하면 운영 비용과 배포 주기를 동시에 줄일 수 있으며 " +
			"보안 대응 역량과 확장성 또한 크게 개선되어 전사적인 경쟁력 확보에 기여합니다",
		Items: []Item{
			{Label: "아키텍처", Value: "모놀리식", Detail: "마이크로서비스"},
			{Label: "확장성", Value: "수직 확장", Detail: "수평 확장"},
			{Label: "유지보수", Value: "고비용", Detail: "저비용"},
		},
	}
	findings := inspectComponent(region, frame, block, design, manifest.SlideWidth, manifest.SlideHeight)
	overflow := false
	for _, finding := range findings {
		if finding.Kind == FindingOverflow {
			overflow = true
		}
	}
	if !overflow {
		t.Fatalf("a heading taller than its reserved room must be reported: %+v", findings)
	}
}

// A component may cover more than the region it is placed in: a comparison
// matrix squeezed into one column of a two-column layout wastes half the slide.
func TestBlockSpanCoversBothRegions(t *testing.T) {
	layout := Layout{ID: "two", Placeholders: []Placeholder{
		{Slot: "body", X: 1000000, Y: 2000000, Width: 4000000, Height: 3000000, Kind: "text", MaxLines: 8},
		{Slot: "body2", X: 6000000, Y: 2000000, Width: 4000000, Height: 3000000, Kind: "text", MaxLines: 8},
	}}
	block := Block{Kind: BlockComparison, Span: []string{"body", "body2"}, Items: []Item{
		{Label: "아키텍처", Value: "모놀리식", Detail: "마이크로서비스"},
		{Label: "확장성", Value: "수직", Detail: "수평"},
		{Label: "비용", Value: "높음", Detail: "낮음"},
	}}
	frame := blockFrame(layout, layout.Placeholders[0], block)
	if frame.X != 1000000 || frame.Width != 9000000 {
		t.Fatalf("spanned frame = %+v", frame)
	}

	slide := Slide{LayoutID: "two", Blocks: map[string]Block{"body": block}}
	if spanned := slide.spannedSlots(); !spanned["body2"] {
		t.Fatalf("the covered region must not be drawn separately: %v", spanned)
	}
	// The covered region is not drawn a second time as an empty box.
	svg := PreviewSVG(Manifest{SlideWidth: 12192000, SlideHeight: 6858000}, layout, slide, PreviewOptions{Width: 960})
	if strings.Count(svg, "아키텍처") != 1 {
		t.Fatalf("the component should be drawn once:\n%s", svg)
	}
}

// "현재 | 목표" is a header of two column names, and it is exactly as short as the
// figures under it — so the row is recognised by what the words are, not by
// their length. Drawn as data it becomes the comparison's first row.
func TestComparisonHeaderOfSideNames(t *testing.T) {
	block := Block{Kind: BlockComparison, Rows: [][]string{
		{"현재", "목표"}, {"4시간", "5분"}, {"수동", "자동"}, {"30%", "95%"},
	}}
	rows := comparisonMatrix(block)
	if !tabularHeader(rows) {
		t.Fatal("a row of side names is a header")
	}

	manifest := Manifest{SlideWidth: 12192000, SlideHeight: 6858000}
	design := NewDesign(manifest)
	frame := Frame{X: 1000000, Y: 2000000, Width: 9000000, Height: 3000000}
	primitives := design.layoutComparison(frame, block)
	// Both columns are sides, so both carry an accent rule and neither is drawn as
	// a row label spanning a third of the slide.
	rules := 0
	for _, primitive := range primitives {
		if primitive.Kind == shapeRectangle && primitive.Frame.Height <= design.Unit/2 {
			rules++
		}
	}
	if rules != 2 {
		t.Fatalf("expected an accent rule under each column, got %d", rules)
	}
	// The figures still read in order, three rows of them.
	texts := 0
	for _, primitive := range primitives {
		if primitive.Kind == shapeText {
			texts++
		}
	}
	if texts != 8 {
		t.Fatalf("expected two titles and six cells, got %d", texts)
	}
}

// A bullet's second line hangs under its text; a title's second line has no
// marker to hang from. Indenting it anyway made the preview show a step no
// exported slide has.
func TestPreviewWrapsATitleWithoutIndenting(t *testing.T) {
	layout := Layout{ID: "cover", Placeholders: []Placeholder{
		{Slot: SlotTitle, X: 1000000, Y: 700000, Width: 9000000, Height: 1400000,
			Kind: "text", MaxLines: 2, FontSize: 3400, LineEm: 18},
		{Slot: "body", X: 1000000, Y: 2400000, Width: 9000000, Height: 2000000,
			Kind: "text", MaxLines: 6, FontSize: 1500, LineEm: 40},
	}}
	slide := Slide{LayoutID: "cover", Fields: map[string][]Paragraph{
		SlotTitle: {{Text: "기존 방식 대비 신규 방식은 운영 효율성을 40% 높입니다"}},
		"body":    {{Text: "전환 이후에는 자원 사용량에 따라 과금되므로 초기 투자 비용이 크게 줄어들고 운영 인력이 반복 작업에서 벗어나 개선 과제에 집중할 수 있게 되며 장애 대응 시간도 함께 줄어듭니다"}},
	}}
	svg := PreviewSVG(Manifest{SlideWidth: 12192000, SlideHeight: 6858000}, layout, slide, PreviewOptions{Width: 960})

	titleXs := tspanColumns(t, svg, "높입니다")
	if len(titleXs) != 1 {
		t.Fatalf("expected the title's last line once: %v", titleXs)
	}
	firstXs := tspanColumns(t, svg, "기존 방식")
	if len(firstXs) != 1 || titleXs[0] != firstXs[0] {
		t.Fatalf("a title's wrapped line must start where the first line does: %v vs %v", titleXs, firstXs)
	}

	// A bullet still hangs: its continuation is indented past its marker.
	bulletFirst := tspanColumns(t, svg, "• 전환 이후")
	bulletNext := tspanColumns(t, svg, "줄어듭니다")
	if len(bulletFirst) != 1 || len(bulletNext) != 1 || bulletNext[0] <= bulletFirst[0] {
		t.Fatalf("a bullet's continuation should hang: %v vs %v", bulletNext, bulletFirst)
	}
}

// tspanColumns is the x of every tspan whose text contains a fragment.
func tspanColumns(t *testing.T, svg, fragment string) []float64 {
	t.Helper()
	var found []float64
	for _, part := range strings.Split(svg, "<tspan ")[1:] {
		end := strings.Index(part, "</tspan>")
		if end < 0 {
			continue
		}
		element := part[:end]
		body := element[strings.Index(element, ">")+1:]
		if !strings.Contains(body, fragment) {
			continue
		}
		attribute := element[strings.Index(element, `x="`)+3:]
		value, err := strconv.ParseFloat(attribute[:strings.Index(attribute, `"`)], 64)
		if err != nil {
			t.Fatalf("x is not a number in %q", element)
		}
		found = append(found, value)
	}
	return found
}

// A card's headline gets three lines at most. Asked for a fourth it is set
// smaller — the alternative, which shipped, was three lines of room and four
// lines of text, with the fourth over the card's edge.
func TestComparisonCardHeadlineFitsItsCard(t *testing.T) {
	manifest := Manifest{SlideWidth: 12192000, SlideHeight: 6858000}
	design := NewDesign(manifest)
	// The pair a real model wrote, in the region a two-column layout gives it.
	block := Block{Kind: BlockComparison, Items: []Item{
		{Label: "1단계", Value: "검증 프로세스 표준화 및 자동화 도구 도입"},
		{Label: "2단계", Value: "모니터링 대시보드 구축 및 정기 점검"},
	}}
	region := Placeholder{Slot: "body3", X: 6324600, Y: 1778508, Width: 4678680, Height: 3525012, Kind: "text"}
	frame := Frame{X: region.X, Y: region.Y, Width: region.Width, Height: region.Height}
	for _, finding := range inspectComponent(region, frame, block, design, manifest.SlideWidth, manifest.SlideHeight) {
		t.Fatalf("the card should hold its own headline: %s", finding.Detail)
	}

	// The headline is still set as large as it can be: a short one is not shrunk.
	short, shortLines := design.comparisonValueType("자동화", 4678680/2)
	long, longLines := design.comparisonValueType("검증 프로세스 표준화 및 자동화 도구 도입", 4678680/2)
	if short <= long || shortLines > longLines {
		t.Fatalf("short=%d/%d long=%d/%d", short, shortLines, long, longLines)
	}
	if longLines > 3 {
		t.Fatalf("a card never carries more than three lines: %d", longLines)
	}
}
