package deck

import (
	"fmt"
	"strings"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// CompileOptions controls how source is bound to a template.
type CompileOptions struct {
	// Accent colours the components. An empty value uses the template's own.
	Accent func(position int) string
	// Language is used for text measurement, since a Korean line holds far fewer
	// characters than an English one of the same width.
	Language string
}

// CompileResult is a compiled deck: slides ready to store, plus everything worth
// telling the author about what was adjusted.
type CompileResult struct {
	Slides   []model.Slide
	Outline  []OutlineEntry
	Warnings []string
}

// OutlineEntry is one line of the deck's structure, for the workspace.
type OutlineEntry struct {
	Position int    `json:"position"`
	Title    string `json:"title"`
	Layout   string `json:"layout"`
	Role     string `json:"role"`
}

// Compile binds parsed source to a template and produces storable slides.
//
// Everything the template knows is applied here: which layout suits each slide,
// which slots that layout actually has, how much text each slot holds, and which
// components fit. Source that asks for something the template cannot do is
// adjusted rather than rejected — a deck the author is waiting for should arrive,
// with a note about what changed.
func Compile(source Source, manifest pptx.Manifest, options CompileOptions) CompileResult {
	result := CompileResult{Warnings: source.Warnings}
	if len(manifest.Layouts) == 0 {
		result.Warnings = append(result.Warnings, "the template exposes no usable layout")
		return result
	}
	for index, slide := range source.Slides {
		layout, note := resolveSourceLayout(manifest, slide, index, len(source.Slides))
		if note != "" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("slide %d: %s", index+1, note))
		}
		content := Content{Type: ContentType, LayoutID: layout.ID}
		if options.Accent != nil {
			content.Accent = options.Accent(index)
		}

		title := strings.TrimSpace(slide.Title)
		if title == "" {
			title = firstSentence(slide.Lead)
		}
		if placeholder, ok := layout.Slot(pptx.SlotTitle); ok && title != "" {
			content.SetField(pptx.SlotTitle, fit(placeholder, []pptx.Paragraph{{Text: title}}, options.Language))
		}

		// A component claims its slot before prose does: a slot holds one or the
		// other, and the drawing is the more deliberate choice.
		claimed := map[string]bool{}
		for _, block := range slide.Blocks {
			placeholder, ok := blockSlot(layout, claimed)
			if !ok {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("slide %d: %s has no free region in layout %q and was written as text",
						index+1, block.Kind, layout.Name))
				slide.Bullets = append(slide.Bullets, blockAsBullets(block)...)
				continue
			}
			assembled := pptx.Block{Kind: block.Kind, Caption: block.Caption, Items: block.Items}
			sanitized, usable := pptx.SanitizeBlock(assembled, placeholder)
			if !usable {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("slide %d: %s did not have enough room and was written as text", index+1, block.Kind))
				slide.Bullets = append(slide.Bullets, blockAsBullets(block)...)
				continue
			}
			content.SetBlock(placeholder.Slot, sanitized)
			claimed[placeholder.Slot] = true
		}

		lead := strings.TrimSpace(slide.Lead)
		if lead != "" && lead != title {
			if placeholder, ok := leadSlot(layout, claimed); ok {
				content.SetField(placeholder.Slot, fit(placeholder, []pptx.Paragraph{{Text: lead}}, options.Language))
				claimed[placeholder.Slot] = true
			} else if len(slide.Bullets) == 0 {
				slide.Bullets = append(slide.Bullets, pptx.Paragraph{Text: lead})
			}
		}

		if len(slide.Bullets) > 0 {
			distributeBullets(&content, layout, claimed, slide.Bullets, options.Language, &result.Warnings, index+1)
		}

		notes := strings.TrimSpace(slide.Notes)
		result.Slides = append(result.Slides, model.Slide{
			Position:     index + 1,
			Title:        truncate(title, 200),
			Subtitle:     truncate(lead, 300),
			Content:      content.WithNotes(notes).Encode(),
			SpeakerNotes: truncate(notes, 4000),
			Layout:       layout.Role,
			LayoutID:     layout.ID,
		})
		result.Outline = append(result.Outline, OutlineEntry{
			Position: index + 1, Title: title, Layout: layout.Name, Role: layout.Role,
		})
	}
	return result
}

// WithNotes attaches speaker notes to content without disturbing the rest.
func (c Content) WithNotes(notes string) Content {
	c.Notes = notes
	return c
}

// resolveLayout picks the template layout a slide should use: an explicit
// request, then the role it declares, then the role its position implies.
func resolveSourceLayout(manifest pptx.Manifest, slide SourceSlide, index, total int) (pptx.Layout, string) {
	if id := strings.TrimSpace(slide.LayoutID); id != "" {
		if layout, ok := manifest.Layout(id); ok {
			return layout, ""
		}
		if layout, ok := manifest.LayoutForRole(sourcePositionRole(index, total)); ok {
			return layout, fmt.Sprintf("layout %q does not exist in this template; used %q instead", id, layout.Name)
		}
	}
	role := slide.Role
	if role == "" {
		role = sourcePositionRole(index, total)
	}
	if layout, ok := manifest.LayoutForRole(role); ok {
		return layout, ""
	}
	if layout, ok := manifest.LayoutForRole(pptx.RoleContent); ok {
		return layout, ""
	}
	return manifest.Layouts[0], ""
}

