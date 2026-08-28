package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/semaphore"
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
	Store    *store.Store
	Settings *settings.Service
	Keys     *keys.Manager
	Worker   *generation.Worker
	// Generator answers the editor directly, for work a person waits on: another
	// draft of one slide. Deck generation stays on the worker's queue.
	Generator     *generation.Generator
	Authenticator auth.Authenticator
	AuthPublic    AuthPublicConfig
	// Version is the build the workspace is running, shown in the account menu so
	// a bug report and a release can be matched up.
	Version string
	// AssetDir is where images are kept when they are not in the database. The
	// administrator's storage screen reads how much room that filesystem has:
	// a deployment with a volume has two places to run out of, and the database
	// is only one of them.
	AssetDir               string
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
	store         *store.Store
	settings      *settings.Service
	keys          *keys.Manager
	worker        *generation.Worker
	generator     *generation.Generator
	authenticator auth.Authenticator
	authPublic    AuthPublicConfig
	version       string
	assetDir      string
	// The last reading of the model host, so a dashboard being refreshed does
	// not knock on somebody's host once per refresh.
	providerCheckMu sync.Mutex
	providerCheck   generation.ProviderCheck
	providerCheckAt time.Time
	// building is the memory budget for making a document, in units of one PDF.
	// A PDF of a forty-slide deck with a photograph on every page costs about
	// 150 MiB to draw; the same deck packaged as .pptx costs 362 MiB, because
	// the package is assembled whole with every picture in it. Counting
	// documents would be the wrong bound: three PDFs fit where three .pptx
	// files killed the pod outright.
	// Eight forty-slide decks carrying a photograph on every page, printed
	// together, killed a pod held to the manifest's limit, and everyone waiting
	// on anything else died with it.
	building *semaphore.Weighted
	// templateReadsTaken counts slots taken, so a test can prove the handler
	// goes through the gate rather than only that the gate works when called
	// directly.
	templateReadsTaken     atomic.Int64
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

// concurrentTemplateReads is how many uploaded templates may be held in memory
// and read at the same time.
//
// One of the largest the settings allow — 64 MB — peaks at 441 MiB inside the
// pod the manifest describes. Two of them fit in a process that has just
// started and do not fit in one that has been working, because what the first
// read allocated has not gone back to the operating system yet: eight uploads
// at once killed a pod that was letting two through. Reading is quick, so the
// queue costs a couple of seconds each and the process stays up.

