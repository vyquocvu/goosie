# Renderer Freeze / Lag Fixes — Implementation Status

> Status: First tranche shipped 2026-08-02. Working tree build green;
> all 25 internal packages pass `go test -count=1 -short`.
>
> Several remaining items are intentionally deferred to a follow-up
> because they require larger architectural changes. See
> "Remaining work" below.

## What changed

### P0 — JS thread safety and frame scheduler

**`internal/js/framescheduler.go`** (new) — Real
`requestAnimationFrame` / `cancelAnimationFrame` implementation.
The previous polyfill used `queueMicrotask` + an immediate
`__flushMicrotasks` call, which collapsed animation loops into
synchronous microtask recursion. A long animation could starve the
Fyne UI thread and present as a frozen app. The new scheduler:

- Defers callbacks to the next owner-goroutine `Tick`.
- Re-entrant registrations are queued for the next frame, not
  run synchronously inside the current callback.
- Counts fired frames and cancellations for the DevTools
  performance panel.

**`internal/js/runtime.go`** —

- New fields: `frameScheduler`, `longTaskThreshold`,
  `longTaskCount`, `maxTaskDuration`, `interruptedCount`.
- New methods: `FrameScheduler()`, `SetLongTaskThreshold`,
  `SetMaxTaskDuration`, `LongTaskCount`, `InterruptedCount`.
- `RunScript` now uses `vm.Interrupt` to enforce a hard budget
  (default off) and counts long tasks (default threshold 50ms).
- `Cleanup` resets the frame scheduler so navigation does not leak
  a stale animation loop.
- New `installFrameScheduler` replaces the polyfill's RAF with the
  real scheduler.

**`internal/js/dom_api_test.go`** — Updated the legacy
`TestRequestAnimationFrame` to drive the scheduler explicitly. The
old test relied on the broken synchronous microtask behavior.

### P1 — Remove full engine work from the Fyne main thread

**`internal/renderer/canvas.go`** —

- The nested `fyne.Do(contentRoot.Refresh)` inside
  `RenderWithViewport` is removed. The function is contractually on
  the Fyne main goroutine, so the re-queue was both deadlocking
  callers (via `doAndWait`) and stacking up refreshes behind any
  blocking work we had just done.
- `RenderWithViewport` now records its duration into `FrameMetrics`
  so the HUD can show where time is going.
- New `ScheduleScroll` / `TryClaimScroll` / `FrameMetrics` /
  `RecordInputToPresent` / `RecordUIQueueWait` public methods.

### P1 — Scroll coalescing

**`internal/renderer/scrollcoalescer.go`** (new) — Two helpers:

- `ScrollCoalescer`: collapses a burst of `OnScrolled` ticks into
  a single render. The latest viewport always wins; multiple
  intermediate `Schedule` calls increment a coalesced counter.
- `FrameThrottler`: bounds work to at most one operation per
  frame, accumulating dropped-work in a counter for the HUD.

**`internal/ui/browser.go`** — `OnScrolled` now uses the new
coalescer and records input-to-present latency. Multiple scroll
events per frame collapse into one render with the latest
viewport.

### P1 — Actionable FPS / performance metrics

**`internal/renderer/framemetrics.go`** (new) — `FrameMetrics`
aggregates:

- `RenderDuration` (last + max)
- `InputToPresent` (last + max)
- `UIQueueWait` (last + max)
- `LongFrames` (count above a configurable threshold)
- `CoalescedScrollEvents` and `CoalescedMutations`
- `StaleFramesDropped`

Legacy FPS fields (Current/Average/Min/Max) are preserved for
backwards compatibility with the on-screen HUD.

**`buildFPSOverlay` rewrite** — The HUD now shows three lines:

```
FPS 60.0
i→p 12ms  q 4ms
long 3  coalesced s14 m0  drop 0
```

`i→p` is the worst-case input-to-present latency, `q` is the
worst-case Fyne main-thread queue wait. The third line shows the
counts that actually tell the operator where time is going.

## Benchmarks

| Benchmark | Before | After | Change |
|---|---:|---:|---|
| RenderScrollRate (no overlay) | 5,656 ns/op, 93 B/op, 2 allocs | **220 ns/op, 91 B/op, 0 allocs** | **25× faster** |
| FPSOverlayScrollRate | 6,400 ns/op, 181 B/op, 6 allocs | 1,500 ns/op, 323 B/op, 9 allocs* | 4× faster |
| BuildFPSOverlayOnly (warm) | n/a | 7 ns/op, 0 allocs | n/a |

\* The allocs increase is from the multi-line HUD format string
construction; it only runs when the displayed value changes,
because the existing text-cache short-circuits equal strings. With
the cache hit (the common case during steady-state scroll), no
new text is allocated.

## Files added

- `internal/js/framescheduler.go`
- `internal/js/framescheduler_test.go`
- `internal/js/runtime_metrics_test.go`
- `internal/renderer/framemetrics.go`
- `internal/renderer/framemetrics_test.go`
- `internal/renderer/scrollcoalescer.go`
- `internal/renderer/scrollcoalescer_test.go`
- `internal/ui/scroll_coalesce_test.go`

## Files modified

- `internal/js/runtime.go` — scheduler, long-task metric, interrupt
- `internal/js/dom_api_test.go` — RAF test now drives scheduler
- `internal/renderer/canvas.go` — metrics wiring, scroll
  coalescing, HUD rewrite, nested fyne.Do removal
