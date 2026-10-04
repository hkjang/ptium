package docs

import "testing"

// YAML front matter is what Jekyll, Hugo and Obsidian write above the first line
// of a document: the file's own bookkeeping, fenced off from the prose by two
// rules so that every renderer knows to leave it alone. This reader read it as
// prose. Once the rules themselves stopped being points, what the author got was
// a slide named after the file carrying one bullet of metadata — "title: 보고서"
// — above the document that actually had a title.
//
// The expectation is the same document with the three lines taken off by hand,
// rather than a string written out here, because that is what skipping front
// matter has to mean: the deck is the deck the author would have got had they
// deleted the header before uploading.
func TestFrontMatterIsNotReadAsASlide(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"---\ntitle: 보고서\n---\n\n# 분기 요약\n문장입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Read("월간 보고서.md", []byte("# 분기 요약\n문장입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if document.Source != plain.Source {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, plain.Source)
	}
	// Front matter is punctuation around something the document says about
	// itself, not a paragraph that failed to arrive — so there is nothing to
	// report, the same as for the rule it is fenced with.
	if len(document.Warnings) != 0 {
		t.Errorf("warned %q about a header that lost nothing", document.Warnings)
	}
}

// A rule at the top of a file is far more often a rule than the opening of front
// matter, and the cost of guessing wrong is the whole document: everything down
// to the next "---" would be skipped, which for a file that opens with a divider
// and closes with one is every slide in it.
//
// So two things have to hold before a line is read as a fence of front matter,
// and when either fails this reader skips nothing at all — the way an unclosed
// code fence gives its lines back. Each shape below is read against the document
// with the opening rule deleted, which is where this reader already stood: the
// metadata stays the point it was, and no line after it moves.
func TestWhatIsNotFrontMatterSkipsNothing(t *testing.T) {
	plain, err := Read("월간 보고서.md", []byte(
		"title: 보고서\n\n# 분기 요약\n문장입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		// Nothing closes the header, so there is no header — only a rule above a
		// line that happens to have a colon in it.
		"---\ntitle: 보고서\n\n# 분기 요약\n문장입니다.\n",
		// Front matter begins in the first column. An indented "---" is a rule
		// somebody laid out, and markdown's own fences are not written this way.
		"   ---\ntitle: 보고서\n---\n\n# 분기 요약\n문장입니다.\n",
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

// The document this guard is drawn for: a memo whose author put a divider above
// the title and another under the last line. Both rules are rules, and between
// them is the entire memo. What tells it apart from front matter is the first
// line with anything on it — front matter opens with a mapping key, and a title
// is not one.
func TestARuleAroundTheBodyIsNotFrontMatter(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"---\n\n# 분기 요약\n문장입니다.\n---\n"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Read("월간 보고서.md", []byte("# 분기 요약\n문장입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if document.Source != plain.Source {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, plain.Source)
	}
}

// A header a generator wrote and nobody filled in is two rules and nothing
// between them. Whether it is read as an empty header or as two rules makes no
// difference to the deck — neither puts anything on a slide — and what this pins
// down is that it takes none of the document with it either way.
func TestEmptyFrontMatterTakesNothingWithIt(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte(
		"---\n---\n\n# 분기 요약\n문장입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Read("월간 보고서.md", []byte("# 분기 요약\n문장입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if document.Source != plain.Source {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, plain.Source)
	}
}

// A file of nothing but front matter is a file of nothing but bookkeeping, and
// it is told the same thing a file of nothing but a code block is told: the
// reader found nothing to make a slide out of. Reading the metadata onto a slide
// instead would be answering a question nobody asked — the author uploaded a
// document, and this file has none.
func TestADocumentOfOnlyFrontMatterHasNoSlides(t *testing.T) {
	document, err := Read("월간 보고서.md", []byte("---\ntitle: 보고서\n---\n"))
	if err == nil {
		t.Errorf("read a deck out of a file of only front matter:\n%s", document.Source)
	}
}
