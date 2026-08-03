package generation

import (
	"context"
	"encoding/json"
	"errors"
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
	data, err := pptx.BuiltinTemplate("aurora")
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

func TestGenerateRequiresExactExternalSlideCount(t *testing.T) {
	template := testTemplate(t)
	for _, test := range []struct {
		name          string
		returned      int
		wantErrorText string
	}{
		{name: "mismatch", returned: 2, wantErrorText: "returned 2 slides; exactly 3 were requested"},
		{name: "exact match", returned: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := newStubProvider(t, writtenSlides(test.returned, template.Manifest.DefaultLayout))
			generated, err := stub.generator().Generate(context.Background(),
				model.Presentation{Title: "Plan", Prompt: "Growth", Language: "ko", RequestedSlideCount: 3}, model.Profile{}, template)
			if test.wantErrorText != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErrorText) {
					t.Fatalf("Generate() error = %v, want containing %q", err, test.wantErrorText)
				}
				if len(generated.Slides) != 0 {
					t.Fatalf("mismatched deck must not be returned: %#v", generated)
				}
				return
			}
			if err != nil || len(generated.Slides) != 3 {
				t.Fatalf("Generate() deck size = %d, error = %v", len(generated.Slides), err)
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
