# Pure-Go WebView Architecture

This document defines the rendering pipeline, raster backends, Fyne shell boundary, and platform WebView exclusion policy for the Goosie engine.

---

## 1. Rendering Pipeline

The complete pipeline from navigation to pixel output:

```
Navigation
  │
  ▼
Resource Loading ──────► HTTP fetch (internal/net)
  │                        Streaming body + discovery
  ▼
HTML/CSS Parsing ───────► Streaming tree construction (internal/dom)
  │                        CSS parsing + rule storage (internal/css)
  ▼
DOM + Stylesheet Storage ► Compact index-based DOM store (ADR 0001)
  │                        Computed-style pool (internal/css)
  ▼
Style Resolution ───────► Selector matching (compiled rules)
  │                        Incremental invalidation (M3.4)
  ▼
Layout + Fragments ─────► Layout store + fragment store (internal/renderer)
  │                        Incremental reflow (M4.4)
  ▼
Display List ───────────► Backend-neutral DisplayCommandList (ADR 0002)
  │                        Paint chunks keyed by LayoutID (M5.2)
  ▼
Raster ─────────────────► Backend interface (CPU / CoreGraphics)
  │                        Dirty-region only raster (M5.3)
  ▼
Composition + Present ──► Tile cache, compositor (M7)
                           Fyne pixel buffer present (M6.4)
```

Each phase has explicit inputs, outputs, and metrics recorded by the `internal/engine/metrics` recorder.

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

```
Compositor.BeginFrame()     ──► Backend.BeginFrame(FrameInfo)
Compositor.Rasterize(list)  ──► Backend.Rasterize(list, dirtyRegions)
Compositor.EndFrame()       ──► Backend.EndFrame()
                              ──► FyneAdapter.PresentFrame(buffer)
```

The compositor owns the frame cycle. The raster backend never touches layout or DOM.

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
`ColorRGBA` (packed uint32), `RectF` (float32), `PointF` (float32).
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

The policy (`None`, `UserRequested`, `Allowlist`, `FailureThreshold`) determines the fallback action — always within the pure-Go engine, never via a platform WebView.

---

## 5. Build Variants

| Variant | Build Command | Backend | Fyne GUI |
|---|---|---|---|
| Pure Go | `go build ./cmd/browser` | CPU only | Yes |
| Headless | `go build -tags headless ./cmd/headless` | CPU only | No (`image.RGBA` output) |
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
