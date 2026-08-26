package pptx

import (
	"strings"
	"testing"
)

// A list handed to a room whole is read ahead of the speaker. A slide built up
// a line at a time draws the points not yet spoken invisibly rather than
// leaving them out, so the ones already on the wall do not move when the next
// one arrives — the fitting is the same fitting either way.
func TestBuildingASlideKeepsItWhereItWas(t *testing.T) {
	manifest, _, layout := testDesign(t, "plum-rail")
	slide := Slide{LayoutID: layout.ID, Fields: map[string][]Paragraph{
		SlotTitle: {{Text: "이행 순서"}},
		SlotBody: {
			{Text: "준비: 범위와 조직을 확정합니다"},
			{Text: "이행: 단계별로 적용합니다"},
			{Text: "안정화: 운영으로 넘깁니다"},
		}}}
	whole := PreviewSVG(manifest, layout, slide, PreviewOptions{Width: 800})
	if strings.Contains(whole, `opacity="0"`) {
		t.Error("a slide nobody is building hides part of itself")
	}
	for reveal, hidden := range map[int]int{1: 2, 2: 1, 3: 0} {
		drawn := PreviewSVG(manifest, layout, slide, PreviewOptions{Width: 800, Reveal: reveal})
		if got := strings.Count(drawn, `opacity="0"`); got != hidden {
			t.Errorf("reveal %d hid %d lines, want %d", reveal, got, hidden)
		}
		// Every word is still in the drawing, and every line is where it was.
		for _, said := range []string{"준비", "이행", "안정화", "이행 순서"} {
			if !strings.Contains(drawn, said) {
				t.Errorf("reveal %d lost %q", reveal, said)
			}
		}
		if positions(drawn) != positions(whole) {
			t.Errorf("reveal %d moved the lines", reveal)
		}
	}
}

// A title is not a point: it is on the wall before anybody says anything.
func TestBuildingNeverHoldsBackTheTitle(t *testing.T) {
	manifest, _, layout := testDesign(t, "plum-rail")
	slide := Slide{LayoutID: layout.ID, Fields: map[string][]Paragraph{
		SlotTitle:    {{Text: "제목"}},
		SlotSubtitle: {{Text: "한 줄 요약"}},
		SlotBody:     {{Text: "하나"}, {Text: "둘"}},
	}}
	drawn := PreviewSVG(manifest, layout, slide, PreviewOptions{Width: 800, Reveal: 1})
	title := drawn[strings.Index(drawn, "제목"):]
	if strings.Contains(title[:min(len(title), 20)], `opacity="0"`) {
		t.Error("the title was held back")
	}
	if strings.Count(drawn, `opacity="0"`) != 1 {
		t.Errorf("one point should be waiting, %d are", strings.Count(drawn, `opacity="0"`))
	}
}

// positions is where every line was drawn, so two drawings can be compared for
// having moved rather than for being identical.
func positions(svg string) string {
	var found []string
	for _, part := range strings.Split(svg, "<tspan ") {
		if index := strings.Index(part, ">"); index > 0 {
			attributes := part[:index]
			if x := strings.Index(attributes, `y="`); x >= 0 {
				rest := attributes[x+3:]
				if end := strings.Index(rest, `"`); end >= 0 {
					found = append(found, rest[:end])
				}
			}
		}
	}
	return strings.Join(found, ",")
}
