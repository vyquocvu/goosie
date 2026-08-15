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

### Known limitations (after PR4)

- Mouse-move hover inspection, the right-click context menu, and hyperlink
  taps still dispatch inside the renderer's canvas internals; they do not yet
  post into the loop. The loop's `InputMouseMove`/`InputClick` slots are wired
  and drained in order, so a later PR can route them without reordering.
- The scroll path's render request is scheduled and gated through the loop,
  but the canvas rebuild (viewport object construction + refresh) still runs
  on the Fyne main thread — that is the retained-display-list path and is
  deliberately the only remaining UI-thread work.
- `go test -race ./internal/renderer/` fails on `main` too (pre-existing):
  Fyne's internal font-metrics cache is not goroutine-safe and the image
  integration tests' async image loading overlaps layout in the next test.
  Unrelated to the render split; tracked for a follow-up.
- Typed mutations (PR6) handle pure attribute/text changes without a full
  reparse; structural mutations (insert/remove/replace) still fall back to
  the full serialize + reparse path because the typed sink cannot yet
  synthesize render subtrees. (Computed-style recompute for the typed path
  landed in PR10; structural full-reparse is the remaining limitation.)
- Image loads are batched (PR7), but the canvas renderer keeps a second
  `onImageLoaded` callback that can win the loader slot if `SetWindow`
  runs after a present; the renderer's batched path is the effective
  owner in the browser flow.
- `RenderResult.Snapshot` is opaque until the renderer adopts an immutable
  frame handoff.

## PR2 — Route UI input through the engine event loop

Scroll and F12 key input from `internal/ui/browser.go` now flow through the
per-tab `eventloop.Loop` instead of calling the renderer directly:

- Each `Tab` owns a bounded `eventloop.Loop` (128-slot FIFO for clicks/keys,
  latest-wins slots for scroll), created in `newTabInternal` and closed on
  tab close (`tabs.OnClosed`).
- `OnScrolled` now only posts an immutable `InputScroll` event
  (`postScrollViewport`); a UI-thread drain scheduled via `fyne.Do`
  (`scheduleInputDrain`, at most one per turn) collapses the burst into one
  `SetViewport` + `refreshTabContent` with the latest position.
- `InputKey` (F12 dev-tools toggle) routes through the loop's FIFO so it is
  drained in the same order as other discrete input.
- The drain reports the per-render coalesced-scroll delta and input-to-present
  latency to `FrameMetrics`, keeping the on-screen HUD numbers intact.
- The tab no longer uses the renderer's `ScrollCoalescer` directly; that path
  remains on the `HTMLRenderer` interface for the canvas-internal callers and
  tests that still exercise it.

Tests (`internal/ui/event_loop_input_test.go`):

- 20-event scroll burst collapses to exactly one drain applying the latest
  viewport (Y=190), with input-to-present recorded once.
- Real `OnScrolled` entry point applies the latest position.
- Click/key FIFO ordering preserved through the tab wiring (two F12 keys
  toggle dev tools off→on→off with clicks interleaved).
- Empty drain is a no-op; posting after `Close` returns `ErrClosed`.

E2E visual verification: `TestScrollCoalescingGoosieVsBrowser` passes at the
10% threshold (known ~8% diff from the 4px Fyne test-driver offset);
screenshots regenerated under `test/e2e/testdata/results/{goosie,browser}/`.

## PR3 — Latest-only render scheduling

The drained scroll state now flows through the loop's render-request
pipeline instead of being applied directly:

- The input drain turns a scroll event into a `RenderRequest` via
  `Loop.ScheduleRender` (queue capacity 1): a newer scroll replaces the
  queued request and cancels the superseded one, so a burst still produces
  exactly one render of the final viewport.
- The drain consumes the latest request, submits its completion via
  `SubmitRenderResult`, and runs the loop's generation/cancellation gate
  through the new non-blocking `Loop.ProcessPendingResults()`. (PR11
  collapsed the former deferred execution turn into the drain itself, so
  input-to-present spans a single UI turn.)
- The `Present` callback (`presentRenderResult`) is the single place a
  completed render touches the canvas (`SetViewport` + `refreshTabContent`)
  and records the coalesced-scroll delta and input-to-present latency. It
  only fires for current, non-stale results — a stale render is never
  painted.
