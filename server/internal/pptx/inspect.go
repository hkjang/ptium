package pptx

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
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
	// FindingEcho is a slide that says what an earlier slide already said. It is
	// measured across the deck rather than inside one slide, which is where the
	// repetition that a room actually notices lives.
	FindingEcho = "echo"
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

// repeatedSlides reports a slide that says what an earlier slide already said.
//
// A deck listed three candidate offices as bullets on one slide and as a table
// on the next, with the same rents and the same commutes, and measured
// perfectly: repetition was only ever looked for inside a slide. A room reads
// the second one as padding, and the presenter has to explain why they are
// being shown it twice.
//
// Two lines that are nearly the same line are not enough — a deck may restate
// its own headline — so it takes several before a slide is called an echo.
func repeatedSlides(deck Deck) []Finding {
	lines := make([][][]string, len(deck.Slides))
	for index, slide := range deck.Slides {
		for _, sentence := range slideSentences(slide) {
			if words := contentWords(sentence); len(words) >= 4 {
				lines[index] = append(lines[index], words)
			}
		}
	}
	var findings []Finding
	for later := 1; later < len(deck.Slides); later++ {
		if len(lines[later]) < 2 {
			continue
		}
		for earlier := 0; earlier < later; earlier++ {
			shared := 0
			for _, line := range lines[later] {
				for _, before := range lines[earlier] {
					if wordOverlap(line, before) >= 0.6 {
						shared++
						break
					}
				}
			}
			// Most of this slide, and more than one line of it, is a slide the
			// room has already been shown.
			if shared >= 2 && shared*2 >= len(lines[later]) {
				findings = append(findings, Finding{Slide: later + 1, Kind: FindingEcho, Advisory: true,
					Detail: fmt.Sprintf("%d of this slide's %d points were already made on slide %d",
						shared, len(lines[later]), earlier+1)})
				break
			}
		}
	}
	return findings
}

// InspectDeck reports the defects of a whole deck.
func InspectDeck(manifest Manifest, deck Deck) []Finding {
	design := NewDesign(manifest)
	briefFigures := NewBriefFigures(deck.Brief)
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
		// Only the figures the brief did not give are asked about. A deck that
		// asks a board for 12억 원 states that number on every slide about the
		// ask, and the author is the source: telling them to cite their own
		// request teaches them to ignore the question. What the brief never
		// said is what the room will ask about.
		if unsourced := unbriefedFigures(slide, briefFigures); len(slide.Sources) == 0 &&
			carriesArgument(slide, layout) && len(unsourced) > 0 {
			findings = append(findings, Finding{Slide: index + 1, Kind: FindingSource, Advisory: true,
				Detail: "figures with no source: " + strings.Join(unsourced, ", ")})
		}
	}
	findings = append(findings, repeatedSlides(deck)...)
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

// unbriefedFigures lists the figures a slide states that the brief did not.
func unbriefedFigures(slide Slide, brief BriefFigures) []string {
	seen := map[string]bool{}
	var missing []string
	consider := func(text string) {
		for _, figure := range brief.Missing(text) {
			if seen[figure] {
				continue
			}
			seen[figure] = true
			missing = append(missing, figure)
		}
	}
	for _, block := range slide.Blocks {
		consider(block.Caption)
		for _, item := range block.Items {
			consider(item.Label + " " + item.Value + " " + item.Detail)
		}
		for _, row := range block.Rows {
			consider(strings.Join(row, " "))
		}
	}
	for _, paragraphs := range slide.Fields {
		for _, paragraph := range paragraphs {
			consider(paragraph.Text)
		}
	}
	sort.Strings(missing)
	return missing
}

// FiguresNotIn lists the figures in text that the brief does not state. It is
// how generation tells a number the author supplied from one the model brought
// in by itself.
func FiguresNotIn(brief, text string) []string {
	return NewBriefFigures(brief).Missing(text)
}

// BriefFigures is what a brief said in numbers, ready to be asked whether a
// deck's figure came from it.
type BriefFigures struct {
	// stated is each number the brief writes, whole. A deck figure has to be one
	// of these, not a run of digits found somewhere among them: a brief that
	// says 매출 1,240억 has not said 240억, and the deck that wrote 240억 has
	// understated the quarter fivefold. Compared as substrings that passed.
	stated  map[string]bool
	numbers []float64
}

