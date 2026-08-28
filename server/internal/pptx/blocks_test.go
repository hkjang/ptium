package pptx

import (
	"encoding/xml"
	"strings"
	"testing"
)

func number(value float64) *float64 { return &value }

func testDesign(t *testing.T, palette string) (Manifest, Design, Layout) {
	t.Helper()
	data, err := BuiltinTemplate(palette)
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	layout, _ := manifest.Layout(manifest.DefaultLayout)
	return manifest, NewDesign(manifest), layout
}

func bodyFrame(layout Layout) Frame {
	placeholder, _ := layout.Slot(SlotBody)
	return Frame{X: placeholder.X, Y: placeholder.Y, Width: placeholder.Width, Height: placeholder.Height}
}

func TestDesignDerivesTokensFromTheTemplate(t *testing.T) {
	for _, palette := range BuiltinDesignKeys() {
		manifest, design, _ := testDesign(t, palette)
		// The design's surface must be the colour the slides actually paint.
		if background := dominantBackground(manifest); design.Surface != background {
			t.Fatalf("%s: design surface %s does not match the slide background %s", palette, design.Surface, background)
		}
		if contrastRatio(design.InkPrimary, design.Surface) < 4.5 {
			t.Fatalf("%s: body text contrast is %.2f, below 4.5", palette, contrastRatio(design.InkPrimary, design.Surface))
		}
		if contrastRatio(design.InkMuted, design.Surface) < 3 {
			t.Fatalf("%s: muted ink contrast is %.2f, below 3", palette, contrastRatio(design.InkMuted, design.Surface))
		}
		if design.SeriesCap() < 3 {
			t.Fatalf("%s: only %d categorical slots survived validation", palette, design.SeriesCap())
		}
		// Adjacent categorical slots must be far enough apart to tell apart.
		for index := 1; index < len(design.Categorical); index++ {
			if distance := colorDistance(design.Categorical[index-1], design.Categorical[index]); distance < 8 {
				t.Fatalf("%s: categorical slots %d and %d are %.1f apart, below 8",
					palette, index, index+1, distance)
			}
		}
		// A near-grey is reserved for de-emphasis and must not be a data slot.
		for _, slot := range design.Categorical {
			if chroma(slot) < 0.045 {
				t.Fatalf("%s: %s is too close to grey for a categorical slot", palette, slot)
			}
		}
	}
}

func TestDesignRejectsGreyCategoricalSlots(t *testing.T) {
	theme := Theme{Colors: map[string]string{
		"accent1": "4472C4", // a real hue
		"accent2": "4A76C8", // too close to accent1
		"accent3": "808080", // grey
		"accent4": "ED7D31", // a real hue
	}}
	order := categoricalOrder(theme, "FFFFFF")
	if len(order) != 2 || order[0] != "4472C4" || order[1] != "ED7D31" {
		t.Fatalf("categorical order = %v, want the two distinct hues", order)
	}
}

