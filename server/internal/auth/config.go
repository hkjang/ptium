package auth

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPTimeout  = 10 * time.Second
	defaultJWKSCacheTTL = 15 * time.Minute
	defaultClockSkew    = 30 * time.Second
	defaultMaxTokenSize = 32 << 10
	defaultDevHeader    = "X-Ptium-Dev-Secret"
)

// ValueSource abstracts process bootstrap configuration. The service can use
// EnvironmentSource at startup and later build the same config from settings in
// Postgres without coupling authentication to either storage mechanism.
type ValueSource interface {
	Lookup(key string) (string, bool)
}

// EnvironmentSource reads bootstrap values from the process environment.
type EnvironmentSource struct{}

func (EnvironmentSource) Lookup(key string) (string, bool) { return os.LookupEnv(key) }

// MapSource is useful for database-backed adapters and focused tests.
type MapSource map[string]string

func (source MapSource) Lookup(key string) (string, bool) {
	value, ok := source[key]
	return value, ok
}

// OIDCConfig configures a generic OIDC resource server. It works with Keycloak
// through standard issuer discovery. A client secret is optional: a public
// client uses Authorization Code + PKCE and needs none, while a confidential
// client needs one and therefore has to exchange the code through Ptium rather
// than from the browser.
type OIDCConfig struct {
	Enabled  bool
	Issuer   string
	ClientID string
	// ClientSecret turns the browser flow into a confidential-client flow: the
	// secret never reaches the browser, so Ptium performs the token exchange.
	ClientSecret      string
	Audiences         []string
	AllowedAlgorithms []string
	AllowHTTP         bool
	HTTPTimeout       time.Duration
	JWKSCacheTTL      time.Duration
	ClockSkew         time.Duration
	MaxTokenBytes     int
}

// DevConfig controls the deliberately separate development credential. It is
// disabled by default, uses a dedicated header, and requires a high-entropy
// shared token. Remote clients are rejected unless AllowRemote is explicit.
type DevConfig struct {
	Enabled bool
	// Secret is compared in constant time with the dedicated request header.
	Secret string
	// Token is a deprecated compatibility alias for Secret.
	Token       string
	Header      string
	AllowRemote bool
	Principal   Principal
}

// BootstrapConfig is the process-level authentication configuration. Admin
// roles are kept here so authorization policy can later be supplied by the
// administrator settings store using the same shape.
type BootstrapConfig struct {
	OIDC       OIDCConfig
	Dev        DevConfig
	AdminRoles []string
}

