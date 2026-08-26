// Package copilot turns what someone types into edits to a deck.
//
// The point is not a chat window. It is the translation:
//
//	what was typed → an intent → editing commands → a changed deck
//
// Everything here is deterministic and runs with no model at all, which is what
// makes it usable in an air-gapped deployment and what makes it testable. A
// model has its own place in this product — rewriting a slide's words — and
// moving, merging, splitting and dropping slides is not it: those are edits
// with one right answer, and asking a model to guess at them would be slower
// and less reliable than reading the sentence.
package copilot

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Kind names what a command does.
const (
	KindDelete    = "delete"
	KindMove      = "move"
	KindDuplicate = "duplicate"
	KindMerge     = "merge"
	KindSplit     = "split"
	KindTrim      = "trim"
)

// Command is one edit, already resolved to slide positions.
type Command struct {
	Kind string `json:"kind"`
	// Slides are the 1-based positions the command acts on.
	Slides []int `json:"slides,omitempty"`
	// To is where a move puts the slide, 1-based.
	To int `json:"to,omitempty"`
	// Count is the deck length a trim aims at.
	Count int `json:"count,omitempty"`
	// Reason is what the parser understood, in the language it was typed in. It
	// is shown before anything is changed: a command nobody can check is a
	// command nobody should run.
	Reason string `json:"reason"`
}

// ErrNotUnderstood says the sentence named no edit this package performs.
type ErrNotUnderstood struct{ Text string }

func (e ErrNotUnderstood) Error() string {
	return fmt.Sprintf("no command in %q", e.Text)
}

// ErrNothingToDo says the sentence was understood and asks for what the deck
// already is. "이미 그렇습니다" is a different answer from "무슨 말인지
//모르겠습니다", and giving the second taught people the copilot was deaf: a
// five-slide deck asked to fit ten minutes is already five slides.
type ErrNothingToDo struct{ Reason string }

func (e ErrNothingToDo) Error() string { return e.Reason }

// ErrOutOfRange says the sentence named a slide the deck does not have.
type ErrOutOfRange struct {
	Position int
	Slides   int
}

func (e ErrOutOfRange) Error() string {
	return fmt.Sprintf("%d번 슬라이드가 없습니다. 이 덱은 %d장입니다", e.Position, e.Slides)
}

// A sentence names a verb and some slides. Reading it that way — rather than
// with one regular expression per phrasing — is what lets Korean and English
// share the parser: the two languages disagree about where the verb goes and
// about nothing else here.
var verbs = []struct {
	kind     string
	patterns []string
}{
	{KindMerge, []string{"합쳐", "합치", "병합", "묶어", "merge", "join"}},
	{KindSplit, []string{"나눠", "나누", "분리", "쪼개", "split"}},
	{KindDuplicate, []string{"복제", "복사", "duplicate", "copy"}},
	{KindMove, []string{"이동", "옮겨", "보내", "move"}},
	{KindTrim, []string{"줄여", "줄이", "맞춰", "자르", "cut", "trim", "reduce", "fit", "shorten"}},
	{KindDelete, []string{"삭제", "지워", "지우", "빼줘", "빼고", "없애", "delete", "remove", "drop"}},
}

// figureWithUnit reads a number and whatever unit follows it: "3번", "8장",
// "10분", "slide 4".
var figureWithUnit = regexp.MustCompile(`([0-9]+)\s*(번째|번|장|분|slides?|minutes?|min)?`)

// minutesPerSlide is how long one slide takes to present. Two minutes is the
// number every speaking coach uses and every rehearsal confirms: a slide with a
// point to make and a sentence to say does not go faster.
const minutesPerSlide = 2

type figure struct {
	value int
	unit  string
}

