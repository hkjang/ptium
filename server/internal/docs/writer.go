package docs

import (
	"fmt"
	"regexp"
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
	heading string
	// where is the place in the file this slide came from, when the file has
	// places: a page in a PDF. Without one, the heading is what a person would
	// search for.
	where  string
	points []string
	// notes is what belongs to the slide but not on it. A page of a report has
	// more lines on it than a slide holds, and the ones past that are said
	// rather than shown — which is what speaker notes are.
	notes    []string
	tables   [][][]string
	slides   int
	warnings []string
	// coverWritten says whether the deck's first slide has been written. It is
	// held back until the first section is known.
	coverWritten bool
	// dropped counts what a long document left behind, so the reader is told.
	dropped int
}

func newDeckWriter(filename, title string) *deckWriter {
	return &deckWriter{filename: filename, title: title}
}

// cover writes the deck's first slide, once the writer has seen enough of the
// document to know what to call it.
//
// A report names itself on its first line. Calling the deck "report" after the
// file, and then giving the document's own title a slide with nothing under it,
// is how an import announces that nobody read the document — so the first
// heading becomes the deck's name when it is the document's name, and the
// sentence under it becomes the line beneath.
func (w *deckWriter) cover() {
	if w.coverWritten {
		return
	}
	w.coverWritten = true
	title := w.title
	if heading := strings.TrimSpace(w.heading); heading != "" {
		title = heading
		w.title = heading
		if len(w.points) == 0 && len(w.tables) == 0 {
			// A title on a line of its own is the document's name, not a slide
			// with nothing on it.
			w.heading = ""
		}
	}
	// The line under the title is the file the deck came from, and a document
	// named after what it is about makes that line say the title again. The
	// product's own measurement calls a cover like that "the same point twice",
	// and it is right: every slide already cites the file.
	if sameThing(title, w.filename) {
		fmt.Fprintf(&w.builder, "# %s\n@cover\n\n", escapeLine(title))
		return
	}
	fmt.Fprintf(&w.builder, "# %s\n@cover\n> %s\n\n", escapeLine(title), escapeLine(w.filename))
}

// copyMarker is what a second download of the same file is called: "보고서 (1)".
var copyMarker = regexp.MustCompile(`\s*\(\d+\)$`)

// sameThing says whether a file's name and a deck's title name the same thing.
func sameThing(title, filename string) bool {
	named := copyMarker.ReplaceAllString(titleOf(filename), "")
	return strings.EqualFold(strings.Join(strings.Fields(named), " "),
		strings.Join(strings.Fields(strings.TrimSpace(title)), " "))
}

// slide ends the slide being written and starts another.
func (w *deckWriter) slide(heading string) {
	w.flush()
	w.heading = strings.TrimSpace(heading)
}

// slideFrom starts a slide that knows where in the file it came from.
func (w *deckWriter) slideFrom(heading, where string) {
	w.slide(heading)
	w.where = strings.TrimSpace(where)
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
		heading, where := w.heading, w.where
		w.flush()
		w.heading = continued(heading)
		// A continuation came from the same place in the file as what it
		// continues. Losing that here is how the second half of a page ended up
		// citing its own title instead of the page.
		w.where = where
	}
	w.points = append(w.points, trimmed)
}

// note adds a line that belongs to the slide but not on it.
func (w *deckWriter) note(text string) {
	if trimmed := strings.TrimSpace(text); trimmed != "" {
		w.notes = append(w.notes, trimmed)
	}
}

// continued names the slide that carries on from another, once.
// "제1장 (계속) (계속) (계속)" is a title nobody wrote.
func continued(heading string) string {
	if strings.HasSuffix(heading, " (계속)") {
		return heading
	}
	return heading + " (계속)"
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
	if w.heading == "" && len(w.points) == 0 && len(w.tables) == 0 && len(w.notes) == 0 {
		return
	}
	if !w.coverWritten {
		// The first section with anything in it decides what the deck is called,
		// and may turn out to be the cover itself rather than a slide.
		w.cover()
		if w.heading == "" && len(w.points) == 0 && len(w.tables) == 0 {
			return
		}
	}
	if w.slides >= maximumSlides {
		w.dropped++
		w.heading, w.points, w.tables, w.notes, w.where = "", nil, nil, nil, ""
		return
	}
	heading := w.heading
	if heading == "" {
		heading = w.title
	}
	tables, points, notes, from := w.tables, w.points, w.notes, w.heading
	if w.where != "" {
		from = w.where
	}
	w.heading, w.points, w.tables, w.notes, w.where = "", nil, nil, nil, ""
	// A table longer than a slide holds continues on the next one, header and
	// all. Cutting it at the eighth row is how a report's twelve-row table
	// arrived as eight rows with the rest on no slide and nobody told.
	first, rest := splitTables(tables, w.report)
	w.writeSlide(heading, first, points, notes, from)
	for _, carried := range rest {
		if w.slides >= maximumSlides {
			w.dropped++
			continue
		}
		w.writeSlide(continued(heading), [][][]string{carried}, nil, nil, from)
	}
}

// writeSlide writes one slide: its heading, its tables, its points and where it
// came from.
func (w *deckWriter) writeSlide(heading string, tables [][][]string, points, notes []string, from string) {
	fmt.Fprintf(&w.builder, "# %s\n", escapeLine(heading))
	for _, table := range tables {
		columns := min(len(table[0]), maximumColumns)
		fmt.Fprintf(&w.builder, "::table\n")
		for _, row := range table {
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
	for _, point := range points {
		fmt.Fprintf(&w.builder, "- %s\n", escapeLine(point))
	}
	for _, note := range notes {
		fmt.Fprintf(&w.builder, "!notes %s\n", escapeField(note))
	}
	w.builder.WriteString(citation(w.filename, from))
	w.builder.WriteString("\n")
	w.slides++
}

// report notes something the reader should know about the import.
func (w *deckWriter) report(warning string) {
	for _, said := range w.warnings {
		if said == warning {
			return
		}
	}
	w.warnings = append(w.warnings, warning)
}

// splitTables cuts each table into slide-sized pieces, repeating its header on
// every one, and says when a table was wider than a slide holds.
func splitTables(tables [][][]string, report func(string)) (first [][][]string, rest [][][]string) {
	for _, table := range tables {
		if len(table) == 0 {
			continue
		}
		if report != nil && len(table[0]) > maximumColumns {
			report(fmt.Sprintf("표 %q의 %d개 열 가운데 %d개만 가져왔습니다. 한 슬라이드가 담는 만큼입니다",
				strings.TrimSpace(table[0][0]), len(table[0]), maximumColumns))
		}
		header, body := table[0], table[1:]
		if len(body) <= maximumRows {
			first = append(first, table)
			continue
		}
		first = append(first, append([][]string{header}, body[:maximumRows]...))
		for at := maximumRows; at < len(body); at += maximumRows {
			end := min(at+maximumRows, len(body))
			rest = append(rest, append([][]string{header}, body[at:end]...))
		}
	}
	return first, rest
}

// document ends the writing and returns what was written.
func (w *deckWriter) document() (Document, error) {
	w.flush()
	w.cover()
	if w.slides == 0 {
		return Document{}, fmt.Errorf("이 문서에서 슬라이드로 만들 내용을 찾지 못했습니다")
	}
	if w.dropped > 0 {
		w.warnings = append(w.warnings,
			fmt.Sprintf("문서가 길어 %d장은 가져오지 않았습니다. 나눠서 올리면 전부 가져옵니다", w.dropped))
	}
	return Document{Title: w.title, Source: w.builder.String(), Warnings: w.warnings}, nil
}
