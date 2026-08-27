package deck

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// A picture on a slide does not make the slide's words written.
//
// A deck imported from a real file put a photograph in the body region and the
// prose that had nowhere else to go into Body. Writing that slide back out saw
// the picture, decided the body had been written, and dropped the words: the
// source an author was shown held half the deck, and applying it — one click in
// the editor — deleted the other half from five slides out of ten.
func TestAPictureDoesNotSwallowTheWordsBesideIt(t *testing.T) {
	t.Parallel()
	content := Content{
		Type:     ContentType,
		LayoutID: "picture",
		Fields:   map[string][]pptx.Paragraph{pptx.SlotTitle: {{Text: "도입 배경 및 전략적 목표"}}},
		Images:   map[string]ContentImage{"body": {Name: "그림.png"}},
		Body:     "핵심 플랫폼 요구사항\nOLTP & OLAP 통합 기능 필수\n고가용성 확보",
	}
	written := Format(presentationOf(content), pptx.Manifest{})
	for _, line := range []string{"핵심 플랫폼 요구사항", "OLTP & OLAP 통합 기능 필수", "고가용성 확보"} {
		if !strings.Contains(written, line) {
			t.Errorf("the words beside the picture were dropped: %q missing from\n%s", line, written)
		}
	}
	if !strings.Contains(written, "그림.png") {
		t.Errorf("the picture was dropped:\n%s", written)
	}

	// Words that already have a slot of their own are not written twice.
	withFields := content
	withFields.Fields = map[string][]pptx.Paragraph{
		pptx.SlotTitle: {{Text: "제목"}},
		"body":         {{Text: "이미 자리를 가진 문장"}},
	}
	withFields.Body = "이미 자리를 가진 문장"
	twice := Format(presentationOf(withFields), pptx.Manifest{})
	if strings.Count(twice, "이미 자리를 가진 문장") != 1 {
		t.Errorf("a line with a slot of its own was written twice:\n%s", twice)
	}
}

func presentationOf(content Content) model.Presentation {
	raw, _ := json.Marshal(content)
	return model.Presentation{Slides: []model.Slide{{Position: 1, Title: firstText(content.Fields[pptx.SlotTitle]), Content: raw}}}
}

// A component's caption runs from the space after ::kind to the end of the
// line, and nothing in between is a mark.
//
// The one that was wrong: the caption was written through the guard that
// protects a line beginning with # or \, and the parser never undoes it — so
// the backslash stayed, and another arrived every time the source was written
// out from the slides. A caption of "# 매출" was "\\\\\\\\# 매출" after four
// rounds.
func TestABlockCaptionSurvivesBeingWrittenOut(t *testing.T) {
	for _, caption := range []string{"# 매출", "매출|비용", `경로\합계`, "매출 (억원)", "@표"} {
		block := pptx.Block{Kind: pptx.BlockColumns, Caption: caption,
			Items: []pptx.Item{{Label: "1분기", Value: "12"}, {Label: "2분기", Value: "15"}}}
		parsed := ParseSource("# 한 장\n" + formatBlock(block))
		got := ""
		for _, slide := range parsed.Slides {
			for _, found := range slide.Blocks {
				got = found.Caption
			}
		}
		if got != caption {
			t.Fatalf("a caption of %q came back as %q", caption, got)
		}
	}
}

// And it must not drift over repeated writings, which is how the marks piled up.
func TestABlockCaptionDoesNotDriftOverRounds(t *testing.T) {
	for _, start := range []string{"# 매출", `\경로`, "매출|비용"} {
		caption := start
		for round := 0; round < 5; round++ {
			parsed := ParseSource("# 한 장\n" + formatBlock(pptx.Block{
				Kind: pptx.BlockColumns, Caption: caption,
				Items: []pptx.Item{{Label: "1분기", Value: "12"}}}))
			for _, slide := range parsed.Slides {
				for _, found := range slide.Blocks {
					caption = found.Caption
				}
			}
			if caption != start {
				t.Fatalf("a caption of %q became %q after %d writings", start, caption, round+1)
			}
		}
	}
}

// A caption that runs onto another line would end the directive early, so it is
// flattened rather than carried.
func TestABlockCaptionIsFlattenedToOneLine(t *testing.T) {
	written := formatBlock(pptx.Block{Kind: pptx.BlockColumns, Caption: "매출\n비용",
		Items: []pptx.Item{{Label: "1분기", Value: "12"}}})
	first := strings.SplitN(written, "\n", 2)[0]
	if !strings.Contains(first, "매출 비용") {
		t.Fatalf("a caption over two lines was written as %q", first)
	}
}
