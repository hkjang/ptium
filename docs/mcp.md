# Ptium MCP server

Ptium exposes a stateless MCP Streamable HTTP endpoint at `/mcp`. It uses the
same identity, ownership and incident pipeline as REST. Create an API key with
the `mcp:use` scope (and presentation read/write scopes required by the chosen
operation), then configure a Streamable HTTP-capable MCP client.

```json
{
  "mcpServers": {
    "ptium": {
      "type": "streamable-http",
      "url": "https://ptium.example.com/mcp",
      "headers": {
        "Authorization": "Bearer ptium_REPLACE_ME"
      }
    }
  }
}
```

The complete key is displayed only once. Rotate it from the developer settings
screen before expiry; both credentials work during the configured overlap.

## Connecting with SSO, without a key

A deployment whose administrator turned on **MCP · SSO** takes a Keycloak
access token at `/mcp` as well as a personal key. Give the client the URL and
nothing else:

```json
{
  "mcpServers": {
    "ptium": {
      "type": "streamable-http",
      "url": "https://ptium.example.com/mcp"
    }
  }
}
```

The client is refused once with a `401` whose `WWW-Authenticate` names
`https://ptium.example.com/.well-known/oauth-protected-resource/mcp`; it reads
that document, sends you through Keycloak (PKCE, no secret), and comes back
with a token whose audience is this server. If you are already signed in to
Keycloak you barely see a screen.

What the token opens is **your existing account**: sign in to the Ptium web
once first, or the server answers "This SSO account is not registered here".
The token does not create an account, does not revive a disabled one, and
grants what the administrator listed in `mcp.oauth.scopes` (reading decks
and templates, as shipped) rather than what a key of yours might hold; a tool
outside that answers `403 insufficient_scope`. A 401 whose message says the
token "was not issued for this server" is the administrator's to fix — it
names the `azp` to allow — not yours.

Keys keep working exactly as above, on the same header. A script on a closed
network, or a deployment without Keycloak, uses a key; a person at a desk uses
SSO. The server does not check tokens back with Keycloak, so a token already
issued lives until it expires (minutes) after you sign out there.

## Protocol and transport

- JSON-RPC 2.0 over HTTP `POST /mcp`.
- Protocol revisions `2025-03-26`, `2025-06-18` and `2025-11-25` are accepted.
- Set `Content-Type: application/json` and, for current clients,
  `MCP-Protocol-Version: 2025-11-25`.
- Sessions and server-sent event streams are intentionally unnecessary for the
  stateless operations. `GET /mcp` returns diagnostic metadata; an SSE-only GET
  returns `405`.
- Maximum request body is 1 MiB and application operations time out after 30
  seconds by default.

Example initialization:

```bash
curl https://ptium.example.com/mcp \
  -H 'Authorization: Bearer ptium_REPLACE_ME' \
  -H 'Content-Type: application/json' \
  -H 'MCP-Protocol-Version: 2025-11-25' \
  --data '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"example","version":"1.0"}}}'
```

## Tools

| Tool | Required arguments | Purpose |
| --- | --- | --- |
| `ptium.list_presentations` | optional `q`, `limit`, `offset` | List owner-visible decks, newest first. `q` narrows to decks whose title or brief contains that text — an account holds thousands, and paging through them is not finding one. |
| `ptium.get_presentation` | `id` | Read a deck and its generated slides. |
| `ptium.create_presentation` | `title`, `prompt`; optional `templateId`, `theme`, `language`, `audience`, `tone`, `slideCount` | Create an owner-scoped draft. Omitted options use administrator generation defaults; an omitted `templateId` selects the built-in design matching `theme`. |
| `ptium.generate_presentation` | `id` | Queue generation or regeneration. **Returns when the work is queued, not when the deck is written**: poll `ptium.get_presentation` until `status` reads `completed` or `failed`, rather than giving up after a set time. The built-in generator answers in seconds; a self-hosted model with repair passes can take tens of minutes, and a deck still being written is not a deck in trouble. `queued` means no worker has picked it up yet, `generating` means one is writing it. |
| `ptium.list_templates` | optional `limit`, `offset` | List the PowerPoint templates the user may generate into, with each layout's role and text capacity. Call this first to pass a deliberate `templateId`. |

Making a deck is two steps, and the second one is asynchronous: create returns a
draft with no slides, generate queues the writing, and get reports `status` until
it settles. An agent that fetches the deck straight after generating finds it
empty and reports a failure that has not happened.

`tools/call` returns both text content and `structuredContent`. Expected
validation and ownership failures use `isError: true`; unexpected server errors
are redacted from the client and captured in the Ptium administrator incident
console.

Example create call:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "ptium.create_presentation",
    "arguments": {
      "title": "2027 Product Strategy",
      "prompt": "Create an executive decision deck for regional leaders.",
      "language": "en",
      "theme": "midnight-wash",
      "slideCount": 10
    }
  }
}
```

## Resources

`resources/list` exposes owner-visible decks as `ptium://presentations/<id>`
resources with cursor pagination. `resources/read` returns the normalized deck
JSON including slides. Resources never bypass ownership checks.

## Scope behavior

OIDC browser users are authorized by their Ptium role and ownership. API-key
requests additionally enforce their explicit scopes. MCP access always requires
`mcp:use`; an API key intended to create and generate decks should normally have
`mcp:use`, `presentations:read`, `presentations:write` and `templates:read`.
`ptium.list_templates` requires `templates:read`. Administrator scopes are not
inferred from `mcp:use`.
