# Goosie v2 M1: Frame Architecture With No Web Content — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build v2's frame path — tile grid, CPU tile rasterizer, damage-blit composer, vsync-paced UI loop, and two `platform` implementations — so that a synthetic display list scrolls in a real macOS window inside the 60fps budget, before any web content exists.

**Architecture:** Retained layers + 256px fixed tile grid + damage blit. Import graph is strictly `frame` ← `paint` ← `surface` ← `raster` ← {`platform/*`, `cmd/*`}. `raster.Scheduler` owns the per-vsync sequence (coalesce input → resolve needed tiles → submit to worker pool → collect completed tiles → compose → present) and never blocks the UI thread on raster. No engine/style/layout in M1.

**Tech Stack:** Go 1.25, stdlib only in the leaf packages; `golang.org/x/image/font/opentype` + `gofont/goregular` for deterministic glyph raster without system fonts; `golang.org/x/image/draw` for image scaling; cgo + AppKit/CoreVideo only in `v2/internal/platform/darwin`.

**Spec:** [2026-09-11-goosie-v2-m1-m3-frame-path-design.md](../specs/2026-09-11-goosie-v2-m1-m3-frame-path-design.md) — M1 exit criteria are the definition of done for this plan.

**A note on code in this plan:** tasks name exact files, exported types, signatures, test functions, and commands. Bodies are written into the source files as they are implemented rather than duplicated here, because this plan and the tree land in the same commit series and a second copy of ~3k lines would drift out of truth immediately. Every type named below is verified against the spec it derives from; nothing here is a placeholder for an undecided design.

---

## Task 1: Repository scaffold and boundary enforcement

**Files:**
- Create: `v2/internal/frame/doc.go`, `v2/internal/paint/doc.go`, `v2/internal/surface/doc.go`, `v2/internal/raster/doc.go`, `v2/internal/platform/doc.go`
- Create: `v2/internal/archtest/boundary_test.go`
- Create: `v2/README.md`

- [ ] **Step 1: Create the package skeleton** so `go build ./v2/...` succeeds from the start.
- [ ] **Step 2: Write the failing boundary test.** It shells out to
  `go list -f '{{.ImportPath}}|{{join .Imports " "}}' ./v2/...` and asserts, against a table:
  no `fyne.io/…` anywhere under `v2`; no import of `platform` or `surface` by `frame`, `paint`,
  `raster`, `dom`, `css`, `style`, `layout`, `engine`; `surface` imports no `platform`.
  Test names: `TestNoFyneImports`, `TestLayeredImports`, `TestCgoConfinedToDarwinPlatform`.
  `TestCgoConfinedToDarwinPlatform` walks `.go` files under `v2/` for files whose build
  constraint contains `cgo`, or that `import "C"`, and fails unless the path is under
  `v2/internal/platform/darwin/`.
- [ ] **Step 3: Run it, expect PASS on the empty skeleton** — this is the guard that must fail
  loudly for the rest of the project, so it goes first, not last.
  Run: `go test ./v2/internal/archtest/`
- [ ] **Step 4: Write `v2/README.md`** with the run and measure commands (GUI, headless, gate,
  benchmarks). Exit criterion: the doc's commands all exist by Task 12.
- [ ] **Step 5: Commit** `git commit -m "feat(v2): scaffold frame-path tree with import boundary enforcement"`

## Task 2: `frame` geometry primitives

**Files:**
- Create: `v2/internal/frame/geom.go`, `v2/test/frame/geom_test.go`

- [ ] **Step 1: Write failing tests** for `Rect` normalization and algebra: `W`/`H` on inverted
  rects, `Empty`, `IsNormal`, `Canon`, `Contains` (exclusive far edge), `Intersects`
  (touching edges do not intersect), `Intersection` (empty when disjoint), `Translate`,
  `RectF`→`Rect` outward rounding (`Enclosing`) and inward (`Inside`), `Size.Area`,
  `Color` premultiply/unpremultiply round trip, `ColorFromRGBA` packing order.