- Each new document render (`Tab.RenderHTML` / `RenderParsedContent`) bumps
  the loop's generation (`bumpDocumentGeneration`), cancelling any render
  scheduled under the prior document so late scroll results are dropped.

Tests (`internal/engine/eventloop/loop_test.go`,
`internal/ui/event_loop_input_test.go`):

- `ProcessPendingResults` processes queued completions non-blocking and
  drops stale-by-generation results.
- Full drain → schedule → execute → present flow: 20-event burst produces
  one presented render with the final viewport.
- Render-queue replacement: scheduling a second request cancels the first
  and only the newest viewport is presented.
- Stale dropping: a scroll render scheduled before a navigation is never
  painted after the generation bump.
- Generation advances on every document render.

E2E visual verification: `TestScrollCoalescingGoosieVsBrowser` passes at the
10% threshold; screenshots regenerated under
`test/e2e/testdata/results/{goosie,browser}/`.

## PR4 — Background BuildFrame / Fyne-thread PresentFrame split

The heavy engine phases no longer run on the Fyne main thread. Renderers
that implement the optional `frameSplitter` interface (the real
`*renderer.Renderer`) split rendering into:

- **Build** — `BuildHTML` / `BuildParsed`: parse, stylesheet assembly,
  style resolution, and layout. No Fyne work; safe to call off the UI
  thread. The tab runs these on the caller's goroutine — in production
  the navigation and mutation-coalescer worker goroutines, so the UI
  thread is no longer blocked by parse/style/layout.
- **Present** — `PresentFrame`: builds the Fyne canvas objects from the
  cached trees. Marshalled onto the UI thread via `doAndWait`; the UI
  thread only constructs/refreshes canvas objects here.

Key details in `internal/renderer/renderer.go`:

- Style/layout run on local trees **without** holding `treeMu`, so a
  background build never blocks a concurrent scroll render; only the
  final tree handoff is atomic.
- A `buildSeq` counter implements newest-build-wins: a slower build for
  an older render intent skips the handoff instead of clobbering a newer
  build's trees.
- `ctx` is checked between phases, so a superseded navigation aborts the
  build; the tab swallows `context.Canceled`/`DeadlineExceeded` instead
  of surfacing an error UI.
- The parsed stylesheet is captured locally at build time, closing a
  read-after-write race that concurrent builds could hit through
  `r.stylesheet`.
- The legacy `RenderHTML`/`RenderParsed` methods are now thin wrappers
  composing Build + Present, preserving behavior for direct callers
  (headless renderer, tests). The tab falls back to the legacy
  single-phase path for renderers without `frameSplitter` (mocks).

Tests: `internal/renderer/render_split_test.go` (two-phase == legacy
output, BuildHTML→Present end to end, newest-build-wins, cancelled-build
handoff) and `internal/ui/render_split_test.go` (tab takes the split path,
error propagation, cancellation swallowed, legacy fallback).

E2E visual verification: `TestScrollCoalescingGoosieVsBrowser` passes at
the 10% threshold.

### Next recommended PR

PR10–PR12 completed the typed-path style recompute, the event-loop
optimization pass, and the remaining follow-ups (image-callback owner,
stream-the-main-response, frame-gated scroll present). The only
remaining item in "Remaining work" is the visual-verification baseline;
a larger architectural candidate would be converting `dom.Document` to
`*html.Node` to eliminate the renderer's second parse (fidelity risk
noted in PR12).

## PR5 — Single-owner JS session for GUI tabs

GUI tabs now route all JavaScript through one `js.Session` owner per tab
so the goja VM is never touched from a non-owner goroutine:

- `js.Session` gains `NewSessionWithRuntime` (own an existing runtime and
  wire its async-callback routing to the session), `SubmitAndWait`
  (blocking owner execution), and `Eval` (blocking script evaluation) in
  `internal/js/session.go`.
- `Runtime.RunScript` is mutex-serialized in `internal/js/runtime.go` as a
  safety net for direct callers.
- `Tab.SetJSRuntime` wraps the runtime in a session and starts its owner
  goroutine; storage/origin wiring runs on that owner.
  `RunScriptOnOwner`/`SubmitOnOwner`/`CloseJSSession` are the tab's
  owner-routed execution helpers (`internal/ui/browser.go`). The console
  eval path, navigation teardown (close the old session before attaching
  a new runtime), and tab close all use them.
