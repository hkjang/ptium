package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/hkjang/ptium/server/internal/auth"
	"github.com/hkjang/ptium/server/internal/export"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/store"
)

func (s *Server) me(writer http.ResponseWriter, request *http.Request) {
	user, ok := UserFromContext(request.Context())
	if !ok {
		writeError(writer, request, http.StatusUnauthorized, "authentication_required", "Authentication is required", nil)
		return
	}
	principal, _ := auth.PrincipalFromContext(request.Context())
	if profile, err := s.store.GetProfile(request.Context(), user.ID); err == nil && strings.TrimSpace(profile.DisplayName) != "" {
		user.Name = profile.DisplayName
	}
	writeData(writer, request, http.StatusOK, map[string]any{"user": user, "authMethod": principal.AuthMethod, "scopes": principal.Scopes})
}

func (s *Server) getProfile(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	profile, err := s.store.GetProfile(request.Context(), user.ID)
	if err != nil {
		s.internalError(writer, request, "profile_read_failed", err)
		return
	}
	writeData(writer, request, http.StatusOK, profile)
}

type profileRequest struct {
	DisplayName string          `json:"displayName"`
	Company     string          `json:"company"`
	JobTitle    string          `json:"jobTitle"`
	Bio         string          `json:"bio"`
	Preferences json.RawMessage `json:"preferences"`
}

func (s *Server) putProfile(writer http.ResponseWriter, request *http.Request) {
	var input profileRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if utf8.RuneCountInString(input.DisplayName) > 120 || utf8.RuneCountInString(input.Company) > 200 || utf8.RuneCountInString(input.JobTitle) > 200 || utf8.RuneCountInString(input.Bio) > 4000 {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "Profile fields exceed their allowed length", nil)
		return
	}
	if len(input.Preferences) == 0 {
		input.Preferences = json.RawMessage(`{}`)
	}
	if !json.Valid(input.Preferences) {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "preferences must be valid JSON", nil)
		return
	}
	user, _ := UserFromContext(request.Context())
	profile, err := s.store.UpdateProfile(request.Context(), user.ID, model.Profile{DisplayName: input.DisplayName, Company: input.Company, JobTitle: input.JobTitle, Bio: input.Bio, Preferences: input.Preferences})
	if err != nil {
		s.internalError(writer, request, "profile_update_failed", err)
		return
	}
	s.store.Audit(request.Context(), &user.ID, "profile.update", "profile", user.ID, nil)
	writeData(writer, request, http.StatusOK, profile)
}

func (s *Server) publicSettings(writer http.ResponseWriter, request *http.Request) {
	settings, err := s.settings.Public(request.Context())
	if err != nil {
		s.internalError(writer, request, "settings_read_failed", err)
		return
	}
	writeData(writer, request, http.StatusOK, settings)
}

type presentationRequest struct {
	Title               *string         `json:"title"`
	Prompt              *string         `json:"prompt"`
	Theme               *string         `json:"theme"`
	TemplateID          *string         `json:"templateId"`
	Language            *string         `json:"language"`
	Audience            *string         `json:"audience"`
	Tone                *string         `json:"tone"`
	RequestedSlideCount *int            `json:"requestedSlideCount"`
	SlideCount          *int            `json:"slideCount"`
	Slides              *[]slideRequest `json:"slides"`
}

type slideRequest struct {
	ID           string          `json:"id"`
	Position     int             `json:"position"`
	Title        string          `json:"title"`
	Subtitle     string          `json:"subtitle"`
	Content      json.RawMessage `json:"content"`
	SpeakerNotes string          `json:"speakerNotes"`
	Layout       string          `json:"layout"`
	LayoutID     string          `json:"layoutId"`
}

func (s *Server) listPresentations(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	limit, offset := pagination(request)
	items, total, err := s.store.ListPresentations(request.Context(), user.ID, false, limit, offset)
	if err != nil {
		s.internalError(writer, request, "presentations_read_failed", err)
		return
	}
	writeList(writer, request, items, total, limit, offset)
}

func (s *Server) createPresentation(writer http.ResponseWriter, request *http.Request) {
	var input presentationRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if input.Slides != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "slides can only be supplied when updating a presentation", nil)
		return
	}
	created, ok := s.createPresentationFromInput(writer, request, input)
	if !ok {
		return
	}
	writeData(writer, request, http.StatusCreated, created)
}

