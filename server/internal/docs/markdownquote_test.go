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

func TestAQuotedListItemLosesItsQuoteMarkersInEveryTextFormat(t *testing.T) {
	for _, extension := range []string{".md", ".markdown", ".txt"} {
		t.Run(extension, func(t *testing.T) {
			assertUnquotedListSource(t, "월간 보고서"+extension,
				"# 제목\n\n- > 인용입니다.\n", "# 제목\n\n- 인용입니다.\n")
		})
	}
}

func TestEveryListMarkerAllowsAQuoteInsideItsPoint(t *testing.T) {
	for _, line := range []string{
		"- > 문장", "* >> 문장", "+ > > 문장", "• > 문장", "•>문장",
		"· > 문장", "‣ > 문장", "▪ > 문장", "▫ > 문장", "◦ > 문장",
		"⦁ > 문장", "– > 문장", "— > 문장", "\t-\t>\t>\t문장\t",
		"> - > 문장",
	} {
		t.Run(line, func(t *testing.T) {
			assertUnquotedListSource(t, "월간 보고서.md",
				"# 제목\n\n"+line+"\n", "# 제목\n\n- 문장\n")
		})
	}
}

func TestAnEmptyQuotedListItemIsNotAPoint(t *testing.T) {
	for _, line := range []string{"- >", "- >>", "+ > >", "\t*\t>\t>\t", "• >"} {
		t.Run(line, func(t *testing.T) {
			assertUnquotedListSource(t, "월간 보고서.md",
				"# 제목\n- 첫 문장\n"+line+"\n- 둘째 문장\n",
				"# 제목\n- 첫 문장\n\n- 둘째 문장\n")
		})
	}
}

func TestEmptyQuotedListItemsDoNotStartAContinuationSlide(t *testing.T) {
	plain := "# 제목\n- 하나\n- 둘\n- 셋\n- 넷\n- 다섯\n"
	document := assertUnquotedListSource(t, "월간 보고서.md",
		plain+"- >\n- >>\n", plain)
	if strings.Contains(document.Source, "(계속)") {
		t.Errorf("empty quoted items started a continuation: %q", document.Source)
	}
}

func TestAnEmptyQuotedListItemStillSeparatesTables(t *testing.T) {
	first := "# 제목\n| 이름 | 값 |\n|---|---|\n| 첫째 | 하나 |\n"
	second := "| 이름 | 값 |\n|---|---|\n| 둘째 | 둘 |\n"
	for _, line := range []string{"- >", "- >>"} {
		t.Run(line, func(t *testing.T) {
			document := assertUnquotedListSource(t, "월간 보고서.md",
				first+line+"\n"+second, first+"\n"+second)
			if count := strings.Count(document.Source, "::table\n"); count != 2 {
				t.Errorf("read %d tables, want two: %q", count, document.Source)
			}
		})
	}
}

// Once a list marker has made a point, unquoting must not read its contents as
// another block or remove a second list marker.
func TestUnquotingAListItemKeepsItsContentsAsAPoint(t *testing.T) {
	for _, content := range []string{"# 소제목", "| 이름 | 값 |", "- 항목", "***"} {
		t.Run(content, func(t *testing.T) {
			assertUnquotedListSource(t, "월간 보고서.md",
				"# 제목\n- > "+content+"\n- 끝\n",
				"# 제목\n- "+content+"\n- 끝\n")
		})
	}
}

func TestAQuotedRuleInsideAListItemRemainsAPoint(t *testing.T) {
	// Removing the quote by hand would make "- ---" a thematic break before
	// the list branch runs, so pin the point directly for this boundary.
	document, err := Read("월간 보고서.md", []byte("# 제목\n- > ---\n- 끝\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 제목\n@cover\n> 월간 보고서.md\n\n" +
		"# 제목\n- \\---\n- 끝\n!source 월간 보고서.md | 제목\n\n"
	if document.Source != expected {
		t.Errorf("read as %q, want %q", document.Source, expected)
	}
	if len(document.Warnings) != 0 {
		t.Errorf("unexpected warnings: %q", document.Warnings)
	}
}

func TestUnquotingListItemsLeavesLiteralContentsAlone(t *testing.T) {
	for _, example := range []struct{ input, point string }{
		{"- 매출 > 목표", "매출 > 목표"},
		{"- # 제목", "\\# 제목"},
		{"-5% 감소", "\\-5% 감소"},
		{"- \\> 문자", "\\\\> 문자"},
	} {
		t.Run(example.input, func(t *testing.T) {
			document, err := Read("월간 보고서.md", []byte("# 제목\n"+example.input+"\n"))
			if err != nil {
				t.Fatal(err)
			}
			expected := "# 제목\n@cover\n> 월간 보고서.md\n\n" +
				"# 제목\n- " + example.point + "\n!source 월간 보고서.md | 제목\n\n"
			if document.Source != expected {
				t.Errorf("read as %q, want %q", document.Source, expected)
			}
			if len(document.Warnings) != 0 {
				t.Errorf("unexpected warnings: %q", document.Warnings)
			}
		})
	}
}

func TestQuotedListItemsInsideAClosedFenceAreOnlyCountedAsCode(t *testing.T) {
	for _, fence := range []string{"```", "~~~"} {
		t.Run(fence, func(t *testing.T) {
			document, err := Read("월간 보고서.md", []byte(
				"# 제목\n- 앞\n"+fence+"\n- > 인용\n- >>\n"+fence+"\n- 뒤\n"))
			if err != nil {
				t.Fatal(err)
			}
			plain, err := Read("월간 보고서.md", []byte("# 제목\n- 앞\n- 뒤\n"))
			if err != nil {
				t.Fatal(err)
			}
			if document.Source != plain.Source {
				t.Errorf("read as %q, want %q", document.Source, plain.Source)
			}
			if len(document.Warnings) != 1 || !strings.Contains(document.Warnings[0], "코드 블록 1개(2줄)") {
				t.Errorf("warned %q, want one code block of two lines", document.Warnings)
			}
		})
	}
}

func TestReplayedUnclosedFencesUnquoteListItems(t *testing.T) {
	for _, fence := range []string{"```", "~~~"} {
		t.Run(fence, func(t *testing.T) {
			assertUnquotedListSource(t, "월간 보고서.md",
				"# 제목\n"+fence+"\n- > 인용입니다.\n- >>\n- 끝\n",
				"# 제목\n"+fence+"\n- 인용입니다.\n\n- 끝\n")
		})
	}
}

func assertUnquotedListSource(t *testing.T, filename, quoted, plain string) Document {
	t.Helper()
	document, err := Read(filename, []byte(quoted))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := Read(filename, []byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	if document.Source != expected.Source {
		t.Errorf("read %q as %q, want %q", quoted, document.Source, expected.Source)
	}
	if len(document.Warnings) != 0 || len(expected.Warnings) != 0 {
		t.Errorf("unexpected warnings: quoted %q, plain %q", document.Warnings, expected.Warnings)
	}
	return document
}
