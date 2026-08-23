package httpapi

import (
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// The workspace shows stored source only while it still describes the stored
// slides. Comparing titles and layouts alone let an edited deck keep showing
// its old text — and applying that text would have thrown the edit away.
func TestSourceIsStaleOnceTheSlidesMoveOn(t *testing.T) {
	data, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 복원 점검\n@cover\n> 첫 번째 판\n\n# 실적\n- 매출 1,240억\n- 이익률 9.8%\n"
	presentation := model.Presentation{Language: "ko"}
	presentation.Slides = deck.Compile(deck.ParseSource(source), manifest,
		deck.CompileOptions{Language: "ko"}).Slides
	if !deck.MatchesSlides(source, presentation, manifest) {
		t.Fatalf("source that produced these slides should still describe them:\n%s",
			deck.Format(presentation, manifest))
	}

	// Everything the source language can say is compared, and none of it changes
	// a slide's title or its layout.
	for name, edit := range map[string]func(*model.Slide){
		"speaker notes": func(slide *model.Slide) { slide.SpeakerNotes = "여기서 말할 것" },
		"a point": func(slide *model.Slide) {
			content := deck.Decode(slide.Content)
			content.SetText(pptx.SlotBody, "매출 1,300억")
			slide.Content = content.Encode()
		},
	} {
		edited := presentation
		edited.Slides = append([]model.Slide(nil), presentation.Slides...)
		edit(&edited.Slides[1])
		if deck.MatchesSlides(source, edited, manifest) {
			t.Errorf("%s changed and the old text still passes as the deck", name)
		}
	}
}
