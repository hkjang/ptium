// Package mcpoauth lets an MCP client in with a Keycloak access token instead
// of a personal key.
//
// The MCP authorization specification (2025-06-18 and later) is OAuth 2.1:
// this server is a resource server that publishes where its authorization
// server is (RFC 9728, /.well-known/oauth-protected-resource), refuses a
// client with a 401 that points there, and checks the token the client comes
// back with. Nothing about issuing tokens happens here — Keycloak does that.
//
// The personal key stays. A token from SSO is a second door into the same
// room: it authenticates an account that already exists, carries the scopes
// the administrator chose, and is held to the same scope checks a key is. It
// never creates an account and never raises anyone's role.
package mcpoauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// The setting keys, the same in every service in the company so an operator
// learns them once. The issuer and client come from the web sign-in's own
// settings and are not repeated here.
const (
	SettingEnabled  = "mcp.oauth.enabled"
	SettingResource = "mcp.oauth.resource"
	SettingAudience = "mcp.oauth.audience"
	SettingScopes   = "mcp.oauth.scopes"

	// Path is where the MCP endpoint is served, and the tail of the resource
	// identifier a token's audience must name.
	Path = "/mcp"
	// MetadataPath is RFC 9728's well-known document for this resource.
	MetadataPath = "/.well-known/oauth-protected-resource"

	// Realm is what the MCP endpoint's challenge names.
	Realm = "ptium-mcp"
	// ResourceName is how the metadata document introduces this server.
	ResourceName = "Ptium MCP"
)

// DefaultScopes is what an SSO token may do unless the administrator says
// otherwise: read. Writing is a deliberate choice, made once in the settings.
var DefaultScopes = []string{"presentations:read", "templates:read"}

// Reader is the little of the settings service this needs.
type Reader interface {
	Get(ctx context.Context, key string, target any) error
}

// Policy is what the administrator decided, read fresh for each request so a
// saved setting is in force on the next call rather than the next restart.
type Policy struct {
	// Enabled is the administrator's switch.
	Enabled bool
	// Resource is the identifier this deployment claims (RFC 8707): the public
	// address plus /mcp. Empty means none can be built, and the policy is off
	// whatever the switch says.
	Resource string
	// Audiences are the values an administrator accepts in a token's aud or
	// azp besides the resource itself — the MCP client's id, in practice.
	Audiences []string
	// Scopes are what a valid token may do.
	Scopes []string
}

// Read maps the stored settings onto a policy. A value that cannot be read is
// left at what the product ships, so a settings outage never turns SSO on.
// publicBaseURL is the operator's PUBLIC_BASE_URL, the resource identifier's
// source when the setting is empty.
func Read(ctx context.Context, reader Reader, publicBaseURL string) Policy {
	policy := Policy{Scopes: append([]string(nil), DefaultScopes...)}
	if reader == nil {
		return policy
	}
	var enabled bool
	if reader.Get(ctx, SettingEnabled, &enabled) == nil {
		policy.Enabled = enabled
	}
	var resource string
	if reader.Get(ctx, SettingResource, &resource) == nil {
		policy.Resource = strings.TrimSpace(resource)
	}
	if policy.Resource == "" {
		if base := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/"); base != "" {
			policy.Resource = base + Path
		}
	}
	var audience string
	if reader.Get(ctx, SettingAudience, &audience) == nil {
		policy.Audiences = SplitList(audience)
	}
	var scopes string
	if reader.Get(ctx, SettingScopes, &scopes) == nil {
		if listed := SplitList(scopes); len(listed) > 0 {
			policy.Scopes = listed
		}
	}
	return policy
}

// FromValues maps stored JSON values onto a policy, for the write path
// checking what a save would leave behind.
func FromValues(values map[string]json.RawMessage, publicBaseURL string) Policy {
	return Read(context.Background(), valueReader(values), publicBaseURL)
}

type valueReader map[string]json.RawMessage

func (v valueReader) Get(_ context.Context, key string, target any) error {
	raw, ok := v[key]
	if !ok {
		return fmt.Errorf("setting %q is not set", key)
	}
	return json.Unmarshal(raw, target)
}

