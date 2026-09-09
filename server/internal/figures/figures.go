// Package figures reads the number a written value carries.
//
// Two parts of this program have to agree about that reading. A spreadsheet's
// importer decides whether a column is figures at all — a sheet of one label
// column and one figure column is drawn as a chart, anything else is laid out
// as a table — and the deck's parser then reads those very cells again to size
// the bars. Two readings of the same cell is the worst of both: the importer
// calls a column figures and hands the chart heights nobody wrote. So both
// call in here, and Alone is Of with one more question asked.
//
// A sheet almost never writes an amount bare. The Currency format puts a sign
// on every row — "₩1,200" in Korea, "$1,200" elsewhere, "1,200원" where the
// currency is a word — an export separates the thousands with a comma or with a
// space, the fixed one (U+00A0) included, and the accounting convention writes
// a negative amount in brackets, "(340)", which is what a refund row looks
// like. None of that is text a person typed into the cell; it is how the sheet
// shows the number, so none of it changes what the number is.
package figures

import (
	"strconv"
	"strings"
	"unicode"
)

// Of pulls the magnitude out of a written value: "18%", "42개", "1,200억",
// "-3.5pt", "₩1,200" and "(340)" all carry one. The figure is what comes before
// the unit, so "42개" is 42.
func Of(value string) (float64, bool) {
	number, ok, _ := read(value)
	return number, ok
}

// Alone reads a value that is a figure and nothing besides — nothing but the
// marks a sheet puts on one. A unit that is a word is something besides: "1월"
// is a month and "3개" is a count of things, and a column of those is the
// labels of a table rather than the heights of a chart.
//
// Whenever Alone finds a figure, Of finds the same one.
func Alone(value string) (float64, bool) {
	number, ok, alone := read(value)
	return number, ok && alone
}

// The marks a sheet puts on a figure without changing what the figure is: the
// currency signs, the word Korean spells its currency with, and the per-cent
// sign a column of percentages already carried.
const marks = "₩￦$€£¥￥원%"

// read scans a value once and answers both questions about it: which figure it
// carries, and whether that figure was all it carried.
func read(value string) (float64, bool, bool) {
	trimmed := strings.TrimSpace(value)
	// Brackets around the whole of it are how an accounting sheet writes a
	// minus: a refund is "(340)", never "-340".
	if len(trimmed) > 2 && strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")") {
		if number, ok, alone := read(trimmed[1 : len(trimmed)-1]); ok {
			return -number, true, alone
		}
		return 0, false, false
	}
	var digits strings.Builder
	seenDigit, alone := false, true
	for _, character := range trimmed {
		switch {
		case character >= '0' && character <= '9':
			digits.WriteRune(character)
			seenDigit = true
		case character == '.' && seenDigit:
			digits.WriteRune(character)
		case (character == '-' || character == '+') && digits.Len() == 0:
			digits.WriteRune(character)
		case character == ',' || unicode.IsSpace(character):
			// A thousands separator, or the gap an export leaves before a
			// unit. Taking a space for the end of the figure is what read
			// "1 200" as 1 while the importer was reading it as 1200.
		case strings.ContainsRune(marks, character):
			// A sign the sheet shows the figure with, not part of the figure.
		default:
			if seenDigit {
				// Stop at the first unit, so "42개" reads as 42.
				number, err := strconv.ParseFloat(digits.String(), 64)
				return number, err == nil, false
			}
			alone = false
		}
	}
	if !seenDigit {
		return 0, false, false
	}
	number, err := strconv.ParseFloat(digits.String(), 64)
	return number, err == nil, alone
}