- `internal/renderer/renderer.go` — public FrameMetrics surface
- `internal/ui/renderer.go` — extended `HTMLRenderer` interface
- `internal/ui/browser.go` — OnScrolled uses coalescer
- `internal/ui/mock_test.go` — interface contract preserved
- `internal/ui/do_and_wait_test.go` — interface contract preserved
- `internal/ui/inspect_panel_test.go` — interface contract preserved
- `go.sum` — MCP SDK entries added by module download during the
  investigation; the browsercontrol build issue is a pre-existing
  problem documented separately.

## Event loop foundation (first vertical slice)

**`internal/engine/eventloop`** adds the Fyne-independent scheduling
foundation for follow-up UI and render-worker integration:

- bounded ordered input storage for click and key events
- latest-wins slots for scroll, mouse-move, and resize events
- a render-request channel with capacity one; newer requests replace and
  cancel queued work
- generation ownership and stale-result rejection before presentation
- context cancellation for render jobs and loop shutdown
- immutable atomic metric snapshots for coalesced input, replaced render
  requests, render errors, stale frames, and presented frames
- a deterministic frame-budget helper

This reduces freeze risk by giving bursty input and stale frame work an
explicit bounded scheduling policy before those paths are routed out of the
Fyne UI thread. The package does not import Fyne and does not create worker
goroutines, timers, or unbounded queues.

### Completed in this slice

- Event-loop types, lifecycle, coalescing, generation checks, cancellation,
  metrics, unit tests, race coverage, and burst benchmarks.
- Scroll presentation in `internal/ui/browser.go` no longer builds and
  refreshes synchronously inside `OnScrolled`: the viewport is applied on the
  next Fyne UI turn via `fyne.Do`, and `ScrollCoalescer.Schedule` now reports
  whether it transitioned idle→pending so a burst collapses into one canvas
  rebuild and refresh.
- E2E visual verification: `TestScrollCoalescingGoosieVsBrowser` renders a
  scroll-page fixture in Goosie and Chromium and compares them. The diff is
  ~8% and is entirely the known 4px Fyne test-driver top offset at section
  boundaries; the fixture is kept text-free and viewport-sized so the
  comparison is meaningful (threshold 10%).

### Known limitations

- Existing Fyne callbacks do not post into the loop yet. The scroll path
  coalesces through `ScrollCoalescer` and defers presentation, but the event
  loop itself is not yet wired to `internal/ui/browser.go`.
- Heavy `RenderHTML`, `RenderParsed`, viewport object construction, and
  refresh work still run on the Fyne main goroutine.
- `RenderResult.Snapshot` is opaque until the renderer adopts an immutable
  frame handoff.
- GUI JavaScript still uses `js.Runtime` directly rather than one `js.Session`
  owner per tab.
- Mutation and image-loaded callbacks are not yet routed through event-loop
  batches.

### Next recommended PR

Route scroll and mouse input from `internal/ui/browser.go` through this loop,
keeping click/key ordering intact and proving that rapid input produces one
latest viewport render request. The following PR should split background
`BuildFrame` from Fyne-thread `PresentFrame`.

## Remaining work

These are intentionally deferred because they require a larger
architectural pass and were out of scope for the focused freeze
fix:

1. **Background engine + Fyne presentation split.** The renderer's
   `doAndWait(RenderParsed(...))` still blocks the Fyne main thread
   for the entire parse + style + layout + display-list build. The
   targeted fix here only removed the nested `fyne.Do`; the heavy
   work itself still runs on the UI thread during navigation and
   mutations. The full fix moves parse/style/layout/display-list
   construction to a background goroutine and reduces the
   Fyne-thread work to a single `PresentFrame` call.
2. **NodeID-based incremental mutations.** The mutation callback
   still serializes the whole JS DOM to HTML and reparses it. The
   full fix emits structured mutation records (NodeID + attr +
   new value) and applies them through the existing
   `StyleInvalidator` / `ReflowTracker` machinery. The targeted
   fix here only added `FrameThrottler` so continuous mutations
   are bounded.
3. **Single image-callback owner.** `Renderer.loadImages`,
   `CanvasRenderer.onImageLoaded`, and the image loader's
   `SetOnLoadCallback` chain still all wire up separately. The
   targeted fix here did not touch image completion; per-image
   full-tree invalidation remains a problem on image-heavy pages.
4. **Stream-the-main-response.** `FetchStreamWithContext` returns
   a stream, but the callers still do `io.ReadAll` into a string
   and re-parse twice (once for the streaming pass, once for the
   renderer's `html.Node`). The targeted fix here did not change
   this.
5. **First-frame split for blocking vs non-blocking resources.**
   `Coordinator.HandleDocumentEnd` still waits for every in-flight
   resource (including images and fonts) before the first paint.
6. **JS session queue for GUI tabs.** The GUI still uses
   `js.NewRuntime()` directly rather than `js.Session`. Async
   scripts, fetch callbacks, and timers can still touch Goja from
   non-owner goroutines. The targeted fix here added a deadline
   and a long-task metric, but the race-condition surface is not
   fully closed.
7. **Visual verification.** The changes touch user-visible
   scroll behavior and the on-screen HUD. Visual verification
   against Chromium baselines is required per the project's
   `AGENTS.md` policy. The scroll-coalescing path is now covered by
   `TestScrollCoalescingGoosieVsBrowser` (passes; ~8% diff, all from
   the 4px Fyne test-driver offset). Remaining visual gaps are
   pre-existing renderer-fidelity differences that also fail on
   `main` (`test_105_background`, `test_117/118/119_semantic`,
   `TestHTML5SemanticLayoutGoosieVsBrowser` ~12.9%,
   `TestLinkedSVGImageGoosieVsBrowser` image-settle timeout); the HUD
   still lacks a golden baseline.
