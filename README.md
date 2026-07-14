# Goosie

A lightweight web browser engine built from scratch in Go. No platform WebViews (WKWebView, WebView2) — the entire rendering pipeline is custom Go code.

**Architecture:**
- **Rendering**: Pure Go CPU raster backend (optional CoreGraphics on macOS via CGo)
- **GUI shell**: Fyne — handles windowing, keyboard/mouse input, pixel buffer presentation
- **HTML/CSS**: Custom parser and layout engine via `golang.org/x/net/html` + internal CSS pipeline
- **JavaScript**: Goja runtime with custom DOM/Browser API bindings


## Features

- **Persistent Profile Foundation**: Browser data can be stored in a profile directory, including bookmarks, history, settings, origin-scoped localStorage, cookies, and cache metadata, with built-in schema versioning, automatic migrations, corruption recovery, and import/export capabilities for profile settings.
- **Private Browsing Foundation**: Ephemeral profile mode keeps browsing state in memory and avoids writing profile data to disk.
- **Developer Tools Foundation**: Console execution, DOM inspection, network log, storage view, security summary, and downloads panels.
- **Release Builds**: Tag-based GitHub Actions workflow builds cross-platform browser binaries.
- **HTTP Fetching**: Async fetch with cancellation support using context
- **HTML Parsing**: Parse HTML and extract body text using golang.org/x/net/html
   - Streaming tree construction with token-by-token parsing into a compact DOM store (M2.4)
   - Context-aware cancellation during parsing for responsive navigation
   - Early resource discovery (CSS, scripts, images) during parse for parallel fetching
   - Unsupported feature detection (canvas, video, audio, iframe) during parse for fallback decisions (M12.1)
   - Runtime detection of unsupported DOM APIs (canvas, video, audio, iframe, object, embed) created via `document.createElement` — deduplicated per page, surfaced through `Runtime.SetRuntimeUnsupportedFeatureCallback` for fallback decisions (M12.1)
   - Runtime detection of dynamic `import()` expressions via source-level pre-scan in `Runtime.ScanAndReportUnsupportedJSFeatures`, auto-invoked from `RunScript` — surfaces ES module graph usage for fallback decisions (M12.1)
   - Runtime detection of WebSocket, Web Worker, and ServiceWorker API usage via stub constructors — `new WebSocket(url)`, `new Worker(url)`, `navigator.serviceWorker.register(...)` each report their `dom.UnsupportedFeatureKind` to the dedup'd runtime detection callback (M12.1)
