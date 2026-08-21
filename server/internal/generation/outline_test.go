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

// A cover title is the most read string the product writes. It must keep the
// words that say what the deck is about, and never keep the auxiliary a removed
// verb left behind.
func TestDeckTitleKeepsTheSubjectAndDropsTheRequest(t *testing.T) {
	cases := []struct{ prompt, want string }{
		{"결제 시스템 이중화 계획을 실무진에게 설명하는 6장짜리 자료. 목표 가용성 99.95%", "결제 시스템 이중화 계획"},
		{"사내 개발팀의 AI 코딩 도구 도입 성과를 경영진에게 보고하는 8장짜리 덱", "사내 개발팀의 AI 코딩 도구 도입 성과"},
	}
	for _, testCase := range cases {
		got := TitleFor(testCase.prompt, testCase.prompt, "ko")
		if got != testCase.want {
			t.Errorf("TitleFor(%q) = %q, want %q", testCase.prompt, got, testCase.want)
		}
		if strings.Contains(got, "하는") || strings.Contains(got, "에게") {
			t.Errorf("the title carries the request rather than the subject: %q", got)
		}
	}
}

// A topic is written into headings and leads, so it must be the subject and not
// the request that produced it.
func TestTopicsDropTheAudienceAndWhatAStrippedVerbLeaves(t *testing.T) {
	outline := outlinePrompt("결제 시스템 이중화 계획을 실무진에게 설명하는 6장짜리 자료", "", koreanCopy)
	if len(outline.Topics) == 0 {
		t.Fatal("no topics")
	}
	first := outline.Topics[0].Name
	if first != "결제 시스템 이중화 계획" {
		t.Fatalf("topic = %q, want the subject with its leading words", first)
	}
	for _, topic := range outline.Topics {
		if strings.Contains(topic.Name, "에게") || strings.Contains(topic.Name, "하는") {
			t.Fatalf("a topic carries the request: %q", topic.Name)
		}
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

// Past half a dozen slides an audience wants to know where the deck is going;
// below that, a contents page is a slide spent saying what the next four say.
func TestLongDecksOpenWithTheirContents(t *testing.T) {
	outline := outlinePrompt("클라우드 전환 로드맵, 투자 타당성, 리스크 대응을 정리해줘", "", koreanCopy)
	for _, testCase := range []struct {
		count  int
		agenda bool
	}{{5, false}, {8, true}} {
		presentation := model.Presentation{Language: "ko", RequestedSlideCount: testCase.count}
		plan := newDeckPlan(outline, presentation, koreanCopy, "임원", "")
		parsed := deck.ParseSource(writeSource(outline, plan, testCase.count))
		if len(parsed.Slides) != testCase.count {
			t.Fatalf("asked for %d slides, wrote %d", testCase.count, len(parsed.Slides))
		}
		second := parsed.Slides[1].Title
		if got := second == "목차"; got != testCase.agenda {
			t.Fatalf("a %d-slide deck: contents page = %v, want %v (second slide is %q)",
				testCase.count, got, testCase.agenda, second)
		}
		if !testCase.agenda {
			continue
		}
		// The contents page names the sections, and nothing else.
		points := []string{}
		for _, bullet := range parsed.Slides[1].Bullets {
			points = append(points, bullet.Text)
		}
		if len(points) != len(outline.Topics) {
			t.Fatalf("the contents page lists %v for topics %+v", points, outline.Topics)
		}
	}
}

// A brief lists its figures the way it lists its subjects, and reading a figure
// as a subject gave it its own slides: a twelve-slide deck came out with the same
// step diagram three times and the same indicators three times.
func TestFiguresAreNotTopics(t *testing.T) {
	prompt := "결제 시스템 이중화 계획을 실무진에게 설명하는 12장짜리 자료. 목표 가용성 99.95%, 예산 4억, 이행 기간 8개월."
	outline := outlinePrompt(prompt, "", koreanCopy)
	if len(outline.Topics) != 1 || outline.Topics[0].Name != "결제 시스템 이중화 계획" {
		t.Fatalf("topics = %+v, want the one subject", outline.Topics)
	}
	if len(outline.Figures) != 3 {
		t.Fatalf("figures = %+v, want three", outline.Figures)
	}
	presentation := model.Presentation{Language: "ko", RequestedSlideCount: 12, Prompt: prompt}
	plan := newDeckPlan(outline, presentation, koreanCopy, audienceName("general", koreanCopy), "")
	parsed := deck.ParseSource(writeSource(outline, plan, 12))

	// One subject cannot honestly fill twelve slides, so the deck is shorter and
	// every slide is a different one.
	if len(parsed.Slides) > 10 {
		t.Fatalf("wrote %d slides for one subject", len(parsed.Slides))
	}
	seen := map[string]int{}
	for _, slide := range parsed.Slides {
		seen[strings.TrimSpace(slide.Title)]++
		body := slide.Lead
		for _, bullet := range slide.Bullets {
			body += "|" + bullet.Text
		}
		for _, block := range slide.Blocks {
			body += "|" + block.Kind
		}
		seen["body:"+body]++
	}
	for key, count := range seen {
		if count > 1 {
			t.Fatalf("the deck repeats %q %d times", key, count)
		}
	}
	// And the audience key never reaches the page.
	if strings.Contains(writeSource(outline, plan, 12), "general") {
		t.Fatal("the stored audience key was written into the deck")
	}
}

// A setting is a key; a cover is words.
func TestAudienceKeysBecomeWords(t *testing.T) {
	if got := audienceName("general", koreanCopy); got != "일반 청중" {
		t.Fatalf("audienceName(general) = %q", got)
	}
	if got := audienceName("executive", koreanCopy); got != "경영진" {
		t.Fatalf("audienceName(executive) = %q", got)
	}
	// What a person typed is theirs.
	if got := audienceName("현장 운영팀", koreanCopy); got != "현장 운영팀" {
		t.Fatalf("audienceName kept nothing of what the author wrote: %q", got)
	}
	// An untranslated key is still not something to print.
	if got := audienceName("stakeholders", koreanCopy); got != "일반 청중" {
		t.Fatalf("an unknown key reached the deck: %q", got)
	}
}

// Three subjects that all default to the same angle produced three slides with
// the same lead and the same shape, and a lead that opens with the words already
// in the title says everything twice.
func TestSlidesDoNotRepeatTheirTitlesOrEachOther(t *testing.T) {
	prompt := "국내 결제 인프라 이중화와 재해복구 체계 구축, 그리고 운영 조직 재편을 경영진에게 보고하는 자료"
	outline := outlinePrompt(prompt, "", koreanCopy)
	if len(outline.Topics) != 3 {
		t.Fatalf("topics = %+v, want three", outline.Topics)
	}
	presentation := model.Presentation{Language: "ko", RequestedSlideCount: 9, Prompt: prompt}
	plan := newDeckPlan(outline, presentation, koreanCopy, audienceName("executive", koreanCopy), "")
	parsed := deck.ParseSource(writeSource(outline, plan, 9))

	leads := map[string]int{}
	for _, slide := range parsed.Slides {
		lead := strings.TrimSpace(slide.Lead)
		if lead == "" {
			continue
		}
		if strings.HasPrefix(lead, strings.TrimSpace(slide.Title)) {
			t.Fatalf("the lead of %q repeats its own title: %q", slide.Title, lead)
		}
		leads[lead]++
		if leads[lead] > 1 {
			t.Fatalf("two slides open with the same line: %q", lead)
		}
	}
	// A cover title is the subject, not the tail of the brief.
	if !strings.HasPrefix(parsed.Slides[0].Title, "국내 결제 인프라") {
		t.Fatalf("the cover lost the words that say what the deck is about: %q", parsed.Slides[0].Title)
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

// A topic is written into headings and leads, so it has to read as a phrase
// rather than as a piece of the brief.
func TestTopicsAreShortEnoughToWriteWith(t *testing.T) {
	outline := outlinePrompt(
		"신규 채널 확장 계획을 임원에게 보고합니다. 목표 성장률 24%, 신규 채널 3개, 예산 12억입니다.",
		"성장 전략", koreanCopy)
	if len(outline.Topics) == 0 {
		t.Fatal("the prompt should produce at least one topic")
	}
	for _, topic := range outline.Topics {
		if length := len([]rune(topic.Name)); length > 16 {
			t.Fatalf("topic %q is %d characters; it will not fit inside a sentence", topic.Name, length)
		}
		if strings.Contains(topic.Name, "보고합니다") {
			t.Fatalf("a topic must not carry the instruction: %q", topic.Name)
		}
	}
}
