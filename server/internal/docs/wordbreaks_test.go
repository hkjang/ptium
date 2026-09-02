package docs

import (
	"strings"
	"testing"
)

// A line break, a tab and a non-breaking hyphen are elements of their own, not
// characters inside a w:t. Reading only the text elements ran the words on
// either side of them together: a paragraph broken with Shift+Enter arrived as
// "첫째 줄둘째 줄", and "비-대면" arrived as a word the document does not contain.
func TestAWordParagraphKeepsWhatSeparatesItsWords(t *testing.T) {
	for _, one := range []struct {
		what     string
		body     string
		expected string
	}{
		{
			"two lines somebody broke with Shift+Enter",
			`<w:p><w:r><w:t>첫째 줄</w:t><w:br/><w:t>둘째 줄</w:t></w:r></w:p>`,
			"첫째 줄 둘째 줄",
		},
		{
			"a break between two runs",
			`<w:p><w:r><w:t>앞 문장</w:t></w:r><w:r><w:br/></w:r><w:r><w:t>뒤 문장</w:t></w:r></w:p>`,
			"앞 문장 뒤 문장",
		},
		{
			"a heading somebody numbered by hand",
			`<w:p><w:r><w:t>1.</w:t><w:tab/><w:t>개요</w:t></w:r></w:p>`,
			"1. 개요",
		},
		{
			"a hyphen the line may not break at",
			`<w:p><w:r><w:t>비</w:t><w:noBreakHyphen/><w:t>대면</w:t></w:r></w:p>`,
			"비-대면",
		},
		{
			// The break does not add to a space that is already there.
			"a run that ends in a space, and a break after it",
			`<w:p><w:r><w:t xml:space="preserve">앞 </w:t><w:br/><w:t>뒤</w:t></w:r></w:p>`,
			"앞 뒤",
		},
		{
			// The tabs a paragraph defines are where its tabs line up, not tabs
			// somebody typed, and they are written before its first word.
			"a paragraph that defines its own tab stops",
			`<w:p><w:pPr><w:tabs><w:tab w:val="left" w:pos="720"/></w:tabs></w:pPr>` +
				`<w:r><w:t>본문입니다</w:t></w:r></w:p>`,
			"본문입니다",
		},
		{
			// A break at the end of a paragraph separates nothing.
			"a paragraph that ends in a break",
			`<w:p><w:r><w:t>마지막 줄</w:t><w:br/></w:r></w:p>`,
			"마지막 줄",
		},
		{
			// A soft hyphen is where the line may break, not a hyphen. It is
			// drawn only when the line breaks there, and a slide is not that line.
			"a hyphen that is only drawn when the line breaks",
			`<w:p><w:r><w:t>가나</w:t><w:softHyphen/><w:t>다라</w:t></w:r></w:p>`,
			"가나다라",
		},
	} {
		document, err := readWordDocument("보고.docx", wordFile(t, `<w:p><w:pPr>`+
			`<w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>개요</w:t></w:r></w:p>`+one.body))
		if err != nil {
			t.Fatalf("%s: %v", one.what, err)
		}
		if !strings.Contains(document.Source, "- "+one.expected+"\n") {
			t.Errorf("%s: read as\n%s\nwant a point %q", one.what, document.Source, one.expected)
		}
	}
}

// A table cell holds the same breaks, and a cell whose two lines ran together
// is a figure nobody can read.
func TestAWordTableCellKeepsWhatSeparatesItsWords(t *testing.T) {
	document, err := readWordDocument("보고.docx", wordFile(t,
		`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>실적</w:t></w:r></w:p>`+
			`<w:tbl>`+
			`<w:tr><w:tc><w:p><w:r><w:t>항목</w:t></w:r></w:p></w:tc>`+
			`<w:tc><w:p><w:r><w:t>값</w:t></w:r></w:p></w:tc></w:tr>`+
			`<w:tr><w:tc><w:p><w:r><w:t>국내</w:t><w:br/><w:t>매출</w:t></w:r></w:p></w:tc>`+
			`<w:tc><w:p><w:r><w:t>120억</w:t></w:r></w:p></w:tc></w:tr>`+
			`</w:tbl>`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(document.Source, "- 국내 매출 | 120억\n") {
		t.Errorf("read as\n%s\nwant a row of 국내 매출 and 120억", document.Source)
	}
}
