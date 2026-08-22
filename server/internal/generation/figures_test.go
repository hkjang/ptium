package generation

import (
	"strings"
	"testing"
)

// The deck that prompted this asked a board for 12억 원 and promised 가용성
// 99.99% — a number the brief never gave and nobody in the room could source.
func TestFiguresNotInBriefFindsTheInventedOne(t *testing.T) {
	brief := "통계청 2026 소비 동향(표 3) 기준 온라인 거래액 28.5% 증가, 내부 결제 로그 기준 " +
		"지난 12개월 장애 2회·4시간 17분. 결제 이중화 투자 12억 원을 이사회에 요청."
	source := strings.Join([]string{
		"# 시장 성장과 거래 밀도",
		"@content",
		"> 온라인 거래액 28.5% 증가",
		"- 결제 이중화 투자 12억 원을 요청합니다",
		"!source 통계청 | 2026 소비 동향 표 3",
		"# 투자 제안 개요",
		"@content",
		"- 장애 시 자동 전환으로 가용성 99.99% 확보 목표",
		"!notes 12억 원을 투자하면 가용성을 99.99%까지 높일 수 있습니다.",
	}, "\n")
	figures := figuresNotInBrief(source, brief)
	if len(figures) != 1 || !strings.Contains(figures[0], "99.99") {
		t.Fatalf("expected only the invented figure, got %q", figures)
	}
}

func TestFiguresNotInBriefLeavesTheBriefsOwnNumbersAlone(t *testing.T) {
	brief := "매출 1,200억 원, 고객 3만 명, 2026년 상반기 목표."
	source := strings.Join([]string{
		"# 지난해 실적",
		"- 매출 1200억 원을 기록했습니다", // written without the separator
		"- 고객 3만 명이 쓰고 있습니다",
		"- 2026년 상반기에 다시 봅니다", // a date is not a claim
		"- 첫 2주 안에 지표를 확인합니다", // nor is a duration
	}, "\n")
	if figures := figuresNotInBrief(source, brief); len(figures) != 0 {
		t.Fatalf("the brief's own numbers were reported as invented: %q", figures)
	}
}

func TestTheInventedFigureNoteNamesTheNumbers(t *testing.T) {
	note := inventedFigureNote([]string{"99.99%", "30억 원"}, "ko")
	for _, wanted := range []string{"99.99%", "30억 원", "브리프에 없는 숫자"} {
		if !strings.Contains(note, wanted) {
			t.Fatalf("the note does not say %q: %s", wanted, note)
		}
	}
	if english := inventedFigureNote([]string{"99.99%"}, "en"); !strings.Contains(english, "99.99%") {
		t.Fatalf("the English note does not name the figure: %s", english)
	}
}

// The rule against inventing a number was written under "Rules for components",
// so a model writing prose was never covered by it.
func TestTheBriefForbidsInventedFiguresInProse(t *testing.T) {
	rule := "- Never invent a figure. Every number on a slide"
	if !strings.Contains(sourceSystemPrompt, rule) {
		t.Fatal("the writing craft rules do not forbid inventing a figure")
	}
	craft := strings.Index(sourceSystemPrompt, "Writing craft:")
	components := strings.Index(sourceSystemPrompt, "Rules for components:")
	if at := strings.Index(sourceSystemPrompt, rule); craft < 0 || components < 0 || at < craft || at > components {
		t.Fatal("the rule must sit with the writing craft, where prose is written")
	}
}
