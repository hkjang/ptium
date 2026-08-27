package generation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// Revision asks for one slide to be written again.
//
// A generated slide is a draft, and a draft that cannot be argued with is a
// picture. This is how the editor asks for a different draft of one slide —
// shorter, sharper, as a component instead of prose — without regenerating the
// deck and losing everything else in it.
type Revision struct {
	Presentation model.Presentation
	Profile      model.Profile
	Template     Template
	// Source is the slide as it stands, in Ptium's slide language.
	Source string
	// Action names the standard rewrite. An unknown action is treated as a
	// free instruction, which is what Instruction is for anyway.
	Action string
	// Instruction is what the author typed, in their own words.
	Instruction string
	// Focus narrows the rewrite to one region of the slide.
	Focus string
	// Findings are what measuring the drawn slide reported: an overflow with its
	// size in centimetres is worth more to a rewrite than "make it shorter".
	Findings []string
	// DeckOutline lists the other slides, so a rewrite does not repeat the slide
	// before it or contradict the one after.
	DeckOutline []string
}

// Rewrite actions the editor offers.
const (
	ReviseRewrite   = "rewrite"
	ReviseShorten   = "shorten"
	ReviseExpand    = "expand"
	ReviseComponent = "component"
	ReviseNotes     = "notes"
	ReviseFit       = "fit"
)

const reviseSystemPrompt = `You are revising ONE slide of an existing deck, written in Ptium's
slide language. Return that one slide and nothing else: no JSON, no fences, no
commentary, no second slide.

Keep what the author already decided unless the instruction says otherwise: the
same layout directive, the same argument, the same language, the same voice.
Change what was asked and leave the rest alone. The slide must still begin with a
line starting "# ", and a component's rows must sit between its ::kind line and
its closing :: line.

Never invent a figure. If a number is not already on the slide or in the brief,
write the point without it.`

// ReviseSlide asks the model for another draft of one slide.
func (g *Generator) ReviseSlide(ctx context.Context, revision Revision) (string, error) {
	if strings.TrimSpace(revision.Source) == "" {
		return "", errors.New("the slide has no source to revise")
	}
	provider, baseURL, modelName, apiKey := "fallback", "https://api.openai.com/v1", "gpt-4.1-mini", ""
	_ = g.settings.Get(ctx, "ai.provider", &provider)
	_ = g.settings.Get(ctx, "ai.base_url", &baseURL)
	_ = g.settings.Get(ctx, "ai.model", &modelName)
	_ = g.settings.Get(ctx, "ai.api_key", &apiKey)
	if strings.EqualFold(provider, "fallback") {
		return "", ErrProviderUnavailable
	}
	if provider != "openai-compatible" && provider != "openai" {
		return "", fmt.Errorf("unsupported AI provider %q", provider)
	}
	endpoint, err := completionsEndpoint(baseURL)
	if err != nil {
		return "", err
	}
	// One slide, on this run's own settings: see forRun.
	g = g.forRun(ctx, 0)
	raw, err := g.completeSource(ctx, endpoint, modelName, apiKey,
		sourceSystemPrompt+exampleDeck(revision.Presentation.Language)+"\n\n"+reviseSystemPrompt,
		revisionPrompt(revision), 0.3)
	if err != nil {
		return "", err
	}
	source := cleanModelSource(raw, revision.Presentation.Language)
	if !strings.Contains(source, "#") {
		return "", errors.New("the AI provider did not return a slide")
	}
	// A rewrite invents citations exactly as readily as a first draft does: asked
	// to make one slide fit, the model returned it with "!source 내부 시스템 대장"
	// on the end, from a brief that mentions no such thing. The filter that
	// guards generation guards this too — nothing that reaches a slide should
	// pass through a door generation does not watch.
	source, _, _ = keepAttributedSources(source, revision.Presentation.Prompt+" "+
		revision.Presentation.Title)
	// A model asked for one slide sometimes returns the next one too. The first
	// is the one that was asked for.
	return strings.TrimSpace(trimSourceSlides(source, 1)), nil
}

// ErrProviderUnavailable says the deployment has no model to ask. The editor
// turns it into an explanation rather than an error page: an air-gapped
// deployment without a provider still edits decks by hand.
var ErrProviderUnavailable = errors.New("no AI provider is configured for this deployment")