- `cmd/browser/main.go`: the coordinator's runtime configuration,
  buffered-async replay, and script queue run inside `SubmitOnOwner`;
  the legacy sync page-load path does the same; the repro workload uses
  `RunScriptOnOwner`. Navigation (`CloseJSSession` → new runtime)
  rejects lingering timers/fetch callbacks from the superseded document.

Tests: `internal/js/session_test.go` (NewSessionWithRuntime wiring,
SubmitAndWait ordering/closed, Eval value/error/closed) and
`internal/ui/js_session_test.go` (lifecycle: no-session reject → eval →
submit → close rejects; nil detach).

E2E visual verification: `TestScrollCoalescingGoosieVsBrowser` still
passes at the 10% threshold (no rendering changes in this PR).

## PR6 — Structured NodeID mutation records

Pure attribute/text DOM mutations no longer serialize the whole JS DOM to
HTML and reparse it. The pipeline is now `JS mutation → MutationRecord →
compact render-tree apply → invalidation → present`:

- **`internal/js`** — `__onDOMChangedGo` builds the typed `DOMMutation`
  record once and shares it between the batch and string callbacks. When a
  typed batch callback is wired, `set-text`/`set-attribute` mutations skip
  the full `serializeJSDOMToCache` + reparse entirely (new
  `needsFullReparse` guard in `mutation.go`); structural mutations
  (insert/remove/replace) and unclassified kinds still fall back to the
  string callback.
- **`internal/renderer`** — `MutationSink.Handle` now syncs the mutation
  value into the render tree before invalidating: `ApplyTypedMutationValue`
  sets `set-text` on the node (or its first text child, appending one when
  an element had none) and updates the `Attrs` map for `set-attribute`.
  Stale NodeIDs (superseded document, reparse) resolve to nothing and are
  rejected safely. `ApplyMutationBatch` also drops the canvas renderer's
  pointer-identity display-list cache on in-place mutation so the next
  `UpdateViewport`/present rebuilds commands from the updated trees
  instead of repainting stale content. The sink records the coalesced-
  mutation metric per batch.
- **`internal/ui`** — new `Tab.RefreshFromMutation` marshals the post-
  mutation canvas refresh onto the Fyne main thread (no full reparse).
- **`cmd/browser`** — both the coordinator and legacy paths wire the sink
  with `RefreshFromMutation` as the present hook (previously the adapter
  was nil, so the typed present was a no-op). The structural-mutation
  coalescer stays as the fallback, and after each structural reparse the
  NodeID lookup is re-snapshotted so subsequent typed mutations map to the
  new render tree.

Tests: `internal/js/serialize_test.go` (attribute/text mutations skip
serialization when a batch callback exists; structural mutations still
serialize; no-batch legacy keeps serializing), `internal/renderer/
mutation_sink_test.go` (text/attr value sync, text-child append on empty
element, stale-ID rejection, display-list cache drop after mutation), and
`internal/ui/event_loop_input_test.go` (RefreshFromMutation refreshes the
canvas once per call).

E2E visual verification: `TestScrollCoalescingGoosieVsBrowser` passes at
the 10% threshold (no layout-affecting rendering changes in this PR).

Known limitations (also recorded in "Known limitations (after PR4)"):
structural mutations still full-reparse (the typed sink cannot synthesize
render subtrees); computed-style recompute for the typed path landed in
PR10.

## PR7 — Image invalidation batching

Image-loaded callbacks are now coalesced into one render per window
instead of one full style+layout+present cycle per completed image:

- **`internal/renderer/imagebatch.go`** (new) — `ImageLoadBatcher`
  accumulates completed image sources into a pending set; the first
  `Signal` after an idle period arms a window timer (16ms), and when it
  fires the whole set is handed to the flush callback in one call.
  Duplicate sources (e.g. a load-goroutine signal racing the loader's
  own callback) collapse to one entry. `Flush()` drains immediately
  (tests/shutdown), `Close()` flushes pending work once and rejects
  further signals, and `Metrics()` reports batches performed and
  signals collapsed.
- **`internal/renderer/renderer.go`** — `onImageLoaded` now only
  signals the batcher; `flushImageBatch` applies the completed image
  data to the current render tree, invalidates the object cache, and
  triggers exactly one `Refresh()` for the whole batch, recording the
  batch size via `RecordCoalescedImages`. A nil batcher (tests) falls
  back to an immediate single-image flush.
