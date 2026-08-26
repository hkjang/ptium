package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/hkjang/ptium/server/internal/keys"
	"github.com/hkjang/ptium/server/internal/store"
)

type createAPIKeyRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

func (s *Server) listAPIKeys(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	items, err := s.keys.List(request.Context(), user.ID)
	if err != nil {
		s.internalError(writer, request, "api_keys_read_failed", err)
		return
	}
	writeData(writer, request, http.StatusOK, items)
}

func (s *Server) createAPIKey(writer http.ResponseWriter, request *http.Request) {
	var input createAPIKeyRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	user, _ := UserFromContext(request.Context())
	created, err := s.keys.Create(request.Context(), user.ID, strings.TrimSpace(input.Name), input.Scopes, input.ExpiresAt, user.IsAdmin)
	if err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
		return
	}
	s.store.Audit(request.Context(), &user.ID, "api_key.create", "api_key", created.APIKey.ID, map[string]any{"scopes": created.APIKey.Scopes})
	writeData(writer, request, http.StatusCreated, created)
}

// apiKeyScopes is what this deployment may put on a key, so the screen that
// grants them does not keep its own list and drift from the server's.
func (s *Server) apiKeyScopes(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	writeData(writer, request, http.StatusOK, keys.Scopes(user.IsAdmin))
}

type updateAPIKeyRequest struct {
	Scopes []string `json:"scopes"`
}

// updateAPIKey changes what a key may do without changing the key.
func (s *Server) updateAPIKey(writer http.ResponseWriter, request *http.Request) {
	var input updateAPIKeyRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	user, _ := UserFromContext(request.Context())
	updated, err := s.keys.SetScopes(request.Context(), user.ID, request.PathValue("id"), input.Scopes, user.IsAdmin)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(writer, request, http.StatusNotFound, "not_found",
				"This API key does not exist, or it has been revoked", nil)
			return
		}
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
		return
	}
	s.store.Audit(request.Context(), &user.ID, "api_key.scopes", "api_key", updated.ID,
		map[string]any{"scopes": updated.Scopes})
	writeData(writer, request, http.StatusOK, updated)
}

func (s *Server) revokeAPIKey(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	if err := s.keys.Revoke(request.Context(), user.ID, request.PathValue("id"), false); err != nil {
		s.handleStoreError(writer, request, err, "api_key_revoke_failed")
		return
	}
	s.store.Audit(request.Context(), &user.ID, "api_key.revoke", "api_key", request.PathValue("id"), nil)
	writer.WriteHeader(http.StatusNoContent)
}

type rotateAPIKeyRequest struct {
	GracePeriod  string `json:"gracePeriod"`
	GraceSeconds *int64 `json:"graceSeconds"`
}

func (s *Server) rotateAPIKey(writer http.ResponseWriter, request *http.Request) {
	var input rotateAPIKeyRequest
	if request.ContentLength != 0 && !decodeJSON(writer, request, &input) {
		return
	}
	grace := 24 * time.Hour
	var configuredGrace string
	if s.settings.Get(request.Context(), "security.api_key_grace", &configuredGrace) == nil && configuredGrace != "" {
		if parsed, err := time.ParseDuration(configuredGrace); err == nil {
			grace = parsed
		}
	}
	if input.GracePeriod != "" {
		parsed, err := time.ParseDuration(input.GracePeriod)
		if err != nil {
			writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "gracePeriod must be a duration such as 24h", nil)
			return
		}
		grace = parsed
	}
	if input.GraceSeconds != nil {
		grace = time.Duration(*input.GraceSeconds) * time.Second
	}
	user, _ := UserFromContext(request.Context())
	created, err := s.keys.Rotate(request.Context(), user.ID, request.PathValue("id"), false, grace)
	if err != nil {
		if strings.Contains(err.Error(), "grace") {
			writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
			return
		}
		s.handleStoreError(writer, request, err, "api_key_rotate_failed")
		return
	}
	s.store.Audit(request.Context(), &user.ID, "api_key.rotate", "api_key", request.PathValue("id"), map[string]any{"replacementId": created.APIKey.ID, "grace": grace.String()})
	writeData(writer, request, http.StatusCreated, created)
}
