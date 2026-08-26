package generation

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/library"
)

// writeSource turns an outline into deck source.
//
// The output is the same language a model is asked to produce, so an air-gapped
// deployment and a connected one differ in how good the prose is, not in how the
// deck is built. Everything here is derived from the prompt: the topics are the
// ones it named, the figures are the ones it supplied, and the frames decide the
// shape of each slide.
func writeSource(outline promptOutline, plan deckPlanCopy, count int) string {
	return writeSourceWith(outline, plan, count, nil)
}

// writeSourceWith is writeSource with the pictures this account has uploaded.
//
// Without a model there is nobody to choose one, so the choice is made the way
// the slide library's is: by name. A topic called "현장 자동화" and a picture
// called "현장 자동화" are the same subject, and a deck that leaves the picture
// in the library while writing about it is a deck of nothing but text — which
// is what every air-gapped deck was until now.
func writeSourceWith(outline promptOutline, plan deckPlanCopy, count int, pictures []library.Entry) string {
	var builder strings.Builder
	write := func(format string, args ...any) {
		fmt.Fprintf(&builder, format, args...)
		builder.WriteString("\n")
	}

	// The cover.
	write("# %s", plan.Title)
	write("@cover")
	if lead := plan.CoverLead; lead != "" {
		write("> %s", lead)
	}
	write("!notes %s", plan.CoverNotes)
	write("")

	slots := count - 1
	closing := slots >= 3
	if closing {
		slots--
	}
	if slots < 1 {
		return strings.TrimRight(builder.String(), "\n") + "\n"
	}

	// A long deck opens with its contents. Past half a dozen slides an audience
	// wants to know where they are going and where the decision sits; below that
	// a contents page is a slide spent saying what the next four slides say.
	if count >= 7 && len(outline.Topics) >= 3 && plan.AgendaTitle != "" {
		write("# %s", plan.AgendaTitle)
		write("@content")
		for _, topic := range outline.Topics {
			write("- %s", capitalized(headingName(topic.Name)))
		}
		if plan.AgendaNotes != "" {
			write("!notes %s", plan.AgendaNotes)
		}
		write("")
		slots--
	}

	// Each topic gets at least one slide; when there is room to spare, the
	// earlier topics get the extra ones, because that is where an argument needs
	// to be built rather than summarised.
	shares := shareSlides(len(outline.Topics), slots)
	position := 0
	// Two slides in a row arguing the same way read as one slide printed twice —
	// especially once a lead drops the subject the title already carries, which is
	// what makes two different subjects sound identical.
	//
	// The same holds across the whole deck, not just between neighbours: two
	// topics that both ask to be argued as a sequence rotate the same way, and a
	// deck came out with the expected outcome — the same figures, the same lead —
	// on two different slides. A frame the deck has already used gives way to one
	// it has not.
	previousFrame := ""
	usedFrames := map[string]bool{}
	// The pictures already placed, so one is never used twice.
	placed := map[string]bool{}
	// The shapes the brief's own numbers are in. Read once: a deck draws each of
	// them at most once, and only where they belong.
	charts := chartsFromFigures(outline.Figures, localizedCopy(plan.Language))
	// The rows a chart has already put on a slide.
	var drawn []string
	for index, topic := range outline.Topics {
		share := shares[index]
		for part := 0; part < share; part++ {
			// A second slide about the same topic argues a different aspect of it;
			// repeating the first one is worse than not having it.
			// A frame the topic's own words chose is the slide that heading is
			// asking for, and variety is worth less than a body that belongs
			// under its heading. It gives way only when an earlier slide has
			// already been that kind of slide — two sections both about cost
			// would otherwise print the same three points twice.
			frame, fresh := topic.Frame, true
			// A frame the words chose is kept unless this deck has already been
			// that kind of slide: no two slides here open with the same lead, so
			// the second section that wants the same frame takes another angle.
			// Which is the one thing still worth improving here — a second
			// schedule section reads better as a schedule than as a risk slide,
			// and that needs a second body for each frame rather than a rule.
			claimed := usedFrames[frame]
			if !topic.Chosen || part > 0 || claimed {
				// A topic whose words said nothing takes an angle nobody else
				// asked for: rotating it into the frame a later section named by
				// its own words leaves that section arguing something else.
				wanted := framesWanted(outline.Topics, index)
				frame, fresh = unusedFrame(topic.Frame, part, previousFrame, usedFrames, wanted)
			}
			if !fresh {
				// Every angle is spoken for. Another slide about this topic would
				// repeat one word for word, and the questions the deck raises next
				// are worth more than a page printed twice.
				break
			}
			// Where the subject is the deck's own title, this slide will be
			// titled by the part of the subject it is — which has to be the part
			// it actually argues. When a section the brief listed already carries
			// that name, the slide takes a different angle rather than a
			// different name for the angle it has: a slide titled 현황 that opens
			// "무엇이 어떻게 달라지는지 지표로 말합니다" is the mismatch this
			// release set out to remove, arriving by the back door.
			named := withoutSpaces(topic.Name) == withoutSpaces(plan.Title)
			if named {
				// And it steps aside for the sections the brief listed: their
				// words asked for those angles, and this slide is the deck's
				// subject taking whichever one is left.
				if angle, ok := unclaimedFrame(plan.Language, frame, outline.Topics,
					usedFrames, framesWanted(outline.Topics, index)); ok {
					frame = angle
				}
			}
			previousFrame = frame
			usedFrames[frame] = true
			section := plan.Section(promptTopic{Name: topic.Name, Frame: frame, Chosen: topic.Chosen}, part, share)
			// A deck whose title is its only subject would say that title on every
			// slide: once as the cover and then as the first half of "제목 —
			// 이행 순서", "제목 — 기대 효과", "제목 — 비용과 효과". The room reads
			// the same twelve syllables four times and learns nothing from three
			// of them. Where the subject is the deck's own title, each slide is
			// titled by the part of it that slide is.
			if named || strings.TrimSpace(section.Title) == strings.TrimSpace(plan.Title) {
				if aspect := frameTitleSuffix[plan.Language][frame]; aspect != "" {
					section.Title = capitalized(aspect)
				}
			}
			write("# %s", section.Title)
			if section.Role != "" {
				write("@%s", section.Role)
			}
			// A picture goes on the slide about the thing it is a picture of, and
			// on the first such slide only: the same photograph on four slides is
			// wallpaper.
			if picture, ok := pictureFor(topic.Name, section.Title, pictures, placed); ok {
				placed[picture] = true
				write("::image %s", picture)
			}
			// A lead that opens with the words already in the title says the same
			// thing twice on one slide — the measurement calls that out, and a
			// reader sees it before the measurement does.
			if lead := withoutSubject(section.Lead, section.Title, topic.Name); lead != "" {
				write("> %s", lead)
			}
			// A brief full of numbers earns a slide that draws them. The charts
			// come from the figures the prompt gave and are used where the slide
			// is already about those numbers — the case for doing this, and what
			// changes if it happens — rather than being pushed onto a slide about
			// something else.
			if len(charts) > 0 && chartFrames[frame] {
				chart := charts[0]
				charts = charts[1:]
				for _, row := range chart.Rows {
					drawn = append(drawn, row)
				}
				write("::%s %s", chart.Kind, chart.Caption)
				for _, row := range chart.Rows {
					write("- %s", row)
				}
				write("::")
				if section.Notes != "" {
					write("!notes %s", section.Notes)
				}
				write("")
				position++
				continue
			}
			switch {
			case section.Block != "" && len(section.Items) > 0:
				items := section.Items
				if section.Block == "kpi" {
					// The same figures the deck already drew are not headline
					// numbers a second time, and a label is the thing counted
					// rather than the words in front of it in the brief.
					items = headlineFigures(items, drawn)
				}
				if len(items) == 0 {
					// Everything this block held is already on a chart.
					for _, point := range section.Points {
						write("- %s", point)
					}
					break
				}
				write("::%s%s", section.Block, optional(" ", section.BlockCaption))
				for _, item := range items {
					write("- %s", item)
				}
				write("::")
			default:
				for _, point := range section.Points {
					write("- %s", point)
				}
			}
			if section.Notes != "" {
				write("!notes %s", section.Notes)
			}
			write("")
			position++
		}
	}
	// A deck asked for more slides than the prompt gives topics: the remainder
	// becomes the questions a reader of this deck would ask next — each asked
	// once. Past that the deck simply ends. Nine slides that each say something
	// is a better answer to "twelve" than twelve with the same page three times.
	for extras := 0; position < slots && extras < followupCount; extras++ {
		extra := plan.Followup(extras)
		write("# %s", extra.Title)
		if extra.Lead != "" {
			write("> %s", extra.Lead)
		}
		for _, point := range extra.Points {
			write("- %s", point)
		}
		if extra.Notes != "" {
			write("!notes %s", extra.Notes)
		}
		write("")
		position++
	}

	if closing {
		write("# %s", plan.ClosingTitle)
		write("@closing")
		if plan.ClosingLead != "" {
			write("> %s", plan.ClosingLead)
		}
		for _, point := range plan.ClosingPoints {
			write("- %s", point)
		}
		write("!notes %s", plan.ClosingNotes)
	}
	return strings.TrimRight(builder.String(), "\n") + "\n"
}

