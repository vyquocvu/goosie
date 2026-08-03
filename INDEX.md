# Goosie Repository Index

A lightweight web browser engine built **from scratch** in Go. No platform WebViews (WKWebView, WebView2, CEF) — the entire rendering pipeline is custom Go code.

## Quick Start

```bash
go run ./cmd/browser      # Main GUI browser
go run ./cmd/headless     # Headless rendering (URL screenshots)
go test -v ./internal/...  # Unit tests
```

## Project Structure

```
goosie/
├── cmd/
│   ├── browser/        # Main GUI browser (Fyne window shell)
│   ├── headless/       # Headless rendering / CLI screenshots
│   ├── test-gen/       # Generate e2e test fixtures
│   └── screenshot-all/ # Screenshot all generated fixtures
│
├── internal/
│   ├── css/           # CSS parser and stylesheet handling
│   ├── dom/           # HTML parsing and DOM operations
│   ├── engine/        # Navigation, session, metrics
│   ├── image/         # Image loading and caching
│   ├── js/            # JavaScript runtime (Goja)
│   ├── renderer/      # Rendering engine (layout, display list, raster)
│   │   └── frame/     # Backend-neutral frame types, CPU raster, compositor
│   └── ui/            # GUI components (Fyne shell)
│
└── examples/          # Example files and demos
```

## Entry Points

- **`cmd/browser/main.go`** - Main GUI browser with navigation, console, bookmarks
- **`cmd/headless/main.go`** - Headless rendering and screenshot capture

## Core Modules

- **`internal/net/`** - Async HTTP client with context support
- **`internal/dom/`** - Compact DOM store and streaming HTML parser
- **`internal/renderer/`** - Layout engine, display list, CPU/GPU raster backends
- **`internal/css/`** - Full CSS parser with advanced selectors and style engine
- **`internal/js/`** - Goja runtime with DOM/Browser APIs
- **`internal/image/`** - Image loading and caching
- **`internal/ui/`** - Browser UI, console, state management (Fyne shell only)
- **`internal/engine/`** - Navigation scheduler, session lifecycle, metrics

## Proposed MCP Integration

- **[MCP Roadmap](ROADMAP_MCP.md)** - Documentation-first milestones and release gates
- **[MCP Architecture](website/docs/mcp-architecture.md)** - UI-independent browser-control boundary
- **[MCP Contracts](website/docs/mcp-protocol-contracts.md)** - Planned tools, resources, limits, and errors
- **[MCP Security](website/docs/mcp-security.md)** - Threat model and mandatory controls
- **[MCP TDD Plan](website/docs/mcp-tdd-plan.md)** - Test order, fixtures, matrices, and CI gates

## Examples

- `examples/console_demo/` - Console API examples
- `examples/dom_api_demo/` - DOM API examples
- `examples/html/` - HTML example files

## Testing

```bash
go test -v -cover ./internal/...  # All tests with coverage
go test -bench=. ./internal/renderer  # Benchmarks
```

**Coverage:**
- internal/renderer: 100% (65+ tests)
- internal/dom: 95.0%
- internal/js: 92.9%
- internal/net: 36.4%

## Key Features

- **From-scratch rendering**: no WKWebView, WebView2, CEF, or platform WebViews
- HTTP fetching with async/cancellation
- Custom HTML parser and compact DOM store
- Full CSS parser with advanced selectors, style invalidation, computed styles
- JavaScript runtime (Goja) with DOM/Browser APIs
- Fyne-based GUI shell (window management only)
- Viewport-based rendering (30-65x faster)
- Pure Go CPU raster backend + optional CoreGraphics on macOS
- Retained tile compositor for smooth scrolling

## Dependencies

- **goja** - JavaScript engine
- **fyne** - GUI framework (window shell only — rendering is custom Go raster)
- **golang.org/x/net/html** - HTML tokenizer (tree construction is custom)
- **golang.org/x/image** - Font rendering and image decoding

See [README.md](README.md) and [ARCHITECTURE.md](ARCHITECTURE.md) for details.
