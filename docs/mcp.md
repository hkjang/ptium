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
| `ptium.list_presentations` | optional `limit`, `offset` | List owner-visible decks. |
| `ptium.get_presentation` | `id` | Read a deck and its generated slides. |
| `ptium.create_presentation` | `title`, `prompt`; optional `templateId`, `theme`, `language`, `audience`, `tone`, `slideCount` | Create an owner-scoped draft. Omitted options use administrator generation defaults; an omitted `templateId` selects the built-in design matching `theme`. |
| `ptium.generate_presentation` | `id` | Queue generation or regeneration. |
| `ptium.list_templates` | optional `limit`, `offset` | List the PowerPoint templates the user may generate into, with each layout's role and text capacity. Call this first to pass a deliberate `templateId`. |

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
      "theme": "aurora",
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
