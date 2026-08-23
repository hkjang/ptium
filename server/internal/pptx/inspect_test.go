package pptx

import (
	"fmt"
	"strings"
	"testing"
)

// exerciseDeck is a deck that uses every kind of region a real one does, with
// Korean copy at realistic length: a cover, prose, a component, a table and a
// chart. Anything that overflows, escapes or collides shows up here.
func exerciseDeck(manifest Manifest) Deck {
	line := func(text string) []Paragraph { return []Paragraph{{Text: text}} }
	points := []Paragraph{
		{Text: "전환 대상 42개 시스템을 세 묶음으로 나누어 순차적으로 이관합니다"},
		{Text: "1차 범위는 트래픽이 낮은 12개로 한정해 위험을 줄입니다"},
		{Text: "선행 조건이 남아 있어 병행 진행은 불가능합니다", Level: 1},
		{Text: "회수 시점은 14개월이며 인건비 동결을 가정했습니다"},
	}
	layoutFor := func(role string) Layout {
		if layout, ok := manifest.LayoutForRole(role); ok {
			return layout
		}
		return manifest.Layouts[0]
	}
	// The roomiest body region, which is what the compiler chooses too: a layout
	// may open with a one-line eyebrow that reading order would otherwise pick.
	bodySlot := func(layout Layout) string {
		best, slot := 0, SlotBody
		for _, placeholder := range layout.BodySlots() {
			if area := placeholder.Width * placeholder.Height; area > best {
				best, slot = area, placeholder.Slot
			}
		}
		return slot
	}

	cover := layoutFor(RoleTitle)
	content := layoutFor(RoleContent)
	closing := layoutFor(RoleClosing)

	deck := Deck{Title: "클라우드 전환 로드맵", Language: "ko"}
	notes := "이 슬라이드에서 말할 내용을 두세 문장으로 적어 둡니다."
	deck.Slides = append(deck.Slides, Slide{LayoutID: cover.ID, Notes: notes, Fields: map[string][]Paragraph{
		SlotTitle:    line("2026년 하반기 클라우드 전환 로드맵과 투자 타당성"),
		SlotSubtitle: line("KCB 데이터혁신본부 · 임원 보고"),
	}})
	deck.Slides = append(deck.Slides, Slide{LayoutID: content.ID, Fields: map[string][]Paragraph{
		SlotTitle:         line("지금 결정이 필요한 이유"),
		bodySlot(content): points,
	}})
	for _, block := range []Block{
		{Kind: BlockKPI, Caption: "규모", Items: []Item{
			{Label: "전환 대상", Value: "42개"}, {Label: "1차 범위", Value: "12개"}, {Label: "예상 절감", Value: "18%"}}},
		{Kind: BlockSteps, Caption: "3단계", Items: []Item{
			{Label: "준비", Value: "범위 · 조직 · 예산 확정"},
			{Label: "이행", Value: "1차 12개 워크로드 이관"},
			{Label: "안정화", Value: "운영 이관과 점검 기준 확정"}}},
		{Kind: BlockComparison, Items: []Item{
			{Label: "현행 유지", Value: "연 4.2억 · 장애 리스크 누적"},
			{Label: "전환 후", Value: "연 3.4억 · 확장 비용 선형"}}},
		{Kind: BlockTable, Caption: "연간 비용 (억원)", Columns: []string{"항목", "2026", "2027", "2028"},
			Rows: [][]string{{"인건비", "4.2", "3.4", "3.1"}, {"라이선스", "1.1", "1.4", "1.4"}, {"운영", "0.8", "0.6", "0.5"}}},
		{Kind: BlockLine, Caption: "월별 처리량", Labels: []string{"1월", "2월", "3월", "4월", "5월"},
			Series: []Series{{Name: "전환 전", Points: []float64{120, 118, 121, 119, 122}},
				{Name: "전환 후", Points: []float64{120, 132, 148, 165, 181}}}},
		{Kind: BlockGrid, Caption: "담당 체계", Grid: gridSpecFor("raci"),
			Columns: []string{"활동", "기획", "개발", "운영"},
			Rows: [][]string{{"요건 정의", "R", "C", "I"}, {"설계", "A", "R", "C"},
				{"이관", "C", "R", "A"}, {"검증", "I", "C", "R"}}},
		{Kind: BlockShare, Caption: "구성", Items: []Item{
			{Label: "핵심", Number: pointer(52)}, {Label: "주변", Number: pointer(31)}, {Label: "폐기", Number: pointer(17)}}},
	} {
		deck.Slides = append(deck.Slides, Slide{LayoutID: content.ID, Notes: notes,
			Fields: map[string][]Paragraph{SlotTitle: line("전환 계획 상세")},
			Blocks: map[string]Block{bodySlot(content): block}})
	}
	deck.Slides = append(deck.Slides, Slide{LayoutID: closing.ID, Notes: notes, Fields: map[string][]Paragraph{
		SlotTitle:         line("다음 단계"),
		bodySlot(closing): line("3분기 착수를 승인해 주십시오"),
	}})
	return deck
}