- **Metrics** — `FrameMetrics` gains `CoalescedImages`
  (`IncCoalescedImages`), plumbed through `CanvasRenderer`/`Renderer`
  and the `HTMLRenderer` interface, and the on-screen HUD now shows
  `i<N>` on the coalesced line.

Tests: `internal/renderer/imagebatch_test.go` (100-signal burst → one
flush with all sources + metrics, separate windows → separate flushes,
immediate `Flush`, `Close` drains-then-rejects, source dedup) and
renderer integration tests (`TestRendererImageLoadsBatchSingleRender`:
5 loads in a burst → exactly one refresh, no immediate present before
the window, `CoalescedImages == 5`; `TestRendererImageLoadsFlushAppliesData`:
img nodes carry loaded data after the flush).

E2E visual verification: `TestScrollCoalescingGoosieVsBrowser` passes at
the 10% threshold. `TestLinkedSVGImageGoosieVsBrowser` still fails but
improved: at HEAD it times out with "images did not finish loading";
with PR7 the image loads and settles and the remaining 34.6% diff is
the pre-existing SVG rendering-fidelity gap (artifacts under
`test/e2e/testdata/results/{goosie,browser}/linked_svg_image.png`).

## PR8 — Non-blocking first paint

`Coordinator.HandleDocumentEnd` no longer waits for every in-flight
resource before first paint. Resources now split into two classes:

