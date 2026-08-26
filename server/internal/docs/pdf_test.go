package docs

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strings"
	"testing"
)

// shown writes a line the way the generators people actually use write Korean:
// one byte for a character that fits in one, two for a character that does not.
func shown(line string) string {
	var hex strings.Builder
	for _, character := range line {
		if character < 0x80 {
			fmt.Fprintf(&hex, "%02x", character)
			continue
		}
		fmt.Fprintf(&hex, "%04x", character)
	}
	return hex.String()
}

// pdfOf assembles a PDF whose pages say the given lines, one line per baseline.
func pdfOf(pages ...[]string) []byte {
	var objects []string
	kids := make([]string, 0, len(pages))
	// Object 1 is the catalogue and object 2 the page tree; each page then takes
	// two objects, its dictionary and its content.
	for index := range pages {
		kids = append(kids, fmt.Sprintf("%d 0 R", 3+index*2))
	}
	objects = append(objects,
		"<</Type /Catalog /Pages 2 0 R>>",
		fmt.Sprintf("<</Type /Pages /Kids [%s] /Count %d>>", strings.Join(kids, " "), len(pages)))
	for index, lines := range pages {
		var content strings.Builder
		for row, line := range lines {
			fmt.Fprintf(&content, "BT /F1 12 Tf 1 0 0 1 72 %d Tm <%s> Tj ET\n", 700-row*20, shown(line))
		}
		body := content.String()
		objects = append(objects,
			fmt.Sprintf("<</Type /Page /Parent 2 0 R /Resources <</Font <</F1 %d 0 R>>>> /Contents %d 0 R>>",
				3+len(pages)*2, 4+index*2),
			fmt.Sprintf("<</Length %d>>\nstream\n%s\nendstream", len(body), body))
	}
	objects = append(objects, "<</Type /Font /Subtype /Type1 /BaseFont /Helvetica>>")

	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n")
	for index, body := range objects {
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", index+1, body)
	}
	out.WriteString("trailer<</Root 1 0 R>>\n%%EOF\n")
	return out.Bytes()
}

// A PDF's pages are its slides, and each one cites the page it came from — the
// thing a person needs to check a figure against the original.
func TestAPDFsPagesBecomeSlidesThatCiteTheirPage(t *testing.T) {
	document, err := Read("실적 보고.pdf", pdfOf(
		[]string{"2026 상반기 실적", "매출은 1,240억입니다.", "영업이익은 210억입니다."},
		[]string{"하반기 계획", "신규 채널 두 곳을 엽니다."}))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	for _, want := range []string{
		"# 2026 상반기 실적",
		"- 매출은 1,240억입니다.",
		"# 하반기 계획",
		"!source 실적 보고.pdf | 2쪽",
	} {
		if !strings.Contains(document.Source, want) {
			t.Errorf("the deck is missing %q:\n%s", want, document.Source)
		}
	}
}

// A document named after what it is about makes the cover say its own title
// twice — once as the title and once as the file it came from. The product's
// own measurement calls that "the same point twice" on a deck nobody has even
// edited yet.
func TestACoverDoesNotSayItsTitleTwice(t *testing.T) {
	for _, filename := range []string{"협업 도구 소개.pdf", "협업 도구 소개 (1).pdf", "협업 도구 소개 (12).pdf"} {
		document, err := Read(filename, pdfOf(
			[]string{"협업 도구 소개", "여는 문장입니다."},
			[]string{"도입 효과", "둘째 쪽입니다."}))
		if err != nil {
			t.Fatalf("Read(%q) error = %v", filename, err)
		}
		if strings.Contains(document.Source, "> 협업 도구 소개") {
			t.Errorf("the cover of %q repeats its own title:\n%s", filename, document.Source)
		}
		if !strings.Contains(document.Source, "# 협업 도구 소개\n@cover") {
			t.Errorf("the cover of %q lost its title:\n%s", filename, document.Source)
		}
		// The file is still named on every slide, which is where it matters.
		if !strings.Contains(document.Source, "!source "+filename) {
			t.Errorf("the slides of %q no longer cite the file:\n%s", filename, document.Source)
		}
	}
	// A file whose name is not the deck's title still says where it came from.
	document, err := Read("2026-02-11 내려받음.pdf", pdfOf([]string{"협업 도구 소개", "여는 문장입니다."}))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !strings.Contains(document.Source, "> 2026-02-11 내려받음.pdf") {
		t.Errorf("a cover lost the file it came from:\n%s", document.Source)
	}
}

