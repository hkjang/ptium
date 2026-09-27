package docs

import (
	"strings"
	"testing"
)

// A runbook is one slide, not four.
//
// readMarkdown reads a line at a time and knew nothing of fenced code blocks, so
// every "# 1단계" inside one started a slide of its own, the ``` lines stayed
// behind as bullets, and the sentence after the block landed on the slide the
// last line of code had made. One page of a document arrived as three slides
// that say nothing anybody wrote.
func TestAFencedCodeBlockDoesNotStartASlide(t *testing.T) {
	document, err := readMarkdown("런북.md", []byte(
		"# 배포 절차\n\n배포는 아래 순서로 합니다.\n\n"+
			"```bash\n# 1단계: 이미지 빌드\nmake build\n"+
			"# 2단계: 배포\nkubectl apply -f deploy/kubernetes.yaml\n```\n\n"+
			"끝입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 배포 절차\n@cover\n> 런북.md\n\n" +
		"# 배포 절차\n- 배포는 아래 순서로 합니다.\n- 끝입니다.\n!source 런북.md | 배포 절차\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
	// What a deck cannot draw is said rather than dropped: there is no code
	// component, so the one block the file held is reported as one block.
	if len(document.Warnings) != 1 || !strings.Contains(document.Warnings[0], "코드 블록 1개") {
		t.Errorf("warned %q, want one warning naming one code block", document.Warnings)
	}
}

// A document with no code block is not told about code blocks.
func TestAFileWithoutCodeBlocksSaysNothingAboutThem(t *testing.T) {
	document, err := readMarkdown("메모.md", []byte("# 개요\n\n한 문장입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range document.Warnings {
		if strings.Contains(warning, "코드 블록") {
			t.Errorf("warned %q about code blocks in a file that has none", warning)
		}
	}
}

// An unclosed fence swallows nothing.
//
// A lone ``` in a document is a typo, not the start of a block that runs to the
// end of the file: treating it as one would lose every slide after it, which is
// worse than the bullet it used to leave behind. So the reader reads the file
// exactly as it did before the fence handling existed — the string below is what
// it produced then, to the character.
func TestAnUnclosedFenceSwallowsNothing(t *testing.T) {
	document, err := readMarkdown("메모.md", []byte(
		"# 첫째 장\n\n```\n\n# 둘째 장\n\n둘째 문장입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 첫째 장\n@cover\n> 메모.md\n\n" +
		"# 첫째 장\n- ```\n!source 메모.md | 첫째 장\n\n" +
		"# 둘째 장\n- 둘째 문장입니다.\n!source 메모.md | 둘째 장\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
	if len(document.Warnings) != 0 {
		t.Errorf("warned %q about a block the document never opened", document.Warnings)
	}
}

// Markdown draws a fence with either character, and a file written with tildes
// is the same file.
func TestATildeFenceIsAlsoACodeBlock(t *testing.T) {
	document, err := readMarkdown("런북.md", []byte(
		"# 배포 절차\n\n한 문장입니다.\n\n~~~\n# 1단계\nmake build\n~~~\n\n끝입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 배포 절차\n@cover\n> 런북.md\n\n" +
		"# 배포 절차\n- 한 문장입니다.\n- 끝입니다.\n!source 런북.md | 배포 절차\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
	if len(document.Warnings) != 1 || !strings.Contains(document.Warnings[0], "코드 블록 1개") {
		t.Errorf("warned %q, want one warning naming one code block", document.Warnings)
	}
}

// The line that opens a block may name the language after it; the line that ends
// one is the fence and nothing else. So "```python" in the middle of a bash
// block is a line of that block, not the end of it and the start of another.
func TestAFenceClosesOnlyOnALineOfNothingButFence(t *testing.T) {
	document, err := readMarkdown("런북.md", []byte(
		"# 배포 절차\n\n한 문장입니다.\n\n```bash\nmake build\n```python\nprint(1)\n```\n\n끝입니다.\n"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "# 배포 절차\n@cover\n> 런북.md\n\n" +
		"# 배포 절차\n- 한 문장입니다.\n- 끝입니다.\n!source 런북.md | 배포 절차\n\n"
	if document.Source != expected {
		t.Errorf("read as\n%s\nwant\n%s", document.Source, expected)
	}
	if len(document.Warnings) != 1 || !strings.Contains(document.Warnings[0], "코드 블록 1개") {
		t.Errorf("warned %q, want one code block, not two", document.Warnings)
	}
}

// A file that is nothing but a code block holds nothing a deck can draw, and
// says so the way an empty file does rather than returning a deck of one slide
// of code.
func TestAFileOfNothingButACodeBlockSaysItHasNothing(t *testing.T) {
	_, err := readMarkdown("코드.md", []byte("```go\nfunc main() {}\n```\n"))
	if err == nil || !strings.Contains(err.Error(), "슬라이드로 만들 내용을 찾지 못했습니다") {
		t.Errorf("read a deck out of a file of only code: %v", err)
	}
}
