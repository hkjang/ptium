package pptx

import (
	"fmt"
	"strconv"
	"strings"
)

// sourceNote is the line a slide prints at its own foot: where the figures on
// it came from.
//
// The notes page has always carried the full list, and still does. But the file
// is what circulates — it is forwarded, printed and read months later by people
// who never open the speaker notes — and a figure whose source is only in the
// notes reads, to them, as a figure with no source. Every deck that has to
// survive that reading prints the line under the chart.
//
// It goes in the band the design already keeps clear for the page number, at
// the number's own size and colour, so it costs the design nothing. When that
// band is not clear — a template with no page number, or content that runs into
// it — the slide keeps its face and the notes keep the sources.
type sourceNote struct {
	Text   string
	X, Y   int
	Width  int
	Height int
	// Inset is the padding the slide's own text has inside its box, so the line
	// begins under the first letter of the body rather than beside it.
	Inset    int
	FontSize int
	Color    string
	Font     string
}

// sourceNoteGap is the space kept between the line and the page number, so the
// two never read as one string.
const sourceNoteGap = 228600

func slideSourceNote(layout Layout, slide Slide, language string) *sourceNote {
	slot := layout.SlideNumber
	if slot == nil || len(slide.Sources) == 0 {
		return nil
	}
	text := sourceLine(slide.Sources, language)
	if text == "" {
		return nil
	}
	left, right, inset := noteBand(layout, slot)
	if right-left < 1828800 {
		// Less than two inches is not a line, it is a fragment.
		return nil
	}
	size := slot.FontSize
	if size <= 0 {
		size = 1100
	}
	note := &sourceNote{
		Text: text, X: left, Y: slot.Y, Width: right - left, Height: slot.Height,
		Inset: inset, FontSize: size, Color: slot.Color, Font: slot.Font,
	}
	if overlapsUsedRegion(layout, slide, note) {
		return nil
	}
	note.Text = fitOneLine(text, size, note.Width-inset)
	return note
}

// noteBand is the horizontal room the line has: from the left edge of the
// slide's own content to wherever the page number starts.
func noteBand(layout Layout, slot *SlideNumberSlot) (int, int, int) {
	left, inset := slot.X, DefaultTextInset
	for _, placeholder := range layout.Placeholders {
		if placeholder.Width <= 0 || placeholder.X >= slot.X {
			continue
		}
		if placeholder.X < left {
			left, inset = placeholder.X, placeholder.TextInset()
		}
	}
	right := slot.X - sourceNoteGap
	if strings.TrimSpace(slot.Align) == "l" {
		// A number set on the left leaves the room on the other side.
		left, right = slot.X+slot.Width+sourceNoteGap, layoutRightEdge(layout, slot)
	}
	return left, right, inset
}

func layoutRightEdge(layout Layout, slot *SlideNumberSlot) int {
	right := slot.X + slot.Width
	for _, placeholder := range layout.Placeholders {
		if edge := placeholder.X + placeholder.Width; edge > right {
			right = edge
		}
	}
	return right
}

// overlapsUsedRegion reports whether anything the slide actually shows reaches
// into the line's band. A design that runs its body to the bottom edge keeps its
// design; the sources stay in the notes.
func overlapsUsedRegion(layout Layout, slide Slide, note *sourceNote) bool {
	spanned := slide.spannedSlots()
	for _, placeholder := range layout.Placeholders {
		if spanned[placeholder.Slot] {
			continue
		}
		placeholder = slide.Place(placeholder)
		if !slideUsesSlot(slide, placeholder.Slot) {
			continue
		}
		if overlaps(placeholder.X, placeholder.Y, placeholder.Width, placeholder.Height, note) {
			return true
		}
	}
	for _, element := range slide.Elements {
		if overlaps(element.Frame.X, element.Frame.Y, element.Frame.Width, element.Frame.Height, note) {
			return true
		}
	}
	return false
}

func slideUsesSlot(slide Slide, slot string) bool {
	if len(slide.Fields[slot]) > 0 {
		return true
	}
	if _, ok := slide.Blocks[slot]; ok {
		return true
	}
	if picture, ok := slide.Pictures[slot]; ok && len(picture.Data) > 0 {
		return true
	}
	return false
}

func overlaps(x, y, width, height int, note *sourceNote) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	return x < note.X+note.Width && x+width > note.X &&
		y < note.Y+note.Height && y+height > note.Y
}

