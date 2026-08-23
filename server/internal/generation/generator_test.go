package generation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/library"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

type testSettings map[string]any

func (settings testSettings) Get(_ context.Context, key string, target any) error {
	value, ok := settings[key]
	if !ok {
		return errors.New("not found")
	}
	encoded, _ := json.Marshal(value)
	return json.Unmarshal(encoded, target)
}

func testTemplate(t *testing.T) Template {
	t.Helper()
	data, err := pptx.BuiltinTemplate("plum-rail")
	if err != nil {
		t.Fatalf("BuiltinTemplate: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatalf("AnalyzeBytes: %v", err)
	}
	return Template{ID: "tpl", Name: "Ptium Aurora", Manifest: manifest}
}

func TestFallbackIsDeterministicAndSized(t *testing.T) {
	template := testTemplate(t)
	p := model.Presentation{Title: "Plan", Prompt: "Growth", Language: "en", RequestedSlideCount: 6}
	first, second := Fallback(p, model.Profile{}, template), Fallback(p, model.Profile{}, template)
	if len(first.Slides) != 6 || string(first.Outline) != string(second.Outline) {
		t.Fatalf("unexpected fallback deck: %#v", first)
	}
	if string(first.Slides[0].Content) != string(second.Slides[0].Content) {
		t.Fatal("fallback deck must be deterministic")
	}
}

func TestFallbackBuildsANarrativeArcWithLayoutVariety(t *testing.T) {
	template := testTemplate(t)
	p := model.Presentation{Title: "성장 전략", Prompt: "국내 재진입", Language: "ko", RequestedSlideCount: 10}
	generated := Fallback(p, model.Profile{Company: "코리아크레딧뷰로"}, template)
	// One subject argued from four angles, the closing questions asked once, and
	// a cover and a close: that is what this brief honestly holds. A deck padded
	// out to exactly ten would repeat pages, so a short deck says so instead.
	if len(generated.Slides) < 8 || len(generated.Slides) > 10 {
		t.Fatalf("expected a deck of 8 to 10 slides, got %d", len(generated.Slides))
	}
	if len(generated.Slides) < 10 && len(generated.Warnings) == 0 {
		t.Fatal("a deck shorter than asked for must say why")
	}
	// And no page may appear twice.
	seen := map[string]bool{}
	for _, slide := range generated.Slides {
		if seen[slide.Title] {
			t.Fatalf("the deck repeats the slide %q", slide.Title)
		}
		seen[slide.Title] = true
	}
	if generated.Slides[0].Layout != pptx.RoleTitle {
		t.Fatalf("deck must open on a title layout, got %q", generated.Slides[0].Layout)
	}
	if last := generated.Slides[len(generated.Slides)-1]; last.Layout != pptx.RoleClosing {
		t.Fatalf("deck must close on a closing layout, got %q", last.Layout)
	}
	roles := map[string]int{}
	for _, slide := range generated.Slides {
		roles[slide.Layout]++
		if slide.LayoutID == "" {
			t.Fatalf("slide %d has no layout binding", slide.Position)
		}
		if _, ok := template.Manifest.Layout(slide.LayoutID); !ok {
			t.Fatalf("slide %d references unknown layout %q", slide.Position, slide.LayoutID)
		}
	}
	if len(roles) < 3 {
		t.Fatalf("a ten-slide deck should use more than %d layout roles: %v", len(roles), roles)
	}
	for _, slide := range generated.Slides {
		if strings.TrimSpace(slide.SpeakerNotes) == "" {
			t.Fatalf("slide %d has no speaker notes", slide.Position)
		}
		content := deck.Decode(slide.Content)
		if _, ok := content.Fields[pptx.SlotTitle]; !ok {
			t.Fatalf("slide %d has no title field: %s", slide.Position, slide.Content)
		}
	}
}

func TestFallbackFitsCopyIntoTemplateSlots(t *testing.T) {
	template := testTemplate(t)
	p := model.Presentation{Title: "Plan", Prompt: "Growth", Language: "en", RequestedSlideCount: 8}
	generated := Fallback(p, model.Profile{}, template)
	for _, slide := range generated.Slides {
		layout, ok := template.Manifest.Layout(slide.LayoutID)
		if !ok {
			t.Fatalf("unknown layout %q", slide.LayoutID)
		}
		content := deck.Decode(slide.Content)
		for slot, paragraphs := range content.Fields {
			placeholder, exists := layout.Slot(slot)
			if !exists {
				t.Fatalf("slide %d writes into slot %q which layout %q does not have", slide.Position, slot, layout.ID)
			}
			if len(paragraphs) > placeholder.MaxLines && placeholder.MaxLines > 0 {
				t.Fatalf("slide %d put %d paragraphs into a %d-line slot", slide.Position, len(paragraphs), placeholder.MaxLines)
			}
		}
	}
}

func TestFallbackUsesProfileBrandColor(t *testing.T) {
	p := model.Presentation{Title: "Plan", Prompt: "Growth", Language: "en", RequestedSlideCount: 2}
	generated := Fallback(p, model.Profile{Preferences: json.RawMessage(`{"brandColor":"#12abEF"}`)}, testTemplate(t))
	if deck.Decode(generated.Slides[0].Content).Accent != "#12ABEF" {
		t.Fatalf("profile brand color was not applied: %s", generated.Slides[0].Content)
	}
}

func TestFallbackLocalizesPersonalization(t *testing.T) {
	p := model.Presentation{Title: "計画", Prompt: "成長", Language: "ja", Audience: "経営陣", Tone: "簡潔", RequestedSlideCount: 4}
	generated := Fallback(p, model.Profile{Company: "Ptium", JobTitle: "PM"}, testTemplate(t))
	joined := ""
	for _, slide := range generated.Slides {
		joined += string(slide.Content) + slide.SpeakerNotes
	}
	if !strings.Contains(joined, "経営陣") || !strings.Contains(joined, "Ptium") {
		t.Fatalf("localized personalization was not applied: %s", joined)
	}
}

func TestGenerateUsesGlobalBrandWhenProfileHasNoOverride(t *testing.T) {
	generator := New(testSettings{"branding.brand_color": "#3456ab", "ai.provider": "fallback"})
	generated, err := generator.Generate(context.Background(),
		model.Presentation{Title: "Plan", Prompt: "Growth", Language: "en", RequestedSlideCount: 1},
		model.Profile{Preferences: json.RawMessage(`{}`)}, testTemplate(t))
	if err != nil || deck.Decode(generated.Slides[0].Content).Accent != "#3456AB" {
		t.Fatalf("global brand color was not applied: deck=%#v err=%v", generated, err)
	}
}

// stubProvider replays canned completions in order and records the prompts it
// received so tests can assert what the model was told.
type stubProvider struct {
	responses []string
	prompts   []string
	server    *httptest.Server
}

func newStubProvider(t *testing.T, responses ...string) *stubProvider {
	t.Helper()
	stub := &stubProvider{responses: responses}
	stub.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		stub.prompts = append(stub.prompts, string(body))
		content := "{}"
		if len(stub.responses) > 0 {
			content = stub.responses[0]
			stub.responses = stub.responses[1:]
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content}}}})
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func (s *stubProvider) generator() *Generator {
	generator := New(testSettings{
		"ai.provider":             "openai-compatible",
		"ai.base_url":             s.server.URL,
		"ai.model":                "test-model",
		"ai.api_key":              "test-key",
		"generation.outline_pass": false,
		"branding.brand_color":    "#7C3AED",
	})
	generator.client = s.server.Client()
	return generator
}

