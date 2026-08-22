package pptx

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Every defect this file looks for was first found by rendering a slide and
// looking at it: text spilling out of its box, a title sitting on a logo, a
// component escaping its frame, white text on a light photograph. Looking is a
// bad regression test, so the same judgements are made here in numbers.

// FindingKind classifies what is wrong with a drawn slide.
const (
	// FindingOverflow is text that cannot fit its region even after shrinking.
	FindingOverflow = "overflow"
	// FindingOutside is something drawn beyond the slide's edge.
	FindingOutside = "outside"
	// FindingCollision is two things drawn on top of each other.
	FindingCollision = "collision"
	// FindingContrast is text that cannot be read against what is behind it.
	FindingContrast = "contrast"
	// The kinds below are advisory. They describe a slide that is drawn correctly
	// and could still be better: nothing is broken, so nothing here justifies
	// rewriting an author's words to satisfy a measurement.
	// FindingOrphan is a line holding one stray word or syllable, which is the
	// detail that makes a deck look generated rather than written.
	FindingOrphan = "orphan"
	// FindingDensity is a slide carrying more than an audience can take in.
	FindingDensity = "density"
	// FindingNotes is a slide with nothing to say out loud.
	FindingNotes = "notes"
	// FindingRepeat is the same point made twice in different words. A model
	// writing to a line count pads rather than stops, and a padded slide reads as
	// generated — so it is measured rather than hoped away.
	FindingRepeat = "repeat"
	// FindingSource is a slide that states figures and says nowhere they came
	// from. In a company the first question asked of any number on a slide is
	// where it is from, and a deck that cannot answer is not finished — however
	// well it is drawn.
	FindingSource = "source"
)

// A slide is a thing someone stands next to and talks over. Two of its failures
// are about that rather than about drawing: too much on one slide, and nothing
// prepared to say. Both are measurable, so both are measured.
const (
	// MaximumPoints is the most top-level points one region should carry. Past
	// this an audience reads instead of listening. Exported because what the
	// measurement asks for and what a rewrite is asked to do must be the same
	// number: a slide told only to "shorten" comes back with eleven shorter
	// lines.
	MaximumPoints = 6
	// crowdedCapacity is the share of a region's lines above which a slide is
	// full rather than composed.
	crowdedCapacity = 0.92
)

