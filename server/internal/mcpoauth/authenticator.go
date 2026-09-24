package mcpoauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/hkjang/ptium/server/internal/auth"
)

// AuthMethod is what a principal that came in through SSO says it is. The
// scope checks read it: a token is bounded by its scopes the way a key is.
const AuthMethod = "mcp_oauth"

// Provider verifies what any token from the identity provider must satisfy —
// signature, issuer, validity window — and says which issuer that is. The
// web sign-in's authenticator is one; the same discovery and key cache serve
// both doors.
type Provider interface {
	VerifySignedToken(ctx context.Context, token string) (map[string]any, error)
	Issuer() string
}

// Account is what the accounts store knows about the person a token names.
type Account struct {
	ID       string
	Email    string
	Name     string
	Roles    []string
	Admin    bool
	Disabled bool
}

// ErrNoAccount is what Accounts answers for a subject nobody registered.
var ErrNoAccount = errors.New("no account is registered to this subject")

// Accounts finds — only finds — the account a provider subject was
// registered to when its holder signed in to the web.
type Accounts interface {
	AccountBySubject(ctx context.Context, subject string) (Account, error)
}

// AccountsFunc adapts a lookup function.
type AccountsFunc func(context.Context, string) (Account, error)

func (fn AccountsFunc) AccountBySubject(ctx context.Context, subject string) (Account, error) {
	return fn(ctx, subject)
}

// Authenticator turns a bearer access token into a principal, or says exactly
// why it will not. It answers ErrNoCredentials for anything that is not a
// token of the shape it takes, so the key authenticator behind it in the
// chain sees a key — or a bad key — exactly as it did before.
type Authenticator struct {
	Provider Provider
	// Policy is read for every token: what the administrator saved is in
	// force on the next call.
	Policy   func(context.Context) Policy
	Accounts Accounts
	Logger   *slog.Logger
}

// LooksLikeJWT is the cheap shape test that separates "not a key" from "not
// a token of any kind we accept".
func LooksLikeJWT(token string) bool {
	parts := strings.Split(token, ".")
	return len(parts) == 3 && parts[0] != "" && parts[1] != "" && parts[2] != ""
}

