# Goosie v2, M1–M3: Frame Path to the 60fps Scroll Gate

## Status
**APPROVED FOR PLANNING** — child of
[2026-09-11-goosie-v2-architecture-design.md](2026-09-11-goosie-v2-architecture-design.md),
which owns the rationale for the architecture chosen here. This document is
decision-complete for implementation: file layout, types, signatures, thresholds.

## Goal

Deliver a Goosie that renders a real web page into its own macOS window, scrolls it with
real input at mean ≤ 16 ms / p99 ≤ 33 ms, with zero allocations on a warm scroll frame,
with no Fyne import anywhere in the tree.

## Scope

M1 (frame architecture, synthetic content) · M2 (ported front-end, first real page) ·
M3 (incremental correctness and the gate). Excluded: text selection, focus, forms (M4),
JavaScript (M5), tabs/history/devtools (M6).

## Constraints

- Fyne appears in **zero** v2 files. Not even test helpers (v1's `internal/testutil`
  imports Fyne and is therefore not portable — v2 gets its own).
- cgo appears only in `v2/internal/platform/darwin`.
- v1 keeps building and its suite stays green throughout M1–M3. Nothing under `internal/`
  or `cmd/` is deleted by this project.
- Shared fixtures under `testdata/` are read-only; v2 does not mutate v1 baselines.

## Repository layout

```
v2/
  cmd/goosie/            GUI entry point (macOS interactive, otherwise headless-only)
  cmd/goosie-headless/   PNG render CLI
  internal/platform/     implements surface.Window; owns no rendering policy
    darwin/              cgo: NSApplication/NSWindow/NSView, CVDisplayLink  [build: darwin && cgo]
    headless/            fake vsync clock, scripted events, PNG sink
  internal/frame/        vocabulary leaf: Rect/Point/Size/Color/Bitmap/Viewport, Layer,
                         TileGrid, Tile, TileCache, FramePlan   (imports nothing else in v2)
  internal/surface/      Window + Event interfaces, damage-blit Composer, UI-thread Loop,
                         FrameRecorder
  internal/raster/       tile rasterizer, GlyphAtlas, ImageCache, FontRegistry
  internal/paint/        DisplayList, DisplayCmd, content versions, Builder
  internal/layout/       arena + Object, block, inline, flex, grid, table
  internal/style/        cascade driver over ported css/, resolved ComputedStyle
  internal/engine/       session, event loop, navigation, Invalidation, frame scheduler
  internal/dom/          ported from v1
  internal/css/          ported from v1
  internal/net/          ported from v1
  internal/image/        ported from v1
  internal/archtest/     boundary enforcement
  test/                  v2 unit + headless integration
```

The `v2/internal/` prefix is load-bearing, not cosmetic: Go's `internal` rule makes
`v2/internal/…` importable only from inside `v2/…`, so v1 cannot accidentally depend on v2
and vice versa while both live in one module. Both trees are covered by `go build ./...`.

Import direction is strictly `frame` ← {`paint`, `raster`, `engine`, `surface`} ←
{`platform/darwin`, `platform/headless`, `cmd/*`}. `platform` implementations depend on
`surface` to satisfy its `Window` interface, never the reverse, so there is no cycle and
`frame` remains testable with nothing else linked in. `Bitmap` therefore lives in `frame`,
not `surface`.

## Boundary enforcement

`v2/internal/archtest/boundary_test.go` shells out to
`go list -f '{{.ImportPath}}|{{join .Imports "\n"}}' ./v2/...` and fails on:

| Forbidden | For packages |
|---|---|
| `fyne.io/…` | every v2 package, without exception |
| any import whose package contains a `//go:build cgo` tag | everything except `platform/darwin` |
| `platform`, `surface` | `dom`, `css`, `style`, `layout`, `paint`, `frame`, `raster`, `engine` |
| `raster`, `paint`, `layout`, `style` | `engine`'s **exported** signatures (engine may call them internally; it may not expose their types) |

This test runs in CI as part of the existing job set, on `ubuntu-latest`, needing no
macOS.

---

# M1 — Frame architecture with no web content