// Finding is one defect in a drawn slide, in the terms an author can act on.
type Finding struct {
	Slide  int    `json:"slide,omitempty"`
	Slot   string `json:"slot,omitempty"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
	// Advisory separates a slide that is unfinished from one that is drawn wrong.
	// Text off the edge of a slide is a defect; a slide with no speaker notes is a
	// judgement about how ready the deck is, and conflating the two would train
	// people to ignore both.
	Advisory bool `json:"advisory,omitempty"`
}

// Defects returns the findings that are about the slide being drawn wrong, as
// opposed to being unfinished.
func Defects(findings []Finding) []Finding {
	result := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		if !finding.Advisory {
			result = append(result, finding)
		}
	}
	return result
}

func (f Finding) String() string {
	where := ""
	if f.Slide > 0 {
		where = fmt.Sprintf("slide %d", f.Slide)
	}
	if f.Slot != "" {
		if where != "" {
			where += " "
		}
		where += f.Slot
	}
	if where == "" {
		return f.Kind + ": " + f.Detail
	}
	return where + " " + f.Kind + ": " + f.Detail
}

// minimumAutofitScale mirrors the floor the renderer applies: below it text is
// not made smaller, so it simply does not fit. crowdedAutofitScale is the point
// at which a slide is too dense to read from the back of a room, which is a
// defect worth reporting even though PowerPoint would render it.
const (
	minimumAutofitScale = 40
	crowdedAutofitScale = 62
)

// InspectSlide reports what is wrong with one drawn slide.
func InspectSlide(manifest Manifest, layout Layout, slide Slide, design Design) []Finding {
	var findings []Finding
	slideWidth, slideHeight := manifest.SlideWidth, manifest.SlideHeight
	if slideWidth <= 0 || slideHeight <= 0 {
		slideWidth, slideHeight = 12192000, 6858000
	}
	// The regions this slide actually paints, in a stable order.
	type region struct {
		slot        string
		frame       Frame
		kind        string
		placeholder Placeholder
	}
	var regions []region
	spanned := slide.spannedSlots()
	for _, placeholder := range layout.Placeholders {
		if spanned[placeholder.Slot] {
			continue
		}
		placeholder = slide.Place(placeholder)
		frame := Frame{X: placeholder.X, Y: placeholder.Y, Width: placeholder.Width, Height: placeholder.Height}
		if block, ok := slide.Blocks[placeholder.Slot]; ok {
			frame = slide.blockFrame(layout, placeholder, block)
		}
		switch {
		case len(slide.Pictures[placeholder.Slot].Data) > 0:
			regions = append(regions, region{placeholder.Slot, frame, "picture", placeholder})
		case hasBlockIn(slide, placeholder.Slot):
			regions = append(regions, region{placeholder.Slot, frame, "component", placeholder})
		case len(slide.Fields[placeholder.Slot]) > 0:
			regions = append(regions, region{placeholder.Slot, frame, "text", placeholder})
		}
	}
	sort.SliceStable(regions, func(i, j int) bool { return regions[i].slot < regions[j].slot })

	for _, current := range regions {
		// Nothing may be drawn off the slide.
		if outside := outsideBy(current.frame, slideWidth, slideHeight); outside > slideWidth/200 {
			findings = append(findings, Finding{Slot: current.slot, Kind: FindingOutside,
				Detail: fmt.Sprintf("%s region extends %.2fcm past the slide edge", current.kind, emuToCm(outside))})
		}
		switch current.kind {
		case "text":
			findings = append(findings, inspectText(current.placeholder, slide.Fields[current.slot])...)
			findings = append(findings, inspectLineBreaks(current.placeholder, slide.Fields[current.slot])...)
			findings = append(findings, inspectDensity(current.placeholder, slide.Fields[current.slot])...)
		case "component":
			findings = append(findings, inspectComponent(current.placeholder, current.frame, slide.Blocks[current.slot], design, slideWidth, slideHeight)...)
		}
		// A composed region carries its own colours, so its readability is Ptium's
		// responsibility rather than the template's.
		if current.placeholder.Synthetic && current.kind == "text" {
			if behind := behindColor(layout, current.frame, manifest); behind != "" {
				if ratio := contrastRatio(current.placeholder.Color, behind); ratio < 4.5 {
					findings = append(findings, Finding{Slot: current.slot, Kind: FindingContrast,
						Detail: fmt.Sprintf("text %s on %s is %.1f:1, below 4.5:1",
							current.placeholder.Color, behind, ratio)})
				}
			}
		}
	}

	// Two things drawn over each other, and text drawn over the layout's own
	// artwork, are both defects a reader sees immediately.
	for i := 0; i < len(regions); i++ {
		for j := i + 1; j < len(regions); j++ {
			if share := overlapShare(regions[i].frame, regions[j].frame); share > 0.18 {
				findings = append(findings, Finding{Slot: regions[i].slot, Kind: FindingCollision,
					Detail: fmt.Sprintf("%s overlaps %s by %.0f%%", regions[i].kind, regions[j].slot, share*100)})
			}
		}
		if regions[i].kind != "text" {
			continue
		}
		if piece, share := artworkUnder(layout, regions[i].frame, slideWidth, slideHeight); share > 0.25 {
			findings = append(findings, Finding{Slot: regions[i].slot, Kind: FindingCollision,
				Detail: fmt.Sprintf("text covers %.0f%% of the layout's own %s", share*100, piece)})
		}
	}
	return findings
}

// inspectDensity reports a region carrying more than an audience can take in.
func inspectDensity(placeholder Placeholder, paragraphs []Paragraph) []Finding {
	switch placeholder.Slot {
	case SlotTitle, SlotSubtitle:
		return nil
	}
	points := 0
	for _, paragraph := range paragraphs {
		if paragraph.Level == 0 {
			points++
		}
	}
	if points > MaximumPoints {
		return []Finding{{Slot: placeholder.Slot, Kind: FindingDensity, Advisory: true,
			Detail: fmt.Sprintf("%d points in one region; past %d an audience reads instead of listening",
				points, MaximumPoints)}}
	}
	// A region filled to its last line has no air in it, even when every line fits.
	if placeholder.MaxLines > 3 {
		lineEm := placeholder.LineEm
		if lineEm <= 0 && placeholder.MaxChars > 0 {
			lineEm = float64(placeholder.MaxChars) / float64(placeholder.MaxLines) * referenceAdvance
		}
		used := 0
		for _, paragraph := range paragraphs {
			available := lineEm - float64(paragraph.Level)*2
			if available < 1 {
				available = 1
			}
			used += wrappedLines(paragraph.Text, available)
		}
		if share := float64(used) / float64(placeholder.MaxLines); share > crowdedCapacity {
			return []Finding{{Slot: placeholder.Slot, Kind: FindingDensity, Advisory: true,
				Detail: fmt.Sprintf("the region is %.0f%% full; a slide needs room to breathe", share*100)}}
		}
	}
	return nil
}

// slideSentences is every point a slide makes, in the terms an audience hears
// them: prose lines and the rows of its components.
func slideSentences(slide Slide) []string {
	var sentences []string
	for _, paragraphs := range slide.Fields {
		for _, paragraph := range paragraphs {
			sentences = append(sentences, paragraph.Text)
		}
	}
	for _, block := range slide.Blocks {
		for _, item := range block.Items {
			for _, part := range []string{item.Label, item.Value, item.Detail} {
				sentences = append(sentences, part)
			}
		}
		sentences = append(sentences, block.Text)
	}
	sort.Strings(sentences)
	return sentences
}

// repeatedPoints finds two lines that make the same point in different words.
//
// A model writing to a line count restates rather than stops, and a restatement
// keeps most of its words while changing their endings — so words are compared
// by their stems, and only lines long enough to carry an argument are compared
// at all. Parallel lines ("매출은 전년 대비 12% 늘었습니다" beside "비용은 전년
// 대비 8% 줄었습니다") are good writing and share too little to trip this.
func repeatedPoints(slide Slide) []Finding {
	sentences := slideSentences(slide)
	words := make([][]string, len(sentences))
	for index, sentence := range sentences {
		words[index] = contentWords(sentence)
	}
	var findings []Finding
	for index, first := range sentences {
		if len(words[index]) < 4 {
			continue
		}
		for other := index + 1; other < len(sentences); other++ {
			if len(words[other]) < 4 {
				continue
			}
			if wordOverlap(words[index], words[other]) >= 0.65 {
				findings = append(findings, Finding{Kind: FindingRepeat, Advisory: true,
					Detail: fmt.Sprintf("the same point twice: %q and %q",
						shorten(first, 34), shorten(sentences[other], 34))})
			}
		}
	}
	return findings
}

// contentWords is a line reduced to the words that carry its meaning.
func contentWords(text string) []string {
	fields := strings.FieldsFunc(strings.TrimSpace(text), func(r rune) bool {
		return r == ' ' || r == '\t' || strings.ContainsRune(".,·|()[]{}\"'`~!?:;-–—/", r)
	})
	words := make([]string, 0, len(fields))
	for _, field := range fields {
		if len([]rune(field)) >= 2 {
			words = append(words, field)
		}
	}
	return words
}

