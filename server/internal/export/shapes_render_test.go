package export

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

func shapeSources() []string {
	var shapes []string
	for _, role := range []string{"content", "two", "comparison", "picture", "table", "chart", "section", "quote", "closing"} {
		for _, kind := range append([]string{""}, pptx.BlockKinds()...) {
			for _, extra := range []string{"", "> 리드 한 줄\n", "!notes 말할 것\n", "!source 내부 로그 | 지난 12개월\n"} {
				var slide strings.Builder
				fmt.Fprintf(&slide, "# %s %s 슬라이드\n@%s\n", role, kind, role)
				if strings.HasPrefix(extra, "> ") {
					slide.WriteString(extra)
				}
				slide.WriteString(shapeBlock(kind))
				if !strings.HasPrefix(extra, "> ") {
					slide.WriteString(extra)
				}
				shapes = append(shapes, slide.String())
			}
		}
	}
	return shapes
}

func shapeBlock(kind string) string {
	switch kind {
	case "":
		return "- 첫 번째 요점\n- 두 번째 요점\n"
	case pptx.BlockTable, pptx.BlockComparison:
		return fmt.Sprintf("::%s 비교\n- 임대료 | 지금 | 다음\n- 비용 | 1억 | 8천만\n::\n", kind)
	case pptx.BlockQuote:
		return "::quote\n- 인용된 한 문장 | 말한 사람\n::\n"
	case pptx.BlockCallout:
		return "::callout 유의\n- 한 가지만 강조합니다\n::\n"
	case pptx.BlockBullets, pptx.BlockGrid:
		return fmt.Sprintf("::%s 목록\n- 첫 줄\n- 둘째 줄\n::\n", kind)
	default:
		return fmt.Sprintf("::%s 제목\n- 처리량 | 1,200건\n- 오류율 | 0.2%%\n::\n", kind)
	}
}

// stripTags leaves only the text a drawing shows.
func stripTags(svg string) string {
	var text strings.Builder
	inside := false
	for _, r := range svg {
		switch {
		case r == '<':
			inside = true
			text.WriteRune(' ')
		case r == '>':
			inside = false
		case !inside:
			text.WriteRune(r)
		}
	}
	return text.String()
}

// Every shape, drawn. Nothing may be measured as a defect, no preview may come
// back blank, and the drawing has to hold the words the slide is made of.
//
// A divider is the exception and is named as one: its layout has a title and
// nothing else, so a component put on one has nowhere to be drawn. Compiling
// says so in a warning; this test holds the line that nothing else loses words.
func TestEveryShapeDrawsWhatItSays(t *testing.T) {
	data, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	defects := map[string]int{}
	byShape := map[string]int{}
	missing, blank, shown := 0, 0, 0
	for _, shape := range shapeSources() {
		compiled := deck.Compile(deck.ParseSource(shape), manifest, deck.CompileOptions{Language: "ko"})
		if len(compiled.Slides) == 0 {
			t.Errorf("compiled to nothing:\n%s", shape)
			continue
		}
		presentation := model.Presentation{Title: "모양 점검", Language: "ko", Slides: compiled.Slides}
		built := deck.Build(presentation, manifest, "")
		for _, finding := range pptx.InspectDeck(manifest, built) {
			if finding.Advisory {
				continue
			}
			defects[finding.Kind]++
			if shown < 10 {
				shown++
				t.Errorf("drawn with a defect — %s: %s\n%s", finding.Kind, finding.Detail, shape)
			}
		}
		svg, err := PreviewSlideSVG(presentation, manifest, 1, pptx.PreviewOptions{Width: 960}, nil)
		if err != nil {
			t.Errorf("preview failed: %v\n%s", err, shape)
			continue
		}
		if len(svg) < 400 {
			blank++
			t.Errorf("the preview is blank:\n%s", shape)
			continue
		}
		// Wrapping splits a line across elements, so the drawing is compared as
		// one run of text with the spacing taken out.
		drawn := strings.Join(strings.Fields(stripTags(svg)), "")
		for _, line := range strings.Split(shape, "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "- "), "> "))
			if line == "" || strings.HasPrefix(line, "@") || strings.HasPrefix(line, "!") ||
				strings.HasPrefix(line, "::") || strings.HasPrefix(line, "# ") {
				continue
			}
			word := strings.TrimSpace(strings.Split(line, "|")[0])
			if word == "" || strings.Contains(drawn, strings.Join(strings.Fields(word), "")) {
				continue
			}
			missing++
			head := strings.SplitN(shape, "\n", 2)[0]
			byShape[strings.TrimSuffix(strings.TrimPrefix(head, "# "), " 슬라이드")+" | "+word]++
			if !strings.HasPrefix(shape, "# section ") {
				t.Errorf("the drawing does not hold %q\n%s", word, shape)
			}
		}
	}
	kinds := make([]string, 0, len(defects))
	for kind := range defects {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	_, _ = shown, byShape
	t.Logf("shapes=%d defects=%v blank previews=%d words not drawn=%d",
		len(shapeSources()), func() string {
			var parts []string
			for _, kind := range kinds {
				parts = append(parts, fmt.Sprintf("%s=%d", kind, defects[kind]))
			}
			return strings.Join(parts, " ")
		}(), blank, missing)
}

// A slide whose layout has no subtitle region keeps its lead in the component's
// heading. Quote and callout cleared that heading before drawing, so the line
// the author wrote was on no slide and in no warning.
func TestAStatementComponentDrawsTheLineThatIntroducesIt(t *testing.T) {
	data, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, one := range []struct{ source, wanted string }{
		{"# 인용 점검\n@content\n> 고객이 한 말입니다\n::quote\n- 기다리는 게 제일 힘듭니다 | 고객 A\n::\n", "고객이 한 말입니다"},
		{"# 강조 점검\n@content\n> 여기만 보십시오\n::callout 유의\n- 3월까지 결정이 필요합니다\n::\n", "여기만 보십시오"},
		{"# 강조 점검\n@content\n::callout 유의\n- 3월까지 결정이 필요합니다\n::\n", "유의"},
	} {
		compiled := deck.Compile(deck.ParseSource(one.source), manifest, deck.CompileOptions{Language: "ko"})
		presentation := model.Presentation{Title: "점검", Language: "ko", Slides: compiled.Slides}
		svg, err := PreviewSlideSVG(presentation, manifest, 1, pptx.PreviewOptions{Width: 960}, nil)
		if err != nil {
			t.Fatalf("preview: %v", err)
		}
		if drawn := strings.Join(strings.Fields(stripTags(svg)), ""); !strings.Contains(drawn, strings.Join(strings.Fields(one.wanted), "")) {
			t.Errorf("the drawing does not hold %q:\n%s\ndrawn: %s", one.wanted, one.source, drawn)
		}
		// And it is measured as fitting: room for the line is asked for, not taken
		// from the statement.
		for _, finding := range pptx.InspectDeck(manifest, deck.Build(presentation, manifest, "")) {
			if !finding.Advisory {
				t.Errorf("%s: %s\n%s", finding.Kind, finding.Detail, one.source)
			}
		}
	}
}
