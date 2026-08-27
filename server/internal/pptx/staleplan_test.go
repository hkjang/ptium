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