func pointer(value float64) *float64 { return &value }

func gridSpecFor(name string) *GridSpec {
	spec, ok := LookupBuiltinGrid(name)
	if !ok {
		return nil
	}
	return &spec
}

// Every shipped design has to draw a realistic deck without a single defect.
// This is the test that replaces looking at a rendered PNG.
func TestBuiltinDesignsDrawARealDeckCleanly(t *testing.T) {
	for _, key := range BuiltinDesignKeys() {
		data, err := BuiltinTemplate(key)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		pkg, manifest, err := AnalyzeBytes(data)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		deck := exerciseDeck(manifest)
		// Drawing defects only: whether a deck has speaker notes is the author's
		// business, and this test is about the designs.
		if findings := Defects(InspectDeck(manifest, deck)); len(findings) > 0 {
			messages := make([]string, 0, len(findings))
			for _, finding := range findings {
				messages = append(messages, finding.String())
			}
			t.Errorf("%s draws a realistic deck with defects:\n  %s", key, strings.Join(messages, "\n  "))
		}
		// The same deck must also render, since a defect-free inspection of a deck
		// that cannot be produced would prove nothing.
		if _, err := Render(pkg, manifest, deck); err != nil {
			t.Fatalf("%s: render: %v", key, err)
		}
	}
}

func TestInspectFindsTextThatCannotFit(t *testing.T) {
	placeholder := Placeholder{Slot: SlotBody, Kind: "text", Type: "body",
		X: 800000, Y: 2000000, Width: 4000000, Height: 900000,
		FontSize: 1800, MaxChars: 60, MaxLines: 3, LineEm: 20}
	layout := Layout{ID: "content", Name: "Content", Role: RoleContent, Background: "FFFFFF",
		Placeholders: []Placeholder{placeholder}}
	manifest := Manifest{Version: ManifestVersion, SlideWidth: 12192000, SlideHeight: 6858000,
		Theme:   Theme{Colors: map[string]string{"lt1": "FFFFFF", "dk1": "111111", "accent1": "1E6FFF"}},
		Layouts: []Layout{layout}}

	fits := Slide{LayoutID: "content", Fields: map[string][]Paragraph{
		SlotBody: {{Text: "짧은 요점 하나"}, {Text: "두 번째 요점"}}}}
	if findings := InspectSlide(manifest, layout, fits, NewDesign(manifest)); len(findings) != 0 {
		t.Fatalf("text that fits must not be reported: %v", findings)
	}

	long := strings.Repeat("전환 대상 시스템의 이관 순서와 선행 조건을 상세히 설명하는 긴 문장입니다. ", 6)
	spills := Slide{LayoutID: "content", Fields: map[string][]Paragraph{SlotBody: {{Text: long}}}}
	findings := InspectSlide(manifest, layout, spills, NewDesign(manifest))
	if len(findings) != 1 || findings[0].Kind != FindingOverflow {
		t.Fatalf("findings = %v", findings)
	}
	if !strings.Contains(findings[0].Detail, "lines of text in room for") {
		t.Fatalf("the finding should say how much does not fit: %q", findings[0].Detail)
	}
}

