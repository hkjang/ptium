package export

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pdf"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// ErrNothingToPrint is a deck whose every slide is kept out of the talk. It is
// not a broken deck and it is not a broken server: there is simply no page to
// print, and the person asking is the one who can fix it.
var ErrNothingToPrint = errors.New("every slide in the deck is skipped, so there is nothing to print")

// PDF puts the deck on paper: one page per slide, at the deck's own size.
//
// Each page is the drawing the workspace already makes of that slide —
// the same one the rail, the preview, the shared link and the presenting screen
// show — translated into PDF. Nothing here lays a slide out a second time,
// because a second layout is a second answer to where a line sits, and the two
// would drift.
//
// The type is not the deck's. A PowerPoint template names its typeface and does
// not carry it, and a PDF reader has no Korean face of its own, so the file is
// set in the one face the workspace ships. The exported pptx is where the
// deck's own design lives; this is where it can be read anywhere.
// PDF prints the deck. What the built-in face could not draw is reported by
// PDFWithMissing; a caller that does not ask is not told.
func PDF(presentation model.Presentation, options Options) ([]byte, error) {
	data, _, err := PDFWithMissing(presentation, options)
	return data, err
}

// PDFWithMissing prints the deck and says which characters the built-in face
// has no glyph for.
//
// The face covers Korean, Latin and kana. Japanese 新字体 and simplified
// Chinese reach past it — 売, 実, 业, 绩 — and those characters leave the page
// as blank space. Printing them away silently is the worst of the three
// possible answers; this one is told to the author while the file is still on
// screen.
func PDFWithMissing(presentation model.Presentation, options Options) ([]byte, []rune, error) {
	if len(presentation.Slides) == 0 {
		return nil, nil, errors.New("the presentation does not contain any slide")
	}
	manifest := options.Manifest
	if manifest.Version != pptx.ManifestVersion || len(manifest.Layouts) == 0 {
		if len(options.TemplateData) == 0 {
			return nil, nil, errors.New("the presentation template is unavailable")
		}
		_, analyzed, err := pptx.AnalyzeBytes(options.TemplateData)
		if err != nil {
			return nil, nil, fmt.Errorf("analyze presentation template: %w", err)
		}
		manifest = analyzed
	}
	font, err := pdf.BuiltinFont()
	if err != nil {
		return nil, nil, fmt.Errorf("read the built-in font: %w", err)
	}
	widthEMU, heightEMU := manifest.SlideWidth, manifest.SlideHeight
	if widthEMU <= 0 || heightEMU <= 0 {
		widthEMU, heightEMU = 12192000, 6858000
	}
	// A point is 12700 EMU. Asking the drawing for exactly as many pixels as the
	// page has points puts the two in the same units, so a font size in the
	// drawing is the same number of points on paper.
	width := float64(widthEMU) / 12700
	height := float64(heightEMU) / 12700
	document := pdf.New(width, height, presentation.Title, font)
	built := deck.BuildWithImages(presentation, manifest, options.Author, options.Images)
	printed := printedPages(built.Slides)
	for index, slide := range built.Slides {
		if slide.Skipped {
			// A slide kept out of the talk is kept out of the handout: the paper
			// is what the room is given, and PowerPoint hides it too.
			continue
		}
		layout, ok := manifest.Layout(slide.LayoutID)
		if !ok {
			if layout, ok = manifest.LayoutForRole(pptx.RoleContent); !ok {
				continue
			}
		}
		slide.Number = index + 1
		// A page is not a screen: it is drawn in points and printed at far more
		// dots than a point, so a picture is embedded for the paper rather than
		// for a monitor.
		drawing := onPrintedPages(pptx.PreviewSVG(manifest, layout, slide, pptx.PreviewOptions{
			Width: int(width), Media: options.Media, Language: presentation.Language,
			PictureDensity: pptx.PrintPictureDensity}), printed)
		page := document.AddPage()
		if !options.WithNotes {
			if err := pdf.DrawSVG(page, drawing); err != nil {
				return nil, nil, fmt.Errorf("draw slide %d: %w", index+1, err)
			}
			continue
		}
		if err := printWithNotes(document, page, drawing, slide, presentation.Language, width, height, printed); err != nil {
			return nil, nil, fmt.Errorf("draw slide %d: %w", index+1, err)
		}
	}
	if document.Pages() == 0 {
		return nil, nil, ErrNothingToPrint
	}
	return document.Bytes(), missingRunes(document), nil
}

// printedPages maps a slide of the deck to the page it is printed on.
//
// The handout leaves out the slides the author is skipping, so the deck's third
// slide is not the third page. A link written [3장](#3) names the deck's third
// slide, and it was handed to the paper as the number alone: on a deck with one
// skipped slide before it, clicking it in the PDF turned to the page after the
// one it names. A jump naming a slide that is not printed goes to the next one
// that is, which is what presenting and a shared link both do; a jump past the
// end of the deck names no page at all, and no link is drawn for it.
func printedPages(slides []pptx.Slide) map[int]int {
	pages := map[int]int{}
	page := 0
	for index := len(slides) - 1; index >= 0; index-- {
		if !slides[index].Skipped {
			page = countPrintedTo(slides, index)
		}
		pages[index+1] = page
	}
	return pages
}

// countPrintedTo is the page number a slide is printed on, counting the slides
// before it that are printed at all.
func countPrintedTo(slides []pptx.Slide, index int) int {
	page := 0
	for at := 0; at <= index; at++ {
		if !slides[at].Skipped {
			page++
		}
	}
	return page
}

