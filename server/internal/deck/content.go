// Package deck bridges the stored representation of a presentation and the
// template renderer. It owns the slide content schema so the generator, the
// editor API and the exporter all agree on what a slide holds.
package deck

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// ContentType marks slide payloads written against the template schema.
const ContentType = "template"

// Content is the JSON stored in slides.content. Fields is the authoritative
// mapping from template slot to text; Bullets mirrors the primary body slot so
// older clients and the legacy exporter keep working.
type Content struct {
	Type     string                      `json:"type"`
	LayoutID string                      `json:"layoutId,omitempty"`
	Fields   map[string][]pptx.Paragraph `json:"fields,omitempty"`
	// Blocks maps a slot to the visual component drawn in it. A slot holds
	// either paragraphs or a component, never both.
	Blocks map[string]pptx.Block `json:"blocks,omitempty"`
	// Images maps a slot to a stored image drawn in it. A slot holds text, a
	// component or an image — never two of them.
	Images map[string]ContentImage `json:"images,omitempty"`
	// Elements are freely positioned objects layered over the template. Their
	// geometry uses slide percentages, so the same payload works with every
	// aspect ratio and can be mapped losslessly to browser CSS and PPTX EMUs.
	Elements []FreeformElement `json:"elements,omitempty"`
	// Frames moves or resizes a template region on this slide alone. A slot with
	// no entry keeps the layout's own geometry, so a deck stays inside its
	// template until someone deliberately drags a region somewhere else.
	Frames map[string]SlotFrame `json:"frames,omitempty"`
	// Styles changes how a region's text is set on this slide: its size, colour,
	// weight and alignment. A slot with no entry keeps the template's own type.
	Styles  map[string]pptx.Style `json:"styles,omitempty"`
	Bullets []string              `json:"bullets,omitempty"`
	Body    string                `json:"body,omitempty"`
	Accent  string                `json:"accent,omitempty"`
	Notes   string                `json:"notes,omitempty"`
	// Sources are where this slide's figures came from. They travel with the
	// slide rather than in a table of their own, so duplicating a deck, restoring
	// a version or exporting a file carries the evidence with the claim.
	Sources []pptx.Citation `json:"sources,omitempty"`
}

// MaxSlideSources caps what one slide can cite. A slide that needs more than a
// dozen sources is a document, not a slide.
const MaxSlideSources = 12

// ValidateSlideSources checks what a caller sends before it is stored.
func ValidateSlideSources(sources []pptx.Citation) error {
	if len(sources) > MaxSlideSources {
		return fmt.Errorf("a slide may cite at most %d sources", MaxSlideSources)
	}
	for index, source := range sources {
		if strings.TrimSpace(source.Title) == "" {
			return fmt.Errorf("source %d has no title", index+1)
		}
		if utf8.RuneCountInString(source.Title) > 300 || utf8.RuneCountInString(source.Locator) > 200 ||
			utf8.RuneCountInString(source.Marker) > 3 {
			return fmt.Errorf("source %d is longer than it may be", index+1)
		}
	}
	return nil
}

const MaxFreeformElements = 200

// SlotFrame is where a template region draws on one slide, in percentages of
// the slide. Percentages rather than EMUs, for the same reason freeform objects
// use them: the browser and the exported file then agree without either side
// knowing the other's units.
type SlotFrame struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// MaxSlotFrames bounds the override map. A layout has a handful of regions; a
// hundred entries is already a client sending nonsense.
const MaxSlotFrames = 100

// ValidateSlotFrames keeps a moved region on, or near, the slide. The bounds
// match the freeform ones: a little overhang is a legitimate design, a region
// parked ten slides to the left is a bug.
func ValidateSlotFrames(frames map[string]SlotFrame) error {
	if len(frames) > MaxSlotFrames {
		return fmt.Errorf("a slide may override at most %d regions", MaxSlotFrames)
	}
	for slot, frame := range frames {
		if strings.TrimSpace(slot) == "" || utf8.RuneCountInString(slot) > 100 {
			return fmt.Errorf("region override %q names an invalid slot", slot)
		}
		for _, value := range []float64{frame.X, frame.Y, frame.Width, frame.Height} {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf("region override %q contains a non-finite number", slot)
			}
		}
		if frame.X < -100 || frame.X > 200 || frame.Y < -100 || frame.Y > 200 ||
			frame.Width < 1 || frame.Width > 300 || frame.Height < 1 || frame.Height > 300 {
			return fmt.Errorf("region override %q is outside the supported canvas range", slot)
		}
	}
	return nil
}

