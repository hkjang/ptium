package pptx

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// now is the clock this package measures a plan against. It is a variable so a
// test can hold the date still.
var now = time.Now

// scheduleYear finds a year on a line, written either in full or in the two
// digit form a Korean deck uses — "2024년 4분기", "24년 4분기", "2024 Q4".
var scheduleYear = regexp.MustCompile(`(?i)\b(20\d{2})\b|\b(\d{2})년`)

// datedEntry is a line that opens with a date and then says what happens then —
// the shape every roadmap is written in, in any language.
var datedEntry = regexp.MustCompile(`(?i)^\s*(20\d{2}|\d{2}년|q[1-4]\s*20\d{2}|20\d{2}\s*q[1-4])[^:：—-]{0,12}[:：—-]`)

// plannedAhead are the words that make a line a plan rather than a record.
// A date on its own says nothing: "2024년 매출" is history a deck may well be
// about, and only a line that says something is going to happen can be late.
var plannedAhead = regexp.MustCompile(`(?i)시작|착수|예정|계획|추진|목표로|즉시|` +
	`kick[- ]?off|launch|begin|start|roll ?out|by then`)

// alreadyHappened are the endings that put a line in the past. Korean marks it
// on the verb — 했, 였, 었 — and a line that says something was done is a record
// however forward its other words read.
var alreadyHappened = regexp.MustCompile(`(?i)했|였|었|지난|완료된|기존|` +
	`completed|finished|last year|to date`)

// plannedLine is one line of a slide as a plan reads it, and whether it came
// from a component that is a schedule by construction.
//
// A step and its date are one line to anybody looking at the slide. Walked as
// the separate strings they are stored as, the words that make a line a plan
// sat in one string and the date in another, so a roadmap written with this
// product's own steps component could not be seen as a plan at all.
type plannedLine struct {
	text     string
	schedule bool
}

func plannedLines(slide Slide) []plannedLine {
	var lines []plannedLine
	slots := make([]string, 0, len(slide.Fields))
	for slot := range slide.Fields {
		slots = append(slots, slot)
	}
	sort.Strings(slots)
	for _, slot := range slots {
		for _, paragraph := range slide.Fields[slot] {
			lines = append(lines, plannedLine{text: paragraph.Text})
		}
	}
	blocks := make([]string, 0, len(slide.Blocks))
	for slot := range slide.Blocks {
		blocks = append(blocks, slot)
	}
	sort.Strings(blocks)
	for _, slot := range blocks {
		block := slide.Blocks[slot]
		schedule := block.Kind == BlockSteps || block.Kind == BlockTimeline
		for _, item := range block.Items {
			parts := make([]string, 0, 3)
			for _, part := range []string{item.Label, item.Value, item.Detail} {
				if strings.TrimSpace(part) != "" {
					parts = append(parts, strings.TrimSpace(part))
				}
			}
			if len(parts) > 0 {
				lines = append(lines, plannedLine{text: strings.Join(parts, " "), schedule: schedule})
			}
		}
		if strings.TrimSpace(block.Text) != "" {
			lines = append(lines, plannedLine{text: block.Text, schedule: schedule})
		}
	}
	return lines
}

// datedMonth reads a year and a month off a line: "2026년 3월", "2026-03", and
// the quarter a Korean plan is as likely to be written in, which is late once
// its last month is behind.
func datedMonth(text string) (int, int, bool) {
	if match := koreanMonth.FindStringSubmatch(text); match != nil {
		year, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		if month >= 1 && month <= 12 {
			return year, month, true
		}
	}
	if match := koreanQuarter.FindStringSubmatch(text); match != nil {
		year, _ := strconv.Atoi(match[1])
		quarter, _ := strconv.Atoi(match[2])
		return year, quarter * 3, true
	}
	return 0, 0, false
}

var koreanMonth = regexp.MustCompile(`(20\d{2})\s*년?\s*[-/.]?\s*(\d{1,2})\s*월`)
var koreanQuarter = regexp.MustCompile(`(20\d{2})\s*년?\s*([1-4])\s*분기`)

