package docs

import (
	"strings"
	"testing"
)

// A table is wrapped the way the body is. The rows of a repeating section live
// inside a w:sdt in the table, and a cell somebody filled in on a form holds its
// paragraph inside one — so reading only what is directly under each level lost
// the cell's words, and lost whole rows.
func TestAWordTableKeepsTheRowsInsideItsWrappers(t *testing.T) {
	for _, one := range []struct {
		what     string
		body     string
		expected []string
	}{
		{
			// Every row but the header is the repeating section's, so the table used
			// to be left with the header alone and dropped for being too short.
			"a repeating section holding the rows",
			`<w:tbl>` +
				`<w:tr><w:tc><w:p><w:r><w:t>항목</w:t></w:r></w:p></w:tc>` +
				`<w:tc><w:p><w:r><w:t>값</w:t></w:r></w:p></w:tc></w:tr>` +
				`<w:sdt><w:sdtPr><w:alias w:val="반복 구역"/></w:sdtPr><w:sdtContent>` +
				`<w:tr><w:tc><w:p><w:r><w:t>매출</w:t></w:r></w:p></w:tc>` +
				`<w:tc><w:p><w:r><w:t>120억</w:t></w:r></w:p></w:tc></w:tr>` +
				`<w:tr><w:tc><w:p><w:r><w:t>영업이익</w:t></w:r></w:p></w:tc>` +
				`<w:tc><w:p><w:r><w:t>18억</w:t></w:r></w:p></w:tc></w:tr>` +
				`</w:sdtContent></w:sdt></w:tbl>`,
			[]string{"- 항목 | 값\n", "- 매출 | 120억\n", "- 영업이익 | 18억\n"},
		},
		{
			// A form's answer, which used to leave the cell blank.
			"a content control inside a cell",
			`<w:tbl>` +
				`<w:tr><w:tc><w:p><w:r><w:t>항목</w:t></w:r></w:p></w:tc>` +
				`<w:tc><w:p><w:r><w:t>값</w:t></w:r></w:p></w:tc></w:tr>` +
				`<w:tr><w:tc><w:p><w:r><w:t>매출</w:t></w:r></w:p></w:tc>` +
				`<w:tc><w:sdt><w:sdtPr><w:alias w:val="금액"/></w:sdtPr><w:sdtContent>` +
				`<w:p><w:r><w:t>120억</w:t></w:r></w:p>` +
				`</w:sdtContent></w:sdt></w:tc></w:tr></w:tbl>`,
			[]string{"- 매출 | 120억\n"},
		},
		{
			"a wrapper around a single row in the middle of the table",
			`<w:tbl>` +
				`<w:tr><w:tc><w:p><w:r><w:t>항목</w:t></w:r></w:p></w:tc></w:tr>` +
				`<w:sdt><w:sdtContent>` +
				`<w:tr><w:tc><w:p><w:r><w:t>둘째</w:t></w:r></w:p></w:tc></w:tr>` +
				`</w:sdtContent></w:sdt>` +
				`<w:tr><w:tc><w:p><w:r><w:t>셋째</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`,
			[]string{"- 항목\n- 둘째\n- 셋째\n"},
		},
		{
			// The wrapper older files and some exporters write instead.
			"a custom XML block holding a row",
			`<w:tbl>` +
				`<w:tr><w:tc><w:p><w:r><w:t>항목</w:t></w:r></w:p></w:tc></w:tr>` +
				`<w:customXml w:element="Row">` +
				`<w:tr><w:tc><w:p><w:r><w:t>매출</w:t></w:r></w:p></w:tc></w:tr>` +
				`</w:customXml></w:tbl>`,
			[]string{"- 항목\n- 매출\n"},
		},
		{
			"a cell wrapped twice over",
			`<w:tbl>` +
				`<w:tr><w:tc><w:p><w:r><w:t>항목</w:t></w:r></w:p></w:tc></w:tr>` +
				`<w:tr><w:tc><w:sdt><w:sdtContent><w:sdt><w:sdtContent>` +
				`<w:p><w:r><w:t>두 겹 안의 칸</w:t></w:r></w:p>` +
				`</w:sdtContent></w:sdt></w:sdtContent></w:sdt></w:tc></w:tr></w:tbl>`,
			[]string{"- 두 겹 안의 칸\n"},
		},
		{
			// The words inside a link in a cell, read the way a paragraph's are.
			"a link inside a wrapped cell",
			`<w:tbl>` +
				`<w:tr><w:tc><w:p><w:r><w:t>항목</w:t></w:r></w:p></w:tc></w:tr>` +
				`<w:tr><w:tc><w:sdt><w:sdtContent><w:p><w:r><w:t>자세한 내용은 </w:t></w:r>` +
				`<w:hyperlink r:id="rId1"><w:r><w:t>여기</w:t></w:r></w:hyperlink>` +
				`<w:r><w:t>를 보십시오</w:t></w:r></w:p></w:sdtContent></w:sdt></w:tc></w:tr></w:tbl>`,
			[]string{"- 자세한 내용은 여기를 보십시오\n"},
		},
		{
			// A cell's own paragraphs, still one cell — the wrapper does not split it.
			"two paragraphs in one cell, one of them wrapped",
			`<w:tbl>` +
				`<w:tr><w:tc><w:p><w:r><w:t>항목</w:t></w:r></w:p></w:tc></w:tr>` +
				`<w:tr><w:tc><w:p><w:r><w:t>첫째 줄</w:t></w:r></w:p>` +
				`<w:sdt><w:sdtContent><w:p><w:r><w:t>둘째 줄</w:t></w:r></w:p></w:sdtContent></w:sdt>` +
				`</w:tc></w:tr></w:tbl>`,
			[]string{"- 첫째 줄 둘째 줄\n"},
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

// A table inside a cell is a table of its own. Its paragraphs are not cells of
// the table around it, and opening the wrappers must not start reading them as
// though they were.
func TestAWordTableDoesNotReadTheTableInsideACell(t *testing.T) {
	document, err := readWordDocument("보고.docx", wordFile(t,
		`<w:tbl>`+
			`<w:tr><w:tc><w:p><w:r><w:t>항목</w:t></w:r></w:p></w:tc>`+
			`<w:tc><w:p><w:r><w:t>값</w:t></w:r></w:p></w:tc></w:tr>`+
			`<w:tr><w:tc><w:p><w:r><w:t>매출</w:t></w:r></w:p></w:tc>`+
			`<w:tc><w:p><w:r><w:t>120억</w:t></w:r></w:p>`+
			`<w:tbl><w:tr><w:tc><w:p><w:r><w:t>안쪽 표</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`+
			`</w:tc></w:tr></w:tbl>`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(document.Source, "- 매출 | 120억\n") {
		t.Errorf("read as\n%s\nwant the cell to be what the cell says", document.Source)
	}
	if strings.Contains(document.Source, "안쪽 표") {
		t.Errorf("read as\n%s\nwant the inner table left out of the outer one", document.Source)
	}
}
