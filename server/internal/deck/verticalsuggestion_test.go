package deck

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/pptx"
)

func sidewaysTemplate() pptx.Manifest {
	return pptx.Manifest{
		AspectRatio: "16:9", SlideWidth: 12192000, SlideHeight: 6858000,
		Layouts: []pptx.Layout{{
			ID: "vertical-text", Name: "VERTICAL", Role: pptx.RoleContent, Type: "vertTx",
			Placeholders: []pptx.Placeholder{
				{Slot: pptx.SlotTitle, Type: "title", Kind: "text", MaxChars: 40, MaxLines: 1, Width: 8229600, Height: 800000},
				{Slot: "body", Type: "body", Kind: "text", MaxChars: 900, MaxLines: 30, Vertical: true, Width: 3000000, Height: 4000000},
			},
		}, {
			ID: "object", Name: "OBJECT", Role: pptx.RoleContent, Type: "obj",
			Placeholders: []pptx.Placeholder{
				{Slot: pptx.SlotTitle, Type: "title", Kind: "text", MaxChars: 40, MaxLines: 1, Width: 8229600, Height: 800000},
				{Slot: "body", Type: "body", Kind: "text", MaxChars: 400, MaxLines: 10, Width: 8229600, Height: 3600000},
			},
		}},
	}
}

// A model naming a vertical-text layout for ordinary bullets does not get it.
//
// The search that picks a layout already knows a vertical-text layout is not
// one anybody chooses on purpose: it holds more lines than any other, so on fit
// alone it always wins. The comparison that decides whether to override a
// model's named layout scored both sides on raw fit and threw that knowledge
// away — so the model's choice stood, and on a real template every deck of
// Korean bullets came back set sideways, its body measured 3.11cm past the
// slide edge and 55% on top of its own title.
func TestAModelsVerticalLayoutIsReplaced(t *testing.T) {
	slide := SourceSlide{
		Title:    "현황: 오라클 라이선스 비용의 지속적 부담",
		LayoutID: "vertical-text",
		Bullets: []pptx.Paragraph{
			{Text: "오라클 엔터프라이즈 라이선스 연 4억 원 고정비 발생"},
			{Text: "리포팅 DB는 고가 성능보다 안정성과 확장성이 핵심"},
			{Text: "현재 12개 DB는 오버프로비저닝으로 비용 효율 저하"},
		},
	}
	layout, said := resolveSourceLayout(sidewaysTemplate(), slide, 2, 8, "",
		CompileOptions{LayoutsAreSuggestions: true})
	if layout.ID == "vertical-text" {
		t.Errorf("the model's vertical layout stood: %q (%s)", layout.ID, said)
	}
	if !strings.Contains(said, "VERTICAL") {
		t.Errorf("the substitution was made silently: %q", said)
	}

	// An author who names a layout gets it: it is their deck, not a suggestion.
	theirs, said := resolveSourceLayout(sidewaysTemplate(), slide, 2, 8, "", CompileOptions{})
	if theirs.ID != "vertical-text" {
		t.Errorf("an author's own choice was overridden: %q (%s)", theirs.ID, said)
	}
}
