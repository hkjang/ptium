package pptx

import "testing"

// A heading is the one line everybody reads. Six decks measured at 98 to 100
// while headed "…개선하려고", "회사 소개 어줘" and "AI 챗봇 도입 검토 서": every
// measurement was about the drawing, and none about whether the words were
// words.
func TestAHeadingThatStopsMidSentenceIsMeasured(t *testing.T) {
	unfinished := []string{
		"협력사 정산 프로세스를 개선하려고",
		"새로 만들려고 하는데",
		"실행 계획을 하려고",
		"회사 소개 자료 만들어줘",
		"고객 이탈률 개선을 위한",
		"2026년 계획을",
		"품질 관리 체계를 새로 만들",
		"Plan for the",
		"Migration from on-premise to",
	}
	for _, heading := range unfinished {
		if !unfinishedHeading(heading) {
			t.Errorf("%q was measured as a finished heading", heading)
		}
	}
	finished := []string{
		"협력사 정산 프로세스 개선",
		"고객 이탈률 개선을 위한 리텐션 전략",
		"결정이 필요한 사항",
		"다음 단계",
		"매출이 12% 늘었습니다",
		"AI 챗봇 도입 검토 보고서",
		"재고 회전율",
		"Payment platform migration",
		"Decisions required",
		"Q3 results and Q4 outlook",
		// Twenty-six decks imported from real files produced two complaints, and
		// both were this slide.
		"Q & A",
		"Q&A",
		"Plan A",
		"매출을", // one word, badly named but not cut off
	}
	for _, heading := range finished {
		if unfinishedHeading(heading) {
			t.Errorf("%q was measured as unfinished", heading)
		}
	}
}

// It reaches the deck's measurement, where the panel and the score read it.
func TestAnUnfinishedHeadingReachesTheScore(t *testing.T) {
	data, err := BuiltinTemplate("plum-rail")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	layout, ok := manifest.LayoutForRole(RoleContent)
	if !ok {
		t.Fatal("the template has no content layout")
	}
	slide := Slide{LayoutID: layout.ID, Fields: map[string][]Paragraph{
		SlotTitle: {{Text: "협력사 정산 프로세스를 개선하려고"}},
	}}
	var findings []Finding
	for _, finding := range InspectDeck(manifest, Deck{Slides: []Slide{slide}}) {
		if finding.Kind == FindingUnfinished {
			findings = append(findings, finding)
		}
	}
	if len(findings) != 1 {
		t.Fatalf("the deck's own measurement did not report it: %#v", findings)
	}
	if !findings[0].Advisory {
		t.Error("a heading is about the writing, which is an advisory")
	}
	if weight := weightOf(findings[0]); weight < 10 {
		t.Errorf("the line the room reads first is worth %d", weight)
	}
}

// The same measure at two values is a comparison, which is what a comparison
// slide is made of. A live model wrote a table of before-and-after figures and
// the measurement called three of its rows repetitions — telling whoever acted
// on it to delete half the table.
func TestTheSameLabelWithADifferentNumberIsAComparison(t *testing.T) {
	comparisons := [][2]string{
		{"과잉 재고 품목 비율: 12%", "과잉 재고 품목 비율: 24% 증가"},
		{"투자 대비 수익률 (ROI): 0%", "투자 대비 수익률 (ROI): 300% 예상"},
		{"재고 부패/폐기 비용: +30% 증가", "재고 부패/폐기 비용: 기준선"},
	}
	for _, pair := range comparisons {
		slide := Slide{Fields: map[string][]Paragraph{
			SlotBody: {{Text: pair[0]}, {Text: pair[1]}},
		}}
		for _, finding := range repeatedPoints(slide) {
			t.Errorf("%q beside %q was called a repetition: %s", pair[0], pair[1], finding.Detail)
		}
	}
	// Saying the same thing twice in different words is still saying it twice.
	slide := Slide{Fields: map[string][]Paragraph{
		SlotBody: {
			{Text: "협업 도구를 도입하면 팀의 커뮤니케이션 비용이 크게 줄어듭니다"},
			{Text: "협업 도구 도입으로 팀 커뮤니케이션 비용이 크게 줄어듭니다"},
		},
	}}
	if len(repeatedPoints(slide)) == 0 {
		t.Error("the same point made twice was not reported")
	}
}

// Two facts that happen to share a value are two facts. An invoice read into a
// deck had "Date due August 20, 2026" and "Date of issue August 20, 2026"
// reported as one point said twice — and the fix for that is to delete a date.
func TestTwoFactsSharingAValueAreNotOnePointTwice(t *testing.T) {
	slide := Slide{Fields: map[string][]Paragraph{
		SlotBody: {{Text: "Date due August 20, 2026"}, {Text: "Date of issue August 20, 2026"}},
	}}
	for _, finding := range repeatedPoints(slide) {
		t.Errorf("two dates were called one point twice: %s", finding.Detail)
	}
	// A line repeated with a label in front of it is still a repetition: what
	// they share is the words, not a value.
	same := Slide{Fields: map[string][]Paragraph{
		SlotBody: {{Text: "Trello: 보드 기반 작업 관리 도구"}, {Text: "보드 기반 작업 관리 도구"}},
	}}
	if len(repeatedPoints(same)) == 0 {
		t.Error("a line repeated under a label was not reported")
	}
}

