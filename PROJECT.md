# Project: Goosie Browser

## Architecture

Goosie is a browser engine written in Go. It implements its own web front end — a
real DOM, a CSS cascade, style resolution, and layout — and renders through a fixed
256px tile frame path: a CPU tile rasterizer with a worker pool, a damage-blit
composer, and a vsync-paced UI loop.

The engine pipeline is **Parse → Style → Layout → Paint**, wired in
`internal/engine/engine.go`. `Session.NewSession` (`engine.go:259`) runs it end to end:

1. **Admission + parse** — byte cap checked, then `dom.ParseBounded` under node,
   depth, and attribute caps (`engine.go:264-278`).
2. **Scripts** — `<script>` bodies execute in document order against the same DOM
   through the goja runtime in `internal/js`; errors are non-fatal (`engine.go:282-285`).
3. **Stylesheets** — `<style>`, inline `style=""`, and `<link rel=stylesheet>` are
   collected in one document walk, with linked sheets fetched concurrently into
   reserved slots so document-order cascade position survives (`limits.go:473-616`);
   `@import` expands to depth 4.
4. **Fonts** — `@font-face` jobs are fetched and registered before style resolution,
   or deferred for progressive paint (`engine.go:471-483`, `:338-351`).
5. **Style / cascade** — `style.ResolveViewport` prepends the UA sheet, builds a
   key-bucketed rule index, and computes per-element style. Cascade entries sort by
   importance split, then origin, then specificity, then document order
   (`internal/style/style.go:705-735`). Pseudo-elements resolve separately.
6. **Images** — `<img src>` and CSS `background-image` refs are fetched under a worker
   and byte/pixel budget, each passing `image.Probe` admission before decode
   (`engine.go:485-688`).
7. **Layout** — `Reflow` (`engine.go:727`) builds the box arena (`layout.Build`) and
   runs the passes in a fixed order: block, inline, float, flex, grid, table, then
   positioning for relative/absolute/fixed.
8. **Paint** — `PaintChecked(scale)` validates scale, arena, and document extent, then
   `paint.Builder` walks the arena into a frozen per-layer display list.
9. **Raster + composite** — `raster.Scheduler` plans tiles per frame, the worker pool
   rasterizes them, `surface.Composer` damage-blits into the backing store, and
   `surface.Loop` drives BeginFrame → Submit → ComposeInto → Present per vsync.

Post-load JS mutations flow through an invalidation batch; `Session.Refresh` re-styles
and re-lays-out only the affected subtrees, and a scroll-only plan is a no-op
(`internal/engine/refresh.go`).

These are implemented subsets, not complete conformance. `docs/roadmap-v2.md` holds the
current production-readiness review and the ordered release gates.

## Code Layout

### `internal/`

| Package | Responsibility |
| --- | --- |
| `archtest` | Mechanically enforces import boundaries, cgo confinement, gofmt, and the no-mutable-globals rule. No shipped binary imports it. |
| `ax` | Accessibility tree node types derived from a laid-out document. Zero dependencies so engine, surface, and platform share one node type. |
| `bookmarks` | Profile's starred pages as an ordered JSON list persisted to disk. |
| `css` | CSS tokenizer and stylesheet parser: selectors, values, `calc()`, `@keyframes`, `@font-face`, transitions, transforms, filters, `@media`/`@supports`, shadow-DOM CSS. |
| `dom` | HTML parser building the `Document` tree with interned atoms, bounded parsing, and DOM events. |
| `download` | Decides whether a response must be saved instead of rendered, and writes it collision-free. |
| `engine` | Wires the front end into the frame path: `Session`, events, focus, forms, selection, IME, animations, transitions, invalidation, refresh, accessibility, and every admission limit. |
| `frame` | Geometry, premultiplied bitmaps, tile grid, layers, plans, bitmap pool, frame recorder. The dependency leaf: it imports nothing from the rest of the engine. |
| `history` | Profile's global visit log, capped and persisted. |
| `image` | Image decode admission: `Probe` and `Decode` under hard size, dimension, and pixel limits. |
| `js` | Per-document JavaScript runtime on goja with DOM bindings, fetch/CORS/CSP, WebSocket, workers, storage, Canvas 2D, custom elements, shadow DOM, observers, URL, TextEncoding, FormData, AbortController, crypto, and performance. A `Runtime` belongs to exactly one document; nothing here is process-global. |
| `layout` | Builds a flat arena of positioned boxes from a styled DOM tree: block, inline, float, flex, grid, table, positioning, anonymous block wrapping. |
| `net` | HTTP transport, response cache, cookie jar, HTTP/2 config, and a dial-time private-address guard. Policy-free by design — the engine supplies the predicate. |
| `paint` | The display list: a frozen, flat sequence of drawing commands per compositing layer. Also the synthetic benchmark scene generator. |
| `platform` | Chooses and constructs the window backend. `platform/headless` is a `surface.Window` with no display; `platform/darwin` is an NSWindow paced by CVDisplayLink and is the only place cgo is allowed. |
| `raster` | Turns frozen paint lists into tile pixels: glyph atlas, per-tile CPU rasterizer, worker pool, and the `Scheduler` that sequences one frame. |
| `session` | Persists open tabs across restarts. |
| `style` | Style resolution and the CSS cascade: UA stylesheet, rule index, cascade ordering, inheritance, presentational hints, custom properties and `var()`, pseudo-elements, gradients, custom fonts. |
| `surface` | The presentation boundary: the window contract platforms implement, the event vocabulary that crosses it, the damage-blit `Composer`, and the UI-thread `Loop`. Depends on no platform code. |
| `tabs` | Tab-strip chrome: `TabManager`, per-tab session/layer/scroll, tab layout and drawing. |
| `toolbar` | Browser toolbar chrome: address-bar input, per-tab navigation history, layout and drawing. |

