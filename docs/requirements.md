# Objective traceability

This document turns the product request into verifiable acceptance criteria. It is
also the completion checklist used before a Ptium release is tagged.

| Requirement | Acceptance evidence |
| --- | --- |
| AI presentation service named Ptium | Branded React workspace can create a prompt-driven deck and inspect/edit generated slides; generation persists in PostgreSQL. |
| Generates into the customer's own PowerPoint template | Uploading a `.pptx`/`.potx` catalogues its masters, layouts, theme and placeholder capacities; generated decks bind each slide to a real layout and export reuses the original package unchanged (`go test ./internal/pptx`, `./internal/export`). |
| Professional output without an AI credential | The deterministic generator produces a full narrative arc with layout variety, per-slide speaker notes and language-correct copy; text is measured and auto-fitted so nothing overflows its box. |
| Go and React | `go test ./...`, Go binary build, frontend tests and Vite production build pass. |
| PostgreSQL DSN-only startup | With only `DATABASE_URL` set, the service starts, applies migrations, passes readiness and can use deterministic generation. |
| Simple Keycloak SSO/OIDC | Issuer + client ID bootstrap discovery/JWKS works, SPA uses Authorization Code + PKCE, roles map to user/admin. |
| Personalization and admin separation | User profile/defaults and user workspace are owner-scoped; admin routes and API policy are independently gated. |
| All service settings manageable by admin | Admin UI/API covers branding, AI provider, OIDC, generation defaults and security policy; secrets are write-only. |
| Server errors manageable by admin | Request IDs, persisted/fingerprinted incidents, filters, acknowledge/resolve/reopen operations and admin UI exist. |
| REST API and MCP | Versioned OpenAPI contract and authenticated MCP JSON-RPC endpoint exercise shared application services. |
| Key management including rotation | Scoped hashed credentials support create/list/rotate with overlap, expiry, last-used tracking and revoke. |
| Operable delivery | Health/readiness, structured logs, containers, sample config, architecture/security/runbook docs and automated verification exist. |

