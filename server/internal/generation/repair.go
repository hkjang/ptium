package generation

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
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
	// Which slides the model was asked about and gave nothing better for. They
	// are the author's now, and the note that says so has to say which.
	var declined []int
	for _, candidate := range worst {
		if attempted >= maximumRepairs || attempted >= g.repairs {
			break
		}
		if attempted > 0 && time.Now().After(deadline) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%d more slide(s) do not fit, and there was no time left to rewrite them", len(worst)-attempted))
			remaining := make([]int, 0, len(worst)-attempted)
			for _, left := range worst[attempted:] {
				remaining = append(remaining, left.position)
			}
			result.Notes = append(result.Notes,
				noTimeLeftNote(remaining, request.Presentation.Language))
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
		// A rewrite that deleted something always measures better. The slide that
		// found this had two columns, and the rewrite came back with one — the
		// heading of the other simply gone, and the defect count down, so it was
		// kept. Fewer words are the point of a rewrite; fewer headings and empty
		// regions are not.
		if lost := structureLost(result.Slides[position-1], proposal); lost != "" {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("slide %d was left as written: the rewrite %s", position, lost))
			declined = append(declined, position)
			continue
		}
		before := candidate.count
		after := defectsOnSlide(manifest, presentation, position, proposal)
		if after >= before {
			// The rewrite is no better than what it replaced. Keeping it would
			// churn the author's deck for nothing.
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("slide %d was left as written: %s", position, strings.Join(candidate.details, "; ")))
			declined = append(declined, position)
			continue
		}
		result.Slides[position-1] = proposal
		presentation.Slides = result.Slides
		repaired++
	}
	// A slide the model was asked about and gave nothing better for is the
	// author's now, and they should be told rather than left to discover it by
	// pressing the same button.
	if len(declined) > 0 {
		sort.Ints(declined)
		result.Notes = append(result.Notes, leftAsWrittenNote(declined, request.Presentation.Language))
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

// structureLost names what a rewrite dropped, or "" when it kept the shape of
// the slide it replaced.
//
// Content is counted as structure rather than as words: shortening a line is
// what a rewrite is for, and merging two into one is allowed, but a heading
// that no longer exists and a region that is now empty are losses whatever the
// measurement says about them.
func structureLost(before, after model.Slide) string {
	wasHeadings, wasRegions := slideShape(before)
	nowHeadings, nowRegions := slideShape(after)
	switch {
	case nowHeadings < wasHeadings:
		return fmt.Sprintf("dropped %d of the slide's %d headings", wasHeadings-nowHeadings, wasHeadings)
	case nowRegions < wasRegions:
		return fmt.Sprintf("left %d of the slide's %d regions empty", wasRegions-nowRegions, wasRegions)
	}
	return ""
}

// slideShape counts what a slide is made of: the headings it carries and the
// regions holding anything at all.
func slideShape(slide model.Slide) (headings, regions int) {
	content := deck.Decode(slide.Content)
	for slot, paragraphs := range content.Fields {
		filled := false
		for _, paragraph := range paragraphs {
			if strings.TrimSpace(paragraph.Text) == "" {
				continue
			}
			filled = true
			if paragraph.Lead && slot != pptx.SlotTitle {
				headings++
			}
		}
		if filled {
			regions++
		}
	}
	return headings, regions + len(content.Blocks) + len(content.Images)
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
		pptx.FindingDensity, pptx.FindingRepeat, pptx.FindingTrimmed:
		return true
	}
	return false
}

