package generation

import (
	"context"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/model"
)

// A brief that asks for a section is not a brief about the asking.
//
// "마지막에 리스크 장을 꼭 넣어주세요" produced a deck with a slide titled
// "리스크 장을 꼭 넣어주세요" — and then another one titled "리스크 장을 꼭
// 넣어주세요 — 현황". The English side did the same with "Include a slide on
// rollback". What the sentence names is the section; the rest is the asking.
func TestASectionAskedForIsASection(t *testing.T) {
	for _, probe := range []struct {
		brief    string
		language string
		wanted   []string
		gone     []string
	}{
		{
			brief:    "물류센터 자동화 도입을 임원에게 보고합니다. 마지막에 리스크 장을 꼭 넣어주세요.",
			language: "ko",
			wanted:   []string{"# 리스크"},
			gone:     []string{"넣어주세요", "마지막에"},
		},
		{
			brief:    "신규 서비스 출시 계획입니다. 일정 슬라이드와 예산 슬라이드를 포함해 주세요.",
			language: "ko",
			wanted:   []string{"# 일정", "# 예산"},
			gone:     []string{"슬라이드", "포함해"},
		},
		{
			brief:    "Report on the migration. Include a slide on rollback and a timeline slide.",
			language: "en",
			wanted:   []string{"# Rollback", "# Timeline"},
			gone:     []string{"Include a slide", "slide on"},
		},
	} {
		made, err := New(testSettings{"ai.provider": "fallback"}).Generate(context.Background(),
			model.Presentation{OwnerID: "owner-1", Language: probe.language, RequestedSlideCount: 8,
				Prompt: probe.brief}, model.Profile{}, testTemplate(t))
		if err != nil {
			t.Fatalf("Generate(%q) error = %v", probe.brief, err)
		}
		for _, want := range probe.wanted {
			if !strings.Contains(made.Source, want+"\n") && !strings.Contains(made.Source, want+" ") {
				t.Errorf("the deck has no %q slide:\n%s", want, headingsOf(made.Source))
			}
		}
		for _, said := range probe.gone {
			if strings.Contains(headingsOf(made.Source), said) {
				t.Errorf("a slide is titled with the request itself (%q):\n%s", said, headingsOf(made.Source))
			}
		}
	}
}

// The deck is not about the section somebody asked for: a report on warehouse
// automation with a risk slide is still a report on warehouse automation.
func TestWhatWasAskedForDoesNotBecomeTheTitle(t *testing.T) {
	made, err := New(testSettings{"ai.provider": "fallback"}).Generate(context.Background(),
		model.Presentation{OwnerID: "owner-1", Language: "ko", RequestedSlideCount: 8,
			Prompt: "물류센터 자동화 도입을 임원에게 보고합니다. 마지막에 리스크 장을 꼭 넣어주세요."},
		model.Profile{}, testTemplate(t))
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	cover := strings.SplitN(strings.TrimPrefix(made.Source, "# "), "\n", 2)[0]
	if cover != "물류센터 자동화 도입" {
		t.Errorf("the cover reads %q", cover)
	}
}

// A slide word is not a request. "랜딩 페이지 개편" and "보안 섹션 강화 방안" are
// subjects that happen to contain one, and reading those as requests took the
// subject out of the deck and left a section called "랜딩".
func TestASlideWordWithoutTheAskingIsJustAWord(t *testing.T) {
	for brief, wanted := range map[string]string{
		"랜딩 페이지 개편과 전환율 개선 계획":                          "랜딩 페이지",
		"제품 소개 페이지 리뉴얼 계획을 보고합니다":                       "제품 소개 페이지",
		"보안 섹션 강화 방안":                                   "보안 섹션",
		"슬라이드 편집기 개선 로드맵":                               "슬라이드 편집기",
		"The landing page redesign and conversion plan": "landing page",
		"A pricing page experiment for the growth team": "pricing page",
	} {
		outline := outlinePrompt(brief, "", koreanCopy)
		if !strings.Contains(outline.Subject, wanted) {
			t.Errorf("%q lost %q from its subject: %q", brief, wanted, outline.Subject)
		}
		for _, topic := range outline.Topics {
			if topic.Asked {
				t.Errorf("%q was read as a request for a section called %q", brief, topic.Name)
			}
		}
	}
}

