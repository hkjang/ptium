package docs

import (
	"fmt"
	"strings"
	"testing"
)

// A report's table is longer than a slide holds, and cutting it at the eighth
// row is how a twelve-row table arrived as eight rows with the rest on no slide
// and nobody told. It continues on the next slide instead, header and all.
func TestALongTableContinuesOnTheNextSlide(t *testing.T) {
	rows := [][]string{{"구분", "내용"}}
	for index := 1; index <= 12; index++ {
		rows = append(rows, []string{fmt.Sprintf("항목%d", index), fmt.Sprintf("값%d", index)})
	}
	writer := newDeckWriter("보고서.docx", "보고서")
	writer.slide("추진 실적")
	writer.table(rows)
	document, err := writer.document()
	if err != nil {
		t.Fatalf("document: %v", err)
	}
	for index := 1; index <= 12; index++ {
		if !strings.Contains(document.Source, fmt.Sprintf("항목%d", index)) {
			t.Errorf("row %d is on no slide:\n%s", index, document.Source)
		}
	}
	headings := 0
	for _, line := range strings.Split(document.Source, "\n") {
		if line == "# 추진 실적" {
			headings++
		}
	}
	if headings != 2 || !strings.Contains(document.Source, "# 추진 실적 (계속)") {
		t.Errorf("the table did not continue on a slide of its own:\n%s", document.Source)
	}
	// Every piece carries the header, or a continuation reads as a list of
	// values with nothing to say what they are.
	if strings.Count(document.Source, "- 구분 | 내용") != 2 {
		t.Errorf("the header is not repeated on the continuation:\n%s", document.Source)
	}
}

// A table wider than a slide holds keeps the columns that fit, and says which
// ones it left.
func TestAWideTableSaysWhatItLeft(t *testing.T) {
	writer := newDeckWriter("보고서.docx", "보고서")
	writer.slide("현황")
	writer.table([][]string{
		{"번호", "구분", "내용", "담당", "기한", "비고"},
		{"1", "운영", "ITSM 개선", "김", "2월", "진행"},
	})
	document, err := writer.document()
	if err != nil {
		t.Fatalf("document: %v", err)
	}
	said := strings.Join(document.Warnings, " | ")
	if !strings.Contains(said, "열") {
		t.Errorf("a six-column table was cut to five and said %q", said)
	}
	if strings.Contains(document.Source, "비고") {
		t.Errorf("the sixth column was written after all:\n%s", document.Source)
	}
}

// A table that fits says nothing and stays on one slide.
func TestAnOrdinaryTableIsLeftAlone(t *testing.T) {
	writer := newDeckWriter("보고서.docx", "보고서")
	writer.slide("현황")
	writer.table([][]string{{"항목", "2026"}, {"인건비", "4.2"}, {"운영비", "1.1"}})
	document, err := writer.document()
	if err != nil {
		t.Fatalf("document: %v", err)
	}
	if strings.Contains(document.Source, "(계속)") {
		t.Errorf("a three-row table was split:\n%s", document.Source)
	}
	if len(document.Warnings) != 0 {
		t.Errorf("an ordinary table was reported: %v", document.Warnings)
	}
}

// A spreadsheet is read the same way: twelve rows are a table, not the first
// eight rows of one. Past a few slides it is a spreadsheet rather than a deck,
// and what is left is said instead of drawn.
func TestASheetLongerThanASlideContinues(t *testing.T) {
	rows := func(count int) [][]string {
		grid := [][]string{{"구분", "내용"}}
		for index := 1; index <= count; index++ {
			grid = append(grid, []string{fmt.Sprintf("항목%d", index), fmt.Sprintf("값%d", index)})
		}
		return grid
	}
	var builder strings.Builder
	written, warnings := writeSheet(&builder, "실적.csv", "", rows(12), placement{})
	if written != 2 {
		t.Errorf("a twelve-row sheet wrote %d slides", written)
	}
	for index := 1; index <= 12; index++ {
		if !strings.Contains(builder.String(), fmt.Sprintf("항목%d", index)) {
			t.Errorf("row %d is on no slide:\n%s", index, builder.String())
		}
	}
	if len(warnings) != 0 {
		t.Errorf("a table that was carried whole was reported: %v", warnings)
	}

	// A sheet nobody would sit through says what it left.
	builder.Reset()
	written, warnings = writeSheet(&builder, "실적.csv", "", rows(40), placement{})
	if written != maximumTableSlides {
		t.Errorf("a forty-row sheet wrote %d slides", written)
	}
	if !strings.Contains(strings.Join(warnings, " "), "32줄") {
		t.Errorf("a forty-row sheet said %v", warnings)
	}

	// And a short one is one slide, as it was.
	builder.Reset()
	if written, warnings = writeSheet(&builder, "실적.csv", "", rows(3), placement{}); written != 1 || len(warnings) != 0 {
		t.Errorf("a three-row sheet wrote %d slides and said %v", written, warnings)
	}
}

