package pptx

import (
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
	case character == '%', character == '\u2030':
		// The percent sign is wide in both families this product meets — 0.89 in
		// the built-in face, 0.889 in Arial — and it was measured at 0.52 with
		// the rest of ASCII. It is in every share and every KPI a brief produces.
		return 0.85
	case character == '@', character == '#', character == '&', character == 'W':
		return 0.85
	case character >= '0' && character <= '9':
		return 0.55
	case character >= 'A' && character <= 'Z':
		return 0.66
	case character == 'm', character == 'w', character == 'M', character == 'W':
		return 0.85
	case character >= 0x1F000, character >= 0x20000 && character <= 0x2FA1F:
		// Emoji and the CJK characters past the basic plane draw full width.
		return 1.0
	case character == '\u2014', character == '\u2026', character == '\u203B':
		// The em dash, the ellipsis and ※. The first is in nearly every heading
		// this product writes — "배치 지연 — 기대 효과" — and it draws at 0.89 em in
		// the built-in face and 1.0 in Arial, against the 0.6 it used to be
		// measured at. Measuring a character narrower than it draws is how a line
		// keeps a size that does not fit and nothing reports it.
		return 0.95
	case character == '\u2013':
		return 0.7
	case character >= 0x2018 && character <= 0x201F:
		// Curly quotes are narrow: 0.3 to 0.45 drawn.
		return 0.45
	case character == '\u00B7', character == '\u00B0', character == '\u00B4', character == '\u2022':
		return 0.5
	case character >= 0x2020 && character <= 0x2BFF:
		// Arrows, geometric shapes, maths and the rest of the symbols a brief
		// uses: 0.89 in the built-in face.
		return 0.9
	case character < 0x80:
		// The rest of ASCII, which is mostly lowercase Latin: 0.556 in Arial and
		// 0.56 to 0.59 in the built-in face. At 0.52 a line of English measured
		// about four percent narrow, always in the direction that overflows.
		return 0.55
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

// TextEm is how wide a string draws, in multiples of the font size.
//
// Callers outside the drawing use it to judge whether a phrase is short: a
// count of characters says a Korean column name of nine is shorter than an
// English one of twenty-one, and on the page it is not.
func TextEm(text string) float64 {
	return measureEm(text)
}

// wrappedLines counts the lines a string needs at a given line width.
//
// Text breaks between words, so this walks the words the way a renderer does
// rather than dividing the total width by the line width. The difference is not
// small: four words of five ems each need four lines in a six-em column, and the
// dividing estimate says three — one line of overflow, every time.
func wrappedLines(text string, lineEm float64) int {
	if lineEm <= 0 {
		return 1
	}
	lines, used := 1, 0.0
	space := advanceEm(' ')
	for _, word := range strings.Fields(text) {
		width := measureEm(word)
		for width > lineEm {
			// A single word wider than the column is broken across lines.
			if used > 0 {
				lines++
				used = 0
			}
			lines++
			width -= lineEm
		}
		switch {
		case used == 0:
			used = width
		case used+space+width <= lineEm:
			used += space + width
		default:
			lines++
			used = width
		}
	}
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

// ReferenceAdvance is the width one character of the reference language takes,
// as a fraction of the type size. Callers outside the renderer need it to turn a
// region's capacity into a number of characters someone can write to.
const ReferenceAdvance = referenceAdvance

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

// wrapText splits a string into lines that fit a width given in em units,
// preferring word boundaries but breaking mid-word for CJK text that has none.
func wrapLines(value string, lineEm float64) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if lineEm < 1 {
		lineEm = 1
	}
	if measureEm(value) <= lineEm {
		return []string{value}
	}
	var lines []string
	current := make([]rune, 0, 32)
	width := 0.0
	lastSpace := -1
	for _, character := range value {
		current = append(current, character)
		width += advanceEm(character)
		if character == ' ' {
			lastSpace = len(current) - 1
		}
		if width < lineEm {
			continue
		}
		if lastSpace > 0 {
			lines = append(lines, strings.TrimSpace(string(current[:lastSpace])))
			current = append([]rune{}, current[lastSpace+1:]...)
		} else {
			lines = append(lines, strings.TrimSpace(string(current)))
			current = current[:0]
		}
		width = measureEm(string(current))
		lastSpace = -1
	}
	if remainder := strings.TrimSpace(string(current)); remainder != "" {
		lines = append(lines, remainder)
	}
	return lines
}

// orphanShare is how narrow a last line may be before it reads as an orphan: a
// title whose final line holds one syllable is the detail that makes a deck look
// generated rather than written.
const orphanShare = 0.3

// orphanedLine reports whether wrapping leaves a final line too short to belong,
// and by how much, measured in em.
func orphanedLine(text string, lineEm float64) (float64, bool) {
	if lineEm <= 1 {
		return 0, false
	}
	lines := wrapLines(text, lineEm)
	if len(lines) < 2 {
		return 0, false
	}
	last := measureEm(lines[len(lines)-1])
	if last >= lineEm*orphanShare {
		return 0, false
	}
	return last, true
}
