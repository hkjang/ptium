package generation

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/model"
)

// A heading is what the slide is about. The offline writer cuts it out of the
// brief's own sentence, and every way that cut can go wrong shows up as a title:
// a bracket nothing opened, a year on its own, half a sentence ending in 을.
func TestAHeadingIsASubjectNotAFragment(t *testing.T) {
	briefs := []struct{ title, prompt string }{
		{"자동화 승인", "물류센터 자동화(AMR 20대) 도입 승인 요청. 투자 18억, 3년 내 회수, 인력 재배치 12명."},
		{"서비스 출시", "유지보수 서비스를 2026년 3분기에 출시합니다. 목표 계약 240곳, 예상 매출 36억."},
		{"플랫폼 재구축", "데이터 플랫폼 재구축. 배치 지연 6시간을 30분으로 줄이고 저장 비용 40% 절감."},
		{"분기 실적", "2026년 상반기 실적. 매출 2024년 910억, 2025년 1,040억. 직판 46%, 대리점 33%, 온라인 21%."},
		{"고객 분석", "지역별 고객 분포. 서울 1,200곳, 경기 860곳, 부산 540곳, 대구 410곳. 이탈률 4.8%."},
		{"보안 강화", "사내 보안 강화 계획. 다중 인증 도입, 로그 보관 1년, 접근 심사 분기 1회."},
		{"채용 계획", "2026년 채용 계획. 개발 18명, 영업 6명, 지원 4명. 평균 온보딩 기간 5주."},
		{"해외 진출", "동남아 진출 검토(베트남, 태국). 초기 투자 12억, 손익분기 18개월, 현지 파트너 3곳."},
	}
	orphanBracket := regexp.MustCompile(`[(（\[{][^)）\]}]*$|^[^(（\[{]*[)）\]}]`)
	template := testTemplate(t)
	for _, brief := range briefs {
		presentation := model.Presentation{Title: brief.title, Prompt: brief.prompt, Language: "ko", RequestedSlideCount: 8}
		for _, slide := range Fallback(presentation, model.Profile{}, template).Slides {
			// The frame suffix — "…— 기대 효과" — is the deck's own wording, so the
			// heading under test is what the brief supplied.
			heading := strings.TrimSpace(strings.SplitN(slide.Title, " — ", 2)[0])
			switch {
			case orphanBracket.MatchString(heading):
				t.Errorf("%s: heading %q carries a bracket it does not close", brief.title, slide.Title)
			case measurementOnly(heading):
				t.Errorf("%s: heading %q is a measurement, not a subject", brief.title, slide.Title)
			case heading != withoutTrailingParticle(heading):
				t.Errorf("%s: heading %q stops in the middle of a sentence", brief.title, slide.Title)
			}
		}
	}
}

// What is inside a parenthesis is an aside, not another subject.
func TestASubjectIsNotSplitInsideItsBrackets(t *testing.T) {
	got := splitTopics("물류센터 자동화(AMR 20대, 컨베이어 3식) 도입 승인")
	if len(got) != 1 {
		t.Fatalf("splitTopics = %q, want the one subject", got)
	}
	if two := splitTopics("전환 계획, 비용, 리스크"); len(two) != 3 {
		t.Errorf("splitTopics = %q, want three subjects", two)
	}
}

func TestAMeasurementIsNotASubject(t *testing.T) {
	for clause, want := range map[string]bool{
		"2026년":       true,
		"영업 6명":       true,
		"매출 1,040억":   true,
		"목차":          false,
		"이탈률":         false,
		"2026년 채용 계획": false,
	} {
		if got := measurementOnly(clause); got != want {
			t.Errorf("measurementOnly(%q) = %v, want %v", clause, got, want)
		}
	}
}

// A connective verb joins two clauses; a heading may keep only the first.
func TestATopicStopsAtItsConnectiveVerb(t *testing.T) {
	if got := topicPhrase("배치 지연 6시간을 30분으로 줄이고 저장 비용 40% 절감"); strings.Contains(got, "저장") {
		t.Errorf("topicPhrase = %q, want the half before 줄이고", got)
	}
}

// Shortening must not keep the number and drop the words.
func TestASectionIsNotNamedAfterAYear(t *testing.T) {
	outline := outlinePrompt("2026년 채용 계획. 개발 18명, 영업 6명, 지원 4명.", "채용 계획", koreanCopy)
	for _, topic := range outline.Topics {
		if measurementOnly(topic.Name) {
			t.Errorf("topic %q is a measurement, not a subject", topic.Name)
		}
	}
}