func TestInspectFindsTextOverALogoAndOffTheSlide(t *testing.T) {
	manifest := Manifest{Version: ManifestVersion, SlideWidth: 12192000, SlideHeight: 6858000,
		Theme: Theme{Colors: map[string]string{"lt1": "FFFFFF", "dk1": "111111"}}}
	logo := Artwork{Kind: "picture", X: 700000, Y: 1800000, Width: 2000000, Height: 1200000,
		Image: "ppt/media/image1.png", Average: "222222"}
	// A composed region that lands on the logo, and a title that runs off the edge.
	layout := Layout{ID: "brand", Name: "Brand", Role: RoleContent, Background: "FFFFFF",
		Fill: Background{Fill: "FFFFFF"}, Artwork: []Artwork{logo},
		Placeholders: []Placeholder{
			{Slot: SlotTitle, Kind: "text", Type: "title", Synthetic: true, Color: "FFFFFF",
				X: 11000000, Y: 400000, Width: 2000000, Height: 800000, FontSize: 3200, MaxChars: 40, MaxLines: 2, LineEm: 22},
			{Slot: SlotBody, Kind: "text", Type: "body", Synthetic: true, Color: "111111",
				X: 800000, Y: 1900000, Width: 2200000, Height: 1000000, FontSize: 1800, MaxChars: 80, MaxLines: 4, LineEm: 24},
		}}
	manifest.Layouts = []Layout{layout}
	slide := Slide{LayoutID: "brand", Fields: map[string][]Paragraph{
		SlotTitle: {{Text: "제목"}}, SlotBody: {{Text: "본문 한 줄"}}}}

	kinds := map[string]string{}
	for _, finding := range InspectSlide(manifest, layout, slide, NewDesign(manifest)) {
		kinds[finding.Kind] = finding.Slot + ": " + finding.Detail
	}
	if _, ok := kinds[FindingOutside]; !ok {
		t.Fatalf("a region past the slide edge must be reported: %v", kinds)
	}
	if detail, ok := kinds[FindingCollision]; !ok || !strings.Contains(detail, "picture") {
		t.Fatalf("text over the layout's own picture must be reported: %v", kinds)
	}
	// White text on a dark logo is fine; white text on white is not.
	if _, ok := kinds[FindingContrast]; !ok {
		t.Fatalf("unreadable composed text must be reported: %v", kinds)
	}
}

func TestInspectFindsAHeadingThatEndsOnOneSyllable(t *testing.T) {
	// A title region fourteen ems wide. "…투자 타" wraps to a second line holding
	// one syllable, which is the classic amateur tell.
	title := Placeholder{Slot: SlotTitle, Kind: "text", Type: "title",
		X: 800000, Y: 600000, Width: 6000000, Height: 1200000,
		FontSize: 3200, MaxChars: 28, MaxLines: 2, LineEm: 14}
	layout := Layout{ID: "content", Name: "Content", Role: RoleContent, Background: "FFFFFF",
		Placeholders: []Placeholder{title}}
	manifest := Manifest{Version: ManifestVersion, SlideWidth: 12192000, SlideHeight: 6858000,
		Theme:   Theme{Colors: map[string]string{"lt1": "FFFFFF", "dk1": "111111", "accent1": "1E6FFF"}},
		Layouts: []Layout{layout}}
	design := NewDesign(manifest)

	orphaned := Slide{LayoutID: "content", Fields: map[string][]Paragraph{
		SlotTitle: {{Text: "클라우드 전환 로드맵과 투자 타"}}}}
	findings := InspectSlide(manifest, layout, orphaned, design)
	if len(findings) != 1 || findings[0].Kind != FindingOrphan {
		t.Fatalf("findings = %+v", findings)
	}
	// Nothing is drawn wrong: the heading simply reads better if it is tightened.
	if !findings[0].Advisory {
		t.Fatal("an orphan is a polish issue, not a defect")
	}

	// A heading that fills both lines is fine, and so is one that fits on one.
	for _, text := range []string{"클라우드 전환 로드맵", "클라우드 전환 로드맵과 투자 타당성 보고서 초안 검"} {
		slide := Slide{LayoutID: "content", Fields: map[string][]Paragraph{SlotTitle: {{Text: text}}}}
		for _, finding := range InspectSlide(manifest, layout, slide, design) {
			if finding.Kind == FindingOrphan {
				t.Fatalf("%q should not be reported as an orphan: %s", text, finding.Detail)
			}
		}
	}

	// A bulleted body legitimately ends its lines wherever the words fall.
	body := Placeholder{Slot: SlotBody, Kind: "text", Type: "body", Width: 6000000, Height: 3000000,
		FontSize: 1800, MaxChars: 200, MaxLines: 8, LineEm: 24}
	bodyLayout := Layout{ID: "b", Name: "B", Role: RoleContent, Placeholders: []Placeholder{body}}
	bodySlide := Slide{LayoutID: "b", Fields: map[string][]Paragraph{
		SlotBody: {{Text: "전환 대상 시스템을 세 묶음으로 나누어 순차적으로 이관합니다 그리고 하"}}}}
	for _, finding := range InspectSlide(manifest, bodyLayout, bodySlide, design) {
		if finding.Kind == FindingOrphan {
			t.Fatal("a body region must not be checked for orphans")
		}
	}
}