// documentBudget is how much document-building may happen at once, and what
// each kind costs against it. Measured in a pod held to the manifest's limit:
// one .pptx of a photograph-heavy forty-slide deck peaks at 362 MiB and two at
// 809 MiB, so three is where the pod dies; a PDF of the same deck is a third of
// that, and sixteen of them queue happily three at a time.
const (
	// heavyBudget is measured in units of about 150 MiB, which is what drawing
	// one PDF of a forty-slide deck with a photograph on every page costs. Three
	// of them together fit the pod the manifest describes.
	heavyBudget = 3
	// What each kind of work costs against it, from the peaks measured in a pod
	// held to that limit: a PDF 150 MiB, an import of a 60 MB deck 220 MiB, the
	// same deck packaged as .pptx 362 MiB, reading a 60 MB template 441 MiB.
	costOfPDF          = 1
	costOfImport       = 2
	costOfPPTX         = 3
	costOfTemplateRead = 3
)

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
		generator:     options.Generator,
		authenticator: options.Authenticator, authPublic: options.AuthPublic, adminRoles: options.AdminRoles,
		assetDir:             options.AssetDir,
		building:             semaphore.NewWeighted(heavyBudget),
		version:              strings.TrimSpace(options.Version),
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
	// A shared deck is read by whoever holds the link, which is the whole point
	// of the link: no session, no account, and nothing but the slides.
	root.HandleFunc("GET /api/v1/shared/{token}", s.sharedPresentation)
	root.HandleFunc("GET /api/v1/shared/{token}/preview.svg", s.sharedPreview)
	// Looking is half of a review. Whoever holds the link can say what is wrong
	// with slide 4, and read what others said.
	root.HandleFunc("GET /api/v1/shared/{token}/comments", s.sharedComments)
	root.HandleFunc("POST /api/v1/shared/{token}/comments", s.addSharedComment)

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
	api.Handle("GET /api/v1/presentations/summary", requireScope("presentations:read", http.HandlerFunc(s.workspaceSummary)))
	api.Handle("GET /api/v1/presentations", requireScope("presentations:read", http.HandlerFunc(s.listPresentations)))
	api.Handle("POST /api/v1/presentations", requireScope("presentations:write", http.HandlerFunc(s.createPresentation)))
	api.Handle("POST /api/v1/presentations/generate", requireScope("presentations:write", http.HandlerFunc(s.createAndGeneratePresentation)))
	api.Handle("GET /api/v1/presentations/{id}", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.getPresentation))))
	api.Handle("PUT /api/v1/presentations/{id}", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.updatePresentation))))
	api.Handle("PATCH /api/v1/presentations/{id}", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.updatePresentation))))
	api.Handle("DELETE /api/v1/presentations/{id}", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.deletePresentation))))
	api.Handle("POST /api/v1/presentations/{id}/duplicate", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.duplicatePresentation))))
	api.Handle("POST /api/v1/presentations/{id}/restore", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.restoreDeletedPresentation))))
	api.Handle("DELETE /api/v1/presentations/{id}/permanent", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.permanentlyDeletePresentation))))
	// Clearing the recycle bin one deck at a time is no way to clear thousands.
	api.Handle("DELETE /api/v1/presentations/trash", requireScope("presentations:write", http.HandlerFunc(s.emptyTrash)))
	api.Handle("GET /api/v1/presentations/{id}/revisions", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.listPresentationRevisions))))
	api.Handle("GET /api/v1/presentations/{id}/revisions/{revisionId}/changes", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.comparePresentationRevision))))
	api.Handle("POST /api/v1/presentations/{id}/revisions/{revisionId}/restore", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.restorePresentationRevision))))
	api.Handle("POST /api/v1/presentations/{id}/generate", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.generatePresentation))))
	// A deck is editable as text: the source compiles to exactly the slides that
	// are stored, so the two are the same deck in two forms.
	api.Handle("GET /api/v1/presentations/{id}/source", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.getPresentationSource))))
	api.Handle("PUT /api/v1/presentations/{id}/source", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.putPresentationSource))))
	api.Handle("POST /api/v1/presentations/{id}/source", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.putPresentationSource))))
	// Rendering source that has not been saved is how the code editor shows a
	// slide as it is typed.
	api.Handle("POST /api/v1/presentations/{id}/source/preview.svg", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.previewSource))))
	// One slide's regions, as objects the canvas can select, move and retype.
	// This is what makes a generated deck editable rather than a picture.
	api.Handle("GET /api/v1/presentations/{id}/slides/{position}/regions", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.slideRegions))))
	// Another draft of one slide, proposed and not saved.
	api.Handle("POST /api/v1/presentations/{id}/slides/{position}/revise", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.reviseSlide))))
	api.Handle("GET /api/v1/presentations/{id}/inspect", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.inspectPresentation))))
	api.Handle("GET /api/v1/presentations/{id}/export", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.exportPresentation))))
	api.Handle("GET /api/v1/presentations/{id}/export.pptx", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.exportPresentation))))
	api.Handle("GET /api/v1/presentations/{id}/export.pdf", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.exportPresentation))))
	api.Handle("GET /api/v1/presentations/{id}/preview.svg", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.presentationPreview))))
	// Links that open this deck for someone without an account.
	api.Handle("POST /api/v1/presentations/{id}/shares", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.createShare))))
	api.Handle("GET /api/v1/presentations/{id}/shares", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.listShares))))
	api.Handle("DELETE /api/v1/presentations/{id}/shares/{shareId}", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.revokeShare))))
	// What the people who read it said, and what the author did about it.
	api.Handle("GET /api/v1/presentations/{id}/comments", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.listComments))))
	// The other half of a review: the author answering what a reviewer said.
	api.Handle("POST /api/v1/presentations/{id}/comments", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.addOwnerComment))))
	api.Handle("POST /api/v1/presentations/{id}/comments/{commentId}/resolve", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.resolveComment))))
	api.Handle("DELETE /api/v1/presentations/{id}/comments/{commentId}", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.deleteComment))))
	// Grid components an organisation defined for itself.
	api.Handle("GET /api/v1/grids", requireScope("presentations:read", http.HandlerFunc(s.listGrids)))
	api.Handle("POST /api/v1/grids", requireScope("presentations:write", http.HandlerFunc(s.saveGrid)))
	api.Handle("PUT /api/v1/grids/{name}", requireScope("presentations:write", http.HandlerFunc(s.saveGrid)))
	api.Handle("DELETE /api/v1/grids/{name}", requireScope("presentations:write", http.HandlerFunc(s.deleteGrid)))
	// Images a deck places on its slides.
	api.Handle("GET /api/v1/assets", requireScope("presentations:read", http.HandlerFunc(s.listAssets)))
	api.Handle("POST /api/v1/assets", requireScope("presentations:write", http.HandlerFunc(s.createAsset)))
	api.Handle("GET /api/v1/assets/tags", requireScope("presentations:read", http.HandlerFunc(s.listAssetTags)))
	api.Handle("GET /api/v1/assets/{id}", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.getAsset))))
	api.Handle("PATCH /api/v1/assets/{id}", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.patchAsset))))
	api.Handle("PUT /api/v1/assets/{id}/favorite", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.favoriteAsset))))
	api.Handle("DELETE /api/v1/assets/{id}", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.deleteAsset))))
	// Slides someone keeps and drops into other decks.
	// A deck someone already has, read in as text and recompiled into a template.
	api.Handle("POST /api/v1/presentations/import", requireScope("presentations:write", http.HandlerFunc(s.importPresentation)))
	// The same queue as generation: a deck that already has text is rewritten
	// rather than written.
	api.Handle("POST /api/v1/presentations/{id}/command", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.runPresentationCommand))))
	api.Handle("POST /api/v1/presentations/{id}/rewrite", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.rewritePresentation))))
	api.Handle("GET /api/v1/snippets", requireScope("presentations:read", http.HandlerFunc(s.listSnippets)))
	api.Handle("POST /api/v1/snippets", requireScope("presentations:write", http.HandlerFunc(s.createSnippet)))
	api.Handle("GET /api/v1/snippets/tags", requireScope("presentations:read", http.HandlerFunc(s.listSnippetTags)))
	api.Handle("GET /api/v1/snippets/{id}", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.getSnippet))))
	api.Handle("PATCH /api/v1/snippets/{id}", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.patchSnippet))))
	api.Handle("DELETE /api/v1/snippets/{id}", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.deleteSnippet))))
	api.Handle("PUT /api/v1/snippets/{id}/favorite", requireUUIDPath(requireScope("presentations:write", http.HandlerFunc(s.favoriteSnippet))))
	api.Handle("POST /api/v1/snippets/{id}/render", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.renderSnippet))))
	api.Handle("GET /api/v1/snippets/{id}/preview.svg", requireUUIDPath(requireScope("presentations:read", http.HandlerFunc(s.snippetPreview))))
	api.Handle("GET /api/v1/templates", requireScope("templates:read", http.HandlerFunc(s.listTemplates)))
	api.Handle("POST /api/v1/templates", requireScope("templates:write", http.HandlerFunc(s.createTemplate)))
	api.Handle("GET /api/v1/templates/{id}", requireUUIDPath(requireScope("templates:read", http.HandlerFunc(s.getTemplate))))
	api.Handle("PATCH /api/v1/templates/{id}", requireUUIDPath(requireScope("templates:write", http.HandlerFunc(s.patchTemplate))))
	api.Handle("DELETE /api/v1/templates/{id}", requireUUIDPath(requireScope("templates:write", http.HandlerFunc(s.deleteTemplate))))
	// Pinning a design is a note about one's own workspace, not a change to a
	// template someone else owns, so it needs no write access to the template.
	api.Handle("PUT /api/v1/templates/{id}/favorite", requireUUIDPath(requireScope("templates:read", http.HandlerFunc(s.favoriteTemplate))))
	api.Handle("GET /api/v1/templates/{id}/download", requireUUIDPath(requireScope("templates:read", http.HandlerFunc(s.downloadTemplate))))
	// What this template will do to a deck, before somebody puts forty through it.
	api.Handle("GET /api/v1/templates/{id}/health",
		requireUUIDPath(requireScope("templates:read", http.HandlerFunc(s.templateHealth))))
	api.Handle("GET /api/v1/templates/{id}/preview.svg", requireUUIDPath(requireScope("templates:read", http.HandlerFunc(s.templateLayoutPreview))))
	api.Handle("GET /api/v1/templates/{id}/layouts/{layoutId}/preview.svg", requireUUIDPath(requireScope("templates:read", http.HandlerFunc(s.templateLayoutPreview))))
	api.Handle("GET /api/v1/api-keys", requireScope("api_keys:manage", http.HandlerFunc(s.listAPIKeys)))
	// What a key may do, from the one list the server validates against.
	api.Handle("GET /api/v1/api-keys/scopes", requireScope("api_keys:manage", http.HandlerFunc(s.apiKeyScopes)))
	api.Handle("POST /api/v1/api-keys", requireScope("api_keys:manage", http.HandlerFunc(s.createAPIKey)))
	// A key is written into a machine's configuration and forgotten. Changing
	// what it may do should not mean issuing another one and going back there.
	api.Handle("PATCH /api/v1/api-keys/{id}", requireUUIDPath(requireScope("api_keys:manage", http.HandlerFunc(s.updateAPIKey))))
	api.Handle("POST /api/v1/api-keys/{id}/revoke", requireUUIDPath(requireScope("api_keys:manage", http.HandlerFunc(s.revokeAPIKey))))
	api.Handle("DELETE /api/v1/api-keys/{id}", requireUUIDPath(requireScope("api_keys:manage", http.HandlerFunc(s.revokeAPIKey))))
	api.Handle("POST /api/v1/api-keys/{id}/rotate", requireUUIDPath(requireScope("api_keys:manage", http.HandlerFunc(s.rotateAPIKey))))

	api.Handle("GET /api/v1/admin/settings", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminListSettings)))
	api.Handle("PUT /api/v1/admin/settings", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminPutSettings)))
	api.Handle("PATCH /api/v1/admin/settings", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminPutSettings)))
	// The settings' own trail, and putting one back. Registered before the
	// {key} route so "changes" is read as itself rather than as a setting named
	// changes.
	// One page a site with no internet can hand to somebody who cannot see it.
	api.Handle("GET /api/v1/admin/report", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminReport)))
	// What has accumulated and is going nowhere. Reads only.
	api.Handle("GET /api/v1/admin/tidy", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminTidyPreview)))
	// The designs this deployment writes decks in.
	api.Handle("GET /api/v1/admin/templates", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminListTemplates)))
	api.Handle("POST /api/v1/admin/templates/{id}/standard", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminSetStandardTemplate)))
	api.Handle("POST /api/v1/admin/templates/{id}/shared", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminShareTemplate)))
	// What this deployment has been doing, day by day.
	api.Handle("GET /api/v1/admin/usage", s.requireAdmin("admin:users", http.HandlerFunc(s.adminUsage)))
	// Every link this deployment has handed out, and closing one.
	api.Handle("GET /api/v1/admin/shares", s.requireAdmin("admin:users", http.HandlerFunc(s.adminListShares)))
	api.Handle("POST /api/v1/admin/shares/{id}/close", s.requireAdmin("admin:users", http.HandlerFunc(s.adminCloseShare)))
	api.Handle("GET /api/v1/admin/settings/changes", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminSettingChanges)))
	api.Handle("POST /api/v1/admin/settings/changes/{id}/revert", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminRevertSettingChange)))
	api.Handle("PUT /api/v1/admin/settings/{key}", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminPutSetting)))
	api.Handle("GET /api/v1/admin/users/counts", s.requireAdmin("admin:users", http.HandlerFunc(s.adminUserCounts)))
	api.Handle("GET /api/v1/admin/users", s.requireAdmin("admin:users", http.HandlerFunc(s.adminListUsers)))
	api.Handle("PATCH /api/v1/admin/users/{id}", requireUUIDPath(s.requireAdmin("admin:users", http.HandlerFunc(s.adminUpdateUser))))
	api.Handle("GET /api/v1/admin/errors", s.requireAdmin("admin:errors", http.HandlerFunc(s.adminListErrors)))
	api.Handle("PATCH /api/v1/admin/errors/{id}", requireUUIDPath(s.requireAdmin("admin:errors", http.HandlerFunc(s.adminUpdateError))))
	// Whether the model host this deployment points at answers. Asking is a
	// POST and reading is a GET, and they are not the same thing.
	api.Handle("POST /api/v1/admin/provider-check", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminCheckProvider)))
	api.Handle("GET /api/v1/admin/provider-check", s.requireAdmin("admin:settings", http.HandlerFunc(s.adminCheckProvider)))
	// What the deployment is keeping, and how much room is left for it.
	api.Handle("GET /api/v1/admin/storage", s.requireAdmin("admin:users", http.HandlerFunc(s.adminStorage)))
	// What is waiting, and the two things an operator can do about it.
	api.Handle("GET /api/v1/admin/generations", s.requireAdmin("admin:users", http.HandlerFunc(s.adminGenerationQueue)))
	api.Handle("POST /api/v1/admin/generations/{id}/requeue",
		requireUUIDPath(s.requireAdmin("admin:users", http.HandlerFunc(s.adminRequeueGeneration))))
	api.Handle("POST /api/v1/admin/generations/{id}/cancel",
		requireUUIDPath(s.requireAdmin("admin:users", http.HandlerFunc(s.adminCancelGeneration))))
	// The trail is written by everything and was readable by nothing.
	api.Handle("GET /api/v1/admin/audit", s.requireAdmin("admin:users", http.HandlerFunc(s.adminListAuditTrail)))
	api.Handle("GET /api/v1/admin/audit/actions", s.requireAdmin("admin:users", http.HandlerFunc(s.adminAuditActions)))
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
	writeListMeta(writer, request, data, map[string]any{"total": total, "limit": limit, "offset": offset})
}

// writeListMeta is writeList for a list whose size is more than one number:
// the generation queue is what is waiting and what failed, and both are larger
// than the rows it hands over.
func writeListMeta(writer http.ResponseWriter, request *http.Request, data any, meta map[string]any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(writer).Encode(map[string]any{"data": data, "meta": meta, "requestId": RequestID(request.Context())})
}

// configurationRefusals are the codes that describe how a deployment is set up
// rather than something going wrong in it. The answer is correct, the caller is
// told what to do, and an operator reading the error centre needs to see the
// faults instead — not the same configuration repeated once per deck.
var configurationRefusals = map[string]bool{"ai_unavailable": true}

func writeError(writer http.ResponseWriter, request *http.Request, status int, code, message string, details any) {
	if recorder, ok := writer.(*responseRecorder); ok && configurationRefusals[code] {
		recorder.refusal = true
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	errorValue := map[string]any{"code": code, "message": message}
	if details != nil {
		errorValue["details"] = details
	}
	_ = json.NewEncoder(writer).Encode(map[string]any{"error": errorValue, "requestId": RequestID(request.Context())})
}
