// Package deck bridges the stored representation of a presentation and the
// template renderer. It owns the slide content schema so the generator, the
// editor API and the exporter all agree on what a slide holds.
package deck

import (
	"encoding/json"
	"strings"

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
	Images  map[string]ContentImage `json:"images,omitempty"`
	Bullets []string                `json:"bullets,omitempty"`
	Body    string                  `json:"body,omitempty"`
	Accent  string                  `json:"accent,omitempty"`
	Notes   string                  `json:"notes,omitempty"`
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
	rendered := pptx.Slide{LayoutID: layout.ID, Fields: map[string][]pptx.Paragraph{}, Notes: slide.SpeakerNotes}
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
					rendered.Fields[target] = paragraphs
				}
			}
		}
	}
	// A subtitle written into a layout without one should not be lost.
	if _, ok := layout.Slot(pptx.SlotSubtitle); !ok && strings.TrimSpace(slide.Subtitle) != "" {
		if target, exists := firstBodySlot(layout); exists {
			existing := rendered.Fields[target]
			rendered.Fields[target] = append([]pptx.Paragraph{{Text: slide.Subtitle}}, existing...)
		}
	}
	return rendered
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
		Author:   author,
		Language: presentation.Language,
	}
	for index, slide := range presentation.Slides {
		layout := resolveLayout(manifest, slide, index, len(presentation.Slides))
		rendered := RenderSlide(slide, layout)
		if images != nil {
			for slot, placed := range Decode(slide.Content).Images {
				if _, ok := layout.Slot(slot); !ok {
					continue
				}
				picture, found := images(placed.AssetID)
				if !found {
					continue
				}
				picture.Caption = placed.Caption
				if rendered.Pictures == nil {
					rendered.Pictures = map[string]pptx.Picture{}
				}
				rendered.Pictures[slot] = picture
				// The slot holds the picture, not text left over from an earlier edit.
				delete(rendered.Fields, slot)
				delete(rendered.Blocks, slot)
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
		if layout, ok := manifest.Layout(slide.LayoutID); ok {
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