// ValidateSlotStyles bounds the typography a slide may override. Anything past
// these is a client sending nonsense rather than a person setting type.
func ValidateSlotStyles(styles map[string]pptx.Style) error {
	if len(styles) > MaxSlotFrames {
		return fmt.Errorf("a slide may restyle at most %d regions", MaxSlotFrames)
	}
	for slot, style := range styles {
		if strings.TrimSpace(slot) == "" || utf8.RuneCountInString(slot) > 100 {
			return fmt.Errorf("region style %q names an invalid slot", slot)
		}
		if math.IsNaN(style.Scale) || math.IsInf(style.Scale, 0) || style.Scale < 0 || style.Scale > 3 {
			return fmt.Errorf("region style %q has an unsupported size", slot)
		}
		if style.Color != "" && !isHexColor(style.Color) {
			return fmt.Errorf("region style %q has an invalid colour", slot)
		}
		if !oneOf(strings.ToLower(strings.TrimSpace(style.Align)), "", "left", "center", "right", "justify") {
			return fmt.Errorf("region style %q has an unsupported alignment", slot)
		}
	}
	return nil
}

// isHexColor accepts the RRGGBB the renderer writes into DrawingML.
func isHexColor(value string) bool {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdefABCDEF", character) {
			return false
		}
	}
	return true
}

// slotFramesInEMU converts the stored percentages to the renderer's units.
func slotFramesInEMU(frames map[string]SlotFrame, slideWidth, slideHeight int) map[string]pptx.Frame {
	if len(frames) == 0 {
		return nil
	}
	converted := make(map[string]pptx.Frame, len(frames))
	for slot, frame := range frames {
		converted[slot] = pptx.Frame{
			X:      int(math.Round(frame.X / 100 * float64(slideWidth))),
			Y:      int(math.Round(frame.Y / 100 * float64(slideHeight))),
			Width:  max(1, int(math.Round(frame.Width/100*float64(slideWidth)))),
			Height: max(1, int(math.Round(frame.Height/100*float64(slideHeight)))),
		}
	}
	return converted
}

// normalizedStyles drops the entries that change nothing and the colours written
// with a leading hash, which is how a browser hands them over.
func normalizedStyles(styles map[string]pptx.Style) map[string]pptx.Style {
	if len(styles) == 0 {
		return nil
	}
	cleaned := make(map[string]pptx.Style, len(styles))
	for slot, style := range styles {
		style.Color = strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(style.Color), "#"))
		if style.Empty() {
			continue
		}
		cleaned[slot] = style
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}

// FreeformElement is one Google-Slides-style object on top of a template:
// text, an editable shape, a line, or an image from the owner's asset library.
// X/Y/Width/Height are percentages of the slide canvas. Opacity is 1..100;
// zero is treated as the backwards-compatible default of fully opaque.
type FreeformElement struct {
	ID            string     `json:"id"`
	Kind          string     `json:"kind"`
	Shape         string     `json:"shape,omitempty"`
	X             float64    `json:"x"`
	Y             float64    `json:"y"`
	Width         float64    `json:"width"`
	Height        float64    `json:"height"`
	Rotation      float64    `json:"rotation,omitempty"`
	ZIndex        int        `json:"zIndex,omitempty"`
	Text          string     `json:"text,omitempty"`
	Cells         [][]string `json:"cells,omitempty"`
	HeaderRows    int        `json:"headerRows,omitempty"`
	HeaderColumns int        `json:"headerColumns,omitempty"`
	FontFamily    string     `json:"fontFamily,omitempty"`
	FontSize      float64    `json:"fontSize,omitempty"`
	TextColor     string     `json:"textColor,omitempty"`
	Bold          bool       `json:"bold,omitempty"`
	Italic        bool       `json:"italic,omitempty"`
	Underline     bool       `json:"underline,omitempty"`
	Align         string     `json:"align,omitempty"`
	VerticalAlign string     `json:"verticalAlign,omitempty"`
	Fill          string     `json:"fill,omitempty"`
	Stroke        string     `json:"stroke,omitempty"`
	StrokeWidth   float64    `json:"strokeWidth,omitempty"`
	StartArrow    string     `json:"startArrow,omitempty"`
	EndArrow      string     `json:"endArrow,omitempty"`
	Dash          string     `json:"dash,omitempty"`
	Opacity       int        `json:"opacity,omitempty"`
	AssetID       string     `json:"assetId,omitempty"`
	Name          string     `json:"name,omitempty"`
	Caption       string     `json:"caption,omitempty"`
	Fit           string     `json:"fit,omitempty"`
	GroupID       string     `json:"groupId,omitempty"`
	Locked        bool       `json:"locked,omitempty"`
	Hidden        bool       `json:"hidden,omitempty"`
}