// wordOverlap is the share of the shorter line's words the longer one repeats,
// counting a word whose stem matches as the same word.
func wordOverlap(left, right []string) float64 {
	used := make([]bool, len(right))
	shared := 0
	for _, word := range left {
		for index, candidate := range right {
			if used[index] || !sameStem(word, candidate) {
				continue
			}
			used[index] = true
			shared++
			break
		}
	}
	smaller := min(len(left), len(right))
	if smaller == 0 {
		return 0
	}
	return float64(shared) / float64(smaller)
}

// sameStem reports whether two words are the same word with different endings.
func sameStem(first, second string) bool {
	if first == second {
		return true
	}
	shorter, longer := []rune(first), []rune(second)
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	// A two-syllable noun with an ending on it — 저하 and 저하됩니다, 비용 and
	// 비용은 — is the same word. One syllable is a coincidence.
	if len(shorter) < 2 {
		return false
	}
	return strings.HasPrefix(string(longer), string(shorter))
}

// InspectDeck reports the defects of a whole deck.
func InspectDeck(manifest Manifest, deck Deck) []Finding {
	design := NewDesign(manifest)
	var findings []Finding
	for index, slide := range deck.Slides {
		layout, ok := manifest.Layout(slide.LayoutID)
		if !ok {
			if layout, ok = manifest.LayoutForRole(RoleContent); !ok {
				continue
			}
		}
		for _, finding := range InspectSlide(manifest, layout, slide, design) {
			finding.Slide = index + 1
			findings = append(findings, finding)
		}
		// A slide with something to argue and nothing prepared to say is half
		// finished. A cover or a divider carries the room on its own.
		for _, finding := range repeatedPoints(slide) {
			finding.Slide = index + 1
			findings = append(findings, finding)
		}
		if strings.TrimSpace(slide.Notes) == "" && carriesArgument(slide, layout) {
			findings = append(findings, Finding{Slide: index + 1, Kind: FindingNotes, Advisory: true,
				Detail: "no speaker notes: nothing is written down to say over this slide"})
		}
		if len(slide.Sources) == 0 && carriesArgument(slide, layout) && statesFigures(slide) {
			findings = append(findings, Finding{Slide: index + 1, Kind: FindingSource, Advisory: true,
				Detail: "figures with no source: nothing on this slide says where its numbers came from"})
		}
	}
	return findings
}

