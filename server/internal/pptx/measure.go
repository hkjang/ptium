package pptx

import (
	"math"
	"strings"
)

// Text fitting works in em units — multiples of the font size — because that
// makes it independent of the point size a template happens to use. A CJK
// glyph occupies a full em while Latin letters average about half of one, and
// ignoring that difference is what makes generated Korean slides overflow.

// advanceEm is the horizontal advance of one rune, in em units.
func advanceEm(character rune) float64 {
	switch {
	case character >= 0x1100 && character <= 0x11FF, // Hangul Jamo
		character >= 0x2E80 && character <= 0x303E, // CJK radicals, punctuation
		character >= 0x3041 && character <= 0x33FF, // Kana, compatibility
		character >= 0x3400 && character <= 0x4DBF, // CJK extension A
		character >= 0x4E00 && character <= 0x9FFF, // CJK unified
		character >= 0xA000 && character <= 0xA4CF, // Yi
		character >= 0xAC00 && character <= 0xD7A3, // Hangul syllables
		character >= 0xF900 && character <= 0xFAFF, // CJK compatibility
		character >= 0xFF01 && character <= 0xFF60, // Fullwidth forms
		character >= 0xFFE0 && character <= 0xFFE6:
		return 1.0
	case character == ' ':
		return 0.28
	case character == 'i', character == 'l', character == 'j', character == 't', character == 'f',
		character == 'I', character == '.', character == ',', character == ':', character == ';',
		character == '\'', character == '|', character == '!':
		return 0.29
	case character >= '0' && character <= '9':
		return 0.55
	case character >= 'A' && character <= 'Z':
		return 0.66
	case character == 'm', character == 'w', character == 'M', character == 'W':
		return 0.85
	case character < 0x80:
		return 0.52
	}
	return 0.6
}

// measureEm is the rendered width of a string in em units.
func measureEm(text string) float64 {
	total := 0.0
	for _, character := range text {
		total += advanceEm(character)
	}
	return total
}

// wrappedLines counts the lines a string needs at a given line width.
func wrappedLines(text string, lineEm float64) int {
	if lineEm <= 0 {
		return 1
	}
	lines := int(math.Ceil(measureEm(text) / lineEm))
	if lines < 1 {
		return 1
	}
	return lines
}

// LanguageAdvance is the average advance of one character in a language, used
// to convert a line width in em units into a character budget a writer can
// reason about.
func LanguageAdvance(language string) float64 {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "ko", "ko-kr", "ja", "ja-jp", "zh", "zh-cn", "zh-tw", "zh-hk":
		return 0.98
	case "":
		return 0.72
	}
	if index := strings.IndexAny(language, "-_"); index > 0 {
		return LanguageAdvance(language[:index])
	}
	return 0.55
}

// referenceAdvance is the mixed-script advance used for the language-agnostic
// character budget stored in a manifest.
const referenceAdvance = 0.72

// LineCount reports how many rendered lines a paragraph occupies inside a
// placeholder, accounting for its indent level.
func LineCount(text string, placeholder Placeholder, level int) int {
	lineEm := placeholder.LineEm
	if lineEm <= 0 {
		if placeholder.MaxChars <= 0 || placeholder.MaxLines <= 0 {
			return 1
		}
		lineEm = float64(placeholder.MaxChars) / float64(placeholder.MaxLines) * referenceAdvance
	}
	available := lineEm - float64(level)*2
	if available < 1 {
		available = 1
	}
	return wrappedLines(text, available)
}