- [ ] **Step 2: Run, expect FAIL** — `go test ./v2/test/frame/ -run TestRect`
- [ ] **Step 3: Implement** `Point{X,Y int32}`, `PointF`, `Size{W,H int32}`, `Rect{X0,Y0,X1,Y1 int32}`,
  `RectF`, `Color uint32` (byte order `R<<24|G<<16|B<<8|A`, premultiplied), `Viewport{Offset
  Point; Size Size}`, `Scale float32` helpers `ToDevice(p PointF, scale float32) Point` and
  `RectF.ToDevice(scale) Rect` with outward rounding so no content is dropped at edges.
- [ ] **Step 4: Run, expect PASS**
- [ ] **Step 5: Commit** `"feat(v2/frame): geometry primitives with device-space rounding"`

## Task 3: `frame.Bitmap`, `BitmapPool`, and `FrameRecorder`

**Files:**
- Create: `v2/internal/frame/bitmap.go`, `v2/internal/frame/pool.go`, `v2/internal/frame/recorder.go`
- Create: `v2/test/frame/bitmap_test.go`, `v2/test/frame/pool_test.go`, `v2/test/frame/recorder_test.go`

- [ ] **Step 1: Write failing tests.** Bitmap: `At`/`Set`/`FillRect`/`CopyRectFrom` with
  clipping at all four edges, `Blur`-free (assert exact pixel values), and a `Bitmap` whose
  stride exceeds `W*4` to prove stride is honored. Pool: `Acquire` after `Release` returns the
  same backing slice (assert `&buf.RGBA[0]` identity) and `len(buf.RGBA) == cap(buf.RGBA)`.
  Recorder: feed 10k frames, assert p50/p99/mean match a hand-computed table and the ring
  buffer never allocates after warmup.
- [ ] **Step 2: Run, expect FAIL**
- [ ] **Step 3: Implement.** `Bitmap{RGBA []byte; W, H, Stride int}`; `NewBitmap(w, h int) *Bitmap`;
  methods `Reset()`, `At(x, y int) Color`, `Set(x, y int, c Color)`, `FillRect(r Rect, c Color)`,
  `CopyRectFrom(src *Bitmap, dstOrigin Point, srcRect Rect, clip Rect)`.
  `BitmapPool` keyed by `Size` in a `map[[2]int32][]*Bitmap` under a `sync.Mutex`, `MaxIdle`
  per key (default 256), `Acquire() *Bitmap` / `Release(*Bitmap)`, plus `func (p *BitmapPool)
  Prealloc(n int, size Size)` used by the benchmarks so warm-up allocations are provably absent.
  `FrameRecorder` with `FrameMark{Serial uint64; VsyncAt, PlanAt, SubmitAt, ComposedAt,
  PresentedAt time.Time; TilesRasterized, TilesReused, StylePasses, LayoutPasses int32}`,
  fixed ring (`Record(FrameMark)`), `Report() Report{Mean, P50, P99, Max time.Duration;
  Frames int64; ...}`, `WriteJSON(io.Writer) error`, and `Since(last FrameMark) time.Duration`
  helpers. The recorder is the instrument every later gate reads, so it is built here rather
  than retrofitted.
- [ ] **Step 4: Run, expect PASS**
- [ ] **Step 5: Commit** `"feat(v2/frame): bitmap, pooled buffers, frame recorder"`

## Task 4: `paint` commands and `LayerDL`

**Files:**
- Create: `v2/internal/paint/cmd.go`, `v2/internal/paint/list.go`
- Create: `v2/test/paint/list_test.go`

- [ ] **Step 1: Write failing tests**: slab `Append` reuses the backing array across
  `Reset()` (assert pointer identity), commands sorted by a stable key, `Bounds()` of a list,
  `LayerDL` frozen after `Publish` (a second `Publish` on a published list panics — this is the
  invariant that lets the raster pool read lists without locks), and
  `Intersecting(dst Rect) []DisplayCmd` returning a sub-slice with no allocation.