func TestRenderBlockProducesWellFormedDrawingML(t *testing.T) {
	_, design, layout := testDesign(t, "plum-rail")
	frame := bodyFrame(layout)
	blocks := []Block{
		{Kind: BlockKPI, Items: []Item{{Label: "사용자", Number: number(128400), Delta: "+12%", Trend: "up"}, {Label: "이탈", Value: "3.2%"}}},
		{Kind: BlockHero, Items: []Item{{Label: "온보딩 이탈", Number: number(62)}}, Unit: "%"},
		{Kind: BlockSteps, Items: []Item{{Label: "계측", Detail: "지표"}, {Label: "검증", Detail: "A/B"}, {Label: "확장", Detail: "전체"}}},
		{Kind: BlockTimeline, Items: []Item{{Value: "1월", Label: "시작"}, {Value: "6월", Label: "확장"}, {Value: "9월", Label: "정착"}}},
		{Kind: BlockComparison, Items: []Item{{Label: "현행", Bullets: []string{"위험"}}, {Label: "전환", Bullets: []string{"권고"}}}},
		{Kind: BlockColumns, Unit: "%", Emphasis: 2, Items: []Item{{Label: "검색", Number: number(18)}, {Label: "추천", Number: number(31)}}},
		{Kind: BlockBars, Unit: "%", Items: []Item{{Label: "온보딩 이해 부족", Number: number(42)}, {Label: "가격 오해", Number: number(24)}}},
		{Kind: BlockLine, Labels: []string{"1Q", "2Q", "3Q"}, Series: []Series{
			{Name: "신규", Points: []float64{62, 71, 84}}, {Name: "기존", Points: []float64{58, 53, 49}}}},
		{Kind: BlockShare, Unit: "%", Items: []Item{{Label: "구독", Number: number(52)}, {Label: "라이선스", Number: number(48)}}},
		{Kind: BlockMeter, Unit: "%", Items: []Item{{Label: "매출", Number: number(78)}, {Label: "유지", Number: number(91)}}},
		{Kind: BlockTable, Columns: []string{"단계", "기간"}, Rows: [][]string{{"계측", "1개월"}, {"확장", "3개월"}}},
		{Kind: BlockQuote, Text: "실행 순서가 성패를 가른다", Attribute: "전략 리뷰"},
		{Kind: BlockCallout, Text: "이번 분기에 담당자를 확정해야 합니다"},
	}
	for _, block := range blocks {
		component := RenderBlock(design, frame, block)
		if len(component.Primitives) == 0 {
			t.Fatalf("%s produced nothing", block.Kind)
		}
		markup, nextID := component.DrawingML(2, "", nil)
		if nextID <= 2 {
			t.Fatalf("%s did not consume shape ids", block.Kind)
		}
		wrapped := `<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
			`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
			`xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` + markup + `</p:sld>`
		if err := xml.Unmarshal([]byte(wrapped), new(struct{ XMLName xml.Name })); err != nil {
			t.Fatalf("%s is not well-formed: %v\n%s", block.Kind, err, markup)
		}
		// Every primitive must stay inside the frame it was given.
		for _, primitive := range component.Primitives {
			if primitive.Kind == shapePolyline {
				continue
			}
			if primitive.Frame.X < frame.X-1 || primitive.Frame.Y < frame.Y-1 ||
				primitive.Frame.Right() > frame.Right()+1 || primitive.Frame.Bottom() > frame.Bottom()+1 {
				t.Fatalf("%s drew %s outside its frame: %+v not inside %+v",
					block.Kind, primitive.Kind, primitive.Frame, frame)
			}
		}
		svg := component.SVG(1.0/9525, "0563C1")
		if strings.TrimSpace(svg) == "" {
			t.Fatalf("%s rendered no SVG", block.Kind)
		}
	}
}

func TestRenderBlockRejectsUnusableComponents(t *testing.T) {
	_, design, layout := testDesign(t, "slate-classic")
	frame := bodyFrame(layout)
	for _, block := range []Block{
		{Kind: "unknown"},
		{Kind: BlockKPI},
		{Kind: BlockSteps, Items: []Item{{Label: "only one"}}},
		{Kind: BlockLine},
		{Kind: BlockTable, Columns: []string{"a"}},
		{Kind: BlockQuote},
	} {
		if component := RenderBlock(design, frame, block); len(component.Primitives) > 0 {
			t.Fatalf("%+v should not have rendered", block)
		}
	}
}

func TestColumnChartEmphasisRecedesOtherBars(t *testing.T) {
	_, design, layout := testDesign(t, "slate-classic")
	block := Block{Kind: BlockColumns, Emphasis: 2, Items: []Item{
		{Label: "a", Number: number(10)}, {Label: "b", Number: number(20)}, {Label: "c", Number: number(15)}}}
	component := RenderBlock(design, bodyFrame(layout), block)
	accent, grey := 0, 0
	for _, primitive := range component.Primitives {
		if primitive.Kind != shapeRound2 {
			continue
		}
		switch primitive.Fill {
		case design.Accent:
			accent++
		case design.DeEmphasis:
			grey++
		}
	}
	if accent != 1 || grey != 2 {
		t.Fatalf("emphasis painted %d accent and %d grey bars, want 1 and 2", accent, grey)
	}
}

func TestBarsAreThinAndRoundedAtTheDataEnd(t *testing.T) {
	_, design, layout := testDesign(t, "slate-classic")
	frame := bodyFrame(layout)
	component := RenderBlock(design, frame, Block{Kind: BlockColumns, Items: []Item{
		{Label: "a", Number: number(10)}, {Label: "b", Number: number(20)}}})
	found := false
	for _, primitive := range component.Primitives {
		if primitive.Kind != shapeRound2 {
			continue
		}
		found = true
		if primitive.Side != sideTop {
			t.Fatalf("a column must round its top, got %q", primitive.Side)
		}
		// The mark never fills its band; the leftover is air.
		if primitive.Frame.Width > frame.Width/2/2 {
			t.Fatalf("bar width %d is too thick for a %d-wide plot", primitive.Frame.Width, frame.Width)
		}
	}
	if !found {
		t.Fatal("no bars were drawn")
	}
	horizontal := RenderBlock(design, frame, Block{Kind: BlockBars, Items: []Item{
		{Label: "a", Number: number(10)}, {Label: "b", Number: number(20)}}})
	for _, primitive := range horizontal.Primitives {
		if primitive.Kind == shapeRound2 && primitive.Side != sideRight {
			t.Fatalf("a horizontal bar must round its right end, got %q", primitive.Side)
		}
	}
}