// NewBriefFigures reads a brief once, for a deck that will be asked about it
// many times.
func NewBriefFigures(brief string) BriefFigures {
	read := BriefFigures{stated: map[string]bool{}}
	for _, match := range leadingNumber.FindAllString(brief, -1) {
		read.stated[digitsOnly(match)] = true
		if value, err := strconv.ParseFloat(digitsOnly(match), 64); err == nil && value > 0 {
			read.numbers = append(read.numbers, value)
		}
	}
	// Written large, a number shares no digits with the same number written
	// small: "1억 2천만" and "1.2억" have none in common. Read both as amounts.
	for _, match := range scaledNumber.FindAllString(brief, -1) {
		if value, ok := myriadValue(match); ok {
			read.numbers = append(read.numbers, value)
		}
	}
	// A brief that says "세 개 시스템" has said 3, and a deck that writes "3개"
	// is quoting it rather than inventing a figure. Counted in words the number
	// is still a number, in every language the deck is written in.
	for _, match := range countedInWords.FindAllStringSubmatch(brief, -1) {
		if value, ok := numberWords[strings.ToLower(match[1])]; ok {
			read.stated[strconv.Itoa(value)] = true
			read.numbers = append(read.numbers, float64(value))
		}
	}
	return read
}

// countedInWords matches a small number written as a word and the counter that
// follows it. The counter is required: "한" alone is the first syllable of half
// the words in Korean.
var countedInWords = regexp.MustCompile(`(?i)(하나|한|둘|두|셋|세|넷|네|다섯|여섯|일곱|여덟|아홉|열|` +
	`一|二|三|四|五|六|七|八|九|十)\s*(개|곳|명|건|가지|번|차례|단계|팀|부서|시스템|사|つ|件|名|社|部|回|年|か月)|` +
	`(one|two|three|four|five|six|seven|eight|nine|ten)`)

// numberWords is how a small number is written when it is not written in
// digits.
var numberWords = map[string]int{
	"하나": 1, "한": 1, "둘": 2, "두": 2, "셋": 3, "세": 3, "넷": 4, "네": 4,
	"다섯": 5, "여섯": 6, "일곱": 7, "여덟": 8, "아홉": 9, "열": 10,
	"一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6, "七": 7, "八": 8, "九": 9, "十": 10,
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
	"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
}

// Missing lists the figures in text the brief neither states nor implies.
func (b BriefFigures) Missing(text string) []string {
	var missing []string
	// A number can be written across what reads as two figures — "하루 1만 2천
	// 건" is one amount, and the brief that said 12,000 said it. Read the whole
	// runs of the line before judging any part of one.
	runs := scaledNumber.FindAllString(text, -1)
	for _, figure := range StatedFigures(text) {
		figure = strings.TrimSpace(figure)
		number := digitsOnly(leadingNumber.FindString(figure))
		if figure == "" || number == "" || b.states(number) {
			continue
		}
		if b.dividesTo(figure, number) {
			continue
		}
		if b.statesAmount(figure) {
			continue
		}
		if b.statesRunAround(figure, runs) {
			continue
		}
		missing = append(missing, figure)
	}
	return missing
}

// states reports whether the brief writes this number, either as these digits
// or as the same value written differently — 0.80 against the brief's 0.8.
func (b BriefFigures) states(number string) bool {
	if b.stated[number] {
		return true
	}
	value, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return false
	}
	return b.holds(value)
}

// statesAmount reports whether the brief gives this amount under another
// notation — 8천만 against a brief that wrote 80,000,000 or 0.8억.
func (b BriefFigures) statesAmount(figure string) bool {
	value, ok := myriadValue(figure)
	return ok && b.holds(value)
}

// statesRunAround reports whether this figure is part of a longer number the
// brief does state.
func (b BriefFigures) statesRunAround(figure string, runs []string) bool {
	figure = strings.TrimSpace(figure)
	for _, run := range runs {
		if !strings.Contains(run, figure) {
			continue
		}
		if value, ok := myriadValue(run); ok && b.holds(value) {
			return true
		}
	}
	return false
}

// holds reports whether any number the brief states is this one.
func (b BriefFigures) holds(value float64) bool {
	for _, stated := range b.numbers {
		if sameAmount(value, stated) {
			return true
		}
	}
	return false
}

// dividesTo reports whether a percentage is two of the brief's own numbers
// divided: a brief that says 34 people and 6 leavers has already said 17.6%,
// and asking where that came from is asking the author to cite their own
// arithmetic. The divisor has to be a real count — one in two is not a
// derivation, it is a coincidence with every brief that mentions a pair.
func (b BriefFigures) dividesTo(figure, number string) bool {
	if !strings.Contains(figure, "%") && !strings.Contains(figure, "퍼센트") {
		return false
	}
	share, err := strconv.ParseFloat(number, 64)
	if err != nil || share <= 0 || share > 100 {
		return false
	}
	for _, whole := range b.numbers {
		if whole < 10 {
			continue
		}
		for _, part := range b.numbers {
			// Strictly smaller: a number divided by itself is 100%, and a brief
			// with any repeated number would otherwise excuse every figure near it.
			if part >= whole {
				continue
			}
			if math.Abs(part/whole*100-share) < 0.05 {
				return true
			}
		}
	}
	return false
}