func writtenSlides(count int, layoutID string) string {
	slides := make([]map[string]any, count)
	for index := range slides {
		slides[index] = map[string]any{
			"layoutId": layoutID,
			"title":    "제목",
			"fields":   map[string]any{"body": []string{"첫 번째 요점", "두 번째 요점"}},
			"notes":    "설명",
		}
	}
	encoded, _ := json.Marshal(map[string]any{"slides": slides})
	return string(encoded)
}

func TestGenerateAdjustsAModelsSlideCountInsteadOfFailing(t *testing.T) {
	template := testTemplate(t)
	for _, test := range []struct {
		name       string
		returned   int
		wantSlides int
		wantWarn   bool
	}{
		// A deck someone is waiting for is delivered and annotated, not discarded.
		{name: "short", returned: 2, wantSlides: 2, wantWarn: true},
		{name: "long", returned: 5, wantSlides: 3, wantWarn: true},
		{name: "exact", returned: 3, wantSlides: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := newStubProvider(t, writtenSlides(test.returned, template.Manifest.DefaultLayout))
			generated, err := stub.generator().Generate(context.Background(),
				model.Presentation{Title: "Plan", Prompt: "Growth", Language: "ko", RequestedSlideCount: 3}, model.Profile{}, template)
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}
			if len(generated.Slides) != test.wantSlides {
				t.Fatalf("deck size = %d, want %d", len(generated.Slides), test.wantSlides)
			}
			if test.wantWarn && len(generated.Warnings) == 0 {
				t.Fatal("an adjusted slide count must be reported")
			}
			if !test.wantWarn && len(generated.Warnings) != 0 {
				t.Fatalf("an exact deck must not warn: %v", generated.Warnings)
			}
			// Every generated deck is editable as text.
			if strings.TrimSpace(generated.Source) == "" {
				t.Fatal("a generated deck must carry its source")
			}
		})
	}
}

func TestGenerateSendsTheLayoutCatalogToTheModel(t *testing.T) {
	template := testTemplate(t)
	stub := newStubProvider(t, writtenSlides(2, template.Manifest.DefaultLayout))
	_, err := stub.generator().Generate(context.Background(),
		model.Presentation{Title: "Plan", Prompt: "Growth", Language: "ko", Audience: "임원", Tone: "간결", RequestedSlideCount: 2},
		model.Profile{Company: "KCB"}, template)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(stub.prompts) != 1 {
		t.Fatalf("expected a single completion call, got %d", len(stub.prompts))
	}
	prompt := stub.prompts[0]
	for _, expected := range []string{template.Manifest.DefaultLayout, "maxChars", "임원", "간결", "KCB"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt is missing %q:\n%s", expected, prompt)
		}
	}
}

func TestGenerateRunsAnOutlinePassWhenEnabled(t *testing.T) {
	template := testTemplate(t)
	plan, _ := json.Marshal(map[string]any{
		"deckTitle": "성장 전략", "thesis": "온보딩이 병목이다",
		"slides": []map[string]any{
			{"role": "title", "layoutId": template.Manifest.TitleLayout, "headline": "성장 전략", "keyPoints": []string{"배경"}},
			{"role": "content", "layoutId": template.Manifest.DefaultLayout, "headline": "핵심 진단", "keyPoints": []string{"이탈 집중"}},
			{"role": "closing", "layoutId": template.Manifest.ClosingLayout, "headline": "다음 단계", "keyPoints": []string{"담당자 확정"}},
		},
	})
	stub := newStubProvider(t, string(plan), writtenSlides(3, template.Manifest.DefaultLayout))
	generator := stub.generator()
	generator.settings = testSettings{
		"ai.provider": "openai-compatible", "ai.base_url": stub.server.URL,
		"ai.model": "test-model", "ai.api_key": "test-key", "generation.outline_pass": true,
	}
	generated, err := generator.Generate(context.Background(),
		model.Presentation{Title: "성장 전략", Prompt: "재진입", Language: "ko", RequestedSlideCount: 3}, model.Profile{}, template)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(stub.prompts) != 2 {
		t.Fatalf("expected an outline pass and a writing pass, got %d calls", len(stub.prompts))
	}
	if !strings.Contains(stub.prompts[1], "온보딩이 병목이다") {
		t.Fatalf("the writing pass did not receive the approved plan:\n%s", stub.prompts[1])
	}
	if len(generated.Slides) != 3 {
		t.Fatalf("expected 3 slides, got %d", len(generated.Slides))
	}
}

