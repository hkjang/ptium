package pptx

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A link written in a slide's text. The text is what the deck stores, drawn as
// `[보이는 말](주소)`, because a link that needed its own field would have to be
// carried by every hand the text passes through — the source language, the
// stored slide, the editor, the file — and the text is already carried by all
// of them.
//
// Only a target the drawing can honour makes a link: the web, an address, or
// another slide in the same deck. Anything else is left as the words somebody
// typed, so a footnote written [1](주석 3) stays a footnote rather than becoming
// a link to nowhere.

// TextRun is a stretch of a paragraph drawn as one run: plain words, or words
// that carry a link, or words the author marked.
type TextRun struct {
	Text string
	// Href is empty for plain words, a URL or mailto: address for a link that
	// leaves the deck, and "#3" for a jump to the deck's third slide.
	Href string
	// Bold and Italic are what the author marked in the line itself, on top of
	// whatever the template sets for the whole region.
	Bold   bool
	Italic bool
}

// SlideJump reads a link that points at another slide in the same deck and
// returns its 1-based number. The second result is false for every other link.
func SlideJump(href string) (int, bool) {
	if !strings.HasPrefix(href, "#") {
		return 0, false
	}
	number := 0
	for _, digit := range href[1:] {
		if digit < '0' || digit > '9' {
			return 0, false
		}
		number = number*10 + int(digit-'0')
		if number > 1000 {
			return 0, false
		}
	}
	if number < 1 {
		return 0, false
	}
	return number, true
}

// linkTarget says whether a target is one the deck can point at, and gives it
// back in the form the file and the browser both want.
func linkTarget(target string) (string, bool) {
	target = strings.TrimSpace(target)
	if target == "" || strings.ContainsAny(target, " \t\"'<>") {
		return "", false
	}
	if _, ok := SlideJump(target); ok {
		return target, true
	}
	lowered := strings.ToLower(target)
	for _, scheme := range []string{"https://", "http://", "mailto:"} {
		if strings.HasPrefix(lowered, scheme) && len(target) > len(scheme) {
			return target, true
		}
	}
	return "", false
}

// SplitLinks reads a paragraph's text as the runs it draws as. Text with no
// link in it comes back as a single run, which is what nearly every paragraph
// is: the caller pays nothing for the feature it is not using.
func SplitRuns(text string) []TextRun {
	if !strings.ContainsAny(text, "[*\\") {
		if text == "" {
			return nil
		}
		return []TextRun{{Text: text}}
	}
	return splitRuns(text, TextRun{})
}

// SplitLinks is what a paragraph draws as, kept under its first name for the
// callers that only care about the links in it.
func SplitLinks(text string) []TextRun { return SplitRuns(text) }

// splitRuns walks the text once, carrying what is true of the words it is in
// the middle of. A link's own label is walked the same way, which is what lets
// **[안내 문서](https://…)** be one bold link rather than three runs of markup.
func splitRuns(text string, carried TextRun) []TextRun {
	var runs []TextRun
	var plain strings.Builder
	flush := func() {
		if plain.Len() > 0 {
			run := carried
			run.Text = plain.String()
			runs = append(runs, run)
			plain.Reset()
		}
	}
	for index := 0; index < len(text); {
		// A mark somebody meant literally: \[ and \* draw as themselves.
		if text[index] == '\\' && index+1 < len(text) && (text[index+1] == '[' || text[index+1] == '*') {
			plain.WriteByte(text[index+1])
			index += 2
			continue
		}
		if text[index] == '[' {
			label, target, width, ok := readLink(text[index:])
			if ok {
				flush()
				linked := carried
				linked.Href = target
				runs = append(runs, splitRuns(label, linked)...)
				index += width
				continue
			}
		}
		if text[index] == '*' {
			before := rune(0)
			if index > 0 {
				before, _ = utf8.DecodeLastRuneInString(text[:index])
			}
			marked, inner, width, ok := readEmphasis(text[index:], before)
			if ok {
				flush()
				within := carried
				if marked == emphasisBold {
					within.Bold = true
				} else {
					within.Italic = true
				}
				runs = append(runs, splitRuns(inner, within)...)
				index += width
				continue
			}
		}
		plain.WriteByte(text[index])
		index++
	}
	flush()
	return runs
}