func (s *Server) createAndGeneratePresentation(writer http.ResponseWriter, request *http.Request) {
	var input presentationRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if input.Slides != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "slides can only be supplied when updating a presentation", nil)
		return
	}
	storeInput := s.defaultPresentationInput(request.Context())
	mergePresentationInput(&storeInput, input)
	if message := validatePresentationInput(storeInput, s.maximumSlides(request.Context())); message != "" {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", message, nil)
		return
	}
	user, _ := UserFromContext(request.Context())
	if err := s.resolveTemplateSelection(request.Context(), user.ID, &storeInput); err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
		return
	}
	queued, err := s.store.CreateAndQueuePresentation(request.Context(), user.ID, storeInput)
	if err != nil {
		s.internalError(writer, request, "presentation_create_failed", err)
		return
	}
	s.store.Audit(request.Context(), &user.ID, "presentation.create_and_generate", "presentation", queued.ID, nil)
	s.worker.Notify()
	writeData(writer, request, http.StatusAccepted, queued)
}

func (s *Server) createPresentationFromInput(writer http.ResponseWriter, request *http.Request, input presentationRequest) (model.Presentation, bool) {
	storeInput := s.defaultPresentationInput(request.Context())
	mergePresentationInput(&storeInput, input)
	if message := validatePresentationInput(storeInput, s.maximumSlides(request.Context())); message != "" {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", message, nil)
		return model.Presentation{}, false
	}
	user, _ := UserFromContext(request.Context())
	if err := s.resolveTemplateSelection(request.Context(), user.ID, &storeInput); err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
		return model.Presentation{}, false
	}
	created, err := s.store.CreatePresentation(request.Context(), user.ID, storeInput)
	if err != nil {
		s.internalError(writer, request, "presentation_create_failed", err)
		return model.Presentation{}, false
	}
	s.store.Audit(request.Context(), &user.ID, "presentation.create", "presentation", created.ID, nil)
	return created, true
}

func (s *Server) getPresentation(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	presentation, err := s.store.GetPresentation(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	writeData(writer, request, http.StatusOK, presentation)
}

func (s *Server) updatePresentation(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	current, err := s.store.GetPresentation(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	var input presentationRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	storeInput := store.PresentationInput{Title: current.Title, Prompt: current.Prompt, Theme: current.Theme, Language: current.Language,
		Audience: current.Audience, Tone: current.Tone, SlideCount: current.RequestedSlideCount, TemplateID: current.TemplateID}
	mergePresentationInput(&storeInput, input)
	if message := validatePresentationEditInput(storeInput); message != "" {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", message, nil)
		return
	}
	if err := s.resolveTemplateSelection(request.Context(), user.ID, &storeInput); err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
		return
	}
	var slides *[]model.Slide
	if input.Slides != nil {
		if len(*input.Slides) == 0 || len(*input.Slides) > 50 {
			writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "slides must contain between 1 and 50 items", nil)
			return
		}
		seenIDs := map[string]struct{}{}
		converted := make([]model.Slide, 0, len(*input.Slides))
		for index, incoming := range *input.Slides {
			if incoming.ID != "" {
				if _, duplicate := seenIDs[incoming.ID]; duplicate {
					writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "slide IDs must be unique", map[string]any{"slide": index + 1})
					return
				}
				seenIDs[incoming.ID] = struct{}{}
			}
			slide, err := convertSlide(incoming, index)
			if err != nil {
				writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), map[string]any{"slide": index + 1})
				return
			}
			converted = append(converted, slide)
		}
		slides = &converted
	}
	updated, err := s.store.UpdatePresentationWithSlides(request.Context(), current.ID, user.ID, false, storeInput, slides)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_update_failed")
		return
	}
	s.store.Audit(request.Context(), &user.ID, "presentation.update", "presentation", updated.ID, map[string]any{"slidesReplaced": slides != nil})
	writeData(writer, request, http.StatusOK, updated)
}

func (s *Server) deletePresentation(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	id := request.PathValue("id")
	if err := s.store.DeletePresentation(request.Context(), id, user.ID, false); err != nil {
		s.handleStoreError(writer, request, err, "presentation_delete_failed")
		return
	}
	s.store.Audit(request.Context(), &user.ID, "presentation.delete", "presentation", id, nil)
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) generatePresentation(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	maximumSlides := s.maximumSlides(request.Context())
	presentation, err := s.store.QueueGeneration(request.Context(), request.PathValue("id"), user.ID, false, maximumSlides)
	if err != nil {
		if errors.Is(err, store.ErrGenerationLimit) {
			writeError(writer, request, http.StatusUnprocessableEntity, "generation_slide_limit_exceeded", fmt.Sprintf("requestedSlideCount must be between 1 and %d before generation", maximumSlides), nil)
			return
		}
		s.handleStoreError(writer, request, err, "generation_queue_failed")
		return
	}
	s.store.Audit(request.Context(), &user.ID, "presentation.generate", "presentation", presentation.ID, nil)
	s.worker.Notify()
	writeData(writer, request, http.StatusAccepted, presentation)
}

