# Goosie Engine Event Loop Feature Design

> Feature: Go-native browser engine event loop for smoother scroll, mouse move, click, JavaScript callbacks, DOM mutations, and frame presentation.
>
> Goal: Prevent UI freezes during heavy interaction by ensuring the Fyne main thread only collects input and presents immutable frames, while engine scheduling and heavy rendering work happen outside the UI thread.

---

## 1. Problem Summary

Goosie currently behaves like a lightweight browser engine, but interactive usage can freeze or lag during heavy actions such as:

- continuous scroll
- mouse move / hover
- rapid click
- JavaScript callbacks
- DOM mutation bursts
- image-heavy page loading
- navigation while rendering is still in progress

The most likely root cause is not that Go is too slow. The issue is architectural:

```text
Input event -> heavy render work -> Fyne refresh
```

If scroll, click, mouse move, JavaScript, image callbacks, or mutations directly trigger parse/style/layout/display-list/raster/presentation work, the Fyne UI thread can be blocked. Once the UI thread is blocked, the app cannot process new input, repaint, or drain queued UI operations.

The existing freeze-fix work already introduced useful pieces such as scroll coalescing, frame metrics, requestAnimationFrame scheduling, and removal of one nested `fyne.Do` pattern. However, the remaining architectural issue is that some heavy work can still run synchronously with the UI/presentation path.

---

## 2. Core Design Principle

Goosie should move from direct event-driven rendering:

```text
OnScrolled -> RenderWithViewport -> Refresh
```

to an event-loop architecture:

```text
Input event
  -> Engine Event Loop
  -> Coalesce / prioritize / schedule
  -> Render Worker builds immutable FrameSnapshot
  -> Fyne UI thread presents FrameSnapshot only
```

The main principle:

```text
Fyne main thread = input collection + presentation only
Engine loop      = scheduling, state ownership, coalescing, cancellation
Render worker    = parse/style/layout/display-list/raster
JS session       = single owner goroutine for Goja runtime
```

---

## 3. Why Event Loop Instead of Just Queue or Stack

A queue or stack is only a data structure. It answers where tasks are stored.

An event loop is a scheduling policy. It answers:

- Which task runs first?
- Which task can be dropped?
- Which task must preserve ordering?
- How much work can run per frame?
- When should rendering happen?
- When should stale work be cancelled?
- When should input events be coalesced?
- When should JavaScript yield?

A simple FIFO queue can still freeze the app:

```text
scroll 1
scroll 2
scroll 3
mousemove 1
mousemove 2
render 1
render 2
render 3
```

The browser may spend time processing old states that are no longer visible.

A LIFO stack can process the newest input first, but it can starve important tasks such as cleanup, navigation cancellation, resource callbacks, and JavaScript timers.

Goosie needs a hybrid model:

```text
bounded channels
+ latest-value coalescing
+ frame budget
+ priority policy
+ cancellation
+ generation-based stale dropping
+ immutable frame handoff
```

---

## 4. Target Architecture

```text
┌─────────────────────────────────────────────┐
│ Fyne UI Thread                              │
│                                             │
│ Responsibilities:                           │
│ - collect scroll/mouse/click/key input      │
│ - update URL bar and browser chrome         │
│ - present immutable FrameSnapshot           │
│                                             │
│ Must NOT do:                                │
│ - parse HTML                                │
│ - run CSS selector matching                 │
│ - compute full layout                       │
│ - rebuild display list                      │
│ - decode images                             │
│ - execute long JavaScript                   │
└──────────────────────┬──────────────────────┘
                       │ PostInput / PresentFrame
                       ▼
┌─────────────────────────────────────────────┐
│ Engine Event Loop                           │
│                                             │
│ Responsibilities:                           │
│ - own engine state                          │
│ - coalesce input                            │
│ - manage generation IDs                     │
│ - schedule render jobs                      │
│ - route JavaScript tasks                    │
│ - batch DOM mutations                       │
│ - drop stale render results                 │
│ - enforce frame budget                      │
└──────────────────────┬──────────────────────┘
                       │ RenderRequest
                       ▼
┌─────────────────────────────────────────────┐
│ Render Worker / Pipeline                    │
│                                             │
│ Responsibilities:                           │
│ - build frame outside Fyne main thread      │
│ - apply style/layout invalidation           │
│ - build display list                        │
│ - raster visible tiles                      │
│ - produce immutable FrameSnapshot           │
└──────────────────────┬──────────────────────┘
                       │ RenderResult
                       ▼
┌─────────────────────────────────────────────┐
│ Presenter                                   │
│                                             │
│ Responsibilities:                           │
│ - verify frame is not stale                 │
│ - call Fyne presentation safely             │
│ - record input-to-present latency           │
└─────────────────────────────────────────────┘
```

