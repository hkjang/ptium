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
	"net/url"
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

// SessionCookieName carries the browser's session token.
//
// A cookie, not the tab's storage, is what keeps a person signed in: sessionStorage
// dies with the tab, so a new tab or a reopened browser would have to sign in
// again, and localStorage is readable by any script on the page. An HttpOnly
// cookie survives the tab and is invisible to JavaScript.
const SessionCookieName = "ptium_session"

// SessionCookie builds the cookie for a freshly issued token. Secure is decided
// per request rather than hardcoded: a browser silently drops a Secure cookie on
// a plain-HTTP origin, which would lock out an evaluation host on localhost.
func SessionCookie(token string, expiresAt time.Time, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:  SessionCookieName,
		Value: token,
		Path:  "/",
		// Lax, not Strict: a person following a link into Ptium from mail or chat
		// should arrive signed in. Lax still keeps the cookie off cross-site
		// writes, which is the case that matters.
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
		Secure:   secure,
		Expires:  expiresAt.UTC(),
		MaxAge:   int(time.Until(expiresAt).Seconds()),
	}
}

// ClearedSessionCookie expires the session cookie.
func ClearedSessionCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name: SessionCookieName, Value: "", Path: "/",
		SameSite: http.SameSiteLaxMode, HttpOnly: true, Secure: secure,
		Expires: time.Unix(0, 0).UTC(), MaxAge: -1,
	}
}

// SessionState describes the session a request arrived with, so the server can
// extend a cookie that is running out before its holder notices.
type SessionState struct {
	UserID     string
	Epoch      int64
	IssuedAt   time.Time
	ExpiresAt  time.Time
	FromCookie bool
}

// SessionStateClaim is where the authenticator records the session it accepted.
const SessionStateClaim = "ptium_session_state"

// SessionStateFromPrincipal returns the session a principal authenticated with.
func SessionStateFromPrincipal(principal *Principal) (SessionState, bool) {
	if principal == nil || principal.Claims == nil {
		return SessionState{}, false
	}
	state, ok := principal.Claims[SessionStateClaim].(SessionState)
	return state, ok
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
	// TrustedOrigin reports whether a cross-origin browser request from origin
	// may present the session cookie. Only origins a deployment has listed for
	// CORS qualify; everything else has to use a bearer token.
	TrustedOrigin func(origin string) bool
}

func (authenticator SessionAuthenticator) Authenticate(ctx context.Context, request *http.Request) (*Principal, error) {
	if authenticator.Issuer == nil || authenticator.Resolver == nil {
		return nil, errors.New("session authenticator is not configured")
	}
	token, fromCookie, err := authenticator.credential(request)
	if err != nil {
		return nil, err
	}
	claims, parseErr := authenticator.Issuer.Parse(token)
	if parseErr != nil {
		return nil, parseErr
	}
	principal, err := authenticator.Resolver.ResolveSession(ctx, claims)
	if err != nil || principal == nil {
		return principal, err
	}
	if principal.Claims == nil {
		principal.Claims = map[string]any{}
	}
	principal.Claims[SessionStateClaim] = SessionState{
		UserID: claims.UserID, Epoch: claims.Epoch, FromCookie: fromCookie,
		IssuedAt: time.Unix(claims.IssuedAt, 0).UTC(), ExpiresAt: time.Unix(claims.ExpiresAt, 0).UTC(),
	}
	return principal, nil
}

// credential picks the session token out of the request. An explicit
// Authorization header always wins so a scripted caller is never silently
// authenticated as whoever the browser last signed in as.
func (authenticator SessionAuthenticator) credential(request *http.Request) (token string, fromCookie bool, err error) {
	if len(request.Header.Values("Authorization")) > 0 {
		bearer, headerErr := bearerToken(request)
		if headerErr != nil || bearer == "" || !strings.HasPrefix(bearer, SessionTokenPrefix) {
			// An API key or some other scheme: let the next authenticator try.
			return "", false, ErrNoCredentials
		}
		return bearer, false, nil
	}
	if request.Header.Get(defaultDevHeader) != "" {
		// A developer sign-in is an explicit request to be someone else; a cookie
		// left over from an earlier session must not shadow it.
		return "", false, ErrNoCredentials
	}
	cookie, cookieErr := request.Cookie(SessionCookieName)
	if cookieErr != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false, ErrNoCredentials
	}
	if !authenticator.cookieAllowed(request) {
		return "", false, ErrNoCredentials
	}
	return cookie.Value, true, nil
}

// cookieAllowed keeps the cookie from authenticating a request another site
// caused. SameSite=Lax already stops the browser from sending it on a cross-site
// write; this refuses to honour one if it arrives anyway.
func (authenticator SessionAuthenticator) cookieAllowed(request *http.Request) bool {
	safe := request.Method == http.MethodGet || request.Method == http.MethodHead || request.Method == http.MethodOptions
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	trusted := origin != "" && authenticator.TrustedOrigin != nil && authenticator.TrustedOrigin(origin)

	switch strings.ToLower(strings.TrimSpace(request.Header.Get("Sec-Fetch-Site"))) {
	case "same-origin", "none":
		return true
	case "same-site":
		return safe || trusted
	case "cross-site":
		return trusted
	}
	// No fetch metadata: an older browser, or a client that set the cookie by
	// hand. A read is harmless; a write has to prove where it came from.
	if safe {
		return true
	}
	if origin == "" {
		return false
	}
	if trusted {
		return true
	}
	parsed, parseErr := url.Parse(origin)
	return parseErr == nil && parsed.Host != "" && strings.EqualFold(parsed.Host, request.Host)
}
