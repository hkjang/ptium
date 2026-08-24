package pptx

import (
	"strings"
	"testing"
)

// Which kinds draw the third field a row can carry?
func TestProbeThirdField(t *testing.T) {
	data, _ := BuiltinTemplate("")
	_, manifest, _ := AnalyzeBytes(data)
	layout, _ := manifest.LayoutForRole(RoleContent)
	for _, kind := range BlockKinds() {
		slide := Slide{LayoutID: layout.ID,
			Fields: map[string][]Paragraph{SlotTitle: {{Text: "제목"}}},
			Blocks: map[string]Block{SlotBody: {Kind: kind, Heading: "머리말", Items: []Item{
				{Label: "첫 항목", Value: "128억", Detail: "세번째칸입니다"},
				{Label: "둘째 항목", Value: "42%", Detail: "여기도세번째"},
			}}},
		}
		svg := PreviewSVG(manifest, layout, slide, PreviewOptions{Width: 960})
		t.Logf("%-11s label=%v value=%v detail=%v", kind,
			strings.Contains(svg, "첫 항목"), strings.Contains(svg, "128억"), strings.Contains(svg, "세번째칸입니다"))
	}
}
