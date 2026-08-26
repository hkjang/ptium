package pptx

import (
	"strings"
	"testing"
)

// A figure is a number with the unit written on it. Korean decks number their
// sections — "1. 개요", "2. 배경", "4. 원인" — and every one of those counters is
// also the first syllable of an ordinary word: 개(요), 배(경), 원(인), 만(족).
// Read as figures, section headings were claims the deck had to cite, and the
// quality panel asked the author for the source of "1. 개" on every numbered
// heading of every imported deck.
func TestANumberedHeadingIsNotAFigure(t *testing.T) {
	for _, heading := range []string{
		"1. 개요", "2. 배경", "3. 개발 계획", "4. 원인 분석", "5. 만족도 조사",
		"6. 건의 사항", "7. 배포 전략", "8. 명확한 기준",
	} {
		if figures := StatedFigures(heading); len(figures) > 0 {
			t.Errorf("%q was read as the figure %q", heading, strings.Join(figures, ", "))
		}
	}
}

// And what a room does ask about is still asked about, with or without the
// space Korean sometimes puts before a counter, and through the particle that
// follows it.
func TestTheFiguresARoomAsksAboutAreStillRead(t *testing.T) {
	cases := map[string]string{
		"매출 1,240억으로 늘었습니다": "1,240억",
		"이익률 9.4%":          "9.4%",
		"전환 대상 42개 시스템":     "42개",
		"인력 12명이 붙습니다":      "12명",
		"예산 18 억을 씁니다":      "18 억",
		"처리 1,200건":         "1,200건",
	}
	for said, want := range cases {
		figures := StatedFigures(said)
		if len(figures) == 0 {
			t.Errorf("%q states no figure", said)
			continue
		}
		if figures[0] != want {
			t.Errorf("%q states %q, want %q", said, figures[0], want)
		}
	}
	// A date is when the deck is about, not a claim to cite.
	for _, said := range []string{"2026년 상반기", "첫 2주에 할 일", "6개월 안에"} {
		if figures := StatedFigures(said); len(figures) > 0 {
			t.Errorf("%q was read as the figure %q", said, strings.Join(figures, ", "))
		}
	}
}