// figurePattern is a number worth asking about: one with a unit, a percentage
// or a thousands separator. A page number or a step count is not a claim.
// aDate is a date or a duration, not a claim. "2026년 상반기" is when the deck is about, and
// asking it for a source teaches people to ignore the question. Durations are
// the same when a plan says when it will happen — "첫 2주에 할 일", "6개월 안에
// 확인할 지표" — so the units of time are not in the pattern below at all. The
// question is asked of money, shares and counts, which is what a room actually
// asks it of.
var aDate = regexp.MustCompile(`(19|20)\d{2}\s*(년|年|년도)?|\d[\d,.]*\s*(개월|시간|주일|분기|주차|일차|년|주|일|분|초)`)

var statedFigure = regexp.MustCompile(`\d[\d,.]*\s*(%|억|만|천|원|달러|명|건|개|배|퍼센트|` +
	`억원|만원|亿|億|円|元|USD|KRW|EUR|JPY|[kmb]n?\b)|\d{1,3}(,\d{3})+`)

// statesFigures reports whether a slide puts numbers in front of a room.
//
// A figure inside a component is the strongest case — a KPI card or a chart is
// nothing but figures — and a figure in the prose counts too. What does not
// count is a slide with no numbers at all: asking it for a source would train
// people to ignore the question.
func statesFigures(slide Slide) bool {
	for _, block := range slide.Blocks {
		switch block.Kind {
		case BlockKPI, BlockHero, BlockColumns, BlockBars, BlockLine, BlockShare, BlockMeter:
			if block.hasPlottableValues() || len(block.Items) > 0 {
				return true
			}
		}
		for _, item := range block.Items {
			if statesFigure(item.Value + " " + item.Label) {
				return true
			}
		}
		for _, row := range block.Rows {
			if statesFigure(strings.Join(row, " ")) {
				return true
			}
		}
	}
	for _, paragraphs := range slide.Fields {
		for _, paragraph := range paragraphs {
			if statesFigure(paragraph.Text) {
				return true
			}
		}
	}
	return false
}