func TestComposeRepairsUnusableModelOutput(t *testing.T) {
	template := testTemplate(t)
	response, _ := json.Marshal(map[string]any{"slides": []map[string]any{
		{
			// Unknown layout, unknown slot, markdown bullets and a missing title.
			"layoutId": "does-not-exist",
			"role":     "content",
			"fields": map[string]any{
				"content":     []string{"- 첫 번째 요점", "  * 근거가 되는 세부", "**두 번째 요점**"},
				"nonexistent": "버려져야 한다",
			},
			"notes": "노트",
		},
	}})
	stub := newStubProvider(t, string(response))
	generated, err := stub.generator().Generate(context.Background(),
		model.Presentation{Title: "회복 테스트", Prompt: "p", Language: "ko", RequestedSlideCount: 1}, model.Profile{}, template)
	if err != nil {
		t.Fatalf("Generate should recover, got %v", err)
	}
	slide := generated.Slides[0]
	if _, ok := template.Manifest.Layout(slide.LayoutID); !ok {
		t.Fatalf("unknown layout was not repaired: %q", slide.LayoutID)
	}
	content := deck.Decode(slide.Content)
	if _, ok := content.Fields["nonexistent"]; ok {
		t.Fatal("an invented slot reached storage")
	}
	body := content.Fields[pptx.SlotBody]
	if len(body) != 3 {
		t.Fatalf("expected three body paragraphs, got %#v", body)
	}
	if body[0].Text != "첫 번째 요점" {
		t.Fatalf("markdown bullet markers were not stripped: %q", body[0].Text)
	}
	if body[1].Level != 1 {
		t.Fatalf("indented line should become a sub-bullet: %#v", body[1])
	}
	if body[2].Text != "두 번째 요점" {
		t.Fatalf("markdown emphasis was not stripped: %q", body[2].Text)
	}
	if strings.TrimSpace(slide.Title) == "" {
		t.Fatal("a missing title was not backfilled")
	}
}

func TestGenerateRejectsMissingTemplate(t *testing.T) {
	generator := New(testSettings{"ai.provider": "fallback"})
	if _, err := generator.Generate(context.Background(),
		model.Presentation{Title: "x", RequestedSlideCount: 3}, model.Profile{}, Template{}); err == nil {
		t.Fatal("generation without a template must fail")
	}
}

func TestGenerateWritesAndCompilesDeckSource(t *testing.T) {
	template := testTemplate(t)
	// A model routinely wraps its answer in a fence even when told not to.
	response := "```\n" + `# 전환은 지금 결정해야 합니다
@cover
> 2026년 하반기 · 임원 보고
!notes 결론부터 말하고 근거를 두 가지로 좁힙니다.

# 전환 대상과 규모
> 42개 시스템을 세 묶음으로 나눴습니다.
::kpi 규모
- 전환 대상 | 42개
- 1차 범위 | 12개
::

# 다음 단계
@closing
- 오늘 요청하는 결정 한 가지
- 30일 내 착수 항목
` + "\n```"
	stub := newStubProvider(t, response)
	generated, err := stub.generator().Generate(context.Background(),
		model.Presentation{Title: "전환", Prompt: "클라우드 전환", Language: "ko", RequestedSlideCount: 3},
		model.Profile{}, template)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(generated.Slides) != 3 {
		t.Fatalf("slides = %d, warnings %v", len(generated.Slides), generated.Warnings)
	}
	// The fence is stripped and the source is what the model wrote.
	if strings.Contains(generated.Source, "```") || !strings.Contains(generated.Source, "# 전환 대상과 규모") {
		t.Fatalf("source = %q", generated.Source)
	}
	// Slides are bound to the template's real layouts, and the component was kept.
	if generated.Slides[0].LayoutID != template.Manifest.TitleLayout {
		t.Fatalf("cover layout = %q, want %q", generated.Slides[0].LayoutID, template.Manifest.TitleLayout)
	}
	content := deck.Decode(generated.Slides[1].Content)
	if len(content.Blocks) != 1 {
		t.Fatalf("the kpi component was not bound: %+v", content)
	}
	// The prompt asked the model for prose, not JSON: forcing JSON mode would make
	// it wrap the deck in a string field.
	if strings.Contains(stub.prompts[0], "json_object") {
		t.Fatal("the writing pass must not request JSON mode")
	}
	if !strings.Contains(stub.prompts[0], "slide language") {
		t.Fatalf("the model was not asked for the slide language: %s", stub.prompts[0])
	}
}

func TestGenerateTrimsAModelThatOverwritesTheSlideCount(t *testing.T) {
	template := testTemplate(t)
	var builder strings.Builder
	for index := 1; index <= 6; index++ {
		fmt.Fprintf(&builder, "# 슬라이드 %d\n- 요점 하나\n- 요점 둘\n\n", index)
	}
	stub := newStubProvider(t, builder.String())
	generated, err := stub.generator().Generate(context.Background(),
		model.Presentation{Title: "계획", Prompt: "성장", Language: "ko", RequestedSlideCount: 3},
		model.Profile{}, template)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(generated.Slides) != 3 {
		t.Fatalf("slides = %d, want 3", len(generated.Slides))
	}
	if generated.Slides[0].Title != "슬라이드 1" || generated.Slides[2].Title != "슬라이드 3" {
		t.Fatalf("the wrong slides were kept: %q … %q", generated.Slides[0].Title, generated.Slides[2].Title)
	}
	if len(generated.Warnings) == 0 {
		t.Fatal("an adjusted slide count must be reported")
	}
	// The stored source matches the slides that were kept.
	if strings.Contains(generated.Source, "슬라이드 4") {
		t.Fatalf("source still carries the dropped slides:\n%s", generated.Source)
	}
}

