package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
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
	// PasswordLoginEnabled tells the workspace to offer a username and password
	// form, which is how a bootstrap administrator signs in.
	PasswordLoginEnabled bool `json:"passwordLoginEnabled"`
	// TokenExchangeURL is set when Ptium exchanges the authorization code on the
	// browser's behalf, which a confidential OIDC client requires.
	TokenExchangeURL string `json:"tokenExchangeUrl,omitempty"`
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
	// WebHandler serves the compiled workspace when the process also hosts it.
	WebHandler http.Handler
	// Sessions issues browser session tokens for password sign-in. Nil disables
	// password sign-in entirely.
	Sessions *auth.SessionIssuer
	// PasswordLoginEnabled tells the workspace whether to offer the form. A
	// deployment with no local account should not show one.
	PasswordLoginEnabled bool
	// TokenExchange performs the OIDC code exchange server-side. Nil leaves the
	// browser talking to the identity provider directly.
	TokenExchange *TokenExchange
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
	webHandler             http.Handler
	sessions               *auth.SessionIssuer
	tokenExchange          *TokenExchange
	loginLimiter           *loginLimiter
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
	options.AuthPublic.PasswordLoginEnabled = options.Sessions != nil && options.PasswordLoginEnabled
	if options.TokenExchange != nil && strings.TrimSpace(options.TokenExchange.Endpoint) != "" {
		options.AuthPublic.TokenExchangeURL = "/api/v1/auth/token"
	}
	return &Server{
		store: options.Store, settings: options.Settings, keys: options.Keys, worker: options.Worker,
		authenticator: options.Authenticator, authPublic: options.AuthPublic, adminRoles: options.AdminRoles,
		bootstrapAdminEmails: options.BootstrapAdminEmails, bootstrapAdminSubjects: options.BootstrapAdminSubjects,
		corsOrigins: options.CORSAllowedOrigins, logger: options.Logger, mcpHandler: options.MCPHandler,
		webHandler:      options.WebHandler,
		sessions:        options.Sessions,
		tokenExchange:   options.TokenExchange,
		loginLimiter:    newLoginLimiter(),
		captureIncident: options.Store.CaptureIncident,
	}, nil
}

func (s *Server) Handler() http.Handler {
	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", s.health)
	root.HandleFunc("GET /readyz", s.ready)
	root.HandleFunc("GET /api/v1/auth/config", s.authConfig)
	root.HandleFunc("GET /api/v1/settings", s.publicSettings)
	// Sign-in and the OIDC code exchange are reachable without credentials, by
	// definition; both apply their own throttling and validation.
	root.HandleFunc("POST /api/v1/auth/login", s.passwordLogin)
	root.HandleFunc("POST /api/v1/auth/token", s.exchangeToken)
	// Signing out clears the cookie whether or not the session still verifies.
	root.HandleFunc("POST /api/v1/auth/logout", s.signOut)

	api := http.NewServeMux()
	api.HandleFunc("GET /api/v1/me", s.me)
	api.HandleFunc("GET /auth/me", s.me) // compatibility for early clients
	api.HandleFunc("POST /api/v1/auth/password", s.changePassword)
	// Any interactive identity — the identity provider included — can trade itself
	// for a renewable session cookie.
	api.HandleFunc("POST /api/v1/auth/session", s.startSession)
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
	// A deck is editable as text: the source compiles to exactly the slides that
	// are stored, so the two are the same deck in two forms.
	api.Handle("GET /api/v1/presentations/{id}/source", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.getPresentationSource))))
	api.Handle("PUT /api/v1/presentations/{id}/source", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.putPresentationSource))))
	api.Handle("POST /api/v1/presentations/{id}/source", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.putPresentationSource))))
	// Rendering source that has not been saved is how the code editor shows a
	// slide as it is typed.
	api.Handle("POST /api/v1/presentations/{id}/source/preview.svg", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.previewSource))))
	api.Handle("GET /api/v1/presentations/{id}/inspect", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.inspectPresentation))))
	api.Handle("GET /api/v1/presentations/{id}/export", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.exportPresentation))))
	api.Handle("GET /api/v1/presentations/{id}/export.pptx", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.exportPresentation))))
	api.Handle("GET /api/v1/presentations/{id}/preview.svg", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.presentationPreview))))
	// Images a deck places on its slides.
	api.Handle("GET /api/v1/assets", requireScope("presentations:read", http.HandlerFunc(s.listAssets)))
	api.Handle("POST /api/v1/assets", requireScope("presentations:write", http.HandlerFunc(s.createAsset)))
	api.Handle("GET /api/v1/assets/{id}", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.getAsset))))
	api.Handle("DELETE /api/v1/assets/{id}", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.deleteAsset))))
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
	})(s.sessionRenewalMiddleware(s.identityMiddleware(api)))
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
	if s.webHandler != nil {
		// Registered last and least specific, so every API, health and MCP
		// route above still wins; anything else is the single-page workspace.
		root.Handle("/", s.webHandler)
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
