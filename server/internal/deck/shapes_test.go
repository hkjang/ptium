package deck

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// Two rules hold a deck's text and its slides together:
//
//   - writing a deck out and reading it back gives the same deck, and
//   - writing one slide out gives what the whole deck says about that slide,
//     which is what saving a slide to reuse depends on.
//
// Both were checked against a single example deck, and both were broken for
// shapes that example did not have: a comparison slide whose second column
// heading sat mid-body lost that heading, and a deck edited on the canvas was
// exported with the source it no longer matched. One example cannot cover a
// language with fifteen components and eleven roles, so this walks the
// combinations instead.

// everyShape is one slide of every kind, in every role that can hold one, with
// and without the parts a slide can carry.
func everyShape() []string {
	var shapes []string
	for _, role := range []string{"content", "two", "comparison", "picture", "table", "chart", "section", "quote", "closing"} {
		for _, kind := range append([]string{""}, pptx.BlockKinds()...) {
			for _, extra := range []string{"", "> 리드 한 줄\n", "!notes 말할 것\n", "!source 내부 로그 | 지난 12개월\n"} {
				var slide strings.Builder
				fmt.Fprintf(&slide, "# %s %s 슬라이드\n@%s\n", role, kind, role)
				if strings.HasPrefix(extra, "> ") {
					slide.WriteString(extra)
				}
				slide.WriteString(sampleBlock(kind))
				if !strings.HasPrefix(extra, "> ") {
					slide.WriteString(extra)
				}
				shapes = append(shapes, slide.String())
			}
		}
	}
	// The shapes that broke before: two columns, each with its own heading.
	shapes = append(shapes,
		"# 투자 대비 리스크\n@comparison\n> 투자 비용\n- 12억 원 투자\n- 기술 부채 해소\n> 잠재 손실\n- 매출 손실\n- 고객 이탈\n",
		"# 두 칸\n@two\n> 왼쪽\n- 한 줄\n> 오른쪽\n- 다른 줄\n",
		"# 열 이름만\n@comparison\n> 왼쪽\n> 오른쪽\n- 한 줄\n",
	)
	return shapes
}

// sampleBlock is a small, valid component of that kind, or plain points when
// the kind is empty.
func sampleBlock(kind string) string {
	switch kind {
	case "":
		return "- 첫 번째 요점\n- 두 번째 요점\n"
	case pptx.BlockTable:
		return "::table 비교\n- 항목 | 지금 | 다음\n- 비용 | 1억 | 8천만\n::\n"
	case pptx.BlockQuote:
		return "::quote\n- 인용된 한 문장 | 말한 사람\n::\n"
	case pptx.BlockCallout:
		return "::callout 유의\n- 한 가지만 강조합니다\n::\n"
	case pptx.BlockComparison:
		return "::comparison 비교\n- 항목 | 지금 | 다음\n- 비용 | 1억 | 8천만\n::\n"
	case pptx.BlockBullets, pptx.BlockGrid:
		return fmt.Sprintf("::%s 목록\n- 첫 줄\n- 둘째 줄\n::\n", kind)
	default:
		return fmt.Sprintf("::%s 제목\n- 처리량 | 1,200건\n- 오류율 | 0.2%%\n::\n", kind)
	}
}

func TestEveryShapeSurvivesBeingWrittenBack(t *testing.T) {
	manifest := testManifest()
	for _, shape := range everyShape() {
		first := Compile(ParseSource(shape), manifest, CompileOptions{Language: "ko"})
		if len(first.Slides) == 0 {
			t.Errorf("this shape compiled to nothing:\n%s", shape)
			continue
		}
		formatted := Format(model.Presentation{Slides: first.Slides, Language: "ko"}, manifest)
		second := Compile(ParseSource(formatted), manifest, CompileOptions{Language: "ko"})
		if len(second.Slides) != len(first.Slides) {
			t.Errorf("the slide count changed:\n%s\nwritten back as:\n%s", shape, formatted)
			continue
		}
		before, after := Decode(first.Slides[0].Content), Decode(second.Slides[0].Content)
		switch {
		case first.Slides[0].Title != second.Slides[0].Title:
			t.Errorf("the title changed: %q then %q\n%s", first.Slides[0].Title, second.Slides[0].Title, formatted)
		case len(before.Blocks) != len(after.Blocks):
			t.Errorf("components changed: %d then %d\nfrom:\n%s\nwritten back as:\n%s",
				len(before.Blocks), len(after.Blocks), shape, formatted)
		case before.Notes != after.Notes:
			t.Errorf("the notes changed: %q then %q\n%s", before.Notes, after.Notes, formatted)
		case len(before.Sources) != len(after.Sources):
			t.Errorf("the sources changed: %d then %d\n%s", len(before.Sources), len(after.Sources), formatted)
		default:
			for slot, paragraphs := range before.Fields {
				if len(paragraphs) != len(after.Fields[slot]) {
					t.Errorf("region %q changed: %+v then %+v\nfrom:\n%s\nwritten back as:\n%s",
						slot, paragraphs, after.Fields[slot], shape, formatted)
				}
			}
		}
	}
}

// Saving a slide writes that slide alone, and a saved slide is inserted
// elsewhere from that text. If writing it alone says something else than the
// deck says about it, the slide someone saved is not the slide they get back.
func TestASlideWrittenAloneSaysWhatTheDeckSays(t *testing.T) {
	manifest := testManifest()
	deck := strings.Join(everyShape(), "\n")
	compiled := Compile(ParseSource(deck), manifest, CompileOptions{Language: "ko"})
	whole := Format(model.Presentation{Slides: compiled.Slides, Language: "ko"}, manifest)
	blocks := strings.Split(strings.TrimSpace(whole), "\n\n")
	if len(blocks) != len(compiled.Slides) {
		t.Fatalf("the deck wrote %d blocks for %d slides", len(blocks), len(compiled.Slides))
	}
	for index, slide := range compiled.Slides {
		alone := strings.TrimSpace(Format(model.Presentation{
			Slides: []model.Slide{slide}, Language: "ko"}, manifest))
		if alone != strings.TrimSpace(blocks[index]) {
			t.Errorf("slide %d is written differently on its own:\n  in the deck:\n%s\n  alone:\n%s",
				index+1, blocks[index], alone)
		}
	}
}