// withoutSubject drops the subject from a lead when the title already carries it.
//
// The plan writes "결제 시스템 이중화 계획을 순서대로 나눠 봅니다" so the sentence
// stands on its own; under a title that already says 결제 시스템 이중화 계획 it is
// the same words twice. Korean drops a known subject without any repair — the
// remainder is a sentence.
func withoutSubject(lead, title, topic string) string {
	lead = strings.TrimSpace(lead)
	title = strings.TrimSpace(title)
	if lead == "" || title == "" {
		return lead
	}
	for _, subject := range []string{topic, title} {
		subject = strings.TrimSpace(subject)
		if subject == "" || !strings.HasPrefix(title, subject) || !strings.HasPrefix(lead, subject) {
			continue
		}
		// The particle the subject was carrying goes with it. Japanese and
		// Chinese leave one behind too — "…の現在地を整理します" — and a lead that
		// opens on a particle is not a sentence in any of these languages.
		rest := strings.TrimLeft(strings.TrimPrefix(lead, subject), "은는이가을를의에서도のをはがにでとも的和与及")
		rest = strings.TrimSpace(rest)
		// Only when what is left is still a sentence rather than a fragment.
		if utf8.RuneCountInString(rest) >= 6 {
			return rest
		}
	}
	return lead
}

