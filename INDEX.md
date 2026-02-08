# Goosie Repository Index

## Quick Start

```bash
go run ./cmd/browser    # Main GUI browser
go run ./cmd/test       # Headless tests
go test -v ./internal/...  # Unit tests
```

## Project Structure

```
goosie/
├── cmd/
│   ├── browser/        # Main GUI browser (Fyne)
│   ├── renderer-demo/  # Renderer demo (no GUI)
│   ├── server/         # HTTP server for examples
│   └── test/           # Headless test utility
│
├── internal/
│   ├── css/           # CSS parser and stylesheet handling
│   ├── dom/           # HTML parsing and DOM operations
│   ├── image/         # Image loading and caching
│   ├── js/            # JavaScript runtime (Goja)
│   ├── net/           # HTTP fetching and network operations
│   ├── renderer/      # HTML rendering engine
│   └── ui/            # GUI components (Fyne)
│
└── examples/          # Example files and demos
```

## Entry Points

- **`cmd/browser/main.go`** - Main GUI browser with navigation, console, bookmarks
- **`cmd/renderer-demo/main.go`** - Renderer demo without GUI
- **`cmd/server/main.go`** - HTTP server for examples (port 8080)
- **`cmd/test/main.go`** - Headless test utility

## Core Modules

- **`internal/net/`** - Async HTTP client with context support
- **`internal/dom/`** - HTML parser using golang.org/x/net/html
- **`internal/renderer/`** - Rendering engine (render tree, layout, display list)
- **`internal/css/`** - Full CSS parser with advanced selectors
- **`internal/js/`** - Goja runtime with DOM/Browser APIs
- **`internal/image/`** - Image loading and caching
- **`internal/ui/`** - Browser UI, console, state management

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

- HTTP fetching with async/cancellation
- HTML parsing and rendering with layout engine
- Full CSS parser with advanced selectors
- JavaScript runtime (Goja) with DOM/Browser APIs
- Fyne-based GUI with navigation, console, bookmarks
- Viewport-based rendering (30-65x faster)

## Dependencies

- **goja** - JavaScript engine
- **fyne** - Cross-platform GUI framework
- **golang.org/x/net/html** - HTML parser
- **golang.org/x/image** - Image processing

See [README.md](README.md) and [ARCHITECTURE.md](ARCHITECTURE.md) for details.