func TestFittingMovesACutRatherThanLeavingAnOrphan(t *testing.T) {
	title := Placeholder{Slot: SlotTitle, Kind: "text", Type: "title",
		Width: 6000000, Height: 1200000, FontSize: 3200, MaxChars: 14, MaxLines: 2, LineEm: 14}
	// The cut lands mid-phrase and would leave a fragment on its own line.
	fitted, report := FitParagraphsReport(
		[]Paragraph{{Text: "클라우드 전환 로드맵과 투자 타당성 검토"}}, title, "ko")
	if !report.Lost() {
		t.Fatal("a heading longer than its region must be reported as shortened")
	}
	if _, orphaned := orphanedLine(fitted[0].Text, title.LineEm); orphaned {
		t.Fatalf("the cut left an orphan: %q", fitted[0].Text)
	}
	// Nothing was invented, and the cut is marked.
	if !strings.HasSuffix(fitted[0].Text, "…") {
		t.Fatalf("a shortened heading should say so: %q", fitted[0].Text)
	}
	if !strings.HasPrefix("클라우드 전환 로드맵과 투자 타당성 검토", strings.TrimSuffix(fitted[0].Text, "…")) {
		t.Fatalf("the text was rewritten rather than cut: %q", fitted[0].Text)
	}
}

func TestInspectSeparatesUnfinishedFromBroken(t *testing.T) {
	body := Placeholder{Slot: SlotBody, Kind: "text", Type: "body",
		X: 800000, Y: 1800000, Width: 6000000, Height: 3600000,
		FontSize: 1800, MaxChars: 240, MaxLines: 10, LineEm: 24}
	layout := Layout{ID: "content", Name: "Content", Role: RoleContent, Background: "FFFFFF",
		Placeholders: []Placeholder{
			{Slot: SlotTitle, Kind: "text", Type: "title", Width: 8000000, Height: 900000,
				FontSize: 3200, MaxChars: 40, MaxLines: 2, LineEm: 22},
			body,
		}}
	manifest := Manifest{Version: ManifestVersion, SlideWidth: 12192000, SlideHeight: 6858000,
		Theme:   Theme{Colors: map[string]string{"lt1": "FFFFFF", "dk1": "111111", "accent1": "1E6FFF"}},
		Layouts: []Layout{layout}}

	points := make([]Paragraph, 0, 8)
	for index := range 8 {
		points = append(points, Paragraph{Text: fmt.Sprintf("%d번째 요점입니다", index+1)})
	}
	crowded := Deck{Slides: []Slide{{LayoutID: "content", Fields: map[string][]Paragraph{
		SlotTitle: {{Text: "전환 계획"}}, SlotBody: points}}}}

	findings := InspectDeck(manifest, crowded)
	kinds := map[string]bool{}
	for _, finding := range findings {
		kinds[finding.Kind] = true
		if !finding.Advisory {
			t.Fatalf("a crowded slide with no notes is unfinished, not broken: %+v", finding)
		}
	}
	if !kinds[FindingDensity] || !kinds[FindingNotes] {
		t.Fatalf("both the density and the missing notes should be reported: %v", kinds)
	}
	// Nothing is drawn wrong, so the defect list is empty.
	if defects := Defects(findings); len(defects) != 0 {
		t.Fatalf("defects = %+v", defects)
	}

	// Six points with notes is a slide, not a wall.
	fine := Deck{Slides: []Slide{{LayoutID: "content", Notes: "말할 내용",
		Fields: map[string][]Paragraph{SlotTitle: {{Text: "전환 계획"}}, SlotBody: points[:5]}}}}
	if reported := InspectDeck(manifest, fine); len(reported) != 0 {
		t.Fatalf("a reasonable slide must be quiet: %+v", reported)
	}

	// A cover carries the room on its own, so it needs no notes.
	coverLayout := Layout{ID: "cover", Name: "Cover", Role: RoleTitle, Background: "FFFFFF",
		Placeholders: []Placeholder{{Slot: SlotTitle, Kind: "text", Type: "title",
			Width: 8000000, Height: 900000, FontSize: 4000, MaxChars: 40, MaxLines: 2, LineEm: 20}}}
	manifest.Layouts = append(manifest.Layouts, coverLayout)
	cover := Deck{Slides: []Slide{{LayoutID: "cover", Fields: map[string][]Paragraph{SlotTitle: {{Text: "전환 계획"}}}}}}
	if reported := InspectDeck(manifest, cover); len(reported) != 0 {
		t.Fatalf("a cover without notes must not be reported: %+v", reported)
	}
}

