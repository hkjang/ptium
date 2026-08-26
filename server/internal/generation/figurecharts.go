package generation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// A deck written from a brief full of numbers should draw them.
//
// Given a model, Ptium already does: told the components it has, a model writes
// ::line for a trend and ::share for a split. Without one — which is the
// deployment this product is built for — the same brief came back as bullet
// lists. Measured across the decks this workspace has generated: 461 decks with
// a chart in them were written by hand, three by generation.
//
// So the figures the prompt gave are read for the shapes a chart is for, and
// nothing else. Every row here is a number the brief stated; none is derived,
// scaled or invented, because a chart that invents its own data is worse than
// the sentence it replaced.

// figureChart is one component the brief's own numbers can fill.
type figureChart struct {
	Kind    string
	Caption string
	Rows    []string
}

// chartsFromFigures picks at most two charts for a deck: the shapes the numbers
// are actually in, best first.
func chartsFromFigures(figures []promptFigure, phrases languageCopy) []figureChart {
	readings := readFigures(figures)
	var charts []figureChart
	if chart, ok := shareChart(readings, phrases); ok {
		charts = append(charts, chart)
	}
	if chart, ok := seriesChart(readings, phrases); ok {
		charts = append(charts, chart)
	}
	if len(charts) == 0 {
		if chart, ok := meterChart(readings, phrases); ok {
			charts = append(charts, chart)
		}
	}
	if len(charts) > 2 {
		charts = charts[:2]
	}
	return charts
}

// reading is one figure the brief gave, as a number with the unit it was
// written in.
type reading struct {
	label  string
	value  string
	number float64
	unit   string
	// timely is a label that names a point in time — a year, a quarter, a month.
	timely bool
}

var (
	numberInValue = regexp.MustCompile(`-?[0-9][0-9,\.]*`)
	timeLabel     = regexp.MustCompile(`(^|\s)(19|20)[0-9]{2}\s*(년|年|$|\s)|[1-4]\s*(분기|Q)|Q[1-4]|(^|\s)[1-9][0-2]?\s*월`)
)

func readFigures(figures []promptFigure) []reading {
	readings := make([]reading, 0, len(figures))
	for _, figure := range figures {
		value := strings.TrimSpace(figure.Value)
		found := numberInValue.FindString(value)
		if found == "" {
			continue
		}
		number, err := strconv.ParseFloat(strings.ReplaceAll(found, ",", ""), 64)
		if err != nil {
			continue
		}
		label := strings.TrimSpace(figure.Label)
		if label == "" {
			continue
		}
		readings = append(readings, reading{
			label:  label,
			value:  value,
			number: number,
			unit:   strings.TrimSpace(strings.Replace(value, found, "", 1)),
			timely: timeLabel.MatchString(label),
		})
	}
	return readings
}

// sameUnit groups readings that can sit on one axis. A chart of "820억" beside
// "96%" is two different questions drawn as one answer.
func sameUnit(readings []reading, want func(reading) bool) map[string][]reading {
	groups := map[string][]reading{}
	for _, item := range readings {
		if want != nil && !want(item) {
			continue
		}
		groups[item.unit] = append(groups[item.unit], item)
	}
	return groups
}

// shareChart is parts of one whole: three to five percentages that add up to a
// hundred, give or take rounding.
func shareChart(readings []reading, phrases languageCopy) (figureChart, bool) {
	percentages := sameUnit(readings, func(item reading) bool { return item.unit == "%" })["%"]
	if len(percentages) < 3 || len(percentages) > 5 {
		return figureChart{}, false
	}
	total := 0.0
	for _, item := range percentages {
		if item.number <= 0 || item.number >= 100 {
			return figureChart{}, false
		}
		total += item.number
	}
	if total < 95 || total > 105 {
		return figureChart{}, false
	}
	return figureChart{Kind: "share", Caption: chartCaption(phrases, "share"), Rows: rowsOf(percentages)}, true
}

// seriesChart is the same measure at three or more points, drawn as magnitudes
// side by side.
//
// A line was tried first and is worse here: the component draws a line from
// values alone, so "2023년 · 2024년 · 2025년" has to become the series name and
// the years stop being labels on the chart. Columns keep every point named,
// which is what a reader of a business deck is looking for.
func seriesChart(readings []reading, phrases languageCopy) (figureChart, bool) {
	for _, group := range sameUnit(readings, func(item reading) bool { return item.unit != "%" }) {
		if len(group) < 3 || len(group) > 6 {
			continue
		}
		return figureChart{Kind: "columns", Caption: chartCaption(phrases, "columns"), Rows: rowsOf(group)}, true
	}
	return figureChart{}, false
}

