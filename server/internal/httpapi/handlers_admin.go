package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/settings"
)

func (s *Server) adminOverview(writer http.ResponseWriter, request *http.Request) {
	overview, err := s.store.AdminOverview(request.Context())
	if err != nil {
		s.internalError(writer, request, "admin_overview_failed", err)
		return
	}
	writeData(writer, request, http.StatusOK, overview)
}

func (s *Server) adminListSettings(writer http.ResponseWriter, request *http.Request) {
	items, err := s.settings.ListForAdmin(request.Context())
	if err != nil {
		s.internalError(writer, request, "admin_settings_read_failed", err)
		return
	}
	writeData(writer, request, http.StatusOK, items)
}

type settingUpdate struct {
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"`
	Sensitive   bool            `json:"sensitive"`
	Description string          `json:"description"`
}

type settingsBatchRequest struct {
	Section  string          `json:"section"`
	Settings json.RawMessage `json:"settings"`
	Values   json.RawMessage `json:"values"`
}

func (s *Server) adminPutSettings(writer http.ResponseWriter, request *http.Request) {
	var input settingsBatchRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	raw := input.Settings
	if len(raw) == 0 {
		raw = input.Values
	}
	updates, err := parseSettingUpdates(input.Section, raw)
	if err != nil || len(updates) == 0 {
		message := "settings must contain at least one setting"
		if err != nil {
			message = err.Error()
		}
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", message, nil)
		return
	}
	user, _ := UserFromContext(request.Context())
	prepared := make([]settings.Update, 0, len(updates))
	for _, update := range updates {
		update.Key = strings.TrimSpace(update.Key)
		if !settingKeyPattern.MatchString(update.Key) || len(update.Key) > 100 {
			writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "setting key must use dotted lowercase names", nil)
			return
		}
		if len(update.Value) == 0 || !json.Valid(update.Value) {
			writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "setting value must be valid JSON", map[string]any{"key": update.Key})
			return
		}
		if err := validateSettingValue(update.Key, update.Value); err != nil {
			writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), map[string]any{"key": update.Key})
			return
		}
		prepared = append(prepared, settings.Update{Key: update.Key, Value: update.Value, Sensitive: update.Sensitive || sensitiveSettingKey(update.Key), Description: update.Description})
	}
	if err := s.validateSettingRelationships(request.Context(), prepared); err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
		return
	}
	result, err := s.settings.PutBatch(request.Context(), user.ID, prepared)
	if err != nil {
		s.internalError(writer, request, "admin_settings_update_failed", err)
		return
	}
	s.store.Audit(request.Context(), &user.ID, "settings.update_batch", "settings", input.Section, map[string]any{"count": len(result)})
	writeData(writer, request, http.StatusOK, result)
}

func (s *Server) adminPutSetting(writer http.ResponseWriter, request *http.Request) {
	var input settingUpdate
	if !decodeJSON(writer, request, &input) {
		return
	}
	input.Key = request.PathValue("key")
	user, _ := UserFromContext(request.Context())
	setting, ok := s.putSetting(writer, request, user.ID, input)
	if !ok {
		return
	}
	s.store.Audit(request.Context(), &user.ID, "settings.update", "setting", input.Key, nil)
	writeData(writer, request, http.StatusOK, setting)
}

var settingKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

func (s *Server) putSetting(writer http.ResponseWriter, request *http.Request, userID string, input settingUpdate) (any, bool) {
	input.Key = strings.TrimSpace(input.Key)
	if !settingKeyPattern.MatchString(input.Key) || len(input.Key) > 100 {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "setting key must use dotted lowercase names", nil)
		return nil, false
	}
	if len(input.Value) == 0 || !json.Valid(input.Value) {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "setting value must be valid JSON", nil)
		return nil, false
	}
	if err := validateSettingValue(input.Key, input.Value); err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), map[string]any{"key": input.Key})
		return nil, false
	}
	sensitive := input.Sensitive || sensitiveSettingKey(input.Key)
	if err := s.validateSettingRelationships(request.Context(), []settings.Update{{Key: input.Key, Value: input.Value, Sensitive: sensitive, Description: input.Description}}); err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
		return nil, false
	}
	setting, err := s.settings.Put(request.Context(), userID, input.Key, input.Value, sensitive, input.Description)
	if err != nil {
		s.internalError(writer, request, "admin_setting_update_failed", err)
		return nil, false
	}
	return setting, true
}

