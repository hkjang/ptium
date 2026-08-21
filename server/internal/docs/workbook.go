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
	Sheets struct {
		Sheet []struct {
			Name string `xml:"name,attr"`
			ID   string `xml:"id,attr"`
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
	Rows []struct {
		Reference string `xml:"r,attr"`
		Cells     []struct {
			Reference string `xml:"r,attr"`
			Type      string `xml:"t,attr"`
			Value     string `xml:"v"`
			Inline    string `xml:"is>t"`
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
	for _, sheet := range index.Sheets.Sheet {
		if written >= maximumSlides {
			warnings = append(warnings, fmt.Sprintf("시트가 많아 앞 %d개만 가져왔습니다", maximumSlides))
			break
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
		rows := gridOf(parsed, shared)
		count, sheetWarnings := writeSheet(&builder, filename, sheet.Name, rows)
		written += count
		warnings = append(warnings, sheetWarnings...)
	}
	if written == 0 {
		return Document{}, fmt.Errorf("이 통합 문서에는 읽을 표가 없습니다")
	}
	document.Source = builder.String()
	document.Warnings = warnings
	return document, nil
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
func gridOf(sheet worksheet, shared []string) [][]string {
	grid := make([][]string, 0, len(sheet.Rows))
	for _, row := range sheet.Rows {
		cells := map[int]string{}
		widest := -1
		for _, cell := range row.Cells {
			column := columnOf(cell.Reference)
			value := cell.Value
			switch cell.Type {
			case "s":
				if index, err := strconv.Atoi(strings.TrimSpace(cell.Value)); err == nil && index >= 0 && index < len(shared) {
					value = shared[index]
				}
			case "inlineStr":
				value = cell.Inline
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

// columnOf reads the column out of a cell reference such as "BC12".
func columnOf(reference string) int {
	column := 0
	for _, symbol := range strings.ToUpper(strings.TrimSpace(reference)) {
		if symbol < 'A' || symbol > 'Z' {
			break
		}
		column = column*26 + int(symbol-'A') + 1
	}
	if column <= 0 {
		return 0
	}
	return column - 1
}