// A line chart labels its x-axis by centring a month in the gap between two
// ticks, which gives a two-character label a box seven centimetres wide. Those
// boxes overlap; the labels do not. Measuring boxes made every trend chart
// report a collision and cost it a third of its score.
func TestAChartsAxisLabelsDoNotReadAsACollision(t *testing.T) {
	_, _, manifest := buildTemplate(t, "plum-rail")
	layout, _ := manifest.Layout(manifest.DefaultLayout)
	slide := Slide{LayoutID: layout.ID,
		Fields: map[string][]Paragraph{SlotTitle: {{Text: "월별 처리량"}}},
		Blocks: map[string]Block{SlotBody: {Kind: BlockLine, Heading: "월별 처리량",
			Labels: []string{"1월", "2월", "3월", "4월"},
			Series: []Series{
				{Name: "전환 전", Points: []float64{120, 118, 121, 119}},
				{Name: "전환 후", Points: []float64{120, 132, 148, 165}}}}}}
	for _, finding := range InspectSlide(manifest, layout, slide, NewDesign(manifest)) {
		if finding.Kind == FindingCollision {
			t.Fatalf("the chart reports a collision it does not have: %s", finding.String())
		}
	}

	// Text that really does land on other text is still caught: two labels of
	// the same width in the same place.
	overlapping := []Primitive{
		text(Frame{X: 0, Y: 0, Width: 1000000, Height: 300000}, line("첫 번째 항목"),
			textOptions{Size: 1800, Color: "000000"}),
		text(Frame{X: 100000, Y: 0, Width: 1000000, Height: 300000}, line("두 번째 항목"),
			textOptions{Size: 1800, Color: "000000"}),
	}
	first, second := inkBounds(overlapping[0]), inkBounds(overlapping[1])
	if overlapArea(first, second) <= 0 {
		t.Fatalf("two labels drawn on each other no longer overlap: %v and %v", first, second)
	}
}

