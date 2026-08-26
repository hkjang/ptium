package docs

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hkjang/ptium/server/internal/pdftext"
)

// A PDF is the format a company sends things in: the report from the agency,
// the regulation, last year's deck after somebody exported it. It is also the
// format that keeps the least — a PDF holds glyphs at coordinates, not
// headings and points — so what can be recovered is the page's own lines, and
// what cannot be recovered is said out loud instead of guessed at.

// readPDF reads a PDF into slides, one page at a time.
func readPDF(filename string, data []byte) (Document, error) {
	pages, err := pdftext.Read(data)
	if err != nil {
		return Document{}, fmt.Errorf("이 파일을 PDF로 읽지 못했습니다")
	}
	furniture := runningLines(pages)
	writer := newDeckWriter(filename, titleOf(filename))
	said, blank := 0, 0
	heading := ""
	for _, page := range pages {
		lines := make([]string, 0, len(page.Lines))
		for _, line := range page.Lines {
			if furniture[line] || pageNumberOnly(line) {
				continue
			}
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			blank++
			continue
		}
		said++
		where := fmt.Sprintf("%d쪽", page.Number)
		if looksLikeHeading(lines[0]) {
			heading = lines[0]
			lines = lines[1:]
		} else if heading != "" {
			heading += " (계속)"
		} else {
			heading = titleOf(filename)
		}
		writer.slideFrom(heading, where)
		// A page of a report holds more lines than a slide does. Spilling them
		// into eight continuation slides turns a 127-page statement into a
		// thousand slides nobody will read; dropping them loses what the page
		// said. What is past the slide goes into its notes, where it is still
		// there to be read, exported and searched.
		for index, line := range lines {
			if index < maximumPoints {
				writer.point(line)
				continue
			}
			writer.note(line)
		}
	}
	if said == 0 {
		return Document{}, fmt.Errorf("이 PDF에는 글자가 없습니다. 스캔했거나 그림으로 내보낸 파일로 보입니다. " +
			"원본 문서나 텍스트가 살아 있는 PDF로 올려주세요")
	}
	document, err := writer.document()
	if err != nil {
		return document, err
	}
	if blank > 0 {
		document.Warnings = append(document.Warnings,
			fmt.Sprintf("%d쪽은 그림뿐이라 글자를 가져오지 못했습니다", blank))
	}
	return document, nil
}

// runningLines finds the header and footer a document repeats on every page.
//
// A regulation prints "법제처 · 국가법령정보센터" at the top of all forty pages.
// Carried into a deck, that line becomes forty slides' first point, and the
// heading of every one of them.
func runningLines(pages []pdftext.Page) map[string]bool {
	if len(pages) < 3 {
		return map[string]bool{}
	}
	seen := map[string]int{}
	shapes := map[string][]string{}
	for _, page := range pages {
		on := map[string]bool{}
		for index, line := range page.Lines {
			// Furniture lives at the edges of a page. A sentence that happens to
			// read like another sentence in the middle of a report is not a
			// header, and dropping it would lose what the page said.
			if index > 1 && index < len(page.Lines)-2 {
				continue
			}
			if len([]rune(line)) > 60 {
				continue
			}
			// The header carries the page number inside it — "법제처 12
			// 국가법령정보센터" — so the line itself is different on every page
			// and only its shape repeats.
			shape := numbersDropped.ReplaceAllString(line, "#")
			if on[shape] {
				continue
			}
			on[shape] = true
			seen[shape]++
			shapes[shape] = append(shapes[shape], line)
		}
	}
	repeated := map[string]bool{}
	for shape, count := range seen {
		if count < 3 || count*2 < len(pages) {
			continue
		}
		for _, line := range shapes[shape] {
			repeated[line] = true
		}
	}
	return repeated
}

// numbersDropped removes what changes from page to page in a running header.
var numbersDropped = regexp.MustCompile(`\d+`)

// numberedLine matches what a page prints to number itself: "12", "- 12 -",
// "12 / 40", "Page 12", "12쪽".
var numberedLine = regexp.MustCompile(`^(?i)[-–—\s]*(page|p\.|쪽)?[\s]*\d+[\s]*(/[\s]*\d+)?[\s]*(쪽|page)?[-–—\s]*$`)

func pageNumberOnly(line string) bool {
	return numberedLine.MatchString(strings.TrimSpace(line))
}

// looksLikeHeading says whether a page's first line names the page or is simply
// the first sentence on it. A heading is short and does not finish a sentence.
func looksLikeHeading(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || len([]rune(trimmed)) > 40 {
		return false
	}
	switch trimmed[len(trimmed)-1] {
	case '.', ',', ';', ':':
		return false
	}
	return !strings.HasSuffix(trimmed, "다") && !strings.HasSuffix(trimmed, "요")
}
