package pptx

import (
	"math"
	"strconv"
	"strings"
)

// chartPart describes a plotted block as a chart, for the exported file. The
// preview keeps drawing it, exactly as it does for tables, so the two never
// disagree about what is on the slide — but the file carries numbers someone
// can open and change.
//
// It returns nil whenever the drawing cannot be reproduced faithfully. A chart
// whose labels come out saying something other than what the preview showed is
// worse than a drawing, so the drawing wins every case this cannot match.
func (d Design) chartPart(frame Frame, block Block) *ChartPart {
	part := &ChartPart{
		Frame: frame, FontSize: d.Small, Font: d.Minor,
		LabelInk: d.InkPrimary, AxisInk: d.InkMuted, AxisLine: d.Line,
	}
	switch block.Kind {
	case BlockColumns, BlockBars:
		items := block.items()
		if len(items) == 0 {
			return nil
		}
		maximum := 0.0
		values := make([]float64, 0, len(items))
		displays := make([]string, 0, len(items))
		colors := make([]string, 0, len(items))
		for index, item := range items {
			maximum = math.Max(maximum, math.Abs(item.number()))
			values = append(values, item.number())
			displays = append(displays, item.Display(block.Unit))
			part.Categories = append(part.Categories, item.Label)
			fill := d.Accent
			if block.Emphasis > 0 {
				fill = d.DeEmphasis
				if block.Emphasis == index+1 {
					fill = d.Accent
				}
			}
			colors = append(colors, fill)
		}
		if maximum == 0 {
			return nil
		}
		format, ok := chartNumberFormat(displays, values)
		if !ok {
			return nil
		}
		part.Kind = chartColumn
		if block.Kind == BlockBars {
			part.Kind = chartBar
		}
		part.FormatCode = format
		// What the numbers are. The source writes it as the component's caption
		// and a stored block may carry it as a heading; either is the series'
		// name, and "값" is only what to say when the deck never said.
		name := strings.TrimSpace(block.Heading)
		if name == "" {
			name = strings.TrimSpace(block.Caption)
		}
		if name == "" {
			name = "값"
		}
		series := ChartSeries{Name: name, Values: values, Color: d.Accent, PointColors: colors}
		if block.Kind == BlockColumns {
			// The drawing labels every column of a short chart, and only the
			// emphasised one when there are too many to label without collisions.
			series.LabelPoints = columnLabelPoints(len(items), block.Emphasis)
		}
		part.Series = []ChartSeries{series}
	case BlockLine:
		series := make([]Series, 0, len(block.Series))
		for _, candidate := range block.Series {
			if len(candidate.Points) >= 2 {
				series = append(series, candidate)
			}
		}
		if len(series) == 0 {
			return nil
		}
		if len(series) > d.SeriesCap() {
			series = series[:d.SeriesCap()]
		}
		longest := 0
		for _, candidate := range series {
			longest = max(longest, len(candidate.Points))
		}
		if longest == 0 {
			return nil
		}
		displays := make([]string, 0, len(series))
		values := make([]float64, 0, len(series))
		for _, candidate := range series {
			if len(candidate.Points) == 0 {
				return nil
			}
			last := candidate.Points[len(candidate.Points)-1]
			displays = append(displays, formatNumber(last)+block.Unit)
			values = append(values, last)
		}
		format, ok := chartNumberFormat(displays, values)
		if !ok {
			return nil
		}
		for index := 0; index < longest; index++ {
			label := ""
			if index < len(block.Labels) {
				label = block.Labels[index]
			}
			part.Categories = append(part.Categories, label)
		}
		part.Kind = chartLine
		part.FormatCode = format
		part.Legend = len(series) > 1
		for index, candidate := range series {
			name := strings.TrimSpace(candidate.Name)
			if name == "" && len(series) > 1 {
				name = "계열 " + strconv.Itoa(index+1)
			}
			part.Series = append(part.Series, ChartSeries{Name: name, Values: candidate.Points,
				Color: d.Series(index), LabelPoints: []int{len(candidate.Points) - 1}})
		}
	default:
		return nil
	}
	if len(part.Series) == 0 || len(part.Categories) == 0 {
		return nil
	}
	return part
}

// chartNumberFormat finds the spreadsheet number format that reproduces what
// the drawing writes on the marks, and reports whether one exists. "1,240억원"
// is a grouped integer with a unit; "9.8%" is one decimal with a unit; a set
// that mixes the two, or that labels its marks with words, has no format, and
// the caller keeps the drawing.
func chartNumberFormat(displays []string, values []float64) (string, bool) {
	if len(displays) == 0 || len(displays) != len(values) {
		return "", false
	}
	suffix, decimals, grouped, known := "", 0, false, false
	numbers := make([]string, 0, len(displays))
	for _, display := range displays {
		trimmed := strings.TrimSpace(display)
		cut := 0
		for cut < len(trimmed) && strings.ContainsRune("0123456789,.-+", rune(trimmed[cut])) {
			cut++
		}
		number := trimmed[:cut]
		if strings.Trim(number, ",.-+") == "" {
			return "", false
		}
		rest := strings.TrimSpace(trimmed[cut:])
		if known && rest != suffix {
			return "", false
		}
		suffix, known = rest, true
		if strings.Contains(number, ",") {
			grouped = true
		}
		if dot := strings.Index(number, "."); dot >= 0 {
			decimals = max(decimals, len(number)-dot-1)
		}
		numbers = append(numbers, number)
	}
	if decimals > 2 {
		return "", false
	}
	for index, number := range numbers {
		if chartValueText(values[index], decimals, grouped) != number {
			return "", false
		}
	}
	pattern := "0"
	if grouped {
		pattern = "#,##0"
	}
	if decimals > 0 {
		pattern += "." + strings.Repeat("#", decimals)
	}
	if suffix != "" {
		// A unit inside a format code is quoted, and the quotes are the reason a
		// label reads "12%" rather than the number alone.
		pattern += `"` + strings.ReplaceAll(suffix, `"`, "") + `"`
	}
	return pattern, true
}

// chartValueText writes a number the way the format code would, so the two can
// be compared before the format is trusted.
func chartValueText(value float64, decimals int, grouped bool) string {
	text := trimZero(value, decimals)
	if !grouped {
		return text
	}
	sign := ""
	if strings.HasPrefix(text, "-") {
		sign, text = "-", text[1:]
	}
	whole, fraction := text, ""
	if dot := strings.Index(text, "."); dot >= 0 {
		whole, fraction = text[:dot], text[dot:]
	}
	var builder strings.Builder
	for index, digit := range whole {
		if index > 0 && (len(whole)-index)%3 == 0 {
			builder.WriteByte(',')
		}
		builder.WriteRune(digit)
	}
	return sign + builder.String() + fraction
}

// columnLabelPoints repeats layoutColumns' own rule about which columns carry
// their value.
func columnLabelPoints(count, emphasis int) []int {
	if count <= 6 {
		return nil
	}
	points := make([]int, 0, 1)
	if emphasis > 0 && emphasis <= count {
		points = append(points, emphasis-1)
	}
	return points
}