Proves the pipeline, the pacing, and the harness before a line of layout exists. Success in
M1 is measured with synthetic display lists, so a failure in M1 can only be the frame
architecture's fault.

## Types

```go
// package frame
type Rect struct{ X0, Y0, X1, Y1 int32 }        // device px
type RectF struct{ X0, Y0, X1, Y1 float32 }     // layout px
type Viewport struct{ Offset Point; Size Size }
type Color uint32                                 // RGBA, premultiplied
type TileCoord struct{ Col, Row int32 }

const TileSize = 256                              // device px, single tuning knob

type Tile struct {
    Coord    TileCoord
    Bounds   Rect
    Version  uint64
    LastUsed uint64
    Pixels   *frame.Bitmap                          // pooled, never GC'd mid-scroll
    Bytes    int64
    State    TileState                              // Empty | Valid | Stale | Failed
}

type Layer struct {
    ID             LayerID
    ContentVersion uint64
    DL             *paint.LayerDL                   // frozen once published
    Grid           *TileGrid
    Bounds         Rect                             // layer extent in device px
}
```

```go
// package frame — Bitmap lives with the other vocabulary types so the leaf stays a leaf
type Bitmap struct { RGBA []byte; W, H int; Stride int }

// package surface
type Window interface {
    Events() <-chan Event
    Present(buf *frame.Bitmap, damage []frame.Rect) error
    SetCursor(Cursor)
    ScaleFactor() float32
    Close() error
}

type Event struct {
    Kind   EventKind   // Vsync | ScrollDelta | Pointer | Key | Resize
    Delta  frame.Point // ScrollDelta, device px
    Size   frame.Size  // Resize
    Key    KeyInfo
    At     time.Time
}
```

## Components

- **`surface.Composer`** — owns the viewport-sized backing `Bitmap`; `Compose(tiles,
  viewport, damage)` blits only damaged tiles, using `memmove` for whole-row shifts when a
  scroll leaves content otherwise identical. Damage is what keeps this near the 3 ms budget
  instead of a naive 25 MB full recompose.
- **`raster.Tiler`** — given `*Layer` and needed tile coords, produces raster jobs. Calls
  `raster.RasterizeTile(dl, tileBounds, glyphAtlas, imageCache, out *frame.Bitmap) error`.
  Output buffers come from `frame.BitmapPool` — the pool sits in `frame` beside `Bitmap`
  because `raster` may not import `surface`. Pure over the frozen display list, so it is
  trivially parallel and unit-testable without a window.
- **`raster.Pool`** — `min(GOMAXPROCS-1, 4)` workers, buffered job channel, completion
  channel back to the UI thread. Recovers panics per job (failure policy in the parent
  spec).
- **`frame.Scheduler`** — vsync tick → coalesce input → resolve needed tiles → submit →
  compose → present. Depth-1 `FramePlan` channel, superseded plans dropped.
- **`surface.FrameRecorder`** — records `(vsyncAt, planAt, submitAt, composedAt,
  presentedAt)` per frame, ring-buffered, exportable as JSON. This is the instrument every
  later gate reads.

## Present-path contract with the shim

`Present(buf, damage)` must return before the next vsync, must not allocate per call, and
must copy nothing beyond the single `CGImage`/`layer.contents` handoff. The darwin
implementation wraps the **existing** backing memory through
`CGDataProviderCreateWithData` with no copy, and marshals the `CVDisplayLink` callback to
the main thread to emit `Event{Vsync}`.

## M1 exit criteria

All must pass before M2 starts.

1. `v2/cmd/goosie` opens a real macOS window driven by a synthetic display list; scrolling
   a 20,000 px procedural document with a trackpad is visually smooth and the tile cache
   plateaus at its budget rather than growing.
2. `go test ./v2/...` green on `ubuntu-latest` through `platform/headless`.
3. `archtest` green.
Gated in **CI** (`ubuntu-latest`, `platform/headless`, fake vsync). CI asserts deterministic
counters only and deliberately asserts **no** wall-clock latency: runner core counts and
memory bandwidth vary enough that a timing gate there would flake, and a flaky gate is
ignored within a month.

4. `BenchmarkWarmScroll` (1440×900 @ DPR 2, checkerboard + glyph stress, tile cache warm):
   **`allocs/op == 0`** and **tiles rasterized per frame after warmup == 0**.
