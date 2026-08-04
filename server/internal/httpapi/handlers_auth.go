package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/store"
)

// TokenExchange performs the OIDC authorization-code exchange on behalf of the
// browser. A confidential client must present a secret that may never reach a
// single-page app, so the exchange happens here instead.
type TokenExchange struct {
	// Endpoint is the identity provider's token endpoint, from discovery.
	Endpoint string
	ClientID string
	// ClientSecret is empty for a public client, in which case this endpoint
	// simply proxies the same PKCE exchange the browser could do itself.
	ClientSecret string
	Client       *http.Client
}

type tokenExchangeRequest struct {
	Code         string
	RedirectURI  string
	CodeVerifier string
}

// tokenExchangeResponse is the subset of the token response the workspace uses.
// Refresh tokens are deliberately not forwarded to the browser.
type tokenExchangeResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type,omitempty"`
	ExpiresIn   int    `json:"expires_in,omitempty"`
	IDToken     string `json:"id_token,omitempty"`
	Scope       string `json:"scope,omitempty"`
}

// exchangeToken handles POST /api/v1/auth/token. It accepts the same form body
// an identity provider would, so the browser client needs no special casing
// beyond pointing at this URL.
func (s *Server) exchangeToken(writer http.ResponseWriter, request *http.Request) {
	if s.tokenExchange == nil || strings.TrimSpace(s.tokenExchange.Endpoint) == "" {
		writeError(writer, request, http.StatusNotFound, "token_exchange_unavailable",
			"This deployment does not exchange OIDC tokens server-side", nil)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 16<<10)
	if err := request.ParseForm(); err != nil {
		writeError(writer, request, http.StatusBadRequest, "invalid_request", "The token request could not be read", nil)
		return
	}
	input := tokenExchangeRequest{
		Code:         strings.TrimSpace(request.PostFormValue("code")),
		RedirectURI:  strings.TrimSpace(request.PostFormValue("redirect_uri")),
		CodeVerifier: strings.TrimSpace(request.PostFormValue("code_verifier")),
	}
	if grant := request.PostFormValue("grant_type"); grant != "" && grant != "authorization_code" {
		writeError(writer, request, http.StatusUnprocessableEntity, "unsupported_grant_type",
			"Only the authorization_code grant is exchanged", nil)
		return
	}
	if input.Code == "" || input.RedirectURI == "" || input.CodeVerifier == "" {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error",
			"code, redirect_uri and code_verifier are required", nil)
		return
	}

	exchanged, status, err := s.tokenExchange.exchange(request.Context(), input)
	if err != nil {
		// A provider rejection is the caller's problem, not a server fault, so
		// it is reported without raising an incident.
		if status >= 400 && status < 500 {
			writeError(writer, request, http.StatusUnauthorized, "oidc_exchange_rejected", err.Error(), nil)
			return
		}
		s.internalError(writer, request, "oidc_exchange_failed", err)
		return
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(exchanged)
}

