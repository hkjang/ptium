package pptx

import (
	"strings"
	"testing"
)

// A template set in a font the viewer does not have used to fall through to
// whatever the browser calls sans-serif — DejaVu on most Linux machines, a
// fifth wider in lowercase than the humanist faces templates are set in. SVG
// cannot reflow, so an English title measured to fit its box was drawn past the
// edge of the slide.
func TestAPreviewNamesFontsAViewerIsLikelyToHave(t *testing.T) {
	_, _, manifest := buildTemplate(t, "plum-rail")
	layout, _ := manifest.Layout(manifest.DefaultLayout)
	slide := Slide{LayoutID: layout.ID, Fields: map[string][]Paragraph{
		SlotTitle: {{Text: "Manual dispatch creates operational drag"}},
		SlotBody:  {{Text: "42 technicians dispatched by hand"}}}}
	svg := PreviewSVG(manifest, layout, slide, PreviewOptions{Width: 1200})
	for _, family := range []string{"Arial", "Liberation Sans", "Malgun Gothic",
		// A Japanese deck was drawn with 遅, 効 and 満 as empty boxes: a Korean font
		// covers the hanja Korean uses and not the kanji it does not.
		"Yu Gothic", "Noto Sans JP", "Microsoft YaHei", "sans-serif"} {
		if !strings.Contains(svg, family) {
			t.Fatalf("the preview does not fall back to %s", family)
		}
	}
	// The Latin faces come before the Korean ones: a browser picks per character,
	// so Hangul still finds its own font further down.
	if strings.Index(svg, "Arial") > strings.Index(svg, "Malgun Gothic") {
		t.Fatal("the Korean fallback comes before the Latin ones")
	}
}
