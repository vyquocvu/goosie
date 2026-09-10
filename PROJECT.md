# Project: Goosie Browser Performance and Memory Optimization

## Architecture
Goosie is a modular Go browser engine. The performance and resource optimization scope covers:
- `cmd/browser`: Desktop GUI browser utilizing Fyne UI, `documentloader.Coordinator`, `renderer.Renderer`, and `js.Session`/`js.Runtime`.
- `internal/ui`: Fyne window canvas management, tab containers, loading indicator lifecycle, event handling, and `InteractiveRasterCanvas`.
- `internal/renderer`: Render tree construction, layout computation, display list generation, spatial Y-band indexing, `CanvasRenderer` viewport culling, and `internal/renderer/frame` tiled rasterizer / compositor.
- `internal/image`: Image loader, decoder, downscaler, byte-bounded LRU cache, and memory manager integration.
- `internal/memory`: Central resource manager, component budgets, eviction callbacks, and limit enforcement.
- `internal/js`: ECMAScript runtime (Goja), timer dispatching, `setInterval` clamping, `Runtime.Close()` lifecycle cleanup.
- `internal/profile`: Background profile persistence, worker tickers, and history/storage flushing.
- `internal/browsercontrol`: In-process automation service, navigation, real viewport scrolling, and screenshot capture.
- `test/e2e`: Playwright E2E visual comparisons, scroll latency benchmarks, and memory/CPU regression tests.

## Feature Inventory
| # | Feature | Description | Milestone | Source | Status |
|---|---------|-------------|-----------|--------|--------|
| 1 | Loading Bar Animation Leak Fix | Explicitly call `Stop()` on `ProgressBarInfinite` in `HideLoading()` and `Start()` in `ShowLoading()` | M1 | Survey E3 | DONE |
| 2 | Goroutine ID Stack-Parsing Optimization | Replace `runtime.Stack` text-parsing in `currentGoroutineID()` / `IsMainGoroutine()` with cached/atomic check | M1 | Survey E1, E3 | DONE |
| 3 | Profile & JS Timer Throttling | Throttle `internal/profile` 50ms ticker when idle; clamp `setInterval` to minimum 4ms; add `Runtime.Close()` to terminate timer dispatcher goroutine | M1 | Survey E2, E3 | DONE |
| 4 | Render-Constrained Image Downscaling | Downscale decoded images to target layout dimensions ($\times$ DPR) via `draw.BiLinear.Scale` and clamp maximum decode resolution | M2 | Survey E2 | DONE |
| 5 | Byte-Bounded Image Cache & Evictor | Upgrade `image.Cache` to enforce byte limits (32–64 MB); register with `memory.Manager.RegisterEvictor` and call `UpdateUsage` | M2 | Survey E1, E2 | DONE |
| 6 | Session Navigation Resource Cleanup | Clear image cache on page navigation, close JS runtime timer dispatchers, and bound HTTP network cache | M2 | Survey E2 | DONE |
| 7 | Spatial Indexing (YBands) Repair | Rebuild `YBands` post-`SortByZIndex`, fix non-monotonic band assignment for out-of-order commands | M3 | Survey E1 | PLANNED |
| 8 | Unconditional / Low-Threshold Viewport Culling | Remove/lower `len(displayList.Commands) > 3000` gate in `canvas.go` so all pages cull offscreen canvas objects and GPU textures | M3 | Survey E1, E2 | PLANNED |
| 9 | Image Batch Relayout Debouncing | Debounce and coalesce image load completion flushes to prevent cascading full document relayouts and cache thrashing | M3 | Survey E1, E2 | PLANNED |
| 10 | InteractiveRasterCanvas Scroll Integration | Wire `TiledRasterizer` and `TileCache` to `InteractiveRasterCanvas` or optimize fast-path viewport scroll translation | M3 | Survey E1 | PLANNED |
| 11 | Realistic Scroll & Frame Latency Benchmarks | Implement real scroll dispatch in `engineContext.Scroll` and add frame-latency assertions (<16ms) in scroll benchmarks | M4 | Survey E1 | PLANNED |
| 12 | Visual Fidelity & Regression Conformance | Ensure 100% pass rate across `go test ./...`, layoutgolden tests, and Playwright visual comparisons (`TestComprehensiveSuite`) | M5 | Survey All | PLANNED |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| E2E | E2E Testing Track | Performance & Memory Benchmark Suite, Conformance Harness, and Headless Repro | None | DONE |
| M1 | Idle CPU & Goroutine Optimization | `internal/ui/browser.go`, `internal/ui/runtime.go`, `internal/profile/profile.go`, `internal/js/runtime.go` (Features 1-3) | None | DONE |
| M2 | Memory Footprint & Image Caching Bounds | `internal/image/`, `internal/memory/`, `internal/renderer/renderer.go`, `cmd/browser/main.go` (Features 4-6) | None | DONE |
| M3 | Smooth Scroll, Spatial Indexing & Viewport Culling | `internal/renderer/canvas.go`, `internal/renderer/display_list.go`, `internal/ui/raster_canvas.go` (Features 7-10) | M1, M2 | PLANNED |
| M4 | Realistic Scroll Benchmarks & Repro Verification | `internal/browsercontrol/`, `test/e2e/`, `examples/scroll_perf_demo`, `cmd/perf-review/` (Feature 11) | M3 | PLANNED |
| M5 | Final Conformance, Visual Verification & Audit | Full E2E Suite, visual comparisons per AGENTS.md, adversarial stress testing, and forensic integrity audit (Feature 12) | E2E, M1, M2, M3, M4 | PLANNED |

