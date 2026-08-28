package pptx

import (
	"strings"
	"testing"
	"time"
)

func atDate(t *testing.T, when time.Time) {
	t.Helper()
	previous := now
	now = func() time.Time { return when }
	t.Cleanup(func() { now = previous })
}

func slideSaying(lines ...string) Slide {
	paragraphs := make([]Paragraph, 0, len(lines))
	for _, line := range lines {
		paragraphs = append(paragraphs, Paragraph{Text: line})
	}
	return Slide{Fields: map[string][]Paragraph{"body": paragraphs}}
}

// A plan whose first step is behind the room reading it.
//
// A model writing a roadmap has no clock. Told the target was 2026 Q3, and told
// what today was, one still wrote "2024년 4분기 PoC 시작을 위한 즉시 조치 필요" —
// two years behind the people it was written for. Nothing measured it, so the
// deck went out saying it.
func TestAPlanDatedBeforeTodayIsReported(t *testing.T) {
	atDate(t, time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC))
	for _, line := range []string{
		"2024년 4분기 PoC 시작을 위한 즉시 조치 필요",
		"24년 4분기 착수 예정",
		"2025 Q1 kick-off for the migration",
		"2025년까지 단계별 전환 추진",
	} {
		if found := stalePlans(slideSaying(line)); len(found) == 0 {
			t.Errorf("a plan dated in the past was not reported: %q", line)
		} else if !strings.Contains(found[0].Detail, "already past") {
			t.Errorf("the finding does not say what is wrong: %q", found[0].Detail)
		}
	}
}

// A roadmap says when without ever saying "will": two or more lines opening with
// a date make the slide a schedule, and then the dates speak for themselves.
func TestARoadmapEntryNeedsNoVerb(t *testing.T) {
	atDate(t, time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC))
	roadmap := slideSaying(
		"Q4 2024: PoC 수행 및 3개 DB 전환",
		"2025년 2분기: 2단계 DB 전환",
		"2026년 3분기: 전 시스템 전환",
	)
	if found := stalePlans(roadmap); len(found) == 0 {
		t.Error("a roadmap starting two years ago was not reported")
	}
	// One dated line is not a schedule: a results slide heads its figure with a
	// year and means nothing by it.
	results := slideSaying("2024년: 매출 120억", "성장률은 시장 평균을 웃돌았습니다")
	if found := stalePlans(results); len(found) != 0 {
		t.Errorf("a single dated line was read as a schedule: %q", found[0].Detail)
	}
}

// The rule is narrow on purpose: a deck about last year is a deck about last
// year, and every one of these lines belongs on a slide somewhere.
func TestARecordOfThePastIsNotAStalePlan(t *testing.T) {
	atDate(t, time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC))
	for _, line := range []string{
		"2024년 매출은 120억이었습니다",      // a fact, with no plan in it
		"2025년 4분기에 전환을 완료했습니다",    // it already happened
		"지난 2024년 착수한 과제의 결과입니다",   // 지난 · 착수한, a record
		"2026년 3분기 전환 완료를 목표로 합니다", // this year, still ahead
		"2027년 1분기 착수 예정",          // next year
		"12년 경력의 데이터 엔지니어가 참여합니다",  // a quantity, not a year
		"AI 도입 효과를 2027년까지 추진합니다",  // ahead
	} {
		if found := stalePlans(slideSaying(line)); len(found) != 0 {
			t.Errorf("an ordinary line was called a stale plan: %q → %q", line, found[0].Detail)
		}
	}
}

// A step and its date are one line to anybody looking at the slide, and were
// walked as the two strings they are stored as: the words that make a line a
// plan sat in one and the date in the other, so a roadmap written with this
// product's own steps component could not be read as a plan at all.
//
// And a plan is late by the month it names, not only by its year. A roadmap
// written in August that opens on March is five months behind the room, and
// its year says nothing is wrong.
func TestARoadmapWrittenAsStepsIsReadAsAPlan(t *testing.T) {
	atDate(t, time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC))

	steps := func(kind string, items ...Item) Slide {
		return Slide{Fields: map[string][]Paragraph{SlotTitle: {{Text: "이행 계획"}}},
			Blocks: map[string]Block{SlotBody: {Kind: kind, Items: items}}}
	}
	said := func(slide Slide) string {
		for _, finding := range stalePlans(slide) {
			return finding.Detail
		}
		return ""
	}

	if detail := said(steps(BlockSteps,
		Item{Label: "설계", Value: "2026년 3월까지"},
		Item{Label: "설치", Value: "2026년 12월까지"})); !strings.Contains(detail, "2026-03") {
		t.Errorf("a step dated five months back was not reported: %q", detail)
	}
	// A quarter is late once its last month is.
	if detail := said(steps(BlockSteps,
		Item{Label: "설계", Value: "2026년 1분기"},
		Item{Label: "설치", Value: "2026년 4분기"})); !strings.Contains(detail, "2026-03") {
		t.Errorf("a step dated to a quarter already over was not reported: %q", detail)
	}
	// A timeline is the same thing under another name.
	if detail := said(steps(BlockTimeline,
		Item{Label: "2026년 2월", Value: "착수"},
		Item{Label: "2026년 11월", Value: "완료"})); !strings.Contains(detail, "2026-02") {
		t.Errorf("a timeline entry already past was not reported: %q", detail)
	}
	// What is still ahead is not late.
	if detail := said(steps(BlockSteps,
		Item{Label: "설계", Value: "2026년 12월까지"},
		Item{Label: "안정화", Value: "2027년 3월까지"})); detail != "" {
		t.Errorf("a plan still ahead of the room was called late: %q", detail)
	}
	// Neither is a record of something done.
	if detail := said(steps(BlockSteps,
		Item{Label: "설계", Value: "2026년 3월에 완료했습니다"},
		Item{Label: "설치", Value: "2026년 5월에 마쳤습니다"})); detail != "" {
		t.Errorf("work already done was called a late plan: %q", detail)
	}
	// A component that carries no date says nothing either way.
	if detail := said(steps(BlockSteps,
		Item{Label: "설계", Value: "먼저"}, Item{Label: "설치", Value: "다음"})); detail != "" {
		t.Errorf("a plan with no date in it was called late: %q", detail)
	}
}

// Korean fuses the past marker into the verb's own syllable, so a line can be
// unmistakably about something already done without containing 했, 였 or 었.
func TestThePastTenseIsReadWhereverItFused(t *testing.T) {
	for _, line := range []string{
		"2026년 5월에 마쳤습니다", "2026년 3월에 끝냈습니다", "2026년 2월에 됐습니다",
		"2026년 1월에 나왔습니다", "2026년 4월에 갔습니다", "2026년 6월에 했습니다",
		"작년에 완료된 일", "finished in March",
	} {
		if !saysItHappened(line) {
			t.Errorf("%q is written in the past and was not read as such", line)
		}
	}
	for _, line := range []string{
		"2026년 12월까지 마칩니다", "2026년 9월 착수 예정", "계획이 있습니다",
		"관련 자료가 없습니다", "설계", "2027년 3월까지",
	} {
		if saysItHappened(line) {
			t.Errorf("%q is not in the past and was read as though it were", line)
		}
	}
}
