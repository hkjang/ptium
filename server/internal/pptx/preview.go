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
	// Slides is how many slides the deck has, so a jump to one it does not have
	// is drawn as the words it is rather than as a link. The exported file and
	// the printed page both refuse such a jump; drawing it underlined and in the
	// link colour was the one surface still promising something to click. Zero
	// reads as "not told", and every jump is drawn as a link — which is what a
	// caller previewing a layout on its own wants.
	Slides int
	// PictureDensity is how many of a picture's own pixels each drawn unit is
	// worth embedding. Zero reads as a screen's two; an export to a page sets
	// more, because a drawn unit there is a point and a printer resolves far
	// more of them than a screen does.
	PictureDensity int
	// Bare drops the template's background and artwork, drawing only what the
	// slide itself puts on the page. The canvas uses it to lift one region off a
	// slide as a transparent sprite it can drag, so what moves under the pointer
	// is the real drawing rather than an outline standing in for it.
	Bare bool
	// Reveal draws only the first N points of the body, with the rest of the
	// slide laid out as if they were all there. It is how a slide is built up a
	// line at a time while presenting: the words that have not been said yet
	// are not on the wall, and the ones that have do not move when the next
	// arrives. Zero draws the whole slide, which is every other caller.
	Reveal int
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
// drawableRuns is a line's runs as the drawing can honour them: a jump to a
// slide the deck does not have carries no link, so it is drawn as the words
// somebody typed. The file and the paper already refuse it, and a drawing that
// underlines it in the link colour is promising a reader something to click
// that goes nowhere.
func drawableRuns(text string, slides int) []TextRun {
	runs := SplitRuns(text)
	if slides <= 0 {
		return runs
	}
	for index, run := range runs {
		if number, ok := SlideJump(run.Href); ok && (number < 1 || number > slides) {
			runs[index].Href = ""
		}
	}
	return runs
}

