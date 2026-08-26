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
