package generation

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// writingRequest is everything both generation passes need to know.
type writingRequest struct {
	Presentation model.Presentation
	Profile      model.Profile
	Template     Template
	Plan         *deckPlan
}

// deckPlan is the narrative design produced by the first pass.
type deckPlan struct {
	DeckTitle string       `json:"deckTitle"`
	Thesis    string       `json:"thesis"`
	Slides    []planSlide  `json:"slides"`
	Sources   []planSource `json:"sources,omitempty"`
}

type planSlide struct {
	Role      string   `json:"role"`
	LayoutID  string   `json:"layoutId"`
	Headline  string   `json:"headline"`
	Intent    string   `json:"intent"`
	KeyPoints []string `json:"keyPoints"`
}

type planSource struct {
	Label string `json:"label"`
}

// writtenDeck is the finished copy produced by the second pass.
type writtenDeck struct {
	Slides []writtenSlide `json:"slides"`
}

type writtenSlide struct {
	LayoutID string                     `json:"layoutId"`
	Role     string                     `json:"role"`
	Title    string                     `json:"title"`
	Fields   map[string]json.RawMessage `json:"fields"`
	Blocks   map[string]pptx.Block      `json:"blocks"`
	Notes    string                     `json:"notes"`
}

const planSystemPrompt = `You are a presentation strategist at a top-tier consultancy.
Design the narrative arc of a deck before any copy is written.

Rules:
- Every slide must advance one argument. No filler, no repetition.
- Open by framing why this matters now; close with a decision or next step.
- Choose the layout whose role and slot capacity fit the point being made:
  a comparison layout for trade-offs, a two-content layout for parallel ideas,
  a quote layout for a single memorable statement, a section layout to change
  chapters. Do not force every slide into a bullet list.
- Return exactly the requested number of slides.

Respond with strict JSON only:
{"deckTitle":"","thesis":"","slides":[{"role":"title|section|content|twoContent|comparison|quote|picture|closing","layoutId":"","headline":"","intent":"","keyPoints":[""]}]}`

const writeSystemPrompt = `You are a senior presentation writer and information designer. Turn a deck plan
into finished slides in a specific PowerPoint template.

Hard constraints:
- Use only the layout ids given in the catalog, and only the slot names that
  layout lists. Never invent a slot.
- Respect each slot's maxChars and maxLines budget. Going over makes the text
  shrink and the slide look amateurish.
- Write in the requested language and tone. Never use markdown, asterisks or
  emoji.

Writing craft:
- Titles are assertions, not labels: "새 공급사 전환이 리스킬를 만든다", not "공급사 분석".
- Bullets are complete thoughts, six to fourteen words, parallel in structure,
  and lead with the point. Three to five per slide, never one.
- Use level 1 sub-bullets only for evidence supporting the bullet above.
- Speaker notes are two or three sentences of what to say out loud, not a
  repeat of the bullets.

Design craft â a body slot may hold a visual component instead of prose, and the
better deck usually does:
- kpi: two to four headline numbers. Items are {label, value, delta, trend}
  where trend is up|down|flat and decides the delta's colour. Use this for a
  handful of metrics; never a chart for three numbers.
- hero: the one number a slide is about, with a label and a line of context.
- meter: progress against a target. Items carry number as a percent, 0-100.
- columnChart: compare magnitude across up to six named categories. Items carry
  {label, number}. Set emphasis to the 1-based item the story is about and the
  rest recede to grey â that is how a chart makes a point.
- barChart: the same comparison when the category names are long.
- lineChart: a trend. series = [{name, points:[...]}], labels = the time axis.
  Two or three series at most.
- shareBar: part-to-whole across three to five parts. Items carry {label, number}.
- steps: a three-to-five stage process. Items carry {label, detail}.
- timeline: dated milestones. Items carry {value: the date, label, detail}.
- comparison: two or three options. Items carry {label, value, bullets:[...]}.
- table: columns plus rows when more than about seven classes carry meaning.
- quote: one memorable sentence in text, with attribute for its source.
- callout: one decision or statement that must not be missed.

Rules for components:
- Never invent a number. Use kpi, hero, meter, shareBar or a chart ONLY when the
  brief supplies the figures or they follow arithmetically from it. When the
  figures are unknown, choose steps, timeline, comparison, table, callout or
  prose â those carry structure without fabricating data.
- One component per slide, in a body slot. Prose bullets are still right for
  argument and nuance; do not force every slide into a graphic.
- Keep component labels short: two or three words, never a sentence.

Respond with strict JSON only:
{"slides":[{"layoutId":"","title":"","fields":{"slotName":["line","line"]},
"blocks":{"body":{"kind":"kpi","heading":"","unit":"%","emphasis":0,
"text":"","attribute":"","caption":"","labels":[""],"columns":[""],"rows":[[""]],
"items":[{"label":"","value":"","number":0,"delta":"","trend":"up","detail":"","bullets":[""]}],
"series":[{"name":"","points":[0]}]}},"notes":""}]}
Omit every key a component does not need. A field slot value may be a string, an
array of strings, or an array of {"text":"","level":0} objects where level 1
means a sub-bullet.`

