package korean

import "testing"

// The rule, against what the product actually wrote and against the words it
// must leave alone.
//
// Ten ordinary briefs driven through the running writer produced the first
// list. The second is the reason this rule is written the way it is: 결과, 평가,
// 회의, 경로, 성과, 보고 and 창고 all end in a syllable that is also a marker or
// a joining ending, and the first rule tried here reported every one of them.

var cutPhrases = []string{
	// measured on the running product, from ten ordinary briefs
	"재고 회전율 개선 근거를 담고", "하반기 목표를 제안", "책임을 나누고 우선순위를 정",
	"만들고 적용 일정을 안내", "다시 만들고 적용 일정 안내", "책임을 나누고 우선순위",
	"이관 일정을 정리", "다음 분기 과제를 정", "3분기 안에 시범 도입을 제안",
	"협력사 평가 기준을 다시 만들고 적용 일정을 안내",
	// the same shapes, written out
	"매출을 보고", "예산을 승인", "체계를 구축", "일정을 앞당기려", "비용을 줄이고",
	"설계를 마치고", "위험을 낮추며", "데이터 거버넌스 체계를 세우", "고객 만족도를 조사",
	"자료를 참고", "품질을 우선", "협력사 평가 기준을 검토", "본사에서 시행",
	"검토하고 승인 요청", "예산을 확보해서 추진", "줄이고 남은 과제",
}

var wholePhrases = []string{
	// headings the same ten briefs produced that are right as they are
	"분기 매출 보고", "고객 만족도 조사", "다음 단계", "결정이 필요한 사항", "목차",
	"클라우드 이전 비용", "부서별 역할", "원인", "일정", "데이터 거버넌스 체계",
	"자동화 도입을 승인받기 위한 발표", "기존 시스템의 문제", "입사까지 걸리는 기간",
	// ordinary headings
	"안전 사고", "물류 창고", "지난 분기 회고", "신규 채용 계획", "품질 개선 활동",
	"제품 소개", "비용 절감 방안", "기대 효과", "실행 준비 상태", "남은 질문",
	"고객을 위한 서비스", "매출을 늘리는 방법", "비용을 줄였습니다", "매출을 늘린다",
	"올해는 어떻게 할까", "비용을 절감함", "재고를 줄이기", "우리가 놓친 것",
	"위험은 무엇인가", "지금 결정해야 하는 이유",
	// a noun whose last syllable is also a marker
	"결과 보고", "협력사 평가 기준", "회의 결과", "경로 설계", "성과 관리", "효과 분석",
	"학과 개편", "사과 상자", "단가 인하", "높이 조절", "길이 측정", "차이 분석",
	"종이 사용량", "평가 기준", "결과 공유", "가을 행사", "마을 지원 사업", "서울 사무소",
	"물을 아끼자", "인사 평가 개선", "매출 성과 보고", "구매 경로 분석",
	// a noun whose last syllable is also a joining ending
	"매출 보고 체계", "중간보고 자료", "최종보고 일정", "참고 자료 목록", "사고 예방 대책",
	"보고 체계 개선", "재고 관리 방안", "광고 성과 분석", "회고 결과 정리",
	// and what is not Korean at all
	"Q&A", "Next steps", "2026년 계획", "AI 도입 로드맵", "ROI 분석",
}

func TestAPhraseCutOutOfASentenceIsRecognised(t *testing.T) {
	for _, phrase := range cutPhrases {
		if !CutPhrase(phrase) && !BeginsMidClause(phrase) {
			t.Errorf("%q reads as a phrase", phrase)
		}
	}
}

// The half that matters more: a rule that fires on everything says nothing, and
// the first one written here fired on 결과 보고 and 남은 질문.
func TestAPhraseIsLeftAlone(t *testing.T) {
	if len(wholePhrases) < 50 {
		t.Fatalf("only %d phrases are standing behind this", len(wholePhrases))
	}
	for _, phrase := range wholePhrases {
		if CutPhrase(phrase) || BeginsMidClause(phrase) {
			t.Errorf("%q was called a fragment", phrase)
		}
	}
}

// And the repair says the same thing in words rather than in a verdict.
func TestAHeadingIsRepairedToANounPhrase(t *testing.T) {
	for _, one := range []struct{ in, want string }{
		{"하반기 목표를 제안", "하반기 목표 제안"},
		{"재고 회전율 개선 근거를 담고", "재고 회전율 개선 근거"},
		{"다음 분기 과제를 정", "다음 분기 과제"},
		{"이관 일정을 정리", "이관 일정 정리"},
		{"3분기 안에 시범 도입을 제안", "3분기 안에 시범 도입 제안"},
		{"데이터 거버넌스 체계를 세우", "데이터 거버넌스 체계"},
		{"협력사 평가 기준을 검토", "협력사 평가 기준 검토"},
		{"비용을 줄이고", "비용"},
	} {
		got, changed := TrimToPhrase(one.in)
		if got != one.want || !changed {
			t.Errorf("TrimToPhrase(%q) = %q %v, want %q true", one.in, got, changed, one.want)
		}
	}
	// A phrase that was never cut comes back untouched, and says so.
	for _, whole := range wholePhrases {
		if got, changed := TrimToPhrase(whole); changed || got != whole {
			t.Errorf("TrimToPhrase(%q) = %q %v", whole, got, changed)
		}
	}
}
