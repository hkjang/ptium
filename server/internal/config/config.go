package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains the small bootstrap configuration surface. Runtime product
// settings live in PostgreSQL and are managed through the admin API.
type Config struct {
	DatabaseURL   string
	HTTPAddr      string
	PublicBaseURL string
	// WebDir holds the compiled single-page workspace. When empty the process
	// serves only the API, which is what a development setup wants while Vite
	// serves the UI.
	WebDir                 string
	CORSAllowedOrigins     []string
	OIDCIssuerURL          string
	OIDCClientID           string
	OIDCClientSecret       string
	OIDCAudience           []string
	OIDCAdminRoles         []string
	BootstrapAdminEmails   []string
	BootstrapAdminSubjects []string
	// BootstrapAdmin is a local administrator that can sign in with a password,
	// so a deployment is administrable before any identity provider exists.
	BootstrapAdmin         string
	BootstrapAdminPassword string
	BootstrapAdminName     string
	// BootstrapAdminPasswordReset overwrites an existing password on start,
	// which is how an operator recovers a lost administrator password.
	BootstrapAdminPasswordReset bool
	SessionLifetime             time.Duration
	DevAuthEnabled              bool
	DevAuthSecret               string
	DevAuthEmail                string
	DevAuthName                 string
	DevAuthRoles                []string
	DevAuthAllowRemote          bool
	KeyEncryptionSecret         string
	// AssetStorage decides where uploaded images are kept: "database" (the
	// default, nothing to mount) or "filesystem" (a directory, which in
	// Kubernetes is a PersistentVolumeClaim).
	AssetStorage       string
	AssetDir           string
	LogLevel           string
	ShutdownTimeout    time.Duration
	WorkerPollInterval time.Duration
}

func Load() (Config, error) {
	dsn := firstNonEmpty(os.Getenv("DATABASE_URL"), os.Getenv("PTIUM_DATABASE_DSN"))
	c := Config{
		DatabaseURL:                 dsn,
		HTTPAddr:                    envDefault("HTTP_ADDR", ":8080"),
		PublicBaseURL:               strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/"),
		WebDir:                      strings.TrimSpace(os.Getenv("WEB_DIR")),
		CORSAllowedOrigins:          splitCSV(envDefault("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173")),
		OIDCIssuerURL:               strings.TrimRight(os.Getenv("OIDC_ISSUER_URL"), "/"),
		OIDCClientID:                os.Getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret:            os.Getenv("OIDC_CLIENT_SECRET"),
		OIDCAudience:                splitCSV(firstNonEmpty(os.Getenv("OIDC_AUDIENCE"), os.Getenv("OIDC_AUDIENCES"))),
		OIDCAdminRoles:              splitCSV(firstNonEmpty(os.Getenv("AUTH_ADMIN_ROLES"), os.Getenv("OIDC_ADMIN_ROLES"), "ptium-admin,admin")),
		BootstrapAdminEmails:        lowerStrings(splitCSV(os.Getenv("BOOTSTRAP_ADMIN_EMAILS"))),
		BootstrapAdminSubjects:      splitCSV(os.Getenv("BOOTSTRAP_ADMIN_SUBJECTS")),
		BootstrapAdmin:              strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN")),
		BootstrapAdminPassword:      os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
		BootstrapAdminName:          strings.TrimSpace(envDefault("BOOTSTRAP_ADMIN_NAME", "Ptium Administrator")),
		BootstrapAdminPasswordReset: envBool("BOOTSTRAP_ADMIN_PASSWORD_RESET", false),
		DevAuthEnabled:              envBool("DEV_AUTH_ENABLED", false),
		DevAuthSecret:               firstNonEmpty(os.Getenv("DEV_AUTH_SECRET"), os.Getenv("DEV_AUTH_TOKEN")),
		DevAuthEmail:                os.Getenv("DEV_AUTH_EMAIL"),
		DevAuthName:                 os.Getenv("DEV_AUTH_NAME"),
		DevAuthRoles:                splitCSV(os.Getenv("DEV_AUTH_ROLES")),
		DevAuthAllowRemote:          envBool("DEV_AUTH_ALLOW_REMOTE", false),
		KeyEncryptionSecret:         os.Getenv("KEY_ENCRYPTION_SECRET"),
		AssetStorage:                strings.ToLower(envDefault("ASSET_STORAGE", "database")),
		AssetDir:                    strings.TrimSpace(envDefault("ASSET_DIR", "/var/lib/ptium/assets")),
		LogLevel:                    envDefault("LOG_LEVEL", "info"),
		ShutdownTimeout:             envDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
		WorkerPollInterval:          envDuration("WORKER_POLL_INTERVAL", 2*time.Second),
		SessionLifetime:             envDuration("SESSION_LIFETIME", 12*time.Hour),
	}
	if c.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL (or PTIUM_DATABASE_DSN) is required")
	}
	if c.DevAuthEnabled && c.DevAuthSecret == "" {
		return Config{}, errors.New("DEV_AUTH_SECRET is required when DEV_AUTH_ENABLED=true")
	}
	if c.OIDCIssuerURL != "" && c.OIDCClientID == "" {
		return Config{}, errors.New("OIDC_CLIENT_ID is required when OIDC_ISSUER_URL is set")
	}
	switch c.AssetStorage {
	case "database", "filesystem":
	case "volume", "pvc", "file", "disk":
		// The names an operator reaches for first mean the same thing.
		c.AssetStorage = "filesystem"
	default:
		return Config{}, errors.New(`ASSET_STORAGE must be "database" or "filesystem"`)
	}
	if c.AssetStorage == "filesystem" && c.AssetDir == "" {
		return Config{}, errors.New("ASSET_DIR is required when ASSET_STORAGE=filesystem")
	}
	return c, nil
}

func envDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func splitCSV(value string) []string {
	var result []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func lowerStrings(values []string) []string {
	for i := range values {
		values[i] = strings.ToLower(values[i])
	}
	return values
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