- [ ] **Step 2: Run, expect FAIL**
- [ ] **Step 3: Implement.** `CmdKind` = `CmdFill | CmdBorder | CmdText | CmdImage`.
  `DisplayCmd{Kind CmdKind; Rect frame.Rect; Color frame.Color; Border BorderSpec; Opacity
  float32; Text TextRun; Image ImageSpec}`. `BorderSpec{Top,Right,Bottom,Left SideSpec}`,
  `SideSpec{Width int32; Color frame.Color}` (solid; radius is out of M1 scope).
  `TextRun{Glyphs []GlyphRun; Color frame.Color}` with `GlyphRun{Rune rune; X, Y int32;
  Size int32}` — positioned by the producer, so the rasterizer performs lookup and blit only.
  `ImageSpec{Src image.Image}`. `List` = grow-only slab with `Reset`, `Append`, `Len`, `At`,
  `Intersecting`, `Bounds`. `LayerDL{Cmds []DisplayCmd; ContentVersion uint64; Bounds
  frame.Rect; published bool}` with `Publish()`, read-only accessors, and `Touch`/`Frozen`
  semantics enforcing invariant "frozen once published".
- [ ] **Step 4: Run, expect PASS**
- [ ] **Step 5: Commit** `"feat(v2/paint): slab-backed display lists with publish-freeze semantics"`

## Task 5: `frame` layer, tile grid, and content versions

**Files:**
- Create: `v2/internal/frame/tile.go`, `v2/internal/frame/grid.go`, `v2/internal/frame/layer.go`,
  `v2/internal/frame/plan.go`
- Create: `v2/test/frame/grid_test.go`, `v2/test/frame/layer_test.go`

- [ ] **Step 1: Write failing tests**, table-driven: `TileCoordFor` at tile boundaries and
  negative origins; `VisibleCoords` count for a 3024×1900 viewport at `TileSize=256` equals
  12×8=96; a scroll that leaves a row of tiles valid keeps exactly those tiles `Valid`
  (assert `Rasterized == 0` after warm-up — this is the test that encodes invariant 1's
  raster half); `Invalidate(rect)` marks exactly the intersecting tiles stale and no others;
  eviction honors the byte budget and picks the least-recently-used victim; out-of-range
  coords outside `Layer.Bounds` are never produced; `Plan` publication and retrieval drop
  superseded values (`Latest` returns only the newest serial).
- [ ] **Step 2: Run, expect FAIL**
- [ ] **Step 3: Implement.** `const TileSize int32 = 256` (single tuning knob, named in the
  spec). `TileCoord{Col, Row int32}`; `TileState` = `TileEmpty | TileValid | TileStale |
  TileFailed`; `Tile{Coord; Bounds frame.Rect; Version uint64; LastUsed uint64; Pixels
  *Bitmap; Bytes int64; State TileState}`. `Grid` owning `map[TileCoord]*Tile`, a
  doubly-linked LRU order, `used int64`, `budget int64`, `frame uint64` clock; methods
  `TileAt`, `CoordFor`, `VisibleCoords(vp Viewport, scratch []TileCoord) []TileCoord`,
  `Invalidate(rect frame.Rect) int`, `Needs(coord, layerVersion) bool`, `MarkValid(coord,
  version)`, `Evict() int`, `Stats() GridStats{Tiles, Valid, Stale, Bytes, Budget, Evictions,
  Rasterized}`. **v1's separate `TileCache` collapses into `Grid`** — one owner for the byte
  budget and the LRU order, rather than two objects that must agree. Validity is
  `State == TileValid && Version == layerVersion`, which is the spec's validity rule in one
  expression.
  `Layer{ID LayerID; ContentVersion uint64; DL *paint.LayerDL; Grid *Grid; Bounds frame.Rect}`
  with `BumpContent(v uint64)` that stales the intersecting tiles, and `NewLayer(id, bounds,
  budget int64, pool *BitmapPool)`. `FramePlan{Serial uint64; Viewport frame.Viewport; Scale
  float32; Layers []*Layer; Background frame.Color}` plus `Plan{mu, latest FramePlan}` with
  `Publish(FramePlan)` / `Latest() (FramePlan, bool)` implementing the depth-1 superseding
  channel semantics.