var leadingNumber = regexp.MustCompile(`\d[\d,.]*`)

// scaledNumber matches a number written the way Korean, Japanese and Chinese
// write large ones: a digit group, then the myriad scale words, possibly
// several times over — 1억 2천만, 9천5백만, 8000만, 1.2억.
var scaledNumber = regexp.MustCompile(`\d[\d,]*(?:\.\d+)?\s*` +
	`(?:[조억만천백십兆億亿萬万千百十](?:\s*\d[\d,]*(?:\.\d+)?)?\s*)+`)

// myriadScales is what each of those words multiplies by. The Latin suffixes
// are here too: a brief that costs something at 240,000 USD and a deck that
// writes $240k are the same money.
var myriadScales = map[rune]float64{
	'k': 1e3, 'm': 1e6, 'b': 1e9,
	'십': 10, '백': 100, '천': 1000, '만': 1e4, '억': 1e8, '조': 1e12,
	'十': 10, '百': 100, '千': 1000, '万': 1e4, '萬': 1e4,
	'億': 1e8, '亿': 1e8, '兆': 1e12,
}

// myriadValue reads such a run as the number it says.
//
// A brief that puts a rent at "월 1억 2천만 원" and a deck that writes it
// "1.2억/월" are stating the same amount, and telling the author the deck
// invented it is worse than saying nothing: a check that accuses the author of
// making up their own figures is one they learn to skip. Digits alone cannot
// see this — the two share no digit string at all.
func myriadValue(run string) (float64, bool) {
	var total, section, current float64
	haveCurrent, scaled := false, false
	digits := ""
	flush := func() {
		if digits == "" {
			return
		}
		if value, err := strconv.ParseFloat(digitsOnly(digits), 64); err == nil {
			current, haveCurrent = value, true
		}
		digits = ""
	}
	for _, r := range strings.ToLower(run) {
		switch {
		case r >= '0' && r <= '9', r == ',', r == '.':
			digits += string(r)
		case unicode.IsSpace(r):
			// A space inside a number is how it was typeset, not what it means.
		default:
			flush()
			scale, ok := myriadScales[r]
			if !ok {
				return 0, false
			}
			scaled = true
			// "만" on its own is ten thousand; "9천5백만" is what came before it.
			base := current
			if !haveCurrent {
				base = 0
				if section == 0 {
					base = 1
				}
			}
			if scale >= 1e4 {
				total += (section + base) * scale
				section = 0
			} else {
				section += base * scale
			}
			current, haveCurrent = 0, false
		}
	}
	flush()
	total += section + current
	if !scaled || total <= 0 {
		return 0, false
	}
	return total, true
}

// sameAmount reports whether two readings of a number are the same number.
func sameAmount(a, b float64) bool {
	return math.Abs(a-b) <= math.Max(math.Abs(b), 1)*1e-9
}

// digitsOnly makes "1,200" and "1200" the same number, which is the only
// difference between how a brief writes a figure and how a deck does.
func digitsOnly(value string) string {
	return strings.TrimSuffix(strings.TrimSpace(strings.ReplaceAll(value, ",", "")), ".")
}

// StatedFigures lists the figures in a line of text, by the same reading the
// measurement uses: money, shares and counts, never dates or durations.
// Generation asks this of a written deck to find the numbers the brief never
// gave it.
func StatedFigures(text string) []string {
	return statedFigure.FindAllString(aDate.ReplaceAllString(text, " "), -1)
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
	wide, wideText := 0, ""
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
			// A line that runs past the side of its own box is drawn over whatever
			// is beside it. "1억 5천만 원" in a card a third of a narrow region wide
			// was painted across the card next to it and the room read "1억 5천".
			//
			// The widest line is measured here rather than taken from inkBounds,
			// which only ever narrows a box to its ink: asked how far the text runs
			// past the box, it answers zero, and a check written on it can never
			// fire. Centred text is exempt — a label centred on a tick is meant to
			// be wider than the gap it sits in, and what it lands on is the
			// collision check's business.
			if !primitive.Wrap && primitive.Align != "ctr" && bounds.Width > 0 {
				widest := 0
				for _, paragraph := range primitive.Lines {
					widest = max(widest, textWidth(paragraph.Text, primitive.FontSize))
				}
				if widest-bounds.Width > wide {
					wide, wideText = widest-bounds.Width, text
				}
			}
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
	if wide > tolerance {
		findings = append(findings, Finding{Slot: placeholder.Slot, Kind: FindingOverflow,
			Detail: fmt.Sprintf("%s draws %q %.2fcm wider than the room it reserved",
				block.Kind, shorten(wideText, 28), emuToCm(wide))})
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
