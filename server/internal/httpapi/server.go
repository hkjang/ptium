package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/ptium/server/internal/auth"
	"github.com/hkjang/ptium/server/internal/generation"
	"github.com/hkjang/ptium/server/internal/keys"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/settings"
	"github.com/hkjang/ptium/server/internal/store"
)

type AuthPublicConfig struct {
	OIDCEnabled           bool   `json:"oidcEnabled"`
	Issuer                string `json:"issuer,omitempty"`
	ClientID              string `json:"clientId,omitempty"`
	AuthorizationEndpoint string `json:"authorizationEndpoint,omitempty"`
	TokenEndpoint         string `json:"tokenEndpoint,omitempty"`
	EndSessionEndpoint    string `json:"endSessionEndpoint,omitempty"`
	Scopes                string `json:"scopes"`
	PKCERequired          bool   `json:"pkceRequired"`
	DevAuthEnabled        bool   `json:"devAuthEnabled"`
	DevAuthHeader         string `json:"devAuthHeader,omitempty"`
}

type Options struct {
	Store                  *store.Store
	Settings               *settings.Service
	Keys                   *keys.Manager
	Worker                 *generation.Worker
	Authenticator          auth.Authenticator
	AuthPublic             AuthPublicConfig
	AdminRoles             []string
	BootstrapAdminEmails   []string
	BootstrapAdminSubjects []string
	CORSAllowedOrigins     []string
	Logger                 *slog.Logger
	MCPHandler             http.Handler
}

type Server struct {
	store                  *store.Store
	settings               *settings.Service
	keys                   *keys.Manager
	worker                 *generation.Worker
	authenticator          auth.Authenticator
	authPublic             AuthPublicConfig
	adminRoles             []string
	bootstrapAdminEmails   []string
	bootstrapAdminSubjects []string
	corsOrigins            []string
	logger                 *slog.Logger
	mcpHandler             http.Handler
	captureIncident        func(context.Context, model.Incident) error
}

