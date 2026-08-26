package deck

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/pptx"
)

func linesOf(texts ...string) []pptx.ImportedLine {
	lines := make([]pptx.ImportedLine, 0, len(texts))
	for _, text := range texts {
		lines = append(lines, pptx.ImportedLine{Text: text})
	}
	return lines
}

// A designer puts the section a slide belongs to in its corner, on every slide
// of that section, and the slide's own heading goes in the body underneath. Read
// as drawn, a twenty-two slide deck came in with fifteen slides called
// "3. 아이디어 구현" — an outline nobody can use.
func TestARepeatedHeaderIsNotEverySlidesName(t *testing.T) {
	slides := []pptx.ImportedSlide{
		{Title: "3. 아이디어 구현", Bullets: linesOf("**키워드 추출**", "형태소 기준으로 뽑습니다")},
		{Title: "3. 아이디어 구현", Bullets: linesOf("**화자 분리**", "두 사람을 나눕니다")},
		{Title: "3. 아이디어 구현", Bullets: linesOf("**감정 분석**")},
		{Title: "4. 기대효과", Bullets: linesOf("비용 절감")},
	}
	source, warnings := SourceFromImport(pptx.ImportedDeck{Slides: slides})
	for _, want := range []string{"# 키워드 추출", "# 화자 분리", "# 감정 분석", "# 4. 기대효과"} {
		if !strings.Contains(source, want) {
			t.Errorf("the deck has no %q:\n%s", want, source)
		}
	}
	if strings.Contains(source, "# 3. 아이디어 구현") {
		t.Error("the section marker is still a slide's name")
	}
	if !strings.Contains(strings.Join(warnings, " "), "3. 아이디어 구현") {
		t.Errorf("the header was dropped without saying so: %v", warnings)
	}
	// The promoted line is a heading now, and a heading cannot carry emphasis.
	if strings.Contains(source, "# **") {
		t.Error("a promoted heading kept its emphasis marks")
	}
}

// A weekly report headed the same on every slide, over the same subheading, has
// nothing else to be called. Taking the header off would leave "2번 슬라이드".
func TestASlideWithNoOtherNameKeepsTheOneItHas(t *testing.T) {
	slides := make([]pptx.ImportedSlide, 4)
	for index := range slides {
		slides[index] = pptx.ImportedSlide{
			Title:   "주간업무 추진실적 및 계획",
			Bullets: linesOf("AI엔지니어링 파트"),
		}
	}
	source, warnings := SourceFromImport(pptx.ImportedDeck{Slides: slides})
	if strings.Count(source, "# 주간업무 추진실적 및 계획") != 4 {
		t.Errorf("a report whose slides have no other name lost its heading:\n%s", source)
	}
	for _, warning := range warnings {
		if strings.Contains(warning, "머리글") {
			t.Errorf("a heading that is the slide's own name was reported as a header: %q", warning)
		}
	}
}