// saysItHappened reports whether a line is written in the past tense.
//
// Korean marks the past on the verb with 았/었/였, and it fuses into the
// syllable before the ending: 마치다 becomes 마쳤습니다, 끝내다 끝냈습니다,
// 되다 됐습니다. Naming the plain forms catches the verbs that do not contract
// and misses every one that does — a step reading "2026년 5월에 마쳤습니다" was
// read as a plan for May and called late.
//
// So what is read is the ㅆ the fusion leaves as the syllable's own final
// consonant. 있다 and 없다 carry that ㅆ without being past at all, and are the
// two this has to name.
func saysItHappened(text string) bool {
	if alreadyHappened.MatchString(text) {
		return true
	}
	for _, letter := range text {
		if letter < hangulFirst || letter > hangulLast || letter == '있' || letter == '없' {
			continue
		}
		if (letter-hangulFirst)%hangulFinals == finalDoubleSiot {
			return true
		}
	}
	return false
}

const (
	hangulFirst     = '\uAC00'
	hangulLast      = '\uD7A3'
	hangulFinals    = 28
	finalDoubleSiot = 20
)

// stalePlans reports a slide that schedules something for a date already past.
//
// A model writing a roadmap has no clock. Told the target is 2026 Q3 and told
// what today is, one still opened its plan with "2024년 4분기 PoC 시작을 위한
// 즉시 조치 필요" — a first step two years behind the room reading it. Nothing
// measured it, so the deck went out saying it.
//
// The rule is deliberately narrow. A date is only late when the line also says
// something is going to happen, and not when the line says it already did:
// a deck about last year's results is a deck about last year's results.
func stalePlans(slide Slide) []Finding {
	today := now()
	sentences := plannedLines(slide)
	// A slide listing two or more lines that open with a date is a schedule,
	// whatever its words are: "Q4 2024: PoC 수행 및 3개 DB 전환 완료" says when
	// something happens without ever saying it will happen. One such line is not
	// a schedule — a results slide may well head a figure with its year.
	scheduled := 0
	for _, sentence := range sentences {
		if sentence.schedule || datedEntry.MatchString(strings.TrimSpace(sentence.text)) {
			scheduled++
		}
	}
	var findings []Finding
	for _, sentence := range sentences {
		text := strings.TrimSpace(sentence.text)
		if text == "" || saysItHappened(text) {
			continue
		}
		if !sentence.schedule && !plannedAhead.MatchString(text) &&
			!(scheduled >= 2 && datedEntry.MatchString(text)) {
			continue
		}
		// A component whose whole purpose is a forward plan is read to the
		// month. A roadmap written in August that opens on March is behind the
		// room by five months, and its year says nothing is wrong.
		if sentence.schedule {
			if year, month, ok := datedMonth(text); ok &&
				(year < today.Year() || (year == today.Year() && month < int(today.Month()))) {
				findings = append(findings, Finding{Kind: FindingStale, Advisory: true,
					Detail: fmt.Sprintf("this plans something for %d-%02d, which is already past: %q",
						year, month, shorten(text, 60))})
				break
			}
		}
		match := scheduleYear.FindStringSubmatch(text)
		if match == nil {
			continue
		}
		year := 0
		if match[1] != "" {
			year, _ = strconv.Atoi(match[1])
		} else if match[2] != "" {
			two, _ := strconv.Atoi(match[2])
			// "24년" is this century, and a two digit year far ahead of today is
			// a quantity rather than a year: "12년 경력" is twelve years of it.
			year = 2000 + two
			if year > today.Year()+10 {
				continue
			}
		}
		if year == 0 || year >= today.Year() {
			continue
		}
		findings = append(findings, Finding{Kind: FindingStale, Advisory: true,
			Detail: fmt.Sprintf("this plans something for %d, which is already past: %q", year, shorten(text, 60))})
		break
	}
	return findings
}