// People ask for a section in whatever words come to hand, and the verbs they
// use are the same ones an instruction is written with — which is why the
// request has to be read before the instructions are stripped out of the brief.
func TestASectionIsAskedForInWhateverWords(t *testing.T) {
	for brief, wanted := range map[string]string{
		"클라우드 전환 계획입니다. 리스크 장도 넣어줘":                             "리스크",
		"클라우드 전환 계획입니다. Q&A 슬라이드 추가해줘":                          "Q&A",
		"클라우드 전환 계획입니다. 마지막에 감사 인사 페이지 하나 넣어주세요":                "감사 인사",
		"클라우드 전환 계획입니다. 일정표 섹션 포함":                              "일정표",
		"클라우드 전환 계획입니다. 경쟁사 비교 장을 만들어 주세요":                      "경쟁사 비교",
		"클라우드 전환 계획입니다. 부록 슬라이드도 부탁해":                           "부록",
		"클라우드 전환 계획입니다. 요약 슬라이드 넣기":                             "요약",
		"Cloud migration plan. Please include a summary slide.": "summary",
		"Cloud migration plan. Add a risks section.":            "risks",
	} {
		outline := outlinePrompt(brief, "", koreanCopy)
		found := false
		for _, topic := range outline.Topics {
			if topic.Asked && strings.EqualFold(topic.Name, wanted) {
				found = true
			}
			if !topic.Asked && strings.Contains(topic.Name, "슬라이드") {
				t.Errorf("%q left the request in a section title: %q", brief, topic.Name)
			}
		}
		if !found {
			t.Errorf("%q did not ask for a %q section: %v", brief, wanted, outline.Topics)
		}
	}
}

// "세 장을 넣어줘" is a length written with the verb a request uses. Read as a
// request it made a section out of the whole brief.
func TestALengthWrittenLikeARequestIsStillALength(t *testing.T) {
	outline := outlinePrompt("클라우드 전환 계획을 세 장을 넣어줘", "", koreanCopy)
	if outline.Subject != "클라우드 전환 계획" {
		t.Errorf("the subject is %q", outline.Subject)
	}
	for _, topic := range outline.Topics {
		if topic.Asked {
			t.Errorf("a length was read as a request for %q", topic.Name)
		}
	}
}

// The audience takes its particle with it. "이사회에 보고하는 자료로 만들어
// 주세요" left "로" behind, and a board pack was titled "2026년 상반기 실적을 로".
func TestTheAudienceLeavesNothingBehind(t *testing.T) {
	outline := outlinePrompt("2026년 상반기 실적을 이사회에 보고하는 자료로 만들어 주세요", "", koreanCopy)
	if outline.Subject != "2026년 상반기 실적" {
		t.Errorf("the subject is %q", outline.Subject)
	}
}

// A count is not a section. "10장으로 정리해줘" says how long the deck is.
func TestALengthIsNotASection(t *testing.T) {
	for _, brief := range []string{
		"2026년 채용 계획을 10장으로 정리해줘",
		"공장 자동화 현황을 세 장으로 요약해 주세요",
	} {
		outline := outlinePrompt(brief, "", koreanCopy)
		for _, topic := range outline.Topics {
			if strings.Contains(topic.Name, "장") && len([]rune(topic.Name)) <= 3 {
				t.Errorf("%q made a section called %q", brief, topic.Name)
			}
			if topic.Asked {
				t.Errorf("%q was read as a request for a section called %q", brief, topic.Name)
			}
		}
	}
}

// 용 says what something is for — "보고용", "고객용" — and it is also the last
// syllable of a great many ordinary words. Reading every one of them as a
// purpose cut each heading short at the first: a deck about cloud costs was
// titled "클라우드", and one about hiring was titled "신규".
func TestAWordEndingInTheSyllableForIsNotAPurpose(t *testing.T) {
	for name, want := range map[string]string{
		"신규 채용 계획":       "신규 채용 계획",
		"클라우드 비용 최적화 방안": "클라우드 비용 최적화 방안",
		"고객 사용 패턴 분석":    "고객 사용 패턴 분석",
		"데이터 활용 전략":      "데이터 활용 전략",
		"약관 내용 변경 안내":    "약관 내용 변경 안내",
		"AI 적용 범위 확대":    "AI 적용 범위 확대",
		// What the rule is actually for still works.
		"임원 보고용 자료": "임원",
		"고객 발표용 덱":  "고객",
	} {
		if got := topicPhrase(name); got != want {
			t.Errorf("topicPhrase(%q) = %q, want %q", name, got, want)
		}
	}
}

