package httpapi

import (
	"testing"

	"github.com/hkjang/ptium/server/internal/generation"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

func probeManifest(t *testing.T) pptx.Manifest {
	t.Helper()
	data, err := pptx.BuiltinTemplate("plum-rail")
	if err != nil {
		t.Fatalf("BuiltinTemplate: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatalf("AnalyzeBytes: %v", err)
	}
	return manifest
}

func compileProbe(t *testing.T, manifest pptx.Manifest) generation.Deck {
	t.Helper()
	probe := model.Presentation{Title: "점검", Language: "ko", RequestedSlideCount: 7}
	return generation.CompileSourceWith(templateProbeSource, probe, model.Profile{},
		generation.Template{ID: "tpl", Manifest: manifest}, probePicture, nil)
}

// The report exists to answer one question before a customer puts forty decks
// through a design: does this template draw what a brief produces, or does it
// turn it into paragraphs? A design we ship draws all four.
func TestAShippedTemplateDrawsEveryComponent(t *testing.T) {
	compiled := compileProbe(t, probeManifest(t))
	if len(compiled.Slides) < 5 {
		t.Fatalf("the probe deck compiled to %d slides", len(compiled.Slides))
	}
	for kind, drawn := range componentsDrawn(compiled.Slides) {
		if !drawn {
			t.Errorf("a shipped template did not draw %s", kind)
		}
	}
}

// And the other side of it: a design with one bare layout draws none of them.
// Without this the report could return "all four drawn" unconditionally and
// every test above would still pass.
func TestATemplateWithNoRoomDrawsNothing(t *testing.T) {
	manifest := probeManifest(t)
	bare := manifest.Layouts[0]
	kept := bare.Placeholders[:0:0]
	for _, placeholder := range bare.Placeholders {
		if placeholder.Type == "title" || placeholder.Type == "ctrTitle" {
			kept = append(kept, placeholder)
		}
	}
	bare.Placeholders, bare.Composed = kept, false
	manifest.Layouts = []pptx.Layout{bare}
	manifest.DefaultLayout, manifest.TitleLayout = bare.ID, ""
	manifest.SectionLayout, manifest.ClosingLayout = "", ""
	compiled := compileProbe(t, manifest)
	if len(compiled.Slides) < 5 {
		t.Fatalf("the probe deck compiled to %d slides, so nothing was measured", len(compiled.Slides))
	}
	for kind, drawn := range componentsDrawn(compiled.Slides) {
		if drawn {
			t.Errorf("%s was reported as drawn by a template with no room for it", kind)
		}
	}
	if len(compiled.Warnings) == 0 {
		t.Error("a template that turned every component into prose said nothing about it")
	}
}
