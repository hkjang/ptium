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

## Local password sign-in

`BOOTSTRAP_ADMIN` and `BOOTSTRAP_ADMIN_PASSWORD` seed an administrator that signs
in with a password, so a deployment is administrable before an identity provider
exists. The design keeps that convenience from becoming a weakness:

- The password is read from the environment only. It is never written to the
  settings table, never logged and never returned by any endpoint.
- It is hashed with bcrypt at cost 12. Sign-in is interactive and rare, so a slow
  hash costs the deployment nothing and costs an attacker a great deal.
- The password is written when the account is created and then left alone, so a
  password changed in the product is not reset by the next restart.
  `BOOTSTRAP_ADMIN_PASSWORD_RESET=true` overwrites it for one start, which is the
  documented recovery path for a forgotten password.
- An unknown username costs the same as a wrong password: the comparison runs
  against a decoy hash, so response time does not reveal which accounts exist.
  The client is told only that the username or password is incorrect.
- Failed attempts back off per client address, doubling from two seconds to a
  five-minute ceiling. The limiter is in-process, so replicas throttle
  independently — it raises the cost of guessing rather than making it
  impossible, and bcrypt is what makes each attempt expensive.
- Sign-in issues a stateless session token, prefixed `ptses_` so it is never
  mistaken for an API key. It carries the account id, an expiry and the
  account's password-change timestamp; roles are read from the database on every
  request, so revoking an administrator takes effect immediately rather than at
  token expiry.
- Changing a password moves that timestamp forward, which invalidates every token
  issued before it. Other browsers are signed out; the browser that made the
  change is handed a fresh token.

## Confidential OIDC clients

`OIDC_CLIENT_SECRET` (environment, or the write-only `auth.oidc.client_secret`
setting) turns the browser flow into a confidential-client flow. The secret must
never reach a single-page app, so Ptium performs the authorization-code exchange
itself at `POST /api/v1/auth/token` and reports that URL in the public auth
config. The secret is presented with HTTP basic authentication rather than in the
request body, keeping it out of provider access logs, and the refresh token the
provider returns is deliberately not forwarded to the browser.

Without a secret the flow is unchanged: a public client exchanges the code
directly with the provider using PKCE.

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

- Change the bootstrap administrator password after the first sign-in, and remove
  `BOOTSTRAP_ADMIN_PASSWORD` from the environment once an identity provider is
  configured.

- Set `DEV_AUTH_ENABLED=false`.
- Use an HTTPS `PUBLIC_BASE_URL` and strict `CORS_ALLOWED_ORIGINS`.
- Configure Keycloak redirect and web origins exactly; use Authorization Code +
  PKCE for the public SPA client.
- Remove bootstrap administrator selectors after the first successful login.
- Replace example database passwords and require TLS for remote PostgreSQL.
- Rotate initial AI/API credentials, validate expiry, and test revocation.
- Back up PostgreSQL and test restore before inviting users.

