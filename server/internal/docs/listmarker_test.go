package docs

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAListMarkerIsRemovedWholeOrNotAtAll(t *testing.T) {
	for _, one := range []struct {
		line   string
		point  string
		marked bool
	}{
		// A deck pasted out of Word or HWP, and a PDF read back.
		{"• 매출이 늘었습니다", "매출이 늘었습니다", true},
		{"· 유통망 확대", "유통망 확대", true},
		{"▪ 해외 진출", "해외 진출", true},
		{"‣ 검토 중", "검토 중", true},
		{"- 매출이 늘었습니다", "매출이 늘었습니다", true},
		{"* 매출이 늘었습니다", "매출이 늘었습니다", true},
		{"  •   여백이 있어도", "여백이 있어도", true},
		// A hyphen also begins a number, and "-5% 감소" is a point about a fall.
		{"-5% 감소", "-5% 감소", false},
		{"–12명", "–12명", false},
		// A marker with nothing after it is punctuation, not an empty point.
		{"-", "-", false},
		{"•", "•", false},
		{"평범한 줄입니다", "평범한 줄입니다", false},
	} {
		point, marked := withoutListMarker(one.line)
		if point != one.point || marked != one.marked {
			t.Errorf("withoutListMarker(%q) = (%q, %v), want (%q, %v)",
				one.line, point, marked, one.point, one.marked)
		}
	}
}

// The bullet is three bytes. Cutting one off left the two it ends with in front
// of the words, which is not text: the deck reached the database as invalid
// UTF-8 and importing a file whose points begin with • failed outright, with
// nothing shown to the person but a server error.
func TestAPastedBulletDoesNotBreakTheDocument(t *testing.T) {
	document, err := readMarkdown("붙여넣기.md",
		[]byte("# 분기 실적\n\n• 매출이 늘었습니다\n• 고객이 늘었습니다\n"))
	if err != nil {
		t.Fatalf("reading the document: %v", err)
	}
	if !utf8.ValidString(document.Source) {
		t.Fatalf("the source is not text: %q", document.Source)
	}
	if strings.Contains(document.Source, "•") {
		t.Errorf("the bullet drawn on the page is still in the words: %q", document.Source)
	}
	for _, want := range []string{"- 매출이 늘었습니다", "- 고객이 늘었습니다"} {
		if !strings.Contains(document.Source, want) {
			t.Errorf("the source does not say %q:\n%s", want, document.Source)
		}
	}
}
