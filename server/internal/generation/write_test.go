package generation

import (
	"strings"
	"testing"
)

// The subject's own slice of the deck is not titled what a section is already
// called.
//
// A brief listing 기대효과 as a section produced both the listed section and the
// subject's outcome slide titled "기대 효과" — the same section twice, told apart
// by a space.
func TestTheSubjectsOwnSlideIsNotTitledAfterAListedSection(t *testing.T) {
	t.Parallel()
	topics := []promptTopic{{Name: "기대효과"}, {Name: "이행 일정"}}
	if aspect := unclaimedAspect("ko", frameOutcome, topics); withoutSpaces(aspect) == "기대효과" {
		t.Errorf("the subject's slide is titled %q, which a section is already called", aspect)
	}
	// With nothing claiming it, the frame's own word is the right title.
	if aspect := unclaimedAspect("ko", frameOutcome, []promptTopic{{Name: "현재 문제"}}); aspect != "기대 효과" {
		t.Errorf("the outcome aspect is titled %q", aspect)
	}
	// English too, and a language with no table falls back to nothing rather
	// than to another language's words.
	if aspect := unclaimedAspect("en", frameRisk, []promptTopic{{Name: "Risk and response"}}); strings.EqualFold(aspect, "risk and response") {
		t.Errorf("the English subject slide is titled %q, which a section is already called", aspect)
	}
	if aspect := unclaimedAspect("xx", frameRisk, nil); aspect != "" {
		t.Errorf("a language this deck does not speak produced %q", aspect)
	}
}

// The deck's own subject steps aside for the sections the brief listed.
//
// A slide titled 현황 that opens "무엇이 어떻게 달라지는지 지표로 말합니다" is the
// mismatch this writer was taught to remove, arriving by the back door: the
// subject's own slide had taken the situation frame, so the section the brief
// called 현재 문제 argued something else, and then the subject's slide was
// renamed after a frame it did not carry.
func TestTheSubjectsSlideLeavesListedSectionsTheirAngle(t *testing.T) {
	t.Parallel()
	topics := []promptTopic{
		{Name: "협력사 정산 개선안", Frame: frameSituation, Chosen: true},
		{Name: "현재 문제", Frame: frameSituation, Chosen: true},
		{Name: "이행 일정", Frame: frameSequence, Chosen: true},
	}
	wanted := framesWanted(topics, 0)
	if !wanted[frameSituation] || !wanted[frameSequence] {
		t.Fatalf("the angles the listed sections asked for are %v", wanted)
	}
	if wanted := framesWanted(topics, 1); wanted[frameSequence] != true || len(wanted) != 2 {
		t.Errorf("a section counts every other section's angle but not its own: %v", wanted)
	}

	// The subject's slide takes an angle nobody named, and is titled by it.
	angle, ok := unclaimedFrame("ko", frameSituation, topics, map[string]bool{}, wanted)
	if !ok {
		t.Fatal("the subject's slide could not find an angle")
	}
	if angle == frameSituation || angle == frameSequence {
		t.Errorf("the subject's slide took %q, which a listed section asked for", angle)
	}
	if aspect := frameTitleSuffix["ko"][angle]; aspect == "" {
		t.Errorf("the angle %q has no word to title the slide with", angle)
	}

	// With every angle spoken for, it keeps its own rather than being titled
	// after one it does not argue.
	all := map[string]bool{frameSituation: true, frameSequence: true, frameCase: true,
		frameOptions: true, frameRisk: true, frameOutcome: true}
	if angle, _ := unclaimedFrame("ko", frameCase, topics, all, wanted); angle != frameCase {
		t.Errorf("with nothing free the subject's slide became %q", angle)
	}
}