// LoadBootstrapConfig reads conventional Ptium environment keys from source.
// OIDC_ISSUER_URL + OIDC_CLIENT_ID are sufficient for a standard Keycloak
// setup. OIDC_AUDIENCE is optional, but should be set whenever the identity
// provider is configured to emit an API-specific audience.
func LoadBootstrapConfig(source ValueSource) (BootstrapConfig, error) {
	if source == nil {
		return BootstrapConfig{}, fmt.Errorf("auth config: nil value source")
	}

	config := BootstrapConfig{
		OIDC: OIDCConfig{
			AllowedAlgorithms: []string{"RS256"},
			HTTPTimeout:       defaultHTTPTimeout,
			JWKSCacheTTL:      defaultJWKSCacheTTL,
			ClockSkew:         defaultClockSkew,
			MaxTokenBytes:     defaultMaxTokenSize,
		},
		Dev: DevConfig{
			Header: defaultDevHeader,
		},
		AdminRoles: []string{"admin", "ptium-admin"},
	}

	config.OIDC.Issuer = firstValue(source,
		"OIDC_ISSUER_URL", "PTIUM_OIDC_ISSUER_URL", "PTIUM_AUTH_OIDC_ISSUER")
	config.OIDC.ClientID = firstValue(source,
		"OIDC_CLIENT_ID", "PTIUM_OIDC_CLIENT_ID", "PTIUM_AUTH_OIDC_CLIENT_ID")
	config.OIDC.ClientSecret = firstValue(source,
		"OIDC_CLIENT_SECRET", "PTIUM_OIDC_CLIENT_SECRET", "PTIUM_AUTH_OIDC_CLIENT_SECRET")

	keycloakURL := firstValue(source, "KEYCLOAK_URL", "PTIUM_KEYCLOAK_URL")
	keycloakRealm := firstValue(source, "KEYCLOAK_REALM", "PTIUM_KEYCLOAK_REALM")
	if config.OIDC.Issuer == "" && (keycloakURL != "" || keycloakRealm != "") {
		if keycloakURL == "" || keycloakRealm == "" {
			return BootstrapConfig{}, fmt.Errorf("auth config: KEYCLOAK_URL and KEYCLOAK_REALM must be set together")
		}
		config.OIDC.Issuer = strings.TrimRight(keycloakURL, "/") + "/realms/" + url.PathEscape(keycloakRealm)
	}
	config.OIDC.Issuer = strings.TrimRight(strings.TrimSpace(config.OIDC.Issuer), "/")
	config.OIDC.Enabled = config.OIDC.Issuer != ""

	audiences := splitList(firstValue(source,
		"OIDC_AUDIENCE", "OIDC_AUDIENCES", "PTIUM_OIDC_AUDIENCE", "PTIUM_AUTH_OIDC_AUDIENCES"))
	config.OIDC.Audiences = audiences
	if algorithms := splitList(firstValue(source, "OIDC_ALLOWED_ALGORITHMS", "PTIUM_AUTH_OIDC_ALGORITHMS")); len(algorithms) > 0 {
		config.OIDC.AllowedAlgorithms = algorithms
	}

	var err error
	if config.OIDC.AllowHTTP, err = boolValue(source, false, "OIDC_ALLOW_HTTP", "PTIUM_AUTH_OIDC_ALLOW_HTTP"); err != nil {
		return BootstrapConfig{}, err
	}
	if config.OIDC.HTTPTimeout, err = durationValue(source, config.OIDC.HTTPTimeout, "OIDC_HTTP_TIMEOUT", "PTIUM_AUTH_OIDC_HTTP_TIMEOUT"); err != nil {
		return BootstrapConfig{}, err
	}
	if config.OIDC.JWKSCacheTTL, err = durationValue(source, config.OIDC.JWKSCacheTTL, "OIDC_JWKS_CACHE_TTL", "PTIUM_AUTH_OIDC_JWKS_CACHE_TTL"); err != nil {
		return BootstrapConfig{}, err
	}
	if config.OIDC.ClockSkew, err = durationValue(source, config.OIDC.ClockSkew, "OIDC_CLOCK_SKEW", "PTIUM_AUTH_OIDC_CLOCK_SKEW"); err != nil {
		return BootstrapConfig{}, err
	}
	if config.OIDC.MaxTokenBytes, err = intValue(source, config.OIDC.MaxTokenBytes, "OIDC_MAX_TOKEN_BYTES", "PTIUM_AUTH_OIDC_MAX_TOKEN_BYTES"); err != nil {
		return BootstrapConfig{}, err
	}

	if config.Dev.Enabled, err = boolValue(source, false, "DEV_AUTH_ENABLED", "PTIUM_AUTH_DEV_ENABLED"); err != nil {
		return BootstrapConfig{}, err
	}
	config.Dev.Secret = firstValue(source, "DEV_AUTH_SECRET", "DEV_AUTH_TOKEN", "PTIUM_AUTH_DEV_TOKEN")
	if header := firstValue(source, "DEV_AUTH_HEADER", "PTIUM_AUTH_DEV_HEADER"); header != "" {
		config.Dev.Header = header
	}
	if config.Dev.AllowRemote, err = boolValue(source, false, "DEV_AUTH_ALLOW_REMOTE", "PTIUM_AUTH_DEV_ALLOW_REMOTE"); err != nil {
		return BootstrapConfig{}, err
	}
	devEmail := firstValue(source, "DEV_AUTH_EMAIL", "PTIUM_AUTH_DEV_EMAIL")
	devSubject := firstValue(source, "DEV_AUTH_SUBJECT", "PTIUM_AUTH_DEV_SUBJECT")
	if devSubject == "" && devEmail != "" {
		devSubject = "dev:" + devEmail
	}
	if devSubject == "" {
		devSubject = "dev:local"
	}
	devName := firstValue(source, "DEV_AUTH_NAME", "PTIUM_AUTH_DEV_NAME")
	if devName == "" {
		devName = "Ptium Developer"
	}
	devRoles := splitList(firstValue(source, "DEV_AUTH_ROLES", "PTIUM_AUTH_DEV_ROLES"))
	if len(devRoles) == 0 {
		devRoles = []string{"ptium-admin", "user"}
	}
	devScopes := splitList(firstValue(source, "DEV_AUTH_SCOPES", "PTIUM_AUTH_DEV_SCOPES"))
	if len(devScopes) == 0 {
		devScopes = []string{"presentations:read", "presentations:write", "mcp:use"}
	}
	config.Dev.Principal = Principal{
		Subject:    devSubject,
		Email:      devEmail,
		Name:       devName,
		Roles:      normalizedRoles(devRoles),
		Scopes:     normalizedRoles(devScopes),
		AuthMethod: "dev",
	}

	if roles := splitList(firstValue(source, "OIDC_ADMIN_ROLES", "AUTH_ADMIN_ROLES", "PTIUM_AUTH_ADMIN_ROLES")); len(roles) > 0 {
		config.AdminRoles = normalizedRoles(roles)
	}

	if err := config.Validate(); err != nil {
		return BootstrapConfig{}, err
	}
	return config, nil
}