// statesFigure reports whether one piece of text puts a number in front of a
// room, with dates read as dates.
func statesFigure(text string) bool {
	return statedFigure.MatchString(aDate.ReplaceAllString(text, " "))
}

// carriesArgument reports whether a slide makes a point, as opposed to opening or
// dividing the deck.
func carriesArgument(slide Slide, layout Layout) bool {
	switch layout.Role {
	case RoleTitle, RoleSection, RoleBlank:
		return false
	}
	if len(slide.Blocks) > 0 || len(slide.Pictures) > 0 {
		return true
	}
	for slot, paragraphs := range slide.Fields {
		if slot == SlotTitle || slot == SlotSubtitle {
			continue
		}
		if len(paragraphs) > 0 {
			return true
		}
	}
	return false
}

func hasBlockIn(slide Slide, slot string) bool {
	block, ok := slide.Blocks[slot]
	return ok && strings.TrimSpace(block.Kind) != ""
}

// inspectText reports copy that cannot fit even at the smallest size the
// renderer will use.
func inspectText(placeholder Placeholder, paragraphs []Paragraph) []Finding {
	if placeholder.MaxLines <= 0 || len(paragraphs) == 0 {
		return nil
	}
	// autofit already answers the question: how far must this text shrink to fit?
	// Anything under the crowding floor is a slide nobody at the back can read,
	// and at the hard floor the text does not fit at all.
	scale, _ := autofit(placeholder, paragraphs)
	if scale >= crowdedAutofitScale {
		return nil
	}
	needed := 0
	lineEm := placeholder.LineEm
	if lineEm <= 0 && placeholder.MaxChars > 0 {
		lineEm = float64(placeholder.MaxChars) / float64(placeholder.MaxLines) * referenceAdvance
	}
	for _, paragraph := range paragraphs {
		available := lineEm - float64(paragraph.Level)*2
		if available < 1 {
			available = 1
		}
		needed += wrappedLines(paragraph.Text, available)
	}
	detail := fmt.Sprintf("%d lines of text in room for %d; it must shrink to %.0f%% of the template's size",
		needed, placeholder.MaxLines, scale)
	if scale <= minimumAutofitScale {
		detail = fmt.Sprintf("%d lines of text in room for %d; it does not fit even at %.0f%%",
			needed, placeholder.MaxLines, scale)
	}
	return []Finding{{Slot: placeholder.Slot, Kind: FindingOverflow, Detail: detail}}
}

// inspectLineBreaks reports a heading whose wrap leaves a stray last line. It is
// only checked where a slide has one statement to make — a title, a lead, a
// component's heading — because a bulleted list of full sentences legitimately
// ends lines wherever the words fall.
func inspectLineBreaks(placeholder Placeholder, paragraphs []Paragraph) []Finding {
	switch placeholder.Slot {
	case SlotTitle, SlotSubtitle:
	default:
		return nil
	}
	lineEm := placeholder.LineEm
	if lineEm <= 0 && placeholder.MaxChars > 0 && placeholder.MaxLines > 0 {
		lineEm = float64(placeholder.MaxChars) / float64(placeholder.MaxLines) * referenceAdvance
	}
	for _, paragraph := range paragraphs {
		width, orphaned := orphanedLine(paragraph.Text, lineEm)
		if !orphaned {
			continue
		}
		// Advisory: the slide is drawn correctly, it just reads slightly
		// amateurish. Treating it as a defect would invite mangling a heading to
		// satisfy a measurement.
		return []Finding{{Slot: placeholder.Slot, Kind: FindingOrphan, Advisory: true,
			Detail: fmt.Sprintf("the last line holds %.0f%% of a line; shortening or rewording the text avoids the stray ending",
				width/lineEm*100)}}
	}
	return nil
}

