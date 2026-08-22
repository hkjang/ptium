package pptx

import "testing"

// A gauge with one bar is three centimetres tall in a region twelve deep. Drawn
// from the top it leaves three quarters of the slide empty; centred, the same
// drawing reads as a statement with room around it.
func TestASlideWithOnlyAComponentCentresIt(t *testing.T) {
	_, _, manifest := buildTemplate(t, "")
	layout, _ := manifest.Layout(manifest.DefaultLayout)
	body, ok := layout.Slot(SlotBody)
	if !ok {
		t.Fatal("the content layout has no body")
	}
	frame := Frame{X: body.X, Y: body.Y, Width: body.Width, Height: body.Height}
	block := Block{Kind: BlockMeter, Heading: "집행률", Items: []Item{{Label: "집행", Value: "68%"}}}
	component := RenderBlock(NewDesign(manifest), frame, block)
	if len(component.Primitives) == 0 {
		t.Fatal("the gauge drew nothing")
	}
	top, bottom, _ := drawnBounds(component.Primitives)
	drawn := bottom - top
	if drawn > frame.Height/2 {
		t.Skipf("this design's gauge already fills its region (%d of %d)", drawn, frame.Height)
	}
	centreInFrame(&component, frame)
	top, bottom, _ = drawnBounds(component.Primitives)
	above, below := top-frame.Y, frame.Y+frame.Height-bottom
	if difference := above - below; difference > frame.Height/50 || difference < -frame.Height/50 {
		t.Fatalf("the drawing is not centred: %d above, %d below", above, below)
	}
}

// The measurement is of ink, not of boxes. A component whose last text box is
// given every remaining millimetre of the region and then draws one line in it
// looks full by the box and empty to a reader.
func TestATallTextBoxDrawingOneLineDoesNotCountAsAFullRegion(t *testing.T) {
	_, _, manifest := buildTemplate(t, "")
	layout, _ := manifest.Layout(manifest.DefaultLayout)
	body, _ := layout.Slot(SlotBody)
	frame := Frame{X: body.X, Y: body.Y, Width: body.Width, Height: body.Height}
	block := Block{Kind: BlockSteps, Items: []Item{
		{Label: "준비", Value: "범위 확정"}, {Label: "이행", Value: "이관"}, {Label: "안정화", Value: "점검"}}}
	component := RenderBlock(NewDesign(manifest), frame, block)
	before, _, _ := drawnBounds(component.Primitives)
	centreInFrame(&component, frame)
	after, _, _ := drawnBounds(component.Primitives)
	if after <= before {
		t.Fatalf("the process was left at the top of its region: %d then %d", before, after)
	}
}

// A component that shares the page with prose stays where the prose starts. The
// two are one argument read in one direction.
func TestAComponentSharingTheSlideWithProseIsNotMoved(t *testing.T) {
	_, _, manifest := buildTemplate(t, "")
	layout, ok := manifest.LayoutForRole(RoleTwoContent)
	if !ok {
		t.Skip("this design has no two-content layout")
	}
	slots := layout.BodySlots()
	if len(slots) < 2 {
		t.Skip("the two-content layout has one region")
	}
	alone := Slide{LayoutID: layout.ID, Blocks: map[string]Block{
		slots[0].Slot: {Kind: BlockMeter, Items: []Item{{Label: "집행", Value: "68%"}}}}}
	if !alone.StandsAlone(layout, slots[0].Slot) {
		t.Fatal("a slide with one component does not count as standing alone")
	}
	shared := alone
	shared.Fields = map[string][]Paragraph{slots[1].Slot: {{Text: "직영 채널이 성장을 이끌었습니다"}}}
	if shared.StandsAlone(layout, slots[0].Slot) {
		t.Fatal("a component beside prose counts as standing alone")
	}
	// A title above it is not company; it is the component's own heading.
	titled := alone
	titled.Fields = map[string][]Paragraph{SlotTitle: {{Text: "예산 집행률"}}}
	if !titled.StandsAlone(layout, slots[0].Slot) {
		t.Fatal("a titled slide does not count as standing alone")
	}
}

// The exported chart and table are the same objects the drawing describes, so
// they move with it or the file and the screen disagree.
func TestARealChartMovesWithTheDrawingItStandsFor(t *testing.T) {
	_, _, manifest := buildTemplate(t, "")
	layout, _ := manifest.Layout(manifest.DefaultLayout)
	body, _ := layout.Slot(SlotBody)
	frame := Frame{X: body.X, Y: body.Y, Width: body.Width, Height: body.Height}
	component := Component{
		Primitives: []Primitive{{Kind: shapeRectangle, Frame: Frame{X: frame.X, Y: frame.Y, Width: 100, Height: 100}}},
		Chart:      &ChartPart{Frame: Frame{X: frame.X, Y: frame.Y, Width: 100, Height: 100}},
		Table:      &TablePart{Frame: Frame{X: frame.X, Y: frame.Y, Width: 100, Height: 100}},
	}
	centreInFrame(&component, frame)
	moved := component.Primitives[0].Frame.Y - frame.Y
	if moved <= 0 {
		t.Fatalf("nothing moved")
	}
	if component.Chart.Frame.Y-frame.Y != moved || component.Table.Frame.Y-frame.Y != moved {
		t.Fatalf("the chart and table were left behind: drawing %d, chart %d, table %d",
			moved, component.Chart.Frame.Y-frame.Y, component.Table.Frame.Y-frame.Y)
	}
}
