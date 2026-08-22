package deck

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

func diffManifest(t *testing.T) pptx.Manifest {
	t.Helper()
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	return manifest
}

func compiledSlides(t *testing.T, manifest pptx.Manifest, source string) []model.Slide {
	t.Helper()
	result := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	if len(result.Slides) == 0 {
		t.Fatalf("the source compiled to nothing: %v", result.Warnings)
	}
	for index := range result.Slides {
		result.Slides[index].ID = string(rune('a' + index))
		result.Slides[index].Position = index + 1
	}
	return result.Slides
}

// "What changed?" is asked of a version, and answered in slides: this one was
// rewritten, that one is new, that one is gone. A character diff of stored JSON
// answers a different question.
func TestCompareSaysWhatChangedSlideBySlide(t *testing.T) {
	manifest := diffManifest(t)
	before := compiledSlides(t, manifest, "# 표지\n@cover\n\n# 비용\n- 이관 비용 12억 원\n- 연간 운영 2.4억 원\n\n# 다음 단계\n@closing\n- 승인 요청\n")
	after := compiledSlides(t, manifest, "# 표지\n@cover\n\n# 비용\n- 이관 비용 14억 원\n- 연간 운영 2.4억 원\n\n# 리스크\n- 단일 리전 의존\n\n# 다음 단계\n@closing\n- 승인 요청\n")
	// The third slide of the new deck is new, so ids line up only for the first two.
	after[2].ID = "new"
	after[3].ID = "c"

	changes := Compare(before, after, manifest)
	byKind := map[string][]SlideChange{}
	for _, change := range changes {
		byKind[change.Kind] = append(byKind[change.Kind], change)
	}
	if len(byKind["changed"]) != 1 {
		t.Fatalf("expected one changed slide: %s", show(changes))
	}
	changed := byKind["changed"][0]
	if changed.Title != "비용" {
		t.Fatalf("the changed slide is %q", changed.Title)
	}
	if !strings.Contains(strings.Join(changed.Added, " "), "14억") ||
		!strings.Contains(strings.Join(changed.Removed, " "), "12억") {
		t.Fatalf("the change does not say what came and went: %+v", changed)
	}
	if len(byKind["added"]) != 1 || byKind["added"][0].Title != "리스크" {
		t.Fatalf("the new slide was not reported: %s", show(changes))
	}
	// The closing slide only moved.
	if len(byKind["moved"]) != 1 || byKind["moved"][0].From != 3 || byKind["moved"][0].Position != 4 {
		t.Fatalf("the moved slide was not reported: %s", show(changes))
	}
	if len(byKind["removed"]) != 0 {
		t.Fatalf("nothing was removed, but %s", show(changes))
	}
}

// A deck that did not change reports nothing at all, which is what makes the
// answer worth reading when it does report something.
func TestCompareSaysNothingWhenNothingChanged(t *testing.T) {
	manifest := diffManifest(t)
	source := "# 표지\n@cover\n\n# 비용\n- 이관 비용 12억 원\n"
	slides := compiledSlides(t, manifest, source)
	if changes := Compare(slides, slides, manifest); len(changes) != 0 {
		t.Fatalf("an unchanged deck reported %s", show(changes))
	}
}

// A deck someone regenerated has all new slide ids. Matching by position is the
// honest reading: the third slide changed, rather than three added and three
// removed.
func TestCompareMatchesByPositionWhenTheIdsAreAllNew(t *testing.T) {
	manifest := diffManifest(t)
	before := compiledSlides(t, manifest, "# 표지\n@cover\n\n# 비용\n- 12억 원\n")
	after := compiledSlides(t, manifest, "# 표지\n@cover\n\n# 비용\n- 14억 원\n")
	for index := range before {
		before[index].ID = ""
	}
	for index := range after {
		after[index].ID = ""
	}
	changes := Compare(before, after, manifest)
	if len(changes) != 1 || changes[0].Kind != "changed" {
		t.Fatalf("a regenerated deck reported %s", show(changes))
	}
}

func show(changes []SlideChange) string {
	out, _ := json.Marshal(changes)
	return string(out)
}