---

## 5. Proposed Package Layout

```text
internal/engine/eventloop
  ├── loop.go
  ├── task.go
  ├── input.go
  ├── frame_budget.go
  ├── generation.go
  ├── coalescer.go
  ├── metrics.go
  ├── loop_test.go
  └── benchmark_test.go

internal/engine/renderworker
  ├── worker.go
  ├── request.go
  ├── result.go
  ├── snapshot.go
  ├── stale.go
  └── worker_test.go

internal/engine/mutation
  ├── record.go
  ├── batch.go
  └── apply.go

internal/engine/presenter
  ├── presenter.go
  └── fyne_presenter.go
```

This can be introduced gradually. The first PR should only implement the event loop skeleton and integrate the smallest safe path, such as scroll/mouse coalescing and latest-only render request scheduling.

---

## 6. Core Types

### 6.1 Task Kind

```go
type TaskKind int

const (
    TaskInput TaskKind = iota
    TaskNavigation
    TaskScript
    TaskMicrotask
    TaskMutation
    TaskResource
    TaskRender
    TaskIdle
)
```

### 6.2 Input Event

```go
type InputEventType int

const (
    InputScroll InputEventType = iota
    InputMouseMove
    InputClick
    InputKey
    InputResize
)

type InputEvent struct {
    Type      InputEventType
    Viewport  Viewport
    X         float32
    Y         float32
    Button    int
    Key       string
    Timestamp time.Time
}
```

Policy:

- scroll: latest wins
- mouse move: latest wins
- click: preserve order
- key input: preserve order
- resize: latest wins, but must trigger layout invalidation

### 6.3 Generation

```go
type Generation struct {
    Navigation uint64
    Document   uint64
    DOM        uint64
    Style      uint64
    Layout     uint64
    Viewport   uint64
}
```

Generation IDs allow stale work to be dropped safely.

Example:

```go
func (g Generation) Matches(current Generation) bool {
    return g.Navigation == current.Navigation &&
        g.Document == current.Document &&
        g.DOM == current.DOM &&
        g.Style == current.Style &&
        g.Layout == current.Layout &&
        g.Viewport == current.Viewport
}
```

### 6.4 Render Request

```go
type RenderReason int

const (
    RenderReasonNavigation RenderReason = iota
    RenderReasonViewport
    RenderReasonMutation
    RenderReasonImageLoaded
    RenderReasonResize
)

type RenderRequest struct {
    Context  context.Context
    Gen      Generation
    Viewport Viewport
    Reason   RenderReason
    Created  time.Time
}
```

### 6.5 Frame Snapshot

```go
type FrameSnapshot struct {
    ID         uint64
    Generation Generation
    Viewport   Viewport
    Size       Size

    DisplayList []DisplayCommand
    Tiles       []TileSnapshot

    Metrics FrameBuildMetrics
}
```

A `FrameSnapshot` must be immutable after creation. It is the handoff object between the engine/render worker and the Fyne UI thread.

### 6.6 Render Result

```go
type RenderResult struct {
    Request  RenderRequest
    Snapshot FrameSnapshot
    Err      error
    Finished time.Time
}
```

### 6.7 Presenter

