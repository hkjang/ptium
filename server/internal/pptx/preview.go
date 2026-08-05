package pptx

import (
	"fmt"
	"math"
	"strings"
)

// PreviewOptions tunes the SVG preview renderer.
type PreviewOptions struct {
	// Width is the rendered pixel width; the height follows the slide ratio.
	Width int
	// ShowEmptySlots draws the layout's own prompt text for slots the deck
	// leaves empty, which is what a template gallery wants to show.
	ShowEmptySlots bool
	// Media resolves a picture part to a data URI. Without it a template built
	// from photographs previews as an empty slide, which reads as the design
	// having been thrown away.
	Media MediaResolver
}

// PreviewSVG renders an approximate but faithful picture of a slide using the
// template's real geometry, palette and typography. It exists so the browser
// can show what the exported file will look like without a PowerPoint engine.
func PreviewSVG(manifest Manifest, layout Layout, slide Slide, options PreviewOptions) string {
	width, height := manifest.SlideWidth, manifest.SlideHeight
	if width <= 0 || height <= 0 {
		width, height = 12192000, 6858000
	}
	pixelWidth := options.Width
	if pixelWidth <= 0 {
		pixelWidth = 960
	}
	pixelHeight := pixelWidth * height / width
	background := layout.Background
	if background == "" {
		background = manifest.Theme.Color("lt1")
	}
	// Coordinates are emitted in CSS pixels rather than EMU. Font sizes then
	// land in a range every SVG renderer handles, instead of the six-digit
	// values a viewBox in EMU would require.
	scale := float64(pixelWidth) / float64(width)

	design := NewDesign(manifest)
	var builder strings.Builder
	fmt.Fprintf(&builder, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" role="img" preserveAspectRatio="xMidYMid meet">`,
		pixelWidth, pixelHeight, pixelWidth, pixelHeight)
	gradients := &gradientRegistry{}
	builder.WriteString(previewBackground(layout, background, pixelWidth, pixelHeight, options.Media, gradients))
	// A template's identity usually lives in its artwork rather than its colour
	// scheme, and it paints in document order underneath the placeholders.
	if len(layout.Artwork) > 0 {
		for _, piece := range layout.Artwork {
			builder.WriteString(previewArtwork(piece, scale, options.Media, gradients))
		}
	} else {
		// A manifest stored by an older release has only the flat decoration list.
		for _, decoration := range layout.Decorations {
			width := float64(decoration.Width) * scale
			height := float64(decoration.Height) * scale
			radius := 0.0
			if decoration.Round {
				radius = math.Min(width, height) / 2
			}
			fmt.Fprintf(&builder, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="%.1f" fill="#%s"/>`,
				float64(decoration.X)*scale, float64(decoration.Y)*scale, width, height, radius, decoration.Fill)
		}
	}
	spanned := slide.spannedSlots()
	for _, placeholder := range layout.Placeholders {
		if spanned[placeholder.Slot] {
			// Covered by a component placed in another region.
			continue
		}
		// An image occupies its slot the same way it does in the exported file.
		if picture, ok := slide.Pictures[placeholder.Slot]; ok && len(picture.Data) > 0 {
			builder.WriteString(previewSlidePicture(placeholder, picture, scale, gradients.clipID()))
			continue
		}
		if block, ok := slide.Blocks[placeholder.Slot]; ok && placeholder.AcceptsText() {
			frame := blockFrame(layout, placeholder, block)
			if component := RenderBlock(design, frame, block); len(component.Primitives) > 0 {
				builder.WriteString(component.SVG(scale))
				continue
			}
		}
		paragraphs := slide.Fields[placeholder.Slot]
		if len(paragraphs) == 0 {
			if !options.ShowEmptySlots {
				continue
			}
			builder.WriteString(previewEmptySlot(placeholder, manifest.Theme, scale))
			continue
		}
		if !placeholder.AcceptsText() {
			builder.WriteString(previewEmptySlot(placeholder, manifest.Theme, scale))
			continue
		}
		builder.WriteString(previewText(placeholder, paragraphs, manifest.Theme, scale))
	}
	builder.WriteString(gradients.defs())
	builder.WriteString(`</svg>`)
	return builder.String()
}

// PreviewLayoutSVG renders a layout on its own, using the prompt text the
// template author wrote into each placeholder.
func PreviewLayoutSVG(manifest Manifest, layout Layout, options PreviewOptions) string {
	slide := Slide{LayoutID: layout.ID, Fields: map[string][]Paragraph{}}
	for _, placeholder := range layout.Placeholders {
		if !placeholder.AcceptsText() {
			continue
		}
		text := strings.TrimSpace(placeholder.Prompt)
		if text == "" {
			text = defaultPromptFor(placeholder)
		}
		paragraphs := []Paragraph{{Text: text}}
		if placeholder.Slot != SlotTitle && placeholder.Slot != SlotSubtitle && placeholder.MaxLines > 2 {
			paragraphs = append(paragraphs,
				Paragraph{Text: strings.Repeat("·", 1) + " " + text, Level: 1},
				Paragraph{Text: strings.Repeat("·", 1) + " " + text, Level: 1})
		}
		slide.Fields[placeholder.Slot] = paragraphs
	}
	options.ShowEmptySlots = true
	return PreviewSVG(manifest, layout, slide, options)
}

func defaultPromptFor(placeholder Placeholder) string {
	switch placeholder.Slot {
	case SlotTitle:
		return "제목"
	case SlotSubtitle:
		return "부제목"
	}
	return "본문"
}

func previewText(placeholder Placeholder, paragraphs []Paragraph, theme Theme, scale float64) string {
	size := placeholder.FontSize
	if size <= 0 {
		size = 1800
	}
	shrink, _ := autofit(placeholder, paragraphs)
	// Hundredths of a point to points to pixels at 96 dpi.
	fontSize := float64(size) / 100 * shrink / 100 * (float64(EMUPerPoint) * scale)
	if fontSize < 4 {
		fontSize = 4
	}
	lineHeight := fontSize * 1.22
	lineEm := placeholder.LineEm
	if lineEm <= 0 && placeholder.MaxChars > 0 && placeholder.MaxLines > 0 {
		lineEm = float64(placeholder.MaxChars) / float64(placeholder.MaxLines) * referenceAdvance
	}
	if lineEm < 1 {
		lineEm = 1
	}
	lineEm = lineEm * 100 / max(shrink, 1)
	color := placeholder.Color
	if color == "" {
		color = theme.Color("tx1")
	}
	family := placeholder.Font
	if family == "" {
		family = theme.MinorLatin
	}
	weight := "400"
	if placeholder.Bold || placeholder.Slot == SlotTitle {
		weight = "700"
	}
	x := (float64(placeholder.X) + 91440) * scale
	y := (float64(placeholder.Y)+45720)*scale + fontSize
	var builder strings.Builder
	fmt.Fprintf(&builder, `<text x="%.1f" y="%.1f" fill="#%s" font-size="%.2f" font-weight="%s" font-family="%s, Malgun Gothic, Apple SD Gothic Neo, sans-serif" xml:space="preserve">`,
		x, y, color, fontSize, weight, escapeAttribute(fallbackFamily(family)))
	line := 0
	for _, paragraph := range paragraphs {
		indent := float64(paragraph.Level) * fontSize
		prefix := ""
		if paragraph.Level > 0 || (placeholder.Slot != SlotTitle && placeholder.Slot != SlotSubtitle) {
			prefix = "• "
		}
		available := lineEm - float64(paragraph.Level)*2
		if available < 1 {
			available = 1
		}
		for index, wrapped := range wrapLines(prefix+strings.TrimSpace(paragraph.Text), available) {
			offset := indent
			// A bullet's continuation lines hang under its text rather than under its
			// marker. A paragraph with no marker has nothing to hang from, and
			// indenting it made every wrapped title step to the right.
			if index > 0 && prefix != "" {
				offset += fontSize
			}
			fmt.Fprintf(&builder, `<tspan x="%.1f" y="%.1f">%s</tspan>`, x+offset, y+float64(line)*lineHeight, escapeText(wrapped))
			line++
		}
	}
	builder.WriteString(`</text>`)
	return builder.String()
}

func previewEmptySlot(placeholder Placeholder, theme Theme, scale float64) string {
	label := map[string]string{
		"picture": "이미지",
		"chart":   "차트",
		"table":   "표",
		"graphic": "그래픽",
	}[placeholder.Kind]
	if label == "" {
		return ""
	}
	color := theme.Color("accent1")
	x := float64(placeholder.X) * scale
	y := float64(placeholder.Y) * scale
	boxWidth := float64(placeholder.Width) * scale
	boxHeight := float64(placeholder.Height) * scale
	fontSize := boxHeight * 0.09
	if fontSize < 8 {
		fontSize = 8
	}
	if fontSize > 22 {
		fontSize = 22
	}
	return fmt.Sprintf(`<g><rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="%.1f" fill="none" stroke="#%s" stroke-width="1.5" stroke-dasharray="7 5" opacity="0.55"/>`+
		`<text x="%.1f" y="%.1f" fill="#%s" font-size="%.1f" text-anchor="middle" opacity="0.75" font-family="Malgun Gothic, sans-serif">%s</text></g>`,
		x, y, boxWidth, boxHeight, fontSize*0.4, color,
		x+boxWidth/2, y+boxHeight/2+fontSize/3, color, fontSize, escapeText(label))
}

func fallbackFamily(family string) string {
	family = strings.TrimSpace(family)
	if family == "" || strings.HasPrefix(family, "+") {
		return "sans-serif"
	}
	return family
}