const (
	emphasisBold   = "**"
	emphasisItalic = "*"
)

// readEmphasis reads **굵게** or *기울임* from the front of text, given the
// character the mark is standing against. A star with no partner is a star: an
// author writing "3 * 4" or a footnote marker gets what they typed.
func readEmphasis(text string, against rune) (mark, inner string, width int, ok bool) {
	// A mark pressed against a letter or a digit is not a mark. "1200*800*750"
	// is how a size is written in a Korean deck — 가로*세로*높이 — and reading it
	// as emphasis drew 1200800750 on the slide. That is worse than markup on the
	// wall: markup anyone catches at a glance, and a number quietly changed into
	// a different number nobody does.
	//
	// Only the opening side is tested. Korean hangs its particles straight off
	// the closing mark — **중요**합니다 — and a rule that wanted air on both
	// sides would refuse the emphasis half this product's decks are written
	// with.
	if unicode.IsLetter(against) || unicode.IsDigit(against) {
		return "", "", 0, false
	}
	mark = emphasisItalic
	if strings.HasPrefix(text, emphasisBold) {
		mark = emphasisBold
	}
	rest := text[len(mark):]
	// Nothing between the marks is not emphasis, and neither is a mark that
	// opens on a space: "a * b * c" is arithmetic, not italics.
	if rest == "" || rest[0] == ' ' || rest[0] == '\n' {
		return "", "", 0, false
	}
	end := strings.Index(rest, mark)
	if end <= 0 {
		return "", "", 0, false
	}
	inner = rest[:end]
	if strings.Contains(inner, "\n") || strings.HasSuffix(inner, " ") {
		return "", "", 0, false
	}
	return mark, inner, len(mark)*2 + end, true
}

// readLink reads `[label](target)` from the front of text, and says how wide it
// was. A label that runs into another bracket, a newline, or the end of the
// text is not a link.
func readLink(text string) (label, target string, width int, ok bool) {
	end := strings.IndexAny(text[1:], "[]\n")
	if end < 0 || text[1+end] != ']' {
		return "", "", 0, false
	}
	label = text[1 : 1+end]
	rest := text[1+end+1:]
	if label == "" || !strings.HasPrefix(rest, "(") {
		return "", "", 0, false
	}
	shut := strings.IndexAny(rest[1:], ")\n")
	if shut < 0 || rest[1+shut] != ')' {
		return "", "", 0, false
	}
	target, ok = linkTarget(rest[1 : 1+shut])
	if !ok {
		return "", "", 0, false
	}
	return label, target, 1 + end + 1 + 1 + shut + 1, true
}

// PlainText is the words a paragraph draws, with the link markup taken out.
// Everything that measures a slide reads this rather than the stored text:
// measuring the markup would call a line too long that draws well within its
// region, and the deck would be repaired for a defect it does not have.
func PlainText(text string) string {
	runs := SplitLinks(text)
	if len(runs) == 1 && runs[0].Href == "" {
		return runs[0].Text
	}
	var builder strings.Builder
	for _, run := range runs {
		builder.WriteString(run.Text)
	}
	return builder.String()
}

// HasLink says whether a paragraph draws any link at all.
func HasLink(text string) bool {
	for _, run := range SplitLinks(text) {
		if run.Href != "" {
			return true
		}
	}
	return false
}

