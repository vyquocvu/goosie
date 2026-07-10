# Goosie

A minimal web browser implemented in Go using Goja (JavaScript engine), Fyne (GUI framework), and x/net/html (HTML parser).

## Features

- **Persistent Profile Foundation**: Browser data can be stored in a profile directory, including bookmarks, history, settings, origin-scoped localStorage, cookies, and cache metadata.
- **Private Browsing Foundation**: Ephemeral profile mode keeps browsing state in memory and avoids writing profile data to disk.
- **Developer Tools Foundation**: Console execution, DOM inspection, network log, storage view, security summary, and downloads panels.
- **Release Builds**: Tag-based GitHub Actions workflow builds cross-platform browser binaries.
- **HTTP Fetching**: Async fetch with cancellation support using context
- **HTML Parsing**: Parse HTML and extract body text using golang.org/x/net/html
  - Streaming tree construction with token-by-token parsing into a compact DOM store (M2.4)
  - Context-aware cancellation during parsing for responsive navigation
  - Early resource discovery (CSS, scripts, images) during parse for parallel fetching
- **HTML Rendering**: Canvas-based renderer with layout engine
  - Render tree for optimized DOM representation
  - Layout engine with box model calculations
  - Support for core HTML elements (headings, paragraphs, lists, links, images)
  - Form elements (input, button, textarea)
  - Table rendering with proper tbody/thead/tfoot handling
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
- **examples/**: Demo files and HTML examples

## Dependencies

- [goja](https://github.com/dop251/goja) - JavaScript engine
- [fyne](https://fyne.io/) - Cross-platform GUI framework
- [x/net/html](https://pkg.go.dev/golang.org/x/net/html) - HTML parser

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

# Style invalidation benchmarks (M3.4)
go test -bench=BenchmarkComputeInvalidation -benchmem ./internal/css/
go test -bench=BenchmarkBatchMutations -benchmem ./internal/css/
go test -bench=BenchmarkAffectedRuleIndices -benchmem ./internal/css/

# Layout store benchmarks (M4.1)
go test -bench=BenchmarkLayoutStore -benchmem ./internal/renderer/

# Fragment store benchmarks (M4.2)
go test -bench=BenchmarkFragment -benchmem ./internal/renderer/
go test -bench=BenchmarkScratchBufferPool -benchmem ./internal/renderer/

# DOM parser benchmarks
go test -bench=. -benchmem ./internal/dom/

# Atom and string interning benchmarks (M2.2)
go test -bench=. -benchmem ./internal/dom/atom/

# Deterministic engine corpus benchmarks (article, documentation, table, form, image, JavaScript-light, scrolling)
go test -bench=. -benchmem ./internal/engine/testpages/

# Full renderer benchmarks (layout, display list, viewport, scroll)
go test -bench=. -benchmem ./internal/renderer/
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
