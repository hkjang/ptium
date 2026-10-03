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
	// handle reads one line, and is told the line after it because a sentence
	// with a row of "=" under it is a heading rather than a sentence. It returns
	// whether it took that next line with it, so the caller does not read the
	// underline as a line of its own.
	handle := func(line, next string) bool {
		if heading, ok := atxHeading(line); ok {
			flush()
			writer.slide(heading)
			return false
		}
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "|"):
			cells := strings.Split(strings.Trim(line, "|"), "|")
			for index := range cells {
				cells[index] = strings.TrimSpace(cells[index])
			}
			// The |---|---| rule under a header row is punctuation, not a row.
			if isRule(cells) {
				return false
			}
			table = append(table, cells)
		case isThematicBreak(line):
			// A rule separates; it has nothing to put on a slide. flush() is here
			// because a rule ends a block at least as firmly as a blank line does,
			// and the one block this reader carries across lines is a table: two
			// tables drawn with a rule between them must not run into one grid
			// whose fourth row is another table's header.
			//
			// This case comes before the list one because "- - -" is a rule drawn
			// with the character a list is drawn with, and withoutListMarker would
			// take the leading "- " off it and leave "- -" as a point.
			flush()
		case isListLine(line):
			flush()
			point, _ := withoutListMarker(line)
			writer.point(point)
		default:
			flush()
			if underlinesHeading(next) {
				writer.slide(line)
				return true
			}
			writer.point(line)
		}
		return false
	}
	// Inside a fenced code block, a line is not markdown.
	//
	// Reading every line the same way turned a runbook's one page into several
	// slides nobody wrote: the "# 1단계" of a shell comment started a slide, the
	// ``` lines stayed as bullets, and the sentence after the block landed on the
	// slide the last line of code had made. So the block is read as the one thing
	// it is, and counted, because a deck has nothing that draws code.
	var fence string
	var held []string
	blocks, skipped := 0, 0
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	// skip says the line coming up is the underline of the heading just written,
	// which the heading has already accounted for.
	skip := false
	for index, raw := range lines {
		line := strings.TrimSpace(raw)
		if fence == "" {
			if skip {
				skip = false
				continue
			}
			if opened := fenceOf(line); opened != "" {
				flush()
				fence = opened
				// The opening line is held with the block: if the fence turns out
				// never to close, it is a line of the document again.
				held = []string{line}
				continue
			}
			skip = handle(line, lineAfter(lines, index))
			continue
		}
		held = append(held, line)
		if closesFence(line, fence) {
			blocks++
			// The two fence lines are punctuation; what did not arrive is the
			// lines between them.
			skipped += len(held) - 2
			fence, held = "", nil
		}
	}
	if fence != "" {
		// A fence that never closes is a stray ``` somebody typed, not a block
		// that runs to the end of the file. Swallowing the rest of the document
		// on the strength of one line would lose every slide after it, so the
		// held lines are read as the lines they are — which is what this reader
		// did before it knew about fences at all.
		//
		// They are read by the same rules as the rest of the file, underlines and
		// all: a stray ``` above a heading must not change what that heading is.
		skip = false
		for index, line := range held {
			if skip {
				skip = false
				continue
			}
			skip = handle(line, lineAfter(held, index))
		}
	}
	flush()
	document, err := writer.document()
	if err != nil {
		return document, err
	}
	// Said rather than dropped, the way the Word reader says it about pictures:
	// the deck has no component that draws code, so what the block held cannot
	// arrive, and a deck that quietly comes back without the commands somebody
	// wrote their runbook around is worse than one that says so.
	//
	// The line count is there because the block count alone does not tell anybody
	// how much of their document is missing — one block is a three-line snippet
	// or forty lines of a file, and only the second is worth reopening the source
	// over. It also puts a number on the one case this reader can still get
	// wrong: if some line were ever read as a fence that the author did not mean
	// as one, the warning says how many lines went with it instead of implying a
	// snippet.
	if blocks > 0 {
		document.Warnings = append(document.Warnings, fmt.Sprintf(
			"코드 블록 %d개(%d줄)는 가져오지 않았습니다. 슬라이드에 필요한 줄은 요점으로 적어 주세요",
			blocks, skipped))
	}
	return document, nil
}