// Parse reads a sentence into commands against a deck of the given length.
func Parse(text string, slides int) ([]Command, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || slides <= 0 {
		return nil, ErrNotUnderstood{Text: text}
	}
	lowered := strings.ToLower(trimmed)
	kind := ""
	for _, verb := range verbs {
		for _, pattern := range verb.patterns {
			if strings.Contains(lowered, pattern) {
				kind = verb.kind
				break
			}
		}
		if kind != "" {
			break
		}
	}
	if kind == "" {
		return nil, ErrNotUnderstood{Text: text}
	}

	figures := make([]figure, 0, 4)
	for _, match := range figureWithUnit.FindAllStringSubmatch(lowered, -1) {
		value := number(match[1])
		if value <= 0 {
			continue
		}
		figures = append(figures, figure{value: value, unit: strings.TrimSpace(match[2])})
	}

	switch kind {
	case KindTrim:
		for _, entry := range figures {
			switch entry.unit {
			case "장", "slide", "slides":
				if entry.value <= 0 {
					continue
				}
				if entry.value < slides {
					return []Command{{Kind: KindTrim, Count: entry.value,
						Reason: fmt.Sprintf("%d장에서 %d장으로 줄입니다", slides, entry.value)}}, nil
				}
				if entry.value == slides {
					return nil, ErrNothingToDo{Reason: fmt.Sprintf("이 덱은 이미 %d장입니다", slides)}
				}
				return nil, ErrNothingToDo{Reason: fmt.Sprintf(
					"이 덱은 %d장입니다. 줄이는 것은 하지만 %d장으로 늘리지는 않습니다", slides, entry.value)}
			case "분", "minute", "minutes", "min":
				count := entry.value / minutesPerSlide
				if count <= 0 {
					continue
				}
				if count < slides {
					return []Command{{Kind: KindTrim, Count: count,
						Reason: fmt.Sprintf("%d분 발표에 맞춰 %d장에서 %d장으로 줄입니다 (한 장에 %d분)",
							entry.value, slides, count, minutesPerSlide)}}, nil
				}
				return nil, ErrNothingToDo{Reason: fmt.Sprintf(
					"이 덱은 %d장이라 %d분 분량입니다. %d분에 맞추려고 줄일 것이 없습니다 (한 장에 %d분)",
					slides, slides*minutesPerSlide, entry.value, minutesPerSlide)}
			}
		}
		return nil, ErrNotUnderstood{Text: text}
	case KindMerge, KindMove:
		positions := slidePositions(figures, slides)
		if len(positions) < 2 || positions[0] == positions[1] {
			if beyond, ok := beyondTheDeck(figures, slides); ok {
				return nil, ErrOutOfRange{Position: beyond, Slides: slides}
			}
			return nil, ErrNotUnderstood{Text: text}
		}
		if kind == KindMerge {
			first, second := positions[0], positions[1]
			if first > second {
				first, second = second, first
			}
			return []Command{{Kind: KindMerge, Slides: []int{first, second},
				Reason: fmt.Sprintf("%d번과 %d번을 한 장으로 합칩니다", first, second)}}, nil
		}
		return []Command{{Kind: KindMove, Slides: []int{positions[0]}, To: positions[1],
			Reason: fmt.Sprintf("%d번을 %d번 자리로 옮깁니다", positions[0], positions[1])}}, nil
	case KindDelete:
		positions := slidePositions(figures, slides)
		if len(positions) == 0 {
			if beyond, ok := beyondTheDeck(figures, slides); ok {
				return nil, ErrOutOfRange{Position: beyond, Slides: slides}
			}
			return nil, ErrNotUnderstood{Text: text}
		}
		sort.Ints(positions)
		return []Command{{Kind: KindDelete, Slides: positions,
			Reason: fmt.Sprintf("%s번 슬라이드를 지웁니다", join(positions))}}, nil
	default:
		positions := slidePositions(figures, slides)
		if len(positions) == 0 {
			if beyond, ok := beyondTheDeck(figures, slides); ok {
				return nil, ErrOutOfRange{Position: beyond, Slides: slides}
			}
			return nil, ErrNotUnderstood{Text: text}
		}
		reason := fmt.Sprintf("%d번을 두 장으로 나눕니다", positions[0])
		if kind == KindDuplicate {
			reason = fmt.Sprintf("%d번을 복제합니다", positions[0])
		}
		return []Command{{Kind: kind, Slides: []int{positions[0]}, Reason: reason}}, nil
	}
}

// slidePositions keeps the figures that name a slide, in the order they were
// typed. A figure with a unit of its own — "10분", "8장" — is not a position.
func slidePositions(figures []figure, slides int) []int {
	positions := make([]int, 0, len(figures))
	seen := map[int]bool{}
	for _, entry := range figures {
		switch entry.unit {
		case "", "번", "번째", "slide", "slides":
		default:
			continue
		}
		if valid(entry.value, slides) && !seen[entry.value] {
			seen[entry.value] = true
			positions = append(positions, entry.value)
		}
	}
	return positions
}

func valid(position, slides int) bool { return position >= 1 && position <= slides }

// beyondTheDeck is the first slide a sentence named that the deck does not
// have. "7번 슬라이드 삭제" on a three-slide deck is a sentence this understood
// perfectly, and answering it with "무슨 말인지 모르겠습니다" sends its author
// looking for better words rather than for the right number.
func beyondTheDeck(figures []figure, slides int) (int, bool) {
	for _, entry := range figures {
		switch entry.unit {
		case "", "번", "번째", "slide", "slides":
		default:
			continue
		}
		if entry.value > slides {
			return entry.value, true
		}
	}
	return 0, false
}

func number(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

func join(positions []int) string {
	parts := make([]string, 0, len(positions))
	for _, position := range positions {
		parts = append(parts, strconv.Itoa(position))
	}
	return strings.Join(parts, "·")
}