// A self-hosted reasoning model is the case a stub cannot teach you about: it
// answers with thinking and no content, and it thinks for longer than any sane
// timeout, so waiting to find out costs a timeout and produces nothing.
func TestGenerateAsksTheProviderNotToThink(t *testing.T) {
	template := testTemplate(t)
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		bodies = append(bodies, string(body))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"# 제목\n@content\n- 요점 하나\n- 요점 둘\n"}}]}`))
	}))
	defer server.Close()

	generator := New(testSettings{
		"ai.provider": "openai-compatible", "ai.base_url": server.URL,
		"ai.model": "local", "ai.api_key": "k", "generation.outline_pass": false,
	})
	generator.client = server.Client()
	if _, err := generator.Generate(context.Background(),
		model.Presentation{Title: "계획", Prompt: "성장", Language: "ko", RequestedSlideCount: 1},
		model.Profile{}, template); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(bodies) != 1 {
		t.Fatalf("expected one request, got %d", len(bodies))
	}
	if !strings.Contains(bodies[0], `"enable_thinking":false`) {
		t.Fatalf("the first request must ask the provider not to think:\n%s", bodies[0])
	}
	// A deck's source is thousands of tokens; without a bound a reasoning model
	// spends the whole context on thinking.
	if !strings.Contains(bodies[0], `"max_tokens"`) {
		t.Fatalf("the request must bound its output:\n%s", bodies[0])
	}
}

func TestGenerateStopsAskingWhenTheProviderRefuses(t *testing.T) {
	template := testTemplate(t)
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		bodies = append(bodies, string(body))
		writer.Header().Set("Content-Type", "application/json")
		// A hosted API rejects a body field it does not know.
		if strings.Contains(string(body), "chat_template_kwargs") {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = writer.Write([]byte(`{"error":{"message":"Unrecognized request argument: chat_template_kwargs"}}`))
			return
		}
		_, _ = writer.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"# 제목\n@content\n- 요점 하나\n"}}]}`))
	}))
	defer server.Close()

	generator := New(testSettings{
		"ai.provider": "openai-compatible", "ai.base_url": server.URL,
		"ai.model": "gpt", "ai.api_key": "k", "generation.outline_pass": false,
	})
	generator.client = server.Client()
	generated, err := generator.Generate(context.Background(),
		model.Presentation{Title: "계획", Prompt: "성장", Language: "ko", RequestedSlideCount: 1},
		model.Profile{}, template)
	if err != nil {
		t.Fatalf("a provider that refuses the field must still produce a deck: %v", err)
	}
	if len(generated.Slides) != 1 {
		t.Fatalf("slides = %d", len(generated.Slides))
	}
	if len(bodies) != 2 {
		t.Fatalf("expected a retry, got %d requests", len(bodies))
	}
	if strings.Contains(bodies[1], "chat_template_kwargs") {
		t.Fatalf("the retry must drop the field:\n%s", bodies[1])
	}
}