```go
type Presenter interface {
    PresentFrame(snapshot FrameSnapshot)
}
```

Fyne implementation:

```go
type FynePresenter struct {
    canvas *CanvasView
}

func (p *FynePresenter) PresentFrame(snapshot FrameSnapshot) {
    fyne.Do(func() {
        p.canvas.PresentFrame(snapshot)
    })
}
```

---

## 7. Event Loop Design

### 7.1 Loop Structure

```go
type Loop struct {
    inputCh    chan InputEvent
    navCh      chan NavigationEvent
    scriptCh   chan ScriptTask
    resourceCh chan ResourceEvent
    mutationCh chan MutationRecord

    renderReqCh chan RenderRequest
    renderDoneCh chan RenderResult

    latestInput LatestInput
    generations GenerationState
    metrics     Metrics

    renderPending atomic.Bool
    presenter     Presenter
}
```

All channels must be bounded.

Recommended defaults:

```go
const (
    InputQueueSize    = 128
    NavigationQueueSize = 16
    ScriptQueueSize   = 256
    ResourceQueueSize = 128
    MutationQueueSize = 1024
    RenderQueueSize   = 1
)
```

### 7.2 Run Loop

```go
func (l *Loop) Run(ctx context.Context) {
    frame := time.NewTicker(time.Second / 60)
    defer frame.Stop()

    for {
        select {
        case <-ctx.Done():
            l.shutdown()
            return

        case ev := <-l.inputCh:
            l.handleInput(ev)

        case nav := <-l.navCh:
            l.handleNavigation(nav)

        case task := <-l.scriptCh:
            l.enqueueScript(task)

        case res := <-l.resourceCh:
            l.handleResource(res)

        case mut := <-l.mutationCh:
            l.enqueueMutation(mut)

        case result := <-l.renderDoneCh:
            l.handleRenderResult(result)

        case <-frame.C:
            l.tickFrame(ctx)
        }
    }
}
```

### 7.3 Frame Tick

```go
func (l *Loop) tickFrame(ctx context.Context) {
    budget := NewFrameBudget(16 * time.Millisecond)

    l.processNavigationControl()
    l.processLatestInput()

    l.runScriptTasks(budget.Slice(4 * time.Millisecond))
    l.drainMicrotasks(budget.Slice(2 * time.Millisecond))

    mutations := l.collectMutationBatch()
    if len(mutations) > 0 {
        l.applyMutations(mutations)
        l.markDirtyFromMutations(mutations)
    }

    if l.needsRender() {
        l.scheduleLatestRender(ctx)
    }

    l.runIdleWork(budget.Remaining())
}
```

---

## 8. Scheduling Policies

### 8.1 Scroll

Policy: latest wins.

```text
scroll y=10
scroll y=30
scroll y=80
scroll y=140
  -> render only y=140
```

### 8.2 Mouse Move

Policy: latest wins and throttle to frame rate.

Mouse move should not force render/hit-test for every event.

### 8.3 Click and Key Input

Policy: preserve order.

Click and keyboard input represent user intent. They should not be dropped casually.

### 8.4 JavaScript Task

Policy: one owner goroutine, bounded duration.

All Goja access should be routed through `js.Session` or an equivalent owner queue.

```text
click handler
fetch callback
timer callback
requestAnimationFrame callback
  -> JS owner queue
```

### 8.5 Microtasks

Policy: bounded drain.

```go
const MaxMicrotasksPerTick = 1000
const MaxMicrotaskDuration = 2 * time.Millisecond
```

If microtasks exceed the limit, record a metric and yield to the next frame.

### 8.6 DOM Mutations

Policy: batch per frame.

```text
setAttribute
appendChild
textContent change
class change
  -> MutationBatch
  -> one invalidation
  -> one render request
```

Avoid full DOM serialization and reparsing.

### 8.7 Render

Policy: latest-only render queue.

The render request channel should have size 1. If a new render request arrives while an old one is queued, replace the old one.

