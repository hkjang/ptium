package httpapi

import (
	"strings"
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

// A template with one content layout is usable — every deck it makes goes
// through that one layout — but it has no cover and no closing, and the report
// has to say so. Every manifest names a layout for all four roles, so reading
// the names back reports "cover ✓" for a design that has none, directly above
// the compiler's own "this template has no closing layout".
func TestRoleCoverageIsAboutTheDesignNotTheFallback(t *testing.T) {
	manifest := probeManifest(t)
	if !hasLayoutForRole(manifest, pptx.RoleClosing) {
		t.Fatal("the fixture has no closing layout to lose")
	}
	only := manifest.Layouts[0]
	for _, layout := range manifest.Layouts {
		if layout.Role == pptx.RoleContent {
			only = layout
			break
		}
	}
	manifest.Layouts = []pptx.Layout{only}
	manifest.DefaultLayout, manifest.TitleLayout = only.ID, only.ID
	manifest.SectionLayout, manifest.ClosingLayout = only.ID, only.ID
	for role, name := range map[string]string{
		pptx.RoleTitle: "cover", pptx.RoleSection: "section", pptx.RoleClosing: "closing",
	} {
		if hasLayoutForRole(manifest, role) {
			t.Errorf("a design with one content layout is reported as having a %s layout", name)
		}
	}
	if !hasLayoutForRole(manifest, pptx.RoleContent) {
		t.Error("the one layout it does have is not reported")
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

// "not a usable PowerPoint file" is true of every rejected upload and helps
// nobody. When the first bytes say what the file actually is, say that.
func TestARejectedUploadIsNamedForWhatItIs(t *testing.T) {
	cases := []struct {
		what  string
		bytes []byte
		says  string
	}{
		{"a 97-2003 deck", []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1, 0x00}, ".pptx"},
		{"a file wrapped by document security", []byte("SCDSA002\x00\x01"), "문서보안"},
		{"something else entirely", []byte("%PDF-1.7\n%..."), "PowerPoint 파일이 아닙니다"},
		{"a zip that is not a deck", []byte("PK\x03\x04rest of a zip"), ""},
	}
	for _, one := range cases {
		hint := templateUploadHint(one.bytes)
		if one.says == "" {
			if hint != "" {
				t.Errorf("%s was described as %q, which the bytes do not say", one.what, hint)
			}
			continue
		}
		if !strings.Contains(hint, one.says) {
			t.Errorf("%s is described as %q, which does not mention %q", one.what, hint, one.says)
		}
	}
}