func (s *Server) exportPresentation(writer http.ResponseWriter, request *http.Request) {
	format := strings.ToLower(request.URL.Query().Get("format"))
	if format == "" && strings.HasSuffix(request.URL.Path, ".pptx") {
		format = "pptx"
	}
	if format == "" {
		format = "pptx"
	}
	if format != "pptx" {
		writeError(writer, request, http.StatusUnprocessableEntity, "unsupported_export_format", "Only pptx export is currently supported", map[string]any{"supported": []string{"pptx"}})
		return
	}
	user, _ := UserFromContext(request.Context())
	presentation, err := s.store.GetPresentation(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_export_failed")
		return
	}
	if len(presentation.Slides) == 0 {
		writeError(writer, request, http.StatusConflict, "presentation_has_no_slides", "Generate or add slides before exporting", nil)
		return
	}
	templateData, manifest, err := s.presentationTemplate(request.Context(), presentation)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_template_unavailable")
		return
	}
	author := user.Name
	if profile, profileErr := s.store.GetProfile(request.Context(), user.ID); profileErr == nil && strings.TrimSpace(profile.Company) != "" {
		author = strings.TrimSpace(profile.Company)
	}
	data, err := export.PPTX(presentation, export.Options{TemplateData: templateData, Manifest: manifest, Author: author})
	if err != nil {
		s.internalError(writer, request, "presentation_export_failed", err)
		return
	}
	filename := safeFilename(presentation.Title) + ".pptx"
	writer.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.presentationml.presentation")
	writer.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	writer.Header().Set("Content-Length", strconv.Itoa(len(data)))
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(data)
}

func (s *Server) defaultPresentationInput(ctx context.Context) store.PresentationInput {
	result := store.PresentationInput{Title: "Untitled presentation", Theme: "modern", Language: "ko", Audience: "general", Tone: "professional", SlideCount: 8}
	_ = s.settings.Get(ctx, "generation.default_slide_count", &result.SlideCount)
	_ = s.settings.Get(ctx, "generation.default_theme", &result.Theme)
	_ = s.settings.Get(ctx, "generation.default_lang", &result.Language)
	_ = s.settings.Get(ctx, "generation.default_audience", &result.Audience)
	_ = s.settings.Get(ctx, "generation.default_tone", &result.Tone)
	if result.SlideCount < 1 || result.SlideCount > s.maximumSlides(ctx) {
		result.SlideCount = 8
	}
	return result
}

func mergePresentationInput(target *store.PresentationInput, input presentationRequest) {
	if input.Title != nil {
		target.Title = strings.TrimSpace(*input.Title)
	}
	if input.Prompt != nil {
		target.Prompt = strings.TrimSpace(*input.Prompt)
	}
	if input.Theme != nil {
		target.Theme = strings.TrimSpace(*input.Theme)
	}
	if input.Language != nil {
		target.Language = strings.TrimSpace(*input.Language)
	}
	if input.Audience != nil {
		target.Audience = strings.TrimSpace(*input.Audience)
	}
	if input.Tone != nil {
		target.Tone = strings.TrimSpace(*input.Tone)
	}
	if input.RequestedSlideCount != nil {
		target.SlideCount = *input.RequestedSlideCount
	}
	if input.SlideCount != nil {
		target.SlideCount = *input.SlideCount
	}
	if input.TemplateID != nil {
		if trimmed := strings.TrimSpace(*input.TemplateID); trimmed != "" {
			target.TemplateID = &trimmed
		} else {
			target.TemplateID = nil
		}
	}
}

// resolveTemplateSelection validates the chosen template and falls back to the
// built-in design matching the requested theme, so a deck always has a design
// to generate into.
func (s *Server) resolveTemplateSelection(ctx context.Context, ownerID string, input *store.PresentationInput) error {
	if input.TemplateID != nil {
		if _, err := uuid.Parse(*input.TemplateID); err != nil {
			return errors.New("templateId must be a UUID")
		}
		if _, err := s.store.GetTemplate(ctx, *input.TemplateID, ownerID, false); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return errors.New("the selected template does not exist")
			}
			return err
		}
		return nil
	}
	resolved, err := s.store.DefaultTemplateID(ctx, ownerID, input.Theme)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	input.TemplateID = &resolved
	return nil
}

