package docs

import (
	"encoding/csv"
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
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff")))
	reader.Comma = separator
	// A ragged file is normal: someone's export has a trailing note, or a blank
	// column. Reading it is better than refusing it.
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return Document{}, fmt.Errorf("이 파일을 표로 읽지 못했습니다: %w", err)
	}
	document := Document{Title: titleOf(filename)}
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s\n@cover\n> %s\n\n", escapeLine(document.Title), escapeLine(filename))
	written, warnings := writeSheet(&builder, filename, "", rows)
	if written == 0 {
		return Document{}, fmt.Errorf("이 파일에는 읽을 표가 없습니다")
	}
	document.Source = builder.String()
	document.Warnings = warnings
	return document, nil
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
	body := rows[1:]
	if len(body) > maximumRows {
		body = body[:maximumRows]
		warnings = append(warnings, fmt.Sprintf("%s의 행이 많아 앞 %d줄만 가져왔습니다",
			sheetLabel(filename, sheet), maximumRows))
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
	return 1, warnings
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
