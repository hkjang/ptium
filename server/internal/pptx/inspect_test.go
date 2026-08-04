package pptx

import (
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
	deck.Slides = append(deck.Slides, Slide{LayoutID: cover.ID, Fields: map[string][]Paragraph{
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
		deck.Slides = append(deck.Slides, Slide{LayoutID: content.ID,
			Fields: map[string][]Paragraph{SlotTitle: line("전환 계획 상세")},
			Blocks: map[string]Block{bodySlot(content): block}})
	}
	deck.Slides = append(deck.Slides, Slide{LayoutID: closing.ID, Fields: map[string][]Paragraph{
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
		if findings := InspectDeck(manifest, deck); len(findings) > 0 {
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