// ValidateFreeformElements bounds stored canvas data before it reaches either
// renderer. Rendering always escapes text, but limits still prevent a single
// slide from becoming an unbounded document inside a document.
func ValidateFreeformElements(elements []FreeformElement) error {
	if len(elements) > MaxFreeformElements {
		return fmt.Errorf("a slide may contain at most %d freeform elements", MaxFreeformElements)
	}
	seen := make(map[string]bool, len(elements))
	for index, element := range elements {
		position := index + 1
		if strings.TrimSpace(element.ID) == "" || utf8.RuneCountInString(element.ID) > 100 {
			return fmt.Errorf("freeform element %d has an invalid id", position)
		}
		if seen[element.ID] {
			return fmt.Errorf("freeform element %d repeats id %q", position, element.ID)
		}
		seen[element.ID] = true
		switch element.Kind {
		case "text", "shape", "line", "image", "table":
		default:
			return fmt.Errorf("freeform element %d has unsupported kind %q", position, element.Kind)
		}
		for _, value := range []float64{element.X, element.Y, element.Width, element.Height, element.Rotation, element.FontSize, element.StrokeWidth} {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf("freeform element %d contains a non-finite number", position)
			}
		}
		if element.X < -100 || element.X > 200 || element.Y < -100 || element.Y > 200 ||
			element.Width < 0.1 || element.Width > 300 || element.Height < 0.1 || element.Height > 300 {
			return fmt.Errorf("freeform element %d geometry is outside the supported canvas range", position)
		}
		if element.Rotation < -3600 || element.Rotation > 3600 || element.ZIndex < -10000 || element.ZIndex > 10000 {
			return fmt.Errorf("freeform element %d transform is outside the supported range", position)
		}
		if utf8.RuneCountInString(element.Text) > 20000 || utf8.RuneCountInString(element.Caption) > 1000 ||
			utf8.RuneCountInString(element.FontFamily) > 200 || utf8.RuneCountInString(element.GroupID) > 100 ||
			utf8.RuneCountInString(element.Name) > 300 {
			return fmt.Errorf("freeform element %d text exceeds its allowed length", position)
		}
		if !pptx.AlignmentIsKnown(element.Align, element.VerticalAlign) {
			return fmt.Errorf("freeform element %d has an unsupported alignment", position)
		}
		if element.Kind == "table" {
			if len(element.Cells) == 0 || len(element.Cells) > 50 {
				return fmt.Errorf("freeform table %d must contain between 1 and 50 rows", position)
			}
			columns := len(element.Cells[0])
			if columns == 0 || columns > 20 {
				return fmt.Errorf("freeform table %d must contain between 1 and 20 columns", position)
			}
			totalText := 0
			for rowIndex, row := range element.Cells {
				if len(row) != columns {
					return fmt.Errorf("freeform table %d row %d has a different column count", position, rowIndex+1)
				}
				for _, cell := range row {
					length := utf8.RuneCountInString(cell)
					if length > 1000 {
						return fmt.Errorf("freeform table %d contains a cell longer than 1000 characters", position)
					}
					totalText += length
				}
			}
			if totalText > 20000 || element.HeaderRows < 0 || element.HeaderRows > len(element.Cells) ||
				element.HeaderColumns < 0 || element.HeaderColumns > columns {
				return fmt.Errorf("freeform table %d structure is outside the supported range", position)
			}
		}
		if element.FontSize < 0 || element.FontSize > 400 || element.StrokeWidth < 0 || element.StrokeWidth > 50 ||
			element.Opacity < 0 || element.Opacity > 100 {
			return fmt.Errorf("freeform element %d style is outside the supported range", position)
		}
		if len(element.Fill) > 32 || len(element.Stroke) > 32 || len(element.TextColor) > 32 {
			return fmt.Errorf("freeform element %d has an invalid colour", position)
		}
		if element.Kind == "image" && strings.TrimSpace(element.AssetID) == "" {
			return fmt.Errorf("freeform image %d does not reference an asset", position)
		}
		if !oneOf(element.StartArrow, "", "none", "triangle", "stealth", "diamond", "oval") ||
			!oneOf(element.EndArrow, "", "none", "triangle", "stealth", "diamond", "oval") ||
			!oneOf(element.Dash, "", "solid", "dash", "dot", "dashDot") {
			return fmt.Errorf("freeform element %d has an unsupported line style", position)
		}
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

// ContentImage is an image placed on a slide, by reference. The bytes stay in
// the asset store: a deck holds pointers, so the same logo on twenty slides is
// stored once and can be replaced in one place.
type ContentImage struct {
	AssetID string `json:"assetId"`
	// Name is what the author wrote in the source, kept so the text can be
	// written back out even if the image is later deleted.
	Name    string `json:"name,omitempty"`
	Caption string `json:"caption,omitempty"`
}

// Decode parses stored slide content, tolerating payloads written before
// templates existed.
func Decode(raw json.RawMessage) Content {
	content := Content{}
	if len(raw) == 0 {
		return content
	}
	if err := json.Unmarshal(raw, &content); err != nil {
		return Content{}
	}
	return content
}

// Encode serializes content for storage.
func (c Content) Encode() json.RawMessage {
	if c.Type == "" {
		c.Type = ContentType
	}
	c.Bullets = c.PrimaryBullets()
	encoded, err := json.Marshal(c)
	if err != nil {
		return json.RawMessage(`{"type":"` + ContentType + `"}`)
	}
	return encoded
}

// PrimaryBullets flattens the first body slot into plain strings.
func (c Content) PrimaryBullets() []string {
	for _, slot := range []string{pptx.SlotBody, "body2", "body3", "body4"} {
		paragraphs, ok := c.Fields[slot]
		if !ok || len(paragraphs) == 0 {
			continue
		}
		result := make([]string, 0, len(paragraphs))
		for _, paragraph := range paragraphs {
			prefix := strings.Repeat("  ", paragraph.Level)
			result = append(result, prefix+paragraph.Text)
		}
		return result
	}
	if len(c.Bullets) > 0 {
		return c.Bullets
	}
	if strings.TrimSpace(c.Body) != "" {
		return strings.Split(c.Body, "\n")
	}
	return nil
}

// SetImage places an image in a slot.
func (c *Content) SetImage(slot string, image ContentImage) {
	if strings.TrimSpace(image.AssetID) == "" {
		return
	}
	if c.Images == nil {
		c.Images = map[string]ContentImage{}
	}
	c.Images[slot] = image
}

// SetField replaces a slot's paragraphs, dropping empty entries.
func (c *Content) SetField(slot string, paragraphs []pptx.Paragraph) {
	cleaned := make([]pptx.Paragraph, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		paragraph.Text = strings.TrimSpace(paragraph.Text)
		if paragraph.Text == "" {
			continue
		}
		if paragraph.Level < 0 {
			paragraph.Level = 0
		}
		if paragraph.Level > 4 {
			paragraph.Level = 4
		}
		cleaned = append(cleaned, paragraph)
	}
	if len(cleaned) == 0 {
		return
	}
	if c.Fields == nil {
		c.Fields = map[string][]pptx.Paragraph{}
	}
	c.Fields[slot] = cleaned
}

// SetText is a convenience wrapper for single-line slots.
func (c *Content) SetText(slot, value string) {
	c.SetField(slot, []pptx.Paragraph{{Text: value}})
}

// SetBlock places a visual component in a slot, replacing any prose there.
func (c *Content) SetBlock(slot string, block pptx.Block) {
	if c.Blocks == nil {
		c.Blocks = map[string]pptx.Block{}
	}
	c.Blocks[slot] = block
	delete(c.Fields, slot)
}

// RenderSlide converts a stored slide into renderer input for a layout. Slides
// written before templates existed are mapped onto the layout's title,
// subtitle and first body slot so old decks still export correctly.
func RenderSlide(slide model.Slide, layout pptx.Layout) pptx.Slide {
	content := Decode(slide.Content)
	rendered := pptx.Slide{LayoutID: layout.ID, Fields: map[string][]pptx.Paragraph{}, Notes: slide.SpeakerNotes,
		Sources: content.Sources}
	for slot, paragraphs := range content.Fields {
		if _, ok := layout.Slot(slot); !ok {
			continue
		}
		rendered.Fields[slot] = paragraphs
	}
	for slot, block := range content.Blocks {
		if _, ok := layout.Slot(slot); !ok {
			continue
		}
		if rendered.Blocks == nil {
			rendered.Blocks = map[string]pptx.Block{}
		}
		rendered.Blocks[slot] = block
	}
	if _, ok := rendered.Fields[pptx.SlotTitle]; !ok && strings.TrimSpace(slide.Title) != "" {
		if _, exists := layout.Slot(pptx.SlotTitle); exists {
			rendered.Fields[pptx.SlotTitle] = []pptx.Paragraph{{Text: slide.Title}}
		}
	}
	if _, ok := rendered.Fields[pptx.SlotSubtitle]; !ok && strings.TrimSpace(slide.Subtitle) != "" {
		if _, exists := layout.Slot(pptx.SlotSubtitle); exists {
			rendered.Fields[pptx.SlotSubtitle] = []pptx.Paragraph{{Text: slide.Subtitle}}
		}
	}
	if (len(rendered.Fields) == 0 || !hasBody(rendered.Fields)) && len(rendered.Blocks) == 0 {
		if bullets := content.PrimaryBullets(); len(bullets) > 0 {
			if target, ok := firstBodySlot(layout); ok {
				paragraphs := make([]pptx.Paragraph, 0, len(bullets))
				for _, bullet := range bullets {
					trimmed := strings.TrimSpace(bullet)
					if trimmed == "" {
						continue
					}
					paragraphs = append(paragraphs, pptx.Paragraph{Text: trimmed, Level: indentLevel(bullet)})
				}
				if len(paragraphs) > 0 {
					// A layout with no body region gives the subtitle as the target,
					// and the subtitle already holds the slide's lead. Replacing it
					// dropped the lead off a closing page — the one slide whose ask
					// nobody can afford to lose — so the points follow it instead.
					rendered.Fields[target] = append(rendered.Fields[target], paragraphs...)
				}
			}
		}
	}
	// A subtitle written into a layout without one should not be lost — but it must
	// not be written twice either, and it must not be dropped on top of a
	// component. A lead line is stored both in the slide's subtitle column and in
	// whichever region compilation gave it, so this only has work to do when that
	// region is gone.
	if _, ok := layout.Slot(pptx.SlotSubtitle); !ok {
		if subtitle := strings.TrimSpace(slide.Subtitle); subtitle != "" && !rendered.Carries(subtitle) {
			if target, exists := freeBodySlot(layout, rendered); exists {
				existing := rendered.Fields[target]
				rendered.Fields[target] = append([]pptx.Paragraph{{Text: subtitle}}, existing...)
			} else if slot, block, exists := headlessBlock(rendered); exists {
				// Every region is drawn: the lead belongs above the component as its
				// heading, which is where the compiler would have put it.
				block.Heading = subtitle
				rendered.Blocks[slot] = block
			}
		}
	}
	return rendered
}

// headlessBlock is a component on the slide that has no heading of its own.
func headlessBlock(rendered pptx.Slide) (string, pptx.Block, bool) {
	slots := make([]string, 0, len(rendered.Blocks))
	for slot := range rendered.Blocks {
		slots = append(slots, slot)
	}
	sort.Strings(slots)
	for _, slot := range slots {
		if strings.TrimSpace(rendered.Blocks[slot].Heading) == "" {
			return slot, rendered.Blocks[slot], true
		}
	}
	return "", pptx.Block{}, false
}

// Build converts a whole presentation into renderer input.
func Build(presentation model.Presentation, manifest pptx.Manifest, author string) pptx.Deck {
	return BuildWithImages(presentation, manifest, author, nil)
}

// ImageSource supplies the bytes of a stored image. A deck references images;
// rendering has to carry them, so the caller that owns the store passes this in.
type ImageSource func(assetID string) (pptx.Picture, bool)

// BuildWithImages is Build with the deck's images resolved.
func BuildWithImages(presentation model.Presentation, manifest pptx.Manifest, author string, images ImageSource) pptx.Deck {
	result := pptx.Deck{
		Title:    presentation.Title,
		Subject:  presentation.Prompt,
		Brief:    presentation.Prompt,
		Author:   author,
		Language: presentation.Language,
		// The exported file carries the text the deck was written from, so
		// importing it back gives the deck rather than a reading of its drawing.
		Source: presentation.Source,
	}
	slideWidth, slideHeight := manifest.SlideWidth, manifest.SlideHeight
	if slideWidth <= 0 || slideHeight <= 0 {
		slideWidth, slideHeight = 12192000, 6858000
	}
	for index, slide := range presentation.Slides {
		layout := resolveLayout(manifest, slide, index, len(presentation.Slides))
		rendered := RenderSlide(slide, layout)
		// Where the slide sits in the deck, so the export and every preview agree
		// about the page number. A cover carries none, the way covers do not.
		rendered.Number = index + 1
		rendered.HideNumber = layout.Role == pptx.RoleTitle
		content := Decode(slide.Content)
		// A region someone dragged on the canvas moves for this slide only; the
		// layout it came from is untouched, so every other slide keeps the design.
		rendered.Frames = slotFramesInEMU(content.Frames, slideWidth, slideHeight)
		rendered.Styles = normalizedStyles(content.Styles)
		if images != nil {
			for slot, placed := range content.Images {
				if _, ok := layout.Slot(slot); !ok {
					continue
				}
				picture, found := images(placed.AssetID)
				if !found {
					continue
				}
				if strings.TrimSpace(placed.Caption) != "" {
					picture.Caption = placed.Caption
				}
				if rendered.Pictures == nil {
					rendered.Pictures = map[string]pptx.Picture{}
				}
				rendered.Pictures[slot] = picture
				// The slot holds the picture, not text left over from an earlier edit.
				delete(rendered.Fields, slot)
				delete(rendered.Blocks, slot)
			}
		}
		if len(content.Elements) > 0 {
			type orderedElement struct {
				position int
				element  FreeformElement
			}
			ordered := make([]orderedElement, 0, len(content.Elements))
			for position, element := range content.Elements {
				if !element.Hidden {
					ordered = append(ordered, orderedElement{position: position, element: element})
				}
			}
			sort.SliceStable(ordered, func(i, j int) bool {
				if ordered[i].element.ZIndex == ordered[j].element.ZIndex {
					return ordered[i].position < ordered[j].position
				}
				return ordered[i].element.ZIndex < ordered[j].element.ZIndex
			})
			for _, item := range ordered {
				element := item.element
				built := pptx.Element{
					ID: element.ID, Kind: element.Kind, Shape: element.Shape,
					Frame: pptx.Frame{
						X:      int(math.Round(element.X / 100 * float64(slideWidth))),
						Y:      int(math.Round(element.Y / 100 * float64(slideHeight))),
						Width:  int(math.Round(element.Width / 100 * float64(slideWidth))),
						Height: int(math.Round(element.Height / 100 * float64(slideHeight))),
					},
					Rotation: element.Rotation, ZIndex: element.ZIndex, Text: element.Text,
					Cells: element.Cells, HeaderRows: element.HeaderRows, HeaderColumns: element.HeaderColumns,
					FontFamily: element.FontFamily, FontSize: int(math.Round(element.FontSize * 100)),
					TextColor: element.TextColor, Bold: element.Bold, Italic: element.Italic, Underline: element.Underline,
					Align: element.Align, VerticalAlign: element.VerticalAlign, Fill: element.Fill, Stroke: element.Stroke,
					StrokeWidth: int(math.Round(element.StrokeWidth * float64(pptx.EMUPerPoint))), Opacity: element.Opacity,
					StartArrow: element.StartArrow, EndArrow: element.EndArrow, Dash: element.Dash,
					Fit: element.Fit, Caption: element.Caption, Locked: element.Locked,
				}
				if element.Kind == "image" && images != nil {
					if picture, found := images(element.AssetID); found {
						if strings.TrimSpace(element.Caption) != "" {
							picture.Caption = element.Caption
						}
						built.Picture = &picture
					}
				}
				rendered.Elements = append(rendered.Elements, built)
			}
		}
		result.Slides = append(result.Slides, rendered)
	}
	return result
}

// resolveLayout picks the template layout for a slide, honouring an explicit
// layout id and otherwise inferring one from the slide's narrative role.
func resolveLayout(manifest pptx.Manifest, slide model.Slide, index, total int) pptx.Layout {
	if slide.LayoutID != "" {
		if layout, ok := manifest.LayoutByReference(slide.LayoutID); ok {
			return layout
		}
	}
	role := RoleForLegacyLayout(slide.Layout, index, total)
	if layout, ok := manifest.LayoutForRole(role); ok {
		return layout
	}
	return pptx.Layout{}
}

// RoleForLegacyLayout maps the free-form layout label older decks stored onto
// a narrative role the template engine understands.
func RoleForLegacyLayout(label string, index, total int) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "title", "cover":
		return pptx.RoleTitle
	case "section", "sectionheader", "divider":
		return pptx.RoleSection
	case "quote":
		return pptx.RoleQuote
	case "closing", "thanks", "end":
		return pptx.RoleClosing
	case "two", "twocontent", "comparison":
		return pptx.RoleTwoContent
	case "picture", "image":
		return pptx.RolePicture
	}
	switch {
	case index == 0:
		return pptx.RoleTitle
	case total > 1 && index == total-1:
		return pptx.RoleClosing
	}
	return pptx.RoleContent
}

