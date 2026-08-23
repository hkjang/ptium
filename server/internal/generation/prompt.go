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
	// Material is a deck that already exists, being rewritten rather than
	// invented. Its facts are the author's and are not up for improvement; its
	// wording, its titles and the shape of its argument are what the model is
	// asked for.
	Material string
	// Today is the date the deck is being written on, as YYYY-MM-DD. A model has
	// no clock; without it, "the second half" is whichever year the model
	// remembers.
	Today string
	// Registered are the names of the slides this owner has already made and
	// agreed. The deck is asked to name one rather than write its own version,
	// and naming it is what lets the registered slide take its place.
	Registered []string
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
- A deck of seven slides or more opens with a contents page after the cover:
  role=content, the section names as its points, in the order they come. A
  shorter deck does not need one — it would be a slide spent saying what the next
  four slides say.
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
!source name | locator   where a figure on this slide came from

A second "> heading" after a slide's points starts the other column of a
two-region layout: heading, its points, second heading, its points.

The lead is one line, not the slide. Three things to say are three points,
each on its own "- " line; a lead holding them all separated by slashes is a
run-on sentence across the top of an empty slide.

Writing craft:
- Titles are assertions, not labels: "새 공급사 전환이 리스크를 만든다", not "공급사 분석".
- Bullets are complete thoughts, six to fourteen words, parallel, leading with the
  point. Three to five in one region, never one and never more than six: a
  region with seven points is read, not listened to. Sub-bullets are evidence
  only.
- Never make the same point twice in different words. A slide with three real
  points is finished; a fourth line restating the first is padding, and it reads
  as generated. If there is nothing further to say, stop writing.
- Never invent a figure. Every number on a slide — in a bullet, in a lead line,
  in the notes — comes from the brief or follows arithmetically from it. A target
  nobody set, a percentage nobody measured, a saving nobody counted: write the
  point without the number instead. The room asks about the number first.
- Notes are two or three sentences of what to say, not a repeat of the bullets.
- When the brief says where a figure came from — a report, a survey, a system,
  a date — put it on that slide with !source. It is printed at the foot of the
  slide and listed in the speaker notes, and it is the first thing a room asks
  about a number. Never invent one: a figure the brief states without a source
  gets no !source line.
- The part of !source after "|" is where in the source it is — a table, a page,
  a period the brief itself names. If the brief does not say where, write the
  name alone. A real system with a made-up month on it is worse than the name
  by itself, because the half that checks out is what makes the rest believed.
- Write in the requested language and tone. No markdown, asterisks or emoji.

Every slide begins with a line starting "# ". A component's rows go between its
::kind line and its closing :: line, never after.

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
- grid: a named grid this deployment ships.     ::grid <name> <caption>
        first row is the column header, then one row per item, each cell one of
        the definition's own values. The definitions available are listed below.

Rules for components:
- Never invent a number. Use kpi, hero, meter, share or a chart ONLY when the
  brief supplies the figures or they follow arithmetically from it. Otherwise use
  steps, timeline, comparison, table, callout or prose, which carry structure
  without fabricating data.
