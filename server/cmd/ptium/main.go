package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hkjang/ptium/server/internal/auth"
	"github.com/hkjang/ptium/server/internal/config"
	"github.com/hkjang/ptium/server/internal/db"
	"github.com/hkjang/ptium/server/internal/generation"
	"github.com/hkjang/ptium/server/internal/httpapi"
	"github.com/hkjang/ptium/server/internal/keys"
	"github.com/hkjang/ptium/server/internal/mcp"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/settings"
	"github.com/hkjang/ptium/server/internal/store"
	"github.com/hkjang/ptium/server/internal/webui"
)

var version = "dev"

func main() {
	applicationConfig, err := config.Load()
	if err != nil {
		fatal("load application configuration", err)
	}
	logger := newLogger(applicationConfig.LogLevel)
	slog.SetDefault(logger)

	rootContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := db.Open(rootContext, applicationConfig.DatabaseURL, logger)
	if err != nil {
		fatal("initialize database", err)
	}
	defer pool.Close()
	dataStore := store.New(pool)
	// The shipped designs are rebuilt from code on every boot, so an offline
	// deployment always has a usable template without any network access.
	seedContext, cancelSeed := context.WithTimeout(rootContext, 60*time.Second)
	if err := dataStore.EnsureBuiltinTemplates(seedContext); err != nil {
		cancelSeed()
		fatal("seed built-in presentation templates", err)
	}
	cancelSeed()

	// A local administrator lets a deployment be administered before any identity
	// provider is wired up. The password is read from the environment only; it is
	// never stored in the settings table and never returned by the API.
	if applicationConfig.BootstrapAdmin != "" {
		if applicationConfig.BootstrapAdminPassword == "" {
			fatal("provision bootstrap administrator", errors.New("BOOTSTRAP_ADMIN requires BOOTSTRAP_ADMIN_PASSWORD"))
		}
		adminContext, cancelAdmin := context.WithTimeout(rootContext, 30*time.Second)
		admin, written, err := dataStore.EnsureLocalAdmin(adminContext,
			applicationConfig.BootstrapAdmin, applicationConfig.BootstrapAdminPassword,
			applicationConfig.BootstrapAdminName, applicationConfig.BootstrapAdminPasswordReset)
		cancelAdmin()
		if err != nil {
			fatal("provision bootstrap administrator", err)
		}
		logger.Info("bootstrap administrator ready", "username", applicationConfig.BootstrapAdmin,
			"user_id", admin.ID, "password_written", written)
		if !written {
			logger.Info("the bootstrap administrator already has a password; set BOOTSTRAP_ADMIN_PASSWORD_RESET=true to overwrite it")
		}
	}
	passwordLoginEnabled, err := dataStore.HasLocalAccounts(rootContext)
	if err != nil {
		fatal("check local accounts", err)
	}

	keyMaterial := applicationConfig.KeyEncryptionSecret
	if keyMaterial == "" {
		keyMaterial = applicationConfig.DatabaseURL
		logger.Warn("KEY_ENCRYPTION_SECRET is unset; sensitive settings are encrypted with a key derived from DATABASE_URL; set an explicit stable secret before rotating database credentials")
	}
	settingService, err := settings.New(dataStore, keyMaterial)
	if err != nil {
		fatal("initialize encrypted settings", err)
	}
	keyManager := keys.New(pool)

	applicationConfig.CORSAllowedOrigins = appendUnique(applicationConfig.CORSAllowedOrigins, databaseCORSOrigins(rootContext, settingService)...)
	authConfig, err := auth.LoadBootstrapConfig(databaseAuthSource(rootContext, settingService))
	if err != nil {
		fatal("load authentication configuration", err)
	}
	if authConfig.OIDC.Enabled && strings.TrimSpace(authConfig.OIDC.ClientID) == "" {
		fatal("load authentication configuration", errors.New("OIDC_CLIENT_ID (or auth.oidc.client_id) is required for the browser PKCE flow"))
	}
	sessionIssuer, err := auth.NewSessionIssuer(keyMaterial, applicationConfig.SessionLifetime)
	if err != nil {
		fatal("initialize session tokens", err)
	}
	authenticator, publicAuth, tokenExchange, err := buildAuthenticator(rootContext, authConfig, keyManager, dataStore, sessionIssuer, logger)
	if err != nil {
		fatal("initialize authentication", err)
	}
	if !authConfig.OIDC.Enabled && !authConfig.Dev.Enabled && !passwordLoginEnabled {
		logger.Warn("no interactive authentication is configured; set BOOTSTRAP_ADMIN and BOOTSTRAP_ADMIN_PASSWORD, or configure OIDC, before anyone can sign in")
	}

	generator := generation.New(settingService)
	worker := generation.NewWorker(dataStore, generator, logger, applicationConfig.WorkerPollInterval)
	workerContext, cancelWorker := context.WithCancel(rootContext)
	defer cancelWorker()
	go worker.Run(workerContext)

	operations := httpapi.MCPOperations{Store: dataStore, Settings: settingService, Worker: worker}
	mcpHandler, err := mcp.New(mcp.Config{
		Operations:      operations,
		UserFromRequest: httpapi.MCPUserFromRequest,
		ServerName:      "ptium",
		ServerVersion:   version,
		AllowedOrigins:  exactOrigins(applicationConfig.CORSAllowedOrigins),
		OnError: func(ctx context.Context, cause error) {
			details, _ := json.Marshal(map[string]any{"transport": "mcp"})
			incident := model.Incident{RequestID: httpapi.RequestID(ctx), Kind: "mcp", Severity: "error", Message: cause.Error(), Details: details}
			if user, ok := httpapi.UserFromContext(ctx); ok {
				incident.UserID = &user.ID
			}
			captureContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			defer cancel()
			if captureErr := dataStore.CaptureIncident(captureContext, incident); captureErr != nil {
				logger.Error("capture MCP incident failed", "error", captureErr)
			}
		},
	})
	if err != nil {
		fatal("initialize MCP endpoint", err)
	}

	// The compiled workspace is served by this process, so a deployment is one
	// container on one port with no reverse proxy in front of it.
	var webHandler http.Handler
	if applicationConfig.WebDir != "" {
		webHandler, err = webui.Handler(applicationConfig.WebDir)
		if err != nil {
			fatal("serve web workspace", err)
		}
		logger.Info("serving web workspace", "directory", applicationConfig.WebDir)
	}

	api, err := httpapi.New(httpapi.Options{
		Store: dataStore, Settings: settingService, Keys: keyManager, Worker: worker,
		Authenticator: authenticator, AuthPublic: publicAuth, AdminRoles: authConfig.AdminRoles,
		BootstrapAdminEmails: applicationConfig.BootstrapAdminEmails, BootstrapAdminSubjects: applicationConfig.BootstrapAdminSubjects,
		CORSAllowedOrigins: applicationConfig.CORSAllowedOrigins, Logger: logger, MCPHandler: mcpHandler,
		WebHandler: webHandler, Sessions: sessionIssuer, TokenExchange: tokenExchange,
		PasswordLoginEnabled: passwordLoginEnabled,
	})
	if err != nil {
		fatal("initialize HTTP API", err)
	}

	httpServer := &http.Server{
		Addr:              applicationConfig.HTTPAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("Ptium server listening", "address", applicationConfig.HTTPAddr, "version", version,
			"oidc_enabled", authConfig.OIDC.Enabled, "dev_auth_enabled", authConfig.Dev.Enabled)
		serverErrors <- httpServer.ListenAndServe()
	}()

	select {
	case <-rootContext.Done():
		logger.Info("shutdown requested")
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			fatal("HTTP server stopped", err)
		}
	}
	cancelWorker()
	shutdownContext, cancel := context.WithTimeout(context.Background(), applicationConfig.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownContext); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		_ = httpServer.Close()
	}
	logger.Info("Ptium server stopped")
}