// "2026년 상반기" on a cover is when the deck is about. Asking a cover for the
// source of its own date teaches people to ignore the question.
func TestACoverIsNotAskedWhereItsYearCameFrom(t *testing.T) {
	_, _, manifest := buildTemplate(t, "plum-rail")
	cover, _ := manifest.Layout(manifest.TitleLayout)
	content, _ := manifest.Layout(manifest.DefaultLayout)
	deck := Deck{Language: "ko", Slides: []Slide{
		{LayoutID: cover.ID, Fields: map[string][]Paragraph{
			SlotTitle: {{Text: "전환 프로그램 보고"}}, SlotSubtitle: {{Text: "2026년 상반기"}}}},
		{LayoutID: content.ID, Fields: map[string][]Paragraph{
			SlotTitle: {{Text: "매출"}}, SlotBody: {{Text: "2026년 매출은 1,240억"}}}},
	}}
	asked := map[int]bool{}
	for _, finding := range InspectDeck(manifest, deck) {
		if finding.Kind == FindingSource {
			asked[finding.Slide] = true
		}
	}
	if asked[1] {
		t.Fatal("the cover was asked for a source")
	}
	if !asked[2] {
		t.Fatal("a slide stating 1,240억 was not asked where it came from")
	}

	// Nor is a schedule. "첫 2주에 할 일" is when something happens, and a room
	// does not ask where a plan's own timetable came from.
	schedule := Deck{Language: "ko", Slides: []Slide{{LayoutID: content.ID, Fields: map[string][]Paragraph{
		SlotTitle: {{Text: "실행 준비 상태"}},
		SlotBody:  {{Text: "첫 2주에 할 일"}, {Text: "6개월 안에 확인할 지표와 목표"}}}}}}
	for _, finding := range InspectDeck(manifest, schedule) {
		if finding.Kind == FindingSource {
			t.Fatalf("a schedule was read as a claim: %s", finding.String())
		}
	}

	// A year on a content slide is still not a figure on its own.
	dateOnly := Deck{Language: "ko", Slides: []Slide{{LayoutID: content.ID, Fields: map[string][]Paragraph{
		SlotTitle: {{Text: "일정"}}, SlotBody: {{Text: "2026년 상반기에 이관을 마칩니다"}}}}}}
	for _, finding := range InspectDeck(manifest, dateOnly) {
		if finding.Kind == FindingSource {
			t.Fatalf("a date was read as a figure: %s", finding.String())
		}
	}
}

// A deck that asks a board for 12억 원 states that number on every slide about
// the ask, and the author is the source: the brief is where it came from. The
// deck's own writing rule forbids putting a !source on it, so asking for one
// made obeying the rule cost a mark. What the brief never said is a different
// matter — that is what the room asks about.
func TestTheAuthorsOwnFigureIsNotAskedForASource(t *testing.T) {
	_, _, manifest := buildTemplate(t, "plum-rail")
	content, _ := manifest.Layout(manifest.DefaultLayout)
	slide := func(title, body string) Slide {
		return Slide{LayoutID: content.ID, Fields: map[string][]Paragraph{
			SlotTitle: {{Text: title}}, SlotBody: {{Text: body}}}}
	}
	deck := Deck{Language: "ko",
		Brief: "결제 이중화 투자 12억 원을 이사회에 요청. 내부 결제 로그 기준 지난 12개월 장애 2회.",
		Slides: []Slide{
			slide("이중화 투자 계획", "구축 비용 12억 원 요청"),
			slide("시장 성장", "온라인 거래액이 28.5% 늘었습니다"),
		}}
	asked := map[int]string{}
	for _, finding := range InspectDeck(manifest, deck) {
		if finding.Kind == FindingSource {
			asked[finding.Slide] = finding.Detail
		}
	}
	if detail, ok := asked[1]; ok {
		t.Fatalf("the author was asked to cite their own request: %s", detail)
	}
	if !strings.Contains(asked[2], "28.5%") {
		t.Fatalf("the figure the brief never gave was not named: %q", asked[2])
	}

	// With no brief — an imported deck, or one written by hand — every figure is
	// still asked about, because there is nothing to say it came from the author.
	deck.Brief = ""
	unbriefed := 0
	for _, finding := range InspectDeck(manifest, deck) {
		if finding.Kind == FindingSource {
			unbriefed++
		}
	}
	if unbriefed != 2 {
		t.Fatalf("a deck with no brief had %d slides asked, wanted 2", unbriefed)
	}
}

