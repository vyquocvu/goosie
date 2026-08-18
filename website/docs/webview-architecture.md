# Pure-Go WebView Architecture

This document defines the rendering pipeline, raster backends, Fyne shell boundary, and platform WebView exclusion policy for the Goosie engine.

---

## 1. Rendering and Resource Pipeline

The complete browser lifecycle includes a main-document path, subresource branches,
script execution, and mutation-driven updates. External CSS and JavaScript are not
part of a single linear parse step:

```
Navigation
  │  create navigation ID, cancellation scope, document priority
  ▼
Resolve URL + Fetch Main Document ──► HTTP request/redirect/response (internal/net)
  │                                     response headers, MIME type, CSP, body
  ▼
HTML Tokenization + Tree Building ──► DOM construction (internal/dom)
  │
  ├── <link rel="stylesheet" href="...">
  │     resolve against document URL
  │       → CSP style-src check
  │       → schedule as blocking CSS
  │       → HTTP fetch
  │       → CSS parse / CSSOM rule storage (internal/css)
  │       → style invalidation ───────────────────────────────┐
  │                                                           │
  ├── <script src="...">                                      │
  │     resolve against document URL                          │
  │       → CSP script-src check                              │
  │       → schedule using blocking / defer / async semantics │
  │       → HTTP fetch → decode / compile → execute (internal/js)
  │       → DOM or CSSOM mutation ────────────────────────────┤
  │                                                           │
  ├── <style> → CSS parse / CSSOM rule storage ───────────────┤
  ├── inline <script> → CSP check → execute ──────────────────┤
  └── images / fonts / other resources → fetch + decode       │
                                                              │
  ┌───────────────────────────────────────────────────────────┘
  ▼
DOM + CSSOM / Stylesheet Storage ► Compact index-based DOM store (ADR 0001)
  │                                 Computed-style pool (internal/css)
  ▼
Style Resolution ───────────────► Cascade + selector matching (compiled rules)
  │                                 Incremental invalidation (M3.4)
  ▼
Layout + Fragments ─────────────► Layout store + fragment store (internal/renderer)
  │                                 Incremental reflow (M4.4)
  ▼
Display List ───────────────────► Backend-neutral DisplayCommandList (ADR 0002)
  │                                 Paint chunks keyed by LayoutID (M5.2)
  ▼
Raster ─────────────────────────► Backend interface (CPU / CoreGraphics)
  │                                 Dirty-region-only raster (M5.3)
  ▼
Composition + Present ──────────► Tile cache, compositor (M7)
                                    Fyne pixel-buffer present (M6.4)
```

### Resource Loading and Ordering

Every discovered URL must be resolved against the final document URL after redirects,
checked by CSP before the request, attached to the active navigation's cancellation
scope, and fetched through `internal/net`. The navigation scheduler assigns priorities:

| Resource | Discovery | Scheduling and render effect |
|---|---|---|
| Main document | Navigation request | `PriorityDocument`; creates the parsing input |
| External stylesheet | `<link rel="stylesheet" href>` | `PriorityBlockingCSS`; blocks the first fully styled frame |
| Classic script | `<script src>` | `PriorityScript`; parser-blocking unless `defer` or `async` applies |
| Inline style/script | `<style>` / `<script>` | No fetch; parse or execute at its document position |
| Image | `<img src>` and CSS image values | Visible images outrank deferred/offscreen images; decode invalidates paint/layout as needed |

Classic script ordering must follow HTML semantics:

- A parser-blocking script pauses tree construction until it is fetched and executed.
- A `defer` script may fetch in parallel but executes in document order after parsing.
- An `async` script executes when ready without preserving document order.
- `DOMContentLoaded` fires after parsing and deferred scripts; `load` waits for required
  page subresources. ES modules remain governed by the unsupported-feature policy below.

CSS fetched from a URL is parsed into the same stylesheet set as inline CSS. A newly
available stylesheet invalidates computed style, then only affected layout, display-list,
and raster work is repeated. `@import`, fonts, and CSS image URLs should enter the same
resource scheduler when their owning CSS rule is parsed.