// meterChart is progress against a target: percentages that do not add up to a
// whole and are not a split of one.
func meterChart(readings []reading, phrases languageCopy) (figureChart, bool) {
	percentages := sameUnit(readings, func(item reading) bool { return item.unit == "%" })["%"]
	if len(percentages) < 2 || len(percentages) > 5 {
		return figureChart{}, false
	}
	for _, item := range percentages {
		if item.number <= 0 || item.number > 100 {
			return figureChart{}, false
		}
	}
	return figureChart{Kind: "meter", Caption: chartCaption(phrases, "meter"), Rows: rowsOf(percentages)}, true
}

func rowsOf(readings []reading) []string {
	rows := make([]string, 0, len(readings))
	for _, item := range readings {
		rows = append(rows, fmt.Sprintf("%s | %s", chartLabel(item), item.value))
	}
	return rows
}

// chartLabel is what goes under the bar. A figure's label is the words around
// it in the brief — "매출은 2023년" — and on a chart of three years the subject
// belongs in the caption, not on the first bar and nowhere else.
func chartLabel(item reading) string {
	if item.timely {
		if found := timeLabel.FindString(item.label); strings.TrimSpace(found) != "" {
			return strings.TrimSpace(found)
		}
	}
	label := item.label
	// A figure's label is the words in front of it, which can reach back over a
	// full stop: "…실적 보고. 서울 1,200건" gives "보고. 서울".
	if cut := strings.LastIndexAny(label, ".。!?！？"); cut >= 0 && cut+1 < len(label) {
		label = strings.TrimSpace(label[cut+1:])
	}
	words := strings.Fields(label)
	// How much of a period the amount covers — 연, 월, 총 — is not what is
	// counted. Left on the end it becomes the entire label as soon as the
	// subject in front of it is dropped: a brief saying the Oracle licence
	// costs 4억 a year produced a headline reading "연 · 4억", which names
	// nothing at all.
	for len(words) > 1 && measureWord(words[len(words)-1]) {
		words = words[:len(words)-1]
	}
	// The subject of the sentence is what the chart's caption is for; the bar is
	// named by the thing counted. "비중은 직판" is one bar called 직판.
	for len(words) > 1 && endsWithParticle(words[0]) {
		words = words[1:]
	}
	// Two words are a name — "신규 고객". More than two are a phrase, and the bar
	// wants the thing at the end of it.
	if len(words) > 2 {
		words = words[len(words)-1:]
	}
	if len(words) == 0 {
		return item.label
	}
	// When the subject is the label — because nothing else was counted — it is
	// still carrying the marker that made it a subject: "오라클 라이선스가".
	words[len(words)-1] = withoutSubjectMarker(words[len(words)-1])
	return strings.Join(words, " ")
}

// measureWord reports whether a word says how the amount is measured rather
// than what is measured.
func measureWord(word string) bool {
	switch strings.Trim(word, " .,·") {
	case "연", "월", "일", "주", "분기", "반기", "연간", "월간", "주간", "매년", "매월",
		"총", "약", "평균", "누적", "최대", "최소", "합계":
		return true
	}
	return false
}

// withoutSubjectMarker takes the marker off a word that turned out to be the
// label. Only long enough words, and only the markers that are rarely a final
// syllable of the noun itself: 차이 must not become 차.
func withoutSubjectMarker(word string) string {
	if utf8.RuneCountInString(word) < 3 {
		return word
	}
	for _, marker := range []string{"은", "는", "이", "가"} {
		if strings.HasSuffix(word, marker) {
			stem := strings.TrimSuffix(word, marker)
			if utf8.RuneCountInString(stem) >= 2 {
				return stem
			}
		}
	}
	return word
}

// endsWithParticle reports whether a word is carrying a Korean subject or topic
// marker, which is what makes it the sentence's subject rather than a label.
func endsWithParticle(word string) bool {
	for _, particle := range []string{"은", "는", "이", "가", "의", "을", "를", "에서", "으로", "로"} {
		if strings.HasSuffix(word, particle) && len([]rune(word)) > len([]rune(particle)) {
			return true
		}
	}
	return false
}

func trimNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// chartCaption names what the chart is showing, in the deck's language.
func chartCaption(phrases languageCopy, kind string) string {
	captions := map[string]map[string]string{
		"ko": {"share": "구성비", "columns": "비교", "line": "추이", "meter": "목표 대비"},
		"ja": {"share": "構成比", "columns": "比較", "line": "推移", "meter": "目標比"},
		"zh": {"share": "构成比", "columns": "对比", "line": "走势", "meter": "对目标"},
		"en": {"share": "Share", "columns": "Comparison", "line": "Trend", "meter": "Against target"},
	}
	language := strings.ToLower(strings.TrimSpace(phrases.Language))
	if language == "" {
		language = "ko"
	}
	if set, ok := captions[language]; ok {
		return set[kind]
	}
	return captions["en"][kind]
}