// linkTable collects the links one slide draws. A link is a relationship of the
// slide part, so the run can only name it by an id the package also writes: the
// table is what keeps the two in step.
//
// The ids are their own series rather than a continuation of rId1, rId2 …,
// because the picture and chart ids are counted from how many of each the slide
// has. A link numbered into that series would have to be counted the same way
// from a third place, and any drawing added later between them would silently
// take an id that a run already refers to.
type linkTable struct {
	links []slideLink
	byID  map[string]string
}

type slideLink struct {
	ID string
	// Target is the address for a link that leaves the deck, and empty for a
	// jump to another slide.
	Target string
	// Slide is the 1-based slide a jump points at, and 0 for every other link.
	Slide int
}

// id gives the relationship id for a target, adding it to the slide if this is
// the first run to use it: the same address linked from three runs is one
// relationship, the way PowerPoint writes it.
func (table *linkTable) id(target string) string {
	if table == nil {
		return ""
	}
	if id, ok := table.byID[target]; ok {
		return id
	}
	link := slideLink{ID: "rIdL" + strconv.Itoa(len(table.links)+1)}
	if number, ok := SlideJump(target); ok {
		link.Slide = number
	} else {
		link.Target = target
	}
	if table.byID == nil {
		table.byID = map[string]string{}
	}
	table.byID[target] = link.ID
	table.links = append(table.links, link)
	return link.ID
}

// asDrawn is the slide as it reaches the wall: the link markup taken out of
// every paragraph, leaving the words a reader sees.
//
// Measurement reads this rather than the stored slide. A line written
// [분기 보고서](https://reports.example.com/2026/q3) is twenty characters on the
// wall and sixty in the deck, so measuring the stored text would call it too
// long for its region, and the deck would be repaired — split across two slides,
// or its font stepped down — for a defect nobody can see.
func (slide Slide) asDrawn() Slide {
	linked := false
	for _, paragraphs := range slide.Fields {
		for _, paragraph := range paragraphs {
			if HasLink(paragraph.Text) {
				linked = true
				break
			}
		}
	}
	if !linked {
		return slide
	}
	fields := make(map[string][]Paragraph, len(slide.Fields))
	for slot, paragraphs := range slide.Fields {
		drawn := make([]Paragraph, len(paragraphs))
		copy(drawn, paragraphs)
		for index := range drawn {
			drawn[index].Text = PlainText(drawn[index].Text)
		}
		fields[slot] = drawn
	}
	slide.Fields = fields
	return slide
}

// RefusedLinks lists the targets in a paragraph that are written as links and
// are not ones the deck will follow.
//
// A line that says [문서](www.example.com) draws exactly those characters on the
// wall: the words, the brackets and the address. Nothing is broken, and nobody
// finds out until the slide is on a screen — which is what the measurement is
// for.
func RefusedLinks(text string) []string {
	if !strings.Contains(text, "](") {
		return nil
	}
	var refused []string
	for index := 0; index < len(text); {
		if text[index] == '\\' && index+1 < len(text) && text[index+1] == '[' {
			index += 2
			continue
		}
		if text[index] != '[' {
			index++
			continue
		}
		if _, _, width, ok := readLink(text[index:]); ok {
			index += width
			continue
		}
		target, width, ok := readRefusedLink(text[index:])
		if !ok {
			index++
			continue
		}
		refused = append(refused, target)
		index += width
	}
	return refused
}

// readRefusedLink reads the shape of a link — [something](something) — without
// asking whether the target is one the deck can point at.
func readRefusedLink(text string) (target string, width int, ok bool) {
	end := strings.IndexAny(text[1:], "[]\n")
	if end < 0 || text[1+end] != ']' || end == 0 {
		return "", 0, false
	}
	rest := text[1+end+1:]
	if !strings.HasPrefix(rest, "(") {
		return "", 0, false
	}
	shut := strings.IndexAny(rest[1:], ")\n")
	if shut < 0 || rest[1+shut] != ')' || shut == 0 {
		return "", 0, false
	}
	return rest[1 : 1+shut], 1 + end + 1 + 1 + shut + 1, true
}
