# Goosie v2: Frame-Path Architecture

## Status
**APPROVED FOR PLANNING** — supersedes no v1 document. First sub-project spec:
[2026-09-11-goosie-v2-m1-m3-frame-path-design.md](2026-09-11-goosie-v2-m1-m3-frame-path-design.md).

## Goal
Rebuild Goosie's frame-critical path so that real input-driven scrolling holds 60fps on
a heavy page, with no GUI toolkit anywhere below the application layer.

## Why a rebuild rather than continued optimisation of v1

Facts verified against the working tree on 2026-09-11:

- **The GUI toolkit is not confined to the UI layer.** 96 files import `fyne.io/fyne`
  (41 of them non-test), including core engine packages: `internal/renderer/canvas.go`,
  `renderer.go`, `inline_layout.go`, `export.go`, `font_metrics.go`, `fyne_adapter.go`.
  `MutationSink` reaches JS→DOM mutation through a `FyneAdapter`.
- **The live paint path is a Fyne widget per paint command.** `internal/renderer/canvas.go`
  (2,995 lines) contains 45 widget-construction call sites (`canvas.NewText`,
  `canvas.NewRectangle`, `widget.NewLabel`, …).
- **A single-surface path already exists but is dormant.** `internal/ui/browser.go` only
  wires `InteractiveRasterCanvas` when `Browser.UseRasterCanvas` is true, and nothing in
  `cmd/browser` ever sets it. Even that path presents through Fyne's renderer with no
  vsync pacing and no tile reuse.
- **Invalidation is triplicated and mutually uncoordinated:** `InvalidationTracker`
  (`invalidation.go:240`), `ReflowTracker` (`incremental_layout.go:60`), and `DirtyFlag`
  (`invalidation.go:203`). `ComputeIncrementalLayout` falls back to a whole-tree
  `ComputeLayout` when any node is dirty, so no incremental reflow occurs.
- **The tile architecture exists as an unused skeleton.** `internal/renderer/frame/`
  already contains `compositor.Tile{Coord, Bounds, Version, LastUsed, Image, ByteSize}`,
  `TileCache` with byte-budget LRU, a dirty-rect-aware `raster.Backend` interface, a
  parallel `TiledRasterizer`, and a `FontRegistry` on `x/image/font.Face` with no Fyne in
  it. It is reachable only from `headless.go`, `frame/golden`, and `fyne_adapter.go` —
  never from the GUI frame path.
- Micro-optimisations are being landed in v1 already (uncommitted: `Refresh()` clone
  removal, `hasNonZeroZIndex` sort skip, `parseLengthFast`). Those are worthwhile and they
  do not change the shape of the problem: the architecture has no vsync-paced present, no
  tile reuse, and no incremental layout.

Conclusion: the remaining gap is architectural, and the architecture that closes it is
different enough in module boundaries that a clean tree is cheaper than a migration.

## Scope of this document

v2 is decomposed into one architecture spec (this file) plus per-milestone specs. This
document fixes module boundaries, thread roles, data contracts, and the milestone
sequence. The **M1–M3 sub-project spec** is decision-complete for implementation; M4–M6
receive only the contracts they need to avoid being designed wrong later.

## Constraints

- No third-party GUI framework. `platform/` is the only package that talks to the OS.
- cgo is permitted **only** inside `platform/darwin`. The raster path is CPU-only.
- KISS: the smallest design that satisfies the gate, and every mechanism must name the
  invariant it serves.
- v1 remains buildable and its test suite remains green throughout; v2 lands in a new tree
  beside it and deletes nothing from v1 until a milestone's gate passes.
- Existing verification rules in `AGENTS.md` apply to user-visible changes: comparison
  against Chromium through `CompareGoosieVsBrowser`, with command, threshold, result, and
  artifact paths recorded. `UPDATE_SNAPSHOTS` is never used to hide a regression.
  One deliberate divergence: v1's pixel-hash manifest is **not** v2's oracle, because v2's
  glyph raster differs by construction from v1's Fyne-mediated path. v2 keeps v1's *layout
  geometry* goldens as a numeric cross-check and establishes its own pixel baseline.

## Acceptance gate

**One non-negotiable gate:** real input-driven scrolling of a heavy page at **mean ≤ 16 ms,
p99 ≤ 33 ms** frame interval, measured from vsync to present on macOS, with
`allocs/op == 0` on a warm scroll frame.

The page is `testdata/perf/gate_scroll.html`, **authored during M3** — it does not exist
today, and the existing `testdata/perf/` fixtures cannot carry the gate:
`large_page.html` is ~208 boxes with no `position: fixed`, while `layout_sample.html` and
`typography_sample.html` are 153- and 186-byte stubs. The fixture's shape, scale, and
pinned SHA-256 are specified in the M1–M3 sub-project spec.