// A page that is one long sentence has no heading on it. Making the sentence
// the heading is how an import produces a slide titled with its own body.
func TestAPageWithNoHeadingContinuesTheOneBefore(t *testing.T) {
	document, err := Read("보고서.pdf", pdfOf(
		[]string{"도입 배경"},
		[]string{"앞 쪽에서 이어지는 긴 문장이 이 쪽 전체를 채우고 있으며 제목이라고 부를 만한 짧은 줄이 없습니다."},
		[]string{"그 다음 쪽도 마찬가지로 제목 없이 문장만 이어지고 있어서 앞 장을 계속 잇게 됩니다."}))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !strings.Contains(document.Source, "# 도입 배경 (계속)") {
		t.Errorf("the second page did not continue the first:\n%s", document.Source)
	}
	if strings.Contains(document.Source, "(계속) (계속)") {
		t.Errorf("a run of pages with no headings piled up its own title:\n%s", document.Source)
	}
}

// A regulation prints its own name at the top of all forty pages. Carried into
// a deck, that line titles every slide and is the first point on each.
func TestARunningHeaderIsNotASlidesTitle(t *testing.T) {
	subjects := []string{"목적", "정의", "적용 범위", "보고 의무", "검사", "제재"}
	sentences := []string{
		"이 규정은 전자금융거래의 안전을 목적으로 합니다.",
		"용어의 뜻은 다음 각 호와 같습니다.",
		"금융회사와 전자금융업자에게 적용합니다.",
		"사고가 나면 지체 없이 알려야 합니다.",
		"감독원은 필요한 자료를 요구할 수 있습니다.",
		"위반한 자에게는 시정을 명할 수 있습니다.",
	}
	var pages [][]string
	for index := 1; index <= 6; index++ {
		pages = append(pages, []string{
			fmt.Sprintf("법제처 %d 국가법령정보센터", index),
			fmt.Sprintf("제%d조(%s)", index, subjects[index-1]),
			sentences[index-1],
			fmt.Sprintf("- %d -", index),
		})
	}
	document, err := Read("규정.pdf", pdfOf(pages...))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if strings.Contains(document.Source, "# 법제처") {
		t.Errorf("the running header became a slide title:\n%s", document.Source)
	}
	if strings.Contains(document.Source, "국가법령정보센터") {
		t.Errorf("the running header became a point:\n%s", document.Source)
	}
	if strings.Contains(document.Source, "- - 3 -") {
		t.Errorf("a page number became a point:\n%s", document.Source)
	}
	if !strings.Contains(document.Source, "# 제3조(적용 범위)") {
		t.Errorf("the third section is not a slide:\n%s", document.Source)
	}
}

// A page holds more lines than a slide does. What is past the slide goes into
// its notes rather than into eight continuation slides or into nothing.
func TestWhatDoesNotFitOnASlideBecomesItsNotes(t *testing.T) {
	lines := []string{"3분기 요약"}
	for index := 1; index <= 12; index++ {
		lines = append(lines, fmt.Sprintf("%d번째 항목의 내용입니다.", index))
	}
	document, err := Read("요약.pdf", pdfOf(lines))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if strings.Count(document.Source, "# ") != 2 { // the cover and the page
		t.Errorf("a single page became more than one slide:\n%s", document.Source)
	}
	if !strings.Contains(document.Source, "!notes 12번째 항목의 내용입니다.") {
		t.Errorf("the twelfth line of the page is on no slide and in no note:\n%s", document.Source)
	}
}