## Interface Contracts
### `internal/image` ↔ `internal/memory`
- `image.Cache` implements byte tracking and exposes `Evict(targetBytes uint64) uint64`.
- `memory.Manager.RegisterEvictor(memory.ComponentImage, evictor)` is called upon renderer/image loader initialization.
- `memory.Manager.UpdateUsage(memory.ComponentImage, bytes)` is called on each cache insert and eviction.

### `internal/renderer/display_list` ↔ `internal/renderer/canvas`
- `buildYBands` runs strictly after `SortByZIndex` sorts `dl.Commands`.
- `dl.YBands` correctly maps vertical screen bands to command slice ranges regardless of non-monotonic command Y coordinates.
- `CanvasRenderer.RenderWithViewport` uses spatial culling for all documents, preventing offscreen objects from entering Fyne's scene graph.

### `internal/ui/browser` ↔ Fyne Widgets
- `ShowLoading()` calls `loadingBar.Start()` and `Show()`.
- `HideLoading()` calls `loadingBar.Stop()` and `Hide()`, ensuring Fyne animation tickers are canceled on idle.

## Code Layout
- `cmd/browser/`: Desktop browser application (Fyne UI)
- `cmd/headless/`: CLI headless renderer
- `cmd/perf-review/`: Performance benchmarking CLI
- `internal/engine/`: Engine lifecycle, coordination, session management, event loop, navigation, and document loader
- `internal/net/`: Network fetching, connection pooling, cookie jar, cache, CSP
- `internal/dom/`: DOM parser, node tree, query selector matching, atom table
- `internal/css/`: CSS parser, selector compiler, cascade, properties, specificity
- `internal/renderer/`: Render tree, style application, layout, display list, raster surfaces, tile compositor
- `internal/renderer/frame/`: Tiled parallel rasterizer, tile cache, compositor, and cache bounds
- `internal/js/`: ECMAScript runtime (Goja), DOM bindings, polyfills, frame scheduler, timer pooling
- `internal/browsercontrol/`: In-process browser automation service, navigation, scroll dispatch
- `internal/ui/`: Desktop browser UI (Fyne) and DevTools dock (`internal/ui/devtools/`)
- `internal/image/`: Image loading, decoding, downscaling, byte-bounded caching
- `internal/memory/`: Memory management and resource limit enforcement
- `internal/profile/`: Profiles, bookmarks, history, persistent storage, background tickers
- `test/e2e/`: E2E tests, Playwright visual comparison (`TestComprehensiveSuite`), performance benchmarks
