package pdf

import (
	"testing"

	"github.com/hkjang/ptium/server/internal/pptx"
)

// The width table the deck is fitted and measured with has to agree with the
// face the PDF actually draws in. Every symbol outside ASCII used to fall to
// one catch-all value: the em dash in every heading this product writes drew at
// 0.89 em and was measured at 0.60, so a line could keep a size that did not
// fit and nothing reported it.
//
// Measuring slightly wide is safe — the text is set a little smaller than it
// need be. Measuring narrow is not, so this asserts the direction as well as
// the size.
func TestTheWidthTableAgreesWithTheFace(t *testing.T) {
	font, err := BuiltinFont()
	if err != nil {
		t.Fatal(err)
	}
	// Whole lines first: fitting works on lines, and a line measured narrower
	// than it draws is one that keeps a size it does not fit at.
	for _, line := range []string{
		"배치 지연 — 기대 효과",
		"직판 46% · 대리점 33% · 온라인 21%",
		"First half 2026 results — cost and return",
		"준비 · 이행 · 안정화",
		"목표 대비 달성률 96% → 100%",
	} {
		drawn := 0.0
		for _, character := range line {
			glyph, ok := font.Glyph(character)
			if !ok {
				continue
			}
			drawn += float64(font.Width(glyph)) / 1000
		}
		if measured := pptx.TextEm(line); measured < drawn {
			t.Errorf("%q draws %.2f em and is measured %.2f — narrower than it draws", line, drawn, measured)
		} else if measured > drawn*1.25 {
			t.Errorf("%q draws %.2f em and is measured %.2f — far wider than it draws", line, drawn, measured)
		}
	}
	written := "물류센터 자동화 — 기대 효과 · 요점 … ※ → ★ ≥ “인용” ‘강조’ 46% ABC abc 123"
	for _, character := range written {
		glyph, ok := font.Glyph(character)
		if !ok {
			continue
		}
		drawn := float64(font.Width(glyph)) / 1000
		measured := pptx.TextEm(string(character))
		if measured < drawn-0.06 {
			t.Errorf("%q U+%04X draws at %.2f em and is measured at %.2f — narrower than it draws",
				string(character), character, drawn, measured)
		}
		if measured > drawn*1.35+0.05 {
			t.Errorf("%q U+%04X draws at %.2f em and is measured at %.2f — far wider than it draws",
				string(character), character, drawn, measured)
		}
	}
}