- **HTML Rendering**: Canvas-based renderer with layout engine
  - Render tree for optimized DOM representation
  - Layout engine with box model calculations
  - Support for core HTML elements (headings, paragraphs, lists, links, images)
  - Form elements (input, button, textarea)
  - Table rendering with proper tbody/thead/tfoot handling, cell spans, and cached column measurements (M4.5)
  - **Full CSS parser** with advanced selector support
    - All combinators (descendant, child, adjacent sibling, general sibling)
    - Attribute selectors with all operators
    - Pseudo-classes (:first-child, :last-child, :nth-child, etc.)
    - Pseudo-elements (::before, ::after)
    - CSS comments and at-rules (@media, @import, @keyframes)
    - !important flag support
  - **Compiled selector engine** (M3.2) with precomputed specificity and bucketed rule lookup
    - 2x faster matching than linear scan
    - 95% less memory per match operation
  - **Computed-style storage** (M3.3) with typed structs and zero-allocation operations
    - Inherited/non-inherited property separation per CSS spec
    - Fingerprint-based style deduplication via bounded StylePool
    - All operations (fingerprint, equality, inheritance, declaration apply) are zero-allocation
  - **Style invalidation** (M3.4) with bucket-based affected rule analysis
    - Mutation classification (class, ID, attribute, inline style, text, insertion, removal)
    - Descendant invalidation for inherited property changes
    - Sibling invalidation for adjacent (+) and general (~) sibling combinators
    - Mutation batching with target deduplication
  - **Layout store** (M4.1) with compact index-based storage
    - Stable LayoutID handles replace pointer-heavy *LayoutBox trees
    - display:none elements receive no layout allocation
    - Bidirectional DOM-to-layout and layout-to-DOM mappings
    - Generated content support (::before, ::after)
  - **Fragment store** (M4.2) for inline layout
    - FragmentID handles for line fragments, text runs, boxes, replaced elements
    - One layout object can produce multiple fragments (line breaks)
    - Text runs batch multiple glyphs (not one object per glyph)
    - Scratch buffer pool for zero-allocation line layout
  - **Text shaping** (M4.3) with backend-neutral measurement
    - FontKey identifies unique font configurations
    - ShapedText contains glyphs with positions and metrics
    - Cache for shaped text runs (O(1) for repeated measurements)
    - Whitespace-aware text wrapping for line layout
    - Direction support (LTR/RTL)
  - **Backend-neutral display commands** (M5.1) with compact value-type storage
    - DisplayCommand value types for rect, border, text, image, clip, transform, opacity, stacking context
    - DisplayCommandList stores commands by value (not pointer) for reduced GC pressure
    - TransformMatrix with multiply, inverse, translate, scale, rotate
    - Full JSON serialization for debugging and future IPC
    - Zero-allocation command creation
  - **Paint chunks** (M5.2) for retained display list invalidation
    - PaintChunk groups commands by LayoutID ownership with union bounds
    - ChunkedDisplayList supports per-chunk dirty tracking and reuse
    - SourceMapping for developer tools (LayoutID → command range)
    - Zero-allocation invalidation and spatial queries
  - **Dirty-region invalidation** (M5.3) for minimal repaint regions
    - DirtyRegion tracks bounded list of dirty rects with automatic overlap merging
    - DirtyRegionTracker tracks per-LayoutID bounds across frames
    - InvalidateMove marks both old and new regions dirty on object movement
    - ExpandForEffects accounts for shadows, borders, and antialiasing
    - DebugDirtyRegionOverlay generates display commands for dev-tools visualization
    - Zero-allocation core operations
  - CSS styling support (colors, font-size, font-weight)
  - Text styling (bold, italic)
  - HTML hierarchy preservation
  - **High-performance viewport-based rendering** (30-65x faster than traditional approaches)
  - Display list caching for smooth scrolling
  - Viewport culling to only render visible content
- **Async Architecture**: Non-blocking page loads with responsive UI
  - Background goroutines for network and parsing operations
  - Loading spinner with visual feedback
  - Cancellable requests (navigate away anytime)
  - Context-based timeout and cancellation support
  - Response metadata preservation for security inspection and developer tools
  - Streaming response body path (M1.3) eliminates intermediate buffer copies
  - Response and decompression size limits (M10.1): default 100 MB body limit, Content-Length pre-check, decompression bomb detection
  - MIME validation (M10.1): optional Content-Type validation against expected types, zero-allocation classification
- **JavaScript Runtime**: Execute JavaScript with Goja engine and comprehensive DOM APIs
  - Enhanced Console API: `console.log()`, `console.error()`, `console.warn()`, `console.info()`, `console.table()`
  - Console panel in browser UI with filtering and error tracking
  - Query methods: `getElementById()`, `getElementsByClassName()`, `getElementsByTagName()`, `querySelector()`, `querySelectorAll()`
  - Element creation: `createElement()`
  - DOM manipulation: `appendChild()`, `removeChild()`, `replaceChild()`, `insertBefore()`
  - Event handling: `addEventListener()`, `removeEventListener()`
  - JavaScript error reporting and tracking
  - See [DOM_API_DOCUMENTATION.md](DOM_API_DOCUMENTATION.md) for complete API reference and examples
  - See [CONSOLE_DOCUMENTATION.md](CONSOLE_DOCUMENTATION.md) for enhanced console features
- **Browser APIs**: Full browser environment with essential web APIs
  - window.location: URL manipulation and query parameters
  - window.history: Session history and navigation
  - Timers: `setTimeout()`, `setInterval()` with automatic cleanup
  - Network: `fetch()` API for HTTP requests
  - Storage: `localStorage` and `sessionStorage` with validation
  - See [BROWSER_API_DOCUMENTATION.md](BROWSER_API_DOCUMENTATION.md) for complete API reference and best practices
- **GUI**: Display rendered content in a Fyne window titled "Goosie"
- **Navigation**: Full-featured navigation system
  - URL bar for entering web addresses
  - Back/Forward navigation buttons with proper state management
  - Refresh/Reload button
  - Session-based navigation history
  - Bookmark management (add/remove with visual indicators)

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed architecture documentation.

