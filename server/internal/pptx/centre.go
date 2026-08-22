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

// inkBounds is the room a primitive's ink actually takes. A text box is often
// given every remaining millimetre of the region and then draws one line in it:
// measuring the box would say the component fills the slide when what a reader
// sees is a sentence at the top of an empty page.
func inkBounds(primitive Primitive) Frame {
	bounds := primitive.bounds()
	if primitive.Kind != shapeText {
		return bounds
	}
	height, _ := drawnTextHeight(primitive)
	if height <= 0 || height >= bounds.Height {
		return bounds
	}
	switch strings.TrimSpace(primitive.Anchor) {
	case "ctr":
		bounds.Y += (bounds.Height - height) / 2
	case "b":
		bounds.Y += bounds.Height - height
	}
	bounds.Height = height
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
