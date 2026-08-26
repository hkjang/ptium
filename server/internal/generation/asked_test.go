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

func headingsOf(source string) string {
	var headings []string
	for _, line := range strings.Split(source, "\n") {
		if strings.HasPrefix(line, "# ") {
			headings = append(headings, line)
		}
	}
	return strings.Join(headings, "\n")
}
