package generation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

type SettingReader interface {
	Get(ctx context.Context, key string, target any) error
}

type Generator struct {
	settings SettingReader
	client   *http.Client
}

// Deck is the generator's output: the outline shown in the workspace plus the
// slides persisted for editing and export.
type Deck struct {
	Outline json.RawMessage `json:"outline"`
	Slides  []model.Slide   `json:"slides"`
}

// Template is the design a deck is written into. The manifest tells the model
// which layouts exist and how much text each slot can hold, which is what
// keeps generated copy inside the customer's own design.
type Template struct {
	ID       string
	Name     string
	Manifest pptx.Manifest
}

func New(settings SettingReader) *Generator {
	return &Generator{settings: settings, client: &http.Client{Timeout: 120 * time.Second}}
}

// Generate produces a deck for a presentation. It plans the narrative first
// and writes slide copy second, so the result reads like a deck a consultant
// would build rather than a list of bullet points.
func (g *Generator) Generate(ctx context.Context, presentation model.Presentation, profile model.Profile, template Template) (Deck, error) {
	profile = g.withDefaultBrand(ctx, profile)
	if len(template.Manifest.Layouts) == 0 {
		return Deck{}, errors.New("the selected template does not expose any usable layout")
	}
	if presentation.RequestedSlideCount < 1 || presentation.RequestedSlideCount > 50 {
		return Deck{}, fmt.Errorf("requested slide count %d is outside the supported range 1-50", presentation.RequestedSlideCount)
	}

	provider, baseURL, modelName, apiKey := "fallback", "https://api.openai.com/v1", "gpt-4.1-mini", ""
	_ = g.settings.Get(ctx, "ai.provider", &provider)
	_ = g.settings.Get(ctx, "ai.base_url", &baseURL)
	_ = g.settings.Get(ctx, "ai.model", &modelName)
	_ = g.settings.Get(ctx, "ai.api_key", &apiKey)
	if strings.EqualFold(provider, "fallback") || strings.TrimSpace(apiKey) == "" {
		return Fallback(presentation, profile, template), nil
	}
	if provider != "openai-compatible" && provider != "openai" {
		return Deck{}, fmt.Errorf("unsupported AI provider %q", provider)
	}

	endpoint, err := completionsEndpoint(baseURL)
	if err != nil {
		return Deck{}, err
	}
	request := writingRequest{Presentation: presentation, Profile: profile, Template: template}
	outlinePass := true
	_ = g.settings.Get(ctx, "generation.outline_pass", &outlinePass)
	if outlinePass && presentation.RequestedSlideCount > 2 {
		plan, err := g.plan(ctx, endpoint, modelName, apiKey, request)
		if err != nil {
			return Deck{}, err
		}
		request.Plan = plan
	}
	written, err := g.write(ctx, endpoint, modelName, apiKey, request)
	if err != nil {
		return Deck{}, err
	}
	return compose(request, written)
}

// plan asks the model to design the deck before writing any copy.
func (g *Generator) plan(ctx context.Context, endpoint, modelName, apiKey string, request writingRequest) (*deckPlan, error) {
	raw, err := g.complete(ctx, endpoint, modelName, apiKey, planSystemPrompt, planUserPrompt(request), 0.5)
	if err != nil {
		return nil, err
	}
	var plan deckPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return nil, errors.New("AI provider returned an outline that could not be read")
	}
	if len(plan.Slides) == 0 {
		return nil, errors.New("AI provider returned an empty outline")
	}
	return &plan, nil
}

// write turns the plan into finished slide copy bound to template slots.
func (g *Generator) write(ctx context.Context, endpoint, modelName, apiKey string, request writingRequest) (writtenDeck, error) {
	raw, err := g.complete(ctx, endpoint, modelName, apiKey, writeSystemPrompt, writeUserPrompt(request), 0.35)
	if err != nil {
		return writtenDeck{}, err
	}
	var written writtenDeck
	if err := json.Unmarshal([]byte(raw), &written); err != nil {
		return writtenDeck{}, errors.New("AI provider response did not contain a valid deck")
	}
	if len(written.Slides) == 0 {
		return writtenDeck{}, errors.New("AI provider returned a deck without slides")
	}
	return written, nil
}

type completionRequest struct {
	Model          string              `json:"model"`
	Messages       []completionMessage `json:"messages"`
	Temperature    float64             `json:"temperature"`
	ResponseFormat map[string]string   `json:"response_format,omitempty"`
}

type completionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type completionResponse struct {
	Choices []struct {
		Message completionMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func completionsEndpoint(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/") + "/chat/completions")
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("AI base URL is invalid")
	}
	return parsed.String(), nil
}

func (g *Generator) complete(ctx context.Context, endpoint, modelName, apiKey, system, user string, temperature float64) (string, error) {
	payload, err := json.Marshal(completionRequest{
		Model:          modelName,
		Messages:       []completionMessage{{Role: "system", Content: system}, {Role: "user", Content: user}},
		Temperature:    temperature,
		ResponseFormat: map[string]string{"type": "json_object"},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("AI request failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return "", err
	}
	var completion completionResponse
	if json.Unmarshal(body, &completion) != nil {
		return "", fmt.Errorf("AI provider returned invalid JSON (status %d)", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := "provider error"
		if completion.Error != nil && completion.Error.Message != "" {
			message = completion.Error.Message
		}
		return "", fmt.Errorf("AI provider status %d: %s", response.StatusCode, truncate(message, 300))
	}
	if len(completion.Choices) == 0 {
		return "", errors.New("AI provider returned no choices")
	}
	return strings.TrimSpace(completion.Choices[0].Message.Content), nil
}

func (g *Generator) withDefaultBrand(ctx context.Context, profile model.Profile) model.Profile {
	preferences := map[string]any{}
	_ = json.Unmarshal(profile.Preferences, &preferences)
	for _, key := range []string{"brandColor", "brand_color"} {
		if color, ok := preferences[key].(string); ok && validHexColor(color) {
			return profile
		}
	}
	var color string
	if g.settings == nil || g.settings.Get(ctx, "branding.brand_color", &color) != nil || !validHexColor(color) {
		return profile
	}
	preferences["brandColor"] = strings.ToUpper(color)
	profile.Preferences, _ = json.Marshal(preferences)
	return profile
}

func fallbackAccent(position int) string {
	colors := []string{"#7C3AED", "#0EA5E9", "#10B981", "#F59E0B"}
	return colors[position%len(colors)]
}

func profileAccent(profile model.Profile, position int) string {
	var preferences map[string]any
	if json.Unmarshal(profile.Preferences, &preferences) == nil {
		for _, key := range []string{"brandColor", "brand_color"} {
			if color, ok := preferences[key].(string); ok && validHexColor(color) {
				return strings.ToUpper(color)
			}
		}
	}
	return fallbackAccent(position)
}

func validHexColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, character := range value[1:] {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
			return false
		}
	}
	return true
}

func truncate(value string, max int) string {
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max])
}
