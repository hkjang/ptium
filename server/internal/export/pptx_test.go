package export

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

func template(t *testing.T) ([]byte, pptx.Manifest) {
	t.Helper()
	data, err := pptx.BuiltinTemplate("modern")
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
	svg, err := PreviewSVG(presentation, manifest, 2, 480)
	if err != nil {
		t.Fatalf("PreviewSVG: %v", err)
	}
	if !strings.Contains(svg, "둘째 장") || strings.Contains(svg, "첫 장") {
		t.Fatalf("preview rendered the wrong slide:\n%s", svg)
	}
	if _, err := PreviewSVG(presentation, manifest, 5, 480); err == nil {
		t.Fatal("an out-of-range slide must fail")
	}
}
