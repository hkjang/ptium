package deck

import (
	"fmt"
	"strings"

	"github.com/hkjang/ptium/server/internal/pptx"
)

// SourceFromImport writes a deck someone already had as deck source.
//
// The reader hands back the argument — titles, points, notes — and this turns it
// into the language the rest of the product is built on. From there the deck is
// an ordinary Ptium deck: it compiles into any template, it can be edited as
// words, and the model can be asked to rewrite a slide of it.
//
// What a slide carried besides words is reported rather than dropped silently. A
// photograph cannot be moved into another design at another aspect ratio and be
// trusted to look right, so the import says what it left behind.
func SourceFromImport(imported pptx.ImportedDeck) (string, []string) {
	return SourceFromImportWithImages(imported, nil)
}

// SourceFromImportWithImages is SourceFromImport with somewhere to put the
// pictures: store takes one and returns the name deck source should call it by.
// A picture it declines — because it is a logo repeated on every slide, or a
// decoration too small to be the point of the slide — is simply not placed.
func SourceFromImportWithImages(imported pptx.ImportedDeck, store func(pptx.ImportedPicture) (string, bool)) (string, []string) {
	var builder strings.Builder
	pictures, placed, tables, charts, plots := 0, 0, 0, 0, 0
	for index, slide := range imported.Slides {
		if index > 0 {
			builder.WriteString("\n")
		}
		title := strings.TrimSpace(slide.Title)
		if title == "" {
			title = fmt.Sprintf("%d번 슬라이드", index+1)
		}
		fmt.Fprintf(&builder, "# %s\n", escapeSourceLine(title))
		if name, ok := canonicalRoleName[strings.TrimSpace(slide.Role)]; ok {
			fmt.Fprintf(&builder, "@%s\n", name)
		}
		if lead := strings.TrimSpace(slide.Lead); lead != "" {
			fmt.Fprintf(&builder, "> %s\n", escapeSourceLine(lead))
		}
		for _, bullet := range slide.Bullets {
			fmt.Fprintf(&builder, "%s- %s\n", strings.Repeat("  ", bullet.Level), escapeSourceLine(bullet.Text))
		}
		// A table comes back as a table: the same grid, drawn by the design it
		// lands in rather than the one it came from.
		for _, table := range slide.Tables {
			builder.WriteString("::table\n")
			for _, row := range table {
				cells := make([]string, 0, len(row))
				for _, cell := range row {
					cells = append(cells, escapeItemField(cell))
				}
				fmt.Fprintf(&builder, "- %s\n", strings.Join(cells, " | "))
			}
			builder.WriteString("::\n")
			tables++
		}
		// A photograph goes into the region the new design keeps for one. Where it
		// sat in the old deck is not carried: coordinates chosen for one layout
		// mean nothing in another.
		for _, picture := range slide.Pictures {
			if store == nil {
				pictures++
				continue
			}
			name, ok := store(picture)
			if !ok {
				continue
			}
			fmt.Fprintf(&builder, "::image %s\n", escapeItemField(name))
			placed++
		}
		if notes := strings.TrimSpace(slide.Notes); notes != "" {
			fmt.Fprintf(&builder, "!notes %s\n", strings.ReplaceAll(notes, "\n", " "))
		}
		// A slide the author took out of the show stays out of it. Carrying it in
		// as an ordinary slide is not losing something: it is putting something
		// back in front of a room that somebody decided a room should not see.
		if slide.Hidden {
			builder.WriteString("!skip\n")
		}
		// A chart comes back as its numbers. The plot in the file was drawn from
		// figures, and those figures are what the slide is arguing; redrawn by the
		// design it lands in, they argue the same thing in the new deck's hand.
		for _, chart := range slide.Charts {
			if written := chartSource(chart); written != "" {
				builder.WriteString(written)
				plots++
			}
		}
		charts += slide.OtherCharts
	}
	var warnings []string
	if placed > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"그림 %d개를 이미지 라이브러리에 저장하고 슬라이드에 넣었습니다", placed))
	}
	if pictures > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"그림 %d개는 가져오지 않았습니다. 이미지 탭에서 올려 다시 넣어 주세요", pictures))
	}
	if tables > 0 {
		warnings = append(warnings, fmt.Sprintf("표 %d개를 이 덱의 디자인으로 다시 그렸습니다", tables))
	}
	if plots > 0 {
		warnings = append(warnings, fmt.Sprintf("차트 %d개를 숫자째 가져와 이 덱의 디자인으로 다시 그렸습니다", plots))
	}
	if charts > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"차트 %d개는 가져오지 않았습니다. 숫자를 ::bars 나 ::line 으로 적으면 다시 그려집니다", charts))
	}
	return builder.String(), warnings
}

