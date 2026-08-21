package generation

import (
	"github.com/hkjang/ptium/server/internal/deck"
)

// Korean the way a person types it.
//
// A model writing Korean routinely puts a space between a figure and its unit,
// and between a foreign word and the particle that follows it: "배치 지연을
// 4 시간에서 15 분으로", "12 억 예산", "deliverables 를", "94% 의". Each one reads
// as machine output to a Korean reader — the deck can be right about everything
// else and still look like nobody wrote it.
//
// The parser has always closed those gaps as it reads a slide's text, so the
// slides came out right. What it did not touch was the deck's own source: the
// text the workspace shows, the text the author edits, and the text an export of
// the deck's speaker notes is written from. Running the same rules over what the
// model wrote, before any of it is parsed, keeps the two the same document.
//
// It applies to a deck written in Korean and to nothing else, and only to what a
// model wrote — never to what a person typed.
func tidyModelKorean(source, language string) string {
	if languageOf(language) != "ko" {
		return source
	}
	return deck.TidyKorean(source)
}
