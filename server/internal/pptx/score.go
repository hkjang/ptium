package pptx

import "sort"

// What the deck scores, and what a score is allowed to mean.
//
// The measurement pass already reads a drawn deck and reports what is wrong
// with it. A list of findings answers "what should I fix"; it does not answer
// the question anyone asks first, which is "is this ready". A score answers
// that, from the same measurements — nothing here is guessed, and nothing here
// is about whether the argument is any good.
//
// So the score is deliberately narrow: it measures the drawing. A deck of
// beautifully set slides saying nothing scores a hundred, and the product says
// so rather than implying otherwise.

// Score dimensions. Each one collects the kinds of finding a reader would call
// by the same name.
const (
	// DimensionReadability is whether the words can be read as set: text that
	// overran its box, ran off the slide, crowded the page, or left a stray
	// syllable on a line of its own.
	DimensionReadability = "readability"
	// DimensionStructure is whether the deck is finished: a slide with nothing
	// to say out loud, or the same point made twice.
	DimensionStructure = "structure"
	// DimensionVisual is whether the drawing holds together: two things on the
	// same spot.
	DimensionVisual = "visual"
	// DimensionAccessibility is whether it can be read by someone who is not
	// looking at it the way the author is: contrast above all.
	DimensionAccessibility = "accessibility"
	// DimensionEvidence is whether the deck can say where its numbers came from.
	DimensionEvidence = "evidence"
)

// dimensionOf maps a finding to the dimension it belongs to.
var dimensionOf = map[string]string{
	FindingOverflow: DimensionReadability,
	FindingOutside:  DimensionReadability,
	FindingDensity:  DimensionReadability,
	FindingOrphan:   DimensionReadability,
	FindingTrimmed:  DimensionReadability,
	FindingLink:     DimensionReadability,
	FindingNotes:    DimensionStructure,
	FindingRepeat:   DimensionStructure,
	// The two findings about a slide's heading were weighted as the costliest
	// advisories this product has — the heading is the line the room reads
	// before anything else — and then counted in no dimension at all, so a deck
	// whose cover stopped mid-sentence scored 100 with the finding printed
	// beside it. A weight nothing spends is not a weight.
	FindingUnfinished:  DimensionStructure,
	FindingTwiceTitled: DimensionStructure,
	FindingStale:       DimensionEvidence,
	FindingEcho:        DimensionStructure,
	FindingCollision:   DimensionVisual,
	FindingContrast:    DimensionAccessibility,
	FindingSource:      DimensionEvidence,
}

// weightOf is what one finding costs. A defect is something drawn wrong and
// costs several times what an advisory costs, because they are not the same
// news: a deck can be finished and imperfect, and it cannot be finished and
// broken.
func weightOf(finding Finding) int {
	if finding.Advisory {
		switch finding.Kind {
		case FindingDensity, FindingRepeat, FindingEcho:
			return 6
		case FindingTrimmed:
			// Content that is on no slide is worse than content that is crowded.
			return 10
		case FindingLink:
			// The markup is printed on the wall, which every reader can see.
			return 10
		case FindingSource:
			// A figure with no source is the advisory a company acts on first.
			return 12
		case FindingStale:
			// A plan whose first step is behind the room reading it is noticed by
			// everybody at once, and it costs the deck the rest of its argument.
			return 12
		case FindingUnfinished:
			// The heading is the line the room reads before anything else.
			return 12
		case FindingTwiceTitled:
			// Also the heading, and also visible from the back of the room —
			// but the words are a phrase, and which of the two should change is
			// the author's call rather than a mistake in the drawing.
			return 8
		default:
			return 4
		}
	}
	switch finding.Kind {
	case FindingOutside, FindingContrast:
		return 40
	default:
		return 30
	}
}

// DimensionScore is one axis of the score, with what it counted.
type DimensionScore struct {
	Key   string `json:"key"`
	Score int    `json:"score"`
	// Counted is how many findings this dimension is standing on, so a low
	// score can be traced rather than trusted.
	Counted int `json:"counted"`
}

// SlideScore is one slide's score and the worst thing measured on it.
type SlideScore struct {
	Slide int    `json:"slide"`
	Score int    `json:"score"`
	Worst string `json:"worst,omitempty"`
}

// QualityScore is a measured deck, scored.
type QualityScore struct {
	Total      int              `json:"total"`
	Dimensions []DimensionScore `json:"dimensions"`
	Slides     []SlideScore     `json:"slides"`
	// Weakest is the slide to open first. Zero means every slide measured clean.
	Weakest int `json:"weakest,omitempty"`
}

// dimensionOrder keeps the axes in the order a reader reads them.
var dimensionOrder = []string{DimensionReadability, DimensionStructure, DimensionVisual,
	DimensionAccessibility, DimensionEvidence}

// ScoreDeck turns measurements into a score.
//
// One rule, so the number can be explained in a sentence: every slide starts at
// a hundred and pays the weight of what was measured on it. A dimension is the
// average of the slides in that dimension, and the deck's score is the average
// of the dimensions. A deck of twenty slides with one crowded page is not the
// same news as five crowded pages, and averaging is what says so.
func ScoreDeck(findings []Finding, slides int) QualityScore {
	if slides < 1 {
		slides = 1
	}
	// cost[dimension][slide]
	cost := map[string]map[int]int{}
	counted := map[string]int{}
	perSlide := map[int]int{}
	worst := map[int]Finding{}
	for _, finding := range findings {
		dimension, known := dimensionOf[finding.Kind]
		if !known {
			continue
		}
		weight := weightOf(finding)
		slide := finding.Slide
		if slide < 1 || slide > slides {
			slide = 1
		}
		if cost[dimension] == nil {
			cost[dimension] = map[int]int{}
		}
		cost[dimension][slide] += weight
		counted[dimension]++
		perSlide[slide] += weight
		if current, ok := worst[slide]; !ok || weightOf(current) < weight {
			worst[slide] = finding
		}
	}

	score := QualityScore{}
	total := 0
	for _, dimension := range dimensionOrder {
		sum := 0
		for slide := 1; slide <= slides; slide++ {
			value := 100 - cost[dimension][slide]
			if value < 0 {
				value = 0
			}
			sum += value
		}
		score.Dimensions = append(score.Dimensions, DimensionScore{
			Key: dimension, Score: sum / slides, Counted: counted[dimension]})
	}

	for slide := 1; slide <= slides; slide++ {
		value := 100 - perSlide[slide]
		if value < 0 {
			value = 0
		}
		total += value
		entry := SlideScore{Slide: slide, Score: value}
		if finding, ok := worst[slide]; ok {
			entry.Worst = finding.Kind
		}
		score.Slides = append(score.Slides, entry)
	}
	// The deck's own score is what its slides average: the dimensions say where
	// the loss came from, the total says how much of it there was.
	score.Total = total / slides
	// The slide to open first is the lowest one, and the earliest of those when
	// several are equally bad.
	lowest := 101
	for _, slide := range score.Slides {
		if slide.Score < lowest {
			lowest, score.Weakest = slide.Score, slide.Slide
		}
	}
	if lowest >= 100 {
		score.Weakest = 0
	}
	sort.SliceStable(score.Slides, func(i, j int) bool { return score.Slides[i].Slide < score.Slides[j].Slide })
	return score
}
