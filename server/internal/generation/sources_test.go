package generation

import (
	"strings"
	"testing"
)

// Told to cite only what the brief attributes and to invent nothing, the model
// wrote three sources for a brief that names none: "내부 시장 보고서 | 2026-08"
// and two more, dated this month. Printed at the foot of a slide, those read as
// evidence — and are the one thing on the deck that will not survive being
// asked about.
func TestAnInventedSourceIsDropped(t *testing.T) {
	brief := "신규 결제 수단 도입을 검토하는 실무 논의 자료. 도입 시 기대 효과와 리스크를 정리."
	source := "# 도입 검토\n- 한 줄\n!source 내부 시장 보고서 | 2026-08\n\n" +
		"# 기대 효과\n- 다른 줄\n!source 고객 경험 분석 데이터 | 2026-07\n"
	cleaned, dropped, _ := keepAttributedSources(source, brief)
	if dropped != 2 {
		t.Fatalf("dropped %d of the two invented sources:\n%s", dropped, cleaned)
	}
	if strings.Contains(cleaned, "!source") {
		t.Fatalf("an invented source survived:\n%s", cleaned)
	}
	// Everything else is untouched.
	for _, wanted := range []string{"# 도입 검토", "- 한 줄", "# 기대 효과", "- 다른 줄"} {
		if !strings.Contains(cleaned, wanted) {
			t.Fatalf("the deck lost %q:\n%s", wanted, cleaned)
		}
	}
}

// A citation the brief does attribute stays, even when the model rewords it:
// "통계청 2026 소비 동향" for a brief that says "통계청 2026 소비 동향(표 3)" is
// citing the brief.
func TestASourceTheBriefAttributesIsKept(t *testing.T) {
	brief := "통계청 2026 소비 동향(표 3) 기준 온라인 거래액 28.5% 증가, 내부 결제 로그 기준 장애 2회."
	source := "# 성장\n- 한 줄\n!source 통계청 2026 소비 동향 | 표 3\n\n" +
		"# 장애\n- 다른 줄\n!source 결제 로그 | 2026-08\n"
	cleaned, dropped, _ := keepAttributedSources(source, brief)
	if dropped != 0 {
		t.Fatalf("dropped %d attributed source(s):\n%s", dropped, cleaned)
	}
	if strings.Count(cleaned, "!source") != 2 {
		t.Fatalf("a cited source went missing:\n%s", cleaned)
	}
}

// A source made only of the words every source has cites nothing.
func TestASourceOfOnlyGenericWordsIsDropped(t *testing.T) {
	cleaned, dropped, _ := keepAttributedSources("# 제목\n!source 내부 자료\n", "무엇이든 적힌 브리프")
	if dropped != 1 || strings.Contains(cleaned, "!source") {
		t.Fatalf("dropped=%d\n%s", dropped, cleaned)
	}
}

// And a deck with no citations at all is returned exactly as it came.
func TestADeckWithoutSourcesIsUntouched(t *testing.T) {
	source := "# 제목\n- 한 줄\n"
	cleaned, dropped, _ := keepAttributedSources(source, "브리프")
	if dropped != 0 || cleaned != source {
		t.Fatalf("dropped=%d cleaned=%q", dropped, cleaned)
	}
}

// The name of a source is the harder thing to invent, so a model told to cite
// the brief mostly gets it right — and then makes up where in it. This deck is
// what the model actually wrote for a brief that says "내부 결제 로그 기준 지난
// 12개월": a real system, a month nobody named.
func TestAMadeUpLocatorGoesAndTheSourceStays(t *testing.T) {
	brief := "통계청 2026 소비 동향(표 3) 기준 온라인 거래액 28.5% 증가, 내부 결제 로그 기준 지난 12개월 장애 2회."
	source := strings.Join([]string{
		"# 시장 성장과 시스템 부하",
		"- 온라인 거래액 28.5% 증가",
		"!source 통계청 2026 소비 동향 | 표 3",
		"# 내부 결제 로그 분석",
		"- 지난 12개월 장애 2회",
		"!source 내부 결제 로그 | 2026-03",
	}, "\n")
	cleaned, dropped, vague := keepAttributedSources(source, brief)
	if dropped != 0 || vague != 1 {
		t.Fatalf("dropped=%d vague=%d, wanted one trimmed locator", dropped, vague)
	}
	if !strings.Contains(cleaned, "!source 통계청 2026 소비 동향 | 표 3") {
		t.Fatalf("the locator the brief gives was trimmed: %s", cleaned)
	}
	if !strings.Contains(cleaned, "!source 내부 결제 로그\n") && !strings.HasSuffix(cleaned, "!source 내부 결제 로그") {
		t.Fatalf("the source name did not survive without its locator: %s", cleaned)
	}
	if strings.Contains(cleaned, "2026-03") {
		t.Fatalf("the invented month is still cited: %s", cleaned)
	}
}

func TestALocatorTheBriefGivesIsKept(t *testing.T) {
	brief := "고객 만족도 조사 2026 상반기 12페이지 기준, 재구매 의향 64%."
	for _, line := range []string{
		"!source 고객 만족도 조사 | 12페이지",
		"!source 고객 만족도 조사 | 2026 상반기",
		"!source 고객 만족도 조사",
	} {
		cleaned, dropped, vague := keepAttributedSources("# 제목\n"+line+"\n", brief)
		if dropped != 0 || vague != 0 {
			t.Fatalf("%q was reported as invented: dropped=%d vague=%d", line, dropped, vague)
		}
		if !strings.Contains(cleaned, line) {
			t.Fatalf("%q was changed: %s", line, cleaned)
		}
	}
}

func TestTheLocatorNoteSaysWhatWasKept(t *testing.T) {
	note := vagueLocatorNote(2, "ko")
	for _, wanted := range []string{"2건", "출처 이름만 남겼습니다"} {
		if !strings.Contains(note, wanted) {
			t.Fatalf("the note does not say %q: %s", wanted, note)
		}
	}
	if english := vagueLocatorNote(1, "en"); !strings.Contains(english, "the source names remain") {
		t.Fatalf("the English note does not say what was kept: %s", english)
	}
}

// A deck built from an attached sheet cites where on the sheet it came from,
// and Ptium writes that locator itself. It is a coordinate into the
// attachment, not a claim about what the attachment says, so the brief has no
// reason to contain it — and trimming it would delete something true.
func TestASpreadsheetRangeIsNotAnInvention(t *testing.T) {
	for _, line := range []string{
		"!source 매출-415626.csv | A1:B4",
		"!source 매출 현황 | Sheet1!A1:C9",
		"!source 매출-415626.csv | B2",
	} {
		cleaned, dropped, vague := keepAttributedSources("# 지난해 실적\n"+line+"\n", "매출-415626.csv에서 가져온 자료, 매출 현황")
		if dropped != 0 || vague != 0 {
			t.Fatalf("%q was treated as invented: dropped=%d vague=%d", line, dropped, vague)
		}
		if !strings.Contains(cleaned, line) {
			t.Fatalf("%q was changed: %s", line, cleaned)
		}
	}
}