- One component per slide. Prose is still right for argument and nuance.
- Component labels are two or three words, never a sentence.
- In Korean, Japanese and Chinese, never put a space between a number and its
  unit: "7월", "2026년", "200%", "2026年8月", "8,400万円", "3時間12分" — not
  "7 월", "2026 년", "200 %", "2026 年 8 月", "8,400 万円", "3 時間 12 分".`

// planSystemPrompt's second pass used to be JSON; the plan itself stays JSON
// because it is consumed by the writer, not by a person.
// writeRegistered offers the deck the slides someone already made.
//
// The substitution that puts one into a deck matches by title, and a model left
// to itself titles the company introduction something of its own — so against a
// real model the slide library almost never fired. Told the names, the deck
// names one exactly, and the slide that was agreed takes the place of the
// model's version of it.
func writeRegistered(builder *strings.Builder, registered []string, brief string) {
	if len(registered) == 0 {
		return
	}
	builder.WriteString("\nSlides this company has already made and agreed. If the deck needs one,\n" +
		"write its title exactly as it appears here and leave that slide otherwise\n" +
		"empty — the agreed slide is put in its place, and anything you write on it\n" +
		"is thrown away. Do not name one the deck does not need:\n")
	for _, name := range registered {
		fmt.Fprintf(builder, "- %s\n", name)
	}
	// Naming the names was not enough on its own: asked for a deck about
	// "회사 소개 471936", the model wrote a slide called "Ptium 기업 개요" and the
	// agreed slide stayed on the shelf. Where the brief itself names one, it is
	// not a suggestion.
	if named := namedInBrief(registered, brief); len(named) > 0 {
		builder.WriteString("\nThe brief names these by name, so the deck must contain them, each with\n" +
			"exactly the title written above:\n")
		for _, name := range named {
			fmt.Fprintf(builder, "- %s\n", name)
		}
	}
}

// namedInBrief lists the registered slides the author asked for in so many
// words.
func namedInBrief(registered []string, brief string) []string {
	haystack := normalizeForMatch(brief)
	var named []string
	for _, name := range registered {
		if trimmed := normalizeForMatch(name); trimmed != "" && strings.Contains(haystack, trimmed) {
			named = append(named, name)
		}
	}
	return named
}

func planUserPrompt(request writingRequest) string {
	var builder strings.Builder
	writeBrief(&builder, request)
	writeRegistered(&builder, request.Registered, request.Presentation.Prompt+" "+request.Presentation.Title)
	builder.WriteString("\nAvailable layouts in the customer's template:\n")
	builder.WriteString(request.Template.Manifest.SummaryFor(request.Presentation.Language, 0))
	fmt.Fprintf(&builder, "\nDesign exactly %d slides.\n", request.Presentation.RequestedSlideCount)
	return builder.String()
}

func sourceUserPrompt(request writingRequest) string {
	var builder strings.Builder
	writeBrief(&builder, request)
	if material := strings.TrimSpace(request.Material); material != "" {
		builder.WriteString("\nThe deck as it stands today, in the slide language. " +
			"Rewrite it; do not start again:\n\n")
		builder.WriteString(material)
		builder.WriteString("\n")
	}
	writeRegistered(&builder, request.Registered, request.Presentation.Prompt+" "+request.Presentation.Title)
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
	builder.WriteString(gridGuide())
	if strings.TrimSpace(request.Material) != "" {
		builder.WriteString("\nReturn the whole deck, rewritten, in the slide language.\n")
		return builder.String()
	}
	fmt.Fprintf(&builder, "\nWrite exactly %d slides, in order, in the slide language.\n",
		request.Presentation.RequestedSlideCount)
	return builder.String()
}

// gridGuide lists the named grids this deployment draws, with the values each
// one accepts.
//
// Without it the model never writes one: a RACI chart, a readiness checklist
// and a likelihood-by-impact matrix are shapes every corporate deck uses, and
// Ptium draws all three — but a model that has not been told they exist writes
// them as bullets. Generated from the definitions themselves so the two cannot
// drift apart.
func gridGuide() string {
	grids := pptx.BuiltinGrids()
	if len(grids) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("\nNamed grids this deployment draws, for ::grid <name>:\n")
	for _, grid := range grids {
		values := make([]string, 0, len(grid.Order))
		for _, key := range grid.Order {
			values = append(values, key)
		}
		label := grid.Title
		if strings.TrimSpace(label) == "" {
			label = grid.Name
		}
		fmt.Fprintf(&builder, "- %s (%s): cells are one of %s\n", grid.Name, label, strings.Join(values, ", "))
	}
	builder.WriteString("A grid's first row is its column header; every later row is an item and its cells.\n")
	return builder.String()
}

// rewriteSystemPrompt is for a deck that already exists.
//
// The difference from writing one is entirely about what may change. The facts
// are the author's: a number invented to make a slide read better is worse than
// a slide that reads badly. What the model is asked for is the craft — the
// title that says what the slide argues, the sentence that is not the title
// again, the point that is a point rather than a paragraph.
// exampleDeck is the two-slide example the writing brief ends with, in the
// language the deck is being written in.
//
// It used to be Korean whatever was asked for, and a model writing English
// copied what it saw: an English deck came back with "::kpi 규모" as a component
// caption, and the example's own citation — "내부 시스템 대장 | 2026-03" — turned
// up as an invented source in decks and revisions that had nothing to do with
// it. An example is read as much as the rules are.
func exampleDeck(language string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "ko") || strings.TrimSpace(language) == "" {
		return koreanExample
	}
	return englishExample
}

const koreanExample = `
Here is a complete two-slide deck, in full:

# 전환은 지금 결정해야 합니다
@cover
> 2026년 하반기 · 임원 보고
!notes 결론부터 말하고, 근거를 두 가지로 좁혀 설명합니다.

# 전환 대상과 규모
@content
> 42개 시스템을 세 묶음으로 나눴습니다.
!source 내부 시스템 대장 | 2026-03
::kpi 규모
- 전환 대상 | 42개
- 1차 범위 | 12개
- 예상 절감 | 18%
::
!notes 1차 범위만 승인받으면 나머지는 실적으로 설득합니다.
`

const englishExample = `
Here is a complete two-slide deck, in full. Write yours in the requested
language, not in this one:

# The migration has to be decided now
@cover
> Second half of 2026 · board review
!notes Lead with the decision, then narrow the evidence to two points.

# What moves, and how much
@content
> Forty-two systems, sorted into three groups.
!source Internal systems register | 2026-03
::kpi Scope
- In scope | 42 systems
- First wave | 12
- Expected saving | 18%
::
!notes Approve the first wave and the rest is argued with results.
`

const rewriteSystemPrompt = `You are a senior presentation writer editing a deck someone already wrote.

The deck's facts are theirs. Every number, name, date and claim in it must appear
in your version, unchanged. Invent nothing: if a slide is thin, it stays thin.

What you change is the craft:
- A title that says what the slide argues, not what it is about.
- One lead sentence that adds to the title instead of repeating it.
- Points that are points: one idea each, no sentence fragments trailing off, no
  paragraph pretending to be a bullet.
- The order, where the argument is out of order — but keep every slide, and keep
  the cover as the cover and the closing as the closing.
- Speaker notes: keep what is there, and write one for every slide that has
  none. Every slide except the cover ends with a !notes line — a deck nobody can
  present from is half a deck.
- Components where they earn their place: figures the deck already states as a
  ::kpi row, a sequence it already describes as ::steps, a table it already has
  as ::table. Never a chart without numbers.

Write to the room each layout has. Output the deck in the slide language and
nothing else: no JSON, no fences, no commentary.`

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
	if today := strings.TrimSpace(request.Today); today != "" {
		// A model has no clock. Without this line a brief that says "second half"
		// or "next quarter" gets whatever year the model's training left it with,
		// and a deck written this week came back titled 2024.
		fmt.Fprintf(builder, "Today: %s. Any period the brief names without a year is this year's.\n", today)
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
