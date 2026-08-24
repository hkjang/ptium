package pptx

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestFreeformElementRendersEditableDrawingMLAndSVG(t *testing.T) {
	element := Element{
		ID: "callout-1", Kind: "shape", Shape: "roundRect", Frame: Frame{X: 100000, Y: 200000, Width: 3000000, Height: 1000000},
		Rotation: 12, Text: "핵심 & 내용", FontFamily: "Aptos", FontSize: 2400, TextColor: "FFFFFF", Bold: true,
		Fill: "725BD6", Stroke: "4C3AA0", StrokeWidth: EMUPerPoint, Opacity: 80,
	}
	drawing := element.drawingML(8, nil)
	for _, wanted := range []string{`<p:sp>`, `prst="roundRect"`, `rot="720000"`, `핵심 &amp; 내용`, `alpha val="80000"`} {
		if !strings.Contains(drawing, wanted) {
			t.Fatalf("DrawingML is missing %q:\n%s", wanted, drawing)
		}
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg">` + element.SVG(.0001) + `</svg>`
	if !strings.Contains(svg, `rotate(12.000`) || !strings.Contains(svg, `핵심 &amp; 내용`) {
		t.Fatalf("SVG did not preserve the transform or text: %s", svg)
	}
	if err := xml.Unmarshal([]byte(svg), new(struct{ XMLName xml.Name })); err != nil {
		t.Fatalf("freeform SVG is not well formed: %v", err)
	}
}

func TestSlideXMLLayersFreeformAfterTemplateContent(t *testing.T) {
	layout := Layout{ID: "content", Placeholders: []Placeholder{{Slot: SlotTitle, Kind: "text", Type: "title", Name: "Title", Width: 1000, Height: 500}}}
	slide := Slide{Fields: map[string][]Paragraph{SlotTitle: {{Text: "Template title"}}}, Elements: []Element{{ID: "overlay", Kind: "text", Frame: Frame{X: 10, Y: 10, Width: 100, Height: 50}, Text: "Overlay"}}}
	markup, _, _ := slideXML(layout, slide, "ko-KR", Design{}, nil)
	if strings.Index(markup, "Template title") >= strings.Index(markup, "Overlay") {
		t.Fatalf("freeform objects must be layered after template content: %s", markup)
	}
}

func TestFreeformTableExportsAsNativePowerPointTable(t *testing.T) {
	element := Element{
		ID: "table-1", Kind: "table", Frame: Frame{X: 100000, Y: 200000, Width: 5000000, Height: 1800000},
		Cells: [][]string{{"항목", "상태"}, {"핵심 & 검증", "완료"}}, HeaderRows: 1,
		FontSize: 1400, Fill: "725BD6", Stroke: "D9D6E1", StrokeWidth: EMUPerPoint,
	}
	drawing := element.drawingML(9, nil)
	for _, wanted := range []string{`<p:graphicFrame>`, `<a:tbl>`, `<a:tblGrid>`, `firstRow="1"`, `핵심 &amp; 검증`} {
		if !strings.Contains(drawing, wanted) {
			t.Fatalf("native table DrawingML is missing %q:\n%s", wanted, drawing)
		}
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg">` + element.SVG(.0001) + `</svg>`
	if !strings.Contains(svg, `<rect`) || !strings.Contains(svg, `핵심 &amp; 검증`) {
		t.Fatalf("table preview did not render cells: %s", svg)
	}
	if err := xml.Unmarshal([]byte(svg), new(struct{ XMLName xml.Name })); err != nil {
		t.Fatalf("table SVG is not well formed: %v", err)
	}
}

func TestFreeformLineExportsArrowheadsAndDash(t *testing.T) {
	element := Element{ID: "line-1", Kind: "line", Frame: Frame{Width: 1000000, Height: 200000}, Stroke: "4C3AA0", StrokeWidth: 2 * EMUPerPoint,
		StartArrow: "oval", EndArrow: "triangle", Dash: "dashDot"}
	drawing := element.drawingML(4, nil)
	for _, wanted := range []string{`<a:headEnd type="oval"`, `<a:tailEnd type="triangle"`, `<a:prstDash val="dashDot"`} {
		if !strings.Contains(drawing, wanted) {
			t.Fatalf("line DrawingML is missing %q: %s", wanted, drawing)
		}
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg">` + element.SVG(.001) + `</svg>`
	if !strings.Contains(svg, `marker-start=`) || !strings.Contains(svg, `marker-end=`) || !strings.Contains(svg, `stroke-dasharray="8 4 2 4"`) {
		t.Fatalf("line SVG is missing its markers or dash: %s", svg)
	}
	if err := xml.Unmarshal([]byte(svg), new(struct{ XMLName xml.Name })); err != nil {
		t.Fatalf("line SVG is not well formed: %v", err)
	}
}

// An object arrives aligned in the words a browser uses or in the words
// DrawingML uses — the workspace converts between them when it lifts a template
// region onto the canvas. Both are drawn the same way, and a value that is
// neither is not an alignment.
func TestAnObjectIsAlignedInEitherVocabulary(t *testing.T) {
	for _, pair := range [][2]string{{"center", "middle"}, {"ctr", "ctr"}} {
		element := Element{ID: "s1", Kind: "shape", Shape: "roundRect", Text: "도형 안의 글",
			Frame: Frame{X: 0, Y: 0, Width: 2000000, Height: 1000000},
			Align: pair[0], VerticalAlign: pair[1]}
		markup := element.drawingML(2, nil)
		if !strings.Contains(markup, `anchor="ctr"`) || !strings.Contains(markup, `algn="ctr"`) {
			t.Errorf("%v was not centred:\n%s", pair, markup)
		}
	}
	if !AlignmentIsKnown("right", "bottom") || !AlignmentIsKnown("r", "b") || !AlignmentIsKnown("", "") {
		t.Error("a known alignment was refused")
	}
	if AlignmentIsKnown("middle", "") || AlignmentIsKnown("", "justify") {
		t.Error("an alignment the renderer cannot draw was accepted")
	}
}

// A text box the author never typed into is nothing, and must not be drawn in
// either the preview or the file. One reached a deck carrying the editor's own
// prompt as its content.
func TestAnEmptyTextBoxIsNotDrawn(t *testing.T) {
	_, pkg, manifest := buildTemplate(t, "plum-rail")
	layout, _ := manifest.Layout(manifest.DefaultLayout)
	box := Element{ID: "a", Kind: "text", Text: "  ",
		Frame: Frame{X: 1000000, Y: 1000000, Width: 2000000, Height: 800000}}
	slide := Slide{LayoutID: layout.ID,
		Fields:   map[string][]Paragraph{SlotTitle: {{Text: "제목"}}},
		Elements: []Element{box}}
	blank := PreviewSVG(manifest, layout, slide, PreviewOptions{Width: 800})
	if !strings.Contains(blank, "제목") {
		t.Fatal("the preview did not draw the slide at all")
	}
	if strings.Contains(blank, "<rect") && strings.Count(blank, "<text") > 2 {
		t.Fatalf("an empty text box left something on the slide: %s", blank)
	}
	slide.Elements[0].Text = "실제로 쓴 글"
	if drawn := PreviewSVG(manifest, layout, slide, PreviewOptions{Width: 800}); !strings.Contains(drawn, "실제로 쓴 글") {
		t.Fatal("a text box with words in it was not drawn")
	}
	// Nor does the exported file carry it.
	deck := Deck{Language: "ko", Slides: []Slide{{LayoutID: layout.ID,
		Fields:   map[string][]Paragraph{SlotTitle: {{Text: "제목"}}},
		Elements: []Element{box}}}}
	file, err := Render(pkg, manifest, deck)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(string(file), "TextBox") {
		t.Fatal("the file carries a text box nobody typed into")
	}
}