### Current Implementation Status

The diagram above is the intended end-to-end architecture. The active browser path has
these transitional limitations:

- `cmd/browser.loadPageAsync` (deprecated; replaced by `loadPageAsyncWithCoordinator`)
  obtains an HTTP response stream, but reads the complete main-document body before
  `internal/renderer.RenderHTML` parses it. The streaming DOM parser can report CSS,
  script, and image discoveries through `OnResource`, but that discovery callback is
  not yet connected to the browser's subresource scheduler.
- `internal/renderer.loadExternalCSS` discovers `<link rel="stylesheet">` after the full
  document parse, resolves and CSP-checks each URL, fetches it asynchronously, appends its
  rules, and triggers a style/layout refresh. The first frame can therefore appear before
  render-blocking CSS is available.
- `cmd/browser` executes all inline scripts after the first render, then fetches and
  executes all external scripts. This does not yet preserve mixed inline/external document
  order or implement parser-blocking, `defer`, and `async` timing.
- JavaScript DOM mutations currently serialize and render the document again. The target
  path is mutation-specific style/layout/paint invalidation without rediscovering and
  refetching unchanged subresources.

Each phase has explicit inputs, outputs, and metrics recorded by the
`internal/engine/metrics` recorder.

---

## 2. Raster Backends

Two backends implement the `Backend` interface (`internal/renderer/frame/raster`):

| Backend | Platform | Implementation | Speed vs CPU | Build Tag |
|---|---|---|---|---|
| CPU | All | Pure Go (`image.RGBA` + `image/draw`) | 1× (baseline) | None (always available) |
| CoreGraphics | macOS only | CGo bindings to CoreGraphics C API | 4-10× faster fills | `darwin && cgo` |

### Backend Selection (M11.3)

Selection is automatic via `NewBackend()`:
1. If `WithBackend(CoreGraphics)` forced → try CG, fall back to CPU with `WithCrashRecover`.
2. If no forced backend → auto-detect: CG on `darwin && cgo`, CPU everywhere else.
3. On auto-detect failure → CPU fallback.

Callers receive `(Backend, BackendType, error)` — the `BackendType` is logged as a metric.

### Frame Lifecycle

The frame cycle is driven by the `Backend` interface directly. A `Compositor` abstraction
is planned (M7) but not yet implemented; callers invoke the backend methods inline:

```
caller.BeginFrame(vp)       ──► Backend.BeginFrame(vp frame.Viewport)
caller.Rasterize(list, dr)  ──► Backend.Rasterize(list []DisplayCmd, dirty []frame.Rect)
caller.EndFrame()           ──► Backend.EndFrame()
                              ──► FyneAdapter.PresentFrame(buffer)
```

The raster backend never touches layout or DOM.

---

## 3. Fyne Shell Boundary

Fyne is strictly the window/pixel-presentation shell. It handles:
- Window management and OS event loop
- Keyboard and mouse input delivery
- Pixel buffer display via `canvas.Image`
- Menu bar, dialogs, and browser chrome

### Import Prohibition

The following packages **must never import `fyne.io/fyne/v2`** or any Fyne subpackage:

```
internal/dom
internal/css
internal/renderer/frame/*
internal/engine/*
internal/js
internal/net
internal/memory
internal/form
internal/image
```

Only these packages may import Fyne:

```
internal/ui          — Browser shell, developer tools
internal/renderer/fyne_adapter.go  — Pixel buffer bridge (single file)
cmd/browser          — Entry point
```

### Fyne Adapter

The `FyneAdapter` (`internal/renderer/fyne_adapter.go`) is the sole bridge:

- Receives `*image.RGBA` from the raster backend.
- Updates a single `canvas.Image` in-place (no widget tree rebuild per frame).
- Thread-safe: concurrent frame production + UI-thread presentation via `fyne.Do()`.
- Must be called on the UI thread for `PresentFrame()`.

### No Fyne Types in Display Commands

Display commands (`DisplayCommandList`) use only backend-neutral types:
`frame.Color` (packed uint32 RGBA), `RectF` (float32).
No `fyne.CanvasObject`, `fyne.Color`, or Fyne type appears in engine core.

