package pptx

import (
	"fmt"
	"regexp"
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
	sentences := slideSentences(slide)
	// A slide listing two or more lines that open with a date is a schedule,
	// whatever its words are: "Q4 2024: PoC 수행 및 3개 DB 전환 완료" says when
	// something happens without ever saying it will happen. One such line is not
	// a schedule — a results slide may well head a figure with its year.
	scheduled := 0
	for _, sentence := range sentences {
		if datedEntry.MatchString(strings.TrimSpace(sentence)) {
			scheduled++
		}
	}
	var findings []Finding
	for _, sentence := range sentences {
		text := strings.TrimSpace(sentence)
		if text == "" || alreadyHappened.MatchString(text) {
			continue
		}
		if !plannedAhead.MatchString(text) && !(scheduled >= 2 && datedEntry.MatchString(text)) {
			continue
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