- [ ] **Step 4: Run, expect PASS**
- [ ] **Step 5: Commit** `"feat(v2/frame): layers, tile grid with byte-budget LRU, frame plans"`

## Task 6: `surface` — Window interface, events, damage-blit composer

**Files:**
- Create: `v2/internal/surface/window.go`, `v2/internal/surface/composer.go`
- Create: `v2/test/surface/composer_test.go`

- [ ] **Step 1: Write failing tests.** Composer blits two overlapping tiles into a backing
  bitmap and the result equals a hand-written expectation bitmap; a tile partially outside the
  viewport clips without writing out of bounds (run under `-race` and with a guard byte past
  the buffer end); `Compose` with empty damage writes nothing; a scroll-damage case blits only
  the newly exposed rows (assert an instrumented pixel-write counter). Also
  `TestComposerReuseBackingAcrossFrames` asserting the backing buffer's address is stable —
  invariant 6 again.
- [ ] **Step 2: Run, expect FAIL**
- [ ] **Step 3: Implement.**
  ```go
  type EventKind uint8 // EvVsync | EvScroll | EvPointer | EvKey | EvResize
  type Event struct { Kind EventKind; Delta frame.Point; Pos frame.Point; Button Button;
                      Key rune; Size frame.Size; Scale float32; At time.Time }
  type Cursor uint8 // CursorDefault | CursorPointer | CursorText
  type Window interface {
      Events() <-chan Event
      Present(buf *frame.Bitmap, damage []frame.Rect) error
      SetCursor(Cursor)
      ScaleFactor() float32
      Close() error
  }
  type Composer struct { Backing *frame.Bitmap }
  func NewComposer(vp frame.Size, pool *frame.BitmapPool) *Composer
  func (c *Composer) Compose(bg frame.Color, tiles []TileBlit, damage []frame.Rect,
                             writes *int64) []frame.Rect
  func (c *Composer) ShiftY(dy int32, writes *int64) // memmove row shift fast path
  type TileBlit struct { Pixels *frame.Bitmap; Dst frame.Rect }
  ```
  `Compose` fills the layer background across damaged rects, then blits each tile clipped to
  the viewport. `writes` is a counter, not a benchmark hack — it is how the CI gate proves
  "warm scroll did no tile rasterization and minimal pixel work" without timing anything.
- [ ] **Step 4: Run, expect PASS**
- [ ] **Step 5: Commit** `"feat(v2/surface): window contract and damage-blit composer"`

## Task 7: `raster` — glyph atlas, font faces, tile rasterizer

**Files:**
- Create: `v2/internal/raster/fonts.go`, `v2/internal/raster/atlas.go`, `v2/internal/raster/tile.go`
- Create: `v2/test/raster/tile_test.go`

- [ ] **Step 1: Write failing tests.** One test per `CmdKind`: a `CmdFill` inside the tile
  bounds produces the exact color; a `CmdFill` entirely outside produces zero pixel changes;
  a `CmdBorder` produces per-side widths; a `CmdText` of "Goosie" produces a nonzero,
  deterministic pixel hash (record the hash once, then assert stability across runs and
  goroutine counts — this proves glyph raster is not order-dependent); `CmdImage` scales a
  4×4 source into a 32×32 rect without writing outside it. Plus
  `TestRasterizeTileZeroAllocAfterWarmup` and `TestGlyphCacheReusesFaceAcrossDPR`.
