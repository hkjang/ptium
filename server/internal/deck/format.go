package deck

import (
	"fmt"
	"strings"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// Format writes stored slides back out as deck source.
//
// It is the inverse of Compile, which is what makes the source a real
// representation of the deck rather than a record of how it was once generated:
// a deck edited on the canvas can be opened as text, and text edited by hand can
// be drawn again. Compiling the output of Format reproduces the same slides.
func Format(presentation model.Presentation, manifest pptx.Manifest) string {
	var builder strings.Builder
	for index, slide := range presentation.Slides {
		if index > 0 {
			builder.WriteString("\n")
		}
		content := Decode(slide.Content)
		title := strings.TrimSpace(slide.Title)
		if title == "" {
			title = firstText(content.Fields[pptx.SlotTitle])
		}
		fmt.Fprintf(&builder, "# %s\n", escapeSourceLine(title))

		if role := sourceRole(slide, manifest); role != "" {
			fmt.Fprintf(&builder, "@%s\n", role)
		}
		lead := strings.TrimSpace(slide.Subtitle)
		if lead == "" {
			lead = firstText(content.Fields[pptx.SlotSubtitle])
		}
		if lead != "" {
			fmt.Fprintf(&builder, "> %s\n", escapeSourceLine(lead))
		}

		// Body slots in reading order, so the text comes back out in the order it
		// appears on the slide.
		layout, hasLayout := manifest.Layout(content.LayoutID)
		for _, slot := range bodySlotOrder(layout, hasLayout, content) {
			if block, ok := content.Blocks[slot]; ok {
				builder.WriteString(formatBlock(block))
				continue
			}
			for _, paragraph := range content.Fields[slot] {
				text := strings.TrimSpace(paragraph.Text)
				if text == "" {
					continue
				}
				fmt.Fprintf(&builder, "%s- %s\n", strings.Repeat("  ", paragraph.Level), escapeSourceLine(text))
			}
		}
		if body := strings.TrimSpace(content.Body); body != "" && len(content.Fields) == 0 {
			for _, line := range strings.Split(body, "\n") {
				if trimmed := strings.TrimSpace(line); trimmed != "" {
					fmt.Fprintf(&builder, "- %s\n", escapeSourceLine(trimmed))
				}
			}
		}
		notes := strings.TrimSpace(slide.SpeakerNotes)
		if notes == "" {
			notes = strings.TrimSpace(content.Notes)
		}
		if notes != "" {
			fmt.Fprintf(&builder, "!notes %s\n", strings.ReplaceAll(notes, "\n", " "))
		}
	}
	return builder.String()
}

// bodySlotOrder lists the slots holding content, preferring the layout's own
// reading order and falling back to whatever the slide happens to carry.
func bodySlotOrder(layout pptx.Layout, hasLayout bool, content Content) []string {
	seen := map[string]bool{pptx.SlotTitle: true, pptx.SlotSubtitle: true}
	var order []string
	if hasLayout {
		for _, placeholder := range layout.BodySlots() {
			if seen[placeholder.Slot] {
				continue
			}
			seen[placeholder.Slot] = true
			order = append(order, placeholder.Slot)
		}
	}
	for _, slot := range []string{pptx.SlotBody, "body2", "body3", "body4"} {
		if seen[slot] {
			continue
		}
		if _, ok := content.Fields[slot]; ok {
			seen[slot] = true
			order = append(order, slot)
			continue
		}
		if _, ok := content.Blocks[slot]; ok {
			seen[slot] = true
			order = append(order, slot)
		}
	}
	return order
}

// sourceRole records the slide's kind when it is not what its position implies,
// so recompiling lands on the same layout.
func sourceRole(slide model.Slide, manifest pptx.Manifest) string {
	role := strings.TrimSpace(slide.Layout)
	if role == "" {
		return ""
	}
	for alias, mapped := range roleAliases {
		if mapped == role && !strings.ContainsFunc(alias, func(character rune) bool { return character > 127 }) {
			// A layout chosen explicitly is recorded as a layout, not a role, when
			// the template has more than one layout for that role.
			if matches := layoutsForRole(manifest, role); matches > 1 && slide.LayoutID != "" {
				return "layout " + slide.LayoutID
			}
			return alias
		}
	}
	if slide.LayoutID != "" {
		return "layout " + slide.LayoutID
	}
	return ""
}

func layoutsForRole(manifest pptx.Manifest, role string) int {
	count := 0
	for _, layout := range manifest.Layouts {
		if layout.Role == role {
			count++
		}
	}
	return count
}

func formatBlock(block pptx.Block) string {
	var builder strings.Builder
	kind := blockSourceName(block.Kind)
	fmt.Fprintf(&builder, "::%s", kind)
	caption := strings.TrimSpace(block.Caption)
	if caption == "" {
		caption = strings.TrimSpace(block.Heading)
	}
	if caption != "" {
		fmt.Fprintf(&builder, " %s", escapeSourceLine(caption))
	}
	builder.WriteString("\n")
	if text := strings.TrimSpace(block.Text); text != "" {
		fmt.Fprintf(&builder, "- %s\n", escapeSourceLine(text))
	}
	for _, item := range block.Items {
		parts := []string{strings.TrimSpace(item.Label)}
		if value := strings.TrimSpace(item.Value); value != "" {
			parts = append(parts, value)
		} else if item.Number != nil {
			parts = append(parts, trimNumber(*item.Number))
		}
		if detail := strings.TrimSpace(item.Detail); detail != "" {
			if len(parts) == 1 {
				parts = append(parts, "")
			}
			parts = append(parts, detail)
		}
		fmt.Fprintf(&builder, "- %s\n", escapeSourceLine(strings.Join(parts, " | ")))
	}
	builder.WriteString("::\n")
	return builder.String()
}

// blockSourceName is the short word an author writes for a component.
func blockSourceName(kind string) string {
	switch kind {
	case pptx.BlockColumns:
		return "columns"
	case pptx.BlockBars:
		return "bars"
	case pptx.BlockLine:
		return "line"
	case pptx.BlockShare:
		return "share"
	}
	return kind
}

func trimNumber(value float64) string {
	text := fmt.Sprintf("%.2f", value)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	if text == "" || text == "-" {
		return "0"
	}
	return text
}

// escapeSourceLine keeps a line of content from being read back as a directive.
func escapeSourceLine(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if text == "" {
		return text
	}
	switch text[0] {
	case '#', '@', '>', '-', '*', '!', ':':
		return "​" + text
	}
	if strings.HasPrefix(text, "//") {
		return "​" + text
	}
	return text
}

func firstText(paragraphs []pptx.Paragraph) string {
	for _, paragraph := range paragraphs {
		if text := strings.TrimSpace(paragraph.Text); text != "" {
			return text
		}
	}
	return ""
}