func TestGenerateReportsAProviderThatOnlyThinks(t *testing.T) {
	template := testTemplate(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		// vLLM's shape for a reasoning model: no content, thinking of its own.
		_, _ = writer.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"role":"assistant","content":null,"reasoning":"Thinking about the deck..."}}]}`))
	}))
	defer server.Close()

	generator := New(testSettings{
		"ai.provider": "openai-compatible", "ai.base_url": server.URL,
		"ai.model": "local", "ai.api_key": "k", "generation.outline_pass": false,
		"ai.reasoning": "on",
	})
	generator.client = server.Client()
	_, err := generator.Generate(context.Background(),
		model.Presentation{Title: "계획", Prompt: "성장", Language: "ko", RequestedSlideCount: 1},
		model.Profile{}, template)
	if err == nil {
		t.Fatal("reasoning without an answer must be reported, not treated as a deck")
	}
	if !strings.Contains(err.Error(), "reasoning") {
		t.Fatalf("the error should name the cause: %v", err)
	}
}

// The repair pass has to pick the right slides and judge its own results. The
// model is asked, not trusted: a rewrite is kept only if it measures better.
func TestRepairPicksTheWorstSlidesAndMeasuresTheResult(t *testing.T) {
	template := testTemplate(t)
	manifest := template.Manifest
	layout, ok := manifest.Layout(manifest.DefaultLayout)
	if !ok {
		t.Fatal("the built-in template has no default layout")
	}
	body := pptx.SlotBody
	for _, placeholder := range layout.BodySlots() {
		if placeholder.MaxLines >= 3 {
			body = placeholder.Slot
			break
		}
	}
	long := strings.Repeat("이 문장은 한 줄에 담기지 않을 만큼 길게 늘어놓은 설명입니다. ", 6)
	slide := func(title string, lines ...string) model.Slide {
		content := deck.Content{Type: deck.ContentType, LayoutID: layout.ID,
			Fields: map[string][]pptx.Paragraph{pptx.SlotTitle: {{Text: title}}}}
		paragraphs := make([]pptx.Paragraph, 0, len(lines))
		for _, line := range lines {
			paragraphs = append(paragraphs, pptx.Paragraph{Text: line})
		}
		if len(paragraphs) > 0 {
			content.Fields[body] = paragraphs
		}
		return model.Slide{Position: 1, Title: title, Content: content.Encode(), LayoutID: layout.ID}
	}
	presentation := model.Presentation{Title: "측정", Language: "ko"}
	presentation.Slides = []model.Slide{
		slide("괜찮은 장", "짧은 요점 하나", "짧은 요점 둘"),
		slide("넘치는 장", long, long, long, long),
	}
	for index := range presentation.Slides {
		presentation.Slides[index].Position = index + 1
	}

	worst := slidesByDefect(manifest, presentation)
	if len(worst) != 1 || worst[0].position != 2 {
		t.Fatalf("the overflowing slide should be the one to repair: %+v", worst)
	}
	if len(worst[0].details) == 0 {
		t.Fatal("a repair request carries the measurement, not an adjective")
	}

	// A shorter rewrite measures better; the original does not.
	shorter := slide("넘치는 장", "한 줄로 줄인 요점")
	if after := defectsOnSlide(manifest, presentation, 2, shorter); after >= worst[0].count {
		t.Fatalf("a slide that fits must measure better: %d vs %d", after, worst[0].count)
	}
	if same := defectsOnSlide(manifest, presentation, 2, presentation.Slides[1]); same != worst[0].count {
		t.Fatalf("measuring the same slide twice must agree: %d vs %d", same, worst[0].count)
	}
}

// A reasoning model that ignores the switch and thinks through its whole budget
// returns nothing at all. Asking again, plainly, turns a failed generation into
// a deck.
func TestGeneratorAsksAgainWhenTheModelOnlyThinks(t *testing.T) {
	var prompts []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		var decoded completionRequest
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("request body: %v", err)
		}
		prompts = append(prompts, decoded.Messages[len(decoded.Messages)-1].Content)
		writer.Header().Set("Content-Type", "application/json")
		if len(prompts) == 1 {
			// The first answer is all thinking and no content.
			fmt.Fprint(writer, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"","reasoning":"먼저 생각을 좀…"}}]}`)
			return
		}
		fmt.Fprint(writer, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"# 다시 물었습니다\n@cover\n"}}]}`)
	}))
	defer server.Close()

	generator := New(testSettings{
		"ai.provider": "openai-compatible", "ai.base_url": server.URL, "ai.model": "test", "ai.api_key": "k",
		"generation.outline_pass": false, "generation.repair_passes": 0,
	})
	generator.client = server.Client()
	deckOut, err := generator.Generate(context.Background(),
		model.Presentation{Title: "다시", Prompt: "한 장짜리", Language: "ko", RequestedSlideCount: 1},
		model.Profile{}, testTemplate(t))
	if err != nil {
		t.Fatalf("a second, plainer ask should have produced a deck: %v", err)
	}
	if len(deckOut.Slides) != 1 {
		t.Fatalf("slides = %d", len(deckOut.Slides))
	}
	if len(prompts) != 2 {
		t.Fatalf("expected exactly one retry, got %d requests", len(prompts))
	}
	if !strings.HasPrefix(prompts[1], "/no_think") {
		t.Fatalf("the retry should say so plainly: %q", prompts[1][:20])
	}
}

// A company's fixed slides are already written and agreed. A deck that writes
// its own version of one is how a company's decks drift apart, so generation
// looks in the library first — and says which slides it took.
func TestGenerationUsesTheLibraryItWasGiven(t *testing.T) {
	template := testTemplate(t)
	generator := New(testSettings{"ai.provider": "fallback"})
	registered := library.Entry{ID: "snippet-1", Name: "회사 소개",
		Source: "# 회사 소개\n@content\n> 2003년 설립\n- 임직원 1,240명\n- 매출 8,200억\n"}
	generator.Library = func(context.Context, string) []library.Entry { return []library.Entry{registered} }
	marked := ""
	generator.Used = func(_ context.Context, _, id string) { marked = id }

	presentation := model.Presentation{OwnerID: "owner", Language: "ko", RequestedSlideCount: 6,
		Prompt: "회사 소개와 2026년 사업 계획을 임원에게 보고"}
	generated, err := generator.Generate(context.Background(), presentation, model.Profile{}, template)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(generated.Source, "- 임직원 1,240명") {
		t.Errorf("the deck wrote its own company introduction:\n%s", generated.Source)
	}
	if marked != "snippet-1" {
		t.Errorf("the registered slide was not counted as used (%q)", marked)
	}
	said := false
	for _, warning := range generated.Warnings {
		if strings.Contains(warning, "라이브러리") {
			said = true
		}
	}
	if !said {
		t.Errorf("the deck did not say it used a registered slide: %v", generated.Warnings)
	}
}

// A model has no clock. A brief that says "the second half" without a year got
// whatever year the model remembered — a deck written in 2026 came back titled
// 2024 — so the brief says what day it is.
func TestTheBriefSaysWhatDayItIs(t *testing.T) {
	template := testTemplate(t)
	var briefs []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		briefs = append(briefs, string(body))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"# 하반기 전략\n- 한 줄\n"}}]}`))
	}))
	defer server.Close()

	generator := New(testSettings{
		"ai.provider": "openai-compatible", "ai.base_url": server.URL,
		"ai.model": "local", "ai.api_key": "k", "generation.outline_pass": false,
	})
	generator.client = server.Client()
	generator.Now = func() time.Time { return time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC) }
	if _, err := generator.Generate(context.Background(),
		model.Presentation{Title: "전략", Prompt: "하반기 전략", Language: "ko", RequestedSlideCount: 3},
		model.Profile{}, template); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(briefs) == 0 {
		t.Fatal("the model was never asked anything")
	}
	if !strings.Contains(briefs[0], "2026-08-22") {
		t.Fatalf("the brief does not say what day it is:\n%s", briefs[0])
	}
}

