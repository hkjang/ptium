package docs

import (
	"strings"
	"testing"
)

// A horizontal rule is punctuation, so it has nothing to put on a slide.
//
// readMarkdown had no branch for one, so a "---" between two paragraphs fell
// through to writer.point and became a bullet — and escapeLine, which protects a
// line opening with "-" from being read as a directive, put a backslash in front
// of it. The reader's own output is what the author then saw on the slide:
// "\---", a glyph nobody typed, sitting between the two sentences the rule was
// drawn to separate.
func TestAHorizontalRuleIsNotAPoint(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"# 월간 보고서\n매출이 늘었습니다.\n\n---\n\n비용은 줄었습니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 월간 보고서\n@cover\n\n" +
		"# 월간 보고서\n- 매출이 늘었습니다.\n- 비용은 줄었습니다.\n" +
		"!source 월간 보고서.md | 월간 보고서\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
}

// A document that opens with YAML front matter opens with a rule, and the two
// rules fencing it are the two lines of it this reader can say anything about.
// They come off for the same reason every other rule does; what is between them
// is a separate question this reader still answers wrongly, and the expectation
// below keeps that answer where it is rather than pretending otherwise.
func TestTheRulesAroundFrontMatterAreNotPoints(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"---\ntitle: 보고서\n---\n\n# 분기 요약\n문장입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 월간 보고서\n@cover\n\n" +
		"# 월간 보고서\n- title: 보고서\n!source 월간 보고서.md\n\n" +
		"# 분기 요약\n- 문장입니다.\n!source 월간 보고서.md | 분기 요약\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
}

// Markdown draws the same rule five ways, and a reader that knows only the one
// the author of the last document happened to use is a reader that works by
// luck. Each shape is read against the document with no rule in it at all,
// because that — and not a string somebody typed twice — is what dropping a
// rule has to mean: the slide is the slide the document would have had.
func TestEveryShapeOfHorizontalRuleDropsOut(t *testing.T) {
	plain, err := Read("월간 보고서.md", []byte("# 제목\n\n한 문장입니다.\n\n끝입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{"---", "***", "___", "- - -", "* * *",
		"-----", "   ---   ", "- -\t-"} {
		document, err := Read("월간 보고서.md", []byte(
			"# 제목\n\n한 문장입니다.\n\n"+rule+"\n\n끝입니다.\n"))
		if err != nil {
			t.Fatal(err)
		}
		if document.Source != plain.Source {
			t.Errorf("read %q as\n%s\nwant\n%s", rule, document.Source, plain.Source)
		}
	}
}

// A rule separates; it does not announce. Nothing of the document fails to
// arrive when one comes off, so unlike a picture or a code block there is
// nothing for the reader to say — and a deck that reports "2 rules removed"
// would be telling the author about the reader rather than about the document.
func TestAHorizontalRuleIsNotWarnedAbout(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"# 제목\n\n한 문장입니다.\n\n---\n\n끝입니다.\n\n***\n\n정말 끝입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Warnings) != 0 {
		t.Errorf("warned %q about rules that lost nothing", document.Warnings)
	}
}

// What a rule is has to be answered as narrowly as what a heading is, because
// the lines it is nearly mistaken for are lines carrying words. Two hyphens are
// a dash somebody typed, "-5% 감소" is a point about a fall, and asterisks
// around a word are emphasis — on the slide and in the source of the deck. The
// strings below are what this reader produced before it knew about rules, to
// the character: none of these lines may lose so much as a backslash.
func TestWhatIsNotAHorizontalRuleStaysAPoint(t *testing.T) {
	for _, document := range []struct{ input, expected string }{
		{"# 제목\n\n--\n", "# 제목\n@cover\n> 월간 보고서.md\n\n" +
			"# 제목\n- \\--\n!source 월간 보고서.md | 제목\n\n"},
		{"# 제목\n\n-5% 감소\n", "# 제목\n@cover\n> 월간 보고서.md\n\n" +
			"# 제목\n- \\-5% 감소\n!source 월간 보고서.md | 제목\n\n"},
		{"# 제목\n\n***중요***\n", "# 제목\n@cover\n> 월간 보고서.md\n\n" +
			"# 제목\n- \\***중요***\n!source 월간 보고서.md | 제목\n\n"},
		{"# 제목\n\n*** 중요 ***\n", "# 제목\n@cover\n> 월간 보고서.md\n\n" +
			"# 제목\n- \\*** 중요 ***\n!source 월간 보고서.md | 제목\n\n"},
		{"# 제목\n\n한 문장입니다.\n", "# 제목\n@cover\n> 월간 보고서.md\n\n" +
			"# 제목\n- 한 문장입니다.\n!source 월간 보고서.md | 제목\n\n"},
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

// A rule ends a block at least as firmly as a blank line does, and the only
// block this reader carries across lines is a table. Two tables drawn with a
// rule between them are two tables, the same as two tables with a blank line
// between them — reading the rule as nothing at all would have run them into
// one grid whose fourth row is another table's header.
func TestARuleBetweenTwoTablesKeepsThemTwoTables(t *testing.T) {
	ruled, err := Read("월간 보고서.md", []byte(
		"# 제목\n| 항목 | 값 |\n|---|---|\n| 매출 | 100 |\n---\n| 항목 | 값 |\n|---|---|\n| 비용 | 50 |\n"))
	if err != nil {
		t.Fatal(err)
	}
	blank, err := Read("월간 보고서.md", []byte(
		"# 제목\n| 항목 | 값 |\n|---|---|\n| 매출 | 100 |\n\n| 항목 | 값 |\n|---|---|\n| 비용 | 50 |\n"))
	if err != nil {
		t.Fatal(err)
	}
	if ruled.Source != blank.Source {
		t.Errorf("read as\n%s\nwant\n%s", ruled.Source, blank.Source)
	}
}

// Inside a fenced code block a line is not markdown, and a row of hyphens is
// how a snippet draws the top of a table or the end of a section. It stays in
// the block, counted and said, rather than being read as punctuation of the
// document.
func TestAHorizontalRuleInsideACodeBlockIsStillCode(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"# 제목\n\n한 문장입니다.\n\n```\n항목\n---\n값\n```\n\n끝입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 제목\n@cover\n> 월간 보고서.md\n\n" +
		"# 제목\n- 한 문장입니다.\n- 끝입니다.\n!source 월간 보고서.md | 제목\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
	if len(document.Warnings) != 1 || !strings.Contains(document.Warnings[0], "코드 블록 1개(3줄)") {
		t.Errorf("warned %q, want the one block of three lines", document.Warnings)
	}
}

// A fence that never closes gives its lines back to the document, read by the
// same rules as the rest of the file. One stray ``` must not be what decides
// whether the rules below it are punctuation.
func TestAReplayedUnclosedFenceStillDropsAHorizontalRule(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"# 첫째 장\n\n```\n\n한 문장입니다.\n\n---\n\n끝입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 첫째 장\n@cover\n> 월간 보고서.md\n\n" +
		"# 첫째 장\n- ```\n- 한 문장입니다.\n- 끝입니다.\n!source 월간 보고서.md | 첫째 장\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
	if len(document.Warnings) != 0 {
		t.Errorf("warned %q about a block the document never opened", document.Warnings)
	}
}