func (exchange *TokenExchange) exchange(ctx context.Context, input tokenExchangeRequest) (tokenExchangeResponse, int, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {input.Code},
		"redirect_uri":  {input.RedirectURI},
		"code_verifier": {input.CodeVerifier},
		"client_id":     {exchange.ClientID},
	}
	requestContext, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(requestContext, http.MethodPost, exchange.Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenExchangeResponse{}, 0, err
	}
	httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpRequest.Header.Set("Accept", "application/json")
	if secret := strings.TrimSpace(exchange.ClientSecret); secret != "" {
		// Client secret basic is the form every provider accepts, and it keeps
		// the secret out of the request body and therefore out of most logs.
		httpRequest.SetBasicAuth(url.QueryEscape(exchange.ClientID), url.QueryEscape(secret))
	}
	client := exchange.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	response, err := client.Do(httpRequest)
	if err != nil {
		return tokenExchangeResponse{}, 0, fmt.Errorf("contact identity provider: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return tokenExchangeResponse{}, response.StatusCode, err
	}
	var parsed struct {
		tokenExchangeResponse
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return tokenExchangeResponse{}, response.StatusCode,
			fmt.Errorf("identity provider returned an unreadable token response (status %d)", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || parsed.Error != "" {
		message := parsed.Description
		if message == "" {
			message = parsed.Error
		}
		if message == "" {
			message = fmt.Sprintf("identity provider status %d", response.StatusCode)
		}
		return tokenExchangeResponse{}, response.StatusCode, errors.New(truncateText(message, 300))
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return tokenExchangeResponse{}, response.StatusCode, errors.New("identity provider returned no access token")
	}
	return parsed.tokenExchangeResponse, response.StatusCode, nil
}

// --- local password sign-in -------------------------------------------------

type passwordLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// passwordLogin handles POST /api/v1/auth/login for the bootstrap
// administrator and any other local account.
func (s *Server) passwordLogin(writer http.ResponseWriter, request *http.Request) {
	if s.sessions == nil {
		writeError(writer, request, http.StatusNotFound, "password_login_disabled",
			"This deployment does not accept password sign-in", nil)
		return
	}
	var input passwordLoginRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	username := strings.TrimSpace(input.Username)
	if username == "" || input.Password == "" {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "username and password are required", nil)
		return
	}
	// Throttle by client address so a stolen username cannot be brute forced.
	if retryAfter := s.loginLimiter.retryAfter(clientAddress(request)); retryAfter > 0 {
		writer.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())+1))
		writeError(writer, request, http.StatusTooManyRequests, "too_many_attempts",
			"Too many sign-in attempts. Try again shortly.", nil)
		return
	}

	user, err := s.store.AuthenticateLocalUser(request.Context(), username, input.Password)
	if err != nil {
		s.loginLimiter.fail(clientAddress(request))
		if errors.Is(err, store.ErrInvalidCredentials) || errors.Is(err, store.ErrNotFound) {
			s.logger.Warn("password sign-in rejected", "request_id", RequestID(request.Context()), "username", username)
			writeError(writer, request, http.StatusUnauthorized, "invalid_credentials", "The username or password is incorrect", nil)
			return
		}
		if errors.Is(err, store.ErrAccountDisabled) {
			writeError(writer, request, http.StatusForbidden, "account_disabled", "This account is disabled", nil)
			return
		}
		s.internalError(writer, request, "password_login_failed", err)
		return
	}
	s.loginLimiter.succeed(clientAddress(request))
	token, expiresAt, err := s.sessions.Issue(user.ID, user.SessionEpoch())
	if err != nil {
		s.internalError(writer, request, "session_issue_failed", err)
		return
	}
	s.store.Audit(request.Context(), &user.ID, "auth.password_login", "user", user.ID, nil)
	writeData(writer, request, http.StatusOK, map[string]any{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   int(time.Until(expiresAt).Seconds()),
		"user":         user,
	})
}

type passwordChangeRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// changePassword handles POST /api/v1/auth/password. Changing the password
// invalidates every session token issued before it.
func (s *Server) changePassword(writer http.ResponseWriter, request *http.Request) {
	user, ok := UserFromContext(request.Context())
	if !ok {
		writeError(writer, request, http.StatusUnauthorized, "authentication_required", "Authentication is required", nil)
		return
	}
	var input passwordChangeRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if message := validatePassword(input.NewPassword); message != "" {
		writeError(writer, request, http.StatusUnprocessableEntity, "weak_password", message, nil)
		return
	}
	if err := s.store.ChangeLocalPassword(request.Context(), user.ID, input.CurrentPassword, input.NewPassword); err != nil {
		switch {
		case errors.Is(err, store.ErrInvalidCredentials):
			writeError(writer, request, http.StatusUnauthorized, "invalid_credentials", "The current password is incorrect", nil)
		case errors.Is(err, store.ErrNotFound):
			writeError(writer, request, http.StatusConflict, "not_a_local_account",
				"This account signs in through the identity provider and has no password", nil)
		default:
			s.internalError(writer, request, "password_change_failed", err)
		}
		return
	}
	s.store.Audit(request.Context(), &user.ID, "auth.password_change", "user", user.ID, nil)
	// Every previously issued token is now invalid, including this request's, so
	// the client is handed a fresh one instead of being signed out.
	refreshed, err := s.store.GetUser(request.Context(), user.ID)
	if err != nil {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	token, expiresAt, err := s.sessions.Issue(refreshed.ID, refreshed.SessionEpoch())
	if err != nil {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	writeData(writer, request, http.StatusOK, map[string]any{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   int(time.Until(expiresAt).Seconds()),
	})
}

// validatePassword enforces a length floor rather than composition rules, which
// is what current guidance recommends: length beats character classes.
func validatePassword(password string) string {
	length := utf8.RuneCountInString(password)
	switch {
	case length < 12:
		return "password must be at least 12 characters"
	case length > 256:
		return "password must not exceed 256 characters"
	case strings.TrimSpace(password) == "":
		return "password must not be only whitespace"
	}
	return ""
}

// clientAddress is the peer address without its port. Splitting on the last
// colon would mangle a bare IPv6 address, so the host is parsed properly and an
// address that carries no port is used as-is.
func clientAddress(request *http.Request) string {
	address := strings.TrimSpace(request.RemoteAddr)
	if address == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(address); err == nil {
		return host
	}
	return strings.Trim(address, "[]")
}

func truncateText(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}