func (s *Server) validateSettingRelationships(ctx context.Context, updates []settings.Update) error {
	defaultSlides, maximumSlides := 10, 50
	issuer, clientID := "", ""
	_ = s.settings.Get(ctx, "generation.default_slide_count", &defaultSlides)
	_ = s.settings.Get(ctx, "generation.max_slides", &maximumSlides)
	_ = s.settings.Get(ctx, "auth.oidc.issuer_url", &issuer)
	_ = s.settings.Get(ctx, "auth.oidc.client_id", &clientID)
	for _, update := range updates {
		switch update.Key {
		case "generation.default_slide_count":
			_ = json.Unmarshal(update.Value, &defaultSlides)
		case "generation.max_slides":
			_ = json.Unmarshal(update.Value, &maximumSlides)
		case "auth.oidc.issuer_url":
			_ = json.Unmarshal(update.Value, &issuer)
		case "auth.oidc.client_id":
			_ = json.Unmarshal(update.Value, &clientID)
		}
	}
	if defaultSlides > maximumSlides {
		return errors.New("generation default slide count cannot exceed the maximum")
	}
	if strings.TrimSpace(issuer) != "" && strings.TrimSpace(clientID) == "" {
		return errors.New("OIDC client ID is required when an issuer is configured")
	}
	return nil
}

func parseSettingUpdates(section string, raw json.RawMessage) ([]settingUpdate, error) {
	var list []settingUpdate
	if len(raw) == 0 {
		return nil, nil
	}
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, err
		}
		return list, nil
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	for key, value := range values {
		if section != "" && !strings.Contains(key, ".") {
			key = section + "." + key
		}
		list = append(list, settingUpdate{Key: key, Value: value, Sensitive: sensitiveSettingKey(key)})
	}
	return list, nil
}

func sensitiveSettingKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "api_key") || strings.Contains(lower, "client_secret") || strings.Contains(lower, "password") || strings.HasSuffix(lower, ".secret")
}