// A continuation came from the same place in the file as what it continues.
func TestAContinuationCitesThePageItContinues(t *testing.T) {
	document, err := Read("보고서.pdf", pdfOf(
		[]string{"1쪽 제목", "첫 줄입니다."},
		[]string{"2쪽 제목", "둘째 줄입니다."}))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if strings.Contains(document.Source, "!source 보고서.pdf | 2쪽 제목") {
		t.Errorf("a slide cited its own title instead of the page:\n%s", document.Source)
	}
	if !strings.Contains(document.Source, "!source 보고서.pdf | 2쪽") {
		t.Errorf("the second page is not cited as a page:\n%s", document.Source)
	}
}

// A PDF too big to unpack in one go is not a PDF whose later pages are
// pictures. Telling somebody their pages are pictures sends them to re-export a
// file that reads perfectly well.
func TestABigPDFIsNotDescribedAsPagesOfPictures(t *testing.T) {
	var page strings.Builder
	for row := 0; row < 12000; row++ {
		fmt.Fprintf(&page, "BT /F1 12 Tf 1 0 0 1 72 %d Tm (%d번째 줄입니다) Tj ET\n", 700-row, row)
	}
	var packed bytes.Buffer
	writer := zlib.NewWriter(&packed)
	if _, err := writer.Write([]byte(page.String())); err != nil {
		t.Fatal(err)
	}
	writer.Close()

	const pages = 110
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n1 0 obj\n<</Type /Catalog /Pages 2 0 R>>\nendobj\n")
	kids := make([]string, 0, pages)
	for index := 0; index < pages; index++ {
		kids = append(kids, fmt.Sprintf("%d 0 R", 3+index*2))
	}
	fmt.Fprintf(&out, "2 0 obj\n<</Type /Pages /Kids [%s] /Count %d>>\nendobj\n", strings.Join(kids, " "), pages)
	font := 3 + pages*2
	for index := 0; index < pages; index++ {
		fmt.Fprintf(&out, "%d 0 obj\n<</Type /Page /Parent 2 0 R /Resources <</Font <</F1 %d 0 R>>>> /Contents %d 0 R>>\nendobj\n",
			3+index*2, font, 4+index*2)
		fmt.Fprintf(&out, "%d 0 obj\n<</Filter /FlateDecode /Length %d>>\nstream\n", 4+index*2, packed.Len())
		out.Write(packed.Bytes())
		out.WriteString("\nendstream\nendobj\n")
	}
	fmt.Fprintf(&out, "%d 0 obj\n<</Type /Font /Subtype /Type1 /BaseFont /Helvetica>>\nendobj\ntrailer<</Root 1 0 R>>\n", font)

	document, err := Read("긴 보고서.pdf", out.Bytes())
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	said := strings.Join(document.Warnings, " | ")
	if strings.Contains(said, "그림뿐") {
		t.Errorf("pages nobody read were called pictures: %q", said)
	}
	if !strings.Contains(said, "파일이 커서") {
		t.Errorf("the reader did not say the file was too big to read in one go: %q", said)
	}
}

// A scan has no text in it. An import that answers with an empty deck teaches
// people that the product does not work; one that says what is wrong tells them
// what to upload instead.
func TestAScannedPDFSaysWhyItCannotBeRead(t *testing.T) {
	scanned := []byte("%PDF-1.7\n" +
		"1 0 obj\n<</Type /Catalog /Pages 2 0 R>>\nendobj\n" +
		"2 0 obj\n<</Type /Pages /Kids [3 0 R] /Count 1>>\nendobj\n" +
		"3 0 obj\n<</Type /Page /Parent 2 0 R /Contents 4 0 R>>\nendobj\n" +
		"4 0 obj\n<</Length 30>>\nstream\nq 612 0 0 792 0 0 cm /Im1 Do Q\nendstream\nendobj\n" +
		"trailer<</Root 1 0 R>>\n")
	_, err := Read("스캔본.pdf", scanned)
	if err == nil {
		t.Fatal("a scan was read as a deck")
	}
	for _, want := range []string{"스캔", "PDF"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message does not say what is wrong: %q", err.Error())
		}
	}
}