// A model that was not available is a moment in time: the author gets the deck
// Ptium can write offline, and the deck says so. A model that is set up wrong
// is an administrator's problem and has to reach them, so it still fails.
func TestAModelThatTimesOutDoesNotCostTheAuthorTheirDeck(t *testing.T) {
	template := testTemplate(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = writer.Write([]byte(`{"error":{"message":"upstream is down"}}`))
	}))
	defer server.Close()

	generator := New(testSettings{
		"ai.provider": "openai-compatible", "ai.base_url": server.URL,
		"ai.model": "local", "ai.api_key": "k", "generation.outline_pass": false,
	})
	generator.client = server.Client()
	deck, err := generator.Generate(context.Background(),
		model.Presentation{Title: "계획", Prompt: "결제 이중화 계획", Language: "ko", RequestedSlideCount: 5},
		model.Profile{}, template)
	if err != nil {
		t.Fatalf("an unavailable model cost the author their deck: %v", err)
	}
	if len(deck.Slides) == 0 {
		t.Fatal("no slides were written")
	}
	said := strings.Join(deck.Notes, " ")
	if !strings.Contains(said, "Ptium이 대신 썼습니다") {
		t.Fatalf("the deck does not say who wrote it: %q", said)
	}

	// A rejected key is not a moment in time. It stays a failure, so that it
	// reaches the error centre and an administrator.
	refusing := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer refusing.Close()
	strict := New(testSettings{
		"ai.provider": "openai-compatible", "ai.base_url": refusing.URL,
		"ai.model": "local", "ai.api_key": "k", "generation.outline_pass": false,
	})
	strict.client = refusing.Client()
	if _, err := strict.Generate(context.Background(),
		model.Presentation{Title: "계획", Prompt: "결제 이중화", Language: "ko", RequestedSlideCount: 5},
		model.Profile{}, template); err == nil {
		t.Fatal("a rejected key was hidden behind an offline deck")
	}
}

// Planning the narrative is the first of two passes, and on a slow model it is
// where the clock runs out. Giving up there hands the deck to the offline
// writer when the model could still have written it, so the pass is dropped and
// the writing goes ahead — with the deck saying that it did.
func TestASlowPlanningPassDoesNotCostTheModelTheDeck(t *testing.T) {
	template := testTemplate(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			// The narrative pass: too slow for this deployment.
			writer.WriteHeader(http.StatusGatewayTimeout)
			_, _ = writer.Write([]byte(`{"error":{"message":"timeout"}}`))
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"# 결제 이중화\n@cover\n\n# 지금의 문제\n- 단일 리전에 의존합니다\n"}}]}`))
	}))
	defer server.Close()

	generator := New(testSettings{
		"ai.provider": "openai-compatible", "ai.base_url": server.URL,
		"ai.model": "local", "ai.api_key": "k", "generation.outline_pass": true,
		"generation.repair_passes": 0,
	})
	generator.client = server.Client()
	deck, err := generator.Generate(context.Background(),
		model.Presentation{Title: "계획", Prompt: "결제 이중화 계획", Language: "ko", RequestedSlideCount: 6},
		model.Profile{}, template)
	if err != nil {
		t.Fatalf("a slow planning pass cost the model the deck: %v", err)
	}
	if calls < 2 {
		t.Fatalf("the writing pass was never attempted (%d calls)", calls)
	}
	said := strings.Join(deck.Notes, " ")
	if !strings.Contains(said, "건너뛰고") {
		t.Fatalf("the deck does not say the planning pass was skipped: %q", said)
	}
	// And it is the model's deck, not the offline writer's.
	if strings.Contains(said, "Ptium이 대신 썼습니다") {
		t.Fatalf("the deck was handed to the offline writer anyway: %q", said)
	}
	if !strings.Contains(deck.Source, "단일 리전에 의존합니다") {
		t.Fatalf("the model's own text is not in the deck:\n%s", deck.Source)
	}
}

// The plan is an aid, not a requirement. A model that answers the first pass
// with something that is not a plan — which a local model does now and then —
// used to sink the whole generation, and the author got a failure screen for a
// deck the model could have written.
func TestAnUnreadablePlanDoesNotSinkTheDeck(t *testing.T) {
	template := testTemplate(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls++
		writer.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			// A plan that is not a plan.
			_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"물론이죠! 이렇게 구성해 보겠습니다."}}]}`))
			return
		}
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"# 결제 이중화\n@cover\n\n# 지금의 문제\n- 단일 리전에 의존합니다\n"}}]}`))
	}))
	defer server.Close()

	generator := New(testSettings{
		"ai.provider": "openai-compatible", "ai.base_url": server.URL,
		"ai.model": "local", "ai.api_key": "k", "generation.outline_pass": true,
		"generation.repair_passes": 0,
	})
	generator.client = server.Client()
	deck, err := generator.Generate(context.Background(),
		model.Presentation{Title: "계획", Prompt: "결제 이중화 계획", Language: "ko", RequestedSlideCount: 6},
		model.Profile{}, template)
	if err != nil {
		t.Fatalf("an unreadable plan sank the deck: %v", err)
	}
	if !strings.Contains(deck.Source, "단일 리전에 의존합니다") {
		t.Fatalf("the model's own text is not in the deck:\n%s", deck.Source)
	}
	if said := strings.Join(deck.Notes, " "); !strings.Contains(said, "건너뛰고") {
		t.Fatalf("the deck does not say the planning pass was skipped: %q", said)
	}

	// A provider that is genuinely misconfigured still fails: the writing pass
	// hits the same wall a moment later and that failure is reported.
	refusing := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer refusing.Close()
	strict := New(testSettings{
		"ai.provider": "openai-compatible", "ai.base_url": refusing.URL,
		"ai.model": "local", "ai.api_key": "k", "generation.outline_pass": true,
	})
	strict.client = refusing.Client()
	if _, err := strict.Generate(context.Background(),
		model.Presentation{Title: "계획", Prompt: "결제", Language: "ko", RequestedSlideCount: 6},
		model.Profile{}, template); err == nil {
		t.Fatal("a rejected key was hidden by skipping the plan")
	}
}

// A RACI chart, a readiness checklist and a likelihood-by-impact matrix are
// shapes every corporate deck uses, and Ptium draws all three. A model that has
// not been told they exist writes them as bullets, so the brief lists them.
func TestTheBriefSaysWhichGridsThisDeploymentDraws(t *testing.T) {
	template := testTemplate(t)
	var asked []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		asked = append(asked, string(body))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"# 담당 체계\n- 한 줄\n"}}]}`))
	}))
	defer server.Close()

	generator := New(testSettings{
		"ai.provider": "openai-compatible", "ai.base_url": server.URL,
		"ai.model": "local", "ai.api_key": "k", "generation.outline_pass": false,
	})
	generator.client = server.Client()
	if _, err := generator.Generate(context.Background(),
		model.Presentation{Title: "담당 체계", Prompt: "이관 프로젝트의 담당 체계", Language: "ko", RequestedSlideCount: 3},
		model.Profile{}, template); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(asked) == 0 {
		t.Fatal("the model was never asked anything")
	}
	brief := asked[0]
	for _, wanted := range []string{"::grid", "raci", "checklist", "column header"} {
		if !strings.Contains(brief, wanted) {
			t.Fatalf("the brief does not mention %q:\n%s", wanted, brief)
		}
	}
	// The values come from the definitions themselves rather than a second copy.
	for _, value := range pptx.BuiltinGrids()[0].Order {
		if !strings.Contains(brief, value) {
			t.Fatalf("the brief does not say a raci cell may be %q", value)
		}
	}
}

