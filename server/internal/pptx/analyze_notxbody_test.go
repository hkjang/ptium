package pptx

import (
	"archive/zip"
	"bytes"
	"io"
	"regexp"
	"strings"
	"testing"
)

// A placeholder is allowed to carry no <p:txBody> at all. Several producers
// write picture and content placeholders that way, and two of the real decks
// used to measure this product are among them: uploading either one ended in a
// panic — a five hundred with nothing the customer could act on — rather than a
// template.
func TestALayoutPlaceholderWithNoTextBodyIsAnalyzed(t *testing.T) {
	data, err := BuiltinTemplate("plum-rail")
	if err != nil {
		t.Fatalf("BuiltinTemplate: %v", err)
	}
	stripped, removed := withoutOneLayoutTextBody(t, data)
	if !removed {
		t.Fatal("no layout placeholder to strip; the fixture no longer reproduces the file")
	}
	_, manifest, err := AnalyzeBytes(stripped)
	if err != nil {
		t.Fatalf("AnalyzeBytes on a layout whose placeholder has no text body: %v", err)
	}
	if len(manifest.Layouts) == 0 {
		t.Fatal("the template came back with no layouts")
	}
	for _, layout := range manifest.Layouts {
		if layout.Composed {
			// Regions derived from free space carry no padding of their own.
			continue
		}
		for _, placeholder := range layout.Placeholders {
			if placeholder.Inset <= 0 {
				t.Errorf("layout %q placeholder %q has inset %d", layout.Name, placeholder.Type, placeholder.Inset)
			}
		}
	}
}

// withoutOneLayoutTextBody rewrites the package so that the first placeholder of
// one layout carries no text body, which is what those files look like.
func withoutOneLayoutTextBody(t *testing.T, data []byte) ([]byte, bool) {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("read package: %v", err)
	}
	body := regexp.MustCompile(`(?s)<p:txBody>.*?</p:txBody>`)
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	removed := false
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
		if !removed && strings.HasPrefix(file.Name, "ppt/slideLayouts/slideLayout") && body.Match(content) {
			content = body.ReplaceAll(content, nil)
			removed = true
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
	return out.Bytes(), removed
}