// framePart varies how a topic is argued across the slides it gets. A roadmap
// followed by its expected outcome reads like a deck; the same roadmap twice
// reads like a bug.
func framePart(frame string, part int) string {
	if part == 0 {
		return frame
	}
	rotation := map[string][]string{
		frameSequence:  {frameOutcome, frameRisk, frameCase},
		frameCase:      {frameOptions, frameOutcome, frameRisk},
		frameOptions:   {frameCase, frameRisk, frameOutcome},
		frameRisk:      {frameSequence, frameOutcome, frameCase},
		frameOutcome:   {frameSequence, frameCase, frameRisk},
		frameSituation: {frameCase, frameSequence, frameRisk},
	}
	alternatives, ok := rotation[frame]
	if !ok || len(alternatives) == 0 {
		return frame
	}
	return alternatives[(part-1)%len(alternatives)]
}

// shareSlides hands out the available slides across topics, giving the earlier
// ones the surplus.
// maximumTopicSlides bounds how far one subject is stretched.
//
// A topic is argued from a different angle on each of its slides — where it
// stands, what it costs, what could go wrong, what changes — and there are four
// such angles. A fifth slide about the same subject can only repeat one of them,
// which is what a reader sees as padding.
const maximumTopicSlides = 4

// followupCount is how many closing questions a deck can carry. Each is asked
// once; the second "남은 질문" in one deck is filler.
const followupCount = 3

func shareSlides(topics, slots int) []int {
	if topics <= 0 {
		return nil
	}
	shares := make([]int, topics)
	if slots <= topics {
		// Not enough room for every topic: the first ones get a slide each and the
		// rest are covered by the slides that exist.
		for index := range shares {
			if index < slots {
				shares[index] = 1
			}
		}
		return shares
	}
	base := min(slots/topics, maximumTopicSlides)
	surplus := slots - base*topics
	for index := range shares {
		shares[index] = base
		if index < surplus && shares[index] < maximumTopicSlides {
			shares[index]++
		}
	}
	return shares
}

func optional(prefix, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return prefix + value
}

// unusedFrame picks the angle a slide takes: the one the topic's own words ask
// for, then the rotation, and finally any angle the deck has not used yet.
//
// A deck of two topics that both read as a plan rotated identically, and the
// expected outcome — the same figures, the same lead, the same notes — came out
// on two slides. Only when every angle has been used does one repeat.
func unusedFrame(frame string, part int, previous string, used, wanted map[string]bool) (string, bool) {
	candidates := make([]string, 0, 4+len(defaultFrames))
	for attempt := 0; attempt <= 3; attempt++ {
		candidates = append(candidates, framePart(frame, part+attempt))
	}
	candidates = append(candidates, frameSituation, frameSequence, frameCase, frameOptions, frameRisk, frameOutcome)
	// The angles another section's own words asked for are left to it, unless
	// there is nothing else left to take.
	for _, candidate := range candidates {
		if candidate != previous && !used[candidate] && !wanted[candidate] {
			return candidate, true
		}
	}
	for _, candidate := range candidates {
		if candidate != previous && !used[candidate] {
			return candidate, true
		}
	}
	return candidates[0], false
}

// chartFrames are the slides a chart belongs on: the ones already about the
// numbers. A risk slide or a roadmap drawn as a bar chart is a chart put where
// there was room, not where it says something.
var chartFrames = map[string]bool{frameOutcome: true, frameCase: true, frameSituation: true}

