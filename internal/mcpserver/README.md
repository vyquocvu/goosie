# Goosie MCP Server

A Model Context Protocol (MCP) server that exposes the Goosie browser engine
as a set of tools for AI assistants like Claude Desktop, Cursor, and other
MCP clients.

| Stdio | HTTP | Hardening |
|:----:|:----:|:---------:|
| ✅ | ✅ | ✅ |

## Overview

The MCP server wraps Goosie's headless browser pipeline behind a clean
JSON-RPC interface. It ships as **both stdio and Streamable HTTP** transports,
backed by the official
[`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk) v1.4.0.

### Architecture

![Architecture](test/mcp-screenshots/png/07_architecture.png)

The protocol layer in `internal/mcpserver` adapts MCP tool calls into the
UI-independent `Service`/`Context` interfaces in `internal/browsercontrol`,
which in turn drive the engine packages (`engine/session`,
`engine/navigation`, `engine/dom`, `js`, `net`). No Fyne or GUI dependency
is introduced to the server.

---

## Server identity

The server identifies itself via `GET /version` (HTTP) and the
`initialize` JSON-RPC response (both transports):

![Server Info](test/mcp-screenshots/png/01_server_info.png)

```json
{
  "name":            "goosie-mcp-server",
  "version":         "1.0.0-alpha",
  "protocolVersion": "2025-11-25",
  "goVersion":       "go1.25",
  "os":              "darwin",
  "arch":            "arm64"
}
```

---

## Tool catalog

Ten tools split across four categories: context lifecycle, navigation,
read-only inspection, mutations, JS evaluation, and capture.

![Tool Catalog](test/mcp-screenshots/png/03_tools.png)

| Category | Tools |
|----------|-------|
| **context** | `browser_context_create`, `browser_context_list`, `browser_context_close` |
| **navigate** | `browser_navigate` |
| **read** | `browser_snapshot`, `browser_page_info` |
| **mutate** | `browser_click`, `browser_type` |
| **eval** | `browser_evaluate` |
| **capture** | `browser_screenshot` |

---

## Session example

Every client gets a cryptographic session on `initialize`:

![Initialize](test/mcp-screenshots/png/02_initialize.png)

```http
POST /mcp HTTP/1.1
Content-Type: application/json

{"jsonrpc":"2.0","id":1,"method":"initialize",
 "params":{"protocolVersion":"2025-11-25"}}
```

```http
HTTP/1.1 200 OK
Mcp-Session-Id: a4f1c2e87b1d4539c2e8b1d4539c2e8b1...
Content-Type: application/json

{"jsonrpc":"2.0","id":1,"result":{
  "protocolVersion":"2025-11-25",
  "serverInfo":{"name":"goosie-mcp-server","version":"1.0.0-alpha"},
  "capabilities":{"tools":{},"resources":{}}}}
```

Subsequent requests carry the `Mcp-Session-Id` header and are rate-limited
per session.

---

## Health & metrics

`GET /health` returns liveness plus a live metrics snapshot:

![Health](test/mcp-screenshots/png/04_health.png)

```json
{
  "healthy": true,
  "reason":  "ok",
  "metrics": {
    "startedAt":         "2026-07-28T14:30:00Z",
    "uptimeSeconds":     4281,
    "totalRequests":     412,
    "totalErrors":       3,
    "totalTimeouts":     1,
    "totalDenied":       7,
    "activeContexts":    2,
    "maxContexts":       100,
    "memoryAllocBytes":  12976128,
    "goroutines":        9,
    "gcRuns":            14
  }
}
```

---

## Hardening

![Hardening](test/mcp-screenshots/png/05_hardening.png)

The server exposes production hardening as first-class features:

| Component | What it does |
|-----------|--------------|
| **Audit logger** | Single-line JSON to stderr; redactable keys (`password`, `secret`, `token`, `cookie`, `authorization`) |
| **Rate limiter** | Token bucket: 100 burst, 50 tokens/sec/process |
| **Quotas** | Per-context: 100 MB memory, 10 000 requests, 1 000 navigations, 100 screenshots |
| **Health reporter** | Memory, goroutines, GC, error/timeout counters |
| **Shutdown** | SIGTERM/SIGINT handlers; audit logger closes last |
| **Loopback binding** | HTTP transport rejects non-loopback binds at startup |

Sample audit trail (real shape):

```jsonl
{"ts":"2026-07-28T14:32:11Z","type":"tool_call","contextId":"ctx_a3f1c2e8","tool":"browser_navigate","outcome":"success","durationMs":412}
{"ts":"2026-07-28T14:32:15Z","type":"tool_call","contextId":"ctx_a3f1c2e8","tool":"browser_snapshot","outcome":"success","durationMs":89}
{"ts":"2026-07-28T14:32:18Z","type":"tool_call","contextId":"ctx_a3f1c2e8","tool":"browser_screenshot","outcome":"success","durationMs":230}
```

---

## Security enforcement

![Origin Rejected](test/mcp-screenshots/png/06_origin.png)

Cross-origin requests with `Origin: https://evil.example.com` are blocked:

```http
HTTP/1.1 403 Forbidden

invalid Origin header
```

The Origin allowlist defaults to **loopback only** (`localhost`, `127.0.0.1`,
`::1`). Operators wanting broader access must explicitly set
`HTTPConfig.AllowedOrigins`.

---

## Installation & usage

### Build

```bash
go build -o bin/mcp-server ./cmd/mcp-server
./bin/mcp-server --help
```

### Stdio (default)

```bash
./bin/mcp-server
```

Add to Claude Desktop or Cursor:

```json
{
  "mcpServers": {
    "goosie": {
      "command": "/absolute/path/to/bin/mcp-server",
      "env": {}
    }
  }
}
```

### HTTP loopback

```bash
./bin/mcp-server --http --port 8088
```

```bash
# Health
curl http://127.0.0.1:8088/health

# Version
curl http://127.0.0.1:8088/version

# Initialize (creates a session)
curl -X POST http://127.0.0.1:8088/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}'
```

```json
{
  "mcpServers": {
    "goosie-http": {
      "url":    "http://127.0.0.1:8088/mcp",
      "headers": {
        "Authorization": "Bearer ${GOOSIE_TOKEN}"
      }
    }
  }
}
```

### With authentication

```bash
GOOSIE_TOKEN=$(openssl rand -hex 24)
./bin/mcp-server --http --port 8088 \
  --auth --auth-token env:GOOSIE_TOKEN
```

---

## Tool reference

### `browser_context_create`

Create a new private ephemeral browser context.

```json
{"name":"browser_context_create","arguments":{
  "viewport":{"width":1280,"height":720}
}}
```

### `browser_navigate`

Navigate using Goosie's headless pipeline with URL policy checks.

```json
{"name":"browser_navigate","arguments":{
  "contextId":"ctx_a3f1c2e8",
  "url":"https://example.com",
  "waitUntil":"interactive"
}}
```

### `browser_snapshot`

Get the accessibility tree with element refs.

```json
{"name":"browser_snapshot","arguments":{
  "contextId":"ctx_a3f1c2e8"
}}
```

### `browser_click` / `browser_type`

Bound to a context + page revision; fail closed on revision mismatch.

```json
{"name":"browser_click","arguments":{
  "contextId":"ctx_a3f1c2e8",
  "ref":"e3",
  "button":"left"
}}

{"name":"browser_type","arguments":{
  "contextId":"ctx_a3f1c2e8",
  "ref":"e7",
  "text":"hello world",
  "submit":true
}}
```

### `browser_evaluate`

JavaScript execution via the runtime owner goroutine.

```json
{"name":"browser_evaluate","arguments":{
  "contextId":"ctx_a3f1c2e8",
  "source":"document.title"
}}
```

### `browser_screenshot`

Capture the current viewport.

```json
{"name":"browser_screenshot","arguments":{
  "contextId":"ctx_a3f1c2e8",
  "format":"png"
}}
```

---

## Error codes

All errors carry a stable code so clients can branch on it:

| Code | Meaning | Retryable |
|------|---------|-----------|
| `context_not_found` | Unknown/closed context | yes |
| `page_changed` | Ref bound to old revision | yes |
| `element_not_found` | Cannot resolve locator/ref | yes |
| `ambiguous_target` | Locator matched multiple nodes | yes |
| `invalid_state` | Cannot run in current state | yes |
| `policy_denied` | Security policy rejected | no |
| `deadline_exceeded` | Operation timed out | yes |
| `cancelled` | Client cancelled | yes |
| `limit_exceeded` | Quota exceeded | no |
| `unsupported` | Not supported | no |
| `internal` | Unexpected error | no |

---

## Operational limits

| Surface | Limit |
|---------|-------|
| Contexts per server | 100 |
| Memory per context | 100 MB |
| Snapshot nodes | 5 000 |
| Snapshot depth | 50 |
| Snapshot bytes | 1 MiB |
| Screenshot pixels | 16 MP |
| Screenshot bytes | 8 MiB |
| Eval source | 256 KiB |
| Eval duration | 5 s |
| Eval result | 1 MiB |
| Rate limit | 100 burst, 50 tokens/s |
| HTTP body | 1 MiB |
| Session timeout | 30 min |

---

## Tests

```bash
# Unit tests
go test ./internal/mcpserver/... -v

# Just hardening
go test ./internal/mcpserver/... -v -run "TestAudit|TestRate|TestQuota|TestHealth|TestShutdown"

# HTTP transport
go test ./internal/mcpserver/... -v -run "TestHTTPServer"

# Race detector
go test ./internal/mcpserver/... -race
```

---

## Development

Layers (`internal/mcpserver` is split by responsibility):

| File | Responsibility |
|------|----------------|
| `server.go` | MCP JSON-RPC handler, rate limit, quota, audit on every tool call |
| `handlers.go` | Tool execution logic |
| `tools.go` | JSON Schema for every tool |
| `audit.go` | Structured JSON audit logger + redaction helpers |
| `health.go` | Health and metrics reporter |
| `quota.go` | Token-bucket rate limiter + per-context quotas |
| `shutdown.go` | SIGTERM handler + graceful shutdown |
| `http.go` | Streamable HTTP transport (loopback, sessions, auth) |

Adding a new tool:

1. Add schema to `tools.go`.
2. Add handler method to `handlers.go`.
3. Add execution case in `Server.executeTool()`.
4. Add unit tests.

---

## Regenerating screenshots

```bash
node scripts/render_screenshots.js   # regenerate HTML demo files
node scripts/html_to_png.js          # convert to PNGs via Playwright
```

Both commands take screenshots from `test/mcp-screenshots/png/` and
embed them in this README.