// markdownFences are the two characters a code fence is drawn with.
const markdownFences = "`~"

// fenceOf reads a line as the opening of a fenced code block and returns the
// fence it opened with — three or more of one of the two characters. A line that
// opens nothing returns "".
//
// An indented fence is read as a fence: the caller has already trimmed the line,
// which is more generous than markdown's three spaces and harms nothing, since a
// deck holds no code at any indent.
//
// What follows the fence is the info string, and a fence that opens a block
// names a language there or nothing at all — ```bash, ~~~, ```. A line that
// carries a sentence after the fence characters is a sentence *about* fences,
// which is exactly what the page of a guide explaining markdown looks like:
//
//	``` 로 감싸면 코드 블록이 됩니다.
//
// Opening a block on that line costs far more than missing one. Missing a fence
// leaves the fence line as a bullet — what this reader did before it knew about
// fences at all — while opening one the author never opened swallows every
// paragraph after it until some later ``` closes it, and those paragraphs are
// then dropped from the deck as code. So the info string has to be one word:
// a ```js {1,3} that some site's renderer accepts reads as it used to instead,
// which is the cheap half of the trade. A backtick in the info string is
// refused for the same reason and on markdown's own authority — CommonMark
// says a backtick fence whose info string holds a backtick opens nothing —
// which is what catches the same sentence written ``` … ``` on one line.
func fenceOf(line string) string {
	if line == "" || strings.IndexByte(markdownFences, line[0]) < 0 {
		return ""
	}
	fence := line[:len(line)-len(strings.TrimLeft(line, line[:1]))]
	if len(fence) < 3 {
		return ""
	}
	if info := line[len(fence):]; strings.ContainsAny(info, " \t`") {
		return ""
	}
	return fence
}

// closesFence says whether a line ends the block a fence opened. The line that
// opens a block may name the language after it — ```bash — but the line that
// ends one is the fence and nothing else, at least as long as the fence that
// opened the block. So a "```python" in the middle of a shell block is a line of
// that block rather than the end of it.
func closesFence(line, fence string) bool {
	return len(line) >= len(fence) && strings.Trim(line, fence[:1]) == ""
}

// lineAfter is the line following the one at index, trimmed the same way, and
// "" where there is no next line — which is also what the last line of a file
// has under it as far as a heading is concerned.
func lineAfter(lines []string, index int) string {
	if index+1 >= len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[index+1])
}

// atxHeading reads a line as markdown's "#" heading and returns the text of it.
// A line that is not a heading returns ok false and belongs to the document as
// the line it is.
//
// Two things make a heading here, and this reader used to ask for neither. The
// first is a space after the hashes. Without it, "#" at the start of a line
// means whatever the author meant by it, and in a Korean memo that is nearly
// always a hashtag — "#출시 #마케팅" under a paragraph cut the paragraph in two
// and took the sentence below it onto a slide named after the tags. Worse is
// the sentence that opens with a numbered reference: a heading is the one line
// whose text does not also become a point, so "#1 우선순위는 출시입니다." left a
// slide with that title and nothing under it, and the sentence saying what the
// memo was about was gone from the deck. Nothing else in this reader can lose a
// sentence that way. The same reader takes .txt, where a "#" opening a line is
// not markup at all.
//
// The second is that markdown counts to six. A run of seven or more is not a
// heading at any level, and a row of hashes is how a plain text file draws a
// divider, so "#######" is a line rather than a slide called by whatever came
// after it.
//
// Requiring the space costs the author who writes "#제목" and means it. That is
// the cheap side of the trade: their heading stays as a point of the slide it
// was written on, with every word of it, which is what a missed heading has
// always cost this reader — while reading a hashtag as a heading splits the
// document and deletes a sentence.
//
// The hashes at the end of a closed heading — "## 분기 요약 ##" — are punctuation
// too, and come off for a reason this reader feels further than most: the first
// heading of a document becomes the deck's name, so a title written closed filed
// the deck under "분기 요약 ##" and put that locator after the file on every
// slide's citation. A space in front of them is what makes them punctuation, as
// CommonMark has it, and the space is worth insisting on rather than trimming
// every trailing hash: "# C# 도입" is a heading naming a language after a hash,
// and "# 제목#" is a word the author spelled with one. Neither has a space
// there, so neither loses a character.
func atxHeading(line string) (string, bool) {
	hashes := len(line) - len(strings.TrimLeft(line, "#"))
	if hashes < 1 || hashes > 6 {
		return "", false
	}
	// The end of the line follows the hashes as well as a space does: "#" alone
	// is markdown's empty heading, and this reader has always read it as one.
	rest := line[hashes:]
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return "", false
	}
	text := strings.TrimSpace(rest)
	// Nothing but hashes left is markdown's empty heading as well: the whole of
	// "## ###" after the opening hashes is a closing sequence, and an opening
	// sequence needs no space in front of it.
	if closing := strings.TrimRight(text, "#"); closing != text &&
		(closing == "" || closing[len(closing)-1] == ' ' || closing[len(closing)-1] == '\t') {
		text = strings.TrimSpace(closing)
	}
	return text, true
}