- **Blocking** (stylesheets, classic/defer scripts) — shape the
  document; `HandleDocumentEnd` still waits for these (with the
  caller's deadline), so first paint is correct.
- **Non-blocking** (images, fonts — primary and CSS-nested) — stream
  in after first paint. Their fetches keep running; results fire via
  `OnImage`/`OnFont` as they settle.

Implementation in `internal/engine/documentloader/coordinator.go`:

- A second `blockingInFlight` WaitGroup counts only blocking kinds
  (`isBlockingKind`); `HandleDocumentEnd`'s initial and secondary-cycle
  waits use it. `pendingN` (atomic mirror of `inFlight`) plus the
  existing `asyncN` drive a new `allDone` channel.
- After the main drain, buffered late results (images/fonts that
  completed after `HandleDocumentEnd`) are emitted by a final drain
  (`finalDrain`), still in document order, so every successfully
  fetched resource fires its callback exactly once.
- `EventLoad`/`EventDocumentEnd` are now gated on ALL work — async
  scripts and non-blocking fetches — via `waitAllDoneAndFinalize`,
  matching browser load semantics.

Tests: `internal/engine/documentloader/coordinator_pr8_test.go`
(HandleDocumentEnd returns while an image/font is still in flight;
stylesheets still block; late image results emit in document order;
EventLoad waits for images). Existing image/font tests updated to poll
for callbacks that now arrive after `HandleDocumentEnd` (with
race-safe accessors).

E2E visual verification: `TestScrollCoalescingGoosieVsBrowser` passes at
the 10% threshold (no rendering changes in this PR).

## PR9 — Route mouse input through the engine event loop

Mouse-move, click, and hyperlink-tap dispatch moved out of the
renderer's canvas internals and into the per-tab engine loop — the last
remaining event-loop wiring. The loop's `InputMouseMove` (latest-wins)
and `InputClick` (FIFO) slots now carry real pointer input end to end:

- **`internal/renderer/mouseinput.go`** (new) — UI-agnostic `MouseInput`
  value (kind, widget-space X/Y, canvas-absolute X/Y, button, link URL)
  and the `mouseInputPoster` callback. `CanvasRenderer`
  (`SetMouseInputCallback`) and `Renderer` forward it; when a poster is
  wired the canvas widgets (`InspectableContainer.MouseMoved`/`MouseDown`/
  `TappedSecondary`, `TappableHyperlink.Tapped`) post immutable events
  instead of dispatching inspect/context-menu/navigation directly. With
  no poster they keep the legacy direct dispatch, so renderer-only
  owners and tests are unaffected.
- **`internal/engine/eventloop`** — `InputEvent` gains `URL` (link
  navigation target) and `AbsX`/`AbsY` (canvas-absolute cursor for
  context-menu placement), keeping the loop fyne-free.
- **`internal/ui/browser.go`** — `Tab.postCanvasMouseInput` maps each
  renderer `MouseInput` into the loop's matching slot and schedules the
  single per-turn drain. The drain (`drainInputLoop`) now dispatches
  `InputMouseMove` (throttled 80ms hover hit-test, element-delta
  checked) and `InputClick` (left → hit-test + inspect select; button 2
  → hit-test + dev-tools context menu at the absolute position; URL
  present → navigate). The tab mirrors the latest drained scroll offset
  (`lastViewportY`) so hit tests convert widget-space to content
  coordinates, and the shared `handleInspect`/`handleContextMenu`
  helpers serve both the direct canvas path and the drain.

Tests: `internal/renderer/mouseinput_test.go` (poster routing carries
positions/buttons/URLs and suppresses direct dispatch; no-poster
fallback keeps dispatching), `internal/ui/event_loop_input_test.go`
(mouse-move burst → one drained hover hit-test at the latest position
with the coalesced delta; left-click hit-tests at the scroll-adjusted
position and selects the element; right-click reaches the context menu;
link taps navigate in FIFO order interleaved with clicks/keys).

E2E visual verification: `TestScrollCoalescingGoosieVsBrowser` passes at
the 10% threshold (no rendering changes in this PR).

## PR10 — Recompute computed styles on the typed mutation path

The PR6 known limitation is closed: class/`style=` mutations on the
incremental path now recompute computed styles (and relayout) instead of
keeping stale styling until the next structural reparse.

- **`internal/renderer/invalidation.go`** — `ApplyMutationBatch` now runs
  `recomputeSubtreeStyles` on every mutation carrying `DirtyStyle` before
  any relayout, so the rebuilt layout boxes read fresh computed styles.
  The new `resetSubtreeStyles` helper clears `ComputedStyle` and the raw
  `Styles` map for the whole mutated subtree first — `ApplyStyles` merges
  into existing computed styles, so without a reset a removed class or
  cleared `style=` declaration would keep its stale styling. The
  re-apply runs under `treeMu` with a fresh `StyleManager` built from the
  current stylesheet and viewport; when no stylesheet is loaded (an
  un-built renderer) styles are left as-is to mirror build-time behavior.
- **`internal/renderer/mutation_sink.go`** — `set-attribute` mutations now
  carry `DirtyLayout` (in addition to `DirtyStyle | DirtyPaint`): an
  attribute change can alter matched rules (`class`, `id`, `style=`) and
  therefore layout, so the subtree is relaid out as well as restyled.

Tests: `internal/renderer/mutation_sink_test.go` — class change flips
computed color and a removed class reverts to the colorless default;
class change from `block` to `flex` recomputes the computed display AND
rebuilds the incremental engine's layout box for the target with
`DisplayFlex`; `style=` re-parse on change and revert on clear.

E2E visual verification: `TestScrollCoalescingGoosieVsBrowser` passes at
the 10% threshold.

## PR11 — Event-loop optimization pass

A focused review of the input→present pipeline with three changes, each
removing per-event or per-frame work without changing behavior:

- **Same-turn scroll present.** The scroll drain previously queued a
  second UI-thread turn (`scheduleRenderExecution`) to consume the loop's
  render request — one extra `fyne.Do` marshal and channel round trip per
  scroll frame in the real app (headless tests ran it inline, so behavior
  was identical there). `drainInputLoop` now executes the final request
  (`executeRenderRequest`) at the end of the same drain turn, halving the
  input-to-present path while keeping the loop's replace-latest burst
  coalescing and the generation/cancellation gate. The now-dead
  `scheduleRenderExecution` and `execScheduled` field were removed; the
  `executeRenderRequest` seam stays (it is exercised directly by the
  replace-old-request and stale-drop tests).
- **Mouse-move pre-throttle.** `postCanvasMouseInput` now drops
  `MouseInputMove` events inside the 80ms hover window before they reach
  the loop — `handleMouseMove` would discard them anyway, so the ~60fps
  pointer stream no longer pays the post + drain cost for positions that
  cannot produce a hit test. Clicks and link taps are discrete and never
  throttled. The 80ms window is now a shared `hoverThrottle` constant.
- **Single culling pass in `RenderWithViewport`.** The leaf-command loop
  culled with viewport bounds and then called `isInViewport` again per
  leaf — provably identical bounds (`y−0.5h` / `y+1.5h`), so the second
  check could never reject a surviving command. Removed the redundant
  pass; the dirty-overlay debug path keeps its own check.

Tests: `internal/ui/event_loop_input_test.go` — the mouse poster drops
in-window moves before posting (clicks always posted, no extra hit tests),
and the full scroll-flow test asserts the render-request queue is empty
once the drain returns (same-turn execution).

E2E visual verification: `TestScrollCoalescingGoosieVsBrowser` passes at
the 10% threshold (no rendering changes).

## PR12 — Close out the remaining follow-ups

Three follow-ups from "Remaining work" landed in one pass:

- **Residual image-callback owner.** `CanvasRenderer.onImageLoaded` no
  longer competes with the renderer's PR7 batched owner for the loader's
  single callback slot. `NewRenderer` wires the canvas's `renderer`
  back-reference (it already existed for hit-testing), and the canvas
  callback now delegates to `Renderer.onImageLoaded` (the batched path)
  whenever a Renderer owns the canvas; a standalone canvas keeps the
  legacy per-image refresh. `SetWindow` after a present can no longer
  clobber the batching — ordering is now irrelevant.
- **Stream-the-main-response.** `updateUIWithCoordinatorContent` (kept as
  the string entry for tests/mock fallback) now wraps a new streaming
  variant, `updateUIWithCoordinatorStream`, that the real navigation path
  feeds the **live response stream** instead of an `io.ReadAll`-ed
  string. The discovery parser consumes the body as it downloads
  (tee-capturing the bytes for the renderer's parse), so `<link>`/
  `<script>` resources in the head are discovered — and their fetches
  start — before the body finishes arriving; CSS fetch RTT now overlaps
  the body download. An `errRecordingReader` preserves the explicit
  read-error contract (the tokenizer would otherwise treat a truncated
  body as clean EOF). The renderer's `html.Parse` pass is unchanged (its
  contract is `*html.Node`; converting `dom.Document` remains a future
  fidelity-risk item).
- **Frame-gated scroll present.** `drainInputLoop` no longer presents on
  every UI turn: `scheduleScrollPresent` executes the queued render
  immediately when ≥1 frame elapsed since the last present, otherwise it
  arms one frame-boundary timer. The request stays in the loop's
  replace-latest queue while waiting, so sub-frame scroll bursts collapse
  into one canvas rebuild per display frame and the boundary always
  paints the newest viewport (no lost final frame). This is the concrete
  realization of the loop's frame-budget machinery: the budget duration
  (`Loop.FrameBudget().Duration`, default 60fps) is the gate. A small
  `renderGateMu` guards the timer/present fields for headless mode where
  `browser.do` runs inline on the timer goroutine.

Tests: `internal/renderer/imagebatch_test.go` (canvas callback delegates
through the batcher: a two-signal burst coalesces to one refresh with
`CoalescedImages == 2`), `cmd/browser/stream_main_response_test.go` (a
stylesheet in the head is fetched while the body is still staged — the
pipelining proof — and a mid-body read error fails the navigation
instead of rendering a truncated page), and `internal/ui/
event_loop_input_test.go` (frame-gated present: a second sub-frame
scroll defers to the boundary and supersedes intermediate positions,
collapsing to one boundary present).

E2E visual verification: `TestScrollCoalescingGoosieVsBrowser` passes at
the 10% threshold (no rendering changes).

## Remaining work

These are intentionally deferred because they require a larger
architectural pass and were out of scope for the focused freeze
fix:

1. **Visual verification.** The changes touch user-visible
   scroll behavior and the on-screen HUD. Visual verification
   against Chromium baselines is required per the project's
   `AGENTS.md` policy. The scroll-coalescing path is now covered by
   `TestScrollCoalescingGoosieVsBrowser` (passes; ~8% diff, all from
   the 4px Fyne test-driver offset). Remaining visual gaps are
   pre-existing renderer-fidelity differences that also fail on
   `main` (`test_105_background`, `test_117/118/119_semantic`,
   `TestHTML5SemanticLayoutGoosieVsBrowser` ~12.9%,
   `TestLinkedSVGImageGoosieVsBrowser` 34.6% SVG-fidelity diff); the HUD
   still lacks a golden baseline.
