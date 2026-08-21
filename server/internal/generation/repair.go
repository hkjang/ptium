package generation

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// maximumRepairs bounds how many slides one generation will send back to the
// model. Repair costs a round trip each, and a deck where most slides overflow
// has a problem no amount of rewriting will fix.
const maximumRepairs = 3

// repairDeck measures the compiled deck and asks the model to rewrite the slides
// that do not fit.
//
// Generation used to hand over whatever came back and report the defects
// afterwards, which left the author to do the last pass by hand. The measurement
// is precise — "three lines of text in room for one" — and the model answers it
// well, so the loop belongs here rather than in the author's afternoon.
//
// A rewrite is kept only if it measures better than what it replaced. The model
// is asked, not trusted.
func (g *Generator) repairDeck(ctx context.Context, request writingRequest, result Deck, budget time.Duration) Deck {
	if len(result.Slides) == 0 {
		return result
	}
	// Repair gets as long as the writing took, and no longer. On a self-hosted
	// model a round trip is a minute, and a deck that arrives twice as late to be
	// slightly better fitted is not a better deck.
	deadline := time.Now().Add(budget)
	manifest := request.Template.Manifest
	presentation := request.Presentation
	presentation.Slides = result.Slides

	worst := slidesByDefect(manifest, presentation)
	if len(worst) == 0 {
		return result
	}
	repaired, attempted := 0, 0
	for _, candidate := range worst {
		if attempted >= maximumRepairs || attempted >= g.repairs {
			break
		}
		if attempted > 0 && time.Now().After(deadline) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%d more slide(s) do not fit, and there was no time left to rewrite them", len(worst)-attempted))
			break
		}
		if ctx.Err() != nil {
			break
		}
		attempted++
		position := candidate.position
		single := presentation
		single.Slides = []model.Slide{result.Slides[position-1]}
		revision := Revision{
			Presentation: presentation,
			Profile:      request.Profile,
			Template:     request.Template,
			Source:       deck.Format(single, manifest),
			Action:       candidate.action,
			Findings:     candidate.details,
			DeckOutline:  deckTitles(result.Slides, position),
		}
		revised, err := g.ReviseSlide(ctx, revision)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("slide %d could not be rewritten to fit: %s", position, err))
			continue
		}
		compiled := CompileGenerated(revised, single, request.Profile, request.Template)
		if len(compiled.Slides) != 1 {
			continue
		}
		proposal := compiled.Slides[0]
		proposal.ID = result.Slides[position-1].ID
		proposal.Position = position
		before := candidate.count
		after := defectsOnSlide(manifest, presentation, position, proposal)
		if after >= before {
			// The rewrite is no better than what it replaced. Keeping it would
			// churn the author's deck for nothing.
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("slide %d was left as written: %s", position, strings.Join(candidate.details, "; ")))
			continue
		}
		result.Slides[position-1] = proposal
		presentation.Slides = result.Slides
		repaired++
	}
	if repaired > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("%d slide(s) were measured against the template and rewritten to fit", repaired))
		// The source is the deck, so it has to say what the slides now say.
		result.Source = deck.Format(presentation, manifest)
	}
	return result
}

// slideDefect is one slide that does not fit, and what the measurement said.
type slideDefect struct {
	position int
	count    int
	details  []string
	// action is what to ask for. A slide crammed with ten points does not need
	// shorter words, it needs fewer points; a line that overflows its box does.
	action string
}

// repairable reports whether a finding is worth another draft.
//
// Nothing advisory justifies rewriting an author's words — but these words are
// the model's, and a slide crammed to a hundred and thirty percent of its region
// or saying the same thing twice is exactly what a second draft should fix.
// Missing speaker notes are left alone: that is a judgement about the deck, not
// about whether it fits.
func repairable(finding pptx.Finding) bool {
	switch finding.Kind {
	case pptx.FindingOverflow, pptx.FindingOutside, pptx.FindingCollision, pptx.FindingContrast,
		pptx.FindingDensity, pptx.FindingRepeat:
		return true
	}
	return false
}

// repairAction is what to ask the model for, given what the measurement found.
func repairAction(kind string) string {
	switch kind {
	case pptx.FindingDensity, pptx.FindingRepeat:
		return ReviseShorten
	}
	return ReviseFit
}

// slidesByDefect lists the slides worth another draft, worst first.
func slidesByDefect(manifest pptx.Manifest, presentation model.Presentation) []slideDefect {
	var findings []pptx.Finding
	for _, finding := range pptx.InspectDeck(manifest, deck.Build(presentation, manifest, "")) {
		if repairable(finding) {
			findings = append(findings, finding)
		}
	}
	bySlide := map[int]*slideDefect{}
	for _, finding := range findings {
		if finding.Slide <= 0 {
			continue
		}
		entry, ok := bySlide[finding.Slide]
		if !ok {
			entry = &slideDefect{position: finding.Slide}
			bySlide[finding.Slide] = entry
		}
		entry.count++
		if len(entry.details) < 4 {
			entry.details = append(entry.details, finding.Slot+": "+finding.Detail)
		}
		// Fitting wins over cutting when both are wanted: text that spills off the
		// slide is a defect, a crowded slide is a judgement.
		if entry.action != ReviseFit {
			entry.action = repairAction(finding.Kind)
		}
	}
	result := make([]slideDefect, 0, len(bySlide))
	for _, entry := range bySlide {
		result = append(result, *entry)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].count != result[j].count {
			return result[i].count > result[j].count
		}
		return result[i].position < result[j].position
	})
	return result
}

// defectsOnSlide measures one candidate slide in place of the current one.
func defectsOnSlide(manifest pptx.Manifest, presentation model.Presentation, position int, candidate model.Slide) int {
	trial := presentation
	trial.Slides = append([]model.Slide(nil), presentation.Slides...)
	trial.Slides[position-1] = candidate
	count := 0
	for _, finding := range pptx.InspectDeck(manifest, deck.Build(trial, manifest, "")) {
		if finding.Slide == position && repairable(finding) {
			count++
		}
	}
	return count
}

// deckTitles is the deck around one slide, so a rewrite does not repeat it.
func deckTitles(slides []model.Slide, position int) []string {
	titles := make([]string, 0, len(slides))
	for index, slide := range slides {
		marker := ""
		if index+1 == position {
			marker = " (this one)"
		}
		titles = append(titles, fmt.Sprintf("%d. %s%s", index+1, strings.TrimSpace(slide.Title), marker))
	}
	return titles
}
