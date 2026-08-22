package pptx

import "strings"

// centreInFrame moves a component down so that what it draws sits in the middle
// of the region it was given, rather than hanging from the top of it.
//
// Components are laid out from the top, which is right when they fill their
// region. A gauge with one bar, or a comparison of two things, is three
// centimetres tall in a region twelve centimetres deep — drawn from the top it
// leaves the bottom three quarters of the slide empty and reads as a slide
// somebody abandoned halfway. Centred, the same drawing reads as a deliberate
// statement with room around it, which is what it is.
//
// The whole component moves together, heading and caption included: they belong
// to the drawing, and separating them would open a gap where a title used to be.
//
// Only a slide whose component is the whole content is centred. Where a
// component shares the page with prose, the two start together at the top —
// they are one argument read in one direction, and centring half of it would
// break the line the eye follows.
func centreInFrame(component *Component, frame Frame) {
	if len(component.Primitives) == 0 || frame.Height <= 0 {
		return
	}
	top, bottom, found := drawnBounds(component.Primitives)
	if !found {
		return
	}
	above := top - frame.Y
	below := frame.Y + frame.Height - bottom
	shift := (below - above) / 2
	// A component that nearly fills its region is already where it belongs, and
	// one that overflows must not be pushed further off the page.
	if shift <= 0 || below <= 0 {
		return
	}
	moveComponent(component, shift)
}

// StandsAlone reports whether this component is everything the slide has to
// say, apart from its title.
func (s Slide) StandsAlone(layout Layout, slot string) bool {
	if len(s.Elements) > 0 {
		return false
	}
	spanned := s.spannedSlots()
	for _, placeholder := range layout.Placeholders {
		if placeholder.Slot == slot || placeholder.Slot == SlotTitle || spanned[placeholder.Slot] {
			continue
		}
		if slideUsesSlot(s, placeholder.Slot) {
			return false
		}
	}
	return true
}

// drawnBounds is the top and bottom of everything a component draws.
func drawnBounds(primitives []Primitive) (int, int, bool) {
	top, bottom, found := 0, 0, false
	consider := func(y, height int) {
		if !found {
			top, bottom, found = y, y+height, true
			return
		}
		top = min(top, y)
		bottom = max(bottom, y+height)
	}
	for _, primitive := range primitives {
		bounds := inkBounds(primitive)
		consider(bounds.Y, bounds.Height)
	}
	return top, bottom, found
}

// inkBounds is the room a primitive's ink actually takes.
//
// A text box is routinely far larger than what it draws: given every remaining
// millimetre of a region and then drawing one line, or made as wide as the gap
// between two ticks so that a two-character month label can be centred on one.
// Measuring the box says the component fills the slide, and says the axis
// labels of every line chart sit on top of each other. Measuring the ink says
// what a reader sees.
func inkBounds(primitive Primitive) Frame {
	bounds := primitive.bounds()
	if primitive.Kind != shapeText {
		return bounds
	}
	if height, _ := drawnTextHeight(primitive); height > 0 && height < bounds.Height {
		switch strings.TrimSpace(primitive.Anchor) {
		case "ctr":
			bounds.Y += (bounds.Height - height) / 2
		case "b":
			bounds.Y += bounds.Height - height
		}
		bounds.Height = height
	}
	// Wrapped text uses the whole width to wrap into, so its longest line is at
	// least as wide as the box and nothing is narrowed.
	widest := 0
	for _, paragraph := range primitive.Lines {
		widest = max(widest, textWidth(paragraph.Text, primitive.FontSize))
	}
	if widest > 0 && widest < bounds.Width {
		switch strings.TrimSpace(primitive.Align) {
		case "ctr":
			bounds.X += (bounds.Width - widest) / 2
		case "r":
			bounds.X += bounds.Width - widest
		}
		bounds.Width = widest
	}
	return bounds
}

func moveComponent(component *Component, shift int) {
	for index := range component.Primitives {
		component.Primitives[index].Frame.Y += shift
		for point := range component.Primitives[index].Points {
			component.Primitives[index].Points[point].Y += shift
		}
	}
	if component.Table != nil {
		component.Table.Frame.Y += shift
	}
	if component.Chart != nil {
		component.Chart.Frame.Y += shift
	}
}
