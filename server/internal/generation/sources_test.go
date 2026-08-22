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
	cleaned, dropped := keepAttributedSources(source, brief)
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
	cleaned, dropped := keepAttributedSources(source, brief)
	if dropped != 0 {
		t.Fatalf("dropped %d attributed source(s):\n%s", dropped, cleaned)
	}
	if strings.Count(cleaned, "!source") != 2 {
		t.Fatalf("a cited source went missing:\n%s", cleaned)
	}
}

// A source made only of the words every source has cites nothing.
func TestASourceOfOnlyGenericWordsIsDropped(t *testing.T) {
	cleaned, dropped := keepAttributedSources("# 제목\n!source 내부 자료\n", "무엇이든 적힌 브리프")
	if dropped != 1 || strings.Contains(cleaned, "!source") {
		t.Fatalf("dropped=%d\n%s", dropped, cleaned)
	}
}

// And a deck with no citations at all is returned exactly as it came.
func TestADeckWithoutSourcesIsUntouched(t *testing.T) {
	source := "# 제목\n- 한 줄\n"
	cleaned, dropped := keepAttributedSources(source, "브리프")
	if dropped != 0 || cleaned != source {
		t.Fatalf("dropped=%d cleaned=%q", dropped, cleaned)
	}
}
