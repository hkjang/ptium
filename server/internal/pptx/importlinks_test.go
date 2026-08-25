package pptx

import (
	"strings"
	"testing"
)

// A deck read back from a file keeps what its runs said, not only what they
// spelled. An address is the one part of a link that cannot be typed again from
// looking at the slide, and it was dropped: the words of the link came across
// and the link did not.
func TestAnImportedDeckKeepsItsLinksAndEmphasis(t *testing.T) {
	_, pkg, manifest := buildTemplate(t, "plum-rail")
	content, _ := manifest.Layout(manifest.DefaultLayout)
	deck := Deck{Title: "링크", Language: "ko", Slides: []Slide{
		{LayoutID: content.ID, Fields: map[string][]Paragraph{
			SlotTitle: {{Text: "자료"}},
			SlotBody: {
				{Text: "자료는 [계획서](https://example.com/plan)에 있습니다"},
				{Text: "**전환 대상** 42개와 *검토 중* 3개"},
			},
		}, Notes: "말할 내용"},
		{LayoutID: content.ID, Fields: map[string][]Paragraph{SlotTitle: {{Text: "두 번째 장"}}}},
	}}
	rendered, err := Render(pkg, manifest, deck)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	stored, err := Open(rendered)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	read := ReadDeck(stored)
	if len(read.Slides) == 0 {
		t.Fatal("nothing was read back")
	}
	said := ""
	for _, line := range read.Slides[0].Bullets {
		said += line.Text + "\n"
	}
	for _, want := range []string{"[계획서](https://example.com/plan)", "**전환 대상**", "*검토 중*"} {
		if !strings.Contains(said, want) {
			t.Errorf("the import lost %q; it read:\n%s", want, said)
		}
	}
}