// chartSource writes an imported chart as a component in deck source.
//
// A plot of one series is the component it looks like. A trend of several is a
// line chart, whose source names the axis first and then each series. Several
// series of columns are a grid of numbers — Ptium draws no grouped column — and
// a table says what they say without inventing a trend that was not claimed.
func chartSource(chart pptx.ImportedChart) string {
	if len(chart.Series) == 0 {
		return ""
	}
	var builder strings.Builder
	if chart.Kind != pptx.BlockLine && len(chart.Series) > 1 {
		if len(chart.Categories) == 0 {
			return ""
		}
		builder.WriteString("::table\n")
		header := append([]string{escapeItemField("구분")}, escapedFields(chart.Categories)...)
		fmt.Fprintf(&builder, "- %s\n", strings.Join(header, " | "))
		for index, series := range chart.Series {
			name := strings.TrimSpace(series.Name)
			if name == "" {
				name = fmt.Sprintf("계열 %d", index+1)
			}
			cells := []string{escapeItemField(name)}
			for point := range chart.Categories {
				value := ""
				if point < len(series.Points) {
					value = trimNumber(series.Points[point])
				}
				cells = append(cells, escapeItemField(value))
			}
			fmt.Fprintf(&builder, "- %s\n", strings.Join(cells, " | "))
		}
		builder.WriteString("::\n")
		return builder.String()
	}
	if chart.Kind == pptx.BlockLine {
		if len(chart.Categories) == 0 {
			return ""
		}
		builder.WriteString("::line\n")
		labels := make([]string, 0, len(chart.Categories))
		for index, category := range chart.Categories {
			if strings.TrimSpace(category) == "" {
				category = fmt.Sprintf("%d", index+1)
			}
			labels = append(labels, strings.ReplaceAll(category, ",", " "))
		}
		fmt.Fprintf(&builder, "- %s | %s\n", escapeItemField("기간"), escapeItemField(strings.Join(labels, ", ")))
		for index, series := range chart.Series {
			if len(series.Points) < 2 {
				continue
			}
			name := strings.TrimSpace(series.Name)
			if name == "" {
				name = fmt.Sprintf("계열 %d", index+1)
			}
			points := make([]string, 0, len(series.Points))
			for _, point := range series.Points {
				points = append(points, trimNumber(point))
			}
			fmt.Fprintf(&builder, "- %s | %s\n", escapeItemField(name), escapeItemField(strings.Join(points, ", ")))
		}
		builder.WriteString("::\n")
		return builder.String()
	}
	series := chart.Series[0]
	name := "columns"
	switch chart.Kind {
	case pptx.BlockBars:
		name = "bars"
	case pptx.BlockShare:
		name = "share"
	}
	fmt.Fprintf(&builder, "::%s\n", name)
	written := 0
	for index, point := range series.Points {
		label := ""
		if index < len(chart.Categories) {
			label = strings.TrimSpace(chart.Categories[index])
		}
		if label == "" {
			label = fmt.Sprintf("%d", index+1)
		}
		fmt.Fprintf(&builder, "- %s | %s\n", escapeItemField(label), escapeItemField(trimNumber(point)))
		written++
	}
	builder.WriteString("::\n")
	if written == 0 {
		return ""
	}
	return builder.String()
}

// escapedFields protects a row's cells from being read as more columns.
func escapedFields(values []string) []string {
	fields := make([]string, 0, len(values))
	for index, value := range values {
		if strings.TrimSpace(value) == "" {
			value = fmt.Sprintf("%d", index+1)
		}
		fields = append(fields, escapeItemField(value))
	}
	return fields
}
