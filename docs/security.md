# Security model

## Identity and roles

Ptium validates OIDC access tokens using the issuer's discovery document and JWKS.
The issuer and audience/client ID are checked; signing algorithms are restricted
to the provider metadata. Keycloak realm or client roles listed in
`OIDC_ADMIN_ROLES` map to the Ptium `admin` role. `BOOTSTRAP_ADMIN_EMAILS` and
`BOOTSTRAP_ADMIN_SUBJECTS` are narrowly scoped escape hatches for the first login
and should be removed after administrators are established.

The React application's route guards are only a usability layer. The API repeats
all authorization checks. Users can access their own profile, API keys and decks;
administrators can access system settings, user controls and incidents.

Development authentication is disabled by default. If enabled, it additionally
requires `DEV_AUTH_SECRET` and must never be exposed on a public deployment.

## API keys and rotation

Ptium API keys are high-entropy bearer credentials. Only a prefix and a
cryptographic hash are stored; the complete value is shown once at creation.
Keys have explicit scopes and optional expiry. Rotation creates a new key and
starts a configurable grace period for the predecessor so unattended MCP and API
clients can migrate without downtime. Revocation is immediate and auditable.

Recommended operational procedure:

1. Rotate the credential and capture the new one-time value.
2. Update every client during the overlap window.
3. Verify the new key's last-used timestamp.
4. Revoke the predecessor early, or let the grace period expire automatically.

## Managed secrets

AI provider credentials and OIDC client secrets are accepted only through the
administrator setting endpoint. Reads return a `configured` marker rather than
the value. Logs, incidents and API errors pass through redaction for authorization
headers, cookies, passwords, tokens, secrets and provider keys.

Use TLS at the ingress, a PostgreSQL account dedicated to Ptium, encrypted database
storage/backups and short-lived OIDC access tokens. Restrict `/mcp` and `/api/v1`
with the same ingress policy; MCP is not a privileged bypass.

## Incident handling

Unhandled server errors are assigned a stable fingerprint so repeated occurrences
can be triaged as one problem while retaining occurrence counts and timestamps.
The administrator console exposes filtering and state changes (`open`,
`acknowledged`, `resolved`). It does not expose bearer credentials or full request
bodies. A request ID is safe to give to support and correlates the client response,
structured log and incident record.

## Production checklist

- Set `DEV_AUTH_ENABLED=false`.
- Use an HTTPS `PUBLIC_BASE_URL` and strict `CORS_ALLOWED_ORIGINS`.
- Configure Keycloak redirect and web origins exactly; use Authorization Code +
  PKCE for the public SPA client.
- Remove bootstrap administrator selectors after the first successful login.
- Replace example database passwords and require TLS for remote PostgreSQL.
- Rotate initial AI/API credentials, validate expiry, and test revocation.
- Back up PostgreSQL and test restore before inviting users.

