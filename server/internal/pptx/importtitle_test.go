package pptx

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

// Most real decks bold their title, and a heading is a slot rather than prose:
// the deck source has no way to bold part of one, so the emphasis marks come
// out as characters. Every one of the real decks measured against this was
// called "**목 차**" or "**POSTGRESQL 도입 타당성 검토**" in the deck list, and drew
// its own asterisks on the title slide.
func TestAnImportedTitleCarriesNoInlineMarkup(t *testing.T) {
	_, pkg, manifest := buildTemplate(t, "plum-rail")
	title, _ := manifest.Layout(manifest.TitleLayout)
	rendered, err := Render(pkg, manifest, Deck{Title: "표지", Language: "ko", Slides: []Slide{
		{LayoutID: title.ID, Fields: map[string][]Paragraph{
			SlotTitle:    {{Text: "AI 플랫폼 도입 방안"}},
			SlotSubtitle: {{Text: "2026년 상반기"}},
		}},
	}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	bolded := withEveryRunBold(t, rendered)
	stored, err := Open(bolded)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	read := ReadDeck(stored)
	if len(read.Slides) == 0 {
		t.Fatal("nothing was read back")
	}
	if strings.ContainsAny(read.Slides[0].Title, "*") {
		t.Errorf("the title is %q", read.Slides[0].Title)
	}
	// The subtitle is prose — "> …" carries emphasis and links — so what it
	// said is kept as it was said.
	if !strings.Contains(read.Slides[0].Lead, "2026년 상반기") {
		t.Errorf("the subtitle lost its words: %q", read.Slides[0].Lead)
	}
	if !strings.Contains(read.Slides[0].Title, "AI 플랫폼") {
		t.Errorf("the title lost its words: %q", read.Slides[0].Title)
	}
	// The deck's own name is the first slide's title unless the file states one,
	// and neither may arrive with markup in it.
	if strings.ContainsAny(read.Title, "*") {
		t.Errorf("the deck is called %q", read.Title)
	}
}

// withEveryRunBold is what a designer does to a title: the emphasis is in the
// file, not in the words.
func withEveryRunBold(t *testing.T, data []byte) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("read package: %v", err)
	}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	bolded := false
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
		if strings.HasPrefix(file.Name, "ppt/slides/slide") && bytes.Contains(content, []byte("<a:rPr")) {
			content = bytes.ReplaceAll(content, []byte("<a:rPr "), []byte(`<a:rPr b="1" `))
			bolded = true
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
	if !bolded {
		t.Fatal("no run to bold; the fixture no longer reproduces a real deck")
	}
	return out.Bytes()
}