// repairAction is what to ask the model for, given what the measurement found.
func repairAction(kind string) string {
	switch kind {
	case pptx.FindingDensity, pptx.FindingRepeat, pptx.FindingTrimmed:
		// A component holding more than it draws is asked to say the same thing
		// in what the drawing shows. Telling the model the limit did not hold —
		// it wrote eight stages against a brief that named eight, twice, under a
		// brief that states the limit in two places. Measuring the result and
		// keeping it only if it improved does hold.
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

// maximumNoteWrites bounds how many slides one generation asks the model to
// find the words for. A deck where nothing has notes is one the model did not
// follow the brief on, and the measurement says so plainly.
const maximumNoteWrites = 4

// writeMissingNotes asks for what to say over the slides that have nothing.
//
// A slide that carries an argument and has no speaker notes is half finished:
// the brief the model is given says every slide but the cover ends with a notes
// line, and a live run of a 122B model returned eight slides with five of them
// bare. That is not a defect anyone can see in the drawing, which is exactly why
// it survives to the room — the deck looks done.
//
// Only the notes are asked for. The slide is already measured and fitted; a
// rewrite that also moved its words would undo work that is finished, so the
// result is kept only when the argument on the slide is unchanged.
func (g *Generator) writeMissingNotes(ctx context.Context, request writingRequest, result Deck,
	budget time.Duration) Deck {
	if len(result.Slides) == 0 {
		return result
	}
	deadline := time.Now().Add(budget)
	manifest := request.Template.Manifest
	presentation := request.Presentation
	presentation.Slides = result.Slides

	var bare []int
	for _, finding := range pptx.InspectDeck(manifest, deck.Build(presentation, manifest, "")) {
		if finding.Kind == pptx.FindingNotes && finding.Slide > 0 && finding.Slide <= len(result.Slides) {
			bare = append(bare, finding.Slide)
		}
	}
	if len(bare) == 0 {
		return result
	}
	written := 0
	for attempted, position := range bare {
		if attempted >= maximumNoteWrites || ctx.Err() != nil {
			break
		}
		if attempted > 0 && time.Now().After(deadline) {
			break
		}
		single := presentation
		single.Slides = []model.Slide{result.Slides[position-1]}
		revised, err := g.ReviseSlide(ctx, Revision{
			Presentation: presentation,
			Profile:      request.Profile,
			Template:     request.Template,
			Source:       deck.Format(single, manifest),
			Action:       ReviseNotes,
			Findings:     []string{"no speaker notes: nothing is written down to say over this slide"},
			DeckOutline:  deckTitles(result.Slides, position),
		})
		if err != nil {
			break
		}
		compiled := CompileGenerated(revised, single, request.Profile, request.Template)
		if len(compiled.Slides) != 1 {
			continue
		}
		proposal := compiled.Slides[0]
		proposal.ID = result.Slides[position-1].ID
		proposal.Position = position
		if strings.TrimSpace(proposal.SpeakerNotes) == "" {
			continue
		}
		// The slide itself is not what was asked about. A draft that came back
		// with different words on it is a rewrite nobody asked for, and the words
		// it replaced were already measured against the template.
		if changed := slideArgumentChanged(result.Slides[position-1], proposal); changed {
			// Take the notes and leave the slide as it was written.
			kept := result.Slides[position-1]
			kept.SpeakerNotes = proposal.SpeakerNotes
			proposal = kept
		}
		if defectsOnSlide(manifest, presentation, position, proposal) >
			defectsOnSlide(manifest, presentation, position, result.Slides[position-1]) {
			continue
		}
		result.Slides[position-1] = proposal
		presentation.Slides = result.Slides
		written++
	}
	if written > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("%d slide(s) had nothing written to say, and the words were written", written))
		result.Source = deck.Format(presentation, manifest)
	}
	return result
}

// slideArgumentChanged says whether a draft altered what the slide says, rather
// than only what is said about it.
func slideArgumentChanged(before, after model.Slide) bool {
	return strings.TrimSpace(before.Title) != strings.TrimSpace(after.Title) ||
		!bytes.Equal(bytes.TrimSpace(before.Content), bytes.TrimSpace(after.Content))
}

// leftAsWrittenNote tells the author what the product tried and could not do.
//
// The measurement panel already shows what is wrong with a slide. It does not
// say that the deck was sent back to the model about that slide and came back no
// better, and the difference matters to whoever opens the deck next: pressing
// "AI로 고치기" on a slide the model has already declined is a minute spent
// learning what the generation already knew.
func leftAsWrittenNote(positions []int, language string) string {
	which := slidesSaid(positions, language)
	switch {
	case strings.HasPrefix(strings.ToLower(language), "ja"):
		return fmt.Sprintf("%sはモデルに書き直させても良くならなかったため、そのままにしました。手で直すか、別の言い回しを指示してください。", which)
	case strings.HasPrefix(strings.ToLower(language), "zh"):
		return fmt.Sprintf("%s请模型重写后并没有变好，因此保持原样。请手动修改或换一种说法。", which)
	case strings.HasPrefix(strings.ToLower(language), "ko"), strings.TrimSpace(language) == "":
		return fmt.Sprintf("%s%s 모델에게 다시 쓰게 했지만 나아지지 않아 그대로 두었습니다. 손으로 고치거나 다른 표현을 지시해 주세요.",
			which, koreanTopic(which))
	}
	if len(positions) == 1 {
		return fmt.Sprintf("%s was sent back to the model and came back no better, so it was left as written. "+
			"Fix it by hand, or say what to change.", which)
	}
	return fmt.Sprintf("%s were sent back to the model and came back no better, so they were left as written. "+
		"Fix them by hand, or say what to change.", which)
}

