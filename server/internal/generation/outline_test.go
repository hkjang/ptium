package generation

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
)

func TestOutlinePromptReadsTopicsFiguresAndPeriod(t *testing.T) {
	outline := outlinePrompt("2026년 하반기 클라우드 전환 로드맵과 투자 타당성을 3장으로 정리해줘", "", koreanCopy)
	if outline.Period != "2026년 하반기" {
		t.Fatalf("period = %q", outline.Period)
	}
	// The instruction is not part of the subject, and the object particle is not
	// part of the topic.
	if strings.Contains(outline.Subject, "3장") || strings.Contains(outline.Subject, "정리해") {
		t.Fatalf("subject still carries the instruction: %q", outline.Subject)
	}
	if len(outline.Topics) != 2 {
		t.Fatalf("topics = %+v", outline.Topics)
	}
	if outline.Topics[0].Frame != frameSequence {
		t.Fatalf("a roadmap should be argued as a sequence: %+v", outline.Topics[0])
	}
	if outline.Topics[1].Name != "투자 타당성" || outline.Topics[1].Frame != frameCase {
		t.Fatalf("second topic = %+v", outline.Topics[1])
	}
}

func TestOutlinePromptKeepsWordsThatEndLikeParticles(t *testing.T) {
	// 효과, 속도, 자료 and 평가 all end in a syllable that is also a particle.
	outline := outlinePrompt("사내 AI 코딩 도구 도입 효과: 개발 속도 32% 개선, 12개월 ROI", "", koreanCopy)
	if outline.Topics[0].Name != "사내 AI 코딩 도구 도입 효과" {
		t.Fatalf("the topic lost its final syllable: %q", outline.Topics[0].Name)
	}
	figures := outline.Figures
	if len(figures) < 2 {
		t.Fatalf("figures = %+v", figures)
	}
	if figures[0].Label != "개발 속도" || figures[0].Value != "32%" {
		t.Fatalf("first figure = %+v", figures[0])
	}
	// "12개월" is a duration; reading it as twelve of something would be wrong.
	for _, figure := range figures {
		if figure.Value == "12개" {
			t.Fatalf("a duration was read as a count: %+v", figures)
		}
	}
}

func TestOutlineIgnoresAYearAsAFigure(t *testing.T) {
	outline := outlinePrompt("2026년 데이터 플랫폼 고도화 계획", "", koreanCopy)
	for _, figure := range outline.Figures {
		if strings.Contains(figure.Value, "2026") {
			t.Fatalf("the year was read as a figure: %+v", outline.Figures)
		}
	}
}

func TestDeckTitleIgnoresATitleSlicedFromThePrompt(t *testing.T) {
	prompt := "2026년 하반기 클라우드 전환 로드맵과 투자 타당성을 3장으로 정리해줘"
	outline := outlinePrompt(prompt, "", koreanCopy)
	// A client with no title of its own sends the first characters of the prompt.
	sliced := string([]rune(prompt)[:20])
	title := outline.deckTitle(sliced, prompt, " · ")
	if strings.Contains(title, "3장") || title == sliced {
		t.Fatalf("a sliced prompt was used as a title: %q", title)
	}
	// A title the author actually wrote is respected.
	if got := outline.deckTitle("클라우드 전환 계획", prompt, " · "); got != "클라우드 전환 계획" {
		t.Fatalf("an authored title was discarded: %q", got)
	}
}

func TestWriteSourceHonoursTheSlideCountAndVariesRepeatedTopics(t *testing.T) {
	outline := outlinePrompt("결제 시스템 도입 방안을 6장으로", "", koreanCopy)
	presentation := model.Presentation{Language: "ko", RequestedSlideCount: 6, Audience: "임원"}
	plan := newDeckPlan(outline, presentation, koreanCopy, "임원", "")
	source := writeSource(outline, plan, 6)

	parsed := deck.ParseSource(source)
	if len(parsed.Slides) != 6 {
		t.Fatalf("wrote %d slides, want 6:\n%s", len(parsed.Slides), source)
	}
	if len(parsed.Warnings) != 0 {
		t.Fatalf("generated source should parse cleanly: %v", parsed.Warnings)
	}
	// One topic across several slides must not repeat itself.
	seen := map[string]bool{}
	for _, slide := range parsed.Slides {
		body := slide.Lead
		for _, bullet := range slide.Bullets {
			body += "|" + bullet.Text
		}
		for _, block := range slide.Blocks {
			body += block.Kind
		}
		if seen[body] {
			t.Fatalf("a slide was repeated verbatim:\n%s", source)
		}
		seen[body] = true
	}
	// The deck opens and closes.
	if parsed.Slides[0].Role != "title" {
		t.Fatalf("first slide role = %q", parsed.Slides[0].Role)
	}
	if parsed.Slides[len(parsed.Slides)-1].Role != "closing" {
		t.Fatalf("last slide role = %q", parsed.Slides[len(parsed.Slides)-1].Role)
	}
}

func TestWriteSourceShortDeckHasNoClosingSlide(t *testing.T) {
	outline := outlinePrompt("클라우드 전환 로드맵과 투자 타당성", "", koreanCopy)
	presentation := model.Presentation{Language: "ko", RequestedSlideCount: 3}
	plan := newDeckPlan(outline, presentation, koreanCopy, "임원", "")
	parsed := deck.ParseSource(writeSource(outline, plan, 3))
	if len(parsed.Slides) != 3 {
		t.Fatalf("wrote %d slides, want 3", len(parsed.Slides))
	}
	// Three slides asked for means three slides of substance, not two and a
	// thank-you.
	titles := []string{parsed.Slides[1].Title, parsed.Slides[2].Title}
	if !strings.Contains(titles[0], "로드맵") || !strings.Contains(titles[1], "타당성") {
		t.Fatalf("both topics should have their own slide, got %v", titles)
	}
}