```go
func replaceLatestRenderRequest(ch chan RenderRequest, req RenderRequest) {
    select {
    case <-ch:
    default:
    }

    select {
    case ch <- req:
    default:
    }
}
```

### 8.8 Resource Callbacks

Policy: batch and prioritize visible resources.

Image loaded events should not trigger a full-tree render per image. They should be batched and applied once per frame.

---

## 9. Cancellation and Stale Work

### 9.1 Navigation Cancellation

When a new navigation starts:

```go
func (l *Loop) handleNavigation(nav NavigationEvent) {
    l.cancelCurrentNavigation()

    l.generations.Navigation++
    l.generations.Document++
    l.generations.DOM++
    l.generations.Style++
    l.generations.Layout++

    ctx, cancel := context.WithCancel(l.rootCtx)
    l.currentNavCancel = cancel

    l.startDocumentLoad(ctx, nav.URL)
}
```

### 9.2 Render Cancellation

When a new render request supersedes an older one:

```go
func (l *Loop) scheduleLatestRender(ctx context.Context) {
    l.cancelCurrentRender()

    renderCtx, cancel := context.WithCancel(ctx)
    l.currentRenderCancel = cancel

    req := RenderRequest{
        Context:  renderCtx,
        Gen:      l.generations.Current(),
        Viewport: l.latestInput.Viewport(),
        Reason:   RenderReasonViewport,
        Created:  time.Now(),
    }

    replaceLatestRenderRequest(l.renderReqCh, req)
}
```

### 9.3 Stale Result Dropping

```go
func (l *Loop) handleRenderResult(result RenderResult) {
    if result.Err != nil {
        l.metrics.RenderErrors++
        return
    }

    if !result.Request.Gen.Matches(l.generations.Current()) {
        l.metrics.StaleFramesDropped++
        return
    }

    l.presenter.PresentFrame(result.Snapshot)
    l.metrics.FramesPresented++
}
```

---

## 10. Render Worker Design

```go
type RenderWorker struct {
    reqCh    <-chan RenderRequest
    resultCh chan<- RenderResult
    pipeline RenderPipeline
}

func (w *RenderWorker) Run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return

        case req := <-w.reqCh:
            started := time.Now()
            snapshot, err := w.pipeline.BuildFrame(req.Context, req)
            snapshot.Metrics.Duration = time.Since(started)

            select {
            case w.resultCh <- RenderResult{
                Request:  req,
                Snapshot: snapshot,
                Err:      err,
                Finished: time.Now(),
            }:
            case <-ctx.Done():
                return
            }
        }
    }
}
```

The pipeline should check `ctx.Err()` between major phases:

```text
parse
check ctx
style
check ctx
layout
check ctx
display-list
check ctx
raster
check ctx
snapshot
```

---

## 11. Metrics

The event loop must expose actionable metrics.

```go
type Metrics struct {
    InputEventsReceived     uint64
    InputSignalsDropped     uint64
    CoalescedScrollEvents   uint64
    CoalescedMouseMoves     uint64

    RenderRequestsCreated   uint64
    RenderRequestsDropped   uint64
    RenderErrors            uint64
    StaleFramesDropped      uint64
    FramesPresented         uint64

    MaxUIQueueWait          time.Duration
    MaxInputToPresent       time.Duration
    MaxRenderDuration       time.Duration

    LongJSTasks             uint64
    MicrotaskBudgetExceeded uint64
    MutationBatches         uint64
    ResourceBatches         uint64
}
```

Recommended HUD:

```text
FPS 60
i→p 12ms q 3ms
render 7ms stale 4
coalesced s120 m300
longJS 2 mutBatch 40
```

---

## 12. Go-Specific Implementation Notes

### 12.1 Goroutines

Use goroutines for ownership and background work, not for every event.

Good:

```text
1 EngineLoop goroutine
1 JS owner goroutine
N bounded render/raster workers
N bounded image decode workers
```

Bad:

```text
1 goroutine per scroll
1 goroutine per mouse move
1 goroutine per DOM node
1 goroutine per CSS rule
```

