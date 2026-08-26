package generation

import "testing"

// A headline names what was counted.
//
// The rule that drops the sentence's subject — "비중은 직판" is a bar called
// 직판 — assumed that what follows the subject is the thing counted. When what
// follows is only how the amount is measured, dropping the subject leaves the
// measure alone: a brief saying the Oracle licence costs 4억 a year produced a
// KPI card reading "연 · 4억", which names nothing.
func TestAHeadlineNamesWhatWasCounted(t *testing.T) {
	for _, want := range []struct{ label, shown string }{
		{"라이선스가 연", "라이선스"},
		{"오라클 라이선스가 연", "오라클 라이선스"},
		{"매출이 총", "매출"},
		{"작년 지출은", "작년 지출"},
		// The subject is still dropped when something was actually counted.
		{"비중은 직판", "직판"},
		{"달성률은 매출", "매출"},
		// And a label that was already a name is left alone.
		{"개발 속도", "개발 속도"},
		{"리포팅 DB", "리포팅 DB"},
		{"총 투자", "총 투자"},
	} {
		if shown := chartLabel(reading{label: want.label}); shown != want.shown {
			t.Errorf("label %q is shown as %q, want %q", want.label, shown, want.shown)
		}
	}
}

// The whole way through: the brief says it, the deck shows it under a name.
func TestTheBriefsFigureReachesTheDeckWithAName(t *testing.T) {
	outline := outlinePrompt(
		"사내 PostgreSQL 전환 타당성 검토 결과를 경영진에게 보고해줘. 현재 오라클 라이선스가 연 4억이고, "+
			"전환 대상은 리포팅 DB 12개, 목표는 2026년 3분기 완료야.", "", koreanCopy)
	labels := map[string]string{}
	for _, figure := range outline.Figures {
		labels[figure.Value] = chartLabel(reading{label: figure.Label})
	}
	if labels["4억"] != "오라클 라이선스" {
		t.Errorf("4억 is headlined as %q, want 오라클 라이선스", labels["4억"])
	}
	if labels["12개"] != "리포팅 DB" {
		t.Errorf("12개 is headlined as %q, want 리포팅 DB", labels["12개"])
	}
}