// inspectComponent reports a drawing that escapes its own frame or the slide.
func inspectComponent(placeholder Placeholder, frame Frame, block Block, design Design, slideWidth, slideHeight int) []Finding {
	component := RenderBlock(design, frame, block)
	if len(component.Primitives) == 0 {
		// The region was meant to hold a drawing and holds nothing at all, which is
		// worse than a cramped one: the slide has a hole in it.
		return []Finding{{Slot: placeholder.Slot, Kind: FindingOverflow,
			Detail: fmt.Sprintf("%s had too little room to draw anything", block.Kind)}}
	}
	tolerance := slideWidth / 200
	worstFrame, worstSlide := 0, 0
	// A drawn line of text is as tall as the lines it wraps into, not as tall as
	// the box it was given. Measuring the box is how a component whose heading
	// covered its own first row passed inspection.
	var drawn []Frame
	overflow, overflowText := 0, ""
	for _, primitive := range component.Primitives {
		bounds := primitive.bounds()
		if bounds.Width <= 0 && bounds.Height <= 0 {
			continue
		}
		if primitive.Kind == shapeText {
			height, text := drawnTextHeight(primitive)
			if height > bounds.Height && height-bounds.Height > overflow {
				overflow, overflowText = height-bounds.Height, text
			}
			// Text is compared as ink rather than as its box: a month label
			// centred in the gap between two ticks owns a box seven centimetres
			// wide and draws two characters in it.
			ink := inkBounds(primitive)
			ink.Height = max(ink.Height, height)
			bounds.Height = max(bounds.Height, height)
			drawn = append(drawn, ink)
		}
		if beyond := outsideBy(bounds, slideWidth, slideHeight); beyond > worstSlide {
			worstSlide = beyond
		}
		if beyond := beyondFrame(bounds, frame); beyond > worstFrame {
			worstFrame = beyond
		}
	}
	var findings []Finding
	if overflow > tolerance {
		findings = append(findings, Finding{Slot: placeholder.Slot, Kind: FindingOverflow,
			Detail: fmt.Sprintf("%s draws %q %.2fcm taller than the room it reserved",
				block.Kind, shorten(overflowText, 28), emuToCm(overflow))})
	}
	// Two lines of a component's own text may not land on each other.
	for i := 0; i < len(drawn); i++ {
		for j := i + 1; j < len(drawn); j++ {
			if area := overlapArea(drawn[i], drawn[j]); area > 0 {
				height := min(drawn[i].Height, drawn[j].Height)
				width := min(drawn[i].Width, drawn[j].Width)
				if height > 0 && width > 0 && area*100/(height*width) > 12 {
					findings = append(findings, Finding{Slot: placeholder.Slot, Kind: FindingCollision,
						Detail: fmt.Sprintf("two lines of the %s overlap", block.Kind)})
					i = len(drawn)
					break
				}
			}
		}
	}
	if worstSlide > tolerance {
		findings = append(findings, Finding{Slot: placeholder.Slot, Kind: FindingOutside,
			Detail: fmt.Sprintf("%s draws %.2fcm past the slide edge", block.Kind, emuToCm(worstSlide))})
	}
	if worstFrame > tolerance {
		findings = append(findings, Finding{Slot: placeholder.Slot, Kind: FindingOutside,
			Detail: fmt.Sprintf("%s draws %.2fcm outside its region", block.Kind, emuToCm(worstFrame))})
	}
	return findings
}

