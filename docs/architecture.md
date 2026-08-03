# Ptium architecture

Ptium is a single-tenant deployable, multi-user AI presentation workspace. The
runtime deliberately has one hard infrastructure dependency: PostgreSQL. The Go
API applies its own idempotent schema migrations and keeps service configuration,
identity mappings, decks, generated slides, API credentials and incidents in the
same database. An external queue, cache or object store can be added later, but is
not required to start or operate the baseline service.

```text
Browser / API client / MCP client
                |
        React single-page app
                |
       /api/v1 REST   /mcp JSON-RPC
                |
            Go service
       +--------+---------+
       |                  |
OIDC discovery/JWKS   AI provider adapter
       |                  |
   Keycloak       OpenAI-compatible endpoint
       \                  /
             PostgreSQL
      (settings, decks, slides, template packages)
```

## Runtime boundaries

- `web/` is a Vite/React/TypeScript SPA. It owns the presentation workspace and
  the separately gated administrator console.
- `server/` is a Go HTTP service. It owns authentication, authorization,
  generation, persistence, REST, MCP, key lifecycle and incident collection.
- `api/openapi.yaml` is the stable public REST contract. MCP is described in
  `docs/mcp.md`.
- `docker-compose.yml` is a local reference deployment, not a requirement. A
  compiled process can start with only `DATABASE_URL`.
- The release image is one static binary that also serves the compiled
  workspace, so a pod is a single container on a single port. No reverse proxy
  and no database are bundled; `deploy/kubernetes.yaml` is the reference
  manifest.

## Request lifecycle

Every API request receives a request ID. The authentication middleware accepts
either a verified OIDC access token or a hashed Ptium API key. The authorization
layer resolves ownership, role and key scope before the handler reaches storage.
Unexpected failures are returned as a stable JSON error and recorded as an
incident with a fingerprint, request ID, severity, redacted context and lifecycle
state. Administrators can acknowledge, resolve or reopen the incident without
access to secrets.

Presentation generation is persisted as an explicit state transition:

```text
draft -> queued -> generating -> ready
                         \-> failed -> queued (retry)
```

Generation is template-aware from end to end. Every deck is bound to a
PowerPoint template — one the customer uploaded or one of the designs Ptium
ships with — and the template's layout catalogue drives both writing and
export.

```text
upload .pptx/.potx
        |
   analyzer  ->  manifest: slide size, theme colours and fonts, per-layout
        |        placeholders with role, position and text capacity
        |
   plan pass  ->  narrative arc, one layout chosen per slide
        |
   write pass ->  copy written into named slots inside their budgets
        |
   composer  ->  validates slots, repairs unknown layouts, fits overflow
        |
   renderer  ->  original package + newly generated slide parts
```

If an administrator configured an OpenAI-compatible provider, the two passes run
against it. Otherwise Ptium uses its deterministic local generator, which builds
the same narrative arc and layout variety without a network call, so onboarding
and air-gapped deployments remain fully usable.

## Template rendering

`internal/pptx` reads and writes Office Open XML directly; no PowerPoint or
LibreOffice process is involved. Export clones the stored package, removes only
the slide and notes-slide parts, and writes new slides whose shapes are
placeholder references — `<p:spPr/>` is left empty so position, size, font,
colour and bullet styling are inherited from the customer's own layout. Masters,
layouts, theme, fonts, media and every other part are carried across byte for
byte, which is what makes an exported deck indistinguishable from one authored
by hand in the same template.

Text that would overflow its box is measured in em units — a Hangul or Kanji
glyph occupies a full em where Latin letters average about half of one — and
given a precomputed `normAutofit` scale, so the file looks right before it is
ever opened. The same measurement renders the SVG previews the workspace shows,
which is why the browser preview and the exported file agree.

## Configuration precedence

Environment variables exist for process bootstrap only: database connectivity,
listen address, initial OIDC discovery and the first administrator claim. Mutable
product settings live in PostgreSQL and are exposed through the administrator
console/API. Database values take effect without rebuilding the frontend; secret
values are write-only and redacted on reads.

An environment-supplied OIDC issuer is used when no database setting exists. Once
an administrator saves OIDC settings, the service refreshes discovery and JWKS
from the configured issuer. Keycloak-specific hard-coded URLs are unnecessary.

## Extension points

- AI providers implement the OpenAI-compatible chat-completions boundary.
- Templates are opaque to the rest of the service: anything the analyzer can
  describe as layouts and placeholders can be generated into, so new template
  sources need no change to generation or export.
- Theme, tone, language and audience are stored as presentation inputs and user
  defaults, allowing additional renderers later.
- REST uses versioned paths. MCP tools call the same application services instead
  of bypassing authorization or persistence.
- API-key hashing and versioned encryption metadata allow stronger key providers
  or a KMS integration without changing clients.