// The inspector marks a slide that states figures and says nowhere they came
// from — 160 times across the decks in one account — and the model was never
// told it could write a source at all. Measuring something nobody asked for is
// not a measurement.
func TestTheModelIsToldHowToCiteASource(t *testing.T) {
	template := testTemplate(t)
	var asked []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		asked = append(asked, string(body))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"# 실적\n- 한 줄\n"}}]}`))
	}))
	defer server.Close()

	generator := New(testSettings{
		"ai.provider": "openai-compatible", "ai.base_url": server.URL,
		"ai.model": "local", "ai.api_key": "k", "generation.outline_pass": false,
	})
	generator.client = server.Client()
	if _, err := generator.Generate(context.Background(),
		model.Presentation{Title: "실적", Prompt: "통계청 2026 소비 동향에 따르면 매출 1,240억", Language: "ko", RequestedSlideCount: 3},
		model.Profile{}, template); err != nil {
		t.Fatalf("generate: %v", err)
	}
	brief := strings.Join(asked, "\n")
	for _, wanted := range []string{"!source", "Never invent one"} {
		if !strings.Contains(brief, wanted) {
			t.Fatalf("the model is not told about %q", wanted)
		}
	}
	// And about the second column, which the language grew in an earlier release.
	if !strings.Contains(brief, "starts the other column") {
		t.Fatal("the model is not told how to write a two-column slide")
	}
}

// The measurement that asks a slide to be cut counts points, so the rewrite is
// asked for the same number. Told only to "shorten", a model came back with
// eleven shorter lines — measured no better, rejected, a round trip spent on
// nothing.
func TestTheShortenTaskNamesTheNumberOfPoints(t *testing.T) {
	task := revisionTask(Revision{Action: ReviseShorten})
	if !strings.Contains(task, fmt.Sprintf("%d", pptx.MaximumPoints)) {
		t.Fatalf("the task does not name the maximum: %q", task)
	}
	for _, wanted := range []string{"top-level points", "merging", "restate"} {
		if !strings.Contains(task, wanted) {
			t.Fatalf("the task does not say how to cut: %q", task)
		}
	}
	// Fitting is a different job and still asks for every point to be kept.
	if fit := revisionTask(Revision{Action: ReviseFit}); !strings.Contains(fit, "keeping every point") {
		t.Fatalf("the fit task changed: %q", fit)
	}
}

// The planning pass asks for JSON and nothing else. A run against a self-hosted
// model answered with the outline inside a fence, the pass was thrown away, and
// the deck was written with no design behind it — the thing that was asked for
// was there, and unread.
func TestThePlanIsReadThroughWhateverWrapsIt(t *testing.T) {
	object := `{"thesis":"지금 결정해야 합니다","slides":[{"role":"content","headline":"현황"}]}`
	for _, answer := range []string{
		object,
		"```json\n" + object + "\n```",
		"```\n" + object + "\n```",
		"다음은 요청하신 구성입니다:\n" + object,
		object + "\n\n필요하시면 슬라이드 수를 조정하겠습니다.",
		"  \n" + object + "  ",
	} {
		var plan deckPlan
		if err := json.Unmarshal(planJSON(answer), &plan); err != nil {
			t.Fatalf("could not read the outline from %q: %v", answer, err)
		}
		if plan.Thesis != "지금 결정해야 합니다" || len(plan.Slides) != 1 {
			t.Fatalf("the outline read from %q is wrong: %+v", answer, plan)
		}
	}
	// An answer with no object in it is still a failure.
	var plan deckPlan
	if err := json.Unmarshal(planJSON("죄송합니다, 구성을 만들 수 없습니다."), &plan); err == nil {
		t.Fatal("an answer with no outline in it was read as one")
	}
}

// The author gets the plain explanation; the warning carries what the model
// actually said, because without it the next person can only guess.
func TestAnUnreadableOutlineIsQuotedInTheWarning(t *testing.T) {
	err := errors.New("AI provider returned an outline that could not be read: 죄송합니다 구성을 만들 수 없습니다")
	if got := AuthorMessage(err, "ko"); !strings.Contains(got, "읽을 수 없는") {
		t.Fatalf("the author sees %q, wanted the plain explanation", got)
	}
	if got := AuthorMessage(err, "en"); strings.Contains(got, "죄송합니다") {
		t.Fatalf("the model's own answer reached the author: %q", got)
	}
}

// A deployment pinned to a model that answers in the older JSON shape used to
// return straight from compose: no repair pass, nothing said about invented
// figures, nothing said about a source the brief named and the deck ignored.
// Both shapes are the same deck and go through the same doors.
func TestTheJSONShapeGoesThroughTheSameDoors(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	request := writingRequest{
		Presentation: model.Presentation{Language: "ko", Title: "이관 보고",
			Prompt:              "내부 검색 로그 기준 검색 실패율 31%. 교체 비용 1억 5천만 원.",
			RequestedSlideCount: 3},
		Template: Template{Manifest: manifest},
	}
	written := writtenDeck{Slides: []writtenSlide{
		{Title: "이관 보고"},
		{Title: "현황", Fields: map[string]json.RawMessage{
			pptx.SlotBody: json.RawMessage(`["검색 실패율 31%","가용성 99.99% 확보"]`)}},
		{Title: "승인 요청"},
	}}
	composed, err := compose(request, written)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	generator := &Generator{}
	finished, err := generator.finishDeck(context.Background(), request, composed, composed.Source, 0, 0, time.Second)
	if err != nil {
		t.Fatalf("finishDeck: %v", err)
	}
	notes := strings.Join(finished.Notes, " ")
	if !strings.Contains(notes, "99.99%") {
		t.Fatalf("the JSON shape says nothing about the figure it invented: %q", finished.Notes)
	}
	if !strings.Contains(notes, "!source") {
		t.Fatalf("the JSON shape says nothing about the source the brief named: %q", finished.Notes)
	}
}

// Text with nowhere to go used to disappear without a word: a body written for
// a slide whose layout has no body region was dropped in silence. The other way
// of writing a deck says so; both should.
func TestComposeSaysWhatItDropped(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	request := writingRequest{
		Presentation: model.Presentation{Language: "ko", Title: "이관 보고", RequestedSlideCount: 2},
		Template:     Template{Manifest: manifest},
	}
	written := writtenDeck{Slides: []writtenSlide{
		{Title: "이관 보고"},
		{Title: "승인 요청", Fields: map[string]json.RawMessage{
			pptx.SlotBody: json.RawMessage(`["예산 승인이 필요합니다"]`)}},
	}}
	composed, err := compose(request, written)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	kept := strings.Contains(composed.Source, "예산 승인이 필요합니다")
	said := false
	for _, warning := range composed.Warnings {
		said = said || strings.Contains(warning, "예산 승인이 필요합니다")
	}
	if !kept && !said {
		t.Fatalf("the line was dropped in silence: %q\n%s", composed.Warnings, composed.Source)
	}
}

// A model that stops because it ran out of room has not answered. Half a plan
// is not valid JSON and half a deck is a deck missing its last slides, so the
// partial answer is a failure that names the setting to raise — it used to be
// passed on as if it were whole whenever the model had written anything at all.
func TestAnAnswerCutOffAtTheOutputLimitIsAFailure(t *testing.T) {
	for _, one := range []struct {
		name, content string
	}{
		{"nothing written", ""},
		{"cut off mid-answer", `{"deckTitle": "ERP 교체", "slides": [{"role": "title", "headl`},
	} {
		capped := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(map[string]any{"choices": []any{map[string]any{
				"message":       map[string]any{"role": "assistant", "content": one.content},
				"finish_reason": "length",
			}}})
		}))
		generator := New(testSettings{"ai.provider": "openai-compatible", "ai.base_url": capped.URL,
			"ai.model": "test-model", "ai.api_key": "test-key"})
		generator.client = capped.Client()
		_, err := generator.complete(context.Background(), capped.URL+"/chat/completions", "test-model", "test-key",
			"system", "user", 0.5)
		capped.Close()
		if err == nil {
			t.Fatalf("%s: a capped answer was accepted as an answer", one.name)
		}
		if !strings.Contains(err.Error(), "output limit") {
			t.Fatalf("%s: error = %v", one.name, err)
		}
		// And the author is told which number fixes it.
		message := AuthorMessage(err, "ko")
		if !strings.Contains(message, "max_output_tokens") {
			t.Fatalf("%s: the author is told %q", one.name, message)
		}
	}
}