// A brief that says 34 people and 6 leavers has already said 17.6%. Asking
// where that figure came from is asking the author to cite their own
// arithmetic, and the deck said it in a note as well.
func TestAPercentageTheBriefImpliesIsNotInvented(t *testing.T) {
	brief := "지난해 입사자 34명 중 6명이 6개월 내 퇴사했고, 사내 설문 응답 112명 중 47%가 첫 달이 어렵다고 답했습니다."
	read := NewBriefFigures(brief)
	if missing := read.Missing("이탈률 17.6%로 인재 확보 비용이 늘었습니다"); len(missing) > 0 {
		t.Fatalf("a figure the brief divides to was reported as invented: %q", missing)
	}
	if missing := read.Missing("입사자 34명 중 6명"); len(missing) > 0 {
		t.Fatalf("the brief's own numbers were reported: %q", missing)
	}
	// A figure nobody can get to from the brief is still reported, and a small
	// divisor is a coincidence rather than a derivation.
	for _, invented := range []string{"이탈률 50% 감소를 목표로 합니다", "가용성 99.99%를 확보합니다"} {
		if missing := read.Missing(invented); len(missing) == 0 {
			t.Fatalf("an invented figure was let through: %q", invented)
		}
	}
	// Only percentages are read this way: a count that happens to equal a ratio
	// is not a derivation.
	if missing := read.Missing("총 18건이 접수되었습니다"); len(missing) == 0 {
		t.Fatal("a count the brief never gave was let through")
	}
}

// A component draws its own text, and nothing was checking that the text fits
// sideways: a figure wider than its box is painted over whatever is beside it
// and the room reads half a number. The check that was written first could
// never fire — it asked inkBounds how far the text ran past its box, and
// inkBounds only ever narrows a box to its ink.
func TestTextWiderThanItsOwnBoxIsMeasured(t *testing.T) {
	_, _, manifest := buildTemplate(t, "plum-rail")
	design := NewDesign(manifest)
	layout, _ := manifest.Layout(manifest.DefaultLayout)
	body, _ := layout.Slot(SlotBody)
	narrow := Frame{X: body.X, Y: body.Y, Width: 4200000, Height: 2400000}
	long := "지식관리 시스템 교체에 따른 이관 일정과 담당"

	wide := func(block Block) string {
		for _, finding := range inspectComponent(body, narrow, block, design, manifest.SlideWidth, manifest.SlideHeight) {
			if strings.Contains(finding.Detail, "wider than") {
				return finding.Detail
			}
		}
		return ""
	}
	// A label nobody can shrink for the author is reported, so the repair pass
	// can ask for a shorter one.
	if detail := wide(Block{Kind: BlockMeter, Items: []Item{{Label: long, Value: "72%"}}}); detail == "" {
		t.Fatal("a label drawn past the side of its own box was not reported")
	}
	// A figure gives way to the room it is in instead, so these are quiet.
	for _, block := range []Block{
		{Kind: BlockHero, Items: []Item{{Label: "총 비용", Value: "1억 5천만 원"}}},
		{Kind: BlockKPI, Items: []Item{
			{Label: "총 비용", Value: "1억 5천만 원"},
			{Label: "이관 기간", Value: "4개월"},
			{Label: "문서 건수", Value: "12,400건"}}},
	} {
		if detail := wide(block); detail != "" {
			t.Fatalf("%s: %s", block.Kind, detail)
		}
	}
}

// A brief that says "세 개 시스템" has said 3, and a deck that writes "3개" is
// quoting it. Counted in words the number is still a number: the deck was told
// it had invented a figure the author had given it.
func TestANumberWrittenInWordsIsStillANumber(t *testing.T) {
	read := NewBriefFigures("현재 세 개 시스템에서 연 18,000건을 처리하고 있고, 대안은 두 가지입니다.")
	for _, quoted := range []string{"3개 시스템을 하나로", "2가지 대안을 비교", "18,000건"} {
		if missing := read.Missing(quoted); len(missing) > 0 {
			t.Fatalf("%q quotes the brief and was reported as invented: %q", quoted, missing)
		}
	}
	if missing := read.Missing("7개 부서가 참여"); len(missing) == 0 {
		t.Fatal("a count the brief never gave was let through")
	}
	// The counter is required: "한" alone is the first syllable of half the words
	// in Korean, and a brief about 한국 has not said 1.
	if read := NewBriefFigures("한국 시장 진출 계획"); len(read.numbers) > 0 {
		t.Fatalf("a word beginning with 한 was read as a number: %v", read.numbers)
	}
}
