package docs

import (
	"strings"
	"unicode/utf8"
)

// The glyphs a list is written with when it was not written as markdown.
//
// A deck pasted out of Word or HWP arrives with •, and a PDF read back has one
// drawn on every point of every body region — the bullet is a character on the
// page there, not a property of a paragraph.
var listMarkers = map[rune]bool{
	'-': true, '*': true, '+': true,
	'•': true, '·': true, '‣': true, '▪': true, '▫': true, '◦': true, '⦁': true,
	'–': true, '—': true,
}

// A hyphen or a dash also begins a number: "-5% 감소" is a point about a fall,
// not an empty bullet in front of "5% 감소". Those have to be followed by a
// space to count as a marker; a bullet glyph never begins a word, so it does
// not.
var needsSpaceAfter = map[rune]bool{'-': true, '*': true, '+': true, '–': true, '—': true}

// withoutListMarker returns the line with its leading list marker removed, and
// whether there was one.
//
// The marker is a rune, not a byte. Cutting one byte off "• 매출" left the two
// bytes the bullet ends with in front of the words, which is not text at all:
// the deck reached the database as invalid UTF-8 and the import failed with
// nothing shown but a server error.
func withoutListMarker(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	first, size := utf8.DecodeRuneInString(trimmed)
	if size == 0 || !listMarkers[first] {
		return trimmed, false
	}
	rest := trimmed[size:]
	if needsSpaceAfter[first] && !strings.HasPrefix(rest, " ") && !strings.HasPrefix(rest, "\t") {
		return trimmed, false
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		// A marker with nothing after it is not a point; it is punctuation.
		return trimmed, false
	}
	return rest, true
}

// isListLine reports whether the line begins with a list marker.
func isListLine(line string) bool {
	_, found := withoutListMarker(line)
	return found
}