// Eight thousand output tokens was chosen when a deck's source was the whole
// answer. Measured against the live model, planning a nine-slide deck spent
// 6,495 completion tokens — four fifths of it thinking — so the default was
// four fifths gone on the smallest deck anyone asks for, and fifty slides had
// no chance. Since a capped answer is a failure, that is a generation that
// falls back rather than one that quietly arrives short.
func TestTheOutputBudgetGrowsWithTheDeck(t *testing.T) {
	generator := New(testSettings{"ai.provider": "openai-compatible"})
	generator.maxOutputTokens = defaultOutputTokens

	// The smallest deck anyone asks for already spent 6,495 tokens of the 8,000
	// on one pass, so even that gets room.
	generator.budgetForDeck(9)
	if generator.maxOutputTokens <= 6495 {
		t.Errorf("a nine-slide deck gets %d tokens; one pass of it measured 6,495", generator.maxOutputTokens)
	}
	generator.budgetForDeck(50)
	if generator.maxOutputTokens <= defaultOutputTokens {
		t.Errorf("a fifty-slide deck was left at %d tokens", generator.maxOutputTokens)
	}
	// Fifty slides at the measured rate, twice over, still fits what was chosen.
	if want := 50 * 116 * 2; generator.maxOutputTokens < want {
		t.Errorf("a fifty-slide deck gets %d tokens, less than %d", generator.maxOutputTokens, want)
	}

	// A budget an administrator set is theirs.
	chosen := New(testSettings{"ai.provider": "openai-compatible", "ai.max_output_tokens": 3000})
	chosen.applyProviderSettings(context.Background())
	chosen.budgetForDeck(50)
	if chosen.maxOutputTokens != 3000 {
		t.Errorf("an administrator's 3000 became %d", chosen.maxOutputTokens)
	}
}
