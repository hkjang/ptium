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

// The same reading of a heading, in the other two scripts the product writes.
// A brief in English or Japanese is cut the same way and goes wrong the same
// way: a date that leaves its marker behind, a phrase that ends on the word
// introducing what was cut, a figure line given a section of its own.
func TestAHeadingIsASubjectInEveryScript(t *testing.T) {
	briefs := []struct{ lang, title, prompt string }{
		{"en", "Warehouse automation", "Approval for warehouse automation (20 AMR units). Investment $1.8M, payback in 3 years, 12 staff redeployed."},
		{"en", "Q2 results", "First half 2026 results. Revenue was $91M in 2024 and $104M in 2025. Direct 46%, partner 33%, online 21%."},
		{"en", "Service launch", "We will launch the maintenance service in Q3 2026. Target 240 contracts, expected revenue $3.6M."},
		{"ja", "採用計画", "2026年の採用計画。開発18名、営業6名、支援4名。平均オンボーディング期間5週間。"},
		{"ja", "四半期実績", "2026年上半期の実績。売上は2024年910億、2025年1,040億。直販46%、代理店33%、オンライン21%。"},
	}
	// Written out rather than asked of the code under test, so that reverting a
	// fix shows up here instead of moving both sides of the comparison.
	danglingMarker := regexp.MustCompile(`^[年月의の・]`)
	japaneseMarker := regexp.MustCompile(`[はがを]$`)
	latinTail := regexp.MustCompile(`(?i)\s(in|on|at|to|of|for|from|by|with|and|or|a|an|the)$`)
	figureLine := regexp.MustCompile(`(?i)^([$€£¥]?[\d,.]+\s*[%a-z]{0,3}|[a-z]+\s+[$€£¥]?[\d,.]+\s*[%a-z]{0,3})$`)
	template := testTemplate(t)
	for _, brief := range briefs {
		presentation := model.Presentation{Title: brief.title, Prompt: brief.prompt, Language: brief.lang, RequestedSlideCount: 8}
		for _, slide := range Fallback(presentation, model.Profile{}, template).Slides {
			heading := strings.TrimSpace(strings.SplitN(slide.Title, " — ", 2)[0])
			switch {
			case danglingMarker.MatchString(heading):
				t.Errorf("%s: heading %q opens on what a date left behind", brief.title, slide.Title)
			case figureLine.MatchString(heading):
				t.Errorf("%s: heading %q is a measurement, not a subject", brief.title, slide.Title)
			case latinTail.MatchString(heading), japaneseMarker.MatchString(heading):
				t.Errorf("%s: heading %q stops in the middle of a sentence", brief.title, slide.Title)
			}
		}
	}
}

// A topic is one subject, so its slides carry one name. The slide that has no
// aspect to add must not show a longer name than the ones that do.
func TestATopicIsCalledOneThingAcrossItsSlides(t *testing.T) {
	presentation := model.Presentation{
		Title:               "Service launch",
		Prompt:              "We will launch the maintenance service in Q3 2026. Target 240 contracts, expected revenue $3.6M.",
		Language:            "en",
		RequestedSlideCount: 8,
	}
	var bases []string
	for _, slide := range Fallback(presentation, model.Profile{}, testTemplate(t)).Slides {
		bases = append(bases, strings.TrimSpace(strings.SplitN(slide.Title, " — ", 2)[0]))
	}
	// One name shortened two ways leaves one a strict prefix of the other, which
	// is what "…maintenance service in Q3" and "…maintenance" were.
	for _, one := range bases {
		for _, other := range bases {
			if one != other && strings.HasPrefix(one, other) {
				t.Errorf("the topic is called %q on one slide and %q on another", one, other)
			}
		}
	}
}

// A figure label is cut out of the brief's sentence the same way a heading is,
// and it goes wrong the same ways. The heading rules were applied to headings
// only, and the label under the number on the same slide kept the bracket the
// title had just lost.
func TestAFigureLabelIsRepairedLikeAHeading(t *testing.T) {
	cases := map[string]string{
		"물류센터 자동화(AMR": "물류센터 자동화", // a bracket it never closes
		"지연 6시간을":      "지연",       // a measurement labelling a measurement
		"개발 속도":        "개발 속도",    // a label that is already a label
		"인력 재배치":       "인력 재배치",
		"目標可用性":        "目標可用性",
	}
	for given, want := range cases {
		if got := figureLabelName(given); got != want {
			t.Errorf("figureLabelName(%q) = %q, want %q", given, got, want)
		}
	}
	// Through the door the deck actually uses, so the repair is wired in and not
	// merely written.
	if got := figureLabel("물류센터 자동화(AMR 20대) 도입 승인 요청.", "20대"); strings.ContainsAny(got, "()") {
		t.Errorf("figureLabel = %q, want no bracket it does not close", got)
	}
}

// A script without spaces still has words. Counting back a fixed number of
// characters to find a label landed inside one.
func TestALabelDoesNotStartInTheMiddleOfAWord(t *testing.T) {
	got := figureLabel("平均オンボーディング期間5週間。", "5週")
	if strings.HasPrefix(got, "ボー") || !strings.Contains(got, "オンボーディング") {
		t.Errorf("figureLabel = %q, want the whole word オンボーディング", got)
	}
	if latin := figureLabel("Average onboarding period 5 weeks.", "5 week"); !strings.Contains(latin, "onboarding") {
		t.Errorf("figureLabel = %q, want the Latin label intact", latin)
	}
}
