package pptx

import "testing"

// A note that is the slide, read back.
//
// This product's own one-click draft wrote `${title}: ${first point}` — the
// words on the slide, joined by a colon — and a slide with nothing but a title
// got its title twice. Reviews that test these tools name speaker notes as the
// weakest thing the category produces, and this was ours.
func TestANoteThatOnlyRepeatsTheSlideIsReported(t *testing.T) {
	kinds := notesKinds(InspectDeck(notesManifest(), Deck{Slides: []Slide{{
		LayoutID: "content",
		Notes:    "재고 회전율 개선 근거: 투입과 회수를 같은 기준으로 놓고 봅니다",
		Fields: map[string][]Paragraph{
			SlotTitle: {{Text: "재고 회전율 개선 근거"}},
			SlotBody: {{Text: "투입과 회수를 같은 기준으로 놓고 봅니다"},
				{Text: "회수 시점과 절감액"}},
		},
	}}}))
	if !kinds[FindingNotesEcho] {
		t.Fatalf("a note made of the slide's own words was not reported: %v", kinds)
	}
	// The worst shape it produced: a slide with only a title.
	only := notesKinds(InspectDeck(notesManifest(), Deck{Slides: []Slide{{
		LayoutID: "content", Notes: "남은 질문: 남은 질문",
		Fields: map[string][]Paragraph{SlotTitle: {{Text: "남은 질문"}}},
	}}}))
	if !only[FindingNotesEcho] {
		t.Errorf("a note that is the title twice was not reported: %v", only)
	}
}

// Word for word, so that a note adding anything at all is left alone. A check
// that argues with an author about how much they added would be worse than no
// check: they would turn it off.
func TestANoteThatSaysSomethingIsLeftAlone(t *testing.T) {
	for _, note := range []string{
		// what the draft writes now
		"이 장에서 남길 한 가지를 정해 두고, 나머지는 질문이 나오면 말합니다. 그다음 비용과 효과로 넘어갑니다.",
		// what the writer writes with no model
		"숫자의 출처와 가정을 먼저 말합니다. 가정이 흔들리면 결론도 흔들린다는 점을 분명히 합니다.",
		// an author's own note that quotes the slide and adds one thing
		"재고 회전율 개선 근거: 투입과 회수를 같은 기준으로 놓고 봅니다. 작년 실사 기준입니다",
		// and a note too short to judge
		"확인",
	} {
		kinds := notesKinds(InspectDeck(notesManifest(), Deck{Slides: []Slide{{
			LayoutID: "content", Notes: note,
			Fields: map[string][]Paragraph{
				SlotTitle: {{Text: "재고 회전율 개선 근거"}},
				SlotBody:  {{Text: "투입과 회수를 같은 기준으로 놓고 봅니다"}, {Text: "회수 시점과 절감액"}},
			},
		}}}))
		if kinds[FindingNotesEcho] {
			t.Errorf("a note that says something was reported: %q", note)
		}
	}
}

func notesKinds(findings []Finding) map[string]bool {
	kinds := map[string]bool{}
	for _, finding := range findings {
		kinds[finding.Kind] = true
	}
	return kinds
}

func notesManifest() Manifest {
	layout := Layout{ID: "content", Name: "제목 및 내용", Role: RoleContent, Background: "FFFFFF",
		Placeholders: []Placeholder{
			{Slot: SlotTitle, Kind: "text", Type: "title", X: 800000, Y: 400000,
				Width: 8000000, Height: 900000, FontSize: 3200, MaxChars: 40, MaxLines: 2, LineEm: 22},
			{Slot: SlotBody, Kind: "text", Type: "body", X: 800000, Y: 1600000,
				Width: 8000000, Height: 3600000, FontSize: 1800, MaxChars: 240, MaxLines: 8, LineEm: 30},
		}}
	return Manifest{Version: ManifestVersion, SlideWidth: 12192000, SlideHeight: 6858000,
		Theme:   Theme{Colors: map[string]string{"lt1": "FFFFFF", "dk1": "111111", "accent1": "1E6FFF"}},
		Layouts: []Layout{layout}}
}
