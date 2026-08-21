package pptx

import "strings"

// Region kinds, as an editor thinks of them.
const (
	RegionText      = "text"
	RegionComponent = "component"
	RegionPicture   = "picture"
	RegionEmpty     = "empty"
)

// Region is one thing a slide draws inside a template region, described for an
// editor rather than for the renderer.
//
// The canvas needs this to make generated content editable: without it, a
// generated slide is a picture, and the only thing a person can do to a picture
// is draw on top of it.
type Region struct {
	Slot string `json:"slot"`
	Kind string `json:"kind"`
	// Frame is where the region draws, in EMUs, after any per-slide override.
	Frame Frame `json:"frame"`
	// Layout is where the template would put it. A region the author dragged
	// reports a Frame that differs from this, which is how "원래 자리로" works.
	Layout     Frame       `json:"layout"`
	Moved      bool        `json:"moved"`
	Paragraphs []Paragraph `json:"paragraphs,omitempty"`
	Block      *Block      `json:"block,omitempty"`
	// FontSize is in hundredths of a point, as the manifest records it.
	FontSize int    `json:"fontSize,omitempty"`
	Bold     bool   `json:"bold,omitempty"`
	Color    string `json:"color,omitempty"`
	Font     string `json:"font,omitempty"`
	Name     string `json:"name,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
	// Accepts reports whether text may be written into the slot at all: a picture
	// placeholder holds an image and nothing else.
	Accepts bool `json:"acceptsText"`
	// Spanned regions are covered by a component placed in another slot. They are
	// reported so the editor can explain why the region cannot be clicked.
	Spanned string `json:"spannedBy,omitempty"`
}

// Text joins a text region's paragraphs the way an editor shows them.
func (r Region) Text() string {
	lines := make([]string, 0, len(r.Paragraphs))
	for _, paragraph := range r.Paragraphs {
		lines = append(lines, strings.Repeat("  ", max(0, paragraph.Level))+paragraph.Text)
	}
	return strings.Join(lines, "\n")
}

// SlideRegions describes every region of a drawn slide, in the layout's own
// order. Empty regions are included: an editor has to be able to put something
// into a region the generator left alone.
func SlideRegions(layout Layout, slide Slide) []Region {
	spannedBy := map[string]string{}
	for slot, block := range slide.Blocks {
		for _, other := range block.Span {
			if other != slot {
				spannedBy[other] = slot
			}
		}
	}
	regions := make([]Region, 0, len(layout.Placeholders))
	for _, original := range layout.Placeholders {
		placeholder := slide.Place(original)
		region := Region{
			Slot:     placeholder.Slot,
			Kind:     RegionEmpty,
			Frame:    Frame{X: placeholder.X, Y: placeholder.Y, Width: placeholder.Width, Height: placeholder.Height},
			Layout:   Frame{X: original.X, Y: original.Y, Width: original.Width, Height: original.Height},
			FontSize: placeholder.FontSize,
			Bold:     placeholder.Bold,
			Color:    placeholder.Color,
			Font:     placeholder.Font,
			Name:     placeholder.Name,
			Prompt:   placeholder.Prompt,
			Accepts:  placeholder.AcceptsText(),
			Spanned:  spannedBy[placeholder.Slot],
		}
		region.Moved = region.Frame != region.Layout
		switch {
		case region.Spanned != "":
			// Nothing of its own is drawn here.
		case len(slide.Pictures[placeholder.Slot].Data) > 0:
			region.Kind = RegionPicture
		default:
			if block, ok := slide.Blocks[placeholder.Slot]; ok && placeholder.AcceptsText() {
				copied := block
				region.Kind = RegionComponent
				region.Block = &copied
				region.Frame = slide.blockFrame(layout, original, block)
				break
			}
			if paragraphs := slide.Fields[placeholder.Slot]; len(paragraphs) > 0 {
				region.Kind = RegionText
				region.Paragraphs = paragraphs
			}
		}
		regions = append(regions, region)
	}
	return regions
}
