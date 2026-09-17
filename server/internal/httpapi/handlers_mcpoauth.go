package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/hkjang/ptium/server/internal/auth"
	"github.com/hkjang/ptium/server/internal/mcpoauth"
)

// mcpOAuth is the administrator's MCP SSO policy as stored now. Whether it
// takes effect also needs the web sign-in's provider to be running — that is
// what tokens are checked against — and the MCP endpoint to be served.
func (s *Server) mcpOAuth(ctx context.Context) mcpoauth.Policy {
	if s.readMCPOAuth != nil {
		return s.readMCPOAuth(ctx)
	}
	return mcpoauth.Read(ctx, s.settings, s.publicBaseURL)
}

// mcpOAuthActive says whether SSO tokens are taken at /mcp right now, and the
// policy that says so.
func (s *Server) mcpOAuthActive(ctx context.Context) (mcpoauth.Policy, bool) {
	policy := s.mcpOAuth(ctx)
	if !s.authPublic.OIDCEnabled || s.mcpHandler == nil {
		return policy, false
	}
	active, _ := policy.Active(s.authPublic.Issuer)
	return policy, active
}

// protectedResourceMetadata handles GET /.well-known/oauth-protected-resource
// and the same path suffixed /mcp: RFC 9728, the document a refused MCP
// client reads to find the authorization server. Served bare, with no
// credentials asked and any origin allowed to read it — a client running in
// a browser reads it — because it says where to sign in, not who is signed in.
// 404 while SSO is off: metadata that points at a provider whose tokens are
// then refused sends a client round a login loop.
func (s *Server) protectedResourceMetadata(writer http.ResponseWriter, request *http.Request) {
	policy, active := s.mcpOAuthActive(request.Context())
	if !active {
		writeError(writer, request, http.StatusNotFound, "mcp_oauth_disabled",
			"This server's MCP endpoint does not take SSO access tokens; use a personal API key", nil)
		return
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "public, max-age=300")
	writer.Header().Set("Access-Control-Allow-Origin", "*")
	writer.Header().Del("Access-Control-Allow-Credentials")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(policy.Metadata(s.authPublic.Issuer))
}

// mcpChallenge is the WWW-Authenticate value of a 401 at /mcp: the bare
// realm while SSO is off, and the pointer to the resource metadata while it
// is on. Only the MCP endpoint carries the pointer — on a REST 401 it would
// send browsers and other clients somewhere they have no business going.
func (s *Server) mcpChallenge(request *http.Request, refused error) string {
	policy, active := s.mcpOAuthActive(request.Context())
	if !active {
		return ""
	}
	return policy.Challenge(refused != nil)
}

// writeMCPAuthError is the MCP endpoint's refusal. An SSO token refused with a
// reason carries that reason: the reader is the operator wiring Keycloak,
// and which claim was wrong and what to put where is the whole answer.
func writeMCPAuthError(writer http.ResponseWriter, request *http.Request, status int, code string, err error) {
	message := http.StatusText(status)
	var refusal *auth.Refusal
	if errors.As(err, &refusal) && refusal.Message != "" {
		message = refusal.Message
	}
	writeError(writer, request, status, code, message, nil)
}

// adminMCPOAuth handles GET /api/v1/admin/mcp/oauth: the connection values
// an administrator copies into an MCP client, and whether SSO is in force
// right now with the reason when it is not. Nothing here is a credential.
func (s *Server) adminMCPOAuth(writer http.ResponseWriter, request *http.Request) {
	policy := s.mcpOAuth(request.Context())
	active, reason := false, ""
	switch {
	case !s.authPublic.OIDCEnabled:
		reason = "no OIDC issuer is running; save the Keycloak issuer in the OIDC settings and restart"
	case s.mcpHandler == nil:
		reason = "the MCP endpoint is not served"
	default:
		active, reason = policy.Active(s.authPublic.Issuer)
	}
	metadataURL := ""
	if policy.Resource != "" {
		metadataURL = policy.MetadataURL()
	}
	writeData(writer, request, http.StatusOK, map[string]any{
		"enabled":     policy.Enabled,
		"active":      active,
		"reason":      reason,
		"issuer":      s.authPublic.Issuer,
		"resource":    policy.Resource,
		"metadataUrl": metadataURL,
		"audience":    append([]string{}, policy.Audiences...),
		"scopes":      append([]string{}, policy.Scopes...),
	})
}