### 12.2 Channels

Use bounded channels to enforce backpressure.

```go
inputCh := make(chan InputEvent, 128)
renderReqCh := make(chan RenderRequest, 1)
```

### 12.3 Select

Use `select` to implement the event loop and coordinate cancellation.

### 12.4 Context

Every navigation, resource load, render job, and long task must accept `context.Context`.

### 12.5 Atomic Latest State

Use `atomic.Value` or a small mutex to store latest viewport/mouse state.

### 12.6 Immutable Snapshot

Use immutable snapshots as handoff objects to avoid data races between engine and UI.

### 12.7 sync.Pool

Use `sync.Pool` only for short-lived scratch buffers with clear ownership.

Do not pool objects that hold references to DOM/session/global mutable state.

### 12.8 pprof and trace

Add tooling or docs for:

```bash
go test -trace trace.out ./internal/renderer
go tool trace trace.out

go test -cpuprofile cpu.out -bench=. ./internal/renderer
go tool pprof cpu.out

go test -bench=. -benchmem ./internal/renderer
```

---

## 13. Migration Plan

### PR 1 — Event Loop Skeleton

Create `internal/engine/eventloop` with:

- `Loop`
- `TaskKind`
- `InputEvent`
- `RenderRequest`
- `RenderResult`
- `Generation`
- `FrameBudget`
- `Metrics`
- latest-wins scroll/mouse coalescing
- render queue size 1
- stale render result dropping

Tests:

- bursty scroll collapses to latest viewport
- mouse move collapses to latest position
- click events preserve order
- render queue replaces older queued request
- stale render result is dropped
- cancellation stops scheduled work
- metrics increment correctly

Benchmark:

- bursty scroll scheduling
- render-request replacement

### PR 2 — Route UI Input Through Event Loop

Replace direct render calls from scroll/mouse paths with `EngineLoop.PostInput`.

Goal:

```text
OnScrolled -> PostInput
not
OnScrolled -> RenderWithViewport
```

Tests:

- rapid scroll does not call renderer for every event
- latest viewport wins
- input-to-present metric is recorded

### PR 3 — Latest-Only Render Scheduling

Integrate render request generation and stale dropping with the current renderer.

Goal:

- render queue size 1
- old render requests are replaced
- render result with old generation is dropped

### PR 4 — Split BuildFrame and PresentFrame

Separate heavy frame construction from Fyne presentation.

Goal:

```text
BuildFrame runs outside Fyne UI thread
PresentFrame runs on Fyne UI thread
```

This is the most important PR for freeze reduction.

### PR 5 — JS Session Owner for GUI Tabs

Route GUI JavaScript through a single owner session.

Goal:

```text
all event handlers / timers / fetch callbacks -> js.Session.Post
```

Tests:

- async callbacks do not access Goja from non-owner goroutines
- navigation cleanup cancels queued JS tasks
- long tasks are counted/interrupted

### PR 6 — Structured Mutation Records

Replace full DOM serialize/reparse mutation handling.

Goal:

```text
JS mutation -> MutationRecord -> compact DOM apply -> invalidation
```

Tests:

- 100 class changes produce one mutation batch
- no full reparse for simple attribute/text mutations
- stale NodeID or generation is rejected safely

### PR 7 — Image Invalidation Batching

Batch image-loaded callbacks per frame.

Goal:

```text
100 image loaded callbacks -> one resource batch -> one render request
```

Tests:

- image-heavy page does not trigger full render per image
- offscreen images do not force immediate present
- visible image dirty rect triggers repaint

### PR 8 — Non-Blocking First Paint

Allow first paint before non-blocking resources finish.

Goal:

```text
main HTML + blocking CSS -> first paint
images/fonts continue async
```

---

## 14. Definition of Done

A PR for this feature is complete only if:

