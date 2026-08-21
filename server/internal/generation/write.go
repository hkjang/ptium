package generation

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// writeSource turns an outline into deck source.
//
// The output is the same language a model is asked to produce, so an air-gapped
// deployment and a connected one differ in how good the prose is, not in how the
// deck is built. Everything here is derived from the prompt: the topics are the
// ones it named, the figures are the ones it supplied, and the frames decide the
// shape of each slide.
func writeSource(outline promptOutline, plan deckPlanCopy, count int) string {
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
	for index, topic := range outline.Topics {
		share := shares[index]
		for part := 0; part < share; part++ {
			// A second slide about the same topic argues a different aspect of it;
			// repeating the first one is worse than not having it.
			frame, fresh := unusedFrame(topic.Frame, part, previousFrame, usedFrames)
			if !fresh {
				// Every angle is spoken for. Another slide about this topic would
				// repeat one word for word, and the questions the deck raises next
				// are worth more than a page printed twice.
				break
			}
			previousFrame = frame
			usedFrames[frame] = true
			section := plan.Section(promptTopic{Name: topic.Name, Frame: frame}, part, share)
			// A deck whose title is its only subject would open with that title
			// twice. The second one says which part of the subject the slide is.
			if strings.TrimSpace(section.Title) == strings.TrimSpace(plan.Title) {
				if aspect := frameTitleSuffix[plan.Language][frame]; aspect != "" {
					section.Title = capitalized(aspect)
				}
			}
			write("# %s", section.Title)
			if section.Role != "" {
				write("@%s", section.Role)
			}
			// A lead that opens with the words already in the title says the same
			// thing twice on one slide — the measurement calls that out, and a
			// reader sees it before the measurement does.
			if lead := withoutSubject(section.Lead, section.Title, topic.Name); lead != "" {
				write("> %s", lead)
			}
			switch {
			case section.Block != "" && len(section.Items) > 0:
				write("::%s%s", section.Block, optional(" ", section.BlockCaption))
				for _, item := range section.Items {
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
func unusedFrame(frame string, part int, previous string, used map[string]bool) (string, bool) {
	candidates := make([]string, 0, 4+len(defaultFrames))
	for attempt := 0; attempt <= 3; attempt++ {
		candidates = append(candidates, framePart(frame, part+attempt))
	}
	candidates = append(candidates, frameSituation, frameSequence, frameCase, frameOptions, frameRisk, frameOutcome)
	for _, candidate := range candidates {
		if candidate != previous && !used[candidate] {
			return candidate, true
		}
	}
	return candidates[0], false
}
