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
