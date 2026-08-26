package library

import "testing"

// A model told to write the agreed slide's title exactly writes
// "1. 회사 소개 및 기술적 우위" — it numbers, and it elaborates. The agreed slide
// stayed on the shelf because the name was less than half of what was written,
// and the company's approved page never made it into the deck.
func TestASlideDressedUpIsStillTheAgreedSlide(t *testing.T) {
	entries := []Entry{{Name: "회사 소개"}, {Name: "보안 아키텍처"}}
	for _, title := range []string{
		"1. 회사 소개 및 기술적 우위",
		"회사 소개 (2026)",
		"제2장 회사 소개 — 연혁과 조직",
		"회사소개",
	} {
		entry, ok := Match(title, entries)
		if !ok || entry.Name != "회사 소개" {
			t.Errorf("%q did not find the agreed slide: %v %q", title, ok, entry.Name)
		}
	}
}

// A short, ordinary word is not a name: an entry called "계획" must not swallow
// every slide about a plan.
func TestAnOrdinaryWordDoesNotSwallowEveryTitle(t *testing.T) {
	entries := []Entry{{Name: "계획"}, {Name: "일정"}}
	for _, title := range []string{"2026년 계획과 예산", "일정과 담당", "이행 계획 상세"} {
		if entry, ok := Match(title, entries); ok {
			t.Errorf("%q was answered with %q", title, entry.Name)
		}
	}
}

// Two entries that both appear in a title mean neither: a wrong substitution is
// worse than none.
func TestAnAmbiguousTitleSubstitutesNothing(t *testing.T) {
	entries := []Entry{{Name: "회사 소개"}, {Name: "회사 소개 상세"}}
	if entry, ok := Match("회사 소개 상세 및 연혁", entries); ok && entry.Name == "회사 소개" {
		t.Errorf("an ambiguous title chose %q", entry.Name)
	}
}