The project follows a clean architecture:
- **cmd/**: Command-line applications (browser, renderer-demo, test, server)
- **internal/**: Core library code (net, dom, renderer, js, ui, css, image)
- **internal/engine/**: Navigation, session lifecycle, metrics
- **internal/renderer/frame/**: Backend-neutral display commands and raster abstraction
- **internal/renderer/frame/raster/**: Pure Go CPU raster backend (default) and CoreGraphics CGo backend (optional)
- **examples/**: Demo files and HTML examples

### Rendering Pipeline (from scratch, no WebViews)

```text
HTML String
  → Custom HTML Parser (internal/dom) — streaming, compact DOM store
  → CSS Pipeline (internal/css) — parser, compiled selectors, computed styles, invalidation
  → Layout Engine (internal/renderer) — block/inline, fragments, incremental reflow
  → Display Command List (internal/renderer) — backend-neutral value types
  → Raster Backend (internal/renderer/frame/raster) — CPU or CoreGraphics
  → Fyne Adapter (internal/renderer) — presents pixel buffer in window
```

Fyne never touches layout, style, or display-list construction. It is strictly the window/presentation shell.

## Dependencies

- [goja](https://github.com/dop251/goja) - JavaScript engine
- [fyne](https://fyne.io/) - Cross-platform GUI framework (window shell only — rendering is done by Go's own raster backend, not Fyne canvas primitives)
- [x/net/html](https://pkg.go.dev/golang.org/x/net/html) - HTML tokenizer (tree construction is custom, in `internal/dom`)
- [golang.org/x/image](https://golang.org/x/image) - Font rendering and image decoding

### What Goosie does NOT use

- ❌ **No WKWebView** (macOS) — the core engine renders HTML/CSS with its own layout and CPU/GPU raster backend
- ❌ **No WebView2** (Windows) — the core engine has no dependency on WebView2 or Edge Chromium
- ❌ **No CEF/Chromium Embedded Framework**
- ❌ **No WebKit** — no WKWebView, no embedded Chromium/WebKit. Everything is custom Go code

## Installation

### Prerequisites

For GUI functionality (cmd/browser), you need:

**Linux:**
```bash
sudo apt-get install libgl1-mesa-dev xorg-dev
```

**macOS:**
```bash
# Xcode command line tools
xcode-select --install
```

**Windows:**
```
# No additional dependencies required
```

### Build

```bash
# Clone the repository
git clone https://github.com/vyquocvu/goosie.git
cd goosie

# Install dependencies
go mod download

# Build the browser
go build ./cmd/browser

# Or run directly
go run ./cmd/browser
```

## Usage

### Test Tiers

```bash
# Tier 1: Quick unit tests (no network, no GUI)
go test ./... -short

# Tier 2: Full local suite (includes httptest loopback servers)
go test ./...

# Tier 3: End-to-end tests (requires Playwright + network)
go test -tags=e2e ./test/e2e

# Tier 4: Roadmap feature verification (live websites)
go run ./cmd/roadmap_test/
```

Use the short tier for sandbox-safe checks and the e2e tier for Playwright-driven browser tests.

### Golden Image Tests

```bash
# Run golden image tests (validates against committed reference images)
go test -v ./internal/renderer/frame/golden/ -run 'TestGolden(Fill|Border|Clipped|Opacity|Composite|Nested|Empty)'

# Update golden reference images after intentional rendering changes
GOOSIE_UPDATE_GOLDEN=1 go test -v ./internal/renderer/frame/golden/ -run 'TestGolden(Fill|Border|Clipped|Opacity|Composite|Nested|Empty)'

# Run golden benchmarks
go test -bench=BenchmarkGolden -benchmem ./internal/renderer/frame/golden/
```

Golden tests use the pure Go CPU raster backend, producing bit-identical output
across platforms. Reference images are stored at `internal/renderer/frame/golden/testdata/golden/`.
The CI workflow `.github/workflows/golden.yml` validates golden images on every PR
touching golden-related paths.

### Golden Layout Tests

The layout engine has a parallel golden-test infrastructure that catches
regressions in box-model, flex, inline, and table layout before pixels are
drawn. Where the raster golden tests compare rendered pixels against PNG
snapshots, layout goldens compare the deterministic text serialization of
the `LayoutBox` tree against committed text snapshots — fast to diff, easy
to review, and unaffected by raster-side changes.

```bash
# Run golden layout tests against committed snapshots
go test ./internal/renderer/layoutgolden/

# Regenerate layout snapshots after intentional engine changes
GOOSIE_UPDATE_GOLDEN=1 go test ./internal/renderer/layoutgolden/
# Or equivalently:
go test -update ./internal/renderer/layoutgolden/

# Verify determinism guard (same input → same bytes twice)
go test ./internal/renderer/layoutgolden/ -run TestGoldenLayoutDeterminism
```

Layout golden snapshots are stored at
`internal/renderer/layoutgolden/testdata/golden-layout/<name>.txt`.
Candidates produced by an in-progress run are written to
`testdata/golden-layout-update/<name>.txt` for review before regeneration.

The serializer rounds floats to two decimal places and emits only
structural fields (geometry, padding, margin, display type, flex/grid
container parameters). Volatile fields such as NodeID, colors, and font
cache keys are intentionally omitted, which keeps snapshots stable
across Go versions and platforms while still pinpointing real layout
behavior changes.

### Fuzz Tests (Parser and Selector)

The HTML streaming parser (`internal/dom`) and the CSS parser + compiled selector
matcher (`internal/css`) ship with native Go fuzz harnesses. By default (no
`-fuzz` flag) each fuzz target runs once with its seed corpus so CI remains
deterministic; with `-fuzz` Go generates random inputs and reports any panic
or invariant violation.

```bash
# Seed-only deterministic run (CI default)
go test ./internal/dom/ ./internal/css/

# Fuzz the streaming HTML parser for 10 seconds
go test -fuzz=FuzzHTMLParseDocument -fuzztime=10s ./internal/dom/

# Fuzz pre-cancelled context behavior
go test -fuzz=FuzzHTMLParseDocumentCancelContext -fuzztime=10s ./internal/dom/

# Fuzz element lookup by ID
go test -fuzz=FuzzHTMLGetElementByID -fuzztime=10s ./internal/dom/

# Fuzz the simple body-text extractor
go test -fuzz=FuzzHTMLParseBodyText -fuzztime=10s ./internal/dom/

# Fuzz the CSS parser
go test -fuzz=FuzzCSSParser -fuzztime=10s ./internal/css/

# Fuzz the compiled selector matcher
go test -fuzz=FuzzSelectorMatcher -fuzztime=10s ./internal/css/
```

Each fuzz target enforces a strict input size bound (4 KiB for input bytes,
256 B for selector fields) so the Go runtime never OOMs while the fuzzer
explores pathological patterns. Invariants are structural — no exact output
shape assertions — because HTML/CSS admit many equivalent parses for
malformed input.

### Milestone-Gated Testing

Tests are gated by roadmap milestone so they unlock automatically as features are completed. The current milestone is controlled by the `GOOSIE_MILESTONE` environment variable (default: `2`).

```bash
# Run e2e tests at current milestone (M2)
go test -tags=e2e ./test/e2e/

# Unlock M3 tests (CSS pipeline validation against real websites)
GOOSIE_MILESTONE=3 go test -tags=e2e ./test/e2e/ -run TestRealWebsitesCSSParsing

# Run roadmap verification at M3
GOOSIE_MILESTONE=3 go run ./cmd/roadmap_test/
```

When a milestone is completed, update the default value in:
- `test/e2e/real_websites_test.go` (`milestoneGate` function)
- `cmd/roadmap_test/main.go` (`currentMilestone` variable)

**Real website test coverage** (10 sites across 3 complexity tiers):

| Milestone | Sites | Validates |
|-----------|-------|----------|
| M1 | example.com, iana.org, info.cern.ch, httpbin, testing.toscrape | HTTP fetch, navigation pipeline, response metadata |
| M2 | w3schools, lipsum, quotes.toscrape | Compact DOM parsing, streaming parser, rendering |
| M3 | wikipedia, MDN | CSS pipeline, computed styles, selector matching |

### Benchmarks

```bash
# CSS parser benchmarks
go test -bench=. -benchmem ./internal/css/

# Compiled selector benchmarks (M3.2)
go test -bench=BenchmarkMatchVsLinear -benchmem ./internal/css/
go test -bench=BenchmarkMatchElement -benchmem ./internal/css/

# Computed-style storage benchmarks (M3.3)
go test -bench=BenchmarkInheritedStyle -benchmem ./internal/css/
go test -bench=BenchmarkStylePool -benchmem ./internal/css/
go test -bench=BenchmarkApplyDeclarations -benchmem ./internal/css/
go test -bench=BenchmarkComputedStyle -benchmem ./internal/css/

# Match cache and style pool eviction benchmarks (M9.2)
go test -bench=BenchmarkMatchCache -benchmem ./internal/css/
go test -bench=BenchmarkStylePool_Evict -benchmem ./internal/css/

# Glyph cache byte budget and eviction benchmarks (M9.2)
go test -bench=BenchmarkGlyphCachePutWithBytes -benchmem ./internal/renderer/frame/cache/
go test -bench=BenchmarkGlyphCacheEvict -benchmem ./internal/renderer/frame/cache/

# Style invalidation benchmarks (M3.4)
go test -bench=BenchmarkComputeInvalidation -benchmem ./internal/css/
go test -bench=BenchmarkBatchMutations -benchmem ./internal/css/
go test -bench=BenchmarkAffectedRuleIndices -benchmem ./internal/css/

# Layout store benchmarks (M4.1)
go test -bench=BenchmarkLayoutStore -benchmem ./internal/renderer/

# Fragment store benchmarks (M4.2)
go test -bench=BenchmarkFragment -benchmem ./internal/renderer/
go test -bench=BenchmarkScratchBufferPool -benchmem ./internal/renderer/

# Text shaping benchmarks (M4.3)
go test -bench=BenchmarkTextShaper -benchmem ./internal/renderer/
go test -bench=BenchmarkFontKeyCacheKey -benchmem ./internal/renderer/

# Display command benchmarks (M5.1)
go test -bench=BenchmarkDisplayCommand -benchmem ./internal/renderer/
go test -bench=BenchmarkTransformMatrix -benchmem ./internal/renderer/

# Paint chunk benchmarks (M5.2)
go test -bench=BenchmarkBuildPaintChunks -benchmem ./internal/renderer/
go test -bench=BenchmarkChunkedDisplayList -benchmem ./internal/renderer/
go test -bench=BenchmarkSourceMapping -benchmem ./internal/renderer/
go test -bench=BenchmarkPaintChunk -benchmem ./internal/renderer/

# Dirty-region benchmarks (M5.3)
go test -bench=BenchmarkDirtyRegion -benchmem ./internal/renderer/
go test -bench=BenchmarkExpandForEffects -benchmem ./internal/renderer/
go test -bench=BenchmarkDebugDirtyRegionOverlay -benchmem ./internal/renderer/

# DOM parser benchmarks
go test -bench=. -benchmem ./internal/dom/

# MIME validation benchmarks (M10.1)
go test -bench=BenchmarkValidateContentType -benchmem ./internal/net/
go test -bench=BenchmarkClassifyContentType -benchmem ./internal/net/

# Atom and string interning benchmarks (M2.2)
go test -bench=. -benchmem ./internal/dom/atom/

# Deterministic engine corpus benchmarks (article, documentation, table, form, image, JavaScript-light, scrolling)
go test -bench=. -benchmem ./internal/engine/testpages/

# Full renderer benchmarks (layout, display list, viewport, scroll)
go test -bench=. -benchmem ./internal/renderer/

# Tile cache benchmarks (M7.1, M9.2)
go test -bench="BenchmarkTileCache_Get" -benchmem ./internal/renderer/frame/compositor/
go test -bench=BenchmarkTileCache_Evict -benchmem ./internal/renderer/frame/compositor/

# Page cache benchmarks (M9.2)
go test -bench=. -benchmem ./internal/engine/pagecache/
```

Pull requests that touch engine benchmark-sensitive paths run a
Performance workflow with:
- Microbenchmarks for DOM parser, CSS selector, layout, display-list,
  session, navigation, metrics, and scrolling
- `go test -race` gate for concurrent engine packages
- Allocation regression detection (fail CI on >5% allocs/op or >10% B/op)
- Timing regression warnings (>10% ns/op)
- Comparison against committed baseline via `benchstat`

Nightly benchmarks run longer (1s benchtime) navigation, scrolling,
and full-pipeline scenarios with artifact storage (90-day retention).

### GUI Browser

Run the full browser with GUI:

```bash
go run ./cmd/browser
```

This will:
1. Open a window titled "Goosie" with navigation controls
2. Display a welcome message
3. Allow you to enter a URL in the address bar
4. Fetch and display web pages with async loading (UI stays responsive)
5. Show a loading spinner during page fetch and render
6. Enable back/forward navigation between pages
7. Support bookmark management with visual indicators
8. Initialize the Goja runtime with `console.log` and `document.getElementById`
9. Allow cancelling slow page loads by navigating to a new URL

### Testing Components (No GUI)

Test the core components without GUI dependencies:

```bash
go run ./cmd/test
```

This validates:
- HTTP fetcher
- HTML parser
- JavaScript runtime with console.log
- document.getElementById functionality

## Example

The browser demonstrates web functionality by:

1. **Navigation**: Enter URLs in the address bar to browse websites
2. **Fetching**: Downloads web pages using HTTP GET
3. **Parsing**: Parses HTML structure using golang.org/x/net/html
4. **Rendering**: Canvas-based renderer that:
   - Builds a render tree from HTML nodes
   - Calculates layout with box model
   - Renders to Fyne canvas with proper formatting
   - Supports headings, paragraphs, lists, links, and images
5. **History**: Navigate back and forward through visited pages
6. **Bookmarks**: Save and manage favorite pages with visual indicators
7. **JavaScript**: Runs JavaScript with Goja, supporting comprehensive DOM and Browser APIs:
   ```javascript
   // Enhanced Console - Multiple log levels and structured data
   console.log("Application started");
   console.info("Version: 1.0.0");
   console.warn("Using default configuration");
   console.error("Failed to load resource");
   
   // Console table for structured data
   var users = {name: "John", age: 30, role: "Developer"};
   console.table(users);
   
   // DOM APIs - Query and manipulate elements
   var elem = document.getElementById("main-content");
   var items = document.querySelectorAll(".list-item");
   
   var newDiv = document.createElement("div");
   newDiv.textContent = "Hello, World!";
   elem.appendChild(newDiv);
   
   // Browser APIs - Location and History
   window.location.setURL("https://example.com?page=1");
   var page = window.location.getQueryParam("page");
   window.history.pushState({}, "Page Title", "/new-page");
   
   // Timers and Async Operations
   setTimeout(function() {
       console.log("Delayed execution");
   }, 1000);
   
   // Network Requests
   fetch("https://api.example.com/data")
       .then(function(response) {
           return response.json();
       })
       .then(function(data) {
           console.log("Data:", data);
       });
   
   // Storage APIs
   localStorage.setItem("theme", "dark");
   var theme = localStorage.getItem("theme");
   ```
   
   See [DOM_API_DOCUMENTATION.md](DOM_API_DOCUMENTATION.md), [BROWSER_API_DOCUMENTATION.md](BROWSER_API_DOCUMENTATION.md), and [CONSOLE_DOCUMENTATION.md](CONSOLE_DOCUMENTATION.md) for complete API references.

8. **Developer Console**: Built-in console panel for debugging JavaScript
   - Click the console button (⊞) in the toolbar to show/hide the panel
   - View all console messages with timestamps and severity levels
   - Filter messages by level (log, error, warn, info, table)
   - Track JavaScript errors with error counter
   - Clear console messages with one click
   - See [CONSOLE_DOCUMENTATION.md](CONSOLE_DOCUMENTATION.md) for details

## Development

### Project Structure

- **internal/engine/navigation**: Monotonic navigation IDs, cancellable load contexts, and stale-callback rejection
- **internal/engine/session**: Session lifecycle (state machine, context propagation, event callbacks) wrapping the navigation scheduler
- **internal/engine/metrics**: Phase-timing recorder and counters for tracing navigation from URL entry to first paint
- **internal/engine/pagecache**: Bounded LRU page cache for instant back/forward navigation (M9.2)
- **internal/memory**: Bounded memory limits, GC tuning, evaluation, thrashing detection, and profiling APIs (M9.1, M9.4)
- **internal/dom**: HTML parser for extracting content; compact DOM store (M2.3) with NodeID-based index storage
- **internal/dom/atom**: String interning with static atoms for HTML tags/attributes and bounded LRU-evicted dynamic table (M2.2)
- **internal/net**: Async HTTP client with context support for fetching web pages
- **internal/renderer**: Canvas-based HTML renderer with layout engine
- **internal/js**: JavaScript runtime wrapper around Goja with enhanced console
- **internal/ui**: Fyne-based GUI components with loading indicator and console panel
- **cmd/browser**: Main browser application with async page loading
- **cmd/renderer-demo**: Renderer demonstration without GUI
- **cmd/test**: Testing utility without GUI dependencies
- **examples**: Demo files including console_demo.go and console_demo.html

### Key Documentation

- **[SUPPORTED_WEB_PLATFORM.md](docs/SUPPORTED_WEB_PLATFORM.md)**: v2 supported web-platform scope, fallbacks, out-of-scope features, and resource limits
- **[DOM_API_DOCUMENTATION.md](DOM_API_DOCUMENTATION.md)**: Comprehensive DOM API reference and examples
- **[BROWSER_API_DOCUMENTATION.md](BROWSER_API_DOCUMENTATION.md)**: Browser APIs (location, history, timers, fetch, storage)
- **[CONSOLE_DOCUMENTATION.md](CONSOLE_DOCUMENTATION.md)**: Enhanced console features and debugging tools
- **[ARCHITECTURE.md](ARCHITECTURE.md)**: System architecture and component flow
- **[PERFORMANCE.md](PERFORMANCE.md)**: Performance optimizations and benchmarks
- **[ROADMAP_V2.md](ROADMAP_V2.md)**: Planned engine milestones, product backlog, and development roadmap

### Adding Features

Goosie includes comprehensive DOM APIs (see [DOM_API_DOCUMENTATION.md](DOM_API_DOCUMENTATION.md)) and browser APIs (see [BROWSER_API_DOCUMENTATION.md](BROWSER_API_DOCUMENTATION.md)). To add additional JavaScript APIs, edit `internal/js/runtime.go`:

```go
// Example: Add a custom API
document.Set("customMethod", func(call goja.FunctionCall) goja.Value {
    // Implementation
})
```

The browser includes:
- **DOM APIs**: Query selectors, element manipulation, event handling
- **Browser APIs**: window.location, window.history, timers, fetch, storage

To add new UI features, edit `internal/ui/browser.go`:

```go
// Add URL bar, navigation buttons, etc.
```

## Performance

Goosie includes advanced performance optimizations for smooth scrolling and high frame rates:

- **Viewport-based rendering**: 30x faster than traditional full-page rendering
- **Display list caching**: Eliminates repeated DOM traversal
- **Scroll optimization**: 65x faster scroll updates
- **Scales to thousands of elements**: Constant-time rendering regardless of page size

Microbenchmarks exist for the CSS parser, DOM parser, and the full rendering pipeline (layout, display list, viewport culling, scroll). All benchmarks include allocation tracking with `-benchmem`.

See [PERFORMANCE.md](PERFORMANCE.md) for detailed benchmarks and technical information.

## Roadmap

See [ROADMAP_V2.md](ROADMAP_V2.md) for planned features and future development goals.

## License

This project is provided as-is for educational purposes.

### CI Benchmark Comparison

A dedicated script `scripts/bench-ci.sh` powers the PR performance gates:

```bash
# Run benchmarks and compare against baseline (default)
./scripts/bench-ci.sh check

# Run benchmarks and save as new baseline
./scripts/bench-ci.sh record

# Run benchmarks without comparison
./scripts/bench-ci.sh run-only
```

### Profiling and Tracing
We provide a convenient bash script at `scripts/bench.sh` to quickly run performance tools:
```bash
# Run all benchmarks across the project
./scripts/bench.sh run

# Capture CPU profile
./scripts/bench.sh profile-cpu ./internal/renderer

# Capture Memory profile
./scripts/bench.sh profile-mem ./internal/renderer

# Capture runtime trace
./scripts/bench.sh trace ./internal/engine/testpages

# Run the full benchmark suite
./scripts/bench.sh suite

# Compare benchmark results with benchstat
./scripts/bench.sh compare main-perf.txt feature-perf.txt
```
