package pptx

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A heading is the one line everybody in the room reads. It is also the line a
// deck built from somebody's own words is most likely to get wrong: a brief
// says "…개선하려고 합니다" and the slide comes out headed "…개선하려고", which is
// half a sentence and not a title of anything.
//
// Six decks measured at 98 to 100 while carrying headings like that. Everything
// the measurement looked at — the drawing, the fit, the contrast — was right.
// Nothing looked at whether the words were words.

// unfinishedEnding matches a heading that stops in the middle of what it was
// saying: an intention with no verb, an instruction that leaked in, a modifier
// with nothing to modify, or a particle waiting for the rest of its clause.
var unfinishedEnding = regexp.MustCompile(
	`(?:하려고|으려고|려고|고자|하는데|하려|해서|어줘|해줘|줘|주세요|부탁해|부탁드립니다)$|` +
		// "…크게 줄여야", "…먼저 정해야": the heading says something must happen
		// and stops before saying it happens. Written out in full rather than as
		// a bare 야, because 분야 and 시야 are ordinary words that end a heading
		// perfectly well.
		`(?:어야|여야|아야|해야|되어야|돼야)$|` +
		`(?:^|\s)(?:하|되|만들|만드|작성|정리|준비)$|` +
		// A verb the reader cut before its ending. Each of these is a whole word
		// only when something was taken off it: "데이터 거버넌스 체계를 세우" was
		// the cover of a deck, cut out of "…세우려고 합니다".
		`(?:^|\s)(?:세우|세워|줄이|줄여|늘리|늘려|높이|높여|낮추|낮춰|바꾸|바꿔|맞추|맞춰|이루|이뤄)$|` +
		`(?:^|\s)(?:위한|위해|통한|통해|대한|관한)$`)

// strandedParticle is a heading whose last word is still holding the particle
// that joined it to words that are no longer there.
var strandedParticle = regexp.MustCompile(`[가-힣]{2,}(?:을|를|의|와|과|에게|에서|으로|로서)$`)

// latinFragment is the same in English: a heading cannot end on a word whose
// whole job is to introduce the next one.
var latinTail = map[string]bool{
	"of": true, "for": true, "the": true, "a": true, "an": true, "and": true,
	"to": true, "with": true, "in": true, "on": true, "by": true, "from": true,
	"about": true, "into": true, "over": true, "as": true, "at": true, "or": true,
}

// unfinishedHeadings reports a slide whose title is not a phrase.
func unfinishedHeadings(slide Slide) []Finding {
	var findings []Finding
	for _, paragraph := range slide.Fields[SlotTitle] {
		heading := strings.TrimSpace(paragraph.Text)
		if heading == "" || utf8.RuneCountInString(heading) < 3 {
			continue
		}
		if !unfinishedHeading(heading) {
			continue
		}
		findings = append(findings, Finding{Slot: SlotTitle, Kind: FindingUnfinished, Advisory: true,
			Detail: fmt.Sprintf("the heading %q stops in the middle of what it was saying", shortDetail(heading))})
		break
	}
	return findings
}

func unfinishedHeading(heading string) bool {
	trimmed := strings.TrimRight(strings.TrimSpace(heading), " .·—-")
	if trimmed == "" {
		return false
	}
	words := strings.Fields(trimmed)
	last := words[len(words)-1]
	// Written in lower case, because that is how a word doing that job is
	// written. "Q & A" ends on a capital A and is a slide everybody has seen.
	if latinTail[last] && last == strings.ToLower(last) && len(words) > 1 {
		return true
	}
	if unfinishedEnding.MatchString(trimmed) {
		return true
	}
	// A particle at the end is only a fragment when something came before it:
	// "매출을" alone is a slide about revenue, badly named but not cut off.
	return len(words) > 1 && strandedParticle.MatchString(last)
}

// headingSaidBefore reports a slide headed what an earlier slide is headed.
//
// A deck generated from a brief that asks for "비용과 일정" and "다음 단계" comes
// back with two slides headed "다음 단계", and everything the measurement looked
// at was right: both were drawn, both fitted, both had notes. A room reading the
// same heading twice cannot tell whether the deck went backwards, repeated
// itself, or has two different things to say under one name — and whoever made
// it never heard about it.
//
// Only the first repeat is reported, on the later slide: the earlier one is
// where the heading belongs until somebody decides otherwise.
func headingSaidBefore(slides []Slide, index int) []Finding {
	heading := headingOf(slides[index])
	if heading == "" || utf8.RuneCountInString(heading) < 2 {
		return nil
	}
	for earlier := 0; earlier < index; earlier++ {
		if !sameHeading(headingOf(slides[earlier]), heading) {
			continue
		}
		return []Finding{{Slot: SlotTitle, Kind: FindingTwiceTitled, Advisory: true,
			Detail: fmt.Sprintf("slide %d is headed %q as well", earlier+1, shortDetail(heading))}}
	}
	return nil
}

// headingOf is a slide's title as a room reads it: the words, without the
// spacing and punctuation that decide nothing.
func headingOf(slide Slide) string {
	var parts []string
	for _, paragraph := range slide.Fields[SlotTitle] {
		if text := strings.TrimSpace(paragraph.Text); text != "" {
			parts = append(parts, text)
		}
	}
	joined := strings.Join(parts, " ")
	joined = strings.TrimSpace(strings.Trim(strings.TrimSpace(joined), " .·—-:"))
	return strings.Join(strings.Fields(joined), " ")
}

// sameHeading is whether a room would read two headings as the same words.
//
// Korean spacing is not what a heading means: a deck came back with "기대 효과"
// on one slide and "기대효과" on another, which is the same section twice by any
// reading, and the measurement compared them character for character and said
// nothing.
func sameHeading(one, other string) bool {
	return strings.EqualFold(withoutSpaces(one), withoutSpaces(other))
}

func withoutSpaces(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value)
}