// numbersWithOneWordAt is a two-column sheet of twelve figures with a single
// cell of text in it, at the row asked for. Which slide that row lands on is
// what used to decide the shape of both of them.
func numbersWithOneWordAt(row int) [][]string {
	grid := [][]string{{"지역", "매출"}}
	for index := 1; index <= 12; index++ {
		figure := fmt.Sprintf("%d", index*100)
		if index == row {
			figure = "미정"
		}
		grid = append(grid, []string{fmt.Sprintf("지역%d", index), figure})
	}
	return grid
}

// A sheet is one shape the whole way through, chart or table, however many
// slides it takes.
//
// Each slide used to be asked on its own whether its own rows were figures, so
// one cell of text among twelve turned a sheet of sales by region into a bar
// chart titled "매출" followed by a table titled "지역 (계속)" — the same sheet
// arriving as two unrelated ones, and which of them came first depended only on
// where in the column the text happened to sit.
func TestASheetThatContinuesIsOneShape(t *testing.T) {
	// Late, so the first slide is all figures; and early, so the carried one
	// is. Either way the sheet has a cell no axis can be drawn from.
	for _, row := range []int{10, 3} {
		var builder strings.Builder
		written, warnings := writeSheet(&builder, "실적.csv", "", numbersWithOneWordAt(row), placement{})
		source := builder.String()
		if written != 2 {
			t.Fatalf("a twelve-row sheet wrote %d slides:\n%s", written, source)
		}
		if charts := strings.Count(source, "::columns"); charts != 0 {
			t.Errorf("text in row %d left %d of the slides a chart:\n%s", row, charts, source)
		}
		if tables := strings.Count(source, "::table"); tables != 2 {
			t.Errorf("text in row %d made %d of the two slides a table:\n%s", row, tables, source)
		}
		// Every piece of a table carries the header, or a continuation reads
		// as a list of values with nothing to say what they are.
		if headers := strings.Count(source, "- 지역 | 매출"); headers != 2 {
			t.Errorf("the header is on %d of the two slides:\n%s", headers, source)
		}
		if len(warnings) != 0 {
			t.Errorf("a sheet that was carried whole was reported: %v", warnings)
		}
	}
}

// A column that is figures all the way down is a chart on every slide, and the
// header row is the chart's own title rather than a row of its own.
func TestASheetOfFiguresThatContinuesStaysAChart(t *testing.T) {
	figures := func(count int) [][]string {
		grid := [][]string{{"지역", "매출"}}
		for index := 1; index <= count; index++ {
			grid = append(grid, []string{fmt.Sprintf("지역%d", index), fmt.Sprintf("%d", index*100)})
		}
		return grid
	}
	var builder strings.Builder
	written, _ := writeSheet(&builder, "실적.csv", "", figures(12), placement{})
	source := builder.String()
	if written != 2 || strings.Count(source, "::columns") != 2 {
		t.Errorf("twelve rows of figures wrote %d slides and %d charts:\n%s",
			written, strings.Count(source, "::columns"), source)
	}
	if strings.Contains(source, "::table") || strings.Contains(source, "- 지역 | 매출") {
		t.Errorf("a chart was given a table's header row:\n%s", source)
	}

	// The shape follows the rows a reader is shown. A sheet too long to be
	// carried whole stops at the thirty-second row, so a cell past that one is
	// on no slide and cannot decide what the slides look like.
	grid := figures(40)
	grid[35][1] = "미정"
	builder.Reset()
	written, warnings := writeSheet(&builder, "실적.csv", "", grid, placement{})
	source = builder.String()
	if written != maximumTableSlides || strings.Count(source, "::columns") != maximumTableSlides {
		t.Errorf("a forty-row sheet wrote %d slides and %d charts:\n%s",
			written, strings.Count(source, "::columns"), source)
	}
	if strings.Contains(source, "미정") {
		t.Errorf("a row the sheet left behind was written after all:\n%s", source)
	}
	if !strings.Contains(strings.Join(warnings, " "), "32줄") {
		t.Errorf("a forty-row sheet said %v", warnings)
	}
}
