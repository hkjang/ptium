// Package korean chooses the particle that follows a word.
//
// 을/를, 이/가, 은/는 and 와/과 are decided by whether the word in front of them
// ends in a consonant. In a message that word is usually something a person
// wrote — a heading lifted out of a file somebody uploaded, a template they
// named — so it cannot be settled when the sentence is written. One form
// written into the sentence is wrong about half the time, and nothing at
// runtime complains: "머리글"은 is a whole phrase either way.
//
// The workspace decides the same way, in web/src/korean.ts.
package korean

import "strings"

const (
	hangulFirst  = 0xac00
	hangulLast   = 0xd7a3
	hangulFinals = 28
)

// The digits, read aloud: 영 일 삼 육 칠 팔 close on a consonant, 이 사 오 구 do
// not. 영 ends on ㅇ, and a number written with a trailing zero is read 십 · 백 ·
// 천 · 만 · 억, every one of which closes on a consonant as well.
var digitCloses = map[rune]bool{
	'0': true, '1': true, '2': false, '3': true, '4': false,
	'5': false, '6': true, '7': true, '8': true, '9': false,
}

// The Latin letters read with a closing consonant: 엘, 엠, 엔.
var letterCloses = map[rune]bool{'l': true, 'm': true, 'n': true}

// closes reports whether the word ends on a consonant once read aloud, and
// whether that could be decided at all.
//
// A word is often quoted where these messages use it — %q wraps it — so the
// quotation marks are stepped over to reach the word itself.
func closes(word string) (bool, bool) {
	trimmed := strings.TrimRight(strings.TrimSpace(word), `"'”’」』])`)
	runes := []rune(strings.TrimSpace(trimmed))
	if len(runes) == 0 {
		return false, false
	}
	last := runes[len(runes)-1]
	if last >= hangulFirst && last <= hangulLast {
		return (last-hangulFirst)%hangulFinals != 0, true
	}
	if closed, ok := digitCloses[last]; ok {
		return closed, true
	}
	if closed, ok := letterCloses[unicodeLower(last)]; ok {
		return closed, true
	}
	if unicodeLower(last) >= 'a' && unicodeLower(last) <= 'z' {
		return false, true
	}
	return false, false
}

func unicodeLower(letter rune) rune {
	if letter >= 'A' && letter <= 'Z' {
		return letter + ('a' - 'A')
	}
	return letter
}

func pick(word, closed, open string) string {
	if ends, _ := closes(word); ends {
		return closed
	}
	return open
}

// Topic writes the 은/는 that marks what a sentence is about.
func Topic(word string) string { return pick(word, "은", "는") }

// Object writes the 을/를 that marks what is being acted on.
func Object(word string) string { return pick(word, "을", "를") }

// Subject writes the 이/가 that marks who is doing something.
func Subject(word string) string { return pick(word, "이", "가") }

// With writes the 와/과 that joins two things.
func With(word string) string { return pick(word, "과", "와") }