func validatePresentationInput(input store.PresentationInput, maximumSlides int) string {
	if input.Title == "" || utf8.RuneCountInString(input.Title) > 200 {
		return "title is required and must not exceed 200 characters"
	}
	if utf8.RuneCountInString(input.Prompt) > 12000 {
		return "prompt must not exceed 12000 characters"
	}
	if input.SlideCount < 1 || input.SlideCount > maximumSlides {
		return fmt.Sprintf("requestedSlideCount must be between 1 and %d", maximumSlides)
	}
	if input.Theme == "" || utf8.RuneCountInString(input.Theme) > 80 || input.Language == "" || utf8.RuneCountInString(input.Language) > 32 {
		return "theme and language are required"
	}
	if input.Audience == "" || utf8.RuneCountInString(input.Audience) > 300 || input.Tone == "" || utf8.RuneCountInString(input.Tone) > 80 {
		return "audience and tone are required"
	}
	return ""
}

func validatePresentationEditInput(input store.PresentationInput) string {
	return validatePresentationInput(input, 50)
}

func (s *Server) maximumSlides(ctx context.Context) int {
	maximum := 50
	if s.settings.Get(ctx, "generation.max_slides", &maximum) != nil || maximum < 1 || maximum > 50 {
		return 50
	}
	return maximum
}

func convertSlide(input slideRequest, index int) (model.Slide, error) {
	if len(input.Content) == 0 || !json.Valid(input.Content) {
		return model.Slide{}, errors.New("slide content must be valid JSON")
	}
	var contentFields struct {
		Title        string `json:"title"`
		Subtitle     string `json:"subtitle"`
		SpeakerNotes string `json:"speaker_notes"`
	}
	_ = json.Unmarshal(input.Content, &contentFields)
	if input.Title == "" {
		input.Title = contentFields.Title
	}
	if input.Subtitle == "" {
		input.Subtitle = contentFields.Subtitle
	}
	if input.SpeakerNotes == "" {
		input.SpeakerNotes = contentFields.SpeakerNotes
	}
	if input.Title == "" || utf8.RuneCountInString(input.Title) > 200 {
		return model.Slide{}, errors.New("slide title is required and must not exceed 200 characters")
	}
	if utf8.RuneCountInString(input.Subtitle) > 300 || utf8.RuneCountInString(input.SpeakerNotes) > 4000 {
		return model.Slide{}, errors.New("slide subtitle or speaker notes exceed their allowed length")
	}
	if input.Layout == "" {
		input.Layout = "content"
	}
	if utf8.RuneCountInString(input.Layout) > 80 {
		return model.Slide{}, errors.New("slide layout must not exceed 80 characters")
	}
	if utf8.RuneCountInString(input.LayoutID) > 80 {
		return model.Slide{}, errors.New("slide layoutId must not exceed 80 characters")
	}
	if input.ID != "" {
		if _, err := uuid.Parse(input.ID); err != nil {
			return model.Slide{}, errors.New("slide id must be a UUID")
		}
	}
	return model.Slide{ID: input.ID, Position: index + 1, Title: input.Title, Subtitle: input.Subtitle,
		Content: input.Content, SpeakerNotes: input.SpeakerNotes, Layout: input.Layout, LayoutID: input.LayoutID}, nil
}

func (s *Server) handleStoreError(writer http.ResponseWriter, request *http.Request, err error, code string) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(writer, request, http.StatusNotFound, "not_found", "The requested resource was not found", nil)
		return
	}
	s.internalError(writer, request, code, err)
}

func pagination(request *http.Request) (int, int) {
	limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(request.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, 2<<20)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(writer, request, http.StatusBadRequest, "invalid_json", "Request body is not valid for this operation", map[string]any{"reason": err.Error()})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(writer, request, http.StatusBadRequest, "invalid_json", "Request body must contain one JSON value", nil)
		return false
	}
	return true
}

func safeFilename(value string) string {
	value = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\\|?*`, r) || r < 32 {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		return "ptium-presentation"
	}
	if utf8.RuneCountInString(value) > 100 {
		value = string([]rune(value)[:100])
	}
	return value
}