// underlinesHeading says whether a line makes a heading of the line above it.
//
// A row of "=" is markdown's other way of writing a heading, and the way a
// report writes its own name on its first line — the title over the underline is
// the "# 분기 요약" the rest of this reader already knows. Not reading it cost
// that title twice: it stayed behind as a bullet and the underline became a
// second bullet of "=========", and since no heading had been seen by then, the
// deck was named after the file rather than after the document. A document whose
// sections are written that way also never started a new slide, so its
// paragraphs piled onto one until maximumPoints spilled them onto a "(계속)".
//
// One "=" is enough, as CommonMark has it. Nothing else in markdown begins a
// line with that character, so unlike the code fence — where a sentence
// *about* fences begins with one — there is no line this can be mistaken for.
//
// This is asked of the line ahead rather than answered by taking back the line
// behind, because the line behind cannot be taken back: writer.point may have
// filled the slide at maximumPoints and opened a "(계속)" to hold the overflow,
// and there is no undoing that once the points have moved.
//
// The other underline markdown allows, a row of "-", is deliberately not read
// here. That character is already a list marker, a thematic break, and the fence
// of YAML front matter, so folding it in would make the "title: 보고서" under a
// front matter "---" the name of a slide. CommonMark does read it as a heading;
// for this reader it would lose more than it found.
func underlinesHeading(line string) bool {
	return line != "" && strings.Trim(line, "=") == ""
}

// markdownBreaks are the three characters a thematic break is drawn with.
const markdownBreaks = "-_*"

// isThematicBreak says whether a line is a horizontal rule.
//
// A rule is the one line of a markdown document with no content in it at all.
// It is not a sentence the reader cannot render, the way a code block is; it is
// punctuation between two things that are rendered, so there is nothing to put
// on a slide and nothing to warn about either — saying "2 rules removed" would
// tell the author about the reader rather than about the document.
//
// Leaving it as a point cost more than a stray bullet. escapeLine protects a
// line opening with "-" or "*" from being read as a directive, so the author
// who drew a rule between two paragraphs got "\---" on the slide: a backslash
// nobody typed, in the gap the rule was drawn to leave empty. A document opening
// with YAML front matter got two of them around its "title:" line, on the first
// slide of the deck.
//
// What counts is CommonMark's rule, and it is worth keeping that narrow, because
// every line this is nearly mistaken for carries words: three or more of one of
// the three characters, with nothing between them but spaces and tabs. Two
// hyphens are a dash somebody typed, "-5% 감소" is a point about a fall, and
// "***중요***" is a word with emphasis around it. The caller has already trimmed
// the line, which is more generous about leading space than markdown's three,
// and harms nothing: a rule at any indent is still a rule with nothing in it.
func isThematicBreak(line string) bool {
	if line == "" || !strings.ContainsRune(markdownBreaks, rune(line[0])) {
		return false
	}
	mark, marks := line[0], 0
	for index := 0; index < len(line); index++ {
		switch line[index] {
		case mark:
			marks++
		case ' ', '\t':
		default:
			return false
		}
	}
	return marks >= 3
}

func isRule(cells []string) bool {
	for _, cell := range cells {
		if strings.Trim(cell, "-: ") != "" {
			return false
		}
	}
	return len(cells) > 0
}