### `cmd/`

| Binary | Responsibility |
| --- | --- |
| `goosie` | The browser binary: the document engine behind a window. Interactive, `-gate`, and `-bench` modes share one code path. |
| `goosie-headless` | Renders an HTML file to a PNG through the same pipeline. |
| `wpt-runner` | Runs curated Web Platform Tests, driven by `testdata/wpt-config.json`. |
| `dump-url`, `dump-layout`, `dump-paint`, `dump-fonts`, `debug-layout`, `test_colors` | Diagnostics for parity and rendering work. |

## Import Rules

The full allowed-imports table lives in `internal/archtest/rules.go:40-64` and is
enforced by `go test ./internal/archtest/`. Anything absent from the table is a
violation, and an internal package with no entry has no engine imports allowed at all —
a new package must be added deliberately. `test/` and `cmd/` are free: they may reach
anything.

Two layering chains run through the table:

- **Frame path**: `frame` ← `paint` ← `surface` ← `raster` ← {`platform/*`, `toolbar`, `cmd/*`}.
  Nothing imports `raster` except a command binary and the tests.
- **Front end**: `dom` ← `css` ← `style` ← `layout`, all feeding `paint` and `engine`.
  `engine` may import `ax`, `dom`, `css`, `style`, `layout`, `paint`, `frame`, `image`,
  and `js`. `tabs` may import `engine`.

Also enforced: cgo appears in exactly one directory (`internal/platform/darwin`),
including via `//go:build` cgo tags; every `.go` file is gofmt-clean; `internal/` has no
mutable package-level globals (`cmd/` is exempt — a binary keeps its own flags and
counters); and no banned GUI toolkit is linked (`fyne.io`).

## Interface Contracts

### `surface.Window`
Platform backends implement `surface.Window`. The surface package knows nothing about
platforms; a backend satisfies the interface.

### `surface.Scheduler`
`Loop` depends on a scheduler *interface*, so `surface` never imports `raster`.

### `raster.Scheduler`
The scheduler reports the content version its tiles were painted at. A version bump
retries a tile that failed under the old one.

### `frame.Recorder`
Records frame plans and reports counters (style passes, layout passes, allocs) that the
gate suite asserts on.

### `Session.PaintChecked`
Binaries processing untrusted input must call `PaintChecked`, not `Paint`. It validates
scale, arena, and document extent before painting. `cmd/goosie` and `cmd/goosie-headless`
both use it; only the `cmd/dump-paint` diagnostic calls unchecked `Paint`.
`test/entrypoints/limits_test.go` guards this contract.

## Resource Limits

Per-document admission caps are declared in `internal/engine/limits.go:23-59` and
`internal/image/decode.go:14-18`. Notable ones: `MaxDocumentBytes` 8 MiB,
`MaxDocumentNodes` 50000, `MaxDocumentDepth` 128, `MaxCSSBytes` 4 MiB,
`MaxLayoutObjects` 131072, `MaxDocumentTiles` 65536, `MaxTileCacheBytes` 64 MiB,
`MaxImageDimension` 8192, `MaxDecodedImagePixels` 16 M. Image data is probed for encoded
size, dimensions, and pixel count *before* full decode, then re-checked after.

Network access additionally goes through a private-address guard
(`internal/net/address_guard.go`, `engine.AddressBlocked`) that rejects local hosts for
public initiators and https→http downgrades.

## Testing

```bash
go test ./...                            # everything
go test -race ./...                      # race detector
go test ./internal/archtest/             # import, cgo, gofmt, globals rules
go test ./test/gate -run TestGate -v     # gate suite
make gate bench wpt-pilot                # see Makefile
```

All tests pass on a machine with no display. Gates and surface tests run against
`platform/headless`, which exercises the same loop, scheduler, composer, and `Present`
path a real window does. Timing budgets are gated only on the macOS nightly
(`scripts/v2-gate-check.sh`), deliberately not on CI.

`test/` holds one directory per unit under test, mirroring `internal/`, plus five special
suites: `domtest` (fixture builder under engine resource caps), `entrypoints` (the
checked-paint contract for the binaries), `gate` (frame-path exit criteria, asserted on
counters rather than durations), `gatecheck` (tests the gate script itself), and `wpt`
(curated Web Platform Tests against a pinned upstream commit).

Supporting assets: `testdata/render/` is the HTML fixture corpus scored against
Chromium/Playwright references; `testdata/interaction-parity/` holds event fixtures with
a Chromium `reference-events.json` for interaction parity; `scripts/` holds the Playwright
verification scripts, the WPT and parity runners, and the footprint/release smoke checks.

## Visual Verification

Pure backend changes with no user-visible output require the automated tests above. When
UI features are added, visual verification through headless comparison against a
reference browser is required before the work can be considered complete.
