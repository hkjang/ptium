package export

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

func template(t *testing.T) ([]byte, pptx.Manifest) {
	t.Helper()
	data, err := pptx.BuiltinTemplate("slate-classic")
	if err != nil {
		t.Fatalf("BuiltinTemplate: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatalf("AnalyzeBytes: %v", err)
	}
	return data, manifest
}

func TestPPTXRendersIntoTheTemplate(t *testing.T) {
	data, manifest := template(t)
	content := deck.Content{Type: deck.ContentType}
	content.SetText(pptx.SlotTitle, "핵심 진단")
	content.SetField(pptx.SlotBody, []pptx.Paragraph{{Text: "온보딩 단계에서 이탈이 집중된다"}, {Text: "재방문 고객은 영향을 받지 않는다", Level: 1}})

	presentation := model.Presentation{
		Title: "성장 전략", Language: "ko",
		Slides: []model.Slide{
			{Position: 1, Title: "성장 전략", Subtitle: "2026 로드맵", Layout: "title", Content: json.RawMessage(`{}`)},
			{Position: 2, Title: "핵심 진단", LayoutID: manifest.DefaultLayout, Content: content.Encode(), SpeakerNotes: "원인에 집중"},
		},
	}
	rendered, err := PPTX(presentation, Options{TemplateData: data, Manifest: manifest, Author: "Ptium"})
	if err != nil {
		t.Fatalf("PPTX: %v", err)
	}
	pkg, err := pptx.Open(rendered)
	if err != nil {
		t.Fatalf("rendered file is not a package: %v", err)
	}
	slide1, _ := pkg.Text("ppt/slides/slide1.xml")
	if !strings.Contains(slide1, "성장 전략") || !strings.Contains(slide1, "2026 로드맵") {
		t.Fatalf("legacy slide fields were not mapped onto the template:\n%s", slide1)
	}
	slide2, _ := pkg.Text("ppt/slides/slide2.xml")
	if !strings.Contains(slide2, "온보딩 단계에서 이탈이 집중된다") || !strings.Contains(slide2, `<a:pPr lvl="1"/>`) {
		t.Fatalf("template fields were not rendered:\n%s", slide2)
	}
	if _, ok := pkg.Part("ppt/notesSlides/notesSlide1.xml"); !ok {
		t.Fatal("speaker notes were dropped")
	}
	// The customer's design must survive untouched.
	original, _ := pptx.Open(data)
	for _, part := range []string{"ppt/theme/theme1.xml", "ppt/slideMasters/slideMaster1.xml", "ppt/slideLayouts/slideLayout1.xml"} {
		before, _ := original.Text(part)
		after, ok := pkg.Text(part)
		if !ok || before != after {
			t.Fatalf("%s was modified during export", part)
		}
	}
}

func TestPPTXRequiresSlidesAndTemplate(t *testing.T) {
	data, manifest := template(t)
	if _, err := PPTX(model.Presentation{Title: "x"}, Options{TemplateData: data, Manifest: manifest}); err == nil {
		t.Fatal("an empty deck must not export")
	}
	presentation := model.Presentation{Title: "x", Slides: []model.Slide{{Position: 1, Title: "a", Content: json.RawMessage(`{}`)}}}
	if _, err := PPTX(presentation, Options{}); err == nil {
		t.Fatal("a missing template must not export")
	}
}

func TestPPTXRecoversFromStaleManifest(t *testing.T) {
	data, _ := template(t)
	presentation := model.Presentation{Title: "x", Slides: []model.Slide{{Position: 1, Title: "제목", Content: json.RawMessage(`{}`)}}}
	rendered, err := PPTX(presentation, Options{TemplateData: data, Manifest: pptx.Manifest{}})
	if err != nil {
		t.Fatalf("a stale manifest should be recomputed, got %v", err)
	}
	pkg, err := pptx.Open(rendered)
	if err != nil {
		t.Fatalf("rendered file is not a package: %v", err)
	}
	slide, _ := pkg.Text("ppt/slides/slide1.xml")
	if !strings.Contains(slide, "제목") {
		t.Fatalf("slide content missing:\n%s", slide)
	}
}

func TestPreviewSVGRendersRequestedSlide(t *testing.T) {
	_, manifest := template(t)
	presentation := model.Presentation{Title: "x", Language: "ko", Slides: []model.Slide{
		{Position: 1, Title: "첫 장", Content: json.RawMessage(`{}`)},
		{Position: 2, Title: "둘째 장", Content: json.RawMessage(`{}`)},
	}}
	svg, err := PreviewSVG(presentation, manifest, 2, 480, nil, nil)
	if err != nil {
		t.Fatalf("PreviewSVG: %v", err)
	}
	if !strings.Contains(svg, "둘째 장") || strings.Contains(svg, "첫 장") {
		t.Fatalf("preview rendered the wrong slide:\n%s", svg)
	}
	if _, err := PreviewSVG(presentation, manifest, 5, 480, nil, nil); err == nil {
		t.Fatal("an out-of-range slide must fail")
	}
}

// A deck edited on the canvas has slides its stored source no longer describes.
// The exported file used to carry that stored text, so importing the file gave
// the older deck back — in the corpus, a slide short.
func TestTheExportedFileCarriesTheDeckAsItStands(t *testing.T) {
	data, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 이중화 계획\n@cover\n> 2026년 8월\n\n# 지표\n- 처리량 1,200건\n- 오류율 0.2%\n"
	presentation := model.Presentation{Title: "이중화 계획", Language: "ko", Source: source}
	presentation.Slides = deck.Compile(deck.ParseSource(source), manifest,
		deck.CompileOptions{Language: "ko"}).Slides
	// Someone adds a slide on the canvas. Nothing rewrites the stored text.
	added := deck.Compile(deck.ParseSource("# 다음 단계\n- 승인 요청\n"), manifest,
		deck.CompileOptions{Language: "ko"}).Slides
	presentation.Slides = append(presentation.Slides, added...)

	file, err := PPTX(presentation, Options{TemplateData: data, Manifest: manifest})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	pkg, err := pptx.Open(file)
	if err != nil {
		t.Fatalf("open the exported file: %v", err)
	}
	carried, ok := pptx.DeckSource(pkg)
	if !ok {
		t.Fatal("the exported file carries no deck source")
	}
	if !strings.Contains(carried, "# 다음 단계") {
		t.Fatalf("the exported file does not carry the slide the deck has:\n%s", carried)
	}
	if got := len(deck.Compile(deck.ParseSource(carried), manifest,
		deck.CompileOptions{Language: "ko"}).Slides); got != len(presentation.Slides) {
		t.Fatalf("the file carries %d slides, the deck has %d", got, len(presentation.Slides))
	}
}

// A slide kept for the questions afterwards belongs in the file and not in the
// show. PowerPoint reads show="0" off the slide part, so a deck exported from
// here walks past the same slides the presenter here does.
func TestASkippedSlideIsMarkedInTheFile(t *testing.T) {
	data, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	source := "# 본론\n@content\n- 첫 줄\n\n# 부록\n@content\n!skip\n- 물어보면 보여 줄 표\n"
	compiled := deck.Compile(deck.ParseSource(source), manifest, deck.CompileOptions{Language: "ko"})
	presentation := model.Presentation{Title: "건너뛰기", Language: "ko", Slides: compiled.Slides}
	file, err := PPTX(presentation, Options{TemplateData: data, Manifest: manifest})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	marked := 0
	for _, entry := range archive.File {
		if !strings.HasPrefix(entry.Name, "ppt/slides/slide") || !strings.HasSuffix(entry.Name, ".xml") {
			continue
		}
		opened, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(opened)
		opened.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), `show="0"`) {
			marked++
		}
	}
	if marked != 1 {
		t.Fatalf("%d of the exported slides are marked as skipped, want exactly 1", marked)
	}
}