5. `BenchmarkColdScrollBurst` (30 frames of 200 px into un-rasterized content):
   `allocs/op == 0`, and raster job count equals stale-tile count (no redundant work).
6. `v2` is GPU-free: no Metal/GL/Vulkan symbol in the binary (`nm` check in the test).

Gated on **macOS** (local run, then the nightly `macos-latest` job, real `CVDisplayLink`):

7. `BenchmarkWarmScroll` UI-thread work **≤ 5.0 ms/op**; `BenchmarkColdScrollBurst` mean
   **≤ 8.0 ms/op** at `min(GOMAXPROCS-1, 4)` workers.
8. Present-path latency and the two figures above recorded to
   `docs/perf/v2-m1-baseline.txt` as M3's reference.

If criterion 4 or 5 fails, the frame architecture is wrong. If only 7 fails, the fix is the
budget, tile size, or composition strategy. Neither is a front-end problem, which is the
point of proving M1 without web content.

---

# M2 — Ported front-end, first real page

## Port table and required adaptations

| v1 package | v2 target | Adaptation required |
|---|---|---|
| `internal/dom` | `v2/internal/dom` | Export a stable `NodeID`. Strip renderer back-references. Keep `atom`, `treebuilder`, `store` intact. |
| `internal/css` | `v2/internal/css` | None to the parser or selector compiler. `computed.go` is consumed by `v2/style` rather than used directly. |
| `internal/net` | `v2/internal/net` | Engine depends on a 4-method `HTTP` interface declared in `engine`, not on the concrete service, so navigation stays mockable. |
| `internal/image` | `v2/internal/image` | Decoder output must be premultiplied RGBA at the tile's target scale; the existing render-constrained downscale is kept as-is. |
| `internal/testutil` | **not ported** | Imports Fyne. v2 gets a Fyne-free `v2/test/harness`. |
| `internal/renderer` | **not ported** | Replaced wholesale by `style`/`layout`/`paint`/`raster`. |
| `internal/ui` | **not ported** | M6 territory. |

## Style

`style.ComputedStyle` is interned by `Fingerprint` and fully resolved: every length,
including `min-*`/`max-*`, padding, and border widths, is a `float32` by the time layout
sees it. `parseLength` does not exist in `layout/`. Invariant 4 is enforced by a test that
renders a page and asserts the layout package's parse-call counter stays at zero.

Percentage lengths re-resolve only when the containing block width changes; that event is
explicit in the invalidation record.

## Layout

Flat arena, `ObjectID`-indexed, one `Object` per element, mutated in place. Algorithms port
in the order block → inline → float → flex → grid → table, each with a direct-vs-v1
geometry comparison.

## Paint

`paint.Builder` walks the layout arena and emits `DisplayCmd`s into the layer slab.
`TextRun` carries **positioned** glyphs resolved from line boxes, so invariant (text
resolved at layout time) is structural rather than a convention.

## Golden strategy, and why v1's pixel hashes are not v2's oracle

- **Layout goldens are shared.** v2's geometry must match v1's within 1.0 px on
  `testdata/*.html` and `testdata/perf/*.html`, since these are numbers, not pixels. This
  cross-check catches genuine layout bugs and is the reason to port layout algorithm by
  algorithm rather than reinventing it.
- **v1's pixel-hash manifest is not carried over.** v2's glyph raster differs by
  construction from v1's Fyne-mediated path; comparing them would produce a wall of
  meaningless diffs. M2 establishes a **new v2 pixel baseline** at `v2/test/golden/`, and
  correctness against the web is asserted the way `AGENTS.md` requires:
  `CompareGoosieVsBrowser` against Chromium.
- **No v1 threshold is loosened.** v2 must satisfy, per fixture, the diff threshold v1's
  `TestComprehensiveSuite` currently uses. A fixture v2 fails outright is reported, not
  re-thresholded.

## M2 exit criteria

1. `go run ./v2/cmd/goosie -url=https://example.com` and
   `-url=file://…/testdata/perf/large_page.html` render correctly in a macOS window.
