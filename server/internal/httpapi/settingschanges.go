package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/hkjang/ptium/server/internal/settings"
	"github.com/hkjang/ptium/server/internal/store"
)

// What a settings change was, kept where an operator can read it.
//
// A change used to be recorded as "settings.update_batch" with a count. Which
// setting, what it had been, what it became and who did it were nowhere: the
// question an audit trail exists to answer — who changed this, and what was it
// before — could not be answered about the settings that decide how every deck
// in the deployment is written.
//
// A secret's value is never written down. That it changed, and by whom, is.
const settingChangeAction = "settings.change"

// settingsNow reads the values a change is about to replace.
func (s *Server) settingsNow(ctx context.Context) map[string]json.RawMessage {
	current := map[string]json.RawMessage{}
	stored, err := s.settings.ListForAdmin(ctx)
	if err != nil {
		return current
	}
	for _, setting := range stored {
		if setting.Sensitive || len(setting.Value) == 0 {
			continue
		}
		current[setting.Key] = setting.Value
	}
	return current
}

// auditSettingChange writes one row per setting, with what it was and is.
func (s *Server) auditSettingChange(ctx context.Context, actorID string, update settings.Update, before json.RawMessage) {
	detail := map[string]any{"key": update.Key}
	switch {
	case update.Sensitive:
		// A secret is recorded as having changed and never as what it is.
		detail["sensitive"] = true
	default:
		if len(before) > 0 {
			detail["from"] = json.RawMessage(before)
		}
		detail["to"] = json.RawMessage(update.Value)
		if len(before) > 0 && strings.TrimSpace(string(before)) == strings.TrimSpace(string(update.Value)) {
			// Saving a screen writes every field on it. A field that came back
			// as it went is not a change anybody made.
			return
		}
	}
	s.store.Audit(ctx, &actorID, settingChangeAction, "setting", update.Key, detail)
}

// adminSettingChanges is the settings' own trail: which setting, from what to
// what, by whom. It is the audit trail filtered to one question, because an
// operator asking it should not have to know that settings changes live among
// every other kind of event.
func (s *Server) adminSettingChanges(writer http.ResponseWriter, request *http.Request) {
	limit, offset := pagination(request)
	entries, total, err := s.store.ListAuditTrail(request.Context(),
		store.AuditFilter{Action: settingChangeAction, TargetID: strings.TrimSpace(request.URL.Query().Get("key"))},
		limit, offset)
	if err != nil {
		s.internalError(writer, request, "admin_setting_changes_failed", err)
		return
	}
	writeList(writer, request, entries, total, limit, offset)
}

// adminRevertSettingChange puts a setting back to what this change replaced.
//
// Reading a trail and then retyping the old value by hand is where an operator
// makes the second mistake. The value is validated the way any other change is,
// and putting it back is itself a change: the trail says who did that too.
func (s *Server) adminRevertSettingChange(writer http.ResponseWriter, request *http.Request) {
	id, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(writer, request, http.StatusNotFound, "not_found", "That change is not in the trail", nil)
		return
	}
	entry, err := s.store.AuditEntry(request.Context(), id)
	if err != nil {
		s.handleStoreError(writer, request, err, "admin_setting_change_read_failed")
		return
	}
	if entry.Action != settingChangeAction {
		writeError(writer, request, http.StatusUnprocessableEntity, "not_a_setting_change",
			"이 기록은 설정 변경이 아닙니다", nil)
		return
	}
	var detail struct {
		Key       string          `json:"key"`
		From      json.RawMessage `json:"from"`
		Sensitive bool            `json:"sensitive"`
	}
	_ = json.Unmarshal(entry.Metadata, &detail)
	switch {
	case detail.Sensitive:
		writeError(writer, request, http.StatusUnprocessableEntity, "secret_not_recorded",
			"비밀 값은 기록하지 않으므로 되돌릴 수 없습니다. 값을 다시 입력해 주세요", nil)
		return
	case detail.Key == "" || len(detail.From) == 0:
		writeError(writer, request, http.StatusUnprocessableEntity, "nothing_to_revert",
			"이 변경에는 되돌릴 이전 값이 없습니다", nil)
		return
	}
	if err := validateSettingValue(detail.Key, detail.From); err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(),
			map[string]any{"key": detail.Key})
		return
	}
	user, _ := UserFromContext(request.Context())
	update := settings.Update{Key: detail.Key, Value: detail.From, Sensitive: sensitiveSettingKey(detail.Key)}
	if err := s.validateSettingRelationships(request.Context(), []settings.Update{update}); err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
		return
	}
	was := s.settingsNow(request.Context())
	if _, err := s.settings.PutBatch(request.Context(), user.ID, []settings.Update{update}); err != nil {
		s.internalError(writer, request, "admin_settings_update_failed", err)
		return
	}
	s.forgetProviderCheckIfAI([]string{detail.Key})
	s.auditSettingChange(request.Context(), user.ID, update, was[detail.Key])
	s.store.Audit(request.Context(), &user.ID, "settings.revert", "setting", detail.Key,
		map[string]any{"change": id})
	writeData(writer, request, http.StatusOK, map[string]any{"key": detail.Key, "value": json.RawMessage(detail.From)})
}
