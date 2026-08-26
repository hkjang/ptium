package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/hkjang/ptium/server/internal/generation"
	"github.com/hkjang/ptium/server/internal/mcp"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/settings"
	"github.com/hkjang/ptium/server/internal/store"
)

type MCPOperations struct {
	Store    *store.Store
	Settings *settings.Service
	Worker   interface{ Notify() }
}

func MCPUserFromRequest(request *http.Request) (model.User, error) {
	user, ok := UserFromContext(request.Context())
	if !ok {
		return model.User{}, errors.New("authenticated Ptium user is unavailable")
	}
	return user, nil
}

func (operations MCPOperations) ListPresentations(ctx context.Context, user model.User, limit, offset int,
	search string) ([]model.Presentation, int, error) {
	if !allowScope(ctx, "presentations:read") {
		return nil, 0, mcp.NewServiceError(mcp.ServiceErrorForbidden, "presentations:read scope is required")
	}
	if strings.TrimSpace(search) != "" {
		return operations.Store.SearchPresentations(ctx, user.ID, false, false, strings.TrimSpace(search), limit, offset)
	}
	return operations.Store.ListPresentations(ctx, user.ID, false, limit, offset)
}

func (operations MCPOperations) GetPresentation(ctx context.Context, user model.User, id string) (model.Presentation, error) {
	if !allowScope(ctx, "presentations:read") {
		return model.Presentation{}, mcp.NewServiceError(mcp.ServiceErrorForbidden, "presentations:read scope is required")
	}
	presentation, err := operations.Store.GetPresentation(ctx, id, user.ID, false)
	return presentation, mapMCPError(err)
}

func (operations MCPOperations) CreatePresentation(ctx context.Context, user model.User, input mcp.CreatePresentationInput) (model.Presentation, error) {
	if !allowScope(ctx, "presentations:write") {
		return model.Presentation{}, mcp.NewServiceError(mcp.ServiceErrorForbidden, "presentations:write scope is required")
	}
	storeInput := store.PresentationInput{Title: input.Title, Prompt: input.Prompt, Theme: input.Theme, Language: input.Language, Audience: input.Audience, Tone: input.Tone, SlideCount: input.SlideCount}
	if trimmed := strings.TrimSpace(input.TemplateID); trimmed != "" {
		if _, err := operations.Store.GetTemplate(ctx, trimmed, user.ID, false); err != nil {
			return model.Presentation{}, mcp.NewServiceError(mcp.ServiceErrorInvalidArgument, "the selected template does not exist")
		}
		storeInput.TemplateID = &trimmed
	}
	// A prompt that names its own length is honoured before any default, the same
	// way the REST API treats it.
	if storeInput.SlideCount == 0 {
		storeInput.SlideCount = generation.ParseIntent(storeInput.Prompt).SlideCount
	}
	if operations.Settings != nil {
		if storeInput.SlideCount == 0 {
			_ = operations.Settings.Get(ctx, "generation.default_slide_count", &storeInput.SlideCount)
		}
		if storeInput.Theme == "" {
			_ = operations.Settings.Get(ctx, "generation.default_theme", &storeInput.Theme)
		}
		if storeInput.Language == "" {
			_ = operations.Settings.Get(ctx, "generation.default_lang", &storeInput.Language)
		}
		if storeInput.Audience == "" {
			_ = operations.Settings.Get(ctx, "generation.default_audience", &storeInput.Audience)
		}
		if storeInput.Tone == "" {
			_ = operations.Settings.Get(ctx, "generation.default_tone", &storeInput.Tone)
		}
		maximum := 50
		_ = operations.Settings.Get(ctx, "generation.max_slides", &maximum)
		if storeInput.SlideCount > maximum {
			return model.Presentation{}, mcp.NewServiceError(mcp.ServiceErrorInvalidArgument, "slideCount exceeds the configured generation limit")
		}
	}
	if storeInput.SlideCount == 0 {
		storeInput.SlideCount = 8
	}
	if storeInput.Theme == "" {
		storeInput.Theme = "modern"
	}
	if storeInput.Language == "" {
		storeInput.Language = "ko"
	}
	if storeInput.Audience == "" {
		storeInput.Audience = "general"
	}
	if storeInput.Tone == "" {
		storeInput.Tone = "professional"
	}
	if storeInput.TemplateID == nil {
		if resolved, err := operations.Store.DefaultTemplateID(ctx, user.ID, storeInput.Theme); err == nil {
			storeInput.TemplateID = &resolved
		}
	}
	created, err := operations.Store.CreatePresentation(ctx, user.ID, storeInput)
	if err == nil {
		operations.Store.Audit(ctx, &user.ID, "mcp.presentation.create", "presentation", created.ID, nil)
	}
	return created, mapMCPError(err)
}

func (operations MCPOperations) GeneratePresentation(ctx context.Context, user model.User, id string) (model.Presentation, error) {
	if !allowScope(ctx, "presentations:write") {
		return model.Presentation{}, mcp.NewServiceError(mcp.ServiceErrorForbidden, "presentations:write scope is required")
	}
	maximumSlides := 50
	if operations.Settings != nil {
		if err := operations.Settings.Get(ctx, "generation.max_slides", &maximumSlides); err != nil || maximumSlides < 1 || maximumSlides > 50 {
			maximumSlides = 50
		}
	}
	queued, err := operations.Store.QueueGeneration(ctx, id, user.ID, false, maximumSlides)
	if err != nil {
		if errors.Is(err, store.ErrGenerationLimit) {
			return model.Presentation{}, mcp.NewServiceError(mcp.ServiceErrorInvalidArgument, fmt.Sprintf("requested slide count exceeds the configured limit of %d", maximumSlides))
		}
		return model.Presentation{}, mapMCPError(err)
	}
	operations.Store.Audit(ctx, &user.ID, "mcp.presentation.generate", "presentation", id, nil)
	if operations.Worker != nil {
		operations.Worker.Notify()
	}
	return queued, nil
}

func (operations MCPOperations) ListTemplates(ctx context.Context, user model.User, limit, offset int) ([]model.Template, int, error) {
	if !allowScope(ctx, "templates:read") {
		return nil, 0, mcp.NewServiceError(mcp.ServiceErrorForbidden, "templates:read scope is required")
	}
	templates, total, err := operations.Store.ListTemplates(ctx, user.ID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	// Callers choose a template by id and layout, so the raw manifest — which
	// can be tens of kilobytes — is replaced by its layout catalog.
	for index := range templates {
		templates[index].Manifest = nil
	}
	return templates, total, nil
}

func mapMCPError(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return mcp.NewServiceError(mcp.ServiceErrorNotFound, "Presentation not found")
	}
	return err
}
