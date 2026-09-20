package docs

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// An .xlsx is a zip of XML, the same shape as the workbook Ptium writes behind
// every exported chart. Reading one needs no library: the sheets are rows of
// cells, the strings are in one shared table, and everything else on the sheet
// — formatting, formulas, pivot caches — is not what a deck is made of.

type workbookIndex struct {
	Properties struct {
		Date1904 string `xml:"date1904,attr"`
	} `xml:"workbookPr"`
	Sheets struct {
		Sheet []struct {
			Name  string `xml:"name,attr"`
			ID    string `xml:"id,attr"`
			State string `xml:"state,attr"`
		} `xml:"sheet"`
	} `xml:"sheets"`
}

type workbookRelationships struct {
	Relationship []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

type sharedStrings struct {
	Items []struct {
		Text string   `xml:"t"`
		Runs []string `xml:"r>t"`
	} `xml:"si"`
}

type worksheet struct {
	// A sheet says which of its columns are hidden ahead of its rows, as
	// ranges: "min" to "max", counted from one.
	Columns []struct {
		Min    int    `xml:"min,attr"`
		Max    int    `xml:"max,attr"`
		Hidden string `xml:"hidden,attr"`
	} `xml:"cols>col"`
	Rows []struct {
		Reference string `xml:"r,attr"`
		Hidden    string `xml:"hidden,attr"`
		Cells     []struct {
			Reference  string   `xml:"r,attr"`
			Type       string   `xml:"t,attr"`
			Style      string   `xml:"s,attr"`
			Value      string   `xml:"v"`
			Inline     string   `xml:"is>t"`
			InlineRuns []string `xml:"is>r>t"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}

// readWorkbook reads a spreadsheet into slides, one per sheet.
func readWorkbook(filename string, data []byte) (Document, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Document{}, fmt.Errorf("이 파일은 엑셀 통합 문서가 아닙니다")
	}
	parts := map[string][]byte{}
	for _, file := range archive.File {
		opened, err := file.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(io.LimitReader(opened, 32<<20))
		opened.Close()
		if err == nil {
			parts[file.Name] = content
		}
	}
	var index workbookIndex
	if err := xml.Unmarshal(parts["xl/workbook.xml"], &index); err != nil || len(index.Sheets.Sheet) == 0 {
		return Document{}, fmt.Errorf("이 통합 문서에서 시트를 찾지 못했습니다")
	}
	var relationships workbookRelationships
	_ = xml.Unmarshal(parts["xl/_rels/workbook.xml.rels"], &relationships)
	target := map[string]string{}
	for _, relationship := range relationships.Relationship {
		target[relationship.ID] = strings.TrimPrefix(relationship.Target, "/")
	}
	var strings0 sharedStrings
	_ = xml.Unmarshal(parts["xl/sharedStrings.xml"], &strings0)
	// What each style means, so a date is a day and a per cent is a per cent —
	// and which day the workbook counts its days from, so the day is the one on
	// the sheet.
	formats := readCellFormats(parts["xl/styles.xml"], counts1904(index.Properties.Date1904))
	shared := make([]string, 0, len(strings0.Items))
	for _, item := range strings0.Items {
		if item.Text != "" {
			shared = append(shared, item.Text)
			continue
		}
		shared = append(shared, strings.Join(item.Runs, ""))
	}

	document := Document{Title: titleOf(filename)}
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s\n@cover\n> %s\n\n", escapeLine(document.Title), escapeLine(filename))
	written := 0
	var warnings []string
	var hidden []string
	var left []string
	for _, sheet := range index.Sheets.Sheet {
		// A workbook says which of its sheets it hides, and a hidden sheet is
		// one nobody meant anyone to see: the code table a formula looks up in,
		// the settings a macro reads, last quarter's working copy. Taken as if
		// it were visible, it became a slide in the middle of the deck.
		if sheetHidden(sheet.State) {
			hidden = append(hidden, sheet.Name)
			continue
		}
		name := target[sheet.ID]
		if name == "" {
			continue
		}
		content, ok := parts[sheetPart(name)]
		if !ok {
			continue
		}
		var parsed worksheet
		if err := xml.Unmarshal(content, &parsed); err != nil {
			continue
		}
		rows, at, hiddenRows, hiddenColumns := onScreen(parsed, gridOf(parsed, shared, formats))
		// A deck holds so many slides, and a workbook of forty sheets runs out
		// of deck before it runs out of sheets. What was left out is what the
		// person who uploaded it has to know, and naming it is the only way
		// they can: "시트가 많아 앞 30개만 가져왔습니다" named neither, and the
		// number in it was how many slides a deck holds — so a workbook whose
		// sheets each fill three slides said it had taken the first thirty of
		// its eleven. A sheet with nothing on it was never going to be a
		// slide, so it is not something that was left out.
		if written >= maximumSlides {
			if len(trimGrid(rows)) >= 2 {
				left = append(left, sheet.Name)
			}
			continue
		}
		count, sheetWarnings := writeSheet(&builder, filename, sheet.Name, rows, at)
		written += count
		warnings = append(warnings, sheetWarnings...)
		// What was hidden is worth a line only under a slide it was hidden
		// from; a sheet that made no slide left nothing out of one.
		if count > 0 {
			if concealed := concealedNamed(hiddenRows, hiddenColumns); concealed != "" {
				warnings = append(warnings, fmt.Sprintf("%s의 숨긴 %s 가져오지 않았습니다",
					sheetLabel(filename, sheet.Name), concealed))
			}
		}
	}
	if written == 0 {
		// Why there is nothing to show matters when the file plainly has
		// figures in it: "읽을 표가 없습니다" sends someone back to a workbook
		// they can see the numbers in, to look for what is wrong with it.
		if len(hidden) > 0 {
			return Document{}, fmt.Errorf("이 통합 문서의 시트(%s)는 모두 숨겨져 있습니다",
				sheetsNamed(hidden))
		}
		return Document{}, fmt.Errorf("이 통합 문서에는 읽을 표가 없습니다")
	}
	if len(left) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"슬라이드가 많아 시트(%s)는 가져오지 않았습니다. 나눠서 올리면 전부 가져옵니다",
			sheetsNamed(left)))
	}
	if len(hidden) > 0 {
		warnings = append(warnings, fmt.Sprintf("숨겨진 시트(%s)는 가져오지 않았습니다",
			sheetsNamed(hidden)))
	}
	document.Source = builder.String()
	document.Warnings = warnings
	return document, nil
}

// sheetHidden reports whether a workbook hides a sheet. "veryHidden" is the
// one only a macro can put back, which is further from being meant for a deck
// rather than nearer; anything else, including the state a sheet writes by
// writing nothing, is a sheet somebody looks at.
func sheetHidden(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "hidden", "veryhidden":
		return true
	}
	return false
}

// sheetsNamed writes the sheets a warning is about, named while naming them is
// what somebody can act on.
//
// A warning is one line that somebody reads. Forty names is the workbook's
// table of contents written into that line, and the ones at the end of it are
// the ones that go off the edge; past a handful, how many there are is the
// part that can be acted on, and the first few say which end of the workbook
// they came from.
func sheetsNamed(names []string) string {
	const named = 5
	if len(names) <= named {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s 외 %d개", strings.Join(names[:named], ", "), len(names)-named)
}

// counts1904 reports whether a workbook counts its days from 1904 rather than
// from 1900, which is what Excel for the Macintosh wrote and what a workbook
// keeps doing however it is opened afterwards.
//
// The switch is written the way the format writes every switch: "1" or "true",
// and a workbook that says nothing counts from 1900.
func counts1904(value string) bool {
	return switchedOn(value)
}

// switchedOn reads a switch the way the format writes every one of them: "1"
// or "true" is on, and anything else, including nothing, is off.
func switchedOn(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true":
		return true
	}
	return false
}

// onScreen keeps of a sheet's grid what is on the screen: the rows and columns
// the sheet does not hide. It reports where on the sheet what it kept was, and
// how many rows and columns it left out.
//
// A sheet hides rows and columns for the same reasons a workbook hides sheets
// — the rows a filter took out of view, the group somebody folded up, the
// column of codes a formula looks up in — and they were as much not meant for
// a deck. Read in, a hidden column of figures beside the one column of them
// that was meant to be seen made the sheet a table where a chart was wanted,
// and a hidden column of anything sat in the middle of the table where nobody
// had seen it. Hidden columns are cut out rather than blanked, since a blank
// column in the middle of a table is not trimmed and would stay there.
//
// What is counted is what was on the screen to hide: a hidden row with no
// cell in it, or a hidden column no shown row writes anything in, was never
// going to be in the table either way, and counting it would put a number in
// the warning that nobody can find on the sheet. Excel hides columns to the
// edge of the sheet in one range, and that range is not sixteen thousand
// columns of anything.
//
// The ranges are kept as the ranges they were written as and asked about one
// column of the grid at a time. Spread out into every column they cover, they
// were sixteen thousand entries for the range Excel writes, and as many as a
// file cared to say for the range a file wrote: a sheet of six cells hiding
// columns C through four billion was enough to run the server out of memory.
// The grid's own width is the only thing here worth paying for.
func onScreen(sheet worksheet, grid [][]string) (rows [][]string, at placement, hiddenRows, hiddenColumns int) {
	rows = make([][]string, 0, len(grid))
	width := 0
	for index, line := range grid {
		// gridOf writes one line for each row, in the row's order.
		if index < len(sheet.Rows) && switchedOn(sheet.Rows[index].Hidden) {
			if !blankLine(line) {
				hiddenRows++
			}
			continue
		}
		rows = append(rows, line)
		at.rows = append(at.rows, index)
		width = max(width, len(line))
	}
	var ranges [][2]int
	for _, column := range sheet.Columns {
		// A range with no bounds, or bounds the wrong way round, is a column
		// setting that names no column, and hides none.
		if !switchedOn(column.Hidden) || column.Min < 1 || column.Max < column.Min {
			continue
		}
		ranges = append(ranges, [2]int{column.Min - 1, column.Max - 1})
	}
	if len(ranges) == 0 {
		return rows, at, hiddenRows, 0
	}
	hidden := func(column int) bool {
		for _, span := range ranges {
			if column >= span[0] && column <= span[1] {
				return true
			}
		}
		return false
	}
	for column := 0; column < width; column++ {
		if !hidden(column) {
			at.columns = append(at.columns, column)
		}
	}
	written := map[int]bool{}
	for index, line := range rows {
		kept := make([]string, 0, len(line))
		for column, value := range line {
			if !hidden(column) {
				kept = append(kept, value)
				continue
			}
			if strings.TrimSpace(value) != "" {
				written[column] = true
			}
		}
		rows[index] = kept
	}
	return rows, at, hiddenRows, len(written)
}

// blankLine reports whether a row of the grid has nothing written in it.
func blankLine(line []string) bool {
	for _, value := range line {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

// concealedNamed writes what a sheet hid, for the warning that says so: rows,
// columns, or both, and nothing when nothing was hidden. The phrase ends in a
// count, and the particle that follows a count is always the same one, so it
// is written here rather than chosen in the warning for a phrase it cannot see.
func concealedNamed(rows, columns int) string {
	var parts []string
	if rows > 0 {
		parts = append(parts, fmt.Sprintf("행 %d개", rows))
	}
	if columns > 0 {
		parts = append(parts, fmt.Sprintf("열 %d개", columns))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "와 ") + "는"
}

// sheetPart joins a workbook-relative part name, which may already be absolute.
func sheetPart(name string) string {
	if strings.HasPrefix(name, "xl/") {
		return name
	}
	return "xl/" + name
}

// gridOf reads a sheet into rows of text, in the cells' own positions: a sheet
// leaves out empty cells, and reading them in order would shift a row left.
//
// The position is in the cell's own reference where it has one. It is allowed
// not to have one — the reference is optional, and the writers that stream a
// sheet out row by row leave it off — and then the cell is simply the next one
// along. Reading a missing reference as column A instead put every cell of the
// row in the same place and kept the last: a two-column sheet came back as one
// column of figures with the labels gone, and the deck said nothing about it.
func gridOf(sheet worksheet, shared []string, formats cellFormats) [][]string {
	grid := make([][]string, 0, len(sheet.Rows))
	for _, row := range sheet.Rows {
		cells := map[int]string{}
		widest := -1
		next := 0
		for _, cell := range row.Cells {
			column := columnOf(cell.Reference)
			if column < 0 {
				column = next
			}
			next = column + 1
			value := cell.Value
			switch cell.Type {
			case "s":
				if index, err := strconv.Atoi(strings.TrimSpace(cell.Value)); err == nil && index >= 0 && index < len(shared) {
					value = shared[index]
				}
			case "inlineStr":
				value = cell.Inline
				if value == "" {
					value = strings.Join(cell.InlineRuns, "")
				}
			case "b":
				// A logical cell stores TRUE as 1 and FALSE as 0, and the sheet
				// shows the word. Read as the number it stores, a checklist came
				// back as a column of 0s and 1s and was drawn as a bar chart of
				// them; with a format on the cell it was worse, because 1 under a
				// date format is a day and 0 under a per cent is "0%".
				value = truthOf(cell.Value)
			case "d":
				// A workbook written to the strict schema keeps a date as the
				// date, not as a count of days. It is the format that says how
				// much of it the sheet shows, the same as for a count.
				if shown, ok := formats.moment(cell.Style, value); ok {
					value = shown
				}
			case "str", "e":
				// A formula's cached result: text, or the error it ended in.
				// Neither is a number, so neither is a day or a per cent —
				// a formula that returned the characters "45678" into a
				// date-formatted cell is not 2025-01-21.
			default:
				// A number carries its meaning in its format: a day counted
				// from 1899-12-30, or a fraction of one written as a per cent.
				if shown, ok := formats.written(cell.Style, value); ok {
					value = shown
				}
			}
			cells[column] = strings.TrimSpace(value)
			if column > widest {
				widest = column
			}
		}
		line := make([]string, widest+1)
		for column, value := range cells {
			if column >= 0 && column <= widest {
				line[column] = value
			}
		}
		grid = append(grid, line)
	}
	return grid
}

// truthOf writes a logical cell the way a spreadsheet shows it. Anything that
// is neither of the two things a logical cell can store is left as it is.
func truthOf(value string) string {
	switch strings.TrimSpace(value) {
	case "1":
		return "TRUE"
	case "0":
		return "FALSE"
	}
	return value
}

// columnOf reads the column out of a cell reference such as "BC12", and
// returns -1 for a reference that names no column at all.
func columnOf(reference string) int {
	column := 0
	for _, symbol := range strings.ToUpper(strings.TrimSpace(reference)) {
		if symbol < 'A' || symbol > 'Z' {
			break
		}
		column = column*26 + int(symbol-'A') + 1
	}
	if column <= 0 {
		return -1
	}
	return column - 1
}
