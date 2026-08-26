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

	// Some of them wordless is the same answer at a smaller size.
	mixed := pptx.ImportedDeck{Slides: []pptx.ImportedSlide{
		{Title: "표지"}, {}, {Bullets: []pptx.ImportedLine{{Text: "요점"}}},
	}}
	_, warnings = SourceFromImport(mixed)
	said = strings.Join(warnings, " | ")
	if !strings.Contains(said, "1장") {
		t.Errorf("one wordless slide among three was reported as %q", said)
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
