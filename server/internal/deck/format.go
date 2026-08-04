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
			if picture, ok := content.Images[slot]; ok {
				builder.WriteString(formatImage(picture))
				continue
			}
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
		// Text kept in Body had no slot to live in; it is still part of the deck and
		// has to come back out, or a round trip would delete it.
		if body := strings.TrimSpace(content.Body); body != "" && !wroteBody(content) {
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

// wroteBody reports whether any body region carried content, as opposed to the
// text having been kept aside in Content.Body.
func wroteBody(content Content) bool {
	for slot := range content.Fields {
		if slot != pptx.SlotTitle && slot != pptx.SlotSubtitle {
			return true
		}
	}
	return len(content.Blocks) > 0 || len(content.Images) > 0
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
	for _, slot := range []string{pptx.SlotBody, pptx.SlotPicture, "body2", "body3", "body4"} {
		if seen[slot] {
			continue
		}
		_, hasField := content.Fields[slot]
		_, hasBlock := content.Blocks[slot]
		_, hasImage := content.Images[slot]
		if hasField || hasBlock || hasImage {
			seen[slot] = true
			order = append(order, slot)
		}
	}
	// A picture region is not a body region, so it is not in BodySlots; an image
	// placed there still has to be written out.
	for slot := range content.Images {
		if !seen[slot] {
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
		if slide.LayoutID != "" {
			return "layout " + slide.LayoutID
		}
		return ""
	}
	// A template with several layouts for one role needs the layout named, or
	// recompiling would move the slide to whichever one ranks first.
	if layoutsForRole(manifest, role) > 1 && slide.LayoutID != "" {
		return "layout " + slide.LayoutID
	}
	if name, ok := canonicalRoleName[role]; ok {
		return name
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

// formatImage writes an image reference back out, preferring the name the author
// used over the id they never typed.
func formatImage(picture ContentImage) string {
	reference := strings.TrimSpace(picture.Name)
	if reference == "" {
		reference = picture.AssetID
	}
	line := "::image " + escapeItemField(reference)
	if caption := strings.TrimSpace(picture.Caption); caption != "" {
		line += " | " + escapeItemField(caption)
	}
	return line + "\n"
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
		parts := []string{escapeItemField(item.Label)}
		if value := strings.TrimSpace(item.Value); value != "" {
			parts = append(parts, escapeItemField(value))
		} else if item.Number != nil {
			parts = append(parts, trimNumber(*item.Number))
		}
		if detail := strings.TrimSpace(item.Detail); detail != "" {
			if len(parts) == 1 {
				parts = append(parts, "")
			}
			parts = append(parts, escapeItemField(detail))
		}
		fmt.Fprintf(&builder, "- %s\n", strings.Join(parts, " | "))
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

// escapeSourceLine protects text that a directive would otherwise misread.
//
// Every line Format writes begins with its own marker, so most content needs no
// escaping at all: "- - dash" reads back as a bullet whose text is "- dash". Only
// a leading hash matters, because a title strips its whole run of them, and a
// leading backslash, because that is the escape itself.
func escapeSourceLine(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if text == "" {
		return text
	}
	if text[0] == '#' || text[0] == '\\' {
		return `\` + text
	}
	return text
}

// escapeItemField protects a component row's fields, which are separated by
// pipes, so a label may contain one.
func escapeItemField(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	text = strings.ReplaceAll(text, `\`, `\\`)
	return strings.ReplaceAll(text, "|", `\|`)
}

func firstText(paragraphs []pptx.Paragraph) string {
	for _, paragraph := range paragraphs {
		if text := strings.TrimSpace(paragraph.Text); text != "" {
			return text
		}
	}
	return ""
}
