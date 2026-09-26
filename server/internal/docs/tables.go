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
	written, warnings := writeSheet(&builder, filename, "", rows, placement{})
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

// placement is where a grid's rows and columns are on the sheet it was read
// from, when the grid is not the whole sheet: what was hidden was cut out of
// the grid, but a slide's source is a place on the sheet, and the place has
// the hidden rows and columns in it. A placement that says nothing is a grid
// that is the sheet, row for row and column for column.
type placement struct {
	rows, columns []int
}

// row is the sheet's row, counted from zero, that a row of the grid was.
func (p placement) row(index int) int {
	if index >= 0 && index < len(p.rows) {
		return p.rows[index]
	}
	return index
}

// column is the sheet's column, counted from zero, that a column of the grid was.
func (p placement) column(index int) int {
	if index >= 0 && index < len(p.columns) {
		return p.columns[index]
	}
	return index
}

// writeSheet turns a grid into slides, and returns how many it wrote.
func writeSheet(builder *strings.Builder, filename, sheet string, rows [][]string, from placement) (int, []string) {
	rows, kept := trimmed(rows)
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
	// The source of a slide reaches to the last column and, given which rows of
	// the grid are the first and the last one on the slide, from the one to the
	// other — where each of them was on the sheet.
	//
	// Each slide names the rows it shows and no others. Every slide used to
	// begin its range at A1, so a sheet of twenty rows cited A1:B9, then A1:B17,
	// then A1:B21: the later slides swallowed rows that are on neither of them,
	// and the more slides a sheet took the more nearly all of them pointed at
	// the same place. A table repeats the header row on every slide, and the
	// range still leaves it out on the continuations — a spreadsheet range
	// cannot name two stretches with a gap between them, and what a reader
	// follows a citation for is where this slide's figures are.
	source := func(first, last int) string {
		return rangeOf(sheet, from.row(kept[first]), from.column(columns-1), from.row(kept[last]))
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
	//
	// The question is asked once, of the sheet, and not of each slide in turn. A
	// sheet carried onto a second slide is still one sheet, and asking each slide
	// on its own is how a single cell of text among twelve came out as a bar chart
	// titled "매출" followed by a table titled "지역 (계속)" — the same sheet
	// arriving as two unrelated ones, and which of them came first decided by
	// nothing but where in the column that cell happened to sit. Only the rows a
	// reader is shown are asked about: a row the sheet was too long to carry is on
	// no slide, and cannot change the shape of the slides that were.
	chart := columns == 2 && allNumeric(body, 1)
	for _, piece := range carried {
		chart = chart && allNumeric(piece, 1)
	}
	writeBody(builder, chart, rows[0], body, columns)
	builder.WriteString("::\n")
	// The heading is row 0 of the grid, so the last row of the slide is one
	// past the number of body rows on it.
	last := len(body)
	// Grid row 0 is the header, which is row 1 of the sheet, and the first slide
	// starts there.
	builder.WriteString(citation(filename, source(0, last)))
	builder.WriteString("\n")
	written := 1
	for _, piece := range carried {
		fmt.Fprintf(builder, "# %s (계속)\n", escapeLine(heading))
		writeBody(builder, chart, rows[0], piece, columns)
		builder.WriteString("::\n")
		// The piece begins at the row after the one the slide before it ended
		// on, which is what last still holds until it is carried forward.
		start := last + 1
		last += len(piece)
		builder.WriteString(citation(filename, source(start, last)))
		builder.WriteString("\n")
		written++
	}
	return written, warnings
}

// writeBody writes one slide's worth of a sheet — the block it opens and the
// rows on it — in whichever shape the sheet as a whole was given. A table
// repeats the header row on every slide it takes, or a continuation reads as a
// list of values with nothing to say what they are; a chart writes that header
// as its title instead, so the figures on the second slide are the same series
// as the ones on the first.
func writeBody(builder *strings.Builder, chart bool, header []string, piece [][]string, columns int) {
	if chart {
		fmt.Fprintf(builder, "::columns %s\n", escapeLine(strings.TrimSpace(header[1])))
		for _, row := range piece {
			fmt.Fprintf(builder, "- %s | %s\n", escapeField(row[0]), escapeField(row[1]))
		}
		return
	}
	fmt.Fprintf(builder, "::table %s\n", escapeLine(strings.TrimSpace(header[0])))
	for _, row := range append([][]string{header}, piece...) {
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

func sheetLabel(filename, sheet string) string {
	if strings.TrimSpace(sheet) == "" {
		return filename
	}
	return sheet
}

// rangeOf is where on the sheet the slide came from, written the way a
// spreadsheet writes it: "Sheet1!A1:C9" for the first slide a sheet takes and
// "Sheet1!A10:C17" for the one that continues it, given the first row, the last
// column and the last row of the slide as the sheet counts them, from zero.
func rangeOf(sheet string, first, column, last int) string {
	reference := fmt.Sprintf("A%d:%s%d", max(first, 0)+1, columnLetter(column), max(last, 0)+1)
	if sheet := strings.TrimSpace(sheet); sheet != "" {
		return sheet + "!" + reference
	}
	return reference
}

// columnLetter names a column, counted from zero, the way a sheet names it: A
// through Z, then AA, and on to XFD at the far edge. Carrying only once ran
// out of letters at ZZ and wrote "[A" for the column after it, into a source a
// slide cites — and a sheet of more than 702 columns is an ordinary sheet.
func columnLetter(index int) string {
	if index < 0 {
		index = 0
	}
	letters := ""
	for {
		letters = string(rune('A'+index%26)) + letters
		// Each place holds 26 names and none of them is a zero, so a carry
		// leaves one less than it would in a numbering that has one.
		index = index/26 - 1
		if index < 0 {
			return letters
		}
	}
}

// trimGrid drops empty rows and trailing empty columns, which every export has.
func trimGrid(rows [][]string) [][]string {
	cleaned, _ := trimmed(rows)
	return cleaned
}

// trimmed is trimGrid that also says, for each row it kept, which row of the
// grid it was.
func trimmed(rows [][]string) ([][]string, []int) {
	widest := 0
	cleaned := make([][]string, 0, len(rows))
	kept := make([]int, 0, len(rows))
	for at, row := range rows {
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
		kept = append(kept, at)
	}
	for index, row := range cleaned {
		for len(row) < widest {
			row = append(row, "")
		}
		cleaned[index] = row
	}
	return cleaned, kept
}

// allNumeric reports whether every row carries a number in a column.
//
// This is the question that sends a sheet on as a chart, and the answer has to
// stay no wider than what the deck's own parser (deck.parseNumber) can read
// back out of the same cell. The two are separate readings on purpose — the
// deck's one keeps a contract this one does not, that a figure ends at a space,
// which is what lets a line chart's row hold four of them — so widening this
// one past that is how a column gets called figures and then handed bar heights
// nobody wrote.
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
		if _, ok := amountOf(value); !ok {
			return false
		}
		found = true
	}
	return found
}

// amountOf reads a figure the way a sheet of money writes one.
//
// A column of amounts is a column of figures, but a spreadsheet almost never
// writes them bare. The Currency format puts a sign on every row — "₩1,200" in
// Korea, "$1,200" elsewhere, "1,200원" where the currency is a word. None of
// that is text a person put there; it is how the sheet shows the number. Read
// as text, one such column was not a column of figures, so a two-column sheet
// of sales by region came out as a table of the very numbers somebody opened it
// to see drawn.
//
// Only the signs come off, and a unit that is a word stays: "1월" is a month
// and "3개" is a count of things, and neither is a figure to plot an axis by.
// A space between the figure and its unit comes off with the unit, but a space
// inside the figure does not: the deck's parser ends a figure at any space, so
// "1 200" — how a sheet in a good many countries writes a thousand — is 1,200
// here and 1 on the bar, and the sheet is worse off as a chart of heights
// nobody wrote than as the table it was. Where the two readings disagree, this
// one gives way.
//
// The accounting bracket — "(340)", how a sheet writes a refund — is the same
// disagreement and stays out for the same reason. A bar is laid out by its
// magnitude, so a refund read as -340 is drawn in the direction and at the
// height of a month that sold 340, and the minus survives only in a value label
// the chart drops once it holds more than six bars. A column with a refund in
// it is left the table it was, where the brackets are still on the page.
// commaIsNotAThousandsSeparator reports whether a comma in the written figure is
// doing something other than grouping thousands — which, in practice, means the
// sheet is written in a locale where the comma is the decimal point.
//
// It matters because amountSigns strips commas outright. "1,200" is 1200 either
// way, but "€1.234,56" becomes "1.23456" and draws a bar a thousandth of its
// height, and the reader has no way to see that the number on the page is not
// the number on the chart. Before the European signs were added here such a cell
// was no figure at all and the sheet stayed a table, which was the safe answer.
//
// A comma that groups thousands is always followed by exactly three digits, and
// a sheet does not use both separators for grouping. Anything else is a locale
// this reader cannot tell apart, so it gives way — the same rule the space
// thousands form is excluded under.
func commaIsNotAThousandsSeparator(value string) bool {
	if !strings.Contains(value, ",") {
		return false
	}
	// Both separators present: whichever comes last is the decimal point.
	// "1,234.56" is English and reads correctly; "1.234,56" is not.
	if dot := strings.LastIndex(value, "."); dot >= 0 {
		return strings.LastIndex(value, ",") > dot
	}
	for index := strings.Index(value, ","); index >= 0; {
		digits := 0
		for position := index + 1; position < len(value) && value[position] >= '0' && value[position] <= '9'; position++ {
			digits++
		}
		if digits != 3 {
			return true
		}
		next := strings.Index(value[index+1:], ",")
		if next < 0 {
			break
		}
		index = index + 1 + next
	}
	return false
}

func amountOf(value string) (float64, bool) {
	if commaIsNotAThousandsSeparator(strings.TrimSpace(value)) {
		return 0, false
	}
	cleaned := strings.TrimSpace(amountSigns.Replace(strings.TrimSpace(value)))
	if !bareFigure(cleaned) {
		return 0, false
	}
	number, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

// bareFigure reports whether what is left once the signs are off is a figure
// the deck's parser reads to the end: a sign, digits, and at most one decimal
// point after a digit, and nothing else.
//
// This is the same giving way, spelt out. Go's own reading of a number is wider
// than the deck's — it takes the exponent a spreadsheet writes a large amount
// with ("1.5E+15", which is how the file itself holds a figure that long), and
// it takes "Inf" and "NaN" besides. The deck's parser stops at the letter, so a
// column of those is one this would call figures and the chart would then draw
// at 1.5. Whatever the two cannot read alike is left the table it was.
func bareFigure(text string) bool {
	digits := false
	point := false
	for index, character := range text {
		switch {
		case character >= '0' && character <= '9':
			digits = true
		case character == '.' && digits && !point:
			point = true
		case (character == '-' || character == '+') && index == 0:
		default:
			return false
		}
	}
	return digits
}

// The marks a spreadsheet puts on a figure without changing what the figure is:
// the thousands separator and the per-cent sign a column already carried, and
// the currency signs.
//
// No space is among them, plain or fixed (U+00A0), though a spreadsheet writes
// both — one between a figure and its unit, which comes off with the trimming
// that follows the replacing, and one inside the figure as the thousands
// separator, which must not come off at all. Taking a space off here while the
// deck's parser ends a figure at it is what read "1 200" as 1,200 on the way in
// and drew it as 1 on the way out, a bar an eighth of a per cent tall. Left in
// place, such a column is no figure column and the sheet stays the table it is.
var amountSigns = strings.NewReplacer(
	",", "", "%", "",
	"₩", "", "￦", "", "$", "", "€", "", "£", "", "¥", "", "￥", "", "원", "",
)