func (config BootstrapConfig) Validate() error {
	if config.OIDC.Enabled {
		if err := config.OIDC.Validate(); err != nil {
			return err
		}
	}
	if err := config.Dev.Validate(); err != nil {
		return err
	}
	if len(normalizedRoles(config.AdminRoles)) == 0 {
		return fmt.Errorf("auth config: at least one admin role is required")
	}
	return nil
}

func (config OIDCConfig) Validate() error {
	issuer := strings.TrimSpace(config.Issuer)
	if issuer == "" {
		return fmt.Errorf("auth config: OIDC issuer is required")
	}
	if err := validateRemoteURL(issuer, config.AllowHTTP); err != nil {
		return fmt.Errorf("auth config: OIDC issuer: %w", err)
	}
	if config.HTTPTimeout <= 0 {
		return fmt.Errorf("auth config: OIDC HTTP timeout must be positive")
	}
	if config.JWKSCacheTTL <= 0 {
		return fmt.Errorf("auth config: OIDC JWKS cache TTL must be positive")
	}
	if config.ClockSkew < 0 {
		return fmt.Errorf("auth config: OIDC clock skew cannot be negative")
	}
	if config.MaxTokenBytes < 1024 || config.MaxTokenBytes > 1<<20 {
		return fmt.Errorf("auth config: OIDC max token bytes must be between 1024 and 1048576")
	}
	if len(config.AllowedAlgorithms) == 0 {
		return fmt.Errorf("auth config: at least one OIDC signing algorithm is required")
	}
	// A secret without a client id cannot be presented to the token endpoint.
	if strings.TrimSpace(config.ClientSecret) != "" && strings.TrimSpace(config.ClientID) == "" {
		return fmt.Errorf("auth config: OIDC client secret requires a client id")
	}
	for _, algorithm := range config.AllowedAlgorithms {
		if !supportedAlgorithm(algorithm) {
			return fmt.Errorf("auth config: unsupported OIDC signing algorithm %q", algorithm)
		}
	}
	return nil
}

func (config DevConfig) Validate() error {
	if !config.Enabled {
		return nil
	}
	secret := config.Secret
	if secret == "" {
		secret = config.Token
	}
	if len(secret) < 32 {
		return fmt.Errorf("auth config: DEV_AUTH_SECRET must contain at least 32 characters when development auth is enabled")
	}
	if strings.TrimSpace(config.Header) == "" || strings.ContainsAny(config.Header, "\r\n:") {
		return fmt.Errorf("auth config: development auth header is invalid")
	}
	if strings.TrimSpace(config.Principal.Subject) == "" {
		return fmt.Errorf("auth config: development auth subject is required")
	}
	return nil
}

func validateRemoteURL(rawURL string, allowHTTP bool) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("URL must have a host and no credentials, query, or fragment")
	}
	if parsed.Scheme != "https" && !(allowHTTP && parsed.Scheme == "http") {
		return fmt.Errorf("URL must use HTTPS (HTTP requires OIDC_ALLOW_HTTP=true)")
	}
	return nil
}

func firstValue(source ValueSource, keys ...string) string {
	for _, key := range keys {
		if value, ok := source.Lookup(key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func boolValue(source ValueSource, fallback bool, keys ...string) (bool, error) {
	value := firstValue(source, keys...)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("auth config: %s must be a boolean", keys[0])
	}
	return parsed, nil
}

func durationValue(source ValueSource, fallback time.Duration, keys ...string) (time.Duration, error) {
	value := firstValue(source, keys...)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("auth config: %s must be a duration: %w", keys[0], err)
	}
	return parsed, nil
}

func intValue(source ValueSource, fallback int, keys ...string) (int, error) {
	value := firstValue(source, keys...)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("auth config: %s must be an integer", keys[0])
	}
	return parsed, nil
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return normalizedRoles(strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	}))
}
