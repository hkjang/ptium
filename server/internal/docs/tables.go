package docs

import (
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// A spreadsheet is a deck's numbers, already gathered.
//
// What it is not is a deck: a hundred rows on one slide is a picture of a
// spreadsheet. So a sheet becomes what a person would draw from it — a chart
// when it is one series of figures against labels, a table otherwise — and the
// slide says which file and which range it came from.

// readSeparated reads a CSV or TSV file.
func readSeparated(filename string, data []byte, separator rune) (Document, error) {
	rows, forgiven, err := separatedRows(strings.TrimPrefix(string(data), "\ufeff"), separator)
	if err != nil {
		return Document{}, err
	}
	document := Document{Title: titleOf(filename)}
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s\n@cover\n> %s\n\n", escapeLine(document.Title), escapeLine(filename))
	written, warnings := writeSheet(&builder, filename, "", rows)
	if written == 0 {
		return Document{}, fmt.Errorf("이 파일에는 읽을 표가 없습니다")
	}
	document.Source = builder.String()
	if forgiven > 0 {
		document.Warnings = append(document.Warnings, fmt.Sprintf(
			"%s의 %d번째 줄에 짝이 없는 따옴표가 있어 글자 그대로 읽었습니다", filename, forgiven))
	}
	document.Warnings = append(document.Warnings, warnings...)
	return document, nil
}

// separatedRows reads the rows, and says which line's quote it had to forgive.
//
// A quote in a field nobody quoted is not a broken file. An export writes
// 15" 모니터, or a width as 21", and the strict reader stops the whole file at
// that one character: five hundred rows refused over a Korean sentence with an
// English parser message inside it, naming a line and a column nobody can act
// on. So a file the strict reader will not take is read again with the quote
// taken literally, and the line it was on is said as a warning instead — in
// case what is written there was meant to be a quoted field after all.
func separatedRows(text string, separator rune) ([][]string, int, error) {
	rows, strict := separatedReader(text, separator, false).ReadAll()
	if strict == nil {
		return rows, 0, nil
	}
	lazy, err := separatedReader(text, separator, true).ReadAll()
	if err != nil {
		return nil, 0, fmt.Errorf("이 파일을 표로 읽지 못했습니다: %w", strict)
	}
	// The line the record started on, not the one the reader gave up on: a
	// quote that never closes is a mistake where it was opened, and the reader
	// runs to the end of the file before it says anything.
	line := 1
	var parse *csv.ParseError
	if errors.As(strict, &parse) {
		if parse.StartLine > 0 {
			line = parse.StartLine
		} else if parse.Line > 0 {
			line = parse.Line
		}
	}
	return lazy, line, nil
}

func separatedReader(text string, separator rune, lazy bool) *csv.Reader {
	reader := csv.NewReader(strings.NewReader(text))
	reader.Comma = separator
	// A ragged file is normal: someone's export has a trailing note, or a blank
	// column. Reading it is better than refusing it.
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = lazy
	return reader
}

// writeSheet turns a grid into slides, and returns how many it wrote.
func writeSheet(builder *strings.Builder, filename, sheet string, rows [][]string) (int, []string) {
	rows = trimGrid(rows)
	if len(rows) < 2 {
		return 0, nil
	}
	var warnings []string
	columns := len(rows[0])
	if columns > maximumColumns {
		columns = maximumColumns
		warnings = append(warnings, fmt.Sprintf("%s의 열이 많아 앞 %d개만 가져왔습니다",
			sheetLabel(filename, sheet), maximumColumns))
	}
	// A sheet longer than a slide holds continues on the next one rather than
	// stopping at the eighth row: a twelve-row report table is a table, not the
	// first eight rows of one. Past a few slides it is a spreadsheet rather than
	// a deck, and what is left is said instead of drawn.
	all := rows[1:]
	body := all
	var carried [][][]string
	if len(body) > maximumRows {
		body = all[:maximumRows]
		for at := maximumRows; at < len(all) && len(carried) < maximumTableSlides-1; at += maximumRows {
			end := min(at+maximumRows, len(all))
			carried = append(carried, all[at:end])
		}
		if written := maximumRows + len(carried)*maximumRows; written < len(all) {
			warnings = append(warnings, fmt.Sprintf("%s의 행이 많아 앞 %d줄만 가져왔습니다",
				sheetLabel(filename, sheet), written))
		}
	}

	heading := strings.TrimSpace(sheet)
	if heading == "" {
		heading = titleOf(filename)
	}
	fmt.Fprintf(builder, "# %s\n", escapeLine(heading))
	// One label column and one column of figures is a chart, which is what a
	// person would draw. Anything wider is a table.
	if columns == 2 && allNumeric(body, 1) {
		fmt.Fprintf(builder, "::columns %s\n", escapeLine(strings.TrimSpace(rows[0][1])))
		for _, row := range body {
			fmt.Fprintf(builder, "- %s | %s\n", escapeField(row[0]), escapeField(row[1]))
		}
	} else {
		fmt.Fprintf(builder, "::table %s\n", escapeLine(strings.TrimSpace(rows[0][0])))
		for _, row := range append([][]string{rows[0]}, body...) {
			fields := make([]string, 0, columns)
			for index := 0; index < columns; index++ {
				value := ""
				if index < len(row) {
					value = row[index]
				}
				fields = append(fields, escapeField(value))
			}
			fmt.Fprintf(builder, "- %s\n", strings.Join(fields, " | "))
		}
	}
	builder.WriteString("::\n")
	builder.WriteString(citation(filename, rangeOf(sheet, columns, len(body)+1)))
	builder.WriteString("\n")
	written := 1
	for _, piece := range carried {
		fmt.Fprintf(builder, "# %s (계속)\n", escapeLine(heading))
		if columns == 2 && allNumeric(piece, 1) {
			fmt.Fprintf(builder, "::columns %s\n", escapeLine(strings.TrimSpace(rows[0][1])))
			for _, row := range piece {
				fmt.Fprintf(builder, "- %s | %s\n", escapeField(row[0]), escapeField(row[1]))
			}
		} else {
			fmt.Fprintf(builder, "::table %s\n", escapeLine(strings.TrimSpace(rows[0][0])))
			for _, row := range append([][]string{rows[0]}, piece...) {
				fields := make([]string, 0, columns)
				for index := 0; index < columns; index++ {
					value := ""
					if index < len(row) {
						value = row[index]
					}
					fields = append(fields, escapeField(value))
				}
				fmt.Fprintf(builder, "- %s\n", strings.Join(fields, " | "))
			}
		}
		builder.WriteString("::\n")
		builder.WriteString(citation(filename, rangeOf(sheet, columns, len(piece)+1)))
		builder.WriteString("\n")
		written++
	}
	return written, warnings
}

func sheetLabel(filename, sheet string) string {
	if strings.TrimSpace(sheet) == "" {
		return filename
	}
	return sheet
}

// rangeOf is where on the sheet the slide came from, written the way a
// spreadsheet writes it: "Sheet1!A1:C9".
func rangeOf(sheet string, columns, rows int) string {
	if columns < 1 {
		columns = 1
	}
	if rows < 1 {
		rows = 1
	}
	reference := fmt.Sprintf("A1:%s%d", columnLetter(columns-1), rows)
	if sheet := strings.TrimSpace(sheet); sheet != "" {
		return sheet + "!" + reference
	}
	return reference
}

func columnLetter(index int) string {
	if index < 0 {
		index = 0
	}
	if index < 26 {
		return string(rune('A' + index))
	}
	return string(rune('A'+index/26-1)) + string(rune('A'+index%26))
}

// trimGrid drops empty rows and trailing empty columns, which every export has.
func trimGrid(rows [][]string) [][]string {
	widest := 0
	cleaned := make([][]string, 0, len(rows))
	for _, row := range rows {
		last := -1
		for index, cell := range row {
			if strings.TrimSpace(cell) != "" {
				last = index
			}
		}
		if last < 0 {
			continue
		}
		row = row[:last+1]
		widest = max(widest, len(row))
		cleaned = append(cleaned, row)
	}
	for index, row := range cleaned {
		for len(row) < widest {
			row = append(row, "")
		}
		cleaned[index] = row
	}
	return cleaned
}

// allNumeric reports whether every row carries a number in a column.
func allNumeric(rows [][]string, column int) bool {
	found := false
	for _, row := range rows {
		if column >= len(row) {
			return false
		}
		value := strings.TrimSpace(row[column])
		if value == "" {
			return false
		}
		if _, err := strconv.ParseFloat(strings.NewReplacer(",", "", "%", "", " ", "").Replace(value), 64); err != nil {
			return false
		}
		found = true
	}
	return found
}