Not gates, and explicitly not reasons to fail a milestone: first-paint time relative to
Chromium, idle CPU, memory footprint. Those are tracked as metrics with alarms, not
acceptance criteria.

## Architecture decision

**Adopted: retained layers + fixed tile grid + damage blit** (classic Chromium shape,
minimum viable version).

Rejected alternatives:

- **Immediate-mode full-frame raster.** Simplest possible, but a text-heavy viewport at
  DPR 2 costs 20–40 ms on CPU with no lever to pull: every fix (cache what did not change,
  re-raster only what did) is the adopted architecture re-derived. Rejected on measurement,
  not taste.
- **Vertically overscanned strips + subtree diff, no tile grid.** Meets the vertical-scroll
  gate for roughly 3k fewer LOC, but strips are the wrong shape for wide overflow,
  horizontal scroll, and `position: fixed`, and their memory scales with overscan instead
  of a budget. Rejected because the stated goal is a ceiling, not a single benchmark.
- **Multi-process isolation.** Out of scope for v2 entirely.

## Module map

Written fresh (frame-critical path):

| Package | Owns |
|---|---|
| `platform/` | Window, vsync, input, cursor, clipboard. Only OS-facing package. |
| `surface/` | `Window` and `Event` interfaces, damage-blit composer, UI-thread loop, `FrameRecorder`. |
| `frame/` | `Rect/Point/Size/Color/Bitmap/Viewport`, `Layer`, `TileGrid`, `Tile`, `TileCache`, `FramePlan`. The dependency leaf. |
| `raster/` | Tile rasterizer, glyph atlas, image cache, font registry. |
| `paint/` | `DisplayList`, `DisplayCmd`, content versions, builder over layout. |
| `layout/` | In-place layout arena; block, inline, flex, grid, table. |
| `style/` | Cascade over ported `css/`, interned resolved `ComputedStyle`. |
| `engine/` | Session, event loop, navigation, frame scheduler, **single invalidation tracker**, mutation sink. |

Ported and adapted from v1 (GUI-independent, not the performance problem):

`dom/` (parser, tree, atoms, selector matching) · `css/` (parser, cascade, specificity,
selector compiler) · `net/` (fetch, pool, cookie jar, cache) · `image/` (decode,
downscale, byte-bounded LRU) · `profile/` (bookmarks, history, storage).

Deferred to their own milestones: `js/` (M5, goja + bindings), `app/` (M6, tabs, history
UI, devtools dock).

## Dependency rule

No package at or below `engine/` may import `platform/`, `surface/`, or any package with a
cgo build tag. Concretely, `dom`, `css`, `style`, `layout`, `paint`, `frame`, and `raster`
import no OS or GUI package. Import direction is strictly
`frame` ← {`paint`, `raster`, `engine`, `surface`} ← {`platform/*`, `cmd/*`}: `engine` may
not import `surface` or `platform` either, so the frame loop lives in `surface` and the
window implementations depend on it rather than the other way round. This is the rule v1
breaks today.

The rule is **enforced by a test**, not by convention: a CI check walks package imports and
fails on a violation. An unenforced boundary rule is the mechanism by which v1 arrived at 41
non-test files importing a GUI toolkit.

## Thread model

Three roles, two handoffs:

- **UI thread** — pumps platform events, receives completed frames, blits the damage list,
  presents on vsync. Never rasterizes, never lays out. AppKit requires this be the main
  thread on macOS.
- **Engine thread** — navigation → parse → style → layout → display list → emits an
  immutable `FramePlan`.
- **Raster pool** — `min(GOMAXPROCS-1, 4)` workers; each rasterizes one tile into a pooled
  buffer. Reads frozen display lists only.

The only cross-thread object is `FramePlan`, passed by value over a depth-1 channel that
coalesces: a newer plan always replaces an unpublished one. No mutex is held across
rasterization.

## Invariants

Every mechanism in v2 exists to keep one of these true. A change that breaks one is wrong
regardless of how fast it benchmarks.

1. A scroll frame runs no style and no layout: cull, raster missing tiles, blit.
2. A tile is never rasterized on the UI thread. A not-ready tile blits its last valid
   content.
3. A DOM mutation reuses existing layout objects. Cost is proportional to the dirty
   subtree, never the document.
4. No string→length parsing during layout or paint. Every length resolves to `float32`
   once, at style time.
5. Exactly one invalidation tracker in the system.
6. A steady-state scroll frame allocates nothing that is not pooled.

Invariants 1 and 3 are asserted by counters in tests, so a regression is a failing build
rather than a slower laptop.

## Frame path

### Display lists