func planUserPrompt(request writingRequest) string {
	var builder strings.Builder
	writeBrief(&builder, request)
	builder.WriteString("\nAvailable layouts in the customer's template:\n")
	builder.WriteString(request.Template.Manifest.SummaryFor(request.Presentation.Language, 0))
	fmt.Fprintf(&builder, "\nDesign exactly %d slides.\n", request.Presentation.RequestedSlideCount)
	return builder.String()
}

func writeUserPrompt(request writingRequest) string {
	var builder strings.Builder
	writeBrief(&builder, request)
	builder.WriteString("\nAvailable layouts in the customer's template:\n")
	builder.WriteString(request.Template.Manifest.SummaryFor(request.Presentation.Language, 0))
	if request.Plan != nil {
		builder.WriteString("\nApproved deck plan — follow it slide by slide:\n")
		if strings.TrimSpace(request.Plan.Thesis) != "" {
			fmt.Fprintf(&builder, "Thesis: %s\n", request.Plan.Thesis)
		}
		for index, slide := range request.Plan.Slides {
			fmt.Fprintf(&builder, "%d. role=%s layout=%s headline=%q intent=%q points=%s\n",
				index+1, slide.Role, slide.LayoutID, slide.Headline, slide.Intent, strings.Join(slide.KeyPoints, " / "))
		}
	}
	fmt.Fprintf(&builder, "\nAvailable components: %s\n", strings.Join(pptx.BlockKinds(), ", "))
	fmt.Fprintf(&builder, "This template can tell %d data series apart, so never plot more than that.\n",
		pptx.NewDesign(request.Template.Manifest).SeriesCap())
	fmt.Fprintf(&builder, "\nWrite exactly %d slides, in order.\n", request.Presentation.RequestedSlideCount)
	return builder.String()
}

func writeBrief(builder *strings.Builder, request writingRequest) {
	presentation := request.Presentation
	fmt.Fprintf(builder, "Deck title: %s\n", presentation.Title)
	if strings.TrimSpace(presentation.Prompt) != "" {
		fmt.Fprintf(builder, "Brief: %s\n", presentation.Prompt)
	}
	fmt.Fprintf(builder, "Language: %s\nAudience: %s\nTone: %s\nSlides: %d\n",
		presentation.Language, presentation.Audience, presentation.Tone, presentation.RequestedSlideCount)
	fmt.Fprintf(builder, "Template: %s (%s, %d layouts)\n",
		request.Template.Name, request.Template.Manifest.AspectRatio, len(request.Template.Manifest.Layouts))
	profile := request.Profile
	details := make([]string, 0, 3)
	if strings.TrimSpace(profile.Company) != "" {
		details = append(details, "company="+strings.TrimSpace(profile.Company))
	}
	if strings.TrimSpace(profile.JobTitle) != "" {
		details = append(details, "role="+strings.TrimSpace(profile.JobTitle))
	}
	if strings.TrimSpace(profile.Bio) != "" {
		details = append(details, "context="+truncate(strings.TrimSpace(profile.Bio), 400))
	}
	if len(details) > 0 {
		fmt.Fprintf(builder, "Presenter: %s\n", strings.Join(details, ", "))
	}
}
