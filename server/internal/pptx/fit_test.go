package pptx

import (
	"strings"
	"testing"
)

// A component's value is cut between words, and the cut is visible. The
// deterministic generator writes "…을 단계별로 적용" for a three-step plan; a hard
// cut at 24 characters put "…을 단계별로 적" on the slide — half of 적용, with
// nothing to say anything had been dropped.
func TestAComponentValueIsNotCutThroughAWord(t *testing.T) {
	steps, ok := SanitizeBlock(Block{Kind: BlockSteps, Items: []Item{
		{Label: "준비", Value: "범위 · 조직 · 예산을 확정"},
		{Label: "이행", Value: "국내 결제 시스템 이중화 계획을 단계별로 적용"},
		{Label: "안정화", Value: "운영 이관과 점검 기준 확정"},
	}}, Placeholder{Width: 6000000, Height: 3000000, FontSize: 1800, MaxChars: 200, MaxLines: 8})
	if !ok {
		t.Fatal("the process was rejected")
	}
	if got := steps.Items[1].Value; got != "국내 결제 시스템 이중화 계획을 단계별로 적용" {
		t.Fatalf("a phrase that fits was changed: %q", got)
	}

	// Something genuinely too long is cut where a word ends and says so.
	long, _ := SanitizeBlock(Block{Kind: BlockSteps, Items: []Item{
		{Label: "이행", Value: "국내 결제 시스템 이중화 계획을 단계별로 적용하고 운영 이관까지 마친 뒤 점검 기준을 확정하며 각 단계의 완료 조건을 문서로 남깁니다"},
		{Label: "준비", Value: "범위 확정"},
	}}, Placeholder{Width: 6000000, Height: 3000000, FontSize: 1800, MaxChars: 200, MaxLines: 8})
	cut := long.Items[0].Value
	if !strings.HasSuffix(cut, "…") {
		t.Fatalf("a cut value does not say it was cut: %q", cut)
	}
	if strings.HasSuffix(strings.TrimSuffix(cut, "…"), " ") {
		t.Fatalf("the cut left a trailing space: %q", cut)
	}
	for _, word := range strings.Fields(strings.TrimSuffix(cut, "…")) {
		if !strings.Contains("국내 결제 시스템 이중화 계획을 단계별로 적용하고 운영 이관까지 마친 뒤 점검 기준을 확정하며 각 단계의 완료 조건을 문서로 남깁니다", word) {
			t.Fatalf("the cut invented or split a word: %q in %q", word, cut)
		}
	}

	// A figure keeps its own tighter limit: a KPI value is a number and a unit.
	kpi, _ := SanitizeBlock(Block{Kind: BlockKPI, Items: []Item{
		{Label: "매출", Value: "1,240억"}, {Label: "신규 고객", Value: "312곳"}}},
		Placeholder{Width: 6000000, Height: 3000000, FontSize: 1800, MaxChars: 200, MaxLines: 8})
	if kpi.Items[0].Value != "1,240억" {
		t.Fatalf("a figure was changed: %q", kpi.Items[0].Value)
	}
}
