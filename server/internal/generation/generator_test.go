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

	"github.com/hkjang/ptium/server/internal/deck"
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
	if len(generated.Slides) != 10 {
		t.Fatalf("expected 10 slides, got %d", len(generated.Slides))
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