2. `go run ./v2/cmd/goosie-headless -in X -out Y.png` matches v2's new baseline for all 130
   `testdata/*.html` fixtures.
3. Layout geometry matches v1 within 1.0 px on the full `testdata` corpus.
4. Chromium comparison passes at v1's existing thresholds on the comprehensive suite;
   artifacts under `test/e2e/testdata/results/` inspected and the command, threshold,
   result, and paths recorded.
5. Static-frame `allocs/op == 0` on a no-change present (nothing dirty → nothing allocated).
6. First-paint stage timings recorded to `docs/perf/v2-m2-firstpaint.txt`. Tracked, **not**
   a gate.

---

# M3 — Incremental correctness and the gate

## The single invalidation tracker

```go
// package engine
type Reason uint8 // DOMMutation | StyleChange | ImageLoad | Scroll | Resize

type Invalidation struct {
    Rects   []frame.RectF
    Subtree []layout.ObjectID
    FullDoc bool
    Counters Counter // read by tests to prove invariant 1
}

func (i *Invalidation) Add(r Reason, rect frame.RectF, id layout.ObjectID)
func (i *Invalidation) Resolve(viewport frame.Viewport, grid *frame.TileGrid) Plan
```

`Scroll` is a first-class reason that carries a viewport offset only — it appends no rects
and no subtree, which is precisely how invariant 1 holds: a scroll resolution produces a
plan with `LayoutObjects == 0` and `StyleObjects == 0`.

v1's `InvalidationTracker`, `ReflowTracker`, and `DirtyFlag` are not ported. `engine`
exposes no invalidation API beyond this type; `archtest` enforces the rule.

## In-place refresh

`engine.Refresh` consumes `Invalidation.Subtree`, re-styles and re-lays-out those objects
under their nearest unmodified containing-block constraint, and bumps the content version
of intersecting layers. There is no tree clone and no whole-document pass. A
`MutationSink`-equivalent path exists in M3 for synthetic/scripted mutations only; the JS
binding attaches in M5 without changing this type.

## Gate fixture — none exists today

The existing `testdata/perf/` fixtures cannot carry the gate, and the earlier assumption
that they could was wrong:

| Fixture | Actual state |
|---|---|
| `testdata/perf/large_page.html` | 89,546 bytes, 344 lines, ~208 block/inline boxes. Real, but far too short to sustain 600 scroll frames and contains **no** `position: fixed`. |
| `testdata/perf/layout_sample.html` | **153 bytes** — a stub. |
| `testdata/perf/typography_sample.html` | **186 bytes** — a stub. |
| `examples/long_page.html` | 103 lines, demo content, not a benchmark input. |

M3 therefore authors `testdata/perf/gate_scroll.html`, generated rather than hand-written so
it is reproducible:

- New mode in `cmd/test-gen`: `go run ./cmd/test-gen -gate-scroll -out testdata/perf -seed
  1` — deterministic output, no clock or randomness dependence.
- Shape: **≥ 40,000 CSS px tall** at a 1,440 px viewport width (≈ 42 viewports, enough for
  600 × 100 px scroll frames to stay inside the document), a **60 px `position: fixed`
  header**, and a mix of roughly 60 % block text, 20 % flex rows, 15 % grid card sections,
  and 5 % images.
- Scale: 2,500–4,000 layout objects and ≥ 1,200 distinct text runs, which is the range where
  v1's per-frame relayout cost becomes visible.
- Its SHA-256 is recorded in `docs/perf/v2-m3-gate.txt` the moment it is generated, and the
  gate test fails if the file no longer hashes to it. An unfixed gate fixture is a gate that
  quietly moves.

## Gate harness

- `v2/test/gate/scroll_gate_test.go` (headless, fake vsync, **CI-gated**): drives 600 scroll
  events of 100 px through a warm cache, asserting `allocs/op == 0`,
  **`Counters.StylePasses == 0 && Counters.LayoutPasses == 0`** for every scroll-only frame,
  and a tile-miss rate under 5 %. Counter assertions only — no wall clock, so it does not
  flake on shared runners. The zero-style/zero-layout counter is the cheap, high-value guard
  against the exact regression class that capped v1.
