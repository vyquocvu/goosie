# Goosie

Goosie is an experimental web browser engine written in Go. It implements its own HTML/CSS pipeline, layout engine, display list, and rasterizer—without WKWebView, WebView2, CEF, or another embedded browser.

Fyne provides the desktop window and input handling; page rendering is done by Goosie.

> Goosie is under active development and supports a subset of the web platform. See [Supported Web Platform](docs/SUPPORTED_WEB_PLATFORM.md) for current coverage.

## Features

- Asynchronous HTTP navigation with cancellation and resource limits
- Custom DOM, CSS selector, style, and layout engines
- Block, inline, flex, grid, form, and table layout
- Pure Go CPU rasterization with retained display lists and dirty-region updates
- JavaScript through Goja, with custom DOM and browser API bindings
- Tabs, history, bookmarks, profiles, local storage, cookies, and cache
- Built-in console, DOM inspector, network log, and storage tools
- GUI and headless PNG rendering

## Quick start

Goosie requires Go 1.24.9 or newer.

```bash
git clone https://github.com/vyquocvu/goosie.git
cd goosie
go mod download
go run ./cmd/browser
```

To open a page at startup:

```bash
go run ./cmd/browser -url=https://example.com
```

On Linux, install the native libraries required by Fyne first:

```bash
sudo apt-get install libgl1-mesa-dev xorg-dev
```

Build a binary with:

```bash
make build
./bin/goosie
```

For a smaller release binary, use the size-optimized target. It keeps the
existing symbol/debug stripping, removes local path metadata with `-trimpath`,
and clears the Go build ID for reproducible, slightly smaller output:

```bash
make build-small
./bin/goosie-small
```

If you need the smallest distributable file and accept the trade-offs of packed
executables (slightly slower startup and occasional antivirus false positives),
install UPX and run:

```bash
make build-small-upx
```

## Headless rendering

Capture a website with the headless-tag browser build:

```bash
go run -tags headless ./cmd/browser -headless \
  -url=https://example.com \
  -screenshot=screenshot.png
# or build it first
make build-headless
./bin/goosie-headless -headless -url=https://example.com -screenshot=screenshot.png
```

The default GUI build intentionally leaves out Fyne's test driver to keep the
binary smaller; use the headless-tag build above for URL screenshots.

Render local HTML or standard input directly to PNG:

```bash
go run ./cmd/headless -html=page.html -output=page.png
# or
echo '<h1>Hello, Goosie</h1>' | go run ./cmd/headless -output=page.png
```

Use `-width` and `-height` to change the headless viewport.

## How it works

```text
HTTP/HTML
  → DOM parser
  → CSS cascade and computed styles
  → layout and fragments
  → display list
  → CPU rasterizer
  → Fyne window or PNG
```

Core code lives under `internal/`; runnable programs live under `cmd/`.

## Testing

```bash
go test ./... -short                         # quick suite
go test ./...                                # full local suite
go test -tags=e2e ./test/e2e                 # end-to-end tests
go test ./internal/renderer/layoutgolden/    # layout snapshots
```

The end-to-end suite requires Playwright and network access. Install Playwright with `make install-playwright`.

## Documentation

- [Architecture](ARCHITECTURE.md)
- [Supported Web Platform](docs/SUPPORTED_WEB_PLATFORM.md)
- [Testing](TESTING.md)
- [Performance](PERFORMANCE.md)
- [Roadmap](ROADMAP_V2.md)
- [MCP Integration Roadmap](ROADMAP_MCP.md)
- [Contributing](CONTRIBUTING.md)

## License

[MIT](LICENSE)