func (authenticator *Authenticator) Authenticate(ctx context.Context, request *http.Request) (*auth.Principal, error) {
	token, ok := bearer(request)
	if !ok || strings.HasPrefix(token, "dev:") || !LooksLikeJWT(token) {
		return nil, auth.ErrNoCredentials
	}
	if authenticator.Provider == nil || authenticator.Policy == nil || authenticator.Accounts == nil {
		return nil, errors.New("MCP OAuth authenticator is not initialized")
	}
	policy := authenticator.Policy(ctx)
	issuer := authenticator.Provider.Issuer()
	if active, reason := policy.Active(issuer); !active {
		if policy.Enabled && authenticator.Logger != nil {
			// The switch is on and nothing happens: say what is missing.
			authenticator.Logger.Warn("MCP SSO is switched on but cannot take tokens", "reason", reason)
		}
		return nil, &auth.Refusal{
			Cause:   fmt.Errorf("sso token refused: %s", reason),
			Message: "This server does not take SSO access tokens at /mcp. Use a personal API key (ptium_…), or ask an administrator to turn on MCP SSO (mcp.oauth.enabled).",
		}
	}

	claims, err := authenticator.Provider.VerifySignedToken(ctx, token)
	if err != nil {
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			// Keys could not be fetched: the provider's problem, not the token's.
			return nil, err
		}
		return nil, &auth.Refusal{
			Cause:   err,
			Message: "The SSO access token was not accepted (signature, issuer or validity). Sign in again from the client.",
		}
	}
	// An ID token is proof that somebody signed in, not a credential for an
	// API; Keycloak marks one typ=ID. A token bound to a key this server
	// cannot check (cnf: DPoP or mTLS) is not one to accept as a bearer.
	if typ, _ := claims["typ"].(string); strings.EqualFold(typ, "ID") {
		return nil, &auth.Refusal{
			Cause:   errors.New("sso token refused: an ID token was presented"),
			Message: "An ID token was presented. Send the access token — an ID token is proof of sign-in, not an API credential.",
		}
	}
	if _, bound := claims["cnf"]; bound {
		return nil, &auth.Refusal{
			Cause:   errors.New("sso token refused: token carries a cnf claim"),
			Message: "The token is bound to a key (cnf) this server cannot verify. Send a plain bearer access token.",
		}
	}
	subject, _ := claims["sub"].(string)
	if strings.TrimSpace(subject) == "" {
		return nil, &auth.Refusal{
			Cause:   errors.New("sso token refused: no subject"),
			Message: "The token names no subject (sub).",
		}
	}

	// Whom the token was minted for. Measured against a real Keycloak 26: an
	// access token issued to a client carries that client in azp and
	// aud=["account"] — the client id is not in aud, whatever an ID token
	// does. So the binding checked is "aud names this resource, or aud or azp
	// is one the administrator listed". Either is the token being for this
	// deployment rather than passed through from some other application in
	// the realm, which is what RFC 8707 and the MCP specification guard
	// against. The plain path — put the MCP client's id in the list — needs
	// no mapper at all.
	audiences := claimStrings(claims["aud"])
	party, _ := claims["azp"].(string)
	if !forThisServer(policy, audiences, party) {
		fix := fmt.Sprintf("add the client id to mcp.oauth.audience, or give the Keycloak client an Audience mapper for %q", policy.Resource)
		if party != "" {
			fix = fmt.Sprintf("add %q to mcp.oauth.audience, or give the Keycloak client an Audience mapper for %q", party, policy.Resource)
		}
		return nil, &auth.Refusal{
			Cause: fmt.Errorf("sso token refused: audience %v / azp %q is not this server (%s)", audiences, party, policy.Resource),
			Message: fmt.Sprintf("The token was not issued for this server (aud=[%s], azp=%q): %s.",
				strings.Join(audiences, " "), party, fix),
		}
	}

	// The same lookup the web sign-in does, without the provisioning half.
	account, err := authenticator.Accounts.AccountBySubject(ctx, subject)
	if errors.Is(err, ErrNoAccount) {
		return nil, &auth.Refusal{
			Cause:   fmt.Errorf("sso token refused: no account for subject %q", subject),
			Message: "This SSO account is not registered here. Sign in to the web once first, then connect.",
		}
	}
	if err != nil {
		return nil, fmt.Errorf("look up the SSO account: %w", err)
	}
	if account.Disabled {
		return nil, &auth.Refusal{
			Cause:   fmt.Errorf("sso token refused: account %s is disabled", account.ID),
			Message: "This account has been disabled.",
		}
	}

	roles := append([]string(nil), account.Roles...)
	if account.Admin && !contains(roles, "ptium-admin") {
		roles = append(roles, "ptium-admin")
	}
	return &auth.Principal{
		Subject:    "mcp-oauth:" + account.ID,
		Email:      account.Email,
		Name:       account.Name,
		Issuer:     issuer,
		Roles:      roles,
		Scopes:     grantedScopes(policy, claims),
		AuthMethod: AuthMethod,
		Claims:     map[string]any{"ptium_user_id": account.ID, "azp": party},
	}, nil
}

// forThisServer is the audience rule: the resource in aud, or a listed value
// in aud or azp.
func forThisServer(policy Policy, audiences []string, party string) bool {
	for _, audience := range audiences {
		if audience != "" && (audience == policy.Resource || contains(policy.Audiences, audience)) {
			return true
		}
	}
	return party != "" && contains(policy.Audiences, party)
}

// grantedScopes is what the administrator allows, narrowed to what the token
// carries when the token speaks this product's scope vocabulary at all. A
// Keycloak token normally says "openid profile email", which names none of
// them, and then the administrator's list is the whole grant. mcp:use is
// implied: an administrator who turned SSO on for the MCP endpoint has
// granted the MCP endpoint.
func grantedScopes(policy Policy, claims map[string]any) []string {
	carried := map[string]struct{}{}
	for _, key := range []string{"scope", "scp"} {
		for _, scope := range claimStrings(claims[key]) {
			carried[scope] = struct{}{}
		}
	}
	overlap := false
	for _, scope := range policy.Scopes {
		if _, ok := carried[scope]; ok {
			overlap = true
			break
		}
	}
	granted := []string{"mcp:use"}
	for _, scope := range policy.Scopes {
		if _, ok := carried[scope]; ok || !overlap {
			if !contains(granted, scope) {
				granted = append(granted, scope)
			}
		}
	}
	return granted
}

func bearer(request *http.Request) (string, bool) {
	values := request.Header.Values("Authorization")
	if len(values) != 1 {
		return "", false
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	return parts[1], true
}

func claimStrings(value any) []string {
	switch typed := value.(type) {
	case string:
		return strings.Fields(typed)
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