- Fyne main thread is not assigned new heavy work.
- All new queues/channels are bounded.
- Scroll and mouse move are coalesced.
- Render queue keeps only the latest render request.
- Stale render results are dropped by generation.
- Context cancellation is propagated.
- Tests cover bursty input, stale work, cancellation, and metrics.
- Benchmarks cover the affected scheduling path.
- Docs explain what changed and what remains.

---

## 15. Test Plan

### Unit Tests

```text
internal/engine/eventloop
  - TestScrollCoalescesToLatestViewport
  - TestMouseMoveCoalescesToLatestPosition
  - TestClickPreservesOrder
  - TestRenderQueueReplacesOldRequest
  - TestStaleRenderResultDropped
  - TestNavigationBumpsGeneration
  - TestCancellationStopsRenderRequest
  - TestMetricsIncrement
```

### Integration Tests

```text
internal/ui
  - rapid scroll does not synchronously render every event
  - browser remains responsive during bursty input
  - latest viewport is eventually presented
```

### Race Tests

```bash
go test -race ./internal/engine/eventloop
```

Later:

```bash
go test -race ./internal/ui ./internal/js ./internal/renderer
```

### Benchmarks

```bash
go test -bench=. -benchmem ./internal/engine/eventloop
```

Recommended benchmarks:

```text
BenchmarkBurstScrollScheduling
BenchmarkRenderRequestReplacement
BenchmarkGenerationCheck
BenchmarkInputCoalescer
```

---

## 16. Coding Agent Prompt

```text
You are working on `vyquocvu/goosie`.

Implement the first vertical slice of a Go-native engine event loop to reduce UI freezes during heavy scroll, mouse move, and click interaction.

Read first:

- `docs/FREEZE_FIXES.md`
- `internal/ui/browser.go`
- `internal/renderer/canvas.go`
- `internal/renderer/scrollcoalescer.go`
- `internal/renderer/framemetrics.go`
- `internal/js/runtime.go`
- `internal/js/session.go`
- `cmd/browser/mutation_coalesce_test.go`
- relevant renderer and UI tests

Create `internal/engine/eventloop` with:

- `Loop`
- `TaskKind`
- `InputEvent`
- `RenderRequest`
- `RenderResult`
- `Generation`
- `FrameBudget`
- `Metrics`
- latest-wins scroll/mouse coalescing
- render queue size 1
- stale render result dropping

Do not rewrite the full renderer in this first PR.

Rules:

- Keep Fyne main thread presentation-only for any touched paths.
- Do not introduce unbounded goroutines, queues, channels, timers, or caches.
- Use `context.Context` for cancellation.
- Coalesce bursty input.
- Drop stale render results by generation.
- Add unit tests for coalescing, generation mismatch, render replacement, cancellation, and metrics.
- Add a benchmark for bursty scroll scheduling.
- Update `docs/FREEZE_FIXES.md` with completed and remaining work.

Validation:

- `gofmt -w .`
- `go test ./internal/engine/eventloop -count=1`
- `go test ./internal/engine/eventloop -bench=. -benchmem`
- `go test ./internal/ui ./internal/renderer -short`
- `go test ./... -short`

If full repository tests fail because of known Fyne/OpenGL/X11 environment limitations, document the exact failure and run the largest valid subset.

Open a focused PR with:

- root cause
- design summary
- package structure
- tests
- benchmarks
- known limitations
- next PR recommendation
```

---

## 17. Summary

The recommended feature is not just a queue. It is a Go-native browser event loop.

The correct direction is:

```text
UI thread
  = input + presentation only

Engine loop
  = scheduling + coalescing + cancellation + generation ownership

Render worker
  = heavy frame construction

Snapshot
  = immutable handoff between engine and UI
```

This design uses Go strengths directly:

- goroutines for actor-style ownership and workers
- channels for bounded message passing
- select for event loop coordination
- context for cancellation
- atomic/latest-value state for scroll and mouse move
- generation IDs for stale-work dropping
- immutable snapshots to avoid races
- pprof/trace/benchmem for diagnosis

This should reduce freeze because interaction no longer forces synchronous heavy rendering on the Fyne main thread.
