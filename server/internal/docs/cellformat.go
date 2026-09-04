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
// A clock is the same count: a time of day is the part of a day gone by, so an
// agenda reads "0.5625" where it should say 13:30, and a length of time is a
// count of whole days, so a day and a half of machine time reads "1.5" where
// the sheet says 36:00.
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

// numberKind is what a style says a number is, and how much of it the sheet
// shows: a format that spells seconds out is written with its seconds.
type numberKind struct {
	what    string // "", "date", "time", "datetime", "elapsed", "percent"
	seconds bool
}

// cellFormats says, for each style a cell can carry, what kind of number it is.
type cellFormats struct{ kinds []numberKind }

// The built-in formats a spreadsheet does not spell out. 14–17 are dates and 22
// is a date with a time on it; 18–21 are times of day; 45–47 are durations; 9
// and 10 are the percentages.
var builtInNumbers = map[string]numberKind{
	"9": {what: "percent"}, "10": {what: "percent"},
	"14": {what: "date"}, "15": {what: "date"}, "16": {what: "date"}, "17": {what: "date"},
	"18": {what: "time"}, "19": {what: "time", seconds: true},
	"20": {what: "time"}, "21": {what: "time", seconds: true},
	"22": {what: "datetime"},
	"45": {what: "elapsed", seconds: true}, "46": {what: "elapsed", seconds: true},
	"47": {what: "elapsed", seconds: true},
}

func readCellFormats(data []byte) cellFormats {
	var sheet styleSheet
	if len(data) == 0 || xml.Unmarshal(data, &sheet) != nil {
		return cellFormats{}
	}
	custom := map[string]string{}
	for _, one := range sheet.Formats {
		custom[one.ID] = one.Code
	}
	kinds := make([]numberKind, 0, len(sheet.Cells))
	for _, one := range sheet.Cells {
		kinds = append(kinds, kindOfFormat(one.FormatID, custom[one.FormatID]))
	}
	return cellFormats{kinds: kinds}
}

// kindOfFormat reads what the format is for.
func kindOfFormat(id, code string) numberKind {
	if built, ok := builtInNumbers[id]; ok {
		return built
	}
	if code == "" {
		return numberKind{}
	}
	// A format code spells the parts out: "yyyy-mm-dd", "0.0%", "h:mm". The
	// literal text inside quotation marks is not a part, so "0\"%\"" is not a
	// percentage, and neither is what stands in square brackets.
	spoken := spokenPartsOf(code)
	if strings.Contains(spoken, "%") {
		return numberKind{what: "percent"}
	}
	// An hour is written "h" and a second "s". A minute is written "m", the
	// same letter a month is, so it is never what makes a format a time — a
	// minute only ever stands beside an hour or a second anyway.
	day := strings.ContainsAny(spoken, "yYdD")
	clock := strings.ContainsAny(spoken, "hH")
	seconds := strings.ContainsAny(spoken, "sS")
	switch {
	// A unit in square brackets — "[h]:mm:ss", "[mm]:ss" — is what a sheet puts
	// on a length of time rather than a moment in one, so that 36 hours stays
	// 36 hours instead of turning midday on the second day.
	case (clock || seconds) && elapsedUnit(code):
		return numberKind{what: "elapsed", seconds: seconds}
	case day && (clock || seconds):
		return numberKind{what: "datetime", seconds: seconds}
	case day:
		return numberKind{what: "date"}
	case clock || seconds:
		return numberKind{what: "time", seconds: seconds}
	}
	return numberKind{}
}

// elapsedUnit says whether a format code carries a unit in square brackets,
// which is how a spreadsheet is told to let hours run past 24.
//
// Only the units are: what else stands in brackets is a colour, a condition, a
// locale or a currency sign, and none of those is spelt out of h, m and s.
func elapsedUnit(code string) bool {
	quoted := false
	for at := 0; at < len(code); at++ {
		switch {
		case code[at] == '"':
			quoted = !quoted
		case code[at] == '\\' && at+1 < len(code):
			at++
		case quoted:
		case code[at] == '[':
			end := strings.IndexByte(code[at:], ']')
			if end < 0 {
				return false
			}
			inside := code[at+1 : at+end]
			at += end
			if inside != "" && strings.Trim(strings.ToLower(inside), "hms") == "" {
				return true
			}
		}
	}
	return false
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
func (formats cellFormats) kind(style string) numberKind {
	if style == "" {
		return numberKind{}
	}
	at, err := strconv.Atoi(strings.TrimSpace(style))
	if err != nil || at < 0 || at >= len(formats.kinds) {
		return numberKind{}
	}
	return formats.kinds[at]
}

// The day a spreadsheet counts from. It is two days before 1900-01-01 because
// the format keeps a leap day in 1900 that never happened, and every reader
// agrees to keep it.
var spreadsheetEpoch = time.Date(1899, time.December, 30, 0, 0, 0, 0, time.UTC)

// The last day the count reaches, 9999-12-31, and with it the largest number
// any of these formats can be asked to write.
const lastCountedDay = 2958465

// written turns the stored number into what the sheet shows.
func (formats cellFormats) written(style, value string) (string, bool) {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return value, false
	}
	kind := formats.kind(style)
	switch kind.what {
	case "date":
		if number < 1 || number > lastCountedDay { // 1900-01-01 to 9999-12-31
			return value, false
		}
		day := spreadsheetEpoch.AddDate(0, 0, int(number))
		return day.Format("2006-01-02"), true
	case "datetime":
		if number < 1 || number > lastCountedDay {
			return value, false
		}
		moment := spreadsheetEpoch.Add(time.Duration(math.Round(number*86400)) * time.Second)
		if kind.seconds {
			return moment.Format("2006-01-02 15:04:05"), true
		}
		return moment.Format("2006-01-02 15:04"), true
	case "time":
		// A time of day is the part of the count that is not whole days: a
		// meeting at half past one is 0.5625, and the sheet shows "13:30".
		if number < 0 || number > lastCountedDay {
			return value, false
		}
		second := int(math.Round((number-math.Floor(number))*86400)) % 86400
		if kind.seconds {
			return fmt.Sprintf("%02d:%02d:%02d", second/3600, second/60%60, second%60), true
		}
		return fmt.Sprintf("%02d:%02d", second/3600, second/60%60), true
	case "elapsed":
		// A length of time is counted whole: a day and a half of machine time
		// is 36:00, not midday.
		if number < 0 || number > lastCountedDay {
			return value, false
		}
		second := int(math.Round(number * 86400))
		if kind.seconds {
			return fmt.Sprintf("%d:%02d:%02d", second/3600, second/60%60, second%60), true
		}
		return fmt.Sprintf("%d:%02d", second/3600, second/60%60), true
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