func TestShareBarSeparatesSegmentsWithSurfaceGaps(t *testing.T) {
	_, design, layout := testDesign(t, "slate-classic")
	component := RenderBlock(design, bodyFrame(layout), Block{Kind: BlockShare, Unit: "%", Items: []Item{
		{Label: "구독", Number: number(50)}, {Label: "라이선스", Number: number(30)}, {Label: "서비스", Number: number(20)}}})
	var segments []Frame
	for _, primitive := range component.Primitives {
		if primitive.Kind == shapeRectangle && primitive.Fill != design.Line && primitive.Frame.Height > design.Unit*2 {
			segments = append(segments, primitive.Frame)
		}
	}
	if len(segments) < 3 {
		t.Fatalf("expected three segments, got %d", len(segments))
	}
	for index := 1; index < len(segments); index++ {
		gap := segments[index].X - segments[index-1].Right()
		if gap <= 0 {
			t.Fatalf("segments %d and %d touch; a surface gap must separate them", index, index+1)
		}
	}
	// No segment carries a border: the gap does the separating.
	for _, primitive := range component.Primitives {
		if primitive.Kind == shapeRectangle && primitive.Stroke != "" {
			t.Fatal("a segment was given a border instead of a gap")
		}
	}
}

func TestFormatNumberReadsLikeASlide(t *testing.T) {
	cases := map[float64]string{
		0.5: "0.5", 42: "42", 128: "128", 1284: "1,284", 12900: "12.9K",
		4200000: "4.2M", 1500000000: "1.5B", -320: "-320",
	}
	for value, want := range cases {
		if got := formatNumber(value); got != want {
			t.Fatalf("formatNumber(%v) = %q, want %q", value, got, want)
		}
	}
}

// A model asked for a comparison writes a two-column table as often as it writes
// two options. Drawn as cards, its header row became a card of its own — this is
// the block a live model produced for "단일 공급사 68% → 목표 40%".
func TestTwoRowComparisonWithNamedSidesIsAMatrix(t *testing.T) {
	block := Block{Kind: BlockComparison, Rows: [][]string{
		{"현재", "목표"},
		{"단일 공급사 의존도 68%", "목표 의존도 40%"},
	}}
	if !IsComparisonMatrix(block) {
		t.Fatal("a first row that names the sides is a header, not an option")
	}
	rows := comparisonMatrix(block)
	if !tabularHeader(rows) {
		t.Fatalf("the header row should be drawn as a header: %v", rows)
	}
	// Two options with real names still read as cards.
	cards := Block{Kind: BlockComparison, Rows: [][]string{
		{"현행 유지", "연 4.2억 · 장애 리스크 누적"},
		{"단계 전환", "연 3.4억 · 1차 12개만 이관"},
	}}
	if IsComparisonMatrix(cards) {
		t.Fatal("two named options are cards")
	}
}

// Six figures used to be four: a row of tiles was capped, and the numbers past
// the cap were dropped without a word. A long row folds instead.
func TestEveryFigureOfAKPIRowIsDrawn(t *testing.T) {
	_, design, _ := testDesign(t, "")
	block := Block{Kind: BlockKPI, Heading: "전체 지표", Items: []Item{
		{Label: "매출", Value: "1,240억"}, {Label: "이익률", Value: "9.8%"}, {Label: "신규 고객", Value: "128개사"},
		{Label: "이탈률", Value: "2.4%"}, {Label: "재구매", Value: "61%"}, {Label: "객단가", Value: "38만원"}}}
	component := RenderBlock(design, Frame{X: 0, Y: 0, Width: 10000000, Height: 3000000}, block)
	drawn := ""
	for _, primitive := range component.Primitives {
		for _, paragraph := range primitive.Lines {
			drawn += paragraph.Text + "\n"
		}
	}
	for _, item := range block.Items {
		if !strings.Contains(drawn, item.Value) {
			t.Errorf("%s (%s) is not on the slide:\n%s", item.Label, item.Value, drawn)
		}
	}
	// Folded into two rows, not squeezed into six slivers on one.
	rows := map[int]bool{}
	for _, primitive := range component.Primitives {
		if primitive.Kind == shapeRounded {
			rows[primitive.Frame.Y] = true
		}
	}
	if len(rows) != 2 {
		t.Errorf("six figures were drawn in %d row(s)", len(rows))
	}
}