- [ ] **Step 2: Run, expect FAIL**
- [ ] **Step 3: Implement.** `fonts.go`: `Fonts` holding `*opentype.Face` parsed once from
  `gofont/goregular.TTF` (deterministic, no system-font dependence, so CI on a fontless
  Ubuntu runner renders identically to macOS) with `Face(size int32, scale float32)
  font.Face` cached by key. `atlas.go`: `GlyphAtlas` storing per-`(rune, size, scale)` `*image.Alpha`
  masks plus `GlyphBound`, byte-accounted with an LRU budget. `tile.go`:
  `RasterizeTile(dl *paint.LayerDL, bounds frame.Rect, out *frame.Bitmap, f *Fonts,
  g *GlyphAtlas) error` — clear to transparent, iterate `dl.Intersecting(bounds)`, dispatch on
  kind, `draw.DrawMask` with `image.NewUniform(color)` over the glyph mask. The tile is
  rasterized in its own local coordinate space (origin at `bounds.X0/Y0`), which is what makes
  the output position-independent and therefore reusable across scroll offsets.
- [ ] **Step 4: Run, expect PASS**
- [ ] **Step 5: Commit** `"feat(v2/raster): deterministic glyph atlas and per-tile rasterizer"`

## Task 8: `raster` worker pool with panic isolation

**Files:**
- Create: `v2/internal/raster/pool.go`, `v2/test/raster/pool_test.go`

- [ ] **Step 1: Write failing tests.** N jobs across `min(GOMAXPROCS-1, 4)` workers complete
  with no lost results; a job that panics is recovered, its tile marked `TileFailed`, retried
  once, and the pool keeps serving (assert the caller never panics and never hangs, with a
  2s deadline); `Stats.Rasterized` counts only actual work; submitting zero jobs allocates
  nothing on the submit path after warm-up.
- [ ] **Step 2: Run, expect FAIL**
- [ ] **Step 3: Implement.** `Job{Layer *frame.Layer; Coord frame.TileCoord; DL *paint.LayerDL;
  Bounds frame.Rect; Out *frame.Bitmap}`; `Pool` with a job channel, completion channel,
  `Start(ctx)`, `Submit(Job) error`, `Poll() (Done, bool)` (non-blocking — this is how
  invariant 2 is enforced structurally), `Wait(n int) []Done`, `Stats()`. `Done{Coord, LayerID,
  Out *frame.Bitmap, Err error}`. Per-job `defer recover()` converting a panic into
  `ErrTilePanic`.
- [ ] **Step 4: Run, expect PASS**
- [ ] **Step 5: Commit** `"feat(v2/raster): worker pool with per-job panic isolation"`

## Task 9: `surface` UI-thread loop

**Files:**
- Create: `v2/internal/surface/loop.go`, `v2/test/surface/loop_test.go`

- [ ] **Step 1: Write failing tests** against `platform/headless` and a synthetic plan:
  `TestWarmScrollRasterizesNoTiles` (after cache warm, 60 vsync ticks with 100px deltas →
  `Stats.Rasterized == 0`); `TestScrollChangesNoContentVersion` (invariant 1: a scroll never
  bumps a layer version); `TestLateTileNeverBlocksPresent` (pool with 1ms artificial per-tile
  cost and 1 worker → every vsync still presents, frame count equals vsync count, and stale
  tiles blit their previous content); `TestPresentUsesLatestPlanAndDropsSuperseded`
  (publish 3 plans without an intervening vsync → only the last is presented);
  `TestVsyncFrameCountEqualsPresentCount`.