---

## 4. Platform WebView Exclusion Policy

**Goosie does not use platform WebViews.** This is a hard architectural rule.

### Excluded Technologies

| Technology | Exclusion reason |
|---|---|
| WKWebView (macOS/iOS) | Platform-specific, Objective-C/Swift dependency, process model mismatch |
| WebView2 (Windows) | COM dependency, Edge runtime requirement, Windows-only |
| CEF / Chromium Embedded Framework | C++ dependency, large binary, different threading model |
| Any embedded browser engine | Defeats the purpose of a pure-Go engine |

### Rationale

1. **Architecture consistency.** A platform WebView would bypass the entire Go rendering pipeline — layout, style, display list, raster — making the custom engine pointless.
2. **Portability.** WebViews are platform-specific. Using them would require per-platform code paths, fragmenting the codebase.
3. **Testability.** WebViews cannot be tested in headless mode without a full browser runtime.
4. **Performance control.** The custom engine provides predictable memory use, measurable rendering, and explicit ownership — properties that cannot be guaranteed through a platform WebView.

### What Happens Instead

When a page requires unsupported features (canvas, video, iframe, ES modules, WebSocket, Web Worker), the engine's fallback layer (`internal/engine/fallback`) decides how to respond:

- **Parse-time detection** (`OnUnsupportedFeature` callback): Fired when `<canvas>`, `<video>`, `<audio>`, `<iframe>`, `<script type="module">`, `<object>`, or `<embed>` elements are encountered during streaming parse.
- **Runtime detection** (`OnRuntimeUnsupportedFeature` callback): Fired from JS when `document.createElement('canvas')` or similar is called.
- **JS feature detection** (`ScanAndReportUnsupportedJSFeatures`): Pre-scans script source for `import()` expressions.

The policy (`None`, `UserRequested`, `UnsupportedFeature`, `Allowlist`, `FailureThreshold`) determines the fallback action — always within the pure-Go engine, never via a platform WebView.

---

## 5. Build Variants

| Variant | Build Command | Backend | Fyne GUI |
|---|---|---|---|
| Pure Go | `go build ./cmd/browser` | CPU only | Yes |
| Headless | `go build ./cmd/headless` | CPU only | No (`image.RGBA` output) |
| macOS accelerated | `go build ./cmd/browser` (on darwin) | Auto: CG or CPU | Yes |

The headless variant (`cmd/headless`) enables scripted rendering without opening a window, useful for automated testing and server-side rendering.

---

## 6. Key Architecture Documents

| Document | Covers |
|---|---|
| `architecture-deep-dives.md` | Subsystem architecture and component flows |
| `memory-model.md` | Memory ownership, budgets, and profiling guidance |
| `memory-model.md` | Cache budgets, eviction, memory management |
| `package-ownership.md` | Package boundaries, responsibilities, import rules |
| `adr/0003-raster-backend-boundaries.md` | ADR for raster backend interface and selection |
| `supported-web-platform.md` | Supported HTML/CSS/JS feature matrix |

---

## 7. Incremental Interaction and Render Architecture

Goosie must use a retained, incremental pipeline rather than treating HTML serialization as the runtime DOM protocol. The goal is to outperform Blink for its target workload: fast startup, predictable resource usage, headless rendering, automation, and pages within the supported platform subset.

### Runtime Pipeline

```text
Input / navigation
  │
  ├── high-priority input queue
  ├── navigation and resource scheduler
  └── JavaScript event loop
        │
        ▼
Live index-based DOM store ◄── typed mutation records
        │
        ├── CSS invalidation
        ├── incremental style resolution
        ├── smallest valid layout/reflow roots
        ├── dirty display-list chunks
        └── dirty raster tiles
                 │
                 ▼
          compositor / tile cache
                 │
                 ▼
          Fyne pixel presentation
```

JavaScript DOM operations must mutate the live Go DOM through stable `NodeID` handles. The normal mutation path must not serialize the entire DOM, parse HTML again, or refetch unchanged resources. Full serialization remains available for debugging, snapshots, and compatibility fallback only.

