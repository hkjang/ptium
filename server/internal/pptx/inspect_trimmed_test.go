package pptx

import (
	"testing"
)

// A point that does not fit is reported. A heading that does not fit was cut
// with an ellipsis and reported nowhere: sixteen words were written, eight were
// drawn, and the quality panel called the deck clean.
func TestTextShortenedToFitIsReported(t *testing.T) {
	slide := Slide{LayoutID: "content", Notes: "말할 내용", Fields: map[string][]Paragraph{
		SlotTitle: {{Text: "항목01가나 항목02가나 항목03가나 항목04가나 항목05가나 항목06가나 항목07가나 항목08가나…"}},
	}}
	// Through the inspector the workspace actually calls, so the check is wired
	// in and not merely written.
	trimmed := trimmedKinds(InspectDeck(trimmedManifest(), Deck{Slides: []Slide{slide}}))
	if !trimmed[FindingTrimmed] {
		t.Fatalf("a title cut to fit was not reported: %v", trimmed)
	}
	whole := Slide{LayoutID: slide.LayoutID, Notes: "말할 내용",
		Fields: map[string][]Paragraph{SlotTitle: {{Text: "물류센터 자동화 도입 승인"}}}}
	if got := trimmedKinds(InspectDeck(trimmedManifest(), Deck{Slides: []Slide{whole}})); got[FindingTrimmed] {
		t.Errorf("a heading that fits was reported as cut: %v", got)
	}
}

func trimmedKinds(findings []Finding) map[string]bool {
	kinds := map[string]bool{}
	for _, finding := range findings {
		kinds[finding.Kind] = true
	}
	return kinds
}

func trimmedManifest() Manifest {
	layout := Layout{ID: "content", Name: "제목 및 내용", Role: RoleContent, Background: "FFFFFF",
		Placeholders: []Placeholder{
			{Slot: SlotTitle, Kind: "text", Type: "title", X: 800000, Y: 400000,
				Width: 8000000, Height: 900000, FontSize: 3200, MaxChars: 40, MaxLines: 2, LineEm: 22},
		}}
	return Manifest{Version: ManifestVersion, SlideWidth: 12192000, SlideHeight: 6858000,
		Theme:   Theme{Colors: map[string]string{"lt1": "FFFFFF", "dk1": "111111", "accent1": "1E6FFF"}},
		Layouts: []Layout{layout}}
}
