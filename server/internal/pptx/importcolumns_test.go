package pptx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"
)

// A slide built in columns is read column by column. Every second real deck has
// a roadmap drawn as three stages side by side — a stage, how long it takes,
// what happens in it — and reading that down the page interleaves the three
// stages into one another. Worse, designers draw the middle stage lower than
// its neighbours to make a zigzag, and that came out as stage 1, stage 3,
// stage 2.
func TestASlideBuiltInColumnsIsReadColumnByColumn(t *testing.T) {
	const inch = 914400
	shapes := []struct {
		left, top int
		text      string
	}{
		{inch / 2, inch / 2, "향후 추진 계획"}, // spans the page: a title above the columns
		{inch / 2, 2 * inch, "1단계: 검증"},
		{inch / 2, 3 * inch, "1개월"},
		{inch / 2, 4 * inch, "테스트 환경 구축"},
		{9 * inch / 2, 4 * inch, "2단계: 시범 운영"}, // drawn lower: a zigzag timeline
		{9 * inch / 2, 5 * inch, "2개월"},
		{9 * inch / 2, 6 * inch, "파일럿 운영"},
		{8 * inch, 2 * inch, "3단계: 전체 전환"},
		{8 * inch, 3 * inch, "3~6개월"},
		{8 * inch, 4 * inch, "전체 마이그레이션"},
	}
	var drawn strings.Builder
	drawn.WriteString(`<p:nvGrpSpPr><p:cNvPr id="1" name="Shape 1"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>`)
	for index, shape := range shapes {
		width := 3 * inch
		if index == 0 {
			width = 11 * inch
		}
		fmt.Fprintf(&drawn, `<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Box %d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>`+
			`<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm></p:spPr>`+
			`<p:txBody><a:bodyPr/><a:p><a:r><a:rPr lang="ko-KR"/><a:t>%s</a:t></a:r></a:p></p:txBody></p:sp>`,
			index+2, index+2, shape.left, shape.top, width, inch/2, shape.text)
	}
	read := ReadDeck(packageWithSlide(t, drawn.String()))
	if len(read.Slides) == 0 {
		t.Fatal("nothing was read back")
	}
	said := []string{read.Slides[0].Title}
	for _, bullet := range read.Slides[0].Bullets {
		said = append(said, bullet.Text)
	}
	want := []string{"향후 추진 계획", "1단계: 검증", "1개월", "테스트 환경 구축",
		"2단계: 시범 운영", "2개월", "파일럿 운영", "3단계: 전체 전환", "3~6개월", "전체 마이그레이션"}
	if strings.Join(said, " / ") != strings.Join(want, " / ") {
		t.Errorf("the slide was read as\n  %s\nwant\n  %s", strings.Join(said, " / "), strings.Join(want, " / "))
	}
}

// An ordinary slide is still read down the page: a heading, two lists under it
// and a caption at the foot are not three columns to be read one after another.
func TestAnOrdinarySlideIsStillReadDownThePage(t *testing.T) {
	const inch = 914400
	shapes := []struct {
		left, top, width int
		text             string
	}{
		{4 * inch, inch, 3 * inch, "목 차"},
		{2 * inch, 2 * inch, 3 * inch, "개요와 배경"},
		{6 * inch, 2 * inch, 3 * inch, "구현과 효과"},
		{inch / 2, 6 * inch, 2 * inch, "주최 / 주관"},
	}
	var drawn strings.Builder
	drawn.WriteString(`<p:nvGrpSpPr><p:cNvPr id="1" name="Shape 1"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>`)
	for index, shape := range shapes {
		fmt.Fprintf(&drawn, `<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Box %d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>`+
			`<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm></p:spPr>`+
			`<p:txBody><a:bodyPr/><a:p><a:r><a:rPr lang="ko-KR"/><a:t>%s</a:t></a:r></a:p></p:txBody></p:sp>`,
			index+2, index+2, shape.left, shape.top, shape.width, inch/2, shape.text)
	}
	read := ReadDeck(packageWithSlide(t, drawn.String()))
	if len(read.Slides) == 0 {
		t.Fatal("nothing was read back")
	}
	if read.Slides[0].Title != "목 차" {
		t.Errorf("the slide is titled %q", read.Slides[0].Title)
	}
	said := []string{}
	for _, bullet := range read.Slides[0].Bullets {
		said = append(said, bullet.Text)
	}
	if strings.Join(said, " / ") != "개요와 배경 / 구현과 효과 / 주최 / 주관" {
		t.Errorf("the slide was read as %q", strings.Join(said, " / "))
	}
}

// packageWithSlide is a real package whose first slide is the drawing given.
func packageWithSlide(t *testing.T, spTree string) *Package {
	t.Helper()
	_, pkg, manifest := buildTemplate(t, "plum-rail")
	title, _ := manifest.Layout(manifest.TitleLayout)
	rendered, err := Render(pkg, manifest, Deck{Title: "자리", Language: "ko", Slides: []Slide{
		{LayoutID: title.ID, Fields: map[string][]Paragraph{SlotTitle: {{Text: "자리"}}}},
	}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	tree := regexp.MustCompile(`(?s)<p:spTree>.*?</p:spTree>`)
	reader, err := zip.NewReader(bytes.NewReader(rendered), int64(len(rendered)))
	if err != nil {
		t.Fatalf("read package: %v", err)
	}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	replaced := false
	for _, file := range reader.File {
		opened, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		content, err := io.ReadAll(opened)
		opened.Close()
		if err != nil {
			t.Fatalf("read %s: %v", file.Name, err)
		}
		if file.Name == "ppt/slides/slide1.xml" {
			content = tree.ReplaceAll(content, []byte("<p:spTree>"+spTree+"</p:spTree>"))
			replaced = true
		}
		entry, err := writer.Create(file.Name)
		if err != nil {
			t.Fatalf("write %s: %v", file.Name, err)
		}
		if _, err := entry.Write(content); err != nil {
			t.Fatalf("write %s: %v", file.Name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close package: %v", err)
	}
	if !replaced {
		t.Fatal("the rendered package has no first slide to draw on")
	}
	opened, err := Open(out.Bytes())
	if err != nil {
		t.Fatalf("open rewritten package: %v", err)
	}
	return opened
}