// A citation is written twice on the way out — drawn on the slide and repeated
// under the notes — so reading both back gives the deck the same source twice.
// The note this product's own offline writer puts on such a slide is "숫자는
// 출처와 함께 말합니다", which contains the word the repetition is introduced
// with, and stopping at the first mention left the citation in the notes: one
// more copy on every trip out and back.
func TestACitationIsNotReadBackTwice(t *testing.T) {
	sources := []string{"2026 시장 조사 보고서, p.42"}
	cases := map[string]string{
		"숫자는 출처와 함께 말합니다 출처 2026 시장 조사 보고서 — p.42": "숫자는 출처와 함께 말합니다",
		"출처 2026 시장 조사 보고서 — p.42":                 "",
		"숫자는 출처와 함께 말합니다":                          "숫자는 출처와 함께 말합니다",
		// An author who ends a note with the word and something else keeps it.
		"출처 확인이 필요합니다": "출처 확인이 필요합니다",
	}
	for notes, want := range cases {
		if got := withoutRepeatedCitations(notes, sources); got != want {
			t.Errorf("withoutRepeatedCitations(%q) = %q, want %q", notes, got, want)
		}
	}
	// With nothing cited, the notes are the notes.
	if got := withoutRepeatedCitations("출처 2026 시장 조사 보고서", nil); got != "출처 2026 시장 조사 보고서" {
		t.Errorf("a note was cut with no citation to match: %q", got)
	}
}

// Two kinds of slide have no layout type of their own in PowerPoint: a closing
// page is built on a section layout and a quotation on an ordinary content one.
// Reading the type first turned a deck's own closing page into a section
// divider and its quotation into a bullet — on the way back in from a file
// Ptium itself had written.
func TestAClosingPageAndAQuotationKeepTheirKind(t *testing.T) {
	cases := []struct {
		layoutType string
		name       string
		want       string
	}{
		{"secHead", "마무리", RoleClosing},
		{"secHead", "Closing", RoleClosing},
		{"obj", "핵심 인용", RoleQuote},
		{"obj", "Quote", RoleQuote},
		// What the type does say is still read first.
		{"secHead", "구역 머리글", RoleSection},
		{"title", "제목 슬라이드", RoleTitle},
		{"obj", "제목 및 내용", RoleContent},
		{"twoObj", "콘텐츠 2개", RoleTwoContent},
		{"picTx", "캡션 있는 그림", RolePicture},
		{"blank", "빈 화면", RoleBlank},
		// A title-only layout is what somebody reaches for before drawing their
		// own thing, and what it is depends on what it holds.
		{"titleOnly", "제목만", ""},
	}
	for _, one := range cases {
		if got := roleForLayoutType(one.layoutType, one.name); got != one.want {
			t.Errorf("roleForLayoutType(%q, %q) = %q, want %q", one.layoutType, one.name, got, one.want)
		}
	}
}

// A colour somebody chose has to reach the drawing.
//
// Every generated slide carried the author's brand colour, the profile screen
// said it would be used, and nothing read it: the value was computed, stored on
// every slide, and thrown away at the moment of drawing.
func TestASlidesOwnAccentColoursItsComponents(t *testing.T) {
	data, err := BuiltinTemplate("plum-rail")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	design := NewDesign(manifest)
	// Without one, the template's own accent is what it always was.
	if got := (Slide{}).withAccent(design); got.Accent != design.Accent {
		t.Errorf("a slide with no colour of its own changed the design to %q", got.Accent)
	}
	chosen := (Slide{Accent: "#0f62fe"}).withAccent(design)
	if chosen.Accent != "#0F62FE" {
		t.Errorf("the slide's colour reached the drawing as %q", chosen.Accent)
	}
	if chosen.OnAccent == "" || chosen.OnAccent == design.OnAccent && design.Accent != "#0F62FE" {
		t.Errorf("what is written on the accent was not recomputed: %q", chosen.OnAccent)
	}
	// The rest of the design — the template's own — is untouched.
	if chosen.Surface != design.Surface || chosen.InkPrimary != design.InkPrimary ||
		chosen.Major != design.Major || chosen.Minor != design.Minor {
		t.Error("a brand colour changed something the template decided")
	}
	// Anything that is not a colour is ignored rather than drawn.
	for _, said := range []string{"", "blue", "#12345", "#12345g", "0F62FE"} {
		if got := (Slide{Accent: said}).withAccent(design); got.Accent != design.Accent {
			t.Errorf("%q was taken for a colour", said)
		}
	}
}
