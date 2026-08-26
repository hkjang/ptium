package copilot

import (
	"errors"
	"strings"
	"testing"
)

// Three answers, and they used to be one. A sentence naming a slide the deck
// does not have was understood; so was one asking for what the deck already is.
// Telling their authors that nothing they said made sense sends them looking
// for better words rather than for the right number.
func TestASentenceUnderstoodIsNotACommandNotUnderstood(t *testing.T) {
	cases := []struct {
		said   string
		slides int
		says   string
	}{
		{"5장으로 줄여줘", 5, "이미 5장"},
		{"10분 발표에 맞춰줘", 5, "10분 분량"},
		{"20분 발표에 맞춰줘", 5, "줄일 것이 없습니다"},
		{"8장으로 줄여줘", 5, "늘리지는 않습니다"},
	}
	for _, one := range cases {
		_, err := Parse(one.said, one.slides)
		var nothing ErrNothingToDo
		if !errors.As(err, &nothing) {
			t.Errorf("%q on %d slides gave %v, want nothing to do", one.said, one.slides, err)
			continue
		}
		if !strings.Contains(nothing.Reason, one.says) {
			t.Errorf("%q on %d slides said %q, which does not mention %q",
				one.said, one.slides, nothing.Reason, one.says)
		}
	}
}

// A slide the deck does not have is a number to correct, not a sentence to
// rewrite.
func TestASlideTheDeckDoesNotHaveIsNamed(t *testing.T) {
	for _, said := range []string{"7번 슬라이드 삭제", "9번을 두 장으로 나눠줘", "7번과 9번 합쳐줘"} {
		_, err := Parse(said, 3)
		var beyond ErrOutOfRange
		if !errors.As(err, &beyond) {
			t.Errorf("%q on three slides gave %v, want out of range", said, err)
			continue
		}
		if beyond.Slides != 3 || beyond.Position < 4 {
			t.Errorf("%q reported %d of %d", said, beyond.Position, beyond.Slides)
		}
	}
}

// And what the parser really cannot read still says so.
func TestASentenceWithNoEditInItIsStillNotUnderstood(t *testing.T) {
	for _, said := range []string{"이거 예쁘게 해줘", "노트 채워줘", ""} {
		_, err := Parse(said, 5)
		var notUnderstood ErrNotUnderstood
		if !errors.As(err, &notUnderstood) {
			t.Errorf("%q gave %v, want not understood", said, err)
		}
	}
}

// The commands that do something still do it.
func TestTheCommandsThatDoSomethingStillDo(t *testing.T) {
	for _, one := range []struct {
		said   string
		slides int
		kind   string
	}{
		{"8장으로 줄여줘", 12, KindTrim},
		{"10분 발표에 맞춰줘", 8, KindTrim},
		{"2번과 3번 합쳐줘", 5, KindMerge},
		{"3번 삭제", 5, KindDelete},
	} {
		commands, err := Parse(one.said, one.slides)
		if err != nil || len(commands) == 0 || commands[0].Kind != one.kind {
			t.Errorf("%q on %d slides gave %v, %v", one.said, one.slides, commands, err)
		}
	}
}
