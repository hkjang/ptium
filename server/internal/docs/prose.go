package docs

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// A report is already written. What a deck needs from it is its structure: the
// headings are the slides, the sentences under them are the points, and the
// tables are tables. Everything else a word processor keeps — styles, comments,
// revision marks, the picture of the office — is not what the deck is made of.

type wordDocument struct {
	Body wordBlocks `xml:"body"`
}

// wordBlocks is a run of blocks: the body's, or a wrapper's. See blocksIn.
type wordBlocks struct {
	Content []wordBlock `xml:",any"`
}

type wordBlock struct {
	XMLName xml.Name
	// encoding/xml cannot reach an attribute through a path, so the paragraph's
	// properties are read as the element they are.
	Properties struct {
		Style struct {
			Value string `xml:"val,attr"`
		} `xml:"pStyle"`
	} `xml:"pPr"`
	// The block's own XML. A paragraph's text is not all in runs directly under
	// it — a link, a tracked insertion and a content control each hold their
	// runs a level down (see textIn) — and a table's rows are not all directly
	// under it either (see tableRows).
	Inner []byte `xml:",innerxml"`
}

// readWordDocument reads a .docx into slides.
func readWordDocument(filename string, data []byte) (Document, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Document{}, fmt.Errorf("이 파일은 워드 문서가 아닙니다")
	}
	var content []byte
	for _, file := range archive.File {
		if file.Name != "word/document.xml" {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			break
		}
		content, _ = io.ReadAll(io.LimitReader(opened, 32<<20))
		opened.Close()
	}
	if len(content) == 0 {
		return Document{}, fmt.Errorf("이 워드 문서에서 본문을 찾지 못했습니다")
	}
	var parsed wordDocument
	if err := xml.Unmarshal(content, &parsed); err != nil {
		return Document{}, fmt.Errorf("이 워드 문서를 읽지 못했습니다")
	}

	writer := newDeckWriter(filename, titleOf(filename))
	for _, block := range blocksIn(parsed.Body.Content, 0) {
		switch block.XMLName.Local {
		case "p":
			text := strings.TrimSpace(textIn(block.Inner))
			if text == "" {
				continue
			}
			if headingLevel(block.Properties.Style.Value) > 0 {
				writer.slide(text)
				continue
			}
			writer.point(text)
		case "tbl":
			writer.table(tableRows(block))
		}
	}
	document, err := writer.document()
	if err != nil {
		return document, err
	}
	// A picture in the file is not read — the reader takes words and tables —
	// and a deck that quietly comes back without the photograph somebody put in
	// their report is worse than one that says so. The presentation reader says
	// the same thing in the same words.
	if drawings := picturesIn(content); drawings > 0 {
		document.Warnings = append(document.Warnings, fmt.Sprintf(
			"그림 %d개는 가져오지 않았습니다. 이미지 탭에서 올려 다시 넣어 주세요", drawings))
	}
	return document, nil
}

// A block is not always where it looks it is.
//
// Word wraps whole runs of a document in a content control — the cover page of a
// template, a table of contents, the answered part of a form — and the
// paragraphs and tables inside one are the document's own, not the wrapper's.
// Reading only the blocks directly under the body passed over every one of them:
// a report whose text was inside a content control came back as a deck with a
// title and nothing else, and nothing was said about the rest.
var wordWrappers = map[string]bool{"sdt": true, "sdtContent": true, "customXml": true}

// wordNesting is how far the reader follows wrappers into one another. Word
// nests a few; a file that nests more than this is not one somebody wrote.
const wordNesting = 12

// blocksIn reads a run of blocks with the wrappers around them opened, so that a
// paragraph is read wherever the document put it.
func blocksIn(blocks []wordBlock, depth int) []wordBlock {
	opened := make([]wordBlock, 0, len(blocks))
	for _, block := range blocks {
		if !wordWrappers[block.XMLName.Local] {
			opened = append(opened, block)
			continue
		}
		if depth >= wordNesting {
			continue
		}
		opened = append(opened, blocksInside(block.Inner, depth+1)...)
	}
	return opened
}

