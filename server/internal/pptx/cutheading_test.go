package pptx

import "testing"

// A heading that stops before its verb finishes is reported.
//
// Six ordinary prose briefs — the kind somebody types rather than dictates —
// produced covers reading "우리 회사는 내년에 클라우드 비용을 크게 줄여야" and
// "데이터 거버넌스 체계를 세우", and every one of those decks measured 100 with
// nothing said. The check for a heading that stops mid-sentence existed; it did
// not know these two shapes.
func TestAHeadingCutBeforeItsVerbIsReported(t *testing.T) {
	for _, heading := range []string{
		"우리 회사는 내년에 클라우드 비용을 크게 줄여야",
		"보안 점검에서 지적된 사항들을 먼저 정해야",
		"데이터 거버넌스 체계를 세우",
		"리포팅 지연을 절반으로 줄여",
		"운영 비용을 더 낮춰",
	} {
		if !unfinishedHeading(heading) {
			t.Errorf("a heading cut before its verb finished was not reported: %q", heading)
		}
	}
}

// The same in English: a word whose job is to grade what comes next, left
// holding nothing. "Onboarding for new engineers is taking far too" was the
// cover of a deck written from an ordinary English brief, and nothing said so.
func TestAnEnglishHeadingLeftOnAGradingWordIsReported(t *testing.T) {
	for _, heading := range []string{
		"Onboarding for new engineers is taking far too",
		"The backlog is growing much",
		"Migration risk is rather",
	} {
		if !unfinishedHeading(heading) {
			t.Errorf("a heading left on a grading word was not reported: %q", heading)
		}
	}
}

// And a heading that simply ends in one of those syllables is left alone: 분야
// and 시야 are words, not verbs somebody cut.
func TestAnOrdinaryHeadingIsNotCalledCut(t *testing.T) {
	for _, heading := range []string{
		"AI 분야 투자 계획",
		"시야 확보 방안",
		"보안 점검에서 지적된 사항들",
		"레거시 오라클을 계속 쓸지 옮길지 결정해야 하는 시점",
		"사내 PostgreSQL 전환 타당성 검토 결과",
		"전사 데이터 거버넌스 체계 수립 방안",
		"클라우드 비용 최적화 3개년 로드맵",
		"이행 순서", "기대 효과", "다음 단계",
		"Reducing cloud spend",
		"Make or buy decision for our data platform",
		"No regrets moves", "Risks and mitigations", "Q & A",
		"We need to cut our cloud spend significantly",
	} {
		if unfinishedHeading(heading) {
			t.Errorf("an ordinary heading was called cut: %q", heading)
		}
	}
}