### Mutation Batches

The event loop runs one render transaction after a macrotask and its microtasks complete. Multiple DOM operations in one task produce one typed batch:

```go
type MutationRecord struct {
    Kind      MutationKind
    Target    dom.NodeID
    Parent    dom.NodeID
    Attribute string
    OldValue  string
    NewValue  string
    Added     []dom.NodeID
    Removed   []dom.NodeID
}
```

The mutation batch is classified before rendering:

| Mutation | Minimum work |
|---|---|
| Color, background, focus outline | Repaint affected chunks |
| Text content | Text layout and repaint |
| Class, ID, or style attribute | Selector/style invalidation |
| Width, height, margin, font, text wrapping | Incremental reflow and repaint |
| Child insertion or removal | Parent/subtree reflow and dirty old bounds |
| Transform or opacity | Compositor update when layer-safe |
| Data attribute or event listener | JavaScript state only |

DOM state changes are synchronous and immediately observable by JavaScript. Style, layout, paint, and raster work are deferred until the transaction flush. APIs such as `getBoundingClientRect()` may force a targeted layout flush when required for correctness.

### User Interaction

Input must be separated from document rendering:

```text
Fyne event
  ▼
Input router
  ├── scroll → compositor viewport transform
  ├── pointer down → hit test and pointer capture
  ├── pointer move → captured target or selection update
  ├── pointer up → click/drag completion
  └── keyboard → focused DOM node and JavaScript event dispatch
```

High-frequency scroll and pointer-move events use latest-value coalescing. Clicks, pointer-up events, and key events are never dropped. Pointer capture avoids repeated full-tree hit testing during drag operations.

Scrolling must not traverse the DOM or recompute layout. The compositor updates the viewport transform immediately, reuses visible display-list and raster tiles, and schedules only newly exposed tiles. Selection and caret movement invalidate only their old and new overlay rectangles.

### Parallel Go Execution Model

Use one owner for mutable document state and bounded workers for independent work:

```text
Document owner goroutine
  ├── JS tasks and DOM mutations
  ├── style invalidation decisions
  └── frame transaction assembly

Worker pools
  ├── CSS/script/image/font fetch and decode
  ├── selector matching and style computation
  ├── independent layout subtrees
  ├── display-list chunk construction
  └── dirty tile rasterization
```

Workers communicate with immutable inputs and typed results. The document owner applies results in document order and drops results from stale navigation revisions. This provides concurrency without allowing unsynchronized DOM mutation.

### Browser Engine Comparison

Blink, WebKit, and Gecko retain DOM, style, layout, display-list, layer, and tile structures and invalidate them incrementally. Goosie follows the same proven model but can be more efficient for its target scope by avoiding a large multi-process compatibility stack, using compact index-based storage, making scheduling explicit, and supporting pure-Go headless execution.

Goosie should not attempt to beat Blink on general web compatibility. It should beat Blink on measurable target workloads:

- time to first useful frame;
- interaction latency during scroll and DOM mutation bursts;
- memory per page and per tab;
- headless startup and screenshot throughput;
- deterministic cancellation and stale-work elimination;
- automation round-trip latency;
- CPU cost for supported HTML/CSS/JS pages.

Every optimization must be validated with phase metrics, benchmarks, `go test -race`, CPU profiles, heap profiles, and visual comparison fixtures. The primary acceptance rule is that user interaction remains responsive while script, resource loading, layout, and raster workers are active.

### Implementation Priorities

1. Replace the JS `func(string)` mutation callback with typed mutation batches.
2. Stop serializing and reparsing HTML after normal DOM mutations.
3. Connect CSS invalidation to `renderer.ReflowTracker` and dirty display chunks.
4. Add one frame scheduler that coalesces mutations and input at frame rate.
5. Keep scroll, transforms, selection, and caret updates on compositor/overlay paths.
6. Add prioritized tile rasterization and stale revision cancellation.
7. Measure against Blink using first-paint, input-latency, memory, and throughput fixtures.

The architectural rule is: HTML is an input format, not a runtime update format; interaction is state plus invalidation, not a full document render.
