package pptx

import (
	"fmt"
	"math"
	"strconv"
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
	// Language is the deck's, for the few things the preview writes in words
	// rather than copying from the slide — the source line at its foot. Empty
	// reads as Korean, which is what the rest of the renderer assumes.
	Language string
	// Bare drops the template's background and artwork, drawing only what the
	// slide itself puts on the page. The canvas uses it to lift one region off a
	// slide as a transparent sprite it can drag, so what moves under the pointer
	// is the real drawing rather than an outline standing in for it.
	Bare bool
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
	if options.Bare {
		builder.WriteString(previewSlideBody(manifest, layout, slide, design, scale, options, gradients))
		builder.WriteString(gradients.defs())
		builder.WriteString(`</svg>`)
		return builder.String()
	}
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
	builder.WriteString(previewSlideBody(manifest, layout, slide, design, scale, options, gradients))
	builder.WriteString(gradients.defs())
	builder.WriteString(`</svg>`)
	return builder.String()
}

// previewSlideBody draws what the slide itself puts on the page: its regions in
// the layout's order, then the freeform objects above them.
func previewSlideBody(manifest Manifest, layout Layout, slide Slide, design Design,
	scale float64, options PreviewOptions, gradients *gradientRegistry) string {
	var builder strings.Builder
	spanned := slide.spannedSlots()
	for _, placeholder := range layout.Placeholders {
		if spanned[placeholder.Slot] {
			// Covered by a component placed in another region.
			continue
		}
		placeholder = slide.Place(placeholder)
		// An image occupies its slot the same way it does in the exported file.
		if picture, ok := slide.Pictures[placeholder.Slot]; ok && len(picture.Data) > 0 {
			builder.WriteString(previewSlidePicture(placeholder, picture, scale, gradients.clipID()))
			continue
		}
		if block, ok := slide.Blocks[placeholder.Slot]; ok && placeholder.AcceptsText() {
			frame := slide.blockFrame(layout, placeholder, block)
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
	for _, element := range slide.Elements {
		builder.WriteString(element.SVG(scale))
	}
	builder.WriteString(previewSourceNote(slideSourceNote(layout, slide, options.Language), manifest.Theme, scale))
	builder.WriteString(previewSlideNumber(layout, slide, manifest.Theme, scale))
	return builder.String()
}

// previewSlideNumber draws the page number where the export will put it, so the
// editor and the file agree about what the slide looks like.
func previewSlideNumber(layout Layout, slide Slide, theme Theme, scale float64) string {
	slot := layout.SlideNumber
	if slot == nil || slide.Number <= 0 || slide.HideNumber {
		return ""
	}
	size := float64(slot.FontSize)
	if size <= 0 {
		size = 1100
	}
	colour := strings.TrimPrefix(strings.TrimSpace(slot.Color), "#")
	if colour == "" {
		colour = theme.Color("tx1")
	}
	anchor, x := "end", float64(slot.X+slot.Width)*scale
	switch strings.TrimSpace(slot.Align) {
	case "l":
		anchor, x = "start", float64(slot.X)*scale
	case "ctr":
		anchor, x = "middle", float64(slot.X)*scale+float64(slot.Width)*scale/2
	}
	// Centred in its box, the way an anchored placeholder sets it.
	drawn := size / 100 * (float64(EMUPerPoint) * scale)
	y := float64(slot.Y)*scale + float64(slot.Height)*scale/2 + drawn*0.35
	return fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="%s" font-size="%.1f" fill="#%s"%s>%s</text>`,
		x, y, anchor, drawn, colour,
		fontAttribute(slot.Font), escapeText(strconv.Itoa(slide.Number)))
}

// fontAttribute names a typeface for SVG when the design asked for one.
func fontAttribute(font string) string {
	if strings.TrimSpace(font) == "" {
		return ""
	}
	return ` font-family="` + escapeAttribute(font) + `, sans-serif"`
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
	// Alignment and italics are only ever set where a slide overrides them, so the
	// default path draws exactly what it drew before.
	anchor, slant := "", ""
	x := (float64(placeholder.X) + float64(placeholder.TextInset())) * scale
	switch placeholder.Align {
	case "ctr":
		anchor, x = ` text-anchor="middle"`, (float64(placeholder.X)+float64(placeholder.Width)/2)*scale
	case "r":
		anchor, x = ` text-anchor="end"`, (float64(placeholder.X+placeholder.Width)-float64(placeholder.TextInset()))*scale
	}
	if placeholder.Italic {
		slant = ` font-style="italic"`
	}
	y := (float64(placeholder.Y)+45720)*scale + fontSize
	var builder strings.Builder
	fmt.Fprintf(&builder, `<text x="%.1f" y="%.1f" fill="#%s" font-size="%.2f" font-weight="%s"%s%s font-family="%s, Malgun Gothic, Apple SD Gothic Neo, sans-serif" xml:space="preserve">`,
		x, y, color, fontSize, weight, anchor, slant, escapeAttribute(fallbackFamily(family)))
	// A line's worth of position, in lines: a lead keeps a little air under it,
	// the same air the exported file gives it.
	position := 0.0
	for _, paragraph := range paragraphs {
		indent := float64(paragraph.Level) * fontSize
		prefix := ""
		if paragraph.Level > 0 || (placeholder.Slot != SlotTitle && placeholder.Slot != SlotSubtitle) {
			prefix = "• "
		}
		if paragraph.Lead {
			// The slide's own sentence, not one of its points.
			prefix = ""
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
			if anchor != "" {
				// A centred or right-set line hangs from its own edge; an indent
				// would push it off that edge rather than in from it.
				offset = 0
			}
			fmt.Fprintf(&builder, `<tspan x="%.1f" y="%.1f">%s</tspan>`, x+offset, y+position*lineHeight, escapeText(wrapped))
			position++
		}
		if paragraph.Lead {
			position += leadSpacing
		}
	}
	builder.WriteString(`</text>`)
	return builder.String()
}

// leadSpacing is the air under a lead, in lines. It matches the six points the
// exported file sets, so the preview and the file space the slide alike.
const leadSpacing = 0.3

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
