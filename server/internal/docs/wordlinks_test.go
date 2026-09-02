package docs

import (
	"strings"
	"testing"
)

// A paragraph's text is not all in runs directly under it. Word puts the words
// a link covers inside the link, the words somebody typed with revision marks
// on inside the insertion, and the words a content control holds inside it —
// and reading only the outer runs dropped all of them without a word said.
func TestAWordParagraphKeepsTheWordsInsideItsWrappers(t *testing.T) {
	for _, one := range []struct {
		what     string
		body     string
		expected string
	}{
		{
			"a sentence with a link in the middle of it",
			`<w:p><w:r><w:t>자세한 내용은 </w:t></w:r>` +
				`<w:hyperlink r:id="rId5"><w:r><w:t>여기</w:t></w:r></w:hyperlink>` +
				`<w:r><w:t>를 참고하십시오</w:t></w:r></w:p>`,
			"자세한 내용은 여기를 참고하십시오",
		},
		{
			"a sentence typed with revision marks on",
			`<w:p><w:ins w:id="1"><w:r><w:t>추가된 문장입니다</w:t></w:r></w:ins></w:p>`,
			"추가된 문장입니다",
		},
		{
			"a sentence inside a content control",
			`<w:p><w:sdt><w:sdtContent><w:r><w:t>표준 문안입니다</w:t></w:r></w:sdtContent></w:sdt></w:p>`,
			"표준 문안입니다",
		},
		{
			"a link inside an insertion",
			`<w:p><w:ins><w:hyperlink><w:r><w:t>겹쳐진 문장</w:t></w:r></w:hyperlink></w:ins></w:p>`,
			"겹쳐진 문장",
		},
		{
			// What the document says now is what it says: text somebody struck
			// out is w:delText, and a field's instructions are not a sentence.
			"a sentence with a deletion and a field in it",
			`<w:p><w:r><w:t>남은 문장</w:t></w:r>` +
				`<w:del><w:r><w:delText>지운 문장</w:delText></w:r></w:del>` +
				`<w:r><w:instrText> PAGE </w:instrText></w:r></w:p>`,
			"남은 문장",
		},
		{
			// An equation spells its text the same way in its own namespace. It
			// was passed over before this and is passed over still.
			"a sentence with an equation after it",
			`<w:p><w:r><w:t>수식 앞</w:t></w:r>` +
				`<m:oMath><m:r><m:t>x+1</m:t></m:r></m:oMath></w:p>`,
			"수식 앞",
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

// A table cell keeps its text the same way a paragraph does, so a linked figure
// in a report's table used to arrive as an empty cell and push nothing into the
// column beside it.
func TestAWordTableCellKeepsTheWordsInsideItsWrappers(t *testing.T) {
	document, err := readWordDocument("보고.docx", wordFile(t,
		`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>실적</w:t></w:r></w:p>`+
			`<w:tbl>`+
			`<w:tr><w:tc><w:p><w:r><w:t>항목</w:t></w:r></w:p></w:tc>`+
			`<w:tc><w:p><w:r><w:t>값</w:t></w:r></w:p></w:tc></w:tr>`+
			`<w:tr><w:tc><w:p><w:hyperlink><w:r><w:t>매출</w:t></w:r></w:hyperlink></w:p></w:tc>`+
			`<w:tc><w:p><w:ins><w:r><w:t>120억</w:t></w:r></w:ins></w:p></w:tc></w:tr>`+
			`</w:tbl>`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(document.Source, "- 매출 | 120억\n") {
		t.Errorf("read as\n%s\nwant a row of 매출 and 120억", document.Source)
	}
}
