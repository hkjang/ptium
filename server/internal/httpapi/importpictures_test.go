package httpapi

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/pptx"
)

// What an import leaves behind, and what it says about it.
//
// A real file with twenty-four pictures on its slides imported with twelve.
// Both halves of that were right — the other twelve were the company logo and
// eleven decorations — and neither half was said. Somebody whose diagram went
// missing could not tell it from a logo being dropped.
func TestAnImportSaysWhichPicturesItLeftOut(t *testing.T) {
	t.Parallel()
	picture := func(name string, area int) pptx.ImportedPicture {
		return pptx.ImportedPicture{Name: name, Data: []byte(name), Area: area}
	}
	// Eight slides: a logo on six of them, two decorations, and two photographs.
	slides := make([]pptx.ImportedSlide, 8)
	for index := range slides {
		if index < 6 {
			slides[index].Pictures = append(slides[index].Pictures, picture("logo.png", 40))
		}
	}
	slides[0].Pictures = append(slides[0].Pictures, picture("bullet.png", 5))
	slides[1].Pictures = append(slides[1].Pictures, picture("arrow.png", 3))
	slides[2].Pictures = append(slides[2].Pictures, picture("diagram.png", 300))
	slides[3].Pictures = append(slides[3].Pictures, picture("photo.jpg", 500))

	filter := newPictureFilter(slides)
	var kept []string
	for _, slide := range slides {
		for _, one := range slide.Pictures {
			if filter.keeps(one) {
				kept = append(kept, one.Name)
			}
		}
	}
	if len(kept) != 2 || kept[0] != "diagram.png" || kept[1] != "photo.jpg" {
		t.Errorf("the pictures carried over were %v", kept)
	}
	said := strings.Join(filter.leftOut(), " | ")
	if !strings.Contains(said, "그림 1개는 로고") {
		t.Errorf("the logo left out is not reported: %q", said)
	}
	if !strings.Contains(said, "6장") {
		t.Errorf("the logo report does not say how many slides carried it: %q", said)
	}
	if !strings.Contains(said, "그림 2개는 장식") {
		t.Errorf("the decorations left out are not reported: %q", said)
	}

	// A deck that leaves nothing out says nothing.
	plain := []pptx.ImportedSlide{{Pictures: []pptx.ImportedPicture{picture("one.png", 300)}},
		{Pictures: []pptx.ImportedPicture{picture("two.png", 400)}}}
	quiet := newPictureFilter(plain)
	for _, slide := range plain {
		for _, one := range slide.Pictures {
			if !quiet.keeps(one) {
				t.Errorf("%s was left out of a deck with nothing to leave out", one.Name)
			}
		}
	}
	if said := quiet.leftOut(); len(said) != 0 {
		t.Errorf("an import that carried everything still said %q", said)
	}

	// Two slides are not enough to call a picture on both of them a logo.
	pair := []pptx.ImportedSlide{{Pictures: []pptx.ImportedPicture{picture("both.png", 200)}},
		{Pictures: []pptx.ImportedPicture{picture("both.png", 200)}}}
	small := newPictureFilter(pair)
	for _, slide := range pair {
		for _, one := range slide.Pictures {
			if !small.keeps(one) {
				t.Error("a picture on both slides of a two-slide deck was called a logo")
			}
		}
	}
}

// What the person is told about their pictures, when the design cannot draw
// them all.
func TestAnImportSaysHowManyPicturesTheDesignCouldDraw(t *testing.T) {
	t.Parallel()
	// Twenty-two carried, twelve drawn: the ten are the news.
	said := picturesLeftUndrawn(22, 12)
	for _, part := range []string{"12개를 슬라이드에 넣었습니다", "10개는", "이미지 탭"} {
		if !strings.Contains(said, part) {
			t.Errorf("the sentence does not say %q: %q", part, said)
		}
	}
	// Everything drawn is nothing to report.
	if said := picturesLeftUndrawn(12, 12); said != "" {
		t.Errorf("a deck that drew every picture still said %q", said)
	}
	// Nor is a deck with no pictures at all, or one that somehow drew more.
	if said := picturesLeftUndrawn(0, 0); said != "" {
		t.Errorf("a deck with no pictures said %q", said)
	}
	if said := picturesLeftUndrawn(3, 5); said != "" {
		t.Errorf("a deck that drew more than it carried said %q", said)
	}

	// And what counts as carried is what the import wrote into the source.
	source := "# 제목\n@picture\n::image 하나.png\n- 요점\n\n# 다음\n::image 둘.png\n::image 셋.png\n"
	if carried := picturesCarried(source); carried != 3 {
		t.Errorf("the source carries three pictures, counted %d", carried)
	}
	if carried := picturesCarried("# 제목\n- ::image 글에 적힌 말\n"); carried != 0 {
		t.Errorf("a point mentioning ::image was counted as a picture")
	}
}

// A sentence that begins "그 가운데" belongs next to the line it answers.
func TestWhatWasDrawnIsSaidBesideWhatWasSaved(t *testing.T) {
	t.Parallel()
	said := sayAfterPicturesSaved([]string{
		"그림 22개를 이미지 라이브러리에 저장했습니다",
		"표 1개를 이 덱의 디자인으로 다시 그렸습니다",
	}, "그 가운데 12개를 슬라이드에 넣었습니다")
	if len(said) != 3 || said[1] != "그 가운데 12개를 슬라이드에 넣었습니다" {
		t.Errorf("the sentence landed away from what it answers: %q", said)
	}
	// With nothing to answer, it still gets said rather than dropped.
	alone := sayAfterPicturesSaved([]string{"표 1개를 다시 그렸습니다"}, "그 가운데 1개를 넣었습니다")
	if len(alone) != 2 || alone[1] != "그 가운데 1개를 넣었습니다" {
		t.Errorf("the sentence was lost: %q", alone)
	}
}
