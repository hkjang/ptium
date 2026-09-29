package docs

import (
	"testing"
)

// A hashtag does not start a slide.
//
// readMarkdown asked only whether a line began with "#", which is not what "#"
// means in markdown: CommonMark wants a space after the hashes, and without
// that rule the "#출시 #마케팅" somebody wrote in the middle of a paragraph cut
// the paragraph in two and took the sentence under it onto a slide of its own.
// The hashtag line belongs where it was written, as the point it is — the
// leading "#" is already protected by escapeLine, which is why the expectation
// below reads "\#출시".
func TestAHashtagDoesNotStartASlide(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"# 분기 요약\n\n매출이 늘었습니다.\n#출시 #마케팅\n비용은 줄었습니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 분기 요약\n@cover\n> 월간 보고서.md\n\n" +
		"# 분기 요약\n- 매출이 늘었습니다.\n- \\#출시 #마케팅\n- 비용은 줄었습니다.\n" +
		"!source 월간 보고서.md | 분기 요약\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
}

// A sentence that opens with a numbered reference keeps its words.
//
// This is the expensive half of the same bug rather than another one. A heading
// is the one line of a document whose text does not become a point: it names
// the slide and the sentence itself is gone from the body. So "#1 우선순위는
// 출시입니다." — the line that says what the whole memo is about — left behind a
// slide with a title and no points at all. Nothing else in this reader can
// delete a sentence that way, which is why the test asserts the sentence is
// still there as a point of the slide it was written on.
func TestASentenceStartingWithAHashNumberKeepsItsWords(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"# 분기 요약\n\n매출이 늘었습니다.\n#1 우선순위는 출시입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 분기 요약\n@cover\n> 월간 보고서.md\n\n" +
		"# 분기 요약\n- 매출이 늘었습니다.\n- \\#1 우선순위는 출시입니다.\n" +
		"!source 월간 보고서.md | 분기 요약\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
}

// Markdown counts to six. A run of seven or more hashes is not a heading at any
// level, so it is a line of the document like any other — and a row of hashes is
// how people draw a divider in a plain .txt file, which this same reader reads.
func TestSevenHashesAreNotAHeading(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"# 분기 요약\n\n####### 일곱개\n매출이 늘었습니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 분기 요약\n@cover\n> 월간 보고서.md\n\n" +
		"# 분기 요약\n- \\####### 일곱개\n- 매출이 늘었습니다.\n" +
		"!source 월간 보고서.md | 분기 요약\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
}

// Every heading that was a heading still is one, at all six levels markdown
// allows. Narrowing a rule is how a fix for one document stops reading another,
// so the levels are spelled out rather than sampled.
func TestEveryLevelOfHashHeadingStillStartsASlide(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"# 하나\n문장 하나.\n## 둘\n문장 둘.\n### 셋\n문장 셋.\n"+
			"#### 넷\n문장 넷.\n##### 다섯\n문장 다섯.\n###### 여섯\n문장 여섯.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 하나\n@cover\n> 월간 보고서.md\n\n" +
		"# 하나\n- 문장 하나.\n!source 월간 보고서.md | 하나\n\n" +
		"# 둘\n- 문장 둘.\n!source 월간 보고서.md | 둘\n\n" +
		"# 셋\n- 문장 셋.\n!source 월간 보고서.md | 셋\n\n" +
		"# 넷\n- 문장 넷.\n!source 월간 보고서.md | 넷\n\n" +
		"# 다섯\n- 문장 다섯.\n!source 월간 보고서.md | 다섯\n\n" +
		"# 여섯\n- 문장 여섯.\n!source 월간 보고서.md | 여섯\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
}

// A "#" with nothing after it is markdown's empty heading, and so is a "#" with
// only spaces after it — this reader trims the line before it looks at it, so
// the two are the same line by the time they arrive. Both were headings before
// the space rule and both stay headings after it: the rule is about what
// follows the hashes, and the end of the line follows them just as a space does.
//
// What an empty heading produces is odd — writer.flush puts the deck's own name
// where the missing heading was and the slide cites the file without a locator —
// but odd and unchanged is the point of this test, so the strings below are what
// this reader produced before the space rule, to the character.
func TestAnEmptyHeadingReadsExactlyAsItDidBefore(t *testing.T) {
	for _, document := range []struct{ input, expected string }{
		{"# 제목\n\n문장 하나.\n\n#\n\n문장 둘.\n",
			"# 제목\n@cover\n> 월간 보고서.md\n\n" +
				"# 제목\n- 문장 하나.\n!source 월간 보고서.md | 제목\n\n" +
				"# 제목\n- 문장 둘.\n!source 월간 보고서.md\n\n"},
		{"# 제목\n\n문장 하나.\n\n#   \n\n문장 둘.\n",
			"# 제목\n@cover\n> 월간 보고서.md\n\n" +
				"# 제목\n- 문장 하나.\n!source 월간 보고서.md | 제목\n\n" +
				"# 제목\n- 문장 둘.\n!source 월간 보고서.md\n\n"},
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

// A fence that never closes gives its lines back to the document, and they are
// read by the same rules as the rest of the file. The space rule has to reach
// that replay too, or one stray ``` above a paragraph would decide whether the
// hashtag in it opens a slide.
func TestAReplayedUnclosedFenceStillRefusesAHashtag(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"# 첫째 장\n\n```\n\n매출이 늘었습니다.\n#출시 #마케팅\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 첫째 장\n@cover\n> 월간 보고서.md\n\n" +
		"# 첫째 장\n- ```\n- 매출이 늘었습니다.\n- \\#출시 #마케팅\n" +
		"!source 월간 보고서.md | 첫째 장\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
	if len(document.Warnings) != 0 {
		t.Errorf("warned %q about a block the document never opened", document.Warnings)
	}
}