func buildAuthenticator(ctx context.Context, config auth.BootstrapConfig, keyManager *keys.Manager,
	dataStore *store.Store, sessions *auth.SessionIssuer, logger *slog.Logger,
) (auth.Authenticator, httpapi.AuthPublicConfig, *httpapi.TokenExchange, error) {
	var authenticators []auth.Authenticator
	var exchange *httpapi.TokenExchange
	public := httpapi.AuthPublicConfig{Scopes: "openid profile email", DevAuthEnabled: config.Dev.Enabled, DevAuthHeader: config.Dev.Header}
	if config.Dev.Enabled {
		dev, err := auth.NewDevAuthenticator(config.Dev)
		if err != nil {
			return nil, public, nil, err
		}
		authenticators = append(authenticators, dev)
	}
	// Session tokens come before the identity provider and the API key: they are
	// prefixed, so recognising them costs nothing and no other authenticator
	// needs to guess at them.
	authenticators = append(authenticators, auth.SessionAuthenticator{
		Issuer: sessions,
		Resolver: auth.SessionResolverFunc(func(ctx context.Context, claims auth.SessionClaims) (*auth.Principal, error) {
			epoch, err := dataStore.SessionEpoch(ctx, claims.UserID)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					return nil, auth.ErrInvalidCredentials
				}
				return nil, err
			}
			// A password change moves the epoch forward and retires the token.
			if epoch != claims.Epoch {
				return nil, auth.ErrInvalidCredentials
			}
			return &auth.Principal{
				Subject:    "session:" + claims.UserID,
				AuthMethod: "password",
				Claims:     map[string]any{"ptium_user_id": claims.UserID},
			}, nil
		}),
	})
	if config.OIDC.Enabled {
		oidc, err := auth.NewOIDCAuthenticator(ctx, config.OIDC)
		if err != nil {
			return nil, public, nil, err
		}
		discovery := oidc.Discovery()
		public.OIDCEnabled = true
		public.Issuer = discovery.Issuer
		public.ClientID = config.OIDC.ClientID
		public.AuthorizationEndpoint = discovery.AuthorizationEndpoint
		public.TokenEndpoint = discovery.TokenEndpoint
		public.EndSessionEndpoint = discovery.EndSessionEndpoint
		authenticators = append(authenticators, oidc)
		// A confidential client must not hand its secret to the browser, so the
		// code exchange runs here instead. A public client keeps talking to the
		// provider directly.
		if strings.TrimSpace(config.OIDC.ClientSecret) != "" && discovery.TokenEndpoint != "" {
			exchange = &httpapi.TokenExchange{
				Endpoint:     discovery.TokenEndpoint,
				ClientID:     config.OIDC.ClientID,
				ClientSecret: config.OIDC.ClientSecret,
			}
			logger.Info("OIDC client secret configured; exchanging authorization codes server-side")
		}
	}
	apiKeyAuthenticator := auth.APIKeyAuthenticator{
		Verifier: auth.APIKeyVerifierFunc(func(ctx context.Context, token string) (*auth.Principal, error) {
			identity, err := keyManager.Authenticate(ctx, token)
			if err != nil {
				if errors.Is(err, keys.ErrInvalidKey) {
					return nil, auth.ErrInvalidCredentials
				}
				return nil, err
			}
			roles := append([]string(nil), identity.User.Roles...)
			if identity.User.IsAdmin && !contains(roles, "ptium-admin") {
				roles = append(roles, "ptium-admin")
			}
			return &auth.Principal{
				Subject: "api-key:" + identity.User.ID, Email: identity.User.Email, Name: identity.User.Name,
				Roles: roles, Scopes: identity.Scopes, AuthMethod: "api_key",
				Claims: map[string]any{"ptium_user_id": identity.User.ID, "ptium_api_key_id": identity.KeyID},
			}, nil
		}),
		AllowBearer: true,
	}
	authenticators = append(authenticators, apiKeyAuthenticator)
	return auth.CompositeAuthenticator{Authenticators: authenticators}, public, exchange, nil
}

