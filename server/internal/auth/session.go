package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SessionTokenPrefix marks a Ptium browser session token. It is deliberately
// different from the API-key prefix so an authenticator can tell the two apart
// without attempting a database lookup.
const SessionTokenPrefix = "ptses_"

const (
	defaultSessionTTL = 12 * time.Hour
	maximumSessionTTL = 30 * 24 * time.Hour
	// A session token is small; anything larger is not one.
	maximumSessionTokenBytes = 4096
)

// SessionClaims is what a session token asserts. It carries no roles: they are
// read from the account on every request, so a revoked administrator loses
// access immediately rather than at token expiry.
type SessionClaims struct {
	UserID string `json:"sub"`
	// Epoch is the account's password-change timestamp. A password change moves
	// it forward and every token issued before then stops verifying.
	Epoch     int64 `json:"pwd"`
	IssuedAt  int64 `json:"iat"`
	ExpiresAt int64 `json:"exp"`
}

// SessionIssuer mints and verifies stateless session tokens. Statelessness is
// deliberate: Ptium keeps no session table, and the password epoch provides the
// one revocation signal that matters.
type SessionIssuer struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

// NewSessionIssuer derives a signing key from the deployment secret. The key is
// domain-separated so it can never collide with another use of the same secret.
func NewSessionIssuer(secret string, ttl time.Duration) (*SessionIssuer, error) {
	if len(strings.TrimSpace(secret)) < 16 {
		return nil, errors.New("auth session: signing secret must be at least 16 characters")
	}
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	if ttl > maximumSessionTTL {
		return nil, fmt.Errorf("auth session: lifetime must not exceed %s", maximumSessionTTL)
	}
	derived := hmac.New(sha256.New, []byte(secret))
	derived.Write([]byte("ptium/session-token/v1"))
	return &SessionIssuer{key: derived.Sum(nil), ttl: ttl, now: time.Now}, nil
}

// Lifetime is how long a freshly issued token stays valid.
func (issuer *SessionIssuer) Lifetime() time.Duration { return issuer.ttl }

// Issue mints a token for an account.
func (issuer *SessionIssuer) Issue(userID string, epoch int64) (string, time.Time, error) {
	if strings.TrimSpace(userID) == "" {
		return "", time.Time{}, errors.New("auth session: user id is required")
	}
	now := issuer.now().UTC()
	expiresAt := now.Add(issuer.ttl)
	claims := SessionClaims{UserID: userID, Epoch: epoch, IssuedAt: now.Unix(), ExpiresAt: expiresAt.Unix()}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return SessionTokenPrefix + encoded + "." + base64.RawURLEncoding.EncodeToString(issuer.sign(encoded)), expiresAt, nil
}

// Parse verifies a token's signature and expiry and returns its claims.
func (issuer *SessionIssuer) Parse(token string) (SessionClaims, error) {
	token = strings.TrimSpace(token)
	if len(token) > maximumSessionTokenBytes {
		return SessionClaims{}, ErrInvalidCredentials
	}
	if !strings.HasPrefix(token, SessionTokenPrefix) {
		return SessionClaims{}, ErrInvalidCredentials
	}
	body := strings.TrimPrefix(token, SessionTokenPrefix)
	encoded, signature, found := strings.Cut(body, ".")
	if !found || encoded == "" || signature == "" {
		return SessionClaims{}, ErrInvalidCredentials
	}
	provided, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return SessionClaims{}, ErrInvalidCredentials
	}
	// Compare before parsing, so unsigned input never reaches the decoder.
	if subtle.ConstantTimeCompare(provided, issuer.sign(encoded)) != 1 {
		return SessionClaims{}, ErrInvalidCredentials
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return SessionClaims{}, ErrInvalidCredentials
	}
	var claims SessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return SessionClaims{}, ErrInvalidCredentials
	}
	if claims.UserID == "" || claims.ExpiresAt == 0 {
		return SessionClaims{}, ErrInvalidCredentials
	}
	if issuer.now().UTC().Unix() >= claims.ExpiresAt {
		return SessionClaims{}, fmt.Errorf("%w: session expired", ErrInvalidCredentials)
	}
	return claims, nil
}

func (issuer *SessionIssuer) sign(encoded string) []byte {
	mac := hmac.New(sha256.New, issuer.key)
	mac.Write([]byte(encoded))
	return mac.Sum(nil)
}

// SessionResolver confirms that a token's claims still match the account. It is
// the hook that makes a password change revoke outstanding tokens.
type SessionResolver interface {
	ResolveSession(context.Context, SessionClaims) (*Principal, error)
}

// SessionResolverFunc adapts a function.
type SessionResolverFunc func(context.Context, SessionClaims) (*Principal, error)

func (fn SessionResolverFunc) ResolveSession(ctx context.Context, claims SessionClaims) (*Principal, error) {
	return fn(ctx, claims)
}

// SessionAuthenticator accepts bearer session tokens. Place it before the
// API-key authenticator in a composite so an opaque session token is never
// looked up as a key.
type SessionAuthenticator struct {
	Issuer   *SessionIssuer
	Resolver SessionResolver
}

func (authenticator SessionAuthenticator) Authenticate(ctx context.Context, request *http.Request) (*Principal, error) {
	if authenticator.Issuer == nil || authenticator.Resolver == nil {
		return nil, errors.New("session authenticator is not configured")
	}
	token, err := bearerToken(request)
	if err != nil || token == "" || !strings.HasPrefix(token, SessionTokenPrefix) {
		// Not a session token: let the next authenticator try.
		return nil, ErrNoCredentials
	}
	claims, parseErr := authenticator.Issuer.Parse(token)
	if parseErr != nil {
		return nil, parseErr
	}
	return authenticator.Resolver.ResolveSession(ctx, claims)
}
