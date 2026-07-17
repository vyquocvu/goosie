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
  │                                                          │
  ├── <script src="...">                                    │
  │     resolve against document URL                         │
  │       → CSP script-src check                              │
  │       → schedule using blocking / defer / async semantics│
  │       → HTTP fetch → decode / compile → execute (internal/js)
  │       → DOM or CSSOM mutation ───────────────────────────┤
  │                                                          │
  ├── <style> → CSS parse / CSSOM rule storage ──────────────┤
  ├── inline <script> → CSP check → execute ─────────────────┤
  └── images / fonts / other resources → fetch + decode      │
                                                             │
  ┌──────────────────────────────────────────────────────────┘
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
| `ARCHITECTURE.md` | Full system architecture, component flow, all subsystems |
| `PERFORMANCE.md` | Performance optimizations, benchmarks, profiling |
| `MEMORY_MODEL.md` | Cache budgets, eviction, memory management |
| `PACKAGE_OWNERSHIP.md` | Package boundaries, responsibilities, import rules |
| `docs/adr/0003-raster-backend-boundaries.md` | ADR for raster backend interface and selection |
| `docs/SUPPORTED_WEB_PLATFORM.md` | Supported HTML/CSS/JS feature matrix |