// sourceLine is what the foot of the slide says. One source reads as a
// sentence; several are numbered the way the notes page numbers them, so a
// marker on the slide and the entry in the notes are the same thing.
func sourceLine(sources []Citation, language string) string {
	label := "출처"
	switch describeLanguage(language) {
	case "en":
		label = "Source"
		if len(sources) > 1 {
			label = "Sources"
		}
	case "ja":
		label = "出典"
	case "zh":
		label = "来源"
	}
	entries := make([]string, 0, len(sources))
	for index, source := range sources {
		title := strings.TrimSpace(source.Title)
		if title == "" {
			continue
		}
		entry := title
		if locator := strings.TrimSpace(source.Locator); locator != "" {
			entry += ", " + locator
		}
		if len(sources) > 1 {
			mark := strings.TrimSpace(source.Marker)
			if mark == "" {
				mark = strconv.Itoa(index + 1)
			}
			entry = mark + ". " + entry
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return ""
	}
	return label + ": " + strings.Join(entries, "  ")
}

// fitOneLine keeps the line to the room it has. What it drops is never lost —
// the notes page lists every source in full — but the reader should be able to
// tell that there is more, so the cut is marked.
func fitOneLine(text string, size, width int) string {
	if textWidth(text, size) <= width {
		return text
	}
	runes := []rune(text)
	for length := len(runes) - 1; length > 0; length-- {
		candidate := strings.TrimRight(string(runes[:length]), " ,·")
		if textWidth(candidate+"…", size) <= width {
			return candidate + "…"
		}
	}
	return text
}

// sourceNoteXML draws the line as an ordinary text box rather than a
// placeholder: it belongs to this slide, not to the design, and nobody editing
// the template should find an empty "sources" box waiting on every layout.
func sourceNoteXML(shapeID int, note *sourceNote, language string, links *linkTable) string {
	if note == nil {
		return ""
	}
	if language == "" {
		language = "ko-KR"
	}
	properties := `<a:rPr lang="` + escapeAttribute(language) + `" sz="` + strconv.Itoa(note.FontSize) + `" dirty="0">`
	if colour := strings.TrimPrefix(strings.TrimSpace(note.Color), "#"); colour != "" {
		properties += `<a:solidFill><a:srgbClr val="` + escapeAttribute(colour) + `"/></a:solidFill>`
	}
	if font := strings.TrimSpace(note.Font); font != "" {
		properties += latinTypefaceXML(font)
	}
	properties += `</a:rPr>`
	return `<p:sp><p:nvSpPr><p:cNvPr id="` + strconv.Itoa(shapeID) + `" name="Source Note"` +
		` descr="` + escapeAttribute(PlainText(note.Text)) + `"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="` + strconv.Itoa(note.X) + `" y="` + strconv.Itoa(note.Y) + `"/>` +
		`<a:ext cx="` + strconv.Itoa(note.Width) + `" cy="` + strconv.Itoa(note.Height) + `"/></a:xfrm>` +
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr>` +
		`<p:txBody><a:bodyPr lIns="` + strconv.Itoa(note.Inset) + `" tIns="0" rIns="0" bIns="0" anchor="ctr" wrap="none"/><a:lstStyle/>` +
		// The line is on the slide, so a link written in it is one of the slide's:
		// "출처: [분기 보고서](https://…)" draws the words and clicks through, the
		// same as any other line of the deck.
		`<a:p><a:pPr algn="l"><a:buNone/></a:pPr>` + runsXML(note.Text, properties, links) +
		`<a:endParaRPr lang="` + escapeAttribute(language) + `"/></a:p></p:txBody></p:sp>`
}

// previewSourceNote draws the same line on screen, in the same place.
func previewSourceNote(note *sourceNote, theme Theme, scale float64) string {
	if note == nil {
		return ""
	}
	colour := strings.TrimPrefix(strings.TrimSpace(note.Color), "#")
	if colour == "" {
		colour = theme.Color("tx1")
	}
	drawn := float64(note.FontSize) / 100 * (float64(EMUPerPoint) * scale)
	y := float64(note.Y)*scale + float64(note.Height)*scale/2 + drawn*0.35
	// The exported line draws its links; the picture of it says the same, or the
	// two disagree about a line that is on the same slide.
	linkColor := theme.Color("hlink")
	if linkColor == "" {
		linkColor = colour
	}
	runs := SplitRuns(note.Text)
	return fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="start" font-size="%.1f" fill="#%s"%s>%s</text>`,
		float64(note.X+note.Inset)*scale, y, drawn, colour, fontAttribute(note.Font),
		markedUpLine(PlainText(note.Text), runs, linkColor))
}
