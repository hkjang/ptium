package deck

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/pptx"
)

// Several of the tools people generate decks with export each slide as one
// picture. Imported, that is a run of slides called "3번 슬라이드" with nothing on
// them — which is what the file holds, and there are no words to invent. But
// coming back to eighteen empty slides with the only message being "그림 2개를
// 저장했습니다" reads as the import having failed.
func TestAnImportOfPicturesOnlySaysThereWereNoWords(t *testing.T) {
	pictures := pptx.ImportedDeck{Slides: []pptx.ImportedSlide{{}, {}, {}}}
	source, warnings := SourceFromImport(pictures)
	if !strings.Contains(source, "1번 슬라이드") {
		t.Fatalf("the fixture did not import as untitled slides:\n%s", source)
	}
	said := strings.Join(warnings, " | ")
	if !strings.Contains(said, "3장") || !strings.Contains(said, "글자가 없습니다") {
		t.Errorf("an import of nothing but pictures said %q", said)
	}

	// Some of them wordless names which ones. This assertion used to read
	// `Contains(said, "1장")` — the count — and that is exactly the defect: the
	// wordless slide here is the second, and "1장에는" tells a reader it is the
	// first. They open the first slide, find words on it, and the product looks
	// like it is lying.
	mixed := pptx.ImportedDeck{Slides: []pptx.ImportedSlide{
		{Title: "표지"}, {}, {Bullets: []pptx.ImportedLine{{Text: "요점"}}},
	}}
	_, warnings = SourceFromImport(mixed)
	said = strings.Join(warnings, " | ")
	if !strings.Contains(said, "2번 슬라이드") {
		t.Errorf("one wordless slide, the second of three, was reported as %q", said)
	}
	if strings.Contains(said, "1장에는") {
		t.Errorf("the count was said where a place is read: %q", said)
	}

	// And a deck that reads normally says nothing about it.
	whole := pptx.ImportedDeck{Slides: []pptx.ImportedSlide{
		{Title: "표지"}, {Title: "현황", Bullets: []pptx.ImportedLine{{Text: "요점"}}},
	}}
	_, warnings = SourceFromImport(whole)
	for _, warning := range warnings {
		if strings.Contains(warning, "글자가 없") {
			t.Errorf("a deck that read normally was told %q", warning)
		}
	}
}

// Which slides, by their place in the deck, so the reader can go and look.
func TestWordlessSlidesAreNamedByTheirPlace(t *testing.T) {
	wordless := func(at map[int]bool, total int) string {
		slides := make([]pptx.ImportedSlide, 0, total)
		for index := 1; index <= total; index++ {
			if at[index] {
				slides = append(slides, pptx.ImportedSlide{})
				continue
			}
			slides = append(slides, pptx.ImportedSlide{Title: "제목", Bullets: []pptx.ImportedLine{{Text: "요점"}}})
		}
		_, warnings := SourceFromImport(pptx.ImportedDeck{Slides: slides})
		for _, warning := range warnings {
			if strings.Contains(warning, "글자가 없") {
				return warning
			}
		}
		return ""
	}

	if got := wordless(map[int]bool{4: true}, 5); !strings.Contains(got, "4번 슬라이드") {
		t.Errorf("the fourth slide of five was reported as %q", got)
	}
	if got := wordless(map[int]bool{2: true, 5: true, 6: true}, 8); !strings.Contains(got, "2·5·6번 슬라이드") {
		t.Errorf("three wordless slides were reported as %q", got)
	}
	// Past a handful the places stop being worth listing, and the count is said
	// in a form that cannot be read as a place.
	many := map[int]bool{}
	for index := 2; index <= 10; index++ {
		many[index] = true
	}
	got := wordless(many, 14)
	if !strings.Contains(got, "슬라이드 9장") {
		t.Errorf("nine wordless slides were reported as %q", got)
	}
	if strings.Contains(got, "번 슬라이드") {
		t.Errorf("nine places were listed one by one: %q", got)
	}
}

func TestSlidesNamedReadsAsPlacesNotCounts(t *testing.T) {
	if got := slidesNamed([]int{4}); got != "4번 슬라이드" {
		t.Errorf("one slide was named %q", got)
	}
	if got := slidesNamed([]int{2, 5, 6}); got != "2·5·6번 슬라이드" {
		t.Errorf("three slides were named %q", got)
	}
	if got := slidesNamed([]int{1, 2, 3, 4, 5, 6}); got != "1·2·3·4·5·6번 슬라이드" {
		t.Errorf("six slides were named %q", got)
	}
	if got := slidesNamed([]int{1, 2, 3, 4, 5, 6, 7}); got != "슬라이드 7장" {
		t.Errorf("seven slides were named %q", got)
	}
}
