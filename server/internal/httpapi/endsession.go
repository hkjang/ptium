package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/hkjang/ptium/server/internal/auth"
)

// Signing out of the identity provider as well as this app.
//
// Measured against a real Keycloak: sign-out sent the browser to the
// provider's end-session endpoint with no id_token_hint, and Keycloak stopped
// there on a confirmation page of its own. Whoever closed that tab left the
// provider's session alive, and the next tab they opened was signed straight
// back in by the silent sign-in this app offers — sign-out that did not sign
// anybody out.

// maximumIDTokenBytes keeps the hint inside what one cookie can carry.
const maximumIDTokenBytes = 3800

// idTokenFor reports whether an ID token is this deployment's, for the person
// the session is being opened for.
//
// Its signature is not checked here: nothing in this app trusts it. It is only
// handed back to the provider that issued it, which checks it itself. What is
// checked is that it is the right shape and belongs to this client and this
// person, so a session cannot be made to carry somebody else's hint.
func idTokenFor(idToken, issuer, clientID, subject string) bool {
	if idToken == "" || len(idToken) > maximumIDTokenBytes || issuer == "" || clientID == "" {
		return false
	}
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		Issuer   string          `json:"iss"`
		Subject  string          `json:"sub"`
		Audience json.RawMessage `json:"aud"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return false
	}
	if strings.TrimRight(claims.Issuer, "/") != strings.TrimRight(issuer, "/") {
		return false
	}
	if subject != "" && claims.Subject != subject {
		return false
	}
	var one string
	if json.Unmarshal(claims.Audience, &one) == nil {
		return one == clientID
	}
	var many []string
	if json.Unmarshal(claims.Audience, &many) == nil {
		for _, audience := range many {
			if audience == clientID {
				return true
			}
		}
	}
	return false
}

// endSessionURL is the provider's sign-out address for this browser, or ""
// when the deployment has no provider to sign out of.
func (s *Server) endSessionURL(request *http.Request, postLogout string) string {
	endpoint := strings.TrimSpace(s.authPublic.EndSessionEndpoint)
	if !s.authPublic.OIDCEnabled || endpoint == "" {
		return ""
	}
	target, err := url.Parse(endpoint)
	if err != nil || (target.Scheme != "https" && target.Scheme != "http") {
		return ""
	}
	query := target.Query()
	if s.authPublic.ClientID != "" {
		query.Set("client_id", s.authPublic.ClientID)
	}
	if cookie, err := request.Cookie(auth.IDTokenHintCookieName); err == nil && cookie.Value != "" {
		query.Set("id_token_hint", cookie.Value)
	}
	if back, err := url.Parse(postLogout); err == nil && (back.Scheme == "https" || back.Scheme == "http") && back.Host != "" {
		// The provider only honours an address registered for this client, so
		// a foreign one here goes nowhere; it is checked for shape only.
		query.Set("post_logout_redirect_uri", back.String())
	}
	target.RawQuery = query.Encode()
	return target.String()
}
