package generation

import (
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
)

// A model writing Korean puts a space between a figure and its unit, and
// between a foreign word and the particle after it. Every one of those reads as
// machine output to a Korean reader.
func TestKoreanSpacingIsClosedWhereItShouldBe(t *testing.T) {
	cases := map[string]string{
		"배치 지연을 4 시간에서 15 분으로 단축":      "배치 지연을 4시간에서 15분으로 단축",
		"12 억 예산으로 투자 효율성을 극대화합니다":     "12억 예산으로 투자 효율성을 극대화합니다",
		"2026 년 하반기 · 3 단계 이행":         "2026년 하반기 · 3단계 이행",
		"각 단계는 명확한 deliverables 를 가지며": "각 단계는 명확한 deliverables를 가지며",
		"15 분으로 줄여 94% 의 개선을 달성합니다":    "15분으로 줄여 94%의 개선을 달성합니다",
		"@layout 콘텐츠 2 개":              "@layout 콘텐츠 2개",
		"총 12 억 원의 예산":                 "총 12억 원의 예산",
	}
	for written, wanted := range cases {
		if got := deck.TidyKorean(written); got != wanted {
			t.Errorf("deck.TidyKorean(%q) = %q, want %q", written, got, wanted)
		}
	}
}

// And nowhere else. A space between two Korean words is a matter of judgement,
// and a rule that guessed at it would rewrite what the author meant.
func TestKoreanSpacingLeavesTheRestAlone(t *testing.T) {
	for _, line := range []string{
		"전체 처리 시간을 94% 단축하여 속도를 확보합니다",
		"운영 비용이 20% 절감되어",
		"1 대 1 면담",
		"은 메달 하나",
		"이 프로젝트는 지금 결정이 필요합니다",
		"차 한 대, 금 서 돈",
		"The plan moves 3 systems to a single region",
	} {
		if got := deck.TidyKorean(line); got != line {
			t.Errorf("deck.TidyKorean(%q) = %q, want it untouched", line, got)
		}
	}
}

// It runs on what the model wrote, for a deck written in Korean, and on nothing
// else: a deck in another language keeps the spacing that language uses.
func TestOnlyAKoreanDeckIsTidied(t *testing.T) {
	written := "# 3 단계 이행\n- 4 시간에서 15 분으로\n"
	if got := cleanModelSource(written, "ko"); got != "# 3단계 이행\n- 4시간에서 15분으로" {
		t.Errorf("a Korean deck was not tidied: %q", got)
	}
	if got := cleanModelSource(written, "en"); got != "# 3 단계 이행\n- 4 시간에서 15 분으로" {
		t.Errorf("a deck in another language was tidied: %q", got)
	}
}

// A unit has to end where the word ends. "2026 시장 조사" is not "2026시" and a
// market, and joining it that way was worse than leaving the space alone.
func TestAUnitDoesNotEatTheNextWord(t *testing.T) {
	for written, wanted := range map[string]string{
		"2026 시장 조사 보고서": "2026 시장 조사 보고서",
		"3 장짜리 자료":       "3장짜리 자료",
		"4 시간에서 15 분으로":  "4시간에서 15분으로",
		"12 억 원":         "12억 원",
		"5 개 회사":         "5개 회사",
		"2 년간":           "2년간",
		"7 월요일":          "7 월요일",
	} {
		if got := deck.TidyKorean(written); got != wanted {
			t.Errorf("TidyKorean(%q) = %q, want %q", written, got, wanted)
		}
	}
}

// The gap a model leaves between a number and its unit is the same mistake in
// Japanese, and the tidier only knew Korean: a deck came back saying
// "2026 年 8 月", "8,400 万円" and "3 時間 12 分".
func TestTheUnitGapIsClosedInJapaneseToo(t *testing.T) {
	cases := map[string]string{
		"8,400 万円": "8,400万円",
		"4,200 件":  "4,200件",
		"9 名":      "9名",
		"186 名":    "186名",
		"第 3 四半期":  "第 3四半期",
		"3 時間":     "3時間",
		"12 か月":    "12か月",
	}
	for written, wanted := range cases {
		if got := deck.TidyKorean(written); got != wanted {
			t.Errorf("TidyKorean(%q) = %q, want %q", written, got, wanted)
		}
	}
	// A space before an ordinary word is not the same mistake.
	for _, line := range []string{"導入により 対応時間を短縮", "AI チャットボット", "58% が最多"} {
		if got := deck.TidyKorean(line); got != line {
			t.Errorf("TidyKorean(%q) = %q, want it untouched", line, got)
		}
	}
}