- [ ] **Step 2: Run, expect FAIL**
- [ ] **Step 3: Implement.**
  ```go
  type Scheduler interface {
      BeginFrame(ev Event) (*frame.FramePlan, []frame.TileCoord)
      ComposeInto(backing *frame.Bitmap, c *Composer, plan *frame.FramePlan,
                  needed []frame.TileCoord) []frame.Rect
  }
  type Loop struct { w Window; s Scheduler; c *Composer; rec *frame.FrameRecorder }
  func NewLoop(w Window, s Scheduler, c *Composer, rec *frame.FrameRecorder) *Loop
  func (l *Loop) Run(ctx context.Context) error
  ```
  `Run` drains pending events before each frame (coalescing to one scroll delta per vsync —
  that's the implementation of "input → coalesce" from the spec diagram), calls `BeginFrame`,
  submits stale/needed coords, collects whatever is ready without waiting, composes the
  damage, `Present`s, and records the frame's five timestamps. Never a blocking receive on the
  completion channel.
- [ ] **Step 4: Run, expect PASS** — including under `-race`
- [ ] **Step 5: Commit** `"feat(v2/surface): vsync-paced UI-thread loop that never blocks on raster"`

## Task 10: `raster.Scheduler` — the M1 frame-path coordinator

**Files:**
- Create: `v2/internal/raster/scheduler.go`, `v2/test/raster/scheduler_test.go`

- [ ] **Step 1: Write failing tests.** `TestScrollPathIsAllocationFree`: drive 600 scroll
  ticks with a warm cache and assert `allocs/op == 0` plus `ScratchAllocs() == 0` (the
  scheduler's own counter for any `make` on its scratch buffers — the spec's CI gate is a
  counter, not a benchmark flake risk); `TestPrefetchAheadOfScrollDirection`: scrolling down
  must enqueue tiles below the viewport before tiles above; `TestBudgetPlateaus`: total tile
  bytes stay ≤ budget across 600 frames of a 20,000px scroll; `TestEvictedTileIsRequeued`;
  `TestFailedTileRendersBlankWithCounter`.
- [ ] **Step 2: Run, expect FAIL**
- [ ] **Step 3: Implement** `Scheduler{plan frame.Plan, grid *frame.Grid, pool *Pool, vp,
  scale, scratch coord/completion buffers, prefs Pref{PrefetchRows int32; Budget int64}}` with
  `NewScheduler(l *frame.Layer, pool *Pool, vp frame.Viewport, scale float32, prefs Pref)`,
  `SetPlan`, `SetViewport`, `SubmitScroll(delta frame.Point, at time.Time) error` (the UI→raster
  handoff, running on the UI thread), `HandleDone(Done) error`, `Damage() []frame.Rect`,
  `Stats() Stats`. Scroll math is pure integer; the only allocation is on content-version
  bumps.
- [ ] **Step 4: Run, expect PASS** with `-race`
- [ ] **Step 5: Commit** `"feat(v2/raster): frame scheduler with prefetch ring and byte budget"`

## Task 11: Synthetic scene generator + `platform/headless`

**Files:**
- Create: `v2/internal/paint/synthetic.go`, `v2/internal/platform/headless/window.go`
- Create: `v2/internal/platform/platform.go`
- Create: `v2/test/paint/synthetic_test.go`, `v2/test/platform/headless_test.go`

- [ ] **Step 1: Write failing tests.** `TestSceneDeterministic`: two builds of the same
  `SceneSpec` hash identically (needed so gate goldens are meaningful). `TestSceneBoundsCover
  Grid`: no command lies outside the layer bounds. Headless: `TestVsyncTicksAtRequestedRate`
  with a manual clock; `TestScrollEventsDelivered`; `TestPresentWritesPNG` (decode the output
  and assert the center pixel); `TestScaleFactorHonored`.
- [ ] **Step 2: Run, expect FAIL**
- [ ] **Step 3: Implement.** `SceneSpec{Cols, Rows, TextRuns int32; DocHeight int32; Seed
  int64; Checkerboard bool}` → `BuildLayer(spec SceneSpec) (*paint.LayerDL, *frame.Layer)`:
  checkerboard fills, borders, ≥1,200 positioned text runs (the glyph-atlas stress the spec
  names), and a fake image element, over a document tall enough for 600 × 100px scroll frames.
  `headless.Window`: configurable vsync period from an injectable clock (so tests are
  deterministic), channel-based event injection, `Present` copies into a per-serial buffer and
  `WritePNG(path string) error` on the last-presented frame. `platform.Select()`: darwin+cgo →
  the real window, otherwise headless, with a `func Available() (name string, interactive
  bool)` the GUI binary prints when it refuses to open a window.
- [ ] **Step 4: Run, expect PASS**
- [ ] **Step 5: Commit** `"feat(v2): synthetic scene generator and headless platform window"`

## Task 12: CI gates — benchmarks and counters

**Files:**
- Create: `v2/test/gate/m1_gate_test.go`
- Create: `v2/test/gate/gate_test.go` (helpers: `newWarmScene`, `assertZeroAllocs`, `mustScroll`)

- [ ] **Step 1: Write the gate tests**, which are the M1 CI exit criteria verbatim:
  `TestGate_WarmScrollZeroAllocsZeroTiles` — 1440×900 @ DPR 2, warm cache, 600 scroll frames:
  `allocs/op == 0` (via `testing.B` + `runtime.AllocsBase`/`testing.AllocsPerRun`) **and**
  `Stats.Rasterized == 0`.
  `TestGate_ColdScrollBurstNoRedundantJobs` — 30 frames of 200px into un-rasterized content:
  `allocs/op == 0` on the UI path and submitted-job count == stale-tile count exactly (no
  duplicate work for the same coord in a frame).
  `TestGate_NoGPULinkage` — `go list -deps ./v2/...` and fail on any import path containing
  `metal`, `OpenGL`, `vulkan`, `glfw`, `fyne`.
  `TestGate_ArchtestBoundaries` — delegate to Task 1 (kept here so "M1 CI green" is one
  command).
  Plus `BenchmarkWarmScroll` and `BenchmarkColdScrollBurst` recording `ms/op` for the macOS
  report but asserting nothing on `ns/op`, per the CI/hardware division.
- [ ] **Step 2: Run** `go test ./v2/test/gate/ -race -count=1` — expect PASS on the headless
  platform, no macOS needed.
- [ ] **Step 3: Run benchmarks** and record output:
  `go test -bench=. -benchmem -count=3 -run '^$' ./v2/test/gate/`
  Expected: `0 allocs/op` on the warm case; `ns/op` recorded, not gated.
- [ ] **Step 4: Commit** `"test(v2): M1 frame-path gate tests and scroll benchmarks"`

## Task 13: `platform/darwin` cgo shim

**Files:**
- Create: `v2/internal/platform/darwin/window.go` (`//go:build darwin && cgo`)
- Create: `v2/internal/platform/darwin/shim.m`, `v2/internal/platform/darwin/shim.h`
- Create: `v2/internal/platform/darwin/stub.go` (`//go:build !darwin || !cgo` →
  `ErrPlatformUnavailable`)
- Create: `v2/internal/platform/platform_darwin.go`

- [ ] **Step 1: Write `v2/cmd/goosie/main.go` first** with flags `-url`, `-gate`, `-frames`,
  `-scene`, `-out`, `-width`, `-height`, `-dpr`, `-bench`, so the shim has a consumer and
  `TestGate_*` paths are exercised identically on both platforms.
- [ ] **Step 2: Implement the shim.** Objective-C: an `NSObject` app delegate, `NSWindow`
  (`titled | closable | miniaturizable | resizable`, `layerBacking = NSBackingStoreBuffered`),
  one layer-backed content `NSView`. Expose to Go: `GoosieAppRun()`, `GoosieNextEvent()` (a
  blocking pump returning a packed struct), `GoosiePresent(bytes ptr, w, h, scale, damage
  rects ptr+count)`, `GoosieSetCursor`, `GoosieScaleFactor`, `GoosieClose`.
  `GoosiePresent` wraps the **existing** Go RGBA buffer as a `CGImage` via
  `CGDataProviderCreateWithData` with a no-op release context (no copy), then assigns it to
  the view's `layer.contents` inside an explicit `CATransaction` with
  `kCATransactionDisableActions` so the implicit animation never interpolates frames.
- [ ] **Step 3: Vsync pacing.** `CVDisplayLink` created on `CGMainDisplayID()`, callback on a
  CoreVideo thread, dispatching `Event{Kind: EvVsync}` onto the Go event channel; the
  display-link context carries a `C.GoString`-free handle to a `C.garbageCollector`-safe
  registry (an `unsafe.Pointer` keyed by `uintptr`) so no Go pointer is stored in C memory.
  Scroll wheel (`scrollWheel:` with `momentumPhase`), mouse, key, and resize map to `Event`;
  `ScaleFactor` from `backingScaleFactor`.
- [ ] **Step 4: Build and run** `go build ./v2/... && go run ./v2/cmd/goosie -scene
  checkerboard -width 1440 -height 900` and scroll with a trackpad. Expected: smooth, cache
  plateaus at budget, no frame hitch while scrolling.
- [ ] **Step 5: Commit** `"feat(v2/platform): macOS window shim with CVDisplayLink pacing"`

## Task 14: M1 macOS measurement and baseline record

**Files:**
- Create: `scripts/v2-gate-check.sh`
- Create: `docs/perf/v2-m1-baseline.txt`
- Modify: `.github/workflows/nightly-bench.yml` (add a `v2-macos` job on `macos-latest`)

- [ ] **Step 1:** `go run ./v2/cmd/goosie -gate -scene checkerboard -frames 600 -out
  /tmp/gate.json` then `scripts/v2-gate-check.sh /tmp/gate.json -mean 16 -p99 33`.
- [ ] **Step 2:** Write the macOS benchmark numbers and gate report into
  `docs/perf/v2-m1-baseline.txt`, with the machine, Go version, and `sysctl` core count, so
  the number is interpretable later.
- [ ] **Step 3:** Add the nightly `macos-latest` job running Step 1; keep CI on ubuntu gated
  on counters only (Task 12), never wall clock.
- [ ] **Step 4: Verify the M1 exit-criteria list end to end** and record any criterion that
  cannot pass yet, with the reason, in the file. **M1 is not done until all eight pass.**
- [ ] **Step 5: Commit** `"perf(v2): M1 macOS frame-path baseline and nightly pacing gate"`

---

## Spec drift this plan resolves (and that the spec text is updated to match)

1. **`frame.Scheduler` → `raster.Scheduler`.** The scheduler needs the worker pool, the glyph
   atlas, and the composer, so it belongs with `raster`, not the dependency leaf `frame`.
   `frame` stays pure data with no imports, which is what makes it independently testable and
   keeps `raster ← surface` from inverting anywhere.
2. **`v1`'s separate `TileCache` collapses into `frame.Grid`.** One owner for the LRU order and
   the byte budget instead of two objects that must agree; the validity rule is unchanged.
3. **`raster` imports `surface`.** Graph is `frame` ← `paint` ← `surface` ← `raster` ←
   {`platform/*`, `cmd/*`}. `surface` depends on no platform code; `platform/*` satisfies
   `surface.Window`.
4. **M1 has no `engine` package.** M1's gate is provable with a synthetic plan source, and
   building `engine` against no web content would be speculative. `engine` lands in M2.

## Self-review

- **Spec coverage:** every M1 exit criterion maps to a task — 1→13/14, 2→12, 3→1+12,
  4→12, 5→12, 6→12, 7→14, 8→14. Invariants 2 (never rasterize on the UI thread) and 6
  (no per-frame allocation) get dedicated tests in Tasks 8, 9, 10.
- **Placeholders:** none; where implementation bodies are intentionally in source files rather
  than duplicated in this plan, that choice is stated at the top with its reason.
- **Type consistency:** `frame.Bitmap`, `frame.TileSize`, `paint.LayerDL`, `surface.TileBlit`,
  `raster.Job/Done`, `raster.Scheduler.Stats{Rasterized, Reused, Failed, Bytes, Budget}` are
  the names used from Task 3 through Task 14; `surface.Scheduler` is satisfied by
  `raster.Scheduler`.
