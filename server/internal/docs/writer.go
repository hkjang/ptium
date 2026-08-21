package docs

import (
	"fmt"
	"strings"
)

// deckWriter turns a stream of headings, sentences and tables into deck source.
//
// The shape it produces is the shape a person makes from a report: a heading
// starts a slide, the sentences under it are its points, and a slide that runs
// long continues rather than losing what would not fit. Every slide says which
// file it came from.
type deckWriter struct {
	filename string
	title    string
	builder  strings.Builder
	// heading is the slide being written, and points what it has so far.
	heading  string
	points   []string
	tables   [][][]string
	slides   int
	warnings []string
	// dropped counts what a long document left behind, so the reader is told.
	dropped int
}

func newDeckWriter(filename, title string) *deckWriter {
	writer := &deckWriter{filename: filename, title: title}
	fmt.Fprintf(&writer.builder, "# %s\n@cover\n> %s\n\n", escapeLine(title), escapeLine(filename))
	return writer
}

// slide ends the slide being written and starts another.
func (w *deckWriter) slide(heading string) {
	w.flush()
	w.heading = strings.TrimSpace(heading)
}

// point adds a sentence to the slide being written. A slide that has taken all
// it can starts a continuation rather than dropping the rest.
func (w *deckWriter) point(text string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return
	}
	// A paragraph is not a bullet. What fits on a slide is its first sentence;
	// the rest of the paragraph is what the notes are for — but a report read
	// into a deck is a draft, so the sentence is kept and the length measured
	// later rather than cut here.
	if len(w.points) >= maximumPoints {
		heading := w.heading
		w.flush()
		w.heading = heading + " (계속)"
	}
	w.points = append(w.points, trimmed)
}

// table adds a grid to the slide being written.
func (w *deckWriter) table(rows [][]string) {
	rows = trimGrid(rows)
	if len(rows) < 2 {
		return
	}
	w.tables = append(w.tables, rows)
}

// flush writes the slide being written, if it has anything on it.
func (w *deckWriter) flush() {
	if w.heading == "" && len(w.points) == 0 && len(w.tables) == 0 {
		return
	}
	if w.slides >= maximumSlides {
		w.dropped++
		w.heading, w.points, w.tables = "", nil, nil
		return
	}
	heading := w.heading
	if heading == "" {
		heading = w.title
	}
	fmt.Fprintf(&w.builder, "# %s\n", escapeLine(heading))
	for _, table := range w.tables {
		columns := min(len(table[0]), maximumColumns)
		body := table
		if len(body) > maximumRows+1 {
			body = body[:maximumRows+1]
		}
		fmt.Fprintf(&w.builder, "::table\n")
		for _, row := range body {
			fields := make([]string, 0, columns)
			for index := 0; index < columns; index++ {
				value := ""
				if index < len(row) {
					value = row[index]
				}
				fields = append(fields, escapeField(value))
			}
			fmt.Fprintf(&w.builder, "- %s\n", strings.Join(fields, " | "))
		}
		w.builder.WriteString("::\n")
	}
	for _, point := range w.points {
		fmt.Fprintf(&w.builder, "- %s\n", escapeLine(point))
	}
	w.builder.WriteString(citation(w.filename, w.heading))
	w.builder.WriteString("\n")
	w.slides++
	w.heading, w.points, w.tables = "", nil, nil
}

// document ends the writing and returns what was written.
func (w *deckWriter) document() (Document, error) {
	w.flush()
	if w.slides == 0 {
		return Document{}, fmt.Errorf("이 문서에서 슬라이드로 만들 내용을 찾지 못했습니다")
	}
	if w.dropped > 0 {
		w.warnings = append(w.warnings,
			fmt.Sprintf("문서가 길어 %d장은 가져오지 않았습니다. 나눠서 올리면 전부 가져옵니다", w.dropped))
	}
	return Document{Title: w.title, Source: w.builder.String(), Warnings: w.warnings}, nil
}
