package httpapi

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// What a person outside the workspace reads.
//
// A shared link lands on a page written in Korean — "덱을 열 수 없습니다" over
// the top, the comment box beside it — and the sentences under that heading
// came from the server in English. A reader who clicked a link that had run out
// met a Korean heading and an English explanation, and that page is often the
// only part of the product they ever see.
func TestTheSharedReaderIsSpokenToInOneLanguage(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("handlers_share.go"))
	if err != nil {
		t.Fatal(err)
	}
	// Every message this file hands to a reader, as a literal.
	messages := regexp.MustCompile(`writeError\([^)]*?"[a-z_]+",\s*"([^"]{6,})"`).FindAllStringSubmatch(string(source), -1)
	if len(messages) < 4 {
		t.Fatalf("found %d reader-facing messages, expected the handful this file has", len(messages))
	}
	for _, found := range messages {
		if !hasHangul(found[1]) {
			t.Errorf("a shared reader is told %q, and the page around it is Korean", found[1])
		}
	}
}

func hasHangul(text string) bool {
	for _, letter := range text {
		if letter >= 0xAC00 && letter <= 0xD7A3 {
			return true
		}
	}
	return false
}

// The Korean line the page keeps for when the server says nothing must still be
// there: it is what a reader sees if the answer ever arrives without a message.
func TestThePageKeepsItsOwnKoreanFallback(t *testing.T) {
	page, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "src", "pages", "SharedDeckPage.tsx"))
	if err != nil {
		t.Skip("the web source is not beside the server here")
	}
	if !strings.Contains(string(page), "이 링크로는 덱을 열 수 없습니다.") {
		t.Error("the shared page no longer carries a Korean line to fall back on")
	}
}