// headlineFigures is what a kpi block should carry: the numbers this deck has
// not already drawn, named by the thing counted, and few enough to be headlines.
//
// The rows come from the brief's own figures, whose labels are the words that
// stood in front of them — "보고. 서울", "달성률은 매출". On a chart that is a
// bar nobody can read; in a headline it is worse, because a headline is all
// there is.
func headlineFigures(items []string, drawn []string) []string {
	const headlines = 4
	already := map[string]bool{}
	for _, row := range drawn {
		if _, value, ok := strings.Cut(row, "|"); ok {
			already[strings.TrimSpace(value)] = true
		}
	}
	kept := make([]string, 0, len(items))
	for _, item := range items {
		label, value, ok := strings.Cut(item, "|")
		if !ok {
			kept = append(kept, item)
			continue
		}
		if already[strings.TrimSpace(value)] {
			continue
		}
		tidy := chartLabel(reading{label: strings.TrimSpace(label), timely: timeLabel.MatchString(label)})
		kept = append(kept, tidy+" | "+strings.TrimSpace(value))
		if len(kept) == headlines {
			break
		}
	}
	return kept
}

// pictureFor is the picture this slide is about, if the account has one.
//
// The match is the library's, which is the same question asked of slides: is
// what this is called what that is called? Nothing here searches the words of
// the slide for something picture-ish — a deck that puts a photograph on a page
// because both mention "현장" is decoration, and decoration nobody chose is
// worse than white space.
func pictureFor(topic, title string, pictures []library.Entry, placed map[string]bool) (string, bool) {
	if len(pictures) == 0 || len(placed) >= 2 {
		return "", false
	}
	for _, name := range []string{topic, title} {
		entry, ok := library.Match(name, pictures)
		if !ok || placed[entry.Name] {
			continue
		}
		return entry.Name, true
	}
	return "", false
}

// unclaimedFrame is the angle this slide can take under a name no section of
// the deck already has.
//
// Renaming the slide instead — giving a metrics slide the title 현황 because
// 기대효과 was taken — puts the heading and the body back out of step, which is
// the thing this writer was just taught not to do.
func unclaimedFrame(language, frame string, topics []promptTopic, used, wanted map[string]bool) (string, bool) {
	if aspectFree(language, frame, topics) && !wanted[frame] {
		return frame, true
	}
	for _, other := range []string{frameSituation, frameSequence, frameCase, frameOptions, frameRisk, frameOutcome} {
		if other == frame || used[other] || wanted[other] || !aspectFree(language, other, topics) {
			continue
		}
		return other, true
	}
	// Every angle is either taken or wanted. Keeping this frame is better than
	// naming the slide after one it does not argue.
	if aspectFree(language, frame, topics) {
		return frame, true
	}
	return frame, false
}

// framesWanted is the angles the sections the brief listed asked for by name.
func framesWanted(topics []promptTopic, except int) map[string]bool {
	wanted := map[string]bool{}
	for index, topic := range topics {
		if index != except && topic.Chosen {
			wanted[topic.Frame] = true
		}
	}
	return wanted
}

// aspectFree says whether this frame's own word is still nobody's section name.
func aspectFree(language, frame string, topics []promptTopic) bool {
	aspect := frameTitleSuffix[language][frame]
	if aspect == "" {
		return false
	}
	for _, topic := range topics {
		if withoutSpaces(strings.ToLower(topic.Name)) == withoutSpaces(strings.ToLower(aspect)) {
			return false
		}
	}
	return true
}

// unclaimedAspect names the part of the subject this slide is, avoiding a name
// the deck already gives a section of its own.
//
// A brief that lists 기대효과 as a section produced a deck with the listed
// section and, ahead of it, the subject's own outcome slide titled "기대 효과":
// one section twice, told apart by a space. The frame's own word is used when
// nothing else claims it, and otherwise the deck says which part this is in a
// word no section has taken.
func unclaimedAspect(language, frame string, topics []promptTopic) string {
	taken := map[string]bool{}
	for _, topic := range topics {
		taken[withoutSpaces(strings.ToLower(topic.Name))] = true
	}
	first := frameTitleSuffix[language][frame]
	if first != "" && !taken[withoutSpaces(strings.ToLower(first))] {
		return first
	}
	// The deck already has a section by that name. Any other aspect of the
	// subject is a better title than the same words twice.
	for _, other := range []string{frameSituation, frameSequence, frameCase, frameOptions, frameRisk, frameOutcome} {
		if other == frame {
			continue
		}
		aspect := frameTitleSuffix[language][other]
		if aspect != "" && !taken[withoutSpaces(strings.ToLower(aspect))] {
			return aspect
		}
	}
	return first
}

func withoutSpaces(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value)
}
