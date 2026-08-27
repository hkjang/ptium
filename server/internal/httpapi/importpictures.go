package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// Which of a deck's pictures come across, and what to say about the rest.
//
// A picture on most of the slides is the company's logo and a picture too small
// to be looked at is a decoration; neither is what a slide is about, and putting
// either into the new design's picture region would be worse than leaving it
// out. That much was already true — what was missing is that nobody was told.
// A file with twenty-four pictures on its slides imported with twelve, and the
// only thing said about the other twelve was two compiler lines about layouts.
// Somebody who put a diagram in their deck cannot tell that from a logo being
// dropped without opening both files side by side.
type pictureFilter struct {
	slides   int
	appears  map[string]int
	logos    map[string]bool
	tiny     map[string]bool
	repeated int
}

func newPictureFilter(slides []pptx.ImportedSlide) *pictureFilter {
	filter := &pictureFilter{slides: len(slides), appears: map[string]int{},
		logos: map[string]bool{}, tiny: map[string]bool{}}
	for _, slide := range slides {
		for _, picture := range slide.Pictures {
			filter.appears[pictureKey(picture)]++
		}
	}
	return filter
}

// keeps reports whether this picture belongs in the imported deck.
func (f *pictureFilter) keeps(picture pptx.ImportedPicture) bool {
	key := pictureKey(picture)
	// Two slides are not enough to tell a logo from a picture that happens to be
	// on both.
	if f.slides > 2 && f.appears[key]*2 > f.slides {
		f.logos[key] = true
		f.repeated = max(f.repeated, f.appears[key])
		return false
	}
	if picture.Area > 0 && picture.Area < 30 {
		f.tiny[key] = true
		return false
	}
	return true
}

// leftOut is what the import says about the pictures it did not carry, counted
// rather than listed: a deck with eleven decorations does not need eleven lines.
func (f *pictureFilter) leftOut() []string {
	var said []string
	if len(f.logos) > 0 {
		said = append(said, fmt.Sprintf("여러 장에 반복되는 그림 %d개는 로고로 보아 넣지 않았습니다(가장 많은 것은 %d장)",
			len(f.logos), f.repeated))
	}
	if len(f.tiny) > 0 {
		said = append(said, fmt.Sprintf("슬라이드의 3%%도 되지 않는 그림 %d개는 장식으로 보아 넣지 않았습니다", len(f.tiny)))
	}
	return said
}

// picturesLeftUndrawn says what happened to the pictures an import carried,
// when what happened is not what the person was told to expect.
//
// The import writes a picture into the deck's source; the design decides
// whether there is a region for it. A layout with one picture region draws the
// first picture on its slide and nothing else — the file this was measured
// against had twenty-two pictures written and twelve drawn, and the other ten
// were explained only in the technical lines this API keeps away from the
// person who uploaded the file. They were told twenty-two were on slides.
//
// The pictures are in their image library either way, which is what makes this
// something they can act on.
func picturesLeftUndrawn(carried, drawn int) string {
	if carried <= 0 || drawn >= carried {
		return ""
	}
	return fmt.Sprintf("그 가운데 %d개를 슬라이드에 넣었습니다. 나머지 %d개는 이 디자인의 레이아웃에 "+
		"그림 자리가 모자라 들어가지 못했습니다 — 이미지 탭에 있으니 원하는 장에 직접 넣을 수 있습니다",
		drawn, carried-drawn)
}

// picturesCarried counts what the import wrote into the deck's source.
func picturesCarried(source string) int {
	carried := 0
	for _, line := range strings.Split(source, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "::image ") {
			carried++
		}
	}
	return carried
}

// imagesOnSlide counts the pictures a compiled slide actually carries. A slide
// holds its images by region, so this is how many regions ended up with one.
func imagesOnSlide(slide model.Slide) int {
	var content struct {
		Images map[string]json.RawMessage `json:"images"`
	}
	if json.Unmarshal(slide.Content, &content) != nil {
		return 0
	}
	return len(content.Images)
}

// sayAfterPicturesSaved puts a sentence that begins "그 가운데" next to the line
// it is answering, rather than at the end of a list of unrelated news.
func sayAfterPicturesSaved(said []string, sentence string) []string {
	return sayAfterLineAbout(said, "이미지 라이브러리에 저장했습니다", sentence)
}

// sayAfterTablesRedrawn puts a sentence beside the line about tables, for the
// same reason: "그 가운데 …" two sentences from what it answers is a sentence
// about nothing.
func sayAfterTablesRedrawn(said []string, sentence string) []string {
	return sayAfterLineAbout(said, "다시 그렸습니다", sentence)
}

func sayAfterLineAbout(said []string, marker, sentence string) []string {
	for index, line := range said {
		if strings.Contains(line, marker) {
			with := make([]string, 0, len(said)+1)
			with = append(with, said[:index+1]...)
			with = append(with, sentence)
			return append(with, said[index+1:]...)
		}
	}
	return append(said, sentence)
}

// modelConnected reports whether this deployment has a model host to send a
// deck to. Without one it writes decks with the built-in writer and cannot
// rewrite an existing one.
func (s *Server) modelConnected(ctx context.Context) bool {
	return modelConnectedTo(ctx, s.settings)
}

// settingsReader is the part of the settings service this question needs, so
// the same answer is given to the web and to an agent over MCP.
type settingsReader interface {
	Get(ctx context.Context, key string, target any) error
}

func modelConnectedTo(ctx context.Context, settings settingsReader) bool {
	if settings == nil {
		return false
	}
	provider := "fallback"
	_ = settings.Get(ctx, "ai.provider", &provider)
	if strings.EqualFold(strings.TrimSpace(provider), "fallback") {
		return false
	}
	return true
}