func New(options Options) (*Server, error) {
	if options.Store == nil || options.Settings == nil || options.Keys == nil || options.Worker == nil {
		return nil, errors.New("http API dependencies are incomplete")
	}
	if options.Authenticator == nil {
		return nil, errors.New("http API authenticator is required")
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if len(options.AdminRoles) == 0 {
		options.AdminRoles = []string{"admin", "ptium-admin"}
	}
	if options.AuthPublic.Scopes == "" {
		options.AuthPublic.Scopes = "openid profile email"
	}
	options.AuthPublic.PKCERequired = options.AuthPublic.OIDCEnabled
	return &Server{
		store: options.Store, settings: options.Settings, keys: options.Keys, worker: options.Worker,
		authenticator: options.Authenticator, authPublic: options.AuthPublic, adminRoles: options.AdminRoles,
		bootstrapAdminEmails: options.BootstrapAdminEmails, bootstrapAdminSubjects: options.BootstrapAdminSubjects,
		corsOrigins: options.CORSAllowedOrigins, logger: options.Logger, mcpHandler: options.MCPHandler,
		captureIncident: options.Store.CaptureIncident,
	}, nil
}

func (s *Server) Handler() http.Handler {
	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", s.health)
	root.HandleFunc("GET /readyz", s.ready)
	root.HandleFunc("GET /api/v1/auth/config", s.authConfig)
	root.HandleFunc("GET /api/v1/settings", s.publicSettings)

	api := http.NewServeMux()
	api.HandleFunc("GET /api/v1/me", s.me)
	api.HandleFunc("GET /auth/me", s.me) // compatibility for early clients
	api.Handle("GET /api/v1/profile", requireScope("profile:read", http.HandlerFunc(s.getProfile)))
	api.Handle("PUT /api/v1/profile", requireScope("profile:write", http.HandlerFunc(s.putProfile)))
	api.Handle("PATCH /api/v1/profile", requireScope("profile:write", http.HandlerFunc(s.putProfile)))
	api.Handle("GET /api/v1/presentations", requireScope("presentations:read", http.HandlerFunc(s.listPresentations)))
	api.Handle("POST /api/v1/presentations", requireScope("presentations:write", http.HandlerFunc(s.createPresentation)))
	api.Handle("POST /api/v1/presentations/generate", requireScope("presentations:write", http.HandlerFunc(s.createAndGeneratePresentation)))
	api.Handle("GET /api/v1/presentations/{id}", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.getPresentation))))
	api.Handle("PUT /api/v1/presentations/{id}", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.updatePresentation))))
	api.Handle("PATCH /api/v1/presentations/{id}", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.updatePresentation))))
	api.Handle("DELETE /api/v1/presentations/{id}", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.deletePresentation))))
	api.Handle("POST /api/v1/presentations/{id}/generate", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.generatePresentation))))
	api.Handle("GET /api/v1/presentations/{id}/export", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.exportPresentation))))
	api.Handle("GET /api/v1/presentations/{id}/export.pptx", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.exportPresentation))))
	api.Handle("GET /api/v1/presentations/{id}/preview.svg", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.presentationPreview))))
	api.Handle("GET /api/v1/templates", requireScope("templates:read", http.HandlerFunc(s.listTemplates)))
	api.Handle("POST /api/v1/templates", requireScope("templates:write", http.HandlerFunc(s.createTemplate)))
	api.Handle("GET /api/v1/templates/{id}", requireUUIDPath(requireScope("templates:read", http.HandlerFunc(s.getTemplate))))
	api.Handle("PATCH /api/v1/templates/{id}", requireUUIDPath(requireScope("templates:write", http.HandlerFunc(s.patchTemplate))))
	api.Handle("DELETE /api/v1/templates/{id}", requireUUIDPath(requireScope("templates:write", http.HandlerFunc(s.deleteTemplate))))
	api.Handle("GET /api/v1/templates/{id}/download", requireUUIDPath(requireScope("templates:read", http.HandlerFunc(s.downloadTemplate))))
	api.Handle("GET /api/v1/templates/{id}/preview.svg", requireUUIDPath(requireScope("templates:read", http.HandlerFunc(s.templateLayoutPreview))))
	api.Handle("GET /api/v1/templates/{id}/layouts/{layoutId}/preview.svg", requireUUIDPath(requireScope("templates:read", http.HandlerFunc(s.templateLayoutPreview))))
	api.Handle("GET /api/v1/api-keys", requireScope("api_keys:manage", http.HandlerFunc(s.listAPIKeys)))
	api.Handle("POST /api/v1/api-keys", requireScope("api_keys:manage", http.HandlerFunc(s.createAPIKey)))
	api.Handle("POST /api/v1/api-keys/{id}/revoke", requireUUIDPath(requireScope("api_keys:manage", http.HandlerFunc(s.revokeAPIKey))))
	api.Handle("DELETE /api/v1/api-keys/{id}", requireUUIDPath(requireScope("api_keys:manage", http.HandlerFunc(s.revokeAPIKey))))
	api.Handle("POST /api/v1/api-keys/{id}/rotate", requireUUIDPath(requireScope("api_keys:manage", http.HandlerFunc(s.rotateAPIKey))))

	api.Handle("GET /api/v1/admin/settings", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminListSettings)))
	api.Handle("PUT /api/v1/admin/settings", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminPutSettings)))
	api.Handle("PATCH /api/v1/admin/settings", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminPutSettings)))
	api.Handle("PUT /api/v1/admin/settings/{key}", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminPutSetting)))
	api.Handle("GET /api/v1/admin/users", s.requireAdmin("admin:users", http.HandlerFunc(s.adminListUsers)))
	api.Handle("PATCH /api/v1/admin/users/{id}", requireUUIDPath(s.requireAdmin("admin:users", http.HandlerFunc(s.adminUpdateUser))))
	api.Handle("GET /api/v1/admin/errors", s.requireAdmin("admin:errors", http.HandlerFunc(s.adminListErrors)))
	api.Handle("PATCH /api/v1/admin/errors/{id}", requireUUIDPath(s.requireAdmin("admin:errors", http.HandlerFunc(s.adminUpdateError))))
	api.Handle("GET /api/v1/admin/overview", s.requireAdmin("admin:users", http.HandlerFunc(s.adminOverview)))

	protected := auth.AuthenticationMiddleware(s.authenticator, auth.MiddlewareOptions{
		Realm: "ptium",
		OnError: func(ctx context.Context, err error) {
			s.logger.Warn("authentication failed", "request_id", RequestID(ctx), "error", err)
		},
		WriteError: func(writer http.ResponseWriter, request *http.Request, status int, code string) {
			writeError(writer, request, status, code, http.StatusText(status), nil)
		},
	})(s.identityMiddleware(api))
	root.Handle("/api/v1/", protected)
	root.Handle("/auth/me", protected)
	if s.mcpHandler != nil {
		mcpProtected := auth.AuthenticationMiddleware(s.authenticator, auth.MiddlewareOptions{
			Realm: "ptium-mcp",
			WriteError: func(writer http.ResponseWriter, request *http.Request, status int, code string) {
				writeError(writer, request, status, code, http.StatusText(status), nil)
			},
		})(s.identityMiddleware(requireScope("mcp:use", s.mcpHandler)))
		root.Handle("/mcp", mcpProtected)
	}
	return s.requestMiddleware(s.corsMiddleware(root))
}

func requireUUIDPath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if _, err := uuid.Parse(request.PathValue("id")); err != nil {
			writeError(writer, request, http.StatusBadRequest, "invalid_id", "Path id must be a UUID", nil)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) health(writer http.ResponseWriter, request *http.Request) {
	writeData(writer, request, http.StatusOK, map[string]any{"status": "ok", "service": "ptium", "time": time.Now().UTC()})
}

func (s *Server) ready(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeError(writer, request, http.StatusServiceUnavailable, "database_unavailable", "Database is not ready", nil)
		return
	}
	writeData(writer, request, http.StatusOK, map[string]any{"status": "ready"})
}

func (s *Server) authConfig(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writeData(writer, request, http.StatusOK, s.authPublic)
}

func writeData(writer http.ResponseWriter, request *http.Request, status int, data any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"data": data, "requestId": RequestID(request.Context())})
}

func writeList(writer http.ResponseWriter, request *http.Request, data any, total, limit, offset int) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(writer).Encode(map[string]any{"data": data, "meta": map[string]any{"total": total, "limit": limit, "offset": offset}, "requestId": RequestID(request.Context())})
}

func writeError(writer http.ResponseWriter, request *http.Request, status int, code, message string, details any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	errorValue := map[string]any{"code": code, "message": message}
	if details != nil {
		errorValue["details"] = details
	}
	_ = json.NewEncoder(writer).Encode(map[string]any{"error": errorValue, "requestId": RequestID(request.Context())})
}