// previewLinkColor is the colour a link is drawn in: the template's own, which
// is what the exported file uses for it too.
func previewLinkColor(theme Theme) string {
	if color := theme.Color("hlink"); color != "" {
		return color
	}
	return theme.Color("tx1")
}

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
			builder.WriteString(previewSlidePicture(placeholder, picture, scale, gradients.clipID(), options.PictureDensity))
			continue
		}
		if block, ok := slide.Blocks[placeholder.Slot]; ok && placeholder.AcceptsText() {
			frame := slide.blockFrame(layout, placeholder, block)
			if component := RenderBlock(slide.withAccent(design), frame, block); len(component.Primitives) > 0 {
				if slide.StandsAlone(layout, placeholder.Slot) {
					centreInFrame(&component, frame)
				}
				builder.WriteString(component.SVG(scale, previewLinkColor(manifest.Theme), options.Slides))
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
		builder.WriteString(previewText(placeholder, paragraphs, manifest.Theme, scale, options.Reveal, options.Slides))
	}
	for _, element := range slide.Elements {
		// An empty text box is nothing, in the preview as in the file.
		if element.Kind == "text" && strings.TrimSpace(element.Text) == "" {
			continue
		}
		builder.WriteString(element.SVG(scale, options.PictureDensity))
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

// held is how many of a body's points are shown, when a slide is being built up
// a line at a time. Everything else on the slide — its title, its lead, its
// components — is not a point and is not held back.
func held(placeholder Placeholder, reveal int) int {
	if reveal <= 0 || placeholder.Slot == SlotTitle || placeholder.Slot == SlotSubtitle {
		return 0
	}
	return reveal
}

func previewText(placeholder Placeholder, paragraphs []Paragraph, theme Theme, scale float64, reveal, slides int) string {
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
	// A link is drawn in the template's own hyperlink colour, which is what the
	// exported file uses for it too.
	linkColor := theme.Color("hlink")
	if linkColor == "" {
		linkColor = color
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
	fmt.Fprintf(&builder, `<text x="%.1f" y="%.1f" fill="#%s" font-size="%.2f" font-weight="%s"%s%s font-family="%s, %s" xml:space="preserve">`,
		x, y, color, fontSize, weight, anchor, slant, escapeAttribute(fallbackFamily(family)), previewFallbacks)
	// A line's worth of position, in lines: a lead keeps a little air under it,
	// the same air the exported file gives it.
	position := 0.0
	// A point not yet spoken is drawn invisible rather than left out, so the
	// ones already on the wall do not move when it arrives.
	shown, points := held(placeholder, reveal), 0
	for _, paragraph := range paragraphs {
		hidden := ""
		if !paragraph.Lead {
			points++
			if shown > 0 && points > shown {
				hidden = ` opacity="0"`
			}
		}
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
		runs := drawableRuns(strings.TrimSpace(paragraph.Text), slides)
		for index, wrapped := range wrapLines(prefix+PlainText(strings.TrimSpace(paragraph.Text)), available) {
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
			fmt.Fprintf(&builder, `<tspan x="%.1f" y="%.1f"%s>%s</tspan>`, x+offset, y+position*lineHeight,
				hidden, markedUpLine(wrapped, runs, linkColor))
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
		`<text x="%.1f" y="%.1f" fill="#%s" font-size="%.1f" text-anchor="middle" opacity="0.75" font-family="`+previewFallbacks+`">%s</text></g>`,
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

// previewFallbacks is what a preview falls back to when the template's own font
// is not on the machine looking at it.
//
// It used to be "Malgun Gothic, Apple SD Gothic Neo, sans-serif". A template
// set in Aptos — which ships with Microsoft 365 and nothing else — drawn on a
// machine with neither Aptos nor a Windows Korean font lands on whatever the
// browser calls sans-serif, and on Linux that is usually DejaVu Sans, whose
// lowercase runs a fifth wider than the humanist faces every template is set
// in. An English title measured to fit its box was drawn past the edge of the
// slide, because SVG cannot reflow the line it is handed.
//
// The Latin fallbacks come first and are all near half an em: the browser picks
// per character, so Hangul still finds the Korean faces further down the list.
//
// Every writing system the product generates in is named. A Japanese deck was
// drawn with 遅, 効, 満, 処 and a dozen others as empty boxes: the list named
// Korean faces and no Japanese ones, and a Korean font covers the hanja Korean
// uses and not the kanji it does not.
const previewFallbacks = `Segoe UI, Roboto, Noto Sans, Helvetica Neue, Arial, Liberation Sans, ` +
	`Malgun Gothic, Apple SD Gothic Neo, Noto Sans KR, ` +
	`Yu Gothic, Hiragino Sans, Meiryo, Noto Sans JP, ` +
	`Microsoft YaHei, PingFang SC, Noto Sans SC, sans-serif`

// markedUpLine draws one wrapped line, underlining the stretches of it that
// carry a link.
//
// The line has already been wrapped, so a label can straddle the break; the
// half that is on this line is found by looking for it, and a label that is not
// found is drawn as the words it is. Underlined and in the link colour is what
// a reader needs from a picture of a slide — the click itself belongs to the
// file and to the workspace, which draw the same text with something behind it.
func markedUpLine(line string, runs []TextRun, linkColor string) string {
	linked := false
	for _, run := range runs {
		if run.Href != "" || run.Bold || run.Italic {
			linked = true
			break
		}
	}
	if !linked {
		return escapeText(line)
	}
	var builder strings.Builder
	rest := line
	for _, run := range runs {
		if run.Text == "" || (run.Href == "" && !run.Bold && !run.Italic) {
			continue
		}
		at := strings.Index(rest, run.Text)
		if at < 0 {
			continue
		}
		builder.WriteString(escapeText(rest[:at]))
		marked := `<tspan` + runStyleSVG(run, linkColor) + `>` + escapeText(run.Text) + `</tspan>`
		if run.Href != "" {
			// A drawing of a slide is a picture in most of the places it is used
			// and a page in one: presenting draws this markup itself, so the link
			// is a link there. A jump names the slide it goes to, which is the
			// only part the page around it has to understand.
			target := run.Href
			if number, ok := SlideJump(target); ok {
				target = fmt.Sprintf("#slide-%d", number)
			}
			marked = `<a href="` + escapeAttribute(target) + `" target="_blank" rel="noreferrer noopener">` +
				marked + `</a>`
		}
		builder.WriteString(marked)
		rest = rest[at+len(run.Text):]
	}
	builder.WriteString(escapeText(rest))
	return builder.String()
}

// runStyleSVG is what one marked stretch of a line looks like: the same weight,
// slant and colour the exported file gives it.
func runStyleSVG(run TextRun, linkColor string) string {
	style := ""
	if run.Bold {
		style += ` font-weight="700"`
	}
	if run.Italic {
		style += ` font-style="italic"`
	}
	if run.Href != "" {
		style += ` fill="#` + escapeAttribute(linkColor) + `" text-decoration="underline"`
	}
	return style
}