// shorten keeps a quoted excerpt short enough to read in a finding.
func shorten(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

// drawnTextHeight is how tall a text primitive really is once its lines wrap,
// with the text that needed the most room.
func drawnTextHeight(primitive Primitive) (int, string) {
	if primitive.FontSize <= 0 {
		return 0, ""
	}
	total, worst, worstLines := 0, "", 0
	for _, paragraph := range primitive.Lines {
		lines := 1
		if primitive.Wrap {
			lines = cellLines(paragraph.Text, primitive.FontSize, primitive.Frame.Width)
		}
		if lines > worstLines {
			worst, worstLines = paragraph.Text, lines
		}
		total += lineHeightFor(primitive.FontSize) * lines
	}
	return total, worst
}

// overlapArea is the area two frames share.
func overlapArea(a, b Frame) int {
	width := min(a.X+a.Width, b.X+b.Width) - max(a.X, b.X)
	height := min(a.Y+a.Height, b.Y+b.Height) - max(a.Y, b.Y)
	if width <= 0 || height <= 0 {
		return 0
	}
	return width * height
}

// outsideBy is how far a frame reaches past the slide, in EMU.
func outsideBy(frame Frame, slideWidth, slideHeight int) int {
	worst := 0
	for _, over := range []int{-frame.X, -frame.Y,
		frame.X + frame.Width - slideWidth, frame.Y + frame.Height - slideHeight} {
		if over > worst {
			worst = over
		}
	}
	return worst
}

// beyondFrame is how far one frame reaches outside another.
func beyondFrame(inner, outer Frame) int {
	worst := 0
	for _, over := range []int{outer.X - inner.X, outer.Y - inner.Y,
		inner.X + inner.Width - (outer.X + outer.Width),
		inner.Y + inner.Height - (outer.Y + outer.Height)} {
		if over > worst {
			worst = over
		}
	}
	return worst
}

// overlapShare is the overlap between two frames as a share of the smaller one.
func overlapShare(first, second Frame) float64 {
	width := math.Min(float64(first.X+first.Width), float64(second.X+second.Width)) - math.Max(float64(first.X), float64(second.X))
	height := math.Min(float64(first.Y+first.Height), float64(second.Y+second.Height)) - math.Max(float64(first.Y), float64(second.Y))
	if width <= 0 || height <= 0 {
		return 0
	}
	smaller := math.Min(float64(first.Width)*float64(first.Height), float64(second.Width)*float64(second.Height))
	if smaller <= 0 {
		return 0
	}
	return width * height / smaller
}

// artworkUnder finds the layout's own decoration a frame sits on top of, ignoring
// backdrops: text belongs over a full-bleed photograph, not over a logo.
func artworkUnder(layout Layout, frame Frame, slideWidth, slideHeight int) (string, float64) {
	slideArea := float64(slideWidth) * float64(slideHeight)
	worst, name := 0.0, ""
	for _, piece := range layout.Artwork {
		if piece.Width <= 0 || piece.Height <= 0 {
			continue
		}
		// A filled shape behind text is a backing panel — the whole point of a
		// panel layout. Only a picture or the template's own lettering underneath
		// makes text unreadable.
		if piece.Kind != "picture" && piece.Kind != "text" {
			continue
		}
		pieceFrame := Frame{X: piece.X, Y: piece.Y, Width: piece.Width, Height: piece.Height}
		if float64(piece.Width)*float64(piece.Height)/slideArea >= backgroundCoverage {
			continue
		}
		share := overlapShare(pieceFrame, frame)
		if share > worst {
			worst, name = share, artworkName(piece)
		}
	}
	return name, worst
}

func artworkName(piece Artwork) string {
	switch piece.Kind {
	case "picture":
		return "picture"
	case "text":
		return "label"
	}
	return "shape"
}

// behindColor is what a frame's text is read against: the nearest artwork that
// covers it, or the background.
func behindColor(layout Layout, frame Frame, manifest Manifest) string {
	behind := layout.Fill.Fill
	if behind == "" {
		behind = layout.Background
	}
	if len(layout.Fill.Gradient) > 0 {
		behind = layout.Fill.Gradient[0].Color
	}
	for _, piece := range layout.Artwork {
		if piece.Kind == "text" || !covers(piece, frame) {
			continue
		}
		switch {
		case piece.Average != "":
			behind = piece.Average
		case piece.Fill != "":
			behind = piece.Fill
		case len(piece.Gradient) > 0:
			behind = piece.Gradient[0].Color
		}
	}
	if behind == "" {
		behind = manifest.Theme.Color("lt1")
	}
	return behind
}

func emuToCm(value int) float64 {
	return float64(value) / float64(EMUPerInch) * 2.54
}