// 와/과 joins two subjects and is written attached to the first, which is also
// how a hundred ordinary words end — 효과, 결과, 성과, 경과. Splitting wherever
// it appeared cut those in half: a deck briefed as "협업 툴 도입 효과 측정 결과"
// had a section called "협업 툴 도입 효".
func TestAJoiningParticleDoesNotCutAWordInHalf(t *testing.T) {
	for subject, want := range map[string][]string{
		"협업 툴 도입 효과 측정 결과": {"협업 툴 도입 효과 측정 결과"},
		"분기 성과 분석":         {"분기 성과 분석"},
		"추진 경과 보고":         {"추진 경과 보고"},
		// What the particle is for still works, on both sides of it.
		"매출과 비용 구조 개선":      {"매출", "비용 구조 개선"},
		"제품과 서비스 로드맵":       {"제품", "서비스 로드맵"},
		"성과와 과제":            {"성과", "과제"},
		"연구개발 투자 계획과 성과 지표": {"연구개발 투자 계획", "성과 지표"},
	} {
		got := splitTopics(subject)
		if len(got) != len(want) {
			t.Errorf("splitTopics(%q) = %q, want %q", subject, got, want)
			continue
		}
		for index := range want {
			if strings.TrimSpace(got[index]) != want[index] {
				t.Errorf("splitTopics(%q) = %q, want %q", subject, got, want)
				break
			}
		}
	}
}

// Which side the number is on says what it is doing. "영업 6명" measures a thing
// and belongs on a slide; "4분기 전망" is a subject the deck argues, and reading
// the two alike threw the second kind away — a brief asking for "3분기 마감
// 결과와 4분기 전망" produced a deck with no fourth quarter in it.
func TestANumberInFrontOfANounIsWhenNotWhat(t *testing.T) {
	for clause, want := range map[string]bool{
		"영업 6명":       true,
		"매출 1,040억":   true,
		"2026년":       true,
		"12개월":        true,
		"4분기 전망":      false,
		"2026년 채용 계획": false,
		"3분기 실적":      false,
	} {
		if got := measurementOnly(clause); got != want {
			t.Errorf("measurementOnly(%q) = %v, want %v", clause, got, want)
		}
	}
	outline := outlinePrompt("3분기 마감 결과와 4분기 전망", "", koreanCopy)
	found := false
	for _, topic := range outline.Topics {
		if strings.Contains(topic.Name, "4분기") {
			found = true
		}
	}
	if !found {
		t.Errorf("the fourth quarter is in no section: %v", outline.Topics)
	}
	// And a quarter is not one of the figures the deck draws.
	for _, figure := range outline.Figures {
		if strings.Contains(figure.Value, "분기") {
			t.Errorf("a quarter was read as a figure: %#v", outline.Figures)
		}
	}
}

// A year on its own is when, not what. A brief ending "…, 2026" gave the deck
// a section titled "2026".
func TestAYearOnItsOwnIsNotASection(t *testing.T) {
	for _, brief := range []string{
		"Cost reduction plan for cloud infrastructure, 2026",
		"클라우드 비용 절감 계획, 2026",
		"Roadmap for the platform team, FY2026",
	} {
		for _, topic := range outlinePrompt(brief, "", englishCopy).Topics {
			if justAPeriod(topic.Name) {
				t.Errorf("%q made a section called %q", brief, topic.Name)
			}
		}
	}
	// A year in front of a subject still belongs to it.
	found := false
	for _, topic := range outlinePrompt("2026년 계획, 조직 개편", "", koreanCopy).Topics {
		if topic.Name == "2026년 계획" {
			found = true
		}
	}
	if !found {
		t.Error("the year was taken off the subject it belongs to")
	}
}

// Taking an instruction out from between two halves of a sentence leaves the
// punctuation that joined them. "Budget request for the design team, 250M KRW"
// put "Budget request , 250M KRW" on the cover.
func TestAnInstructionLeavesNoPunctuationBehind(t *testing.T) {
	for _, brief := range []string{
		"Budget request for the design team, 250M KRW",
		"Roadmap for the platform team, FY2026",
	} {
		if subject := outlinePrompt(brief, "", englishCopy).Subject; strings.Contains(subject, " ,") {
			t.Errorf("%q left a gap before its comma: %q", brief, subject)
		}
	}
	if got := tidySeparators("Budget request , 250M KRW"); got != "Budget request, 250M KRW" {
		t.Errorf("tidySeparators = %q", got)
	}
	if got := tidySeparators(" , 계획 , , 예산 , "); got != "계획, 예산" {
		t.Errorf("tidySeparators = %q", got)
	}
}

func headingsOf(source string) string {
	var headings []string
	for _, line := range strings.Split(source, "\n") {
		if strings.HasPrefix(line, "# ") {
			headings = append(headings, line)
		}
	}
	return strings.Join(headings, "\n")
}
