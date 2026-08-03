package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// DiscoveryDocument contains the OIDC endpoints useful to the API and browser
// login flow. The authenticator only requires Issuer and JWKSURI.
type DiscoveryDocument struct {
	Issuer                string `json:"issuer"`
	JWKSURI               string `json:"jwks_uri"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

type oidcOptions struct {
	httpClient *http.Client
	now        func() time.Time
}

// OIDCOption customizes the authenticator without making bootstrap callers
// depend on implementation details.
type OIDCOption func(*oidcOptions)

// WithOIDCHTTPClient supplies a client with custom transport/TLS policy.
func WithOIDCHTTPClient(client *http.Client) OIDCOption {
	return func(options *oidcOptions) { options.httpClient = client }
}

// WithOIDCClock supplies a clock for deterministic verification tests.
func WithOIDCClock(now func() time.Time) OIDCOption {
	return func(options *oidcOptions) { options.now = now }
}

// OIDCAuthenticator validates bearer JWTs with discovery and a rotation-aware
// remote JWKS cache.
type OIDCAuthenticator struct {
	config    OIDCConfig
	discovery DiscoveryDocument
	keys      *remoteKeySet
	now       func() time.Time
}

// NewOIDCAuthenticator performs discovery and an initial JWKS load. This makes
// issuer mistakes fail at startup instead of during the first user request.
func NewOIDCAuthenticator(ctx context.Context, config OIDCConfig, optionFunctions ...OIDCOption) (*OIDCAuthenticator, error) {
	config = oidcDefaults(config)
	config.Issuer = strings.TrimRight(strings.TrimSpace(config.Issuer), "/")
	config.Audiences = normalizedRoles(config.Audiences)
	config.AllowedAlgorithms = normalizedRoles(config.AllowedAlgorithms)
	if err := config.Validate(); err != nil {
		return nil, err
	}

	options := oidcOptions{now: time.Now}
	for _, apply := range optionFunctions {
		if apply != nil {
			apply(&options)
		}
	}
	if options.now == nil {
		options.now = time.Now
	}
	client := options.httpClient
	if client == nil {
		client = &http.Client{Timeout: config.HTTPTimeout}
	}

	discovery, err := discoverProvider(ctx, client, config)
	if err != nil {
		return nil, err
	}
	keySet := newRemoteKeySet(client, discovery.JWKSURI, config.JWKSCacheTTL, config.AllowHTTP, options.now)
	if err := keySet.refresh(ctx, true); err != nil {
		return nil, fmt.Errorf("load OIDC JWKS: %w", err)
	}
	return &OIDCAuthenticator{
		config:    config,
		discovery: discovery,
		keys:      keySet,
		now:       options.now,
	}, nil
}

// Discovery returns the validated provider metadata.
func (authenticator *OIDCAuthenticator) Discovery() DiscoveryDocument {
	if authenticator == nil {
		return DiscoveryDocument{}
	}
	return authenticator.discovery
}

func (authenticator *OIDCAuthenticator) Authenticate(ctx context.Context, request *http.Request) (*Principal, error) {
	if authenticator == nil || authenticator.keys == nil {
		return nil, errors.New("OIDC authenticator is not initialized")
	}
	token, err := bearerToken(request)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(token, "dev:") {
		return nil, ErrNoCredentials
	}
	// Opaque bearer values belong to the API-key/dev mechanisms in a composite.
	if strings.Count(token, ".") != 2 {
		return nil, ErrNoCredentials
	}
	if len(token) > authenticator.config.MaxTokenBytes {
		return nil, fmt.Errorf("%w: OIDC token is too large", ErrInvalidCredentials)
	}

	header, claims, signingInput, signature, err := parseJWT(token)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCredentials, err)
	}
	if !contains(authenticator.config.AllowedAlgorithms, header.Algorithm) {
		return nil, fmt.Errorf("%w: OIDC signing algorithm is not allowed", ErrInvalidCredentials)
	}
	keys, err := authenticator.keys.verificationKeys(ctx, header.KeyID, header.Algorithm)
	if err != nil {
		return nil, fmt.Errorf("refresh OIDC verification keys: %w", err)
	}
	verified := false
	for _, key := range keys {
		if verifyJWTSignature(header.Algorithm, key.publicKey, signingInput, signature) == nil {
			verified = true
			break
		}
	}
	if !verified {
		// A provider should issue a new kid when rotating keys, but a forced,
		// rate-limited refresh also handles providers that reuse identifiers.
		if refreshedKeys, refreshErr := authenticator.keys.forceVerificationKeys(ctx, header.KeyID, header.Algorithm); refreshErr == nil {
			for _, key := range refreshedKeys {
				if verifyJWTSignature(header.Algorithm, key.publicKey, signingInput, signature) == nil {
					verified = true
					break
				}
			}
		}
	}
	if !verified {
		return nil, fmt.Errorf("%w: OIDC signature verification failed", ErrInvalidCredentials)
	}

	principal, err := authenticator.validateClaims(claims)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCredentials, err)
	}
	return principal, nil
}

type jwtHeader struct {
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
}

func parseJWT(token string) (jwtHeader, map[string]any, []byte, []byte, error) {
	segments := strings.Split(token, ".")
	if len(segments) != 3 || segments[0] == "" || segments[1] == "" || segments[2] == "" {
		return jwtHeader{}, nil, nil, nil, errors.New("JWT must contain three non-empty segments")
	}
	headerBytes, err := decodeJWTPart(segments[0])
	if err != nil {
		return jwtHeader{}, nil, nil, nil, fmt.Errorf("decode JWT header: %w", err)
	}
	claimBytes, err := decodeJWTPart(segments[1])
	if err != nil {
		return jwtHeader{}, nil, nil, nil, fmt.Errorf("decode JWT claims: %w", err)
	}
	signature, err := decodeJWTPart(segments[2])
	if err != nil {
		return jwtHeader{}, nil, nil, nil, fmt.Errorf("decode JWT signature: %w", err)
	}

	var header jwtHeader
	if err := decodeJSONObject(headerBytes, &header); err != nil {
		return jwtHeader{}, nil, nil, nil, fmt.Errorf("parse JWT header: %w", err)
	}
	if header.Algorithm == "" || header.Algorithm == "none" {
		return jwtHeader{}, nil, nil, nil, errors.New("JWT algorithm is missing or unsafe")
	}
	var claims map[string]any
	if err := decodeJSONObject(claimBytes, &claims); err != nil {
		return jwtHeader{}, nil, nil, nil, fmt.Errorf("parse JWT claims: %w", err)
	}
	return header, claims, []byte(segments[0] + "." + segments[1]), signature, nil
}

func decodeJWTPart(value string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err == nil {
		return decoded, nil
	}
	return base64.URLEncoding.DecodeString(value)
}

func decodeJSONObject(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("unexpected data after JSON object")
	}
	return nil
}

func (authenticator *OIDCAuthenticator) validateClaims(claims map[string]any) (*Principal, error) {
	now := authenticator.now()
	skew := authenticator.config.ClockSkew
	issuer, ok := stringClaim(claims, "iss")
	if !ok || issuer != authenticator.discovery.Issuer {
		return nil, errors.New("issuer claim does not match the discovered provider")
	}
	subject, ok := stringClaim(claims, "sub")
	if !ok || strings.TrimSpace(subject) == "" {
		return nil, errors.New("subject claim is required")
	}
	expiresAt, ok := numericDateClaim(claims, "exp")
	if !ok {
		return nil, errors.New("expiration claim is required")
	}
	if now.After(expiresAt.Add(skew)) {
		return nil, errors.New("token has expired")
	}
	if notBefore, present := numericDateClaim(claims, "nbf"); present && now.Add(skew).Before(notBefore) {
		return nil, errors.New("token is not valid yet")
	}
	if issuedAt, present := numericDateClaim(claims, "iat"); present && now.Add(skew).Before(issuedAt) {
		return nil, errors.New("token was issued in the future")
	}
	if len(authenticator.config.Audiences) > 0 && !audienceMatches(claims["aud"], authenticator.config.Audiences) {
		return nil, errors.New("audience claim does not match this service")
	}

	email, _ := stringClaim(claims, "email")
	name, _ := stringClaim(claims, "name")
	if name == "" {
		name, _ = stringClaim(claims, "preferred_username")
	}
	return &Principal{
		Subject:    subject,
		Email:      email,
		Name:       name,
		Issuer:     issuer,
		Roles:      oidcRoles(claims, authenticator.config.ClientID),
		Scopes:     oidcScopes(claims),
		AuthMethod: "oidc",
		Claims:     claims,
	}, nil
}

func oidcScopes(claims map[string]any) []string {
	return normalizedRoles(claimStrings(claims["scope"]), claimStrings(claims["scp"]))
}

func discoverProvider(ctx context.Context, client *http.Client, config OIDCConfig) (DiscoveryDocument, error) {
	discoveryURL := config.Issuer + "/.well-known/openid-configuration"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return DiscoveryDocument{}, fmt.Errorf("create OIDC discovery request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return DiscoveryDocument{}, fmt.Errorf("OIDC discovery request: %w", err)
	}
	defer response.Body.Close()
	if !config.AllowHTTP && response.Request != nil && response.Request.URL.Scheme != "https" {
		return DiscoveryDocument{}, errors.New("OIDC discovery redirected to a non-HTTPS endpoint")
	}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return DiscoveryDocument{}, fmt.Errorf("OIDC discovery returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20+1))
	if err != nil {
		return DiscoveryDocument{}, fmt.Errorf("read OIDC discovery: %w", err)
	}
	if len(data) > 1<<20 {
		return DiscoveryDocument{}, errors.New("OIDC discovery response is too large")
	}
	var discovery DiscoveryDocument
	if err := decodeJSONObject(data, &discovery); err != nil {
		return DiscoveryDocument{}, fmt.Errorf("decode OIDC discovery: %w", err)
	}
	if discovery.Issuer != config.Issuer {
		return DiscoveryDocument{}, fmt.Errorf("OIDC discovery issuer %q does not match configured issuer", discovery.Issuer)
	}
	if discovery.JWKSURI == "" {
		return DiscoveryDocument{}, errors.New("OIDC discovery did not provide jwks_uri")
	}
	if err := validateRemoteURL(discovery.JWKSURI, config.AllowHTTP); err != nil {
		return DiscoveryDocument{}, fmt.Errorf("OIDC jwks_uri: %w", err)
	}
	return discovery, nil
}

func oidcDefaults(config OIDCConfig) OIDCConfig {
	if len(config.AllowedAlgorithms) == 0 {
		config.AllowedAlgorithms = []string{"RS256"}
	}
	if config.HTTPTimeout == 0 {
		config.HTTPTimeout = defaultHTTPTimeout
	}
	if config.JWKSCacheTTL == 0 {
		config.JWKSCacheTTL = defaultJWKSCacheTTL
	}
	if config.ClockSkew == 0 {
		config.ClockSkew = defaultClockSkew
	}
	if config.MaxTokenBytes == 0 {
		config.MaxTokenBytes = defaultMaxTokenSize
	}
	return config
}

func stringClaim(claims map[string]any, key string) (string, bool) {
	value, ok := claims[key].(string)
	return value, ok
}

func numericDateClaim(claims map[string]any, key string) (time.Time, bool) {
	value, exists := claims[key]
	if !exists {
		return time.Time{}, false
	}
	var seconds float64
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return time.Time{}, false
		}
		seconds = parsed
	case float64:
		seconds = typed
	default:
		return time.Time{}, false
	}
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds > float64(math.MaxInt64) || seconds < float64(math.MinInt64) {
		return time.Time{}, false
	}
	whole := int64(seconds)
	nanos := int64((seconds - float64(whole)) * float64(time.Second))
	return time.Unix(whole, nanos), true
}

func audienceMatches(value any, expected []string) bool {
	var actual []string
	switch typed := value.(type) {
	case string:
		actual = []string{typed}
	case []any:
		for _, item := range typed {
			if audience, ok := item.(string); ok {
				actual = append(actual, audience)
			}
		}
	case []string:
		actual = typed
	}
	for _, candidate := range actual {
		if contains(expected, candidate) {
			return true
		}
	}
	return false
}

func oidcRoles(claims map[string]any, clientID string) []string {
	roles := claimStrings(claims["roles"])
	if realmAccess, ok := claims["realm_access"].(map[string]any); ok {
		roles = append(roles, claimStrings(realmAccess["roles"])...)
	}
	if clientID != "" {
		if resourceAccess, ok := claims["resource_access"].(map[string]any); ok {
			if clientAccess, ok := resourceAccess[clientID].(map[string]any); ok {
				roles = append(roles, claimStrings(clientAccess["roles"])...)
			}
		}
	}
	return normalizedRoles(roles)
}

func claimStrings(value any) []string {
	switch typed := value.(type) {
	case string:
		return splitList(typed)
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if stringItem, ok := item.(string); ok {
				result = append(result, stringItem)
			}
		}
		return result
	default:
		return nil
	}
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