// freeBodySlot is the roomiest body region nothing has been drawn into yet.
// Writing text into a region that already holds a component or a picture puts
// two things in one place, which is a collision, not a layout.
func freeBodySlot(layout pptx.Layout, rendered pptx.Slide) (string, bool) {
	best, found := pptx.Placeholder{}, false
	for _, placeholder := range layout.BodySlots() {
		if _, taken := rendered.Blocks[placeholder.Slot]; taken {
			continue
		}
		if _, taken := rendered.Pictures[placeholder.Slot]; taken {
			continue
		}
		if !found || placeholder.Width*placeholder.Height > best.Width*best.Height {
			best, found = placeholder, true
		}
	}
	if found {
		return best.Slot, true
	}
	return "", false
}

func hasBody(fields map[string][]pptx.Paragraph) bool {
	for slot := range fields {
		if slot != pptx.SlotTitle && slot != pptx.SlotSubtitle {
			return true
		}
	}
	return false
}

// firstBodySlot is where a deck's text goes when nothing else has claimed a
// region: the roomiest body region, not merely the first one. A layout may put a
// one-line eyebrow above its title, and reading order would hand the deck's
// content to that.
func firstBodySlot(layout pptx.Layout) (string, bool) {
	best, found := pptx.Placeholder{}, false
	for _, placeholder := range layout.BodySlots() {
		if !found || placeholder.Width*placeholder.Height > best.Width*best.Height {
			best, found = placeholder, true
		}
	}
	if found {
		return best.Slot, true
	}
	if _, ok := layout.Slot(pptx.SlotSubtitle); ok {
		return pptx.SlotSubtitle, true
	}
	return "", false
}

func indentLevel(bullet string) int {
	level := 0
	for strings.HasPrefix(bullet, "  ") {
		bullet = bullet[2:]
		level++
	}
	if level > 4 {
		return 4
	}
	return level
}
