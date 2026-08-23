package pptx

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

// A run used to name the same font for East Asian characters as for Latin ones,
// which told PowerPoint to set Hangul and kanji in Aptos — a face with neither.
// The template's own East Asian font is what those characters are for, and a
// run inherits it by leaving the element out.
func TestARunDoesNotSetALatinFaceForEastAsianText(t *testing.T) {
	_, pkg, manifest := buildTemplate(t, "plum-rail")
	layout, _ := manifest.Layout(manifest.DefaultLayout)
	deck := Deck{Language: "ko", Title: "전환 보고", Slides: []Slide{{
		LayoutID: layout.ID, Number: 1,
		Fields: map[string][]Paragraph{
			SlotTitle: {{Text: "전환은 지금 결정해야 합니다"}},
			SlotBody:  {{Text: "42개 시스템을 세 묶음으로 나눴습니다"}}}}}}
	file, err := Render(pkg, manifest, deck)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, part := range reader.File {
		if !strings.HasPrefix(part.Name, "ppt/slides/slide") {
			continue
		}
		opened, err := part.Open()
		if err != nil {
			t.Fatalf("open %s: %v", part.Name, err)
		}
		content, err := io.ReadAll(opened)
		opened.Close()
		if err != nil {
			t.Fatalf("read %s: %v", part.Name, err)
		}
		if strings.Contains(string(content), `<a:ea typeface="Aptos`) {
			t.Fatalf("%s sets a Latin face for East Asian text", part.Name)
		}
	}
	// The theme still says what East Asian text is set in.
	for _, part := range reader.File {
		if part.Name != "ppt/theme/theme1.xml" {
			continue
		}
		opened, _ := part.Open()
		content, _ := io.ReadAll(opened)
		opened.Close()
		if !strings.Contains(string(content), `<a:ea typeface="`) {
			t.Fatal("the theme names no East Asian font at all")
		}
	}
}
