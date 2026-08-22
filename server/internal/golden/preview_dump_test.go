package golden

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// TestWritePreviews dumps every fixture slide as SVG when PTIUM_PREVIEW_DIR
// names a directory. The recorded outlines say what the file contains; this is
// for the other question, which no assertion answers — whether the slide looks
// like something a person would present.
func TestWritePreviews(t *testing.T) {
	dir := os.Getenv("PTIUM_PREVIEW_DIR")
	if dir == "" {
		t.Skip("PTIUM_PREVIEW_DIR is unset")
	}
	for _, fixture := range fixtures {
		template, err := pptx.BuiltinTemplate(fixture.design)
		if err != nil {
			t.Fatalf("%s: builtin template: %v", fixture.name, err)
		}
		_, manifest, err := pptx.AnalyzeBytes(template)
		if err != nil {
			t.Fatalf("%s: analyze: %v", fixture.name, err)
		}
		compiled := deck.Compile(deck.ParseSource(fixture.source), manifest, deck.CompileOptions{
			Language: "ko",
			ResolveImage: func(reference string) (deck.ContentImage, bool) {
				return deck.ContentImage{AssetID: "asset-" + reference, Name: reference}, true
			},
		})
		built := deck.BuildWithImages(model.Presentation{Title: fixture.title, Language: "ko",
			Slides: compiled.Slides}, manifest, "Ptium", fixedPicture)
		for index, slide := range built.Slides {
			layout, _ := manifest.Layout(slide.LayoutID)
			svg := pptx.PreviewSVG(manifest, layout, slide, pptx.PreviewOptions{Width: 1280, Language: "ko"})
			path := filepath.Join(dir, fmt.Sprintf("%s-%02d.svg", fixture.name, index+1))
			if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}
		}
	}
}
