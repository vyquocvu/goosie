# internal/mcpserver — Agent Constraints & Architecture

## Core Responsibilities

The `internal/mcpserver` package exposes Goosie's browser automation engine to AI agents and external tools via the Model Context Protocol (MCP) standard over stdio and HTTP/SSE transports.

## Architecture & Transports

- `Server` (`server.go`) wraps `browsercontrol.Service` and registers MCP protocol capabilities.
- **Stdio Transport**: Standard I/O loop for local command-line agent integration (e.g. Claude Desktop, local LLM runners).
- **HTTP/SSE Transport**: `http.go` exposes Server-Sent Events (SSE) and `/message` JSON-RPC endpoints for remote agent coordination.

## Tool Registry & Dispatch (`tools.go`, `handlers.go`)

Exposes a suite of structured MCP tools:
- `browser_context_create` / `browser_context_list` / `browser_context_close`: Ephemeral browser context lifecycle management.
- `browser_navigate`: URL loading with lifecycle wait conditions (`commit`, `interactive`, `complete`).
- `browser_snapshot`: Bounded semantic page tree snapshot with opaque element references.
- `browser_screenshot`: Viewport PNG raster capture.
- `browser_page_info`: Current page URL, title, lifecycle state, revision, and viewport.
- `browser_query`: CSS selector query resolution.
- `browser_click` / `browser_type`: Direct user input simulation.

## Security Hardening & Quota Protection

- `QuotaTracker` (`quota.go`): Enforces limits on concurrent contexts, maximum page snapshots, and payload sizes.
- `RateLimiter` (`Server.Limiter`): Token-bucket rate limiting protecting against request storms.
- `AuditLogger` (`audit.go`): Structured logging of all tool invocations and parameters.
- `HealthReporter` (`health.go`): Live health checks and telemetry.

## Graceful Shutdown & Cancellation

- `shutdown.go` handles OS termination signals (`SIGINT`, `SIGTERM`), cancels all active context requests, flushes logs, and releases browser sessions cleanly.

## Testing & Verification

All MCP server tests reside in `test/internal/mcpserver/...`.

Run the MCP server test suite with the race detector:
```bash
go test -race -short ./test/internal/mcpserver/...
```
