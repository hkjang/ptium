package httpapi

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hkjang/ptium/server/internal/generation"
	"github.com/hkjang/ptium/server/internal/store"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
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
		sensitive := update.Sensitive || sensitiveSettingKey(update.Key)
		if sensitive {
			// Clearing a secret is typing whitespace into a box that shows
			// nothing. Stored as it was typed, a single space is a secret as far
			// as the settings are concerned — reported as configured, and read
			// back at boot as unset. It means "remove this", so remove it.
			var typed string
			if err := json.Unmarshal(update.Value, &typed); err == nil && strings.TrimSpace(typed) == "" {
				update.Value = json.RawMessage(`""`)
			}
		}
		prepared = append(prepared, settings.Update{Key: update.Key, Value: update.Value, Sensitive: sensitive, Description: update.Description})
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
	issuer, clientID, clientSecret := "", "", ""
	_ = s.settings.Get(ctx, "generation.default_slide_count", &defaultSlides)
	_ = s.settings.Get(ctx, "generation.max_slides", &maximumSlides)
	_ = s.settings.Get(ctx, "auth.oidc.issuer_url", &issuer)
	_ = s.settings.Get(ctx, "auth.oidc.client_id", &clientID)
	_ = s.settings.Get(ctx, "auth.oidc.client_secret", &clientSecret)
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
		case "auth.oidc.client_secret":
			_ = json.Unmarshal(update.Value, &clientSecret)
		}
	}
	if defaultSlides > maximumSlides {
		return errors.New("generation default slide count cannot exceed the maximum")
	}
	if strings.TrimSpace(issuer) != "" && strings.TrimSpace(clientID) == "" {
		return errors.New("OIDC client ID is required when an issuer is configured")
	}
	if strings.TrimSpace(clientSecret) != "" && strings.TrimSpace(clientID) == "" {
		// A secret with no client is a secret for nobody, and the server refuses
		// to start on it. Better said now than at the next rollout.
		return errors.New("OIDC client ID is required when a client secret is set")
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
	case "auth.oidc.client_secret":
		value, err := decodeString()
		if err != nil {
			return err
		}
		if utf8.RuneCountInString(value) > 500 {
			return errors.New("OIDC client secret must be at most 500 characters")
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

// adminCheckProvider asks the configured model host whether it is there.
//
// Setting a provider and finding out whether it answers were two different
// days: the only way to learn was to generate a deck and watch it fail. This
// asks with the settings as stored and saves nothing.
//
// A POST is somebody asking, and is written down and always fresh. A GET is a
// screen reading, and is neither: a dashboard that writes an audit entry every
// time it opens fills the trail with itself, and one that knocks on a shared
// model host on every refresh is a nuisance to whoever else uses it.
func (s *Server) adminCheckProvider(writer http.ResponseWriter, request *http.Request) {
	if s.generator == nil {
		writeError(writer, request, http.StatusServiceUnavailable, "ai_unavailable",
			"This deployment has no AI provider configured", nil)
		return
	}
	asked := request.Method == http.MethodPost
	if !asked {
		if cached, ok := s.recentProviderCheck(); ok {
			writeData(writer, request, http.StatusOK, cached)
			return
		}
	}
	check := s.generator.CheckProvider(request.Context())
	s.rememberProviderCheck(check)
	if asked {
		actor, _ := UserFromContext(request.Context())
		// On a deployment where several installations share one model host, who
		// knocked on it and when is itself worth being able to see.
		s.store.Audit(request.Context(), &actor.ID, "settings.provider_check", "settings", "ai.provider",
			map[string]any{"reachable": check.Reachable, "ms": check.Milliseconds})
	}
	writeData(writer, request, http.StatusOK, check)
}

// providerCheckFor is how long a reading of the provider stands. Long enough
// that a dashboard being refreshed asks once, short enough that an operator
// who just fixed the host does not read a stale "응답 없음" for long.
const providerCheckFor = 30 * time.Second

func (s *Server) recentProviderCheck() (generation.ProviderCheck, bool) {
	s.providerCheckMu.Lock()
	defer s.providerCheckMu.Unlock()
	if s.providerCheckAt.IsZero() || time.Since(s.providerCheckAt) > providerCheckFor {
		return generation.ProviderCheck{}, false
	}
	return s.providerCheck, true
}

func (s *Server) rememberProviderCheck(check generation.ProviderCheck) {
	s.providerCheckMu.Lock()
	defer s.providerCheckMu.Unlock()
	s.providerCheck, s.providerCheckAt = check, time.Now()
}

// adminStorage is what this deployment is keeping and how much room is left.
//
// A box off the network has one disk, and when it fills the failures arrive as
// whatever the layer underneath happens to raise. These are the numbers that
// say why before that happens.
func (s *Server) adminStorage(writer http.ResponseWriter, request *http.Request) {
	usage, err := s.store.Storage(request.Context(), s.assetDir)
	if err != nil {
		s.internalError(writer, request, "admin_storage_read_failed", err)
		return
	}
	writeData(writer, request, http.StatusOK, usage)
}

// adminGenerationQueue is what is waiting, what is being written, and what
// failed recently — the list behind the overview's "the oldest has been waiting
// twenty minutes".
func (s *Server) adminGenerationQueue(writer http.ResponseWriter, request *http.Request) {
	hours := 24
	if asked := strings.TrimSpace(request.URL.Query().Get("failedHours")); asked != "" {
		if value, err := strconv.Atoi(asked); err == nil && value >= 0 {
			hours = min(value, 24*30)
		}
	}
	queue, err := s.store.GenerationQueue(request.Context(), hours, 100)
	if err != nil {
		s.internalError(writer, request, "admin_queue_read_failed", err)
		return
	}
	writeData(writer, request, http.StatusOK, queue)
}

// adminRequeueGeneration puts a deck back in the queue.
//
// A deck that failed, or one a dead worker left behind, is a deck its author
// cannot get back without asking for the whole thing again. An operator who can
// see the queue should be able to push one through it.
func (s *Server) adminRequeueGeneration(writer http.ResponseWriter, request *http.Request) {
	actor, _ := UserFromContext(request.Context())
	deck, err := s.store.GetPresentation(request.Context(), request.PathValue("id"), "", true)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	queued, err := s.store.QueueGeneration(request.Context(), deck.ID, deck.OwnerID, true, s.maximumSlides(request.Context()))
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_queue_failed")
		return
	}
	// Written down like everything else: the trail says who pushed it.
	s.store.Audit(request.Context(), &actor.ID, "generation.requeue", "presentation", deck.ID,
		map[string]any{"was": deck.Status, "owner": deck.OwnerID})
	writeData(writer, request, http.StatusOK, queued)
}

// adminCancelGeneration stops a deck that is going nowhere, with a reason its
// author can read.
func (s *Server) adminCancelGeneration(writer http.ResponseWriter, request *http.Request) {
	actor, _ := UserFromContext(request.Context())
	var input struct {
		Reason string `json:"reason"`
	}
	if request.ContentLength > 0 && !decodeJSON(writer, request, &input) {
		return
	}
	deck, err := s.store.GetPresentation(request.Context(), request.PathValue("id"), "", true)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	if deck.Status != "queued" && deck.Status != "generating" {
		writeError(writer, request, http.StatusConflict, "not_in_the_queue",
			"That deck is not waiting or being written", map[string]any{"status": deck.Status})
		return
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "관리자가 생성을 중단했습니다"
	}
	stopped, err := s.store.StopGeneration(request.Context(), deck.ID, reason)
	if err != nil {
		s.internalError(writer, request, "presentation_cancel_failed", err)
		return
	}
	if !stopped {
		// It finished between the queue being read and the button being pressed.
		// Answering as though it had been stopped would leave an operator sure
		// they stopped something they did not.
		current, readErr := s.store.GetPresentation(request.Context(), deck.ID, "", true)
		if readErr != nil {
			s.handleStoreError(writer, request, readErr, "presentation_read_failed")
			return
		}
		writeError(writer, request, http.StatusConflict, "not_in_the_queue",
			"That deck finished before it could be stopped", map[string]any{"status": current.Status})
		return
	}
	s.store.Audit(request.Context(), &actor.ID, "generation.cancel", "presentation", deck.ID,
		map[string]any{"was": deck.Status, "reason": reason})
	standing, err := s.store.GetPresentation(request.Context(), deck.ID, "", true)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	writeData(writer, request, http.StatusOK, standing)
}

// adminListAuditTrail answers what was written down about who did what.
//
// The trail is filtered the way an operator arrives at it: with an action
// ("who changed a setting"), a person ("what did this address do"), or one
// thing ("what happened to this deck"). Everything is optional, and the
// default is the last thing that happened.
func (s *Server) adminListAuditTrail(writer http.ResponseWriter, request *http.Request) {
	limit, offset := pagination(request)
	query := request.URL.Query()
	filter := store.AuditFilter{
		Action:   strings.TrimSpace(query.Get("action")),
		Actor:    strings.TrimSpace(query.Get("actor")),
		Target:   strings.TrimSpace(query.Get("target")),
		TargetID: strings.TrimSpace(query.Get("targetId")),
		Search:   strings.TrimSpace(query.Get("search")),
	}
	// "since" is said in days, because that is how the question is asked: what
	// happened today, this week, since the deployment on Tuesday.
	if days := strings.TrimSpace(query.Get("days")); days != "" {
		if count, err := strconv.Atoi(days); err == nil && count > 0 {
			if count > 3650 {
				count = 3650
			}
			filter.Since = time.Now().AddDate(0, 0, -count)
		}
	}
	// An auditor asks for the trail as a file, and a page of fifty is not that:
	// the whole of what was asked for goes out at once, in the order it is read
	// on screen.
	if strings.EqualFold(strings.TrimSpace(query.Get("format")), "csv") {
		s.writeAuditCSV(writer, request, filter)
		return
	}
	entries, total, err := s.store.ListAuditTrail(request.Context(), filter, limit, offset)
	if err != nil {
		s.internalError(writer, request, "admin_audit_read_failed", err)
		return
	}
	writeList(writer, request, entries, total, limit, offset)
}

// writeAuditCSV sends the filtered trail as a file.
//
// It is written straight to the response a page at a time rather than gathered
// first: a year of a busy deployment is hundreds of thousands of rows, and
// holding all of them to hand over a file is how a report takes the server with
// it.
func (s *Server) writeAuditCSV(writer http.ResponseWriter, request *http.Request, filter store.AuditFilter) {
	writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	writer.Header().Set("Content-Disposition",
		`attachment; filename="ptium-audit-`+time.Now().Format("20060102")+`.csv"`)
	// Excel reads a CSV as the machine's own code page unless the file says
	// otherwise, and a trail full of Korean opens as mojibake without this.
	if _, err := writer.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return
	}
	out := csv.NewWriter(writer)
	defer out.Flush()
	_ = out.Write([]string{"시각", "동작", "행위자", "행위자 ID", "대상 종류", "대상 ID", "기록된 값"})
	const page = 500
	for offset := 0; offset < auditExportLimit; offset += page {
		entries, _, err := s.store.ListAuditTrail(request.Context(), filter, page, offset)
		if err != nil {
			s.logger.Warn("the audit export stopped early", "error", err,
				"request_id", RequestID(request.Context()))
			return
		}
		for _, entry := range entries {
			actor := entry.ActorEmail
			if actor == "" {
				actor = entry.ActorName
			}
			_ = out.Write([]string{
				entry.CreatedAt.Format(time.RFC3339), entry.Action, actor, entry.ActorID,
				entry.TargetType, entry.TargetID, string(entry.Metadata),
			})
		}
		out.Flush()
		if len(entries) < page {
			return
		}
	}
}

// auditExportLimit is where a single export stops. A file nobody can open is
// not a report, and a filter narrows it further than this in every real use.
const auditExportLimit = 100000

// adminAuditActions is what the trail holds, so a filter can offer the actions
// this deployment actually writes rather than a list somebody has to remember.
func (s *Server) adminAuditActions(writer http.ResponseWriter, request *http.Request) {
	var since time.Time
	if days := strings.TrimSpace(request.URL.Query().Get("days")); days != "" {
		if count, err := strconv.Atoi(days); err == nil && count > 0 {
			since = time.Now().AddDate(0, 0, -count)
		}
	}
	actions, err := s.store.AuditActions(request.Context(), since)
	if err != nil {
		s.internalError(writer, request, "admin_audit_read_failed", err)
		return
	}
	writeData(writer, request, http.StatusOK, actions)
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