func revisionPrompt(revision Revision) string {
	var builder strings.Builder
	presentation := revision.Presentation
	fmt.Fprintf(&builder, "Deck: %s\nLanguage: %s\nAudience: %s\nTone: %s\n",
		presentation.Title, presentation.Language, presentation.Audience, presentation.Tone)
	if strings.TrimSpace(presentation.Prompt) != "" {
		fmt.Fprintf(&builder, "Brief: %s\n", truncate(strings.TrimSpace(presentation.Prompt), 1200))
	}
	if len(revision.DeckOutline) > 0 {
		builder.WriteString("\nThe deck around this slide — do not repeat it:\n")
		for _, line := range revision.DeckOutline {
			fmt.Fprintf(&builder, "- %s\n", line)
		}
	}
	builder.WriteString("\nThe layout this slide is bound to, and what each region holds:\n")
	builder.WriteString(layoutCapacity(revision))
	builder.WriteString("\nThe slide as it stands:\n---\n")
	builder.WriteString(strings.TrimSpace(revision.Source))
	builder.WriteString("\n---\n\n")
	builder.WriteString(revisionTask(revision))
	if focus := strings.TrimSpace(revision.Focus); focus != "" {
		fmt.Fprintf(&builder, "Change only the %s region. Every other line comes back unchanged.\n", focus)
	}
	if len(revision.Findings) > 0 {
		builder.WriteString("\nMeasuring the drawn slide reported:\n")
		for _, finding := range revision.Findings {
			fmt.Fprintf(&builder, "- %s\n", finding)
		}
		builder.WriteString("Fix these by writing less, not by removing the point.\n")
	}
	if instruction := strings.TrimSpace(revision.Instruction); instruction != "" {
		fmt.Fprintf(&builder, "\nThe author asks: %s\n", truncate(instruction, 2000))
	}
	builder.WriteString("\nReturn the revised slide in the slide language, and nothing else.\n")
	return builder.String()
}

func revisionTask(revision Revision) string {
	switch revision.Action {
	case ReviseShorten:
		// The measurement that asks for this counts points, so the instruction
		// names the same number. "Shorten" alone came back as eleven shorter
		// lines, measured no better, and was rejected — a round trip spent on
		// nothing.
		return fmt.Sprintf("Task: this slide carries more than a room can take in. Cut it to at most %d "+
			"top-level points — fewer if it has less to say — by merging lines that make one point together "+
			"and dropping any that only restate another. Fewer words per line as well. Keep the argument the "+
			"slide actually makes, in the order it makes it; do not bring in a point from elsewhere in the deck.\n",
			pptx.MaximumPoints)
	case ReviseExpand:
		return "Task: this slide is thin. Add the evidence a reader would ask for, from the brief and from what the slide already says. Do not pad it with adjectives.\n"
	case ReviseComponent:
		return "Task: this slide's body should be a component rather than prose, if one fits what it says — steps for a process, timeline for dates, comparison for options, table for a matrix, callout for one statement. Use kpi, hero, meter, share or a chart only if the figures are already on the slide. The prose that became the component then goes: a region holds a component or bullets, never both, and bullets left behind are pushed into whatever smaller region remains. Keep at most one short lead line. If no component honestly fits, return the slide unchanged.\n"
	case ReviseNotes:
		return "Task: rewrite the !notes line only. Two or three sentences of what the presenter says out loud — the argument, the transition, the number to stress — never a repeat of the bullets.\n"
	case ReviseFit:
		return "Task: this slide does not fit its template. Make the text shorter so it fits, keeping every point.\n"
	}
	return "Task: write a better draft of this slide. Sharper title, tighter lines, the same argument.\n"
}

// layoutCapacity tells the model how much room each region of this slide's
// layout actually has, which is the difference between a rewrite that fits and
// one that has to be cut again by hand.
func layoutCapacity(revision Revision) string {
	layoutID := ""
	for _, line := range strings.Split(revision.Source, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "@layout ") {
			layoutID = strings.TrimSpace(strings.TrimPrefix(trimmed, "@layout"))
			break
		}
	}
	manifest := revision.Template.Manifest
	if layout, ok := manifest.Layout(layoutID); ok {
		single := manifest
		single.Layouts = []pptx.Layout{layout}
		return single.SummaryFor(revision.Presentation.Language, 1)
	}
	return manifest.SummaryFor(revision.Presentation.Language, 12)
}