// blocksInside reads a block's own XML as the run of blocks it holds, with the
// wrappers around them opened. The XML is a fragment, so it is given a root to
// hang from first.
func blocksInside(inner []byte, depth int) []wordBlock {
	var inside wordBlocks
	fragment := append(append([]byte("<blocks>"), inner...), "</blocks>"...)
	if err := xml.Unmarshal(fragment, &inside); err != nil {
		return nil
	}
	return blocksIn(inside.Content, depth)
}

// A table is wrapped the same way the body is.
//
// The rows of a repeating section are held by a w:sdt inside the table, and a
// cell somebody filled in on a form holds its paragraph inside one too. Reading
// only what is directly under each level lost both: a wrapped cell came back
// empty, and a table whose rows were all in a repeating section was left with
// its header alone and dropped for being too short to be a table.
//
// A table inside a cell is passed over here as it was before: its paragraphs
// are not this table's cells.
func tableRows(table wordBlock) [][]string {
	rows := make([][]string, 0, 8)
	for _, row := range blocksInside(table.Inner, 0) {
		if row.XMLName.Local != "tr" {
			continue
		}
		cells := make([]string, 0, 4)
		for _, cell := range blocksInside(row.Inner, 0) {
			if cell.XMLName.Local != "tc" {
				continue
			}
			parts := make([]string, 0, 2)
			for _, paragraph := range blocksInside(cell.Inner, 0) {
				if paragraph.XMLName.Local != "p" {
					continue
				}
				parts = append(parts, textIn(paragraph.Inner))
			}
			cells = append(cells, strings.TrimSpace(strings.Join(parts, " ")))
		}
		rows = append(rows, cells)
	}
	return rows
}

// picturesIn counts the pictures a Word document draws, in either of the two
// ways it writes them: the drawing a modern Word writes, and the shape that
// older files and some exporters still carry.
func picturesIn(content []byte) int {
	return bytes.Count(content, []byte("<w:drawing")) + bytes.Count(content, []byte("<w:pict"))
}

// headingLevel reads a paragraph's style. Word writes "Heading1" in English and
// "1" in some localisations, and a Korean install writes "제목 1".
func headingLevel(style string) int {
	lowered := strings.ToLower(strings.TrimSpace(style))
	switch {
	case lowered == "":
		return 0
	case strings.HasPrefix(lowered, "heading"), strings.HasPrefix(lowered, "제목"),
		strings.HasPrefix(lowered, "title"), strings.HasPrefix(lowered, "見出し"), strings.HasPrefix(lowered, "标题"):
		for _, symbol := range lowered {
			if symbol >= '1' && symbol <= '9' {
				return int(symbol - '0')
			}
		}
		return 1
	}
	return 0
}

// readMarkdown reads markdown or plain text into slides.
func readMarkdown(filename string, data []byte) (Document, error) {
	writer := newDeckWriter(filename, titleOf(filename))
	var table [][]string
	flush := func() {
		if len(table) > 0 {
			writer.table(table)
			table = nil
		}
	}
	for _, raw := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "#"):
			flush()
			writer.slide(strings.TrimSpace(strings.TrimLeft(line, "#")))
		case strings.HasPrefix(line, "|"):
			cells := strings.Split(strings.Trim(line, "|"), "|")
			for index := range cells {
				cells[index] = strings.TrimSpace(cells[index])
			}
			// The |---|---| rule under a header row is punctuation, not a row.
			if isRule(cells) {
				continue
			}
			table = append(table, cells)
		case isListLine(line):
			flush()
			point, _ := withoutListMarker(line)
			writer.point(point)
		default:
			flush()
			writer.point(line)
		}
	}
	flush()
	return writer.document()
}

func isRule(cells []string) bool {
	for _, cell := range cells {
		if strings.Trim(cell, "-: ") != "" {
			return false
		}
	}
	return len(cells) > 0
}
