package pptx

import "testing"

// An orphan is a word left behind, not a short last line.
//
// The rule says what it looks for — "a line holding one stray word or syllable"
// — and then fired on any last line under 30% of a line's width. A Korean
// heading that wraps to "검토 결과" and an English one that wraps to "cloud
// spend" are two words: a phrase, and how a heading of that length is meant to
// break. Nearly every deck measured came back advising its author to reword a
// title that reads perfectly well, which is how a measurement stops being read.
func TestAnOrphanIsAWordLeftBehindNotAShortLine(t *testing.T) {
	const lineEm = 14.0
	for _, want := range []struct {
		name   string
		text   string
		orphan bool
	}{
		{"a narrow phrase on the last line is still a wrap", "데이터품질개선방안수립계획 검토 안", false},
		{"two narrow English words are a phrase", "Migratingourdatawarehouse to it", false},
		{"one syllable left behind is an orphan", "데이터품질개선방안을정리하는 안", true},
		{"one short word left behind is an orphan", "Reducing our operating spend a", true},
		{"a single line has no orphan", "짧은 제목", false},
	} {
		_, orphan := orphanedLine(want.text, lineEm)
		if orphan != want.orphan {
			lines := wrapLines(want.text, lineEm)
			t.Errorf("%s: %q wrapped to %q — orphan=%v, want %v", want.name, want.text, lines, orphan, want.orphan)
		}
	}
}