// positionRole is the shape of an unannotated deck: it opens, it argues, it ends.
func sourcePositionRole(index, total int) string {
	switch {
	case index == 0:
		return pptx.RoleTitle
	case total >= 3 && index == total-1:
		return pptx.RoleClosing
	default:
		return pptx.RoleContent
	}
}

// blockSlot finds the region a component should be drawn in: the largest free
// body slot, since a drawing wants room.
func blockSlot(layout pptx.Layout, claimed map[string]bool) (pptx.Placeholder, bool) {
	var best pptx.Placeholder
	found := false
	for _, placeholder := range layout.BodySlots() {
		if claimed[placeholder.Slot] || !placeholder.AcceptsText() {
			continue
		}
		if !found || placeholder.Width*placeholder.Height > best.Width*best.Height {
			best, found = placeholder, true
		}
	}
	return best, found
}

// leadSlot is where a lead line goes: the subtitle if the layout has one.
func leadSlot(layout pptx.Layout, claimed map[string]bool) (pptx.Placeholder, bool) {
	if placeholder, ok := layout.Slot(pptx.SlotSubtitle); ok && !claimed[placeholder.Slot] {
		return placeholder, true
	}
	return pptx.Placeholder{}, false
}

// distributeBullets fills the layout's body slots, splitting the list across
// columns when the layout has more than one.
func distributeBullets(content *Content, layout pptx.Layout, claimed map[string]bool,
	bullets []pptx.Paragraph, language string, warnings *[]string, position int) {
	var slots []pptx.Placeholder
	for _, placeholder := range layout.BodySlots() {
		if !claimed[placeholder.Slot] {
			slots = append(slots, placeholder)
		}
	}
	if len(slots) == 0 {
		// Nowhere to write: keep the text on the slide so nothing is silently lost.
		content.Body = paragraphsText(bullets)
		*warnings = append(*warnings,
			fmt.Sprintf("slide %d: layout %q has no free body region, so its points were kept as plain text",
				position, layout.Name))
		return
	}
	groups := splitEvenly(bullets, len(slots))
	for index, group := range groups {
		if len(group) == 0 {
			continue
		}
		content.SetField(slots[index].Slot, fit(slots[index], group, language))
	}
}

// splitEvenly divides a list across n columns, keeping a sub-bullet with the
// point it belongs to.
func splitEvenly(bullets []pptx.Paragraph, columns int) [][]pptx.Paragraph {
	if columns <= 1 {
		return [][]pptx.Paragraph{bullets}
	}
	// Count top-level points; sub-bullets travel with their parent.
	tops := 0
	for _, bullet := range bullets {
		if bullet.Level == 0 {
			tops++
		}
	}
	if tops <= 1 {
		return [][]pptx.Paragraph{bullets}
	}
	perColumn := (tops + columns - 1) / columns
	result := make([][]pptx.Paragraph, columns)
	column, taken := 0, 0
	for _, bullet := range bullets {
		if bullet.Level == 0 && taken >= perColumn && column < columns-1 {
			column++
			taken = 0
		}
		if bullet.Level == 0 {
			taken++
		}
		result[column] = append(result[column], bullet)
	}
	return result
}

func blockAsBullets(block SourceBlock) []pptx.Paragraph {
	result := make([]pptx.Paragraph, 0, len(block.Items)+1)
	if caption := strings.TrimSpace(block.Caption); caption != "" {
		result = append(result, pptx.Paragraph{Text: caption})
	}
	for _, item := range block.Items {
		text := strings.TrimSpace(item.Label)
		if value := strings.TrimSpace(item.Value); value != "" {
			if text == "" {
				text = value
			} else {
				text += ": " + value
			}
		}
		if text == "" {
			continue
		}
		level := 0
		if len(result) > 0 && strings.TrimSpace(block.Caption) != "" {
			level = 1
		}
		result = append(result, pptx.Paragraph{Text: text, Level: level})
	}
	return result
}

func paragraphsText(paragraphs []pptx.Paragraph) string {
	lines := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		lines = append(lines, strings.Repeat("  ", paragraph.Level)+paragraph.Text)
	}
	return strings.Join(lines, "\n")
}

// fit trims paragraphs to what a slot can hold, so no exported slide overflows
// its template's box.
func fit(placeholder pptx.Placeholder, paragraphs []pptx.Paragraph, language string) []pptx.Paragraph {
	return pptx.FitParagraphs(paragraphs, placeholder, language)
}

func firstSentence(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, terminator := range []string{". ", "? ", "! ", ".\n"} {
		if index := strings.Index(value, terminator); index > 0 {
			return strings.TrimSpace(value[:index+1])
		}
	}
	return value
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
