package pptx

import (
	"strings"
	"testing"
)

// Text that fits because it was made too small to read.
//
// A region that will not hold its words is fitted by scaling the type down, so
// the deck always exports clean and the preview always looks right. Measured on
// the running product, a template with a twenty-point body drew a full slide at
// sixteen points and reported nothing at all about it: the only findings were
// "trimmed" and "no speaker notes". Sixteen points is a document, not a slide.
func TestBodyShrunkBelowWhatARoomCanReadIsReported(t *testing.T) {
	full := []Paragraph{}
	for range 6 {
		full = append(full, Paragraph{Text: strings.Repeat("자동화 도입으로 처리 시간이 줄었습니다 ", 3)})
	}
	kinds := sizeKinds(InspectDeck(sizeManifest(2000), Deck{Slides: []Slide{sizeSlide(full)}}))
	if !kinds[FindingTooSmall] {
		t.Fatalf("a body shrunk under the readable floor was not reported: %v", kinds)
	}

	// And the same template with what it holds is not reported. A check that
	// fires on everything says nothing.
	few := []Paragraph{{Text: "처리 시간이 줄었습니다"}, {Text: "재고 회전이 빨라졌습니다"}}
	if got := sizeKinds(InspectDeck(sizeManifest(2000), Deck{Slides: []Slide{sizeSlide(few)}})); got[FindingTooSmall] {
		t.Errorf("a body drawn at the template's own size was reported as too small: %v", got)
	}
}

// A design that sets its body small did that on purpose, on every slide of
// every deck, and it is not this author's doing. Saying so on each slide would
// be noise about a decision they cannot act on.
func TestATemplateThatIsSimplySmallIsNotReported(t *testing.T) {
	few := []Paragraph{{Text: "처리 시간이 줄었습니다"}, {Text: "재고 회전이 빨라졌습니다"}}
	if got := sizeKinds(InspectDeck(sizeManifest(1200), Deck{Slides: []Slide{sizeSlide(few)}})); got[FindingTooSmall] {
		t.Errorf("a twelve-point template drawing text at its own size was reported: %v", got)
	}
}

// Below the crowding floor the deck is already told the text does not fit. One
// region reported twice for one cause reads as two problems.
func TestTextThatDoesNotFitIsReportedOnce(t *testing.T) {
	spilling := []Paragraph{}
	for range 14 {
		spilling = append(spilling, Paragraph{Text: strings.Repeat("자동화 도입으로 처리 시간이 크게 줄었습니다 ", 4)})
	}
	kinds := sizeKinds(InspectDeck(sizeManifest(2000), Deck{Slides: []Slide{sizeSlide(spilling)}}))
	if !kinds[FindingOverflow] {
		t.Fatalf("text past the floor was not reported as overflowing: %v", kinds)
	}
	if kinds[FindingTooSmall] {
		t.Errorf("text that does not fit was reported both as overflowing and as too small: %v", kinds)
	}
}

// The finding says the two numbers an author acts on: what the template asks
// for and what they will get.
func TestTheFindingSaysBothSizes(t *testing.T) {
	full := []Paragraph{}
	for range 6 {
		full = append(full, Paragraph{Text: strings.Repeat("자동화 도입으로 처리 시간이 줄었습니다 ", 3)})
	}
	for _, finding := range InspectDeck(sizeManifest(2000), Deck{Slides: []Slide{sizeSlide(full)}}) {
		if finding.Kind != FindingTooSmall {
			continue
		}
		if !strings.Contains(finding.Detail, "from 20pt") || !strings.Contains(finding.Detail, "18pt") {
			t.Errorf("the finding reads %q", finding.Detail)
		}
		if !finding.Advisory {
			t.Error("text that is drawn correctly and reads badly was reported as a defect")
		}
		return
	}
	t.Fatal("nothing was reported")
}

func sizeKinds(findings []Finding) map[string]bool {
	kinds := map[string]bool{}
	for _, finding := range findings {
		kinds[finding.Kind] = true
	}
	return kinds
}

func sizeSlide(body []Paragraph) Slide {
	return Slide{LayoutID: "content", Notes: "말할 내용", Fields: map[string][]Paragraph{
		SlotTitle: {{Text: "물류센터 자동화"}},
		SlotBody:  body,
	}}
}

func sizeManifest(bodyPoints int) Manifest {
	layout := Layout{ID: "content", Name: "제목 및 내용", Role: RoleContent, Background: "FFFFFF",
		Placeholders: []Placeholder{
			{Slot: SlotTitle, Kind: "text", Type: "title", X: 800000, Y: 400000,
				Width: 8000000, Height: 900000, FontSize: 3200, MaxChars: 40, MaxLines: 2, LineEm: 22},
			{Slot: SlotBody, Kind: "text", Type: "body", X: 800000, Y: 1600000,
				Width: 8000000, Height: 3600000, FontSize: bodyPoints, MaxChars: 240, MaxLines: 8, LineEm: 30},
		}}
	return Manifest{Version: ManifestVersion, SlideWidth: 12192000, SlideHeight: 6858000,
		Theme:   Theme{Colors: map[string]string{"lt1": "FFFFFF", "dk1": "111111", "accent1": "1E6FFF"}},
		Layouts: []Layout{layout}}
}
