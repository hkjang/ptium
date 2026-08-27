package deck

import (
	"fmt"
	"strconv"
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
	// Which slides carried nothing to read, by their place in the deck. A count
	// on its own could not be said without being misread: "%d장에는" is how a
	// reader is told which slide, so one wordless slide at the fourth place
	// announced itself as the first.
	var wordless []int
	slides, headers := withoutRunningHeaders(imported.Slides)
	imported.Slides = withRolesThatFitWhatTheyHold(slides)
	for index, slide := range imported.Slides {
		if wordlessSlide(slide) {
			wordless = append(wordless, index+1)
		}
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
		// A citation is written where the deck keeps citations, not as a point
		// that says "출처: …".
		for _, cited := range slide.Sources {
			if cited = strings.TrimSpace(cited); cited != "" {
				fmt.Fprintf(&builder, "!source %s\n", escapeSourceLine(cited))
			}
		}
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
	for _, header := range headers {
		warnings = append(warnings, fmt.Sprintf(
			"슬라이드마다 반복되던 머리글 %q은 가져오지 않고, 각 장의 제목을 그 아래에서 찾았습니다", header))
	}
	// A deck exported as pictures — one image filling each slide, which is what
	// several of the tools people generate decks with produce — reads as a run
	// of slides called "3번 슬라이드" with nothing on them. That is what the file
	// holds, and the import is not going to invent the words; but coming back to
	// a deck of empty slides with no explanation reads as the import failing.
	switch {
	case len(wordless) > 0 && len(wordless) == len(imported.Slides):
		warnings = append(warnings, fmt.Sprintf(
			"슬라이드 %d장 모두에 읽을 수 있는 글자가 없습니다. 슬라이드가 그림 한 장으로 된 파일이라 "+
				"제목을 임시로 붙였습니다", len(wordless)))
	case len(wordless) > 0:
		warnings = append(warnings, fmt.Sprintf(
			"%s에는 읽을 수 있는 글자가 없어 제목을 임시로 붙였습니다", slidesNamed(wordless)))
	}
	if placed > 0 {
		// What this step knows is how many pictures it carried out of the file
		// and into the deck's source. Whether the design can draw them is the
		// next step's to say — and claiming "슬라이드에 넣었습니다" here told the
		// owner of a file with twenty-two pictures that all twenty-two were on
		// slides, when the design had one picture region per layout and ten of
		// them never arrived.
		warnings = append(warnings, fmt.Sprintf(
			"그림 %d개를 이미지 라이브러리에 저장했습니다", placed))
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

// withRolesThatFitWhatTheyHold keeps a slide's kind honest.
//
// A cover and a closing hold a name and a line or two under it. Both are read
// from the layout a slide was drawn on, which says nothing about what somebody
// then put on it: the last slide of one real deck is twelve points of what the
// product could go on to do, and it was called a closing — twelve lines of text
// into room for three.
//
// A deck's kind of slide is read from the layout it was built on, and a weekly
// report drawn on the title layout every week says every one of its slides is a
// cover. A cover has room for a name and a line under it — so the whole of a
// slide's argument, a table of what was done and what is planned, went into the
// subtitle: nine lines of text in room for two, on every slide of the deck.
//
// What those slides hold decides what they are, so the rest become content.
func withRolesThatFitWhatTheyHold(slides []pptx.ImportedSlide) []pptx.ImportedSlide {
	slides = withOneCover(slides)
	result := make([]pptx.ImportedSlide, len(slides))
	copy(result, slides)
	for index, slide := range result {
		switch slide.Role {
		case pptx.RoleTitle, pptx.RoleClosing:
			if len(slide.Tables) > 0 || len(slide.Charts) > 0 || len(slide.Bullets) > roomOnAnEndPage {
				result[index].Role = pptx.RoleContent
			}
		}
	}
	return result
}

// roomOnAnEndPage is how many lines a cover or a closing is drawn to hold.
const roomOnAnEndPage = 3

// withOneCover leaves the cover to the first slide.
func withOneCover(slides []pptx.ImportedSlide) []pptx.ImportedSlide {
	covers := 0
	for _, slide := range slides {
		if slide.Role == pptx.RoleTitle {
			covers++
		}
	}
	if covers < 2 {
		return slides
	}
	result := make([]pptx.ImportedSlide, len(slides))
	copy(result, slides)
	seen := false
	for index, slide := range result {
		if slide.Role != pptx.RoleTitle {
			continue
		}
		// A cover holds a name and a line under it. A slide carrying a table or
		// a chart is the report itself, whatever layout it was drawn on: put it
		// on a cover and the table is written out as text into the room kept for
		// one line — nine lines of it, in room for two.
		if !seen && len(slide.Tables) == 0 && len(slide.Charts) == 0 {
			seen = true
			continue
		}
		result[index].Role = pptx.RoleContent
	}
	return result
}

// withoutRunningHeaders takes the section marker off the slides that carry one.
//
// A deck of twenty-two slides had fifteen of them titled "3. 아이디어 구현": the
// designer put the section it belongs to in the corner of every slide, and the
// slide's own heading — "키워드 추출", "화자 분리" — sat in the body underneath.
// Read as it was drawn, the deck's outline is five slides with the same name.
//
// A repeated line is only a header when the slide has a name of its own under
// it. A weekly report whose every slide is headed "주간업무 추진실적 및 계획" over
// the same subheading has nothing else to be called, and keeps what it says.
func withoutRunningHeaders(slides []pptx.ImportedSlide) ([]pptx.ImportedSlide, []string) {
	if len(slides) < 4 {
		return slides, nil
	}
	most := len(slides) / 2
	titles := map[string]int{}
	lines := map[string]int{}
	for _, slide := range slides {
		if title := strings.TrimSpace(slide.Title); title != "" {
			titles[title]++
		}
		for _, bullet := range slide.Bullets {
			lines[strings.TrimSpace(bullet.Text)]++
		}
	}
	var headers []string
	seen := map[string]bool{}
	result := make([]pptx.ImportedSlide, len(slides))
	copy(result, slides)
	for index, slide := range result {
		title := strings.TrimSpace(slide.Title)
		if title == "" || titles[title] <= most {
			continue
		}
		for at, bullet := range slide.Bullets {
			name := strings.TrimSpace(bullet.Text)
			if name == "" || lines[name] > most {
				continue
			}
			// A heading is a slot: the emphasis a designer put on the line
			// cannot be drawn in one, and would be printed as characters.
			result[index].Title = pptx.WithoutInlineMarkup(name)
			result[index].Bullets = append(append([]pptx.ImportedLine{}, slide.Bullets[:at]...), slide.Bullets[at+1:]...)
			if !seen[title] {
				seen[title] = true
				headers = append(headers, title)
			}
			break
		}
	}
	return result, headers
}

// slidesNamed says which slides something happened to, by their place in the
// deck, so the reader can go and look at them.
//
// A count cannot be said here: "3장에는" is read as the third slide, not as
// three of them, so a message meant to point at one slide pointed at another.
// Past a handful the places stop being worth listing and the count is said in a
// form that cannot be mistaken for one.
func slidesNamed(positions []int) string {
	const listed = 6
	if len(positions) > listed {
		return fmt.Sprintf("슬라이드 %d장", len(positions))
	}
	places := make([]string, 0, len(positions))
	for _, at := range positions {
		places = append(places, strconv.Itoa(at))
	}
	return strings.Join(places, "·") + "번 슬라이드"
}

// wordlessSlide is a slide that carried nothing to read: no title, no subtitle,
// no points, no table and no chart. A picture is not words.
func wordlessSlide(slide pptx.ImportedSlide) bool {
	if strings.TrimSpace(slide.Title) != "" || strings.TrimSpace(slide.Lead) != "" {
		return false
	}
	if len(slide.Bullets) > 0 || len(slide.Tables) > 0 || len(slide.Charts) > 0 {
		return false
	}
	return strings.TrimSpace(slide.Notes) == ""
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
	// What the numbers are, which is the series' own name. A chart of one series
	// carries it nowhere else — the categories say when, the points say how much,
	// and only this says of what. Every other shape a chart comes back as keeps
	// it: two series become a table's row labels, a line chart names each line.
	// This branch used to drop it, so an imported chart of revenue became a
	// column of unlabelled numbers.
	if label := strings.TrimSpace(series.Name); label != "" {
		// A component's caption runs to the end of its line, so nothing in it
		// needs protecting — escapeItemField guards the fields of a
		// pipe-separated row, and a name like "매출|비용" put through it left a
		// backslash in the deck that nobody typed.
		fmt.Fprintf(&builder, "::%s %s\n", name, blockCaption(label))
	} else {
		fmt.Fprintf(&builder, "::%s\n", name)
	}
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
