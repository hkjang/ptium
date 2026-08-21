package generation

import (
	"fmt"
	"strings"
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
			write("- %s", headingName(topic.Name))
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
	for index, topic := range outline.Topics {
		share := shares[index]
		for part := 0; part < share; part++ {
			// A second slide about the same topic argues a different aspect of it;
			// repeating the first one is worse than not having it.
			section := plan.Section(promptTopic{Name: topic.Name, Frame: framePart(topic.Frame, part)}, part, share)
			// A deck whose title is its only subject would open with that title
			// twice. The second one says which part of the subject the slide is.
			if strings.TrimSpace(section.Title) == strings.TrimSpace(plan.Title) {
				if aspect := frameTitleSuffix[plan.Language][framePart(topic.Frame, part)]; aspect != "" {
					section.Title = aspect
				}
			}
			write("# %s", section.Title)
			if section.Role != "" {
				write("@%s", section.Role)
			}
			if section.Lead != "" {
				write("> %s", section.Lead)
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
