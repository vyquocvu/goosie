# MCP Server

This package provides a Model Context Protocol (MCP) server implementation for the Goosie browser engine.

## Overview

The MCP server exposes Goosie's browser automation capabilities via the MCP protocol over stdio transport.

## Architecture

```
MCP Client (stdio)
       │
       ▼
cmd/mcp-server (entry point)
       │
       ▼
internal/mcpserver (protocol adapter)
       │
       ▼
internal/browsercontrol (UI-independent browser service)
       │
       ├─ EngineService (real browser contexts)
       └─ FakeService (testing)
```

## Tools

| Tool | Description |
|------|-------------|
| `browser_context_create` | Create a new private ephemeral browser context |
| `browser_context_list` | List all contexts owned by this connection |
| `browser_context_close` | Idempotently close a context |
| `browser_navigate` | Navigate to a URL |
| `browser_snapshot` | Get semantic page snapshot with element refs |
| `browser_screenshot` | Capture viewport as PNG |
| `browser_page_info` | Get page URL, title, state, viewport |
| `browser_query` | Query elements by role/CSS/text |
| `browser_click` | Click an element |
| `browser_type` | Type text into an element |

## Usage

### Command Line

```bash
# Run the server
go run ./cmd/mcp-server

# With custom name and version
go run ./cmd/mcp-server --name goosie-browser --version 1.0.0

# With debug logging
go run ./cmd/mcp-server --log-level debug
```

### MCP Client Integration

Add to your MCP client configuration (e.g., Claude Desktop):

```json
{
  "mcpServers": {
    "goosie": {
      "command": "go",
      "args": ["run", "github.com/vyquocvu/goosie/cmd/mcp-server"],
      "env": {}
    }
  }
}
```

## Protocol

The server uses MCP JSON-RPC 2.0 over stdio transport.

### Initialize

```json
{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2025-11-25"}}
```

### List Tools

```json
{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}
```

### Call Tool

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "browser_context_create",
    "arguments": {"viewport": {"width": 1280, "height": 720}}
  }
}
```

## Error Codes

| Code | Meaning | Retryable |
|------|---------|-----------|
| `context_not_found` | Unknown/closed context | Yes |
| `page_changed` | Reference belongs to old revision | Yes |
| `element_not_found` | Cannot resolve locator/ref | Yes |
| `ambiguous_target` | Locator matched multiple nodes | Yes |
| `invalid_state` | Cannot run in current state | Yes |
| `policy_denied` | Security policy rejected | No |
| `deadline_exceeded` | Operation timed out | Yes |
| `cancelled` | Client cancelled | Yes |
| `limit_exceeded` | Quota exceeded | No |
| `unsupported` | Not supported | No |
| `internal` | Unexpected error | No |

## Limits

| Limit | Value |
|-------|-------|
| Max contexts per server | 10 |
| Default timeout | 10 seconds |
| Max snapshot depth | 50 |
| Max snapshot nodes | 5000 |
| Max snapshot bytes | 1 MiB |
| Max screenshot pixels | 16 MP |
| Max screenshot encoded | 8 MiB |
| Max eval source | 256 KiB |
| Max eval duration | 5 seconds |
| Max eval result | 1 MiB |

## Testing

```bash
# Run unit tests
go test ./internal/mcpserver/... -v

# Run with race detector
go test ./internal/mcpserver/... -race

# Run benchmarks
go test ./internal/mcpserver/... -bench=. -benchmem
```

## Development

The MCP server is built in layers:

1. **Protocol layer** (`server.go`): MCP JSON-RPC handling
2. **Tool handlers** (`handlers.go`): Tool execution logic
3. **Tool schemas** (`tools.go`): JSON Schema definitions
4. **Browser control** (`internal/browsercontrol`): UI-independent browser API

For new tools:
1. Add schema to `tools.go`
2. Add handler method to `handlers.go`
3. Add execution case in `executeTool()`
4. Add tests