// A pull quote is a sentence. Carried as an entry's label it was cut at sixty
// characters, which ends a statement mid-thought.
func TestAQuoteKeepsItsWholeSentence(t *testing.T) {
	statement := "데이터 처리 속도는 단순한 기술 지표가 아니라 시장 변화에 얼마나 빠르게 대응할 수 있는지를 결정하는 핵심 경쟁력입니다"
	placeholder := Placeholder{Slot: SlotBody, MaxLines: 6, MaxChars: 200, Width: 8000000, Height: 3000000}
	sanitized, ok := SanitizeBlock(Block{Kind: BlockQuote, Items: []Item{{Label: statement}}}, placeholder)
	if !ok {
		t.Fatal("the quote was rejected")
	}
	if sanitized.Text != statement {
		t.Errorf("the quote reads %q", sanitized.Text)
	}
}

// "1억 5천만 원" in a card sized for three figures was drawn straight across the
// card beside it, and the room read "1억 5천". The number gives way to the card
// it is in.
func TestAKPIValueFitsItsCard(t *testing.T) {
	_, _, manifest := buildTemplate(t, "plum-rail")
	design := NewDesign(manifest)
	// The region this KPI row actually landed in, three cards across.
	frame := Frame{X: 838200, Y: 2000000, Width: 4200000, Height: 1600000}
	block := Block{Kind: BlockKPI, Items: []Item{
		{Label: "총 비용", Value: "1억 5천만 원"},
		{Label: "이관 기간", Value: "4개월"},
		{Label: "문서 건수", Value: "12,400건"},
	}}
	cardWidth := (frame.Width - design.Unit*2*2) / 3
	inner := cardWidth - design.Unit*4
	widest := 0
	for _, primitive := range design.layoutKPI(frame, block) {
		for _, line := range primitive.Lines {
			if primitive.Kind != "text" || !strings.Contains(line.Text, "1억") {
				continue
			}
			widest = max(widest, textWidth(line.Text, primitive.FontSize))
		}
	}
	if widest == 0 {
		t.Fatal("the value was not drawn at all")
	}
	// Down to the label's own size, which is the floor: a figure smaller than the
	// words around it stops being the thing the slide is about.
	if widest > inner && design.Small < 1200 {
		t.Fatalf("the value draws %d wide in a card of %d", widest, inner)
	}
	if widest > inner*11/10 {
		t.Fatalf("the value draws %d wide in a card of %d, past what the floor explains", widest, inner)
	}
}

// A component is drawn into the room the template gave it. The editorial
// layouts have a short body region, and steps held the badge and two lines of
// title and drew the rest below the region — over whatever was under it, or off
// the slide.
func TestAComponentIsDrawnInsideItsRegion(t *testing.T) {
	_, _, manifest := buildTemplate(t, "plum-rail")
	design := NewDesign(manifest)
	steps := Block{Kind: BlockSteps, Items: []Item{
		{Label: "준비", Detail: "범위 · 조직 · 예산을 확정"},
		{Label: "이행", Detail: "단계별로 적용하고 완료 조건을 확인"},
		{Label: "안정화", Detail: "운영 이관과 점검 기준 확정"},
	}}
	kpi := Block{Kind: BlockKPI, Items: []Item{
		{Label: "전환 시스템", Value: "42개"}, {Label: "절감", Value: "18억"}, {Label: "복구 시간", Value: "30분"},
	}}
	for _, height := range []int{3600000, 2400000, 1200000, 900000, 600000} {
		frame := Frame{X: 800000, Y: 1800000, Width: 9000000, Height: height}
		for name, block := range map[string]Block{"steps": steps, "kpi": kpi} {
			primitives := RenderBlock(design, frame, block).Primitives
			if len(primitives) == 0 {
				continue
			}
			lowest := 0
			for _, primitive := range primitives {
				lowest = max(lowest, primitive.Frame.Bottom())
			}
			if over := lowest - frame.Bottom(); over > frame.Height/100 {
				t.Errorf("%s in a %dEMU region draws %dEMU below it", name, height, over)
			}
		}
	}
}