// SplitList reads a space-, comma- or line-separated list, without repeats.
func SplitList(value string) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, item := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ';'
	}) {
		if _, duplicate := seen[item]; duplicate {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

// Active reports whether SSO tokens are taken at all, and when not, why —
// the reason is for the log, so an administrator who turned the switch on and
// sees nothing happen can read what is missing.
func (policy Policy) Active(issuer string) (bool, string) {
	if !policy.Enabled {
		return false, "mcp.oauth.enabled is off"
	}
	if strings.TrimSpace(issuer) == "" {
		return false, "no OIDC issuer is configured; the web sign-in's auth.oidc.issuer_url (or OIDC_ISSUER_URL) is what tokens are checked against"
	}
	if policy.Resource == "" {
		return false, "no resource identifier: set mcp.oauth.resource or PUBLIC_BASE_URL"
	}
	return true, ""
}

// MetadataURL is where a refused client is sent to learn the above: the
// well-known document beside the resource, suffixed with the resource's path
// as RFC 9728 has it.
func (policy Policy) MetadataURL() string {
	base := strings.TrimSuffix(policy.Resource, Path)
	return base + MetadataPath + Path
}

// Metadata is the RFC 9728 document a refused MCP client reads to find the
// authorization server. Public by design — it says where to sign in, not who
// is signed in — and served bare, not in the product's envelope: the reader
// is an OAuth client library that knows nothing about {data: …}.
func (policy Policy) Metadata(issuer string) map[string]any {
	return map[string]any{
		"resource":                 policy.Resource,
		"authorization_servers":    []string{issuer},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         append([]string(nil), policy.Scopes...),
		"resource_name":            ResourceName,
	}
}

// Challenge is the WWW-Authenticate value that turns a 401 into an
// invitation: the MCP client reads resource_metadata and starts the OAuth flow
// from there. Without it a refusal is a dead end. rejected says whether a
// credential came and was refused, which RFC 6750 has the header say.
func (policy Policy) Challenge(rejected bool) string {
	value := fmt.Sprintf(`Bearer realm=%q, resource_metadata=%q`, Realm, policy.MetadataURL())
	if rejected {
		value += `, error="invalid_token"`
	}
	return value
}

// ValidateResource says whether a value may be stored as the resource
// identifier: empty, or an absolute HTTPS address (HTTP only on the loopback
// host) ending in /mcp with no credentials, query or fragment. A client
// compares this against the address it connected to, so an identifier that
// is not that address is one no token will ever match.
func ValidateResource(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.ForceQuery {
		return errors.New("the MCP resource identifier must be an absolute URL without credentials, query or fragment")
	}
	loopback := parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1"
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
		return errors.New("the MCP resource identifier must use HTTPS (HTTP only on localhost)")
	}
	if parsed.Path != Path {
		return fmt.Errorf("the MCP resource identifier must end in %s, the path MCP clients connect to", Path)
	}
	return nil
}

// ValidateAudience says whether a value may be stored as the accepted
// audiences: up to 50 client ids or identifiers of printable characters.
func ValidateAudience(value string) error {
	listed := SplitList(value)
	if len(listed) > 50 {
		return errors.New("at most 50 accepted audiences can be listed")
	}
	for _, item := range listed {
		if len(item) > 256 {
			return errors.New("an accepted audience must be at most 256 characters")
		}
		for _, r := range item {
			if r < 0x21 || r > 0x7e {
				return fmt.Errorf("accepted audience %q must be printable ASCII without spaces", item)
			}
		}
	}
	return nil
}

// Validate refuses a switch that would do nothing: SSO turned on with no
// issuer to check tokens against or no identifier for them to name. The
// issuer may be the one running or the one just stored — the latter is in
// force at the next restart, and the switch may wait with it.
func (policy Policy) Validate(issuer string) error {
	if !policy.Enabled {
		return nil
	}
	if strings.TrimSpace(issuer) == "" {
		return errors.New("MCP SSO needs an OIDC issuer: set the Keycloak issuer in the OIDC settings first")
	}
	if policy.Resource == "" {
		return errors.New("MCP SSO needs a resource identifier: set mcp.oauth.resource (https://…/mcp) or PUBLIC_BASE_URL")
	}
	if err := ValidateResource(policy.Resource); err != nil {
		return err
	}
	if len(policy.Scopes) == 0 {
		return errors.New("MCP SSO needs at least one scope for SSO tokens to hold")
	}
	return nil
}