func validateSettingValue(key string, raw json.RawMessage) error {
	decodeString := func() (string, error) {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", errors.New("setting value must be a string")
		}
		return strings.TrimSpace(value), nil
	}
	validURL := func(value string, httpsOnly, originOnly bool) bool {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return false
		}
		if parsed.Scheme != "https" && (httpsOnly || parsed.Scheme != "http") {
			return false
		}
		return !originOnly || parsed.Path == "" || parsed.Path == "/"
	}

	switch key {
	case "ai.provider":
		value, err := decodeString()
		if err != nil || (value != "fallback" && value != "openai" && value != "openai-compatible") {
			return errors.New("AI provider must be fallback, openai, or openai-compatible")
		}
	case "ai.base_url":
		value, err := decodeString()
		if err != nil || !validURL(value, false, false) {
			return errors.New("AI base URL must be an HTTP(S) URL without credentials, query, or fragment")
		}
	case "ai.model", "branding.product_name", "generation.default_theme", "generation.default_lang", "generation.default_tone", "generation.default_audience":
		value, err := decodeString()
		if err != nil || value == "" || utf8.RuneCountInString(value) > 200 {
			return errors.New("setting value must contain 1-200 characters")
		}
	case "branding.brand_color":
		value, err := decodeString()
		if err != nil || !regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`).MatchString(value) {
			return errors.New("brand color must use #RRGGBB format")
		}
	case "branding.logo_url":
		value, err := decodeString()
		if err != nil || (value != "" && !validURL(value, false, false)) {
			return errors.New("logo URL must be empty or a valid HTTP(S) URL")
		}
	case "auth.oidc.issuer_url":
		value, err := decodeString()
		if err != nil || (value != "" && !validURL(value, true, false)) {
			return errors.New("OIDC issuer must be empty or a valid HTTPS URL")
		}
	case "auth.oidc.client_id":
		if _, err := decodeString(); err != nil {
			return err
		}
	case "auth.oidc.admin_roles":
		var roles []string
		if err := json.Unmarshal(raw, &roles); err != nil || len(roles) == 0 {
			return errors.New("OIDC admin roles must be a non-empty string array")
		}
		for _, role := range roles {
			if strings.TrimSpace(role) == "" || utf8.RuneCountInString(role) > 100 {
				return errors.New("OIDC admin roles must contain non-empty values of at most 100 characters")
			}
		}
	case "generation.default_slide_count", "generation.max_slides":
		var value int
		if err := json.Unmarshal(raw, &value); err != nil || value < 1 || value > 50 {
			return errors.New("slide count settings must be integers between 1 and 50")
		}
	case "security.api_key_grace":
		value, err := decodeString()
		if err != nil {
			return err
		}
		duration, err := time.ParseDuration(value)
		if err != nil || duration < 0 || duration > 30*24*time.Hour {
			return errors.New("API key grace must be a duration between zero and 30 days")
		}
	case "security.cors_origins":
		var origins []string
		if err := json.Unmarshal(raw, &origins); err != nil {
			return errors.New("CORS origins must be a string array")
		}
		for _, origin := range origins {
			if !validURL(strings.TrimSpace(origin), false, true) {
				return fmt.Errorf("invalid CORS origin %q", origin)
			}
		}
	}
	return nil
}

func (s *Server) adminListUsers(writer http.ResponseWriter, request *http.Request) {
	limit, offset := pagination(request)
	items, total, err := s.store.ListUsers(request.Context(), strings.TrimSpace(request.URL.Query().Get("search")), limit, offset)
	if err != nil {
		s.internalError(writer, request, "admin_users_read_failed", err)
		return
	}
	writeList(writer, request, items, total, limit, offset)
}

type adminUserUpdate struct {
	IsAdmin  *bool `json:"isAdmin"`
	Disabled *bool `json:"disabled"`
}

func (s *Server) adminUpdateUser(writer http.ResponseWriter, request *http.Request) {
	var input adminUserUpdate
	if !decodeJSON(writer, request, &input) {
		return
	}
	actor, _ := UserFromContext(request.Context())
	current, err := s.store.GetUser(request.Context(), request.PathValue("id"))
	if err != nil {
		s.handleStoreError(writer, request, err, "admin_user_read_failed")
		return
	}
	isAdmin, disabled := current.IsAdmin, current.Disabled
	if input.IsAdmin != nil {
		if !*input.IsAdmin && (containsAnyFold(current.Roles, s.adminRoles) || containsFold(s.bootstrapAdminEmails, current.Email) || contains(s.bootstrapAdminSubjects, current.Subject)) {
			writeError(writer, request, http.StatusConflict, "externally_managed_admin", "Remove the administrator role or bootstrap selector at the identity source before demoting this account", nil)
			return
		}
		isAdmin = *input.IsAdmin
	}
	if input.Disabled != nil {
		disabled = *input.Disabled
	}
	if current.ID == actor.ID && (!isAdmin || disabled) {
		writeError(writer, request, http.StatusConflict, "cannot_disable_self", "Administrators cannot disable or demote their own active account", nil)
		return
	}
	updated, err := s.store.UpdateUserAdmin(request.Context(), current.ID, isAdmin, disabled)
	if err != nil {
		s.handleStoreError(writer, request, err, "admin_user_update_failed")
		return
	}
	s.store.Audit(request.Context(), &actor.ID, "user.admin_update", "user", current.ID, map[string]any{"isAdmin": isAdmin, "disabled": disabled})
	writeData(writer, request, http.StatusOK, updated)
}

func (s *Server) adminListErrors(writer http.ResponseWriter, request *http.Request) {
	limit, offset := pagination(request)
	status := strings.TrimSpace(request.URL.Query().Get("status"))
	if status != "" && !validIncidentStatus(status) {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "invalid incident status", nil)
		return
	}
	items, total, err := s.store.ListIncidents(request.Context(), status, limit, offset)
	if err != nil {
		s.internalError(writer, request, "admin_errors_read_failed", err)
		return
	}
	writeList(writer, request, items, total, limit, offset)
}

type incidentUpdate struct {
	Status string  `json:"status"`
	Notes  *string `json:"notes"`
}

func (s *Server) adminUpdateError(writer http.ResponseWriter, request *http.Request) {
	var input incidentUpdate
	if !decodeJSON(writer, request, &input) {
		return
	}
	if !validIncidentStatus(input.Status) || (input.Notes != nil && utf8.RuneCountInString(*input.Notes) > 4000) {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "invalid incident status or notes", nil)
		return
	}
	actor, _ := UserFromContext(request.Context())
	updated, err := s.store.UpdateIncident(request.Context(), request.PathValue("id"), actor.ID, input.Status, input.Notes)
	if err != nil {
		s.handleStoreError(writer, request, err, "admin_error_update_failed")
		return
	}
	s.store.Audit(request.Context(), &actor.ID, "incident.update", "incident", updated.ID, map[string]any{"status": input.Status})
	writeData(writer, request, http.StatusOK, updated)
}

func validIncidentStatus(value string) bool {
	return value == "open" || value == "acknowledged" || value == "resolved" || value == "ignored"
}