func exactOrigins(origins []string) []string {
	result := make([]string, 0, len(origins))
	for _, origin := range origins {
		if strings.TrimSpace(origin) != "" && origin != "*" {
			result = append(result, origin)
		}
	}
	return result
}

type layeredAuthSource struct {
	environment auth.EnvironmentSource
	database    auth.MapSource
}

func (source layeredAuthSource) Lookup(key string) (string, bool) {
	if value, ok := source.environment.Lookup(key); ok && strings.TrimSpace(value) != "" {
		return value, true
	}
	return source.database.Lookup(key)
}

func databaseAuthSource(ctx context.Context, service *settings.Service) auth.ValueSource {
	values := auth.MapSource{}
	if !anyEnvironment("OIDC_ISSUER_URL", "PTIUM_OIDC_ISSUER_URL", "PTIUM_AUTH_OIDC_ISSUER", "KEYCLOAK_URL", "PTIUM_KEYCLOAK_URL") {
		var value string
		if service.Get(ctx, "auth.oidc.issuer_url", &value) == nil && value != "" {
			values["OIDC_ISSUER_URL"] = value
		}
	}
	if !anyEnvironment("OIDC_CLIENT_ID", "PTIUM_OIDC_CLIENT_ID", "PTIUM_AUTH_OIDC_CLIENT_ID") {
		var value string
		if service.Get(ctx, "auth.oidc.client_id", &value) == nil && value != "" {
			values["OIDC_CLIENT_ID"] = value
		}
	}
	if !anyEnvironment("OIDC_CLIENT_SECRET", "PTIUM_OIDC_CLIENT_SECRET", "PTIUM_AUTH_OIDC_CLIENT_SECRET") {
		var value string
		if service.Get(ctx, "auth.oidc.client_secret", &value) == nil && value != "" {
			values["OIDC_CLIENT_SECRET"] = value
		}
	}
	if !anyEnvironment("AUTH_ADMIN_ROLES", "OIDC_ADMIN_ROLES", "PTIUM_AUTH_ADMIN_ROLES") {
		var roles []string
		if service.Get(ctx, "auth.oidc.admin_roles", &roles) == nil && len(roles) > 0 {
			values["AUTH_ADMIN_ROLES"] = strings.Join(roles, ",")
		}
	}
	return layeredAuthSource{environment: auth.EnvironmentSource{}, database: values}
}

func databaseCORSOrigins(ctx context.Context, service *settings.Service) []string {
	var origins []string
	if err := service.Get(ctx, "security.cors_origins", &origins); err != nil {
		return nil
	}
	return origins
}

func anyEnvironment(keys ...string) bool {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func appendUnique(values []string, extras ...string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values)+len(extras))
	for _, value := range append(values, extras...) {
		value = strings.TrimRight(strings.TrimSpace(value), "/")
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func newLogger(levelName string) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(levelName)) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

func fatal(operation string, err error) {
	slog.Error(operation, "error", err)
	os.Exit(1)
}
