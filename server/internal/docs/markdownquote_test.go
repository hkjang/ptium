package docs

import (
	"strings"
	"testing"
)

// A quote is markdown's outermost block: the ">" says who is speaking, not what
// is said. This reader had no case for it, so the marker travelled all the way
// to the slide — and because escapeLine protects a line opening with ">" from
// being read as the deck DSL's cover subtitle, the author who quoted a sentence
// got "- \> 인용입니다." on it: a backslash nobody typed, in front of a marker
// that was never meant to be read aloud.
//
// The expectation is the same document with the markers taken off by hand,
// rather than a string written out here, because that is what unquoting has to
// mean: the deck is the deck the author would have got had they deleted the
// markers before uploading.
func TestAQuotedSentenceIsAPointWithoutItsMarker(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte("# 제목\n\n> 인용입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Read("월간 보고서.md", []byte("# 제목\n\n인용입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if document.Source != plain.Source {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, plain.Source)
	}
	// A marker is punctuation that says where a sentence came from, not a
	// sentence that failed to arrive — so there is nothing to report, the same as
	// for a rule or a header.
	if len(document.Warnings) != 0 {
		t.Errorf("warned %q about a marker that lost nothing", document.Warnings)
	}
}

// A quote of several lines is several points. Running them into one would need a
// block this reader does not have — the only thing it carries across lines is a
// table — and the join would then have to answer to maximumPoints, which has
// already moved earlier points onto a "(계속)" by the time the quote ends.
func TestEachLineOfAQuoteIsItsOwnPoint(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"# 제목\n\n> 첫 문장입니다.\n> 둘째 문장입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Read("월간 보고서.md", []byte(
		"# 제목\n\n첫 문장입니다.\n둘째 문장입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if document.Source != plain.Source {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, plain.Source)
	}
}

// A quote inside a quote is written both ways by the editors people use, and
// neither of them is content: every marker comes off, however many there are and
// whatever space is between them.
func TestANestedQuoteLosesEveryMarker(t *testing.T) {
	plain, err := Read("월간 보고서.md", []byte("# 제목\n\n깊은 인용\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		"# 제목\n\n>> 깊은 인용\n",
		"# 제목\n\n> > 깊은 인용\n",
		"# 제목\n\n>>> 깊은 인용\n",
	} {
		document, err := Read("월간 보고서.md", []byte(input))
		if err != nil {
			t.Fatal(err)
		}
		if document.Source != plain.Source {
			t.Errorf("read %q as\n%s\nwant\n%s", input, document.Source, plain.Source)
		}
	}
}

// Unquoting is taking the marker off and reading the line again, so whatever
// block the author wrote inside the quote is the block they get: a heading names
// the slide — and the deck, and every locator under it — a list item is a point
// without a backslash in front of it, and a row of a table is a row of that
// table rather than a sentence with bars in it.
func TestABlockInsideAQuoteIsReadAsThatBlock(t *testing.T) {
	for _, document := range []struct{ quoted, plain string }{
		{"> # 분기 요약\n> 매출이 늘었습니다.\n", "# 분기 요약\n매출이 늘었습니다.\n"},
		{"# 제목\n\n> - 항목\n", "# 제목\n\n- 항목\n"},
		{
			"# 제목\n\n> | 분기 | 매출 |\n> |---|---|\n> | 1분기 | 100 |\n",
			"# 제목\n\n| 분기 | 매출 |\n|---|---|\n| 1분기 | 100 |\n",
		},
	} {
		quoted, err := Read("월간 보고서.md", []byte(document.quoted))
		if err != nil {
			t.Fatal(err)
		}
		plain, err := Read("월간 보고서.md", []byte(document.plain))
		if err != nil {
			t.Fatal(err)
		}
		if quoted.Source != plain.Source {
			t.Errorf("read %q as\n%s\nwant\n%s", document.quoted, quoted.Source, plain.Source)
		}
	}
}

// The blank line of a quote is written as a marker with nothing after it. There
// is no sentence on it to put anywhere, and it ends a block the way a blank line
// does — which is what it is.
func TestALineOfOnlyAQuoteMarkerIsNotAPoint(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"# 제목\n\n> 첫 문장입니다.\n>\n> 둘째 문장입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Read("월간 보고서.md", []byte(
		"# 제목\n\n첫 문장입니다.\n\n둘째 문장입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if document.Source != plain.Source {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, plain.Source)
	}
}

// What a quote is, is a line that *opens* with the marker. The same character in
// the middle of a sentence is the comparison somebody wrote, and a point that
// opens with anything else is still protected by escapeLine — including the ">"
// the Word and PDF readers can leave at the front of a paragraph, which is why
// the fix is here rather than in the escaping.
func TestAQuoteMarkerInsideASentenceIsNotAMarker(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte("# 제목\n\n매출 > 목표\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 제목\n@cover\n> 월간 보고서.md\n\n" +
		"# 제목\n- 매출 > 목표\n!source 월간 보고서.md | 제목\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
}

// Inside a fenced code block a line is not markdown, and ">" at the front of one
// is a shell prompt or a diff. It stays in the block, counted and said, rather
// than being read as a quote of the document.
func TestAQuoteInsideACodeBlockIsStillCode(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"# 제목\n\n한 문장입니다.\n\n```\n> 인용\n```\n\n끝입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 제목\n@cover\n> 월간 보고서.md\n\n" +
		"# 제목\n- 한 문장입니다.\n- 끝입니다.\n!source 월간 보고서.md | 제목\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
	if len(document.Warnings) != 1 || !strings.Contains(document.Warnings[0], "코드 블록 1개(1줄)") {
		t.Errorf("warned %q, want the one block of one line", document.Warnings)
	}
}

// A fence that never closes gives its lines back to the document, read by the
// same rules as the rest of the file. One stray ``` must not be what decides
// whether a marker reaches a slide.
func TestAReplayedUnclosedFenceStillUnquotesAQuote(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"# 첫째 장\n\n```\n\n> 인용입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 첫째 장\n@cover\n> 월간 보고서.md\n\n" +
		"# 첫째 장\n- ```\n- 인용입니다.\n!source 월간 보고서.md | 첫째 장\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
	if len(document.Warnings) != 0 {
		t.Errorf("warned %q about a block the document never opened", document.Warnings)
	}
}
