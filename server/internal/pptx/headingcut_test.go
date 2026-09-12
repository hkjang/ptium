package pptx

import "testing"

// The headings ten ordinary briefs put on the wall, and what the measurement
// says about each.
//
// Sixty headings were measured through the running product. Four were cut
// sentences — "재고 회전율 개선 근거를 담고", "하반기 목표를 제안", "다음 분기
// 과제를 정", "만들고 적용 일정을 안내" — and one of the four was reported. The
// check and the writer each kept a hand-written list of verbs, and 담고 was on
// neither.
func TestACutHeadingIsReportedHoweverItWasCut(t *testing.T) {
	for _, heading := range []string{
		"재고 회전율 개선 근거를 담고", "하반기 목표를 제안", "다음 분기 과제를 정",
		"만들고 적용 일정을 안내", "이관 일정을 정리", "보고서를 작성",
		"데이터 거버넌스 체계를 세우", "비용을 크게 줄여야", "Plan for the",
	} {
		if !unfinishedHeading(heading) {
			t.Errorf("%q reads as a heading", heading)
		}
	}
}

// 정리, 작성 and 준비 were reported as cut verbs and are ordinary nouns. The
// writer now repairs "이관 일정을 정리" into "이관 일정 정리" before it is drawn,
// so the two of them disagreed on every deck that had one.
func TestAHeadingThatIsAPhraseIsNotReported(t *testing.T) {
	for _, heading := range []string{
		"이관 일정 정리", "회의 자료 작성", "행사 준비", "다음 분기 과제",
		"재고 회전율 개선 근거", "3분기 안에 시범 도입 제안", "하반기 목표 제안",
		"협력사 평가 기준", "부서별 역할", "개선 활동 결과", "보안 사고 대응 체계",
		"클라우드 이전 비용", "입사까지 걸리는 기간", "고객관리 시스템 도입 계획",
		"기존 시스템의 문제", "다음 단계", "결과 보고", "매출 보고 체계", "Q & A",
	} {
		if unfinishedHeading(heading) {
			t.Errorf("%q was reported as cut", heading)
		}
	}
}
