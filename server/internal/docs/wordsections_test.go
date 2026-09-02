package docs

import (
	"strings"
	"testing"
)

// A content control does not hold only a sentence. Word wraps whole runs of a
// document in one — a template's cover page, a table of contents, the answered
// part of a form — and reading only the blocks directly under the body passed
// over every paragraph and every table inside it, with nothing said.
func TestAWordDocumentKeepsTheSectionsInsideItsWrappers(t *testing.T) {
	for _, one := range []struct {
		what     string
		body     string
		expected []string
	}{
		{
			// The whole report is the content control's, so this used to come back
			// as a deck with a title and nothing on it.
			"a report whose body is one content control",
			`<w:sdt><w:sdtPr><w:alias w:val="요약"/></w:sdtPr><w:sdtContent>` +
				`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>개요</w:t></w:r></w:p>` +
				`<w:p><w:r><w:t>본문 한 줄</w:t></w:r></w:p>` +
				`</w:sdtContent></w:sdt>`,
			[]string{"# 개요\n", "- 본문 한 줄\n"},
		},
		{
			"a table inside a content control",
			`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>실적</w:t></w:r></w:p>` +
				`<w:sdt><w:sdtContent><w:tbl>` +
				`<w:tr><w:tc><w:p><w:r><w:t>항목</w:t></w:r></w:p></w:tc>` +
				`<w:tc><w:p><w:r><w:t>값</w:t></w:r></w:p></w:tc></w:tr>` +
				`<w:tr><w:tc><w:p><w:r><w:t>매출</w:t></w:r></w:p></w:tc>` +
				`<w:tc><w:p><w:r><w:t>120억</w:t></w:r></w:p></w:tc></w:tr>` +
				`</w:tbl></w:sdtContent></w:sdt>`,
			[]string{"- 항목 | 값\n", "- 매출 | 120억\n"},
		},
		{
			"a content control inside another one",
			`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>개요</w:t></w:r></w:p>` +
				`<w:sdt><w:sdtContent><w:sdt><w:sdtContent>` +
				`<w:p><w:r><w:t>두 겹 안의 문장</w:t></w:r></w:p>` +
				`</w:sdtContent></w:sdt></w:sdtContent></w:sdt>`,
			[]string{"- 두 겹 안의 문장\n"},
		},
		{
			// The wrapper older files and some exporters write instead.
			"a section inside a custom XML block",
			`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>개요</w:t></w:r></w:p>` +
				`<w:customXml w:element="Body"><w:p><w:r><w:t>사용자 XML 안의 문장</w:t></w:r></w:p></w:customXml>`,
			[]string{"- 사용자 XML 안의 문장\n"},
		},
		{
			// What the control is called and what it offers to pick are how the
			// field works, not what the document says.
			"a content control that names itself and lists its choices",
			`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>개요</w:t></w:r></w:p>` +
				`<w:sdt><w:sdtPr><w:alias w:val="부서 선택"/>` +
				`<w:dropDownList><w:listItem w:displayText="영업본부" w:value="1"/></w:dropDownList>` +
				`</w:sdtPr><w:sdtContent><w:p><w:r><w:t>영업본부</w:t></w:r></w:p></w:sdtContent></w:sdt>`,
			[]string{"- 영업본부\n"},
		},
	} {
		document, err := readWordDocument("보고.docx", wordFile(t, one.body))
		if err != nil {
			t.Errorf("%s: %v", one.what, err)
			continue
		}
		for _, want := range one.expected {
			if !strings.Contains(document.Source, want) {
				t.Errorf("%s: read as\n%s\nwant %q in it", one.what, document.Source, want)
			}
		}
	}
}

// A wrapper in the middle of a report is not the end of it: what follows the
// content control is still under the heading the reader was on.
func TestAWordDocumentReadsAWrappedSectionWhereItWasWritten(t *testing.T) {
	document, err := readWordDocument("보고.docx", wordFile(t,
		`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>개요</w:t></w:r></w:p>`+
			`<w:p><w:r><w:t>첫째 줄</w:t></w:r></w:p>`+
			`<w:sdt><w:sdtContent><w:p><w:r><w:t>둘째 줄</w:t></w:r></w:p></w:sdtContent></w:sdt>`+
			`<w:p><w:r><w:t>셋째 줄</w:t></w:r></w:p>`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(document.Source, "- 첫째 줄\n- 둘째 줄\n- 셋째 줄\n") {
		t.Errorf("read as\n%s\nwant the three lines in the order they were written", document.Source)
	}
}
