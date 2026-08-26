package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/hkjang/ptium/server/internal/settings"
	"github.com/hkjang/ptium/server/internal/store"
)

// The designs this deployment writes decks in.
//
// A person's own screens show the designs they may use. An operator asked which
// designs their organisation actually writes in — or asked to make one team's
// upload the standard for everybody — could see none of it: uploads are private
// to whoever uploaded them, and the standard was a text field in settings whose
// value meant nothing to anyone reading it.

func (s *Server) adminListTemplates(writer http.ResponseWriter, request *http.Request) {
	limit, offset := pagination(request)
	kind := strings.ToLower(strings.TrimSpace(request.URL.Query().Get("kind")))
	switch kind {
	case "", "builtin", "uploaded":
	default:
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error",
			"kind must be builtin or uploaded", nil)
		return
	}
	standard := ""
	_ = s.settings.Get(request.Context(), "generation.default_theme", &standard)
	designs, total, err := s.store.ListDeploymentTemplates(request.Context(),
		store.TemplateFilter{Kind: kind, Search: searchTerm(request)}, standard, limit, offset)
	if err != nil {
		s.internalError(writer, request, "admin_templates_read_failed", err)
		return
	}
	writeList(writer, request, designs, total, limit, offset)
}

// adminSetStandardTemplate makes one design what a new deck lands in when
// nobody chooses. It is the same setting the settings screen holds, set from
// where an operator can see what they are choosing.
func (s *Server) adminSetStandardTemplate(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	template, err := s.store.GetTemplate(request.Context(), request.PathValue("id"), user.ID, true)
	if err != nil {
		s.handleStoreError(writer, request, err, "template_read_failed")
		return
	}
	key := strings.TrimSpace(template.PaletteKey)
	if key == "" {
		// An uploaded design has no palette key of its own; the deployment's
		// standard is then that template itself.
		key = template.ID
	}
	value, _ := json.Marshal(key)
	if err := validateSettingValue("generation.default_theme", value); err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
		return
	}
	was := s.settingsNow(request.Context())
	update := settings.Update{Key: "generation.default_theme", Value: value}
	if _, err := s.settings.PutBatch(request.Context(), user.ID, []settings.Update{update}); err != nil {
		s.internalError(writer, request, "admin_settings_update_failed", err)
		return
	}
	// Changing the standard is a settings change like any other, and the trail
	// says which design it became rather than only that something changed.
	s.auditSettingChange(request.Context(), user.ID, update, was["generation.default_theme"])
	writeData(writer, request, http.StatusOK, map[string]any{
		"standard": key, "templateId": template.ID, "name": template.Name})
}

// adminShareTemplate opens one person's upload to everybody, or takes it back
// to being theirs. A built-in design is everybody's already.
func (s *Server) adminShareTemplate(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	var input struct {
		Shared *bool `json:"shared"`
	}
	if !decodeJSON(writer, request, &input) {
		return
	}
	if input.Shared == nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error",
			"shared must be true or false", nil)
		return
	}
	template, err := s.store.GetTemplate(request.Context(), request.PathValue("id"), user.ID, true)
	if err != nil {
		s.handleStoreError(writer, request, err, "template_read_failed")
		return
	}
	if template.Kind != "uploaded" {
		writeError(writer, request, http.StatusUnprocessableEntity, "builtin_template",
			"내장 디자인은 이미 모두가 쓸 수 있습니다", nil)
		return
	}
	scope := "private"
	if *input.Shared {
		scope = "shared"
	}
	updated, err := s.store.UpdateTemplate(request.Context(), template.ID, user.ID, true,
		store.TemplateInput{Name: template.Name, Description: template.Description, Scope: scope})
	if err != nil {
		s.handleStoreError(writer, request, err, "template_update_failed")
		return
	}
	s.store.Audit(request.Context(), &user.ID, "template.scope", "template", template.ID,
		map[string]any{"scope": scope, "ownerId": template.OwnerID, "name": template.Name})
	writeData(writer, request, http.StatusOK, updated)
}
