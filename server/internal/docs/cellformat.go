package docs

import (
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// What a number in a spreadsheet means is in its format, not in the number.
//
// A date is stored as a count of days and a percentage as a fraction, so a
// schedule imported as it is written reads "45678" where it should say a day,
// and a split reads "0.68" where the sheet on screen says 68%. Both are the
// point of the cell; neither survives being read as a number.
//
// The rest of the formatting — fonts, borders, colours — is not what a deck is
// made of and is still passed over.

type styleSheet struct {
	Formats []struct {
		ID   string `xml:"numFmtId,attr"`
		Code string `xml:"formatCode,attr"`
	} `xml:"numFmts>numFmt"`
	Cells []struct {
		FormatID string `xml:"numFmtId,attr"`
	} `xml:"cellXfs>xf"`
}

// cellFormats says, for each style a cell can carry, what kind of number it is.
type cellFormats struct{ kinds []string }

// The built-in formats a spreadsheet does not spell out. 14–17 and 22 are the
// dates and date-times; 45–47 are durations, which are read as times of day and
// left alone; 9 and 10 are the percentages.
var builtInDates = map[string]bool{"14": true, "15": true, "16": true, "17": true, "22": true}
var builtInPercents = map[string]bool{"9": true, "10": true}

func readCellFormats(data []byte) cellFormats {
	var sheet styleSheet
	if len(data) == 0 || xml.Unmarshal(data, &sheet) != nil {
		return cellFormats{}
	}
	custom := map[string]string{}
	for _, one := range sheet.Formats {
		custom[one.ID] = one.Code
	}
	kinds := make([]string, 0, len(sheet.Cells))
	for _, one := range sheet.Cells {
		kinds = append(kinds, kindOfFormat(one.FormatID, custom[one.FormatID]))
	}
	return cellFormats{kinds: kinds}
}

// kindOfFormat reads what the format is for: "date", "percent" or "".
func kindOfFormat(id, code string) string {
	if builtInPercents[id] {
		return "percent"
	}
	if builtInDates[id] {
		return "date"
	}
	if code == "" {
		return ""
	}
	// A format code spells the parts out: "yyyy-mm-dd", "0.0%". The literal text
	// inside quotation marks is not a part, so "0\"%\"" is not a percentage, and
	// neither is what stands in square brackets.
	spoken := spokenPartsOf(code)
	if strings.Contains(spoken, "%") {
		return "percent"
	}
	if strings.ContainsAny(spoken, "yYdD") {
		return "date"
	}
	return ""
}

// spokenPartsOf keeps only the parts of a format code that say what shape the
// number has, dropping the parts that only say how to print it.
//
// Those are the text inside quotation marks, the character after a backslash,
// and every section in square brackets. A bracket holds a colour, a condition,
// a locale or a currency symbol — "[Red]", "[>=1000]", "[$-409]", "[$₩-412]" —
// and none of them makes the cell a date or a percentage. Read as if they did,
// "#,##0;[Red]-#,##0" — which is what the Currency dialog writes when negatives
// are shown in red, and one of the commonest formats on any sheet of money —
// has a "d" in it, and every amount on the sheet came back as a day in 1903.
func spokenPartsOf(code string) string {
	var out strings.Builder
	quoted := false
	for at := 0; at < len(code); at++ {
		switch {
		case code[at] == '"':
			quoted = !quoted
		case code[at] == '\\' && at+1 < len(code):
			at++
		case quoted:
		case code[at] == '[':
			// A bracket somebody never closed is not a bracket; keep it, so a
			// broken code is read the way the rest of it is written.
			end := strings.IndexByte(code[at:], ']')
			if end < 0 {
				out.WriteByte(code[at])
				continue
			}
			at += end
		default:
			out.WriteByte(code[at])
		}
	}
	return out.String()
}

// kind says what the style at this index is for.
func (formats cellFormats) kind(style string) string {
	if style == "" {
		return ""
	}
	at, err := strconv.Atoi(strings.TrimSpace(style))
	if err != nil || at < 0 || at >= len(formats.kinds) {
		return ""
	}
	return formats.kinds[at]
}

// The day a spreadsheet counts from. It is two days before 1900-01-01 because
// the format keeps a leap day in 1900 that never happened, and every reader
// agrees to keep it.
var spreadsheetEpoch = time.Date(1899, time.December, 30, 0, 0, 0, 0, time.UTC)

// written turns the stored number into what the sheet shows.
func (formats cellFormats) written(style, value string) (string, bool) {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return value, false
	}
	switch formats.kind(style) {
	case "date":
		if number < 1 || number > 2958465 { // 1900-01-01 to 9999-12-31
			return value, false
		}
		day := spreadsheetEpoch.AddDate(0, 0, int(number))
		return day.Format("2006-01-02"), true
	case "percent":
		return trimZeros(number*100) + "%", true
	}
	return value, false
}

// trimZeros writes a number the way somebody would: 68, not 68.000000.
func trimZeros(number float64) string {
	if number == math.Trunc(number) && math.Abs(number) < 1e15 {
		return strconv.FormatInt(int64(number), 10)
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", number), "0"), ".")
}