`paint.DisplayCmd` keeps v1's flat command shape (`Kind, Rect, Color, Border, Opacity,
TextRun, Image`) with three corrections:

- Commands come from a grow-only slab owned by the layer, not `make` per element per frame.
- Clip and opacity resolve into a flattened clip stack **at build time**; the rasterizer
  carries no stack while scanning commands.
- Z-order sorts once per content version, behind a `hasPositionedZChildren` bit computed at
  style time, so an all-zero document skips the sort.

A list publishes as `*LayerDL{Cmds, ContentVersion, Bounds}` and is frozen thereafter: the
raster pool reads it without locks or copies.

### One invalidation tracker

```go
type InvalidateReason uint8 // DOMMutation | StyleChange | ImageLoad | Scroll | Resize

type Invalidation struct {
    Rects   []frame.Rect // layout space
    Subtree []ObjectID   // layout objects needing re-style/re-layout
    FullDoc bool         // stylesheet add/remove, viewport resize only
}
```

Sources append to the current batch; at vsync the batch resolves **once** into (a) objects
to re-layout and (b) tiles whose bounds intersect `Rects`, which are marked stale by
bumping the containing layer's content version. `InvalidationTracker`, `ReflowTracker`,
and `DirtyFlag` are not ported.

### Tile grid

256×256 device px per tile, per layer, keyed by `TileCoord`. `compositor.Tile`/`TileCache`
are the reference, rewritten against `Invalidation`.

- **Validity rule, complete:** a tile is valid iff `tile.version >= layer.contentVersion`
  and no invalidated rect has intersected it since that version.
- **Budget:** viewport tiles + prefetch ring, hard-capped (default 64 MB), enforced through
  `memory.Manager`'s existing evictor rather than a bespoke policy.
- **Priority:** visible → ring ahead of scroll velocity → rest. Prefetching one viewport
  ahead of scroll direction is what protects p99 rather than the mean.

Scale, for a 1512×950 window at DPR 2: 3024×1900 device px ≈ 12×8 = **96 visible tiles**,
256 KB each ≈ 25 MB. Full-viewport CPU raster is about 12 ms across 4 workers — far too
slow per frame, which is the quantitative reason a warm scroll must cost zero tile work.

### Lifecycle

```
input ─► coalesce to one event/frame ─► engine: resolve invalidation
                                          │
                             layout affected subtree only
                                          │
                     rebuild display list only if style/layout changed
                                          │
                                    FramePlan (immutable)
                                          │
              UI: cull ─► mark needed tiles ─► submit stale tiles to pool
                                          │
              pool: rasterize into pooled buffers, return ready tiles
                                          │
              UI: blit damaged tiles ─► set layer.contents ─► vsync present
```

If raster cannot keep up, the UI thread blits the stale-but-valid tile and presents. It
never blocks. A dropped frame is one late tile next vsync; a blocked UI thread is a missed
vsync cascade, and the gate is on p99.

### Budget inside 16.6 ms

Input→plan 1.0 ms · tile submission 0.5 ms · blit 3.0 ms · present 1.5 ms · **≈10 ms raster
slack** (~40 tiles across 4 workers). Style and layout are absent from this budget by
invariant 1.

### Text

Line breaks, glyph advances, and per-glyph positions are computed **during layout** into
the line box. `TextRun` carries positioned glyphs; the rasterizer performs atlas lookup and
blit only. The glyph atlas sits over the existing byte-budgeted `cache.GlyphCache`
(`memory.Manager`-integrated), with `FontRegistry` system-font probing and
`go-text/typesetting` for shaping. Both are pure Go and neither is a GUI framework.

## Style and layout model

`ComputedStyle` is interned (v1's `Fingerprint` + pool approach, retained) and **fully
resolved**: every length, including `min-*`/`max-*`, padding, and border widths, becomes
`float32` at style time, removing `parseLengthWithViewport` from layout. Container-relative
percentages re-resolve only when the containing block width changes.

The layout tree is a flat arena of `layout.Object` indexed by `ObjectID`, not pointer-linked
nodes:

```go
type Object struct {
    DomID                     dom.NodeID
    Style                     *style.ComputedStyle // interned pointer, never copied
    Bounds                    frame.RectF
    Children                  []ObjectID           // paint order, resolved once
    Parent                    ObjectID
    MinContent, MaxContent    float32
}
```

One object per DOM element, created on first style and mutated in place; identity is
`DomID`. `Refresh` walks `Invalidation.Subtree`, re-styles and re-layouts those objects
under their nearest unmodified containing-block constraint, and bumps affected layers'
content versions. There is no tree clone and no whole-document relayout.

## Platform contract

```go
type Window interface {
    Events() <-chan Event            // Vsync | ScrollDelta | Pointer | Key | Resize
    Present(buf *frame.Bitmap, damage []frame.Rect) error
    SetCursor(Cursor)
    ScaleFactor() float32
    Close() error
}
```

**`platform/darwin`** (cgo, ~600–900 lines): `NSApplication` + `NSWindow` + one content
`NSView`. `Present` composes only damaged tiles into a persistent device-pixel RGBA buffer,
wraps that same memory as a `CGImage` via `CGDataProviderCreateWithData` (no copy), and
assigns it to the view's `layer.contents`. Pacing from a `CVDisplayLink` callback
marshalled to the main thread emitting `Event{Vsync}`. Scroll deltas, buttons, and keys map
to `Event`; `NSTextInputClient` arrives in M4 with selection.

**`platform/headless`** (no cgo, same interface): scripted event feed, **fake vsync clock**,
PNG written on `Present`. Serves two purposes: it is what CI runs on `ubuntu-latest`, and
the deterministic clock is what turns the frame budget into an automated allocation and
latency test instead of a manual eyeball.

Under `//go:build !darwin || !cgo`, only headless compiles and the GUI binary refuses to
start with an explicit message.

