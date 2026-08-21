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

// sourceSystemPrompt asks for the deck in Ptium's slide language.
//
// A model writes this far more reliably than a nested JSON schema: there is one
// construct per line, nothing to balance, and a mistake costs one line rather
// than the whole response. It is also the same text a person edits, so what the
// model produced can be read, corrected and re-applied instead of being opaque.
const sourceSystemPrompt = `You are a senior presentation writer and information designer.
Write a deck in Ptium's slide language. Output the deck and nothing else: no
JSON, no markdown fences, no commentary.

The language, in full:
# Slide title            starts a slide
@cover                   slide kind: cover|section|content|two|comparison|quote|picture|closing
@layout <id>             use one exact layout from the catalog instead of a kind
> lead line              one line under the title
- point                  a bullet
  - evidence             a sub-bullet, indented two spaces
::kind caption           a component, ended by a line containing only ::
- label | value | detail one component row; omit any part
!notes ...               speaker notes, what to say out loud

Writing craft:
- Titles are assertions, not labels: "새 공급사 전환이 리스크를 만든다", not "공급사 분석".
- Bullets are complete thoughts, six to fourteen words, parallel, leading with the
  point. Three to five per slide, never one. Sub-bullets are evidence only.
- Never make the same point twice in different words. A slide with three real
  points is finished; a fourth line restating the first is padding, and it reads
  as generated. If there is nothing further to say, stop writing.
- Notes are two or three sentences of what to say, not a repeat of the bullets.
- Write in the requested language and tone. No markdown, asterisks or emoji.

Every slide begins with a line starting "# ". A component's rows go between its
::kind line and its closing :: line, never after. Here is a complete two-slide
deck, in full:

# 전환은 지금 결정해야 합니다
@cover
> 2026년 하반기 · 임원 보고
!notes 결론부터 말하고, 근거를 두 가지로 좁혀 설명합니다.

# 전환 대상과 규모
@content
> 42개 시스템을 세 묶음으로 나눴습니다.
::kpi 규모
- 전환 대상 | 42개
- 1차 범위 | 12개
- 예상 절감 | 18%
::
!notes 1차 범위만 승인받으면 나머지는 실적으로 설득합니다.

Components — a body region may hold one instead of prose, and the better deck
usually does:
- kpi: two to four headline numbers.            rows: label | value
- hero: the one number the slide is about.      rows: label | value | context
- meter: progress against a target.             rows: label | 72%
- columns: magnitude across up to six categories. rows: label | number
- bars: the same when category names are long.  rows: label | number
- line: a trend.                                rows: series name | v1, v2, v3
- share: part-to-whole across three to five parts. rows: label | number
- steps: a three-to-five stage process.         rows: stage | what happens
- timeline: dated milestones.                   rows: date | milestone | detail
- comparison: two or three options.             rows: option | headline | detail
- table: columns then rows.                     first row is the header
- quote: one memorable sentence.                rows: the sentence | source
- callout: one statement that must not be missed. rows: the statement

Rules for components:
- Never invent a number. Use kpi, hero, meter, share or a chart ONLY when the
  brief supplies the figures or they follow arithmetically from it. Otherwise use
  steps, timeline, comparison, table, callout or prose, which carry structure
  without fabricating data.
- One component per slide. Prose is still right for argument and nuance.
- Component labels are two or three words, never a sentence.
- In Korean, never put a space between a number and its unit: "7월", "2026년",
  "200%", not "7 월", "2026 년", "200 %".`

// planSystemPrompt's second pass used to be JSON; the plan itself stays JSON
// because it is consumed by the writer, not by a person.
func planUserPrompt(request writingRequest) string {
	var builder strings.Builder
	writeBrief(&builder, request)
	builder.WriteString("\nAvailable layouts in the customer's template:\n")
	builder.WriteString(request.Template.Manifest.SummaryFor(request.Presentation.Language, 0))
	fmt.Fprintf(&builder, "\nDesign exactly %d slides.\n", request.Presentation.RequestedSlideCount)
	return builder.String()
}

func sourceUserPrompt(request writingRequest) string {
	var builder strings.Builder
	writeBrief(&builder, request)
	builder.WriteString("\nAvailable layouts in the customer's template:\n")
	builder.WriteString(request.Template.Manifest.SummaryFor(request.Presentation.Language, 0))
	if request.Plan != nil {
		builder.WriteString("\nApproved deck plan — follow it slide by slide. The room line is what\n" +
			"that slide's layout holds; write to it and nothing has to be cut:\n")
		if strings.TrimSpace(request.Plan.Thesis) != "" {
			fmt.Fprintf(&builder, "Thesis: %s\n", request.Plan.Thesis)
		}
		for index, slide := range request.Plan.Slides {
			fmt.Fprintf(&builder, "%d. role=%s layout=%s headline=%q intent=%q points=%s\n",
				index+1, slide.Role, slide.LayoutID, slide.Headline, slide.Intent, strings.Join(slide.KeyPoints, " / "))
			if room := slideRoom(request.Template.Manifest, slide.LayoutID, request.Presentation.Language); room != "" {
				fmt.Fprintf(&builder, "   room: %s\n", room)
			}
		}
	}
	fmt.Fprintf(&builder, "This template can tell %d data series apart, so never plot more than that.\n",
		pptx.NewDesign(request.Template.Manifest).SeriesCap())
	fmt.Fprintf(&builder, "\nWrite exactly %d slides, in order, in the slide language.\n",
		request.Presentation.RequestedSlideCount)
	return builder.String()
}

// slideRoom states, in numbers, what one layout holds.
//
// A catalogue of every layout tells a model what exists; this tells it what the
// slide it is writing right now can take. The difference shows up as text that
// does not have to be cut afterwards.
func slideRoom(manifest pptx.Manifest, layoutID, language string) string {
	layout, ok := manifest.LayoutByReference(strings.TrimSpace(layoutID))
	if !ok {
		return ""
	}
	adjust := pptx.ReferenceAdvance / pptx.LanguageAdvance(language)
	parts := make([]string, 0, 4)
	componentLines := 0
	for _, placeholder := range layout.TextSlots() {
		budget := int(float64(placeholder.MaxChars) * adjust)
		switch placeholder.Slot {
		case pptx.SlotTitle:
			parts = append(parts, fmt.Sprintf("title ≤%d chars", max(budget/max(placeholder.MaxLines, 1), 1)))
		case pptx.SlotSubtitle:
			parts = append(parts, fmt.Sprintf("lead ≤%d chars", max(budget/max(placeholder.MaxLines, 1), 1)))
		default:
			if placeholder.MaxLines <= 1 {
				parts = append(parts, fmt.Sprintf("%s: one line ≤%d chars", placeholder.Slot, max(budget, 1)))
				continue
			}
			perLine := max(budget/max(placeholder.MaxLines, 1), 1)
			parts = append(parts, fmt.Sprintf("%s: %d lines × ~%d chars",
				placeholder.Slot, placeholder.MaxLines, perLine))
			componentLines = max(componentLines, placeholder.MaxLines)
		}
	}
	if componentLines >= 4 {
		parts = append(parts, "a component fits in the largest region")
	} else if componentLines > 0 {
		parts = append(parts, "too shallow for a component — write prose")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
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