// onPrintedPages rewrites a drawing's jumps from the deck's numbering into the
// paper's. A jump with no page left is written as #slide-0, which names no page
// and draws no link.
func onPrintedPages(drawing string, printed map[int]int) string {
	if len(printed) == 0 || !strings.Contains(drawing, "#slide-") {
		return drawing
	}
	return jumpHref.ReplaceAllStringFunc(drawing, func(match string) string {
		number, err := strconv.Atoi(strings.TrimPrefix(match, "#slide-"))
		if err != nil {
			return match
		}
		return fmt.Sprintf("#slide-%d", printed[number])
	})
}

var jumpHref = regexp.MustCompile(`#slide-\d+`)

// printWithNotes is the handout: the slide at the top of the page and what the
// presenter meant to say underneath it.
//
// The slide keeps its shape — a deck is 16:9 and cropping it to fit more words
// would be printing a different deck — so the room for notes is what is left of
// the page under it.
// missingRunes is what the face could not draw, in a stable order so the same
// deck reports the same list twice.
func missingRunes(document *pdf.Document) []rune {
	undrawn := document.Undrawn()
	if len(undrawn) == 0 {
		return nil
	}
	out := make([]rune, 0, len(undrawn))
	for character := range undrawn {
		out = append(out, character)
	}
	sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
	return out
}

func printWithNotes(document *pdf.Document, page *pdf.Page, drawing string, slide pptx.Slide,
	language string, width, height float64, printed map[int]int) error {
	const margin = 36
	const gap = 22
	scale := 0.62
	drawnWidth := width * scale
	drawnHeight := height * scale
	left := (width - drawnWidth) / 2
	page.Rect(0, 0, width, height, "FFFFFF")
	if err := pdf.DrawSVGAt(page, drawing, left, margin, scale); err != nil {
		return err
	}
	// A hairline around the slide, so the page reads as "the slide, and then
	// the notes" rather than as one drawing that stops.
	page.Rect(left, margin, drawnWidth, 0.75, "D6D8DE")
	page.Rect(left, margin+drawnHeight, drawnWidth, 0.75, "D6D8DE")
	page.Rect(left, margin, 0.75, drawnHeight, "D6D8DE")
	page.Rect(left+drawnWidth, margin, 0.75, drawnHeight, "D6D8DE")

	top := margin + drawnHeight + gap
	page.Text(left, top, 9, "8A8D96", noteHeading(slide), false, false)
	top += 16
	// The same thing the exported file's notes page carries: what to say, and
	// where the figures came from.
	notes := strings.TrimSpace(pptx.NotesWithSources(slide, language))
	if notes == "" {
		page.Text(left, top, 10.5, "AFB2BA", "적어 둔 말이 없습니다", false, true)
		return nil
	}
	// The notes are written the way every other line of the deck is — a link, a
	// marked word — so they are drawn the same way. Printing [보기](https://…)
	// on a handout is the markup reaching a reader, which is the one thing the
	// drawing of a slide never does.
	runs := pptx.SplitRuns(notes)
	plain := pptx.PlainText(notes)
	cursor := 0
	for _, line := range document.WrapText(plain, 10.5, drawnWidth) {
		// The last line has to sit above the margin, not on it: a note that runs
		// into the edge of the paper reads as a printing fault.
		if top > height-margin-15 {
			// The rest of a very long note is on the slide's own notes page in
			// the pptx; a handout that runs off the paper is not a handout.
			page.Text(left, top, 10.5, "AFB2BA", "…", false, false)
			break
		}
		at := strings.Index(plain[cursor:], line)
		if at < 0 {
			page.Text(left, top, 10.5, "3C4250", line, false, false)
			top += 15
			continue
		}
		start := cursor + at
		cursor = start + len(line)
		drawNoteLine(document, page, runs, plain, start, cursor, left, top, printed)
		top += 15
	}
	return nil
}

// drawNoteLine draws one wrapped line of the notes as the runs it is made of.
//
// The line is a stretch of the plain text, and every run knows where it sits in
// that same text, so what belongs to this line is an overlap rather than a
// search for the words again.
func drawNoteLine(document *pdf.Document, page *pdf.Page, runs []pptx.TextRun, plain string,
	from, to int, x, y float64, printed map[int]int) {
	at := 0
	for _, run := range runs {
		start, end := at, at+len(run.Text)
		at = end
		if end <= from || start >= to {
			continue
		}
		piece := run.Text[max(from-start, 0):min(to-start, len(run.Text))]
		if piece == "" {
			continue
		}
		colour := "3C4250"
		if run.Href != "" {
			colour = "1155CC"
		}
		width := page.Text(x, y, 10.5, colour, piece, run.Bold, run.Italic)
		if run.Href != "" {
			page.Underline(x, y, width, colour, 10.5)
			target, jump := run.Href, 0
			if number, ok := pptx.SlideJump(run.Href); ok {
				// The note is on the paper too, and the paper's pages are not
				// the deck's slides when a slide is being skipped.
				target, jump = "", printed[number]
			}
			page.Link(x, y-8.4, width, 11, target, jump)
		}
		x += width
	}
}

func noteHeading(slide pptx.Slide) string {
	title := ""
	for _, paragraph := range slide.Fields[pptx.SlotTitle] {
		title = strings.TrimSpace(pptx.PlainText(paragraph.Text))
		break
	}
	if title == "" {
		return fmt.Sprintf("%d — 발표자 노트", slide.Number)
	}
	return fmt.Sprintf("%d · %s — 발표자 노트", slide.Number, title)
}