## Milestones

| # | Deliverable | Gate |
|---|---|---|
| **M1** | Shim + headless present + tile grid + raster pool, driven by a **synthetic** display list (scrolling checkerboard, glyph-atlas stress). No web content. | Frame path, pacing, and harness proven before any layout code exists. |
| **M2** | Ported leaves wired: `dom`+`css`+`net`+`image` → `layout` → `paint` → real static page in the window. SVG rides along via `oksvg` as today. | Layout geometry matches v1 within 1.0 px; v2 pixel baseline established; `CompareGoosieVsBrowser` green at v1's existing thresholds. |
| **M3** | Scroll, link click, hover; single invalidation tracker; in-place `Refresh`. | **The 60fps gate.** Plus counters proving a scroll frame ran zero style and zero layout passes. |
| M4 | Text selection, caret, copy, keyboard focus, forms. | Own spec. |
| M5 | JavaScript (goja), `rAF` driving the frame loop, timers. | Own spec. |
| M6 | Tabs, history, devtools dock on the engine API only. | Own spec; forbidden from importing `raster`/`platform`. |

M1–M3 are the first sub-project. M4–M6 each get their own spec and plan.

Two contracts are fixed now so later milestones are not designed wrong:

- **M5:** every JS DOM mutation enters through the batched `Invalidation` sink. `rAF`
  callbacks coalesce to one frame. No binding may call raster or platform directly.
- **M6:** devtools and tab toolbar consume the engine API only — they may read display
  lists, tile state, and `FrameRecorder` output, never mutate them.

## Verification

- `surface/FrameRecorder` timestamps vsync→present per frame and emits JSON, consumed by a
  CI artifact check.
- **Division of assertions between CI and hardware.** CI (`ubuntu-latest`, headless, fake
  vsync) gates deterministic counters only: warm-scroll `allocs/op == 0`, the
  zero-style/zero-layout counters, tiles-rasterized counts, and import boundaries. Wall-clock
  latency is never gated there — runner core counts and memory bandwidth vary enough to make a
  timing gate flake, and a flaky gate is ignored within a month.
- Wall-clock mean and p99 on a real `CVDisplayLink` are gated on macOS, measured locally and
  on a `macos-latest` nightly runner. v1's seven workflows all run on `ubuntu-latest`; that is
  sufficient for M1–M2 correctness and insufficient for the pacing gate, hence the nightly.
- Every user-visible milestone runs the existing Chromium comparison and records command,
  threshold, result, and artifact paths per `AGENTS.md`.

## Failure policy

- **Raster worker panic:** recover, mark tile failed, retry once next frame, then render
  blank with a counter. The UI thread never dies.
- **Font probe failure:** fall back to `defaultMetrics`, as v1 already does.
- **Present while occluded or zero-sized:** skip, retain last plan.
- **Tile budget exhausted:** evict LRU and re-raster next frame. No resolution reduction,
  no new machinery.

## Non-goals for v2

GPU raster or compositing · multi-process isolation · CSS transition/animation timelines ·
`<canvas>` and WebGL · audio/video · service workers and WASM · Windows and Linux
interactive windowing · subpixel text positioning · **any increase in web-platform coverage
beyond v1's current subset**.

Coverage expansion is a separate project with its own gate. Folding it into v2 would consume
the performance work, which is the stated primary goal.

## Risks

| Risk | Mitigation |
|---|---|
| The cgo shim is the least familiar code in the project and it gates everything. | M1 isolates it; `platform/headless` provides the same interface so all engine work proceeds without it. |
| Tiles add cache-coherence bugs v1 never had. | One validity rule, one tracker, and a warm-scroll zero-work assertion that fails loudly. |
| 256px tiles are the wrong constant. | Grid dimension is a single parameter, with the budget math in this document restated when measured. |
| A from-scratch tree fragments effort across v1 and v2. | v1 takes only correctness fixes after this point; feature work stops on the frame path. |
