package deck

import (
	"fmt"
	"strconv"
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
	// ResolveImage turns the name an author wrote into a stored image. Without it
	// an ::image directive is reported rather than silently dropped, because a
	// missing picture is something the author has to know about.
	ResolveImage func(reference string) (ContentImage, bool)
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
		// Warnings name the line the slide starts on as well as its position: an
		// author fixing something needs to find it in the text.
		where := fmt.Sprintf("slide %d", index+1)
		if slide.Line > 0 {
			where = fmt.Sprintf("line %d (slide %d)", slide.Line, index+1)
		}
		layout, note := resolveSourceLayout(manifest, slide, index, len(source.Slides))
		if note != "" {
			result.Warnings = append(result.Warnings, where+": "+note)
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
					fmt.Sprintf("%s: %s has no free region in layout %q and was written as text",
						blockWhere(where, block), block.Kind, layout.Name))
				slide.Bullets = append(slide.Bullets, blockAsBullets(block)...)
				continue
			}
			assembled := pptx.Block{Kind: block.Kind, Caption: block.Caption, Items: block.Items}
			if block.Kind == pptx.BlockTable && len(block.Rows) > 1 {
				// The first row is the header; the rest are the body.
				assembled.Columns = block.Rows[0]
				assembled.Rows = block.Rows[1:]
				assembled.Items = nil
			}
			if block.Kind == pptx.BlockLine {
				assembled.Series, assembled.Labels = seriesFromRows(block.Rows)
			}
			sanitized, usable := pptx.SanitizeBlock(assembled, placeholder)
			if !usable {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("%s: %s did not have enough room and was written as text",
						blockWhere(where, block), block.Kind))
				slide.Bullets = append(slide.Bullets, blockAsBullets(block)...)
				continue
			}
			content.SetBlock(placeholder.Slot, sanitized)
			claimed[placeholder.Slot] = true
		}

		// An image takes the layout's picture region when it has one, and otherwise
		// the largest free body region — the same rule a component follows.
		for _, picture := range slide.Images {
			at := where
			if picture.Line > 0 {
				at = fmt.Sprintf("line %d", picture.Line)
			}
			if options.ResolveImage == nil {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("%s: images cannot be resolved here, so %q was skipped", at, picture.Reference))
				continue
			}
			resolved, ok := options.ResolveImage(picture.Reference)
			if !ok {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("%s: no uploaded image is named %q", at, picture.Reference))
				continue
			}
			placeholder, found := imageSlot(layout, claimed)
			if !found {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("%s: layout %q has no region for an image", at, layout.Name))
				continue
			}
			resolved.Name = picture.Reference
			resolved.Caption = picture.Caption
			content.SetImage(placeholder.Slot, resolved)
			claimed[placeholder.Slot] = true
		}

		lead := strings.TrimSpace(slide.Lead)
		// subtitle is only set when the lead really went into the layout's subtitle
		// region. Recording it otherwise makes the renderer write the same sentence
		// a second time, into whichever subtitle slot the layout happens to have.
		subtitle := ""
		if lead != "" && lead != title {
			switch placeholder, ok := leadSlot(layout, claimed); {
			case ok:
				content.SetField(placeholder.Slot, fit(placeholder, []pptx.Paragraph{{Text: lead}}, options.Language))
				claimed[placeholder.Slot] = true
				subtitle = lead
			case len(content.Blocks) > 0:
				// The component took the only body region. A lead line belongs above
				// it as its heading rather than being dropped or held aside as text.
				for slot, block := range content.Blocks {
					if strings.TrimSpace(block.Heading) == "" {
						block.Heading = lead
						content.SetBlock(slot, block)
					}
					break
				}
			default:
				// No subtitle region and no component: the lead leads the points,
				// which is where it belongs and stops it being dropped.
				slide.Bullets = append([]pptx.Paragraph{{Text: lead}}, slide.Bullets...)
			}
		}

		if len(slide.Bullets) > 0 {
			distributeBullets(&content, layout, claimed, slide.Bullets, options.Language, &result.Warnings, where)
		}

		notes := strings.TrimSpace(slide.Notes)
		result.Slides = append(result.Slides, model.Slide{
			Position:     index + 1,
			Title:        truncate(title, 200),
			Subtitle:     truncate(subtitle, 300),
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
		// Position only implies a role. A first slide that carries a component or a
		// list of points is not a cover, whatever its position says, and putting it
		// on a title layout would throw the content away.
		if role == pptx.RoleTitle && (len(slide.Blocks) > 0 || len(slide.Bullets) > 1) {
			role = pptx.RoleContent
		}
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

// imageSlot is where a picture goes: the layout's own picture region first,
// since that is where its designer meant one to be.
func imageSlot(layout pptx.Layout, claimed map[string]bool) (pptx.Placeholder, bool) {
	for _, placeholder := range layout.Placeholders {
		if placeholder.Kind == "picture" && !claimed[placeholder.Slot] {
			return placeholder, true
		}
	}
	return blockSlot(layout, claimed)
}

// leadSlot is where a lead line goes: the layout's subtitle region, when it has
// one that is still free.
func leadSlot(layout pptx.Layout, claimed map[string]bool) (pptx.Placeholder, bool) {
	if placeholder, ok := layout.Slot(pptx.SlotSubtitle); ok && !claimed[placeholder.Slot] {
		return placeholder, true
	}
	return pptx.Placeholder{}, false
}

// distributeBullets fills the layout's body slots, splitting the list across
// columns when the layout has more than one.
// blockWhere names a component's own line when it has one.
func blockWhere(slideWhere string, block SourceBlock) string {
	if block.Line > 0 {
		return fmt.Sprintf("line %d", block.Line)
	}
	return slideWhere
}

func distributeBullets(content *Content, layout pptx.Layout, claimed map[string]bool,
	bullets []pptx.Paragraph, language string, warnings *[]string, where string) {
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
			fmt.Sprintf("%s: layout %q has no free body region, so its points were kept as plain text",
				where, layout.Name))
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

// seriesFromRows reads "name | v1, v2, v3" rows into chart series. A row of bare
// labels with no numbers is the time axis.
func seriesFromRows(rows [][]string) ([]pptx.Series, []string) {
	var series []pptx.Series
	var labels []string
	for _, row := range rows {
		if len(row) < 2 {
			continue
		}
		name := row[0]
		fields := strings.Split(strings.Join(row[1:], ","), ",")
		points := make([]float64, 0, 8)
		labelled := 0
		for _, field := range fields {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			// A plotted value is a bare number. "1월" carries a digit but is a month,
			// and reading it as the number one would draw the axis as data.
			if value, ok := parseBareNumber(field); ok {
				points = append(points, value)
				continue
			}
			labelled++
		}
		// A row whose values are labels is the time axis, not a series.
		if labelled > 0 && len(points) == 0 {
			labels = labels[:0]
			for _, field := range fields {
				if trimmed := strings.TrimSpace(field); trimmed != "" {
					labels = append(labels, trimmed)
				}
			}
			continue
		}
		if len(points) >= 2 {
			series = append(series, pptx.Series{Name: name, Points: points})
		}
	}
	return series, labels
}

// parseBareNumber accepts only a number, optionally signed, with thousands
// separators and an optional percent sign — the forms a chart value takes.
func parseBareNumber(value string) (float64, bool) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "%"))
	if trimmed == "" {
		return 0, false
	}
	cleaned := strings.ReplaceAll(trimmed, ",", "")
	for index, character := range cleaned {
		switch {
		case character >= '0' && character <= '9', character == '.':
		case (character == '-' || character == '+') && index == 0:
		default:
			return 0, false
		}
	}
	number, err := strconv.ParseFloat(cleaned, 64)
	return number, err == nil
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
