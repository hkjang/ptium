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
	written, warnings := writeSheet(&builder, "실적.csv", "", rows(12))
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
	written, warnings = writeSheet(&builder, "실적.csv", "", rows(40))
	if written != maximumTableSlides {
		t.Errorf("a forty-row sheet wrote %d slides", written)
	}
	if !strings.Contains(strings.Join(warnings, " "), "32줄") {
		t.Errorf("a forty-row sheet said %v", warnings)
	}

	// And a short one is one slide, as it was.
	builder.Reset()
	if written, warnings = writeSheet(&builder, "실적.csv", "", rows(3)); written != 1 || len(warnings) != 0 {
		t.Errorf("a three-row sheet wrote %d slides and said %v", written, warnings)
	}
}