// koreanTopic is 은 or 는 for what comes before it.
//
// Which one depends on the last syllable: 는 after a vowel, 은 after a final
// consonant. The phrase before it is not fixed — "4번 슬라이드" ends in a vowel
// and "슬라이드 9장" in a consonant — so the particle cannot be written into the
// sentence, and writing it there gave "슬라이드는" as "슬라이드은".
func koreanTopic(phrase string) string {
	runes := []rune(strings.TrimSpace(phrase))
	if len(runes) == 0 {
		return "은"
	}
	last := runes[len(runes)-1]
	if last >= 0xAC00 && last <= 0xD7A3 {
		if (last-0xAC00)%28 == 0 {
			return "는"
		}
		return "은"
	}
	// A number is read aloud, and four of the ten readings end in a vowel.
	switch last {
	case '2', '4', '5', '9':
		return "는"
	}
	return "은"
}

// slidesSaid names the slides something happened to, in the deck's own
// language, so the author can go and look at them.
//
// It says which rather than how many for two reasons. A note about slides an
// author now has to fix by hand is only useful if it says which ones. And in
// Korean a bare count could not be said at all: "3장은" is read as the third
// slide, not as three of them, so a note about three slides pointed at one.
//
// Past a handful the places stop being worth listing, and the count is said in
// a form that cannot be mistaken for a place.
func slidesSaid(positions []int, language string) string {
	const listed = 6
	korean := strings.HasPrefix(strings.ToLower(language), "ko") || strings.TrimSpace(language) == ""
	japanese := strings.HasPrefix(strings.ToLower(language), "ja")
	chinese := strings.HasPrefix(strings.ToLower(language), "zh")
	if len(positions) > listed || len(positions) == 0 {
		switch {
		case japanese:
			return fmt.Sprintf("%d 枚", len(positions))
		case chinese:
			return fmt.Sprintf("有 %d 页", len(positions))
		case korean:
			return fmt.Sprintf("슬라이드 %d장", len(positions))
		}
		return fmt.Sprintf("%d slides", len(positions))
	}
	places := make([]string, 0, len(positions))
	for _, at := range positions {
		places = append(places, strconv.Itoa(at))
	}
	switch {
	case japanese:
		return strings.Join(places, "·") + " 枚目"
	case chinese:
		return "第 " + strings.Join(places, "·") + " 页"
	case korean:
		return strings.Join(places, "·") + "번 슬라이드"
	}
	if len(places) == 1 {
		return "Slide " + places[0]
	}
	return "Slides " + strings.Join(places[:len(places)-1], ", ") + " and " + places[len(places)-1]
}

// noTimeLeftNote says the deck was handed over before every slide had been
// measured against the template — which is the author's to finish, with the
// workspace's own "fix it" on the slides the panel lists.
func noTimeLeftNote(positions []int, language string) string {
	which := slidesSaid(positions, language)
	switch {
	case strings.HasPrefix(strings.ToLower(language), "ja"):
		return fmt.Sprintf("残り %sは書き直す時間がありませんでした。編集画面の測定結果から直せます。", which)
	case strings.HasPrefix(strings.ToLower(language), "zh"):
		return fmt.Sprintf("%s没有时间重写。可在编辑器的检查结果中修复。", which)
	case strings.HasPrefix(strings.ToLower(language), "ko"), strings.TrimSpace(language) == "":
		return fmt.Sprintf("%s%s 다시 쓸 시간이 없었습니다. 편집기의 검사 결과에서 고칠 수 있습니다.",
			which, koreanTopic(which))
	}
	return fmt.Sprintf("%s had no time left to be rewritten. The editor's measurements will fix them.", which)
}
