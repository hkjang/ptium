package pptx

import "testing"

func TestProbeKPIDrawn(t *testing.T) {
	_, _, manifest := buildTemplate(t, "plum-rail")
	design := NewDesign(manifest)
	block := Block{Kind: BlockKPI, Items: []Item{
		{Label: "전환 시스템", Value: "42개"}, {Label: "절감", Value: "18억"}, {Label: "복구 시간", Value: "30분"},
	}}
	for _, height := range []int{1200000, 900000, 600000} {
		frame := Frame{X: 800000, Y: 1800000, Width: 9000000, Height: height}
		component := RenderBlock(design, frame, block)
		for _, primitive := range component.Primitives {
			if len(primitive.Lines) == 0 {
				continue
			}
			// What the text actually draws, the way the inspector measures it.
			lines := 0
			for _, line := range primitive.Lines {
				lines += max(cellLines(line.Text, primitive.FontSize, primitive.Frame.Width), 1)
			}
			drawnHeight := lines * lineHeightFor(primitive.FontSize)
			over := primitive.Frame.Y + drawnHeight - frame.Bottom()
			if over > 0 {
				t.Logf("height=%7d  %-12q size=%5d box=%7d drawn=%7d over=%7d",
					height, line0(primitive), primitive.FontSize, primitive.Frame.Height, drawnHeight, over)
			}
		}
	}
}

func line0(primitive Primitive) string {
	if len(primitive.Lines) == 0 {
		return ""
	}
	return primitive.Lines[0].Text
}
