package deck

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
	"unicode/utf8"
)

// CompileOptions controls how source is bound to a template.
type CompileOptions struct {
	// Accent colours the components. An empty value uses the template's own.
	Accent func(position int) string
	// Language is used for text measurement, since a Korean line holds far fewer
	// characters than an English one of the same width.
	Language string
	// ResolveGrid finds the grid definition a ::grid component names. Without it
	// only the shipped definitions are available.
	ResolveGrid func(name string) (pptx.GridSpec, bool)
	// ResolveImage turns the name an author wrote into a stored image. Without it
	// an ::image directive is reported rather than silently dropped, because a
	// missing picture is something the author has to know about.
	ResolveImage func(reference string) (ContentImage, bool)
	// LayoutsAreSuggestions says the @layout lines were written by the model
	// rather than by a person.
	//
	// A person naming a layout has decided; the compiler obeys. A plan naming one
	// chose it before knowing what the slide would end up carrying, so a component
	// and four points can land on a layout with one region between them. When the
	// names are the model's, a layout that would lose content is replaced by one
	// that holds it, and the substitution is reported.
	LayoutsAreSuggestions bool
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
	previousLayout := ""
	for index, slide := range source.Slides {
		// Warnings name the line the slide starts on as well as its position: an
		// author fixing something needs to find it in the text.
		where := fmt.Sprintf("slide %d", index+1)
		if slide.Line > 0 {
			where = fmt.Sprintf("line %d (slide %d)", slide.Line, index+1)
		}
		layout, note := resolveSourceLayout(manifest, slide, index, len(source.Slides), previousLayout, options)
		if note != "" {
			result.Warnings = append(result.Warnings, where+": "+note)
		}
		previousLayout = layout.ID
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
			block = withHeaderRow(block)
			assembled := pptx.Block{Kind: block.Kind, Caption: block.Caption, Items: block.Items}
			if block.Kind == pptx.BlockGrid {
				spec, ok := resolveGrid(options, block.Definition)
				if !ok {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("%s: no grid is defined as %q", blockWhere(where, block), block.Definition))
					slide.Bullets = append(slide.Bullets, blockAsBullets(block)...)
					continue
				}
				assembled.Grid = &spec
				if len(block.Rows) > 1 {
					assembled.Columns, assembled.Rows = block.Rows[0], block.Rows[1:]
				} else {
					assembled.Rows = block.Rows
				}
				assembled.Items = nil
				if strings.TrimSpace(assembled.Caption) == "" {
					assembled.Caption = spec.Title
				}
			}
			if block.Kind == pptx.BlockComparison {
				// A comparison's rows are kept verbatim as well as parsed into items:
				// "항목 | 기존 | 신규" is a matrix of as many columns as it was written
				// with, which label/value/detail cannot hold.
				assembled.Rows = block.Rows
			}
			if block.Kind == pptx.BlockTable && len(block.Rows) > 1 {
				// The first row is the header; the rest are the body.
				assembled.Columns = block.Rows[0]
				assembled.Rows = block.Rows[1:]
				assembled.Items = nil
			}
			if block.Kind == pptx.BlockLine {
				assembled.Series, assembled.Labels = seriesFromRows(block.Rows)
			}
			// A hero is one number, and the model is told so. When it writes two,
			// drawing the first and dropping the rest loses a figure the author
			// asked for without saying so; a row of indicators keeps them all and
			// is what two labelled figures are.
			if assembled.Kind == pptx.BlockHero && len(assembled.Items) > 1 {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("%s: a hero draws one figure and %d were written, so they were drawn as %s",
						blockWhere(where, block), len(assembled.Items), pptx.BlockKPI))
				assembled.Kind = pptx.BlockKPI
			}
			sanitized, usable := pptx.SanitizeBlock(assembled, placeholder)
			if !usable {
				// A chart whose values are not numbers — "Q3 | 1시간" — cannot be
				// plotted, but the rows are still a set of labelled figures. Drawing
				// them as indicators keeps the author's intent; prose throws it away.
				if fallback, kind := chartFallback(assembled); kind != "" {
					if retried, ok := pptx.SanitizeBlock(fallback, placeholder); ok {
						result.Warnings = append(result.Warnings,
							fmt.Sprintf("%s: %s had no numeric values and was drawn as %s",
								blockWhere(where, block), block.Kind, kind))
						content.SetBlock(placeholder.Slot, retried)
						claimed[placeholder.Slot] = true
						continue
					}
				}
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("%s: %s did not have enough room and was written as text",
						blockWhere(where, block), block.Kind))
				slide.Bullets = append(slide.Bullets, blockAsBullets(block)...)
				continue
			}
			// A component that reads across the page should have the page. One
			// comparison matrix or chart squeezed into one column of a two-column
			// layout leaves the other column empty, which looks like a mistake.
			if len(slide.Blocks) == 1 && len(slide.Images) == 0 && len(slide.Bullets) == 0 {
				if sibling, ok := twinSlot(layout, placeholder, claimed); ok && spansWell(sanitized) {
					sanitized.Span = []string{placeholder.Slot, sibling.Slot}
					claimed[sibling.Slot] = true
				}
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

		// A slide written as two columns heads each one itself, so its first lead
		// is that column's heading rather than the slide's subtitle.
		// A slide whose whole body is one slash-separated line is a list written
		// on one line, and is drawn as the list it is.
		slide = withLeadAsPoints(slide, layout)
		// A comparison whose two sides were written as an ordinary first line —
		// "현재 | 자동화" above the points, with no "> " on it — names its columns
		// the same way a lead does, so it is read the same way.
		slide = withPairedFirstPoint(slide, layout, claimed)
		_, twoColumns := columnGroups(slide, layout, claimed)
		// An agenda whose first item was written on the lead line is not a slide
		// with a lead: it is a list missing its first item. Drawn as written, "1."
		// sits above the list without a bullet and "2." to "5." look like the
		// whole list, which is the one thing a contents slide must not look like.
		slide = withNumberedLeadAsPoint(slide)
		lead := strings.TrimSpace(slide.Lead)
		// A slide that says its own lead again as a bullet says it twice on the
		// page. The model does this often enough to be worth removing here: the
		// same sentence in two places is never what anyone meant.
		slide.Bullets = dropEcho(slide.Bullets, lead, title)
		// subtitle is only set when the lead really went into the layout's subtitle
		// region. Recording it otherwise makes the renderer write the same sentence
		// a second time, into whichever subtitle slot the layout happens to have.
		subtitle := ""
		if lead != "" && lead != title && !twoColumns {
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
				// which is where it belongs and stops it being dropped. It is marked
				// as the lead so it is drawn as one — without a bullet — and so a
				// round trip writes it back out as the lead rather than demoting it
				// to a point.
				// The slide keeps no subtitle of its own here: a renderer that found
				// one would write the line twice, once in each place.
				slide.Bullets = append([]pptx.Paragraph{{Text: lead, Lead: true}}, slide.Bullets...)
			}
		}

		if len(slide.Bullets) > 0 {
			if columns, ok := columnGroups(slide, layout, claimed); ok {
				placeColumns(&content, layout, claimed, columns, options.Language)
			} else {
				distributeBullets(&content, layout, claimed, withGroupHeadings(slide),
					options.Language, &result.Warnings, where)
			}
		}
		// A caption an author wrote for a photograph is theirs, and it was only
		// being carried as alternative text. When the layout still has a text
		// region free — which is exactly what a picture layout keeps beside the
		// picture — the caption is written there, where a caption belongs.
		placeImageCaptions(&content, layout, claimed, options.Language)

		for _, citation := range slide.Sources {
			content.Sources = append(content.Sources, pptx.Citation{
				Marker: citation.Marker, Title: citation.Title, Locator: citation.Locator})
		}
		if len(content.Sources) > MaxSlideSources {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: a slide may cite at most %d sources; the rest were dropped", where, MaxSlideSources))
			content.Sources = content.Sources[:MaxSlideSources]
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
// request, then the layout that can actually hold what the slide carries.
//
// The role a slide declares says what it is for; it does not say whether the
// chosen layout has room for a chart and four points at once. Choosing on role
// alone is how a component ends up in a caption strip and three of the author's
// points get dropped, so the role is a strong preference and the fit decides.
func resolveSourceLayout(manifest pptx.Manifest, slide SourceSlide, index, total int, previous string,
	options CompileOptions) (pptx.Layout, string) {
	if id := strings.TrimSpace(slide.LayoutID); id != "" {
		if layout, ok := manifest.LayoutByReference(id); ok {
			// An author who names a layout gets it, fit or no fit: it is their deck.
			if !options.LayoutsAreSuggestions {
				return layout, ""
			}
			// A model's choice is a suggestion. It stands unless it would cost the
			// slide something a different layout would keep.
			named := layoutFitScore(layout, slide, slideRole(slide, index, total))
			if better, ok := bestFittingLayout(manifest, slide, slideRole(slide, index, total), previous); ok &&
				better.ID != layout.ID && layoutFitScore(better, slide, slideRole(slide, index, total))-named >= 25 {
				return better, fmt.Sprintf("layout %q cannot hold this slide; used %q instead", layout.Name, better.Name)
			}
			return layout, ""
		}
		// The named layout is not in this template. Rather than dropping to the
		// role's default — which is how a component and its prose end up with one
		// region between them — choose by what the slide has to carry.
		if layout, ok := bestFittingLayout(manifest, slide, slideRole(slide, index, total), ""); ok {
			return layout, fmt.Sprintf("layout %q does not exist in this template; used %q instead", id, layout.Name)
		}
		if layout, ok := manifest.LayoutForRole(sourcePositionRole(index, total)); ok {
			return layout, fmt.Sprintf("layout %q does not exist in this template; used %q instead", id, layout.Name)
		}
	}
	role := slideRole(slide, index, total)
	// A cover, a divider and a closing slide are about the deck's arc, not about
	// how much text they hold: those roles keep their own layout. Everything in
	// between is chosen by what it has to carry.
	if !structuralRole(role) {
		if layout, ok := bestFittingLayout(manifest, slide, role, previous); ok {
			return layout, ""
		}
	}
	if layout, ok := manifest.LayoutForRole(role); ok {
		// A structural layout keeps its slide even when the fit is loose — that is
		// the deck's shape. But a closing page that cannot hold the ask is the one
		// slide nobody can afford to lose: "다음 단계" with three requests set as a
		// stray line under the title is the most important slide of the deck being
		// dropped on the floor.
		// A template with no layout for this role falls back to a neighbouring one,
		// and a neighbour designed for something else can read badly: an Office
		// section header puts its title below its body, which turns a closing slide
		// with three requests upside down. When the role is missing entirely, a
		// slide that carries points is better off on a content layout.
		if structuralRole(role) && layout.Role != role &&
			(len(slide.Bullets) > 0 || len(slide.Blocks) > 0) {
			if content, ok := manifest.LayoutForRole(pptx.RoleContent); ok && content.Role == pptx.RoleContent {
				return content, fmt.Sprintf("this template has no %s layout; used %q, which has room for the points", role, content.Name)
			}
		}
		if structuralRole(role) && !layoutHoldsBody(layout, slide) {
			// Another layout of the same kind, if the template has one. The deck's
			// shape is not negotiable — a closing slide stays a closing slide — but
			// a template with two closing pages should be given the one that can
			// hold the ask.
			if better, ok := roleLayoutThatHolds(manifest, slide, role, layout.ID); ok {
				return better, fmt.Sprintf("the %q layout has no room for this slide's points; used %q instead",
					layout.Name, better.Name)
			}
		}
		return layout, ""
	}
	if layout, ok := manifest.LayoutForRole(pptx.RoleContent); ok {
		return layout, ""
	}
	return manifest.Layouts[0], ""
}

// withGroupHeadings puts a column's heading back among the points, for a layout
// that has nowhere to put the columns. The heading is the author's sentence: it
// leads the points it was written for rather than being dropped.
func withGroupHeadings(slide SourceSlide) []pptx.Paragraph {
	if len(slide.Groups) == 0 {
		return slide.Bullets
	}
	folded := make([]pptx.Paragraph, 0, len(slide.Bullets)+len(slide.Groups))
	previous := 0
	for _, group := range slide.Groups {
		at := min(max(group.From, 0), len(slide.Bullets))
		folded = append(folded, slide.Bullets[previous:at]...)
		if heading := strings.TrimSpace(group.Heading); heading != "" {
			folded = append(folded, pptx.Paragraph{Text: heading, Lead: true})
		}
		previous = at
	}
	return append(folded, slide.Bullets[previous:]...)
}

// withHeaderRow moves a caption written inside a table's fence out of its rows.
//
// A model wrote its grid like this:
//
//	::grid checklist
//	준비 상태
//	항목 | 상태
//	핵심 인력 교육 | 완료
//
// "준비 상태" is what the table is called, but the first row of a table is its
// column headings, so the table came out with one column headed "준비 상태",
// "항목 | 상태" as its first row of data, and the name printed twice — once as
// the caption the grid definition supplies and once as that lone column.
//
// One cell where the rows below have more is not a header. It is the caption,
// and it is used as one when the fence did not already carry it.
func withHeaderRow(block SourceBlock) SourceBlock {
	switch block.Kind {
	case pptx.BlockTable, pptx.BlockGrid, pptx.BlockComparison:
	default:
		return block
	}
	if len(block.Rows) < 3 || len(block.Rows[0]) != 1 {
		return block
	}
	for _, row := range block.Rows[1:] {
		if len(row) < 2 {
			return block
		}
	}
	if strings.TrimSpace(block.Caption) == "" {
		block.Caption = strings.TrimSpace(block.Rows[0][0])
	}
	block.Rows = block.Rows[1:]
	return block
}

// withLeadAsPoints splits a lead that is the slide's whole body.
//
// A model wrote this and nothing else on the slide:
//
//	> 3개 창고, 일 12,000건 처리 중 / 오배송률 0.8% 유지 / 인력 의존도 높음
//
// Three points, written on one line, drawn as one run-on sentence across the
// top of an otherwise empty slide. The separator is unambiguous — a slash with
// a space each side, which nothing writes inside a phrase — and the slide has
// nothing else in it, so there is nothing for the split to compete with.
//
// A cover or a divider keeps its line: its subtitle is a single statement about
// the deck, not a list of points.
func withLeadAsPoints(slide SourceSlide, layout pptx.Layout) SourceSlide {
	lead := strings.TrimSpace(slide.Lead)
	if lead == "" || len(slide.Bullets) > 0 || len(slide.Blocks) > 0 || len(slide.Groups) > 0 {
		return slide
	}
	switch layout.Role {
	case pptx.RoleTitle, pptx.RoleSection, pptx.RoleBlank:
		return slide
	}
	if len(freeBodySlots(layout, map[string]bool{})) == 0 {
		return slide
	}
	parts := strings.Split(lead, " / ")
	if len(parts) < 2 {
		return slide
	}
	points := make([]pptx.Paragraph, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if utf8.RuneCountInString(part) < 4 {
			return slide
		}
		points = append(points, pptx.Paragraph{Text: part})
	}
	slide.Lead = ""
	slide.Bullets = points
	return slide
}

// withPairedFirstPoint reads a first point that names both columns as the lead
// that names both columns.
//
// A model wrote a comparison slide as "현재 | 자동화" on its own line, then the
// points. Written with "> " in front it is a lead, and the compiler has split
// it into two column headings since v0.60. Written without, it stayed a point:
// the slide's first bullet read "현재 | 자동화" and neither column was named.
// The two are the same slide, so they are read the same way.
func withPairedFirstPoint(slide SourceSlide, layout pptx.Layout, claimed map[string]bool) SourceSlide {
	if strings.TrimSpace(slide.Lead) != "" || len(slide.Groups) > 0 || len(slide.Bullets) < 3 {
		return slide
	}
	first := slide.Bullets[0]
	if first.Level > 0 {
		return slide
	}
	sides := strings.Split(first.Text, "|")
	if len(sides) != 2 || !columnName(sides[0]) || !columnName(sides[1]) {
		return slide
	}
	if len(freeBodySlots(layout, claimed)) < 2 {
		return slide
	}
	slide.Lead = strings.TrimSpace(first.Text)
	slide.Bullets = slide.Bullets[1:]
	return slide
}

// withNumberedLeadAsPoint moves a lead that is the first item of a numbered
// list back into the list.
//
// A model writing a contents slide put "1. 시장 성장과 시스템 리스크" on the lead
// line and "2." to "5." below it as points. Every item is the same kind of
// thing, so drawing one of them differently — no bullet, in the lead's own
// size, or worse, alone in the layout's subtitle region — breaks the numbering
// on the one slide whose whole job is the numbering.
func withNumberedLeadAsPoint(slide SourceSlide) SourceSlide {
	lead := strings.TrimSpace(slide.Lead)
	if lead == "" || len(slide.Bullets) == 0 || len(slide.Groups) > 0 {
		return slide
	}
	if itemNumber(lead) != 1 || itemNumber(slide.Bullets[0].Text) != 2 {
		return slide
	}
	slide.Lead = ""
	slide.Bullets = append([]pptx.Paragraph{{Text: lead}}, slide.Bullets...)
	return slide
}

// itemNumber reads the number a line begins with — "3.", "3)", "③" — and
// reports 0 when it begins with none. Only a small number counts: "2026년" is a
// year and "12억" is money.
func itemNumber(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	first := []rune(trimmed)[0]
	for index, circled := range []rune("①②③④⑤⑥⑦⑧⑨") {
		if circled == first {
			return index + 1
		}
	}
	digits := 0
	for digits < len(trimmed) && trimmed[digits] >= '0' && trimmed[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits > 2 || digits >= len(trimmed) {
		return 0
	}
	switch trimmed[digits] {
	case '.', ')', ':':
	default:
		return 0
	}
	// "1.5배 늘었다" is a figure, not an item.
	if rest := strings.TrimSpace(trimmed[digits+1:]); rest == "" ||
		(rest[0] >= '0' && rest[0] <= '9') {
		return 0
	}
	number := 0
	for _, symbol := range trimmed[:digits] {
		number = number*10 + int(symbol-'0')
	}
	return number
}

// column is one side of a two-column slide: its heading and its points.
type column struct {
	Heading string
	Bullets []pptx.Paragraph
}

// columnGroups reads a slide written as columns — a heading, its points, another
// heading, its points — and reports whether this layout can hold it that way.
//
// It can when the layout has a body region for each column. One region short and
// the slide is better off as one list: two headings crammed into one column read
// as a sentence that lost its verb.
func columnGroups(slide SourceSlide, layout pptx.Layout, claimed map[string]bool) ([]column, bool) {
	if len(slide.Groups) == 0 {
		return pairedLead(slide, layout, claimed)
	}
	columns := []column{{Heading: strings.TrimSpace(slide.Lead)}}
	starts := append([]SourceGroup{}, slide.Groups...)
	previous := 0
	for _, group := range starts {
		at := min(max(group.From, 0), len(slide.Bullets))
		columns[len(columns)-1].Bullets = slide.Bullets[previous:at]
		columns = append(columns, column{Heading: strings.TrimSpace(group.Heading)})
		previous = at
	}
	columns[len(columns)-1].Bullets = slide.Bullets[previous:]
	for _, entry := range columns {
		if len(entry.Bullets) == 0 {
			return nil, false
		}
	}
	if len(freeBodySlots(layout, claimed)) < len(columns) {
		return nil, false
	}
	return columns, true
}

// pairedLead reads a lead written as two headings — "성장 채널 | 위축 채널" — which
// is how a comparison slide names its sides when the points below are one list.
//
// The bar is the same separator the components use for a row, and on a slide
// with two regions it means the same thing: this is the left one and that is
// the right one. Printed whole it lands as a stray line above the left column
// and the right column is left unnamed, which is what a model's comparison
// slide looked like.
func pairedLead(slide SourceSlide, layout pptx.Layout, claimed map[string]bool) ([]column, bool) {
	sides := strings.Split(slide.Lead, "|")
	if len(sides) != 2 {
		return nil, false
	}
	left, right := strings.TrimSpace(sides[0]), strings.TrimSpace(sides[1])
	if !columnName(left) || !columnName(right) {
		return nil, false
	}
	slots := freeBodySlots(layout, claimed)
	if len(slots) < 2 {
		return nil, false
	}
	// The points often say which side they are on — "현재: 0.8% 오배송" under a
	// slide headed "현재 | 자동화". Splitting those down the middle put a point
	// about one side under the other's heading, which is worse than not naming
	// the sides at all: the slide reads as an argument for the opposite thing.
	if sided, ok := bulletsBySide(slide.Bullets, left, right); ok {
		return sided, true
	}
	halves := splitEvenly(slide.Bullets, 2)
	if len(halves) != 2 || len(halves[0]) == 0 || len(halves[1]) == 0 {
		return nil, false
	}
	return []column{{Heading: left, Bullets: halves[0]}, {Heading: right, Bullets: halves[1]}}, true
}

// bulletsBySide sorts points that name their own side into that side.
//
// A point with no prefix follows the one before it, which is how a list like
// "자동화: 0.1% 목표" then "처리 속도 2배 향상 기대" is read by anyone: the second
// line is still about automation. It reports false unless both sides claim a
// point, since one side's name on every line says nothing about the other.
func bulletsBySide(bullets []pptx.Paragraph, left, right string) ([]column, bool) {
	columns := []column{{Heading: left}, {Heading: right}}
	claimed := [2]bool{}
	side := -1
	for _, bullet := range bullets {
		if text, ok := withoutSidePrefix(bullet.Text, left); ok {
			side, bullet.Text = 0, text
			claimed[0] = true
		} else if text, ok := withoutSidePrefix(bullet.Text, right); ok {
			side, bullet.Text = 1, text
			claimed[1] = true
		} else if side < 0 {
			return nil, false
		}
		columns[side].Bullets = append(columns[side].Bullets, bullet)
	}
	if !claimed[0] || !claimed[1] {
		return nil, false
	}
	return columns, true
}

// withoutSidePrefix removes "현재:" or "현재 -" from the front of a point.
func withoutSidePrefix(text, side string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if side == "" || !strings.HasPrefix(trimmed, side) {
		return text, false
	}
	rest := strings.TrimSpace(trimmed[len(side):])
	if rest == "" {
		return text, false
	}
	switch rest[0] {
	case ':', '-', 0xEF:
		// ':' and '-' in either width; 0xEF starts the full-width colon "：".
	default:
		return text, false
	}
	rest = strings.TrimSpace(strings.TrimLeft(rest, ":-： "))
	if rest == "" {
		return text, false
	}
	return rest, true
}

// columnName reports whether a phrase is the kind of thing that heads a column.
//
// A heading is a label — "성장 채널", "Growth channels" — not a clause. Three
// words is the line: past that the bar is punctuation inside a sentence
// somebody wrote, and splitting it would cut their sentence in half.
func columnName(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || utf8.RuneCountInString(trimmed) > 24 {
		return false
	}
	return len(strings.Fields(trimmed)) <= 3
}

// placeColumns writes each column into its own region, its heading above its
// own points rather than above the slide.
func placeColumns(content *Content, layout pptx.Layout, claimed map[string]bool,
	columns []column, language string) {
	slots := freeBodySlots(layout, claimed)
	for index, entry := range columns {
		if index >= len(slots) {
			return
		}
		paragraphs := make([]pptx.Paragraph, 0, len(entry.Bullets)+1)
		if entry.Heading != "" {
			// A comparison layout offers a one-line region above each column for
			// exactly this — "왼쪽 항목", "오른쪽 항목" — styled by the template as a
			// heading. Writing the heading into the points instead left those
			// regions empty and drew the heading in body text, which is the
			// template's own design going unused on the slide that most needs it.
			placed := false
			if slot, ok := headingSlot(layout, claimed, slots[index]); ok {
				// One line only. A heading too long for it would be cut mid-sentence
				// and nobody told, so a long one stays where it has room to wrap.
				heading := []pptx.Paragraph{{Text: entry.Heading, Lead: true}}
				if fitted, report := pptx.FitParagraphsReport(heading, slot, language); !report.Lost() {
					content.SetField(slot.Slot, fitted)
					claimed[slot.Slot] = true
					placed = true
				}
			}
			if !placed {
				paragraphs = append(paragraphs, pptx.Paragraph{Text: entry.Heading, Lead: true})
			}
		}
		paragraphs = append(paragraphs, entry.Bullets...)
		fitted, _ := pptx.FitParagraphsReport(paragraphs, slots[index], language)
		content.SetField(slots[index].Slot, fitted)
		claimed[slots[index].Slot] = true
	}
}

// freeBodySlots is every body region the slide has not already given away.
// headingSlot is the one-line region a template puts above a column, when it
// has one free and unclaimed.
//
// It is found by where it is rather than by what it is called: a body region of
// a single line, above the column's own region and over the same part of the
// page. Templates name these differently and Ptium binds any of them.
func headingSlot(layout pptx.Layout, claimed map[string]bool, body pptx.Placeholder) (pptx.Placeholder, bool) {
	var best pptx.Placeholder
	found := false
	for _, candidate := range layout.BodySlots() {
		if claimed[candidate.Slot] || candidate.Slot == body.Slot || candidate.MaxLines > 1 {
			continue
		}
		if candidate.Y >= body.Y || candidate.Width <= 0 || body.Width <= 0 {
			continue
		}
		overlap := min(candidate.X+candidate.Width, body.X+body.Width) - max(candidate.X, body.X)
		if overlap*2 < min(candidate.Width, body.Width) {
			continue
		}
		if !found || candidate.Y > best.Y {
			best, found = candidate, true
		}
	}
	return best, found
}

func freeBodySlots(layout pptx.Layout, claimed map[string]bool) []pptx.Placeholder {
	var slots []pptx.Placeholder
	for _, placeholder := range layout.BodySlots() {
		if !claimed[placeholder.Slot] && placeholder.MaxLines >= 2 {
			slots = append(slots, placeholder)
		}
	}
	return slots
}

// roleLayoutThatHolds finds another layout of the same role with room for the
// slide's points.
func roleLayoutThatHolds(manifest pptx.Manifest, slide SourceSlide, role, avoid string) (pptx.Layout, bool) {
	for _, layout := range manifest.Layouts {
		if layout.Role != role || layout.ID == avoid {
			continue
		}
		if layoutHoldsBody(layout, slide) {
			return layout, true
		}
	}
	return pptx.Layout{}, false
}

// layoutHoldsBody reports whether a layout has somewhere to put what the slide
// carries below its title. A slide with nothing below its title always fits.
func layoutHoldsBody(layout pptx.Layout, slide SourceSlide) bool {
	if len(slide.Bullets) == 0 && len(slide.Blocks) == 0 {
		return true
	}
	for _, placeholder := range layout.Placeholders {
		if placeholder.Slot == pptx.SlotTitle || placeholder.Slot == pptx.SlotSubtitle {
			continue
		}
		if placeholder.AcceptsText() && placeholder.MaxLines > 0 {
			return true
		}
	}
	return false
}

// bestFittingLayout scores every layout against what the slide actually holds.
func bestFittingLayout(manifest pptx.Manifest, slide SourceSlide, role, previous string) (pptx.Layout, bool) {
	best, bestScore, found := pptx.Layout{}, 0.0, false
	for _, layout := range manifest.Layouts {
		// Structural layouts are reserved for structural slides. A content slide
		// that lands on the closing layout breaks the deck's shape, however well
		// its words happen to fit there.
		if structuralRole(layout.Role) || layout.Role == pptx.RoleBlank {
			continue
		}
		score := layoutFitScore(layout, slide, role)
		// A layout nobody would pick on purpose does not win on capacity. A
		// vertical-text layout holds the most lines of any layout in an Office
		// template, and a Korean bullet slide set vertically reads as a fault in
		// the product rather than as a choice.
		score -= float64(layout.PreferenceRank()) * 5
		// Two slides running on the same layout is a rhythm; five is a rut. The
		// nudge is small enough that it never beats a real difference in fit.
		if layout.ID == previous {
			score -= 2
		}
		if !found || score > bestScore {
			best, bestScore, found = layout, score, true
		}
	}
	return best, found
}

// layoutFitScore is how well one layout holds one slide. It counts what would be
// lost, what would be left empty, and whether the slide's role is honoured.
func layoutFitScore(layout pptx.Layout, slide SourceSlide, role string) float64 {
	score := 0.0
	if layout.Role == role {
		score += 14
	}
	if _, ok := layout.Slot(pptx.SlotTitle); ok {
		if strings.TrimSpace(slide.Title) != "" {
			score += 4
		}
	} else if strings.TrimSpace(slide.Title) != "" {
		score -= 22
	}
	if strings.TrimSpace(slide.Lead) != "" {
		if _, ok := layout.Slot(pptx.SlotSubtitle); ok {
			score += 3
		}
	}

	// Regions are handed out the way the compiler hands them out: components
	// take the roomiest first, then images, then the prose fills what is left.
	free := make([]pptx.Placeholder, 0, len(layout.Placeholders))
	for _, placeholder := range layout.BodySlots() {
		free = append(free, placeholder)
	}
	sort.SliceStable(free, func(i, j int) bool {
		return free[i].Width*free[i].Height > free[j].Width*free[j].Height
	})
	take := func(minimumLines int) (pptx.Placeholder, bool) {
		for index, placeholder := range free {
			if placeholder.MaxLines >= minimumLines {
				free = append(free[:index], free[index+1:]...)
				return placeholder, true
			}
		}
		return pptx.Placeholder{}, false
	}

	for _, block := range slide.Blocks {
		if _, ok := take(pptx.BlockMinimumLines(block.Kind)); ok {
			score += 12
			continue
		}
		// No room: the compiler writes the component out as bullets, which is a
		// real loss of the shape the author asked for.
		score -= 34
	}
	for range slide.Images {
		if picture, ok := layout.Slot(pptx.SlotPicture); ok && picture.Kind == "picture" {
			score += 10
			continue
		}
		if _, ok := take(3); ok {
			score += 6
			continue
		}
		score -= 24
	}

	if len(slide.Bullets) > 0 {
		room, needed := 0, 0
		for _, placeholder := range free {
			if placeholder.MaxLines < 2 {
				continue
			}
			room += placeholder.MaxLines
			if needed == 0 {
				needed = pptx.LinesNeeded(slide.Bullets, placeholder)
			}
		}
		switch {
		case room == 0:
			// Nowhere to write is worse than anywhere to write: every line is lost,
			// not merely crowded.
			score -= float64(len(slide.Bullets))*7 + 30
		case needed > room:
			// Every line that would not fit is a line the author wrote and the
			// audience will not read.
			score -= float64(needed-room) * 7
		default:
			score += 8
			// A layout three times larger than the text leaves a column staring
			// back at the room.
			if room > needed*3 && room-needed > 8 {
				score -= 5
			}
		}
	}
	// Whatever is still free and roomy would be drawn as an empty region.
	for _, placeholder := range free {
		if placeholder.MaxLines >= 3 {
			score -= 4
		}
	}
	return score
}

// slideRole is what a slide is for: what it declared, or what its position
// implies. Position only implies a role — a first slide that carries a component
// or a list of points is not a cover, whatever its position says, and putting it
// on a title layout would throw the content away.
func slideRole(slide SourceSlide, index, total int) string {
	if role := strings.TrimSpace(slide.Role); role != "" {
		return role
	}
	role := sourcePositionRole(index, total)
	if role == pptx.RoleTitle && (len(slide.Blocks) > 0 || len(slide.Bullets) > 1) {
		return pptx.RoleContent
	}
	// Nor is the last slide a closing page just because it is last. A closing
	// page is a title and an ask; a table or a plotted trend is the argument
	// still running, and closing layouts have nowhere to put one — the component
	// would be flattened into lines of "1월, 2월, 3월" under the title. When the
	// author says @closing we obey, but position alone does not decide this.
	if role == pptx.RoleClosing && len(slide.Blocks) > 0 {
		return pptx.RoleContent
	}
	return role
}

// dropEcho removes the points that only repeat a line the slide already shows.
func dropEcho(bullets []pptx.Paragraph, already ...string) []pptx.Paragraph {
	seen := map[string]bool{}
	for _, line := range already {
		if key := comparableLine(line); key != "" {
			seen[key] = true
		}
	}
	if len(seen) == 0 {
		return bullets
	}
	kept := make([]pptx.Paragraph, 0, len(bullets))
	for _, bullet := range bullets {
		key := comparableLine(bullet.Text)
		if key != "" && seen[key] {
			continue
		}
		if key != "" {
			seen[key] = true
		}
		kept = append(kept, bullet)
	}
	return kept
}

// comparableLine is a line stripped to what it says, so "2026 년" and "2026년"
// are recognised as the same sentence written twice.
func comparableLine(text string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(strings.TrimSpace(text)) {
		switch character {
		case ' ', '\t', '.', ',', '·', '!', '?', ':', ';', '"', '\'':
			continue
		}
		builder.WriteRune(character)
	}
	return builder.String()
}

// structuralRole marks the slides that carry the deck's shape rather than its
// argument.
func structuralRole(role string) bool {
	switch role {
	case pptx.RoleTitle, pptx.RoleSection, pptx.RoleClosing:
		return true
	}
	return false
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

// chartFallback is the component a chart becomes when its values are not
// numbers: labelled figures if every row has a value, otherwise nothing.
func chartFallback(block pptx.Block) (pptx.Block, string) {
	switch block.Kind {
	case pptx.BlockLine, pptx.BlockColumns, pptx.BlockBars, pptx.BlockShare, pptx.BlockMeter:
	default:
		return pptx.Block{}, ""
	}
	if len(block.Items) < 2 {
		return pptx.Block{}, ""
	}
	for _, item := range block.Items {
		if strings.TrimSpace(item.Label) == "" || strings.TrimSpace(item.Display(block.Unit)) == "" {
			return pptx.Block{}, ""
		}
	}
	fallback := block
	fallback.Kind = pptx.BlockKPI
	fallback.Series, fallback.Labels = nil, nil
	if len(fallback.Items) > 4 {
		fallback.Kind = pptx.BlockTimeline
	}
	// Values that are phrases rather than figures are a table of two columns: a
	// KPI row sets "개발/운영 분리" in the size of a headline number and it wraps
	// out of its card.
	if wordy(block) {
		fallback.Kind = pptx.BlockTable
		fallback.Columns = []string{"", ""}
		fallback.Rows = nil
		for _, item := range block.Items {
			fallback.Rows = append(fallback.Rows, []string{item.Label, item.Display(block.Unit)})
		}
		fallback.Items = nil
	}
	return fallback, fallback.Kind
}

// wordy reports whether a component's values are phrases rather than figures.
// Anything past a few characters is a phrase; "18%" and "4억" are not.
func wordy(block pptx.Block) bool {
	for _, item := range block.Items {
		if utf8.RuneCountInString(strings.TrimSpace(item.Display(block.Unit))) > 6 {
			return true
		}
	}
	return false
}

// spansWell reports whether a component reads better across the whole body than
// in one column of it. A matrix, a table and a chart do; a card row already
// divides its own frame into columns and gains nothing.
func spansWell(block pptx.Block) bool {
	switch block.Kind {
	case pptx.BlockTable, pptx.BlockLine, pptx.BlockColumns, pptx.BlockBars, pptx.BlockGrid:
		return true
	case pptx.BlockComparison:
		// Only the matrix shape, not the two-or-three-card shape.
		return pptx.IsComparisonMatrix(block)
	}
	return false
}

// twinSlot is the body region beside another one: same top, same height, free.
// Two regions like that are one region a layout divided, so a component may take
// them back.
func twinSlot(layout pptx.Layout, placeholder pptx.Placeholder, claimed map[string]bool) (pptx.Placeholder, bool) {
	for _, other := range layout.BodySlots() {
		if other.Slot == placeholder.Slot || claimed[other.Slot] || !other.AcceptsText() {
			continue
		}
		if other.Y != placeholder.Y || other.Height != placeholder.Height {
			continue
		}
		return other, true
	}
	return pptx.Placeholder{}, false
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
		// A region that holds one line is a caption, not a column: putting a share
		// of the points there crowds them into type nobody can read.
		if !claimed[placeholder.Slot] && placeholder.MaxLines >= 2 {
			slots = append(slots, placeholder)
		}
	}
	if len(slots) == 0 {
		// Nothing roomy enough: use whatever body region exists rather than
		// dropping the text.
		for _, placeholder := range layout.BodySlots() {
			if !claimed[placeholder.Slot] {
				slots = append(slots, placeholder)
				break
			}
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
		fitted, report := pptx.FitParagraphsReport(group, slots[index], language)
		content.SetField(slots[index].Slot, fitted)
		if report.Lost() {
			*warnings = append(*warnings, fmt.Sprintf("%s: %s", where, describeFitLoss(report, slots[index].Slot)))
		}
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

// resolveGrid finds a grid definition: the deployment's own first, then the
// shipped examples.
func resolveGrid(options CompileOptions, name string) (pptx.GridSpec, bool) {
	if options.ResolveGrid != nil {
		if spec, ok := options.ResolveGrid(name); ok {
			return spec, true
		}
	}
	return pptx.LookupBuiltinGrid(name)
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

// describeFitLoss says what a region could not hold, in the terms an author can
// act on: shorten the text, or move it to another slide.
func describeFitLoss(report pptx.FitReport, slot string) string {
	parts := make([]string, 0, 2)
	if report.Dropped > 0 {
		parts = append(parts, fmt.Sprintf("%d point(s) did not fit in %s and were left out", report.Dropped, slot))
	}
	if report.Shortened > 0 {
		parts = append(parts, fmt.Sprintf("%d line(s) were shortened to fit %s", report.Shortened, slot))
	}
	return strings.Join(parts, "; ")
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

// placeImageCaptions writes the caption of each placed image into a free text
// region, as a caption rather than as a point: no bullet, one line.
//
// It runs after the points, so a slide that has something to say keeps saying
// it; the caption fills a region that would otherwise be empty.
func placeImageCaptions(content *Content, layout pptx.Layout, claimed map[string]bool, language string) {
	for _, slot := range sortedSlots(content.Images) {
		caption := strings.TrimSpace(content.Images[slot].Caption)
		if caption == "" {
			continue
		}
		placeholder, found := freeTextSlot(*content, layout, claimed)
		if !found {
			return
		}
		content.SetField(placeholder.Slot, fit(placeholder, []pptx.Paragraph{{Text: caption, Lead: true}}, language))
		claimed[placeholder.Slot] = true
	}
}

// freeTextSlot is a body region with nothing in it at all. The points are
// distributed before this runs and do not mark the regions they fill, so asking
// only what is claimed would write a caption over what a slide already says.
func freeTextSlot(content Content, layout pptx.Layout, claimed map[string]bool) (pptx.Placeholder, bool) {
	for _, placeholder := range layout.BodySlots() {
		if claimed[placeholder.Slot] || !placeholder.AcceptsText() {
			continue
		}
		if len(content.Fields[placeholder.Slot]) > 0 {
			continue
		}
		if _, taken := content.Blocks[placeholder.Slot]; taken {
			continue
		}
		if _, taken := content.Images[placeholder.Slot]; taken {
			continue
		}
		return placeholder, true
	}
	return pptx.Placeholder{}, false
}

// sortedSlots is the slots of a map in a stable order, so a deck compiles the
// same way twice.
func sortedSlots(images map[string]ContentImage) []string {
	slots := make([]string, 0, len(images))
	for slot := range images {
		slots = append(slots, slot)
	}
	sort.Strings(slots)
	return slots
}