- `macos-latest` nightly (new `v2` job): `go run ./v2/cmd/goosie -gate
  -url=file://…/testdata/perf/gate_scroll.html -frames 600 -out gate.json`, then
  `scripts/v2-gate-check.sh gate.json` asserting **mean ≤ 16.0 ms** and **p99 ≤ 33.0 ms** on
  real `CVDisplayLink` pacing, plus the M1 warm/cold `ms/op` budgets. Local equivalent
  documented in `v2/README.md` for pre-push runs.

## M3 exit criteria

1. Trackpad scroll of `testdata/perf/gate_scroll.html` in a macOS window: mean ≤ 16.0 ms,
   p99 ≤ 33.0 ms over 600 frames, on the nightly harness.
2. `allocs/op == 0` on a warm scroll frame.
3. Zero style and zero layout passes across all scroll-only frames, by counter.
4. Tiles prefetched ahead of scroll direction; tile-miss rate **< 5 %** across a sustained
   600-frame scroll, asserted by counter in CI and by the nightly report on macOS.
5. A hover state change, an image finishing load, and a scripted CSS-class mutation each
   invalidate only the intersecting tiles — asserted by tile-rasterized counters. (Form
   controls themselves are M4; M3 tests class toggling through the scripted mutation path.)
6. `archtest` green, meaning Fyne remains absent and cgo remains confined.
7. M2's Chromium comparison still passes at unchanged thresholds.

---

## Test plan summary

| Layer | Kind | Location | Runs on |
|---|---|---|---|
| frame, tile grid, validity | unit | `v2/test/frame/` | CI ubuntu |
| composer damage blit | unit + golden PNG | `v2/test/surface/` | CI ubuntu |
| tile rasterizer | unit, table-driven per `CmdKind` | `v2/test/raster/` | CI ubuntu |
| warm/cold scroll | benchmark + assertion | `v2/test/gate/` | CI ubuntu (fake clock) |
| allocs-per-frame | benchmark `allocs/op == 0` | `v2/test/gate/` | CI ubuntu |
| window + vsync pacing | manual + nightly | `v2/cmd/goosie -gate` | macOS nightly |
| layout geometry vs v1 | golden comparison | `v2/test/golden/` | CI ubuntu |
| pixels vs Chromium | E2E comparison | existing `CompareGoosieVsBrowser` | CI ubuntu + local |
| import boundaries | static | `v2/internal/archtest/` | CI ubuntu |

## Definition of done (M1–M3)

- All exit criteria above pass, and the gate has been run on real hardware, not only the
  fake clock.
- Zero `fyne.io` imports and zero cgo outside `platform/darwin` in `v2/`, by test.
- `docs/perf/v2-m1-baseline.txt`, `v2-m2-firstpaint.txt`, and a `v2-m3-gate.txt` gate
  report committed, so the next person can regress us.
- `v2/README.md` documents how to run the GUI, the headless renderer, and the gate locally.
- v1's own suite is still green, so a reader can compare the two implementations.
- Any accepted pixel-diff limitation against Chromium is explicitly written down with its
  measured value and reason — never absorbed by loosening a threshold.

## Decided, so no implementer needs to ask

- **Tile size 256 device px**, one constant, `frame.TileSize`. Justification and the
  96-tile/25 MB/12 ms math are in the parent spec; revisit only with M1 measurements.
- **CPU raster only.** The cgo CoreGraphics backend is not ported. `platform/darwin` is the
  sole cgo package.
- **New tree beside v1**, not a migration of v1 files, and not an in-place carve-out.
- **v1's pixel hashes are not v2's oracle**; layout geometry is the shared cross-check and
  Chromium is the correctness oracle.
- **First-paint vs Chromium is tracked, not gated**, in M1–M3.
- **`memory.Manager` is reused** for the tile and glyph budgets rather than a v2 bespoke
  policy — it already supports evictor registration and usage updates.
- **One scrolling layer in M1–M3.** Additional layers only where a stacking context plus
  `position: fixed` or a transform demands one. `position: fixed` support lands in M3 —
  not because existing fixtures need it (none of them contain it, verified by grep), but
  because the new gate fixture authors a fixed header deliberately: a scroll path that
  re-rasterizes a fixed element every frame passes no real-world gate.
