package docs

import (
	"strings"
	"testing"
)

// A title underlined with "=" is a title.
//
// readMarkdown knew only the "#" heading, so a document whose first line is its
// name written the other way markdown allows lost that name twice over: the deck
// was called after the file instead of after the document, and the row of "="
// stayed behind as a bullet of its own. The two ways of writing the same heading
// have to read as the same document, so the test reads both rather than spelling
// one of them out — a copied expectation only pins down what somebody typed
// twice.
func TestASetextHeadingReadsLikeAHashHeading(t *testing.T) {
	underlined, err := Read("월간 보고서.md", []byte("분기 요약\n=========\n\n매출이 늘었습니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	hashed, err := Read("월간 보고서.md", []byte("# 분기 요약\n\n매출이 늘었습니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if underlined.Source != hashed.Source {
		t.Errorf("read as\n%s\nwant\n%s", underlined.Source, hashed.Source)
	}
	if underlined.Title != hashed.Title {
		t.Errorf("called the deck %q, want %q — the document's own title", underlined.Title, hashed.Title)
	}
}

// A heading in the middle of a document starts a slide there, the same as a "#"
// would: without that, the sections of a long document pile onto one slide until
// maximumPoints spills the rest onto a "(계속)" nobody wrote.
func TestASetextHeadingInTheMiddleStartsItsOwnSlide(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"분기 요약\n=========\n\n매출이 늘었습니다.\n\n다음 분기\n====\n\n비용을 줄입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 분기 요약\n@cover\n> 월간 보고서.md\n\n" +
		"# 분기 요약\n- 매출이 늘었습니다.\n!source 월간 보고서.md | 분기 요약\n\n" +
		"# 다음 분기\n- 비용을 줄입니다.\n!source 월간 보고서.md | 다음 분기\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
}

// An underline underlines something. A row of "=" with no sentence over it —
// the first line of a file, or the first line after a blank one — underlines
// nothing, so it is read as the line it is, exactly as this reader read it
// before it knew about setext headings. The strings below are what it produced
// then, to the character.
func TestAnUnderlineWithNothingAboveItStaysAPoint(t *testing.T) {
	for _, document := range []struct{ input, expected string }{
		{"=====\n\n한 문장입니다.\n",
			"# 월간 보고서\n@cover\n\n" +
				"# 월간 보고서\n- =====\n- 한 문장입니다.\n!source 월간 보고서.md\n\n"},
		{"# 제목\n\n=====\n\n한 문장입니다.\n",
			"# 제목\n@cover\n> 월간 보고서.md\n\n" +
				"# 제목\n- =====\n- 한 문장입니다.\n!source 월간 보고서.md | 제목\n\n"},
	} {
		read, err := Read("월간 보고서.md", []byte(document.input))
		if err != nil {
			t.Fatal(err)
		}
		if read.Source != document.expected {
			t.Errorf("read %q as\n%s\nwant\n%s", document.input, read.Source, document.expected)
		}
	}
}

// The underline is the line under the heading, with nothing in between: a blank
// line after a sentence ends the paragraph, and markdown does not reach back
// over it. A document that leaves a row of "=" on its own line that way keeps
// both lines as points.
func TestABlankLineBeforeAnUnderlineLeavesNoHeading(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte("# 보고서\n\n분기 요약\n\n=====\n\n매출이 늘었습니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 보고서\n@cover\n> 월간 보고서.md\n\n" +
		"# 보고서\n- 분기 요약\n- =====\n- 매출이 늘었습니다.\n!source 월간 보고서.md | 보고서\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
}

// Inside a fenced code block a line is not markdown, and that did not stop
// being true when this reader learned to read underlines: the "=====" of an
// ASCII table in a snippet neither starts a slide nor takes the line above it
// out of the block, and the block is still counted and said.
func TestAnUnderlineInsideACodeBlockIsStillCode(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"# 제목\n\n한 문장입니다.\n\n```\n분기 요약\n=====\n```\n\n끝입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 제목\n@cover\n> 월간 보고서.md\n\n" +
		"# 제목\n- 한 문장입니다.\n- 끝입니다.\n!source 월간 보고서.md | 제목\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
	if len(document.Warnings) != 1 || !strings.Contains(document.Warnings[0], "코드 블록 1개(2줄)") {
		t.Errorf("warned %q, want the one block of two lines", document.Warnings)
	}
}

// A fence that never closes gives its lines back to the document, and they are
// read as the lines they are — including a heading written with an underline.
// Reading the replayed lines by any other rule than the rest of the file would
// mean one stray ``` changes what the headings under it are.
func TestAReplayedUnclosedFenceStillReadsASetextHeading(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"# 첫째 장\n\n```\n\n둘째 장\n=====\n\n둘째 문장입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 첫째 장\n@cover\n> 월간 보고서.md\n\n" +
		"# 첫째 장\n- ```\n!source 월간 보고서.md | 첫째 장\n\n" +
		"# 둘째 장\n- 둘째 문장입니다.\n!source 월간 보고서.md | 둘째 장\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
	if len(document.Warnings) != 0 {
		t.Errorf("warned %q about a block the document never opened", document.Warnings)
	}
}
