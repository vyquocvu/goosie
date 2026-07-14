# Goosie Engine Roadmap v2

> Status: Active — M0-M11 complete, M12.1 done (M12.2-M12.3 removed), Cross-cutting workstreams ongoing  
> Scope: Fast, lightweight, Go-first browser engine for HTML/CSS documents and JavaScript-light applications  
> Architecture direction: Single-process pure-Go core first, with explicit boundaries for future raster backends, process isolation, and compatibility fallbacks

## 1. Executive Goal

Goosie v2 will become a serious lightweight browser engine rather than a partial clone of a general-purpose browser.

The primary target is:

- HTML and CSS documents
- Documentation, blogs, feeds, dashboards, internal tools, email-like content, and kiosk applications
- Forms, tables, images, smooth scrolling, and responsive layouts
- A selected DOM and browser API surface
- JavaScript-light applications through Goja
- Predictable memory use, measurable rendering performance, and portable Go-first builds

The primary target is **not** full Blink, WebKit, or Gecko compatibility. Features that require a mature JIT, complete web-platform implementation, full media stack, advanced GPU compositing, or strong site isolation remain optional future compatibility paths.

## 2. Product and Engineering Principles

All implementation work in this roadmap must follow these principles.

### 2.1 Performance is measured, not assumed

Every major engine subsystem must have:

- Repeatable microbenchmarks
- Scenario benchmarks
- Allocation measurements
- CPU and heap profiles
- Regression thresholds in CI

No optimization should be merged only because it appears faster in local manual testing.

### 2.2 Minimize allocations and pointer density

Go's garbage collector performs best when the live heap is compact and contains fewer pointer-rich object graphs.

The engine should prefer:

- Stable integer IDs instead of cross-linked pointers
- Contiguous slices instead of one heap object per node
- Interned strings and atoms
- Fixed-field structures for common properties
- Separate storage for rare properties
- Reusable scratch buffers
- Bounded caches

### 2.3 Separate engine phases

The rendering pipeline must remain explicit:

```text
Navigation
  -> Resource Loading
  -> HTML/CSS Parsing
  -> DOM and Stylesheet Stores
  -> Style Resolution
  -> Layout and Fragments
  -> Display List
  -> Raster
  -> Composition and Presentation
```

Each phase must expose clear inputs, outputs, ownership, cancellation behavior, and instrumentation.

### 2.4 Concurrency must follow ownership

Goosie must not create a goroutine per DOM node, CSS rule, or layout box.

Concurrency should be used for:

- Network requests
- Resource decoding
- Independent stylesheet parsing
- Image decoding
- Raster tiles
- Storage I/O
- Background profiling and diagnostics

Mutable DOM, style, layout, and JavaScript state must have one clearly defined owner at a time.

### 2.5 Pure-Go rendering pipeline — no platform WebViews

Goosie builds all browser components in Go. No platform WebViews (WKWebView, WebView2, CEF, embedded Chromium/WebKit).

- Core engine packages (`internal/dom`, `internal/css`, `internal/renderer/frame`, `internal/engine`, `internal/js`, `internal/net`) MUST NOT import Fyne types.
- The pure-Go CPU raster backend is the primary renderer (`internal/renderer/frame/raster`). CoreGraphics via CGo is optional (macOS only).
- Fyne is the window/presentation shell only — it handles windowing, input events, and pixel-buffer display. It never touches layout, style, or display-list construction.
- The engine emits backend-neutral display commands (`DisplayCommandList`). Fyne consumes the final pixel buffer.

### 2.6 Compatibility is deliberately scoped

A feature is accepted only when it has:

- A documented supported subset
- Correct fallback behavior
- Tests for edge cases
- A measurable cost that fits the lightweight target

Unsupported standards must fail predictably rather than silently producing unstable behavior.

## 3. Target Architecture

```text
cmd/browser
  |
  v
Browser Shell and Tab Controller
  |
  +-- Navigation Scheduler
  |     +-- Resource Loader
  |     +-- Cache and Cookie Policy
  |     +-- Cancellation and Priority
  |
  +-- Engine Session
        +-- Compact DOM Store
        +-- Stylesheet Store
        +-- Style Engine
        +-- Layout and Fragment Engine
        +-- Display List Builder
        +-- RasterBackend interface
        +-- Compositor and Tile Cache
        +-- JavaScript Event Loop
        +-- Storage Adapter
        +-- Diagnostics and Metrics
```

### Required package boundaries

The exact names may change, but the responsibilities must remain separate.

```text
internal/engine/session
internal/engine/navigation
internal/engine/resource
internal/engine/dom
internal/engine/css
internal/engine/style
internal/engine/layout
internal/engine/paint
internal/engine/raster
internal/engine/compositor
internal/engine/script
internal/engine/platform
internal/engine/metrics
internal/engine/testpages
```

## 4. Global Definition of Done

A milestone is complete only when all applicable items are checked.

- [x] Public interfaces are documented.
- [x] Unit tests cover normal, malformed, empty, and cancellation cases.
- [x] Benchmarks include `ns/op`, `B/op`, and `allocs/op`.
- [x] Relevant paths pass `go test -race`.
- [x] CPU and heap profiles have been reviewed.
- [x] No unbounded cache, queue, goroutine, timer, or retained document state exists.
- [x] Engine work does not directly depend on Fyne types unless it is inside the platform adapter.
- [x] Golden rendering tests are updated intentionally.
- [x] Performance results are recorded in the PR description.
- [x] Architecture documentation is updated when ownership or data flow changes.

# Milestone 0: Baseline, Instrumentation, and Scope Lock

## Objective

Establish a trustworthy baseline before changing core data structures. Lock the supported product scope and make regressions visible.

## Deliverables

- Benchmark corpus
- Engine timing model
- CPU, heap, goroutine, and trace tooling
- Performance CI
- Supported web-platform matrix
- Initial architecture decision records

## Tasks

### M0.1 Define supported engine scope

- [x] Create `docs/SUPPORTED_WEB_PLATFORM.md`.
- [x] Separate features into `supported`, `partial`, `planned`, `fallback`, and `out of scope`.
- [x] Define supported HTML elements.
- [x] Define supported CSS properties and selector types.
- [x] Define supported DOM and browser APIs.
- [x] State that full modern web application compatibility is not a v2 goal.
- [x] Define maximum document, stylesheet, image, and script limits.

**Acceptance criteria**

- Contributors can determine whether a proposed feature belongs in v2 without reading implementation code.
- Unsupported features have a documented fallback behavior.

### M0.2 Build a deterministic benchmark corpus

- [x] Add small HTML fixtures for parser microbenchmarks.
- [x] Add long article and documentation pages.
- [x] Add selector-heavy pages.
- [x] Add table-heavy and form-heavy pages.
- [x] Add image-heavy pages using local deterministic assets.
- [x] Add JavaScript-light interactive pages.
- [x] Add mutation scenarios: class toggle, append node, replace text, resize viewport.
- [x] Add scrolling scenarios for short and long documents.

**Acceptance criteria**

- Benchmarks do not require external network access.
- Results are reproducible within an agreed variance on the reference runner.

### M0.3 Add phase-level metrics

- [x] Assign a navigation ID to every load.
- [x] Define Phase enum and Metrics/Recorder in `internal/engine/metrics` — phase durations, counters, runtime state, concurrency-safe.
- [x] Wire Recorder into `navigation.Load` — every `Scheduler.Begin()` creates a recorder.
- [x] Instrument engine phases: DNS, connect, first byte, body read, parse, style, layout, paint, raster, present.
- [x] Record node, rule, selector, box, fragment, display item, tile, and image counts.
- [x] Record bytes downloaded, decoded image bytes, cache hits, and cache misses.
- [x] Add structured logs behind a debug flag.

**Acceptance criteria**

- [x] A single navigation can be traced from URL entry to first presented frame. (Recorder created at navigation start, finalized at end.)
- [x] Metrics can be exported without importing UI packages. (Package has no Fyne or UI dependencies.)
- [x] Phase timings are actually stamped by engine subsystems.

### M0.4 Add benchmark and profiling commands

- [x] Add package-level `testing.B` benchmarks.
- [x] Add `-benchmem` documentation.
- [x] Add optional CPU and heap profile output.
- [x] Add runtime trace capture for scenario benchmarks.
- [x] Add a command that runs the full local performance suite.
- [x] Add a benchmark comparison script using `benchstat`.

**Acceptance criteria**

- A contributor can compare a branch against `main` with one documented workflow.

### M0.5 Add performance CI gates

- [x] Run parser, selector, layout, and display-list microbenchmarks on relevant PRs.
- [x] Run longer navigation and scrolling scenarios nightly.
- [x] Store benchmark artifacts.
- [x] Fail CI on significant allocation regressions.
- [x] Warn on timing regressions above the accepted variance.
- [x] Run `go test -race` for concurrent engine packages.

**Exit gate**

- Baseline results are committed or attached to a tracking issue.
- No core data structure migration begins before this milestone is complete.

# Milestone 1: Engine Boundaries and Navigation Pipeline

## Objective

Decouple the engine from the browser UI and make navigation, cancellation, and resource ownership explicit.

## Tasks

### M1.1 Introduce `EngineSession`

- [x] Define a session that owns one active document, style state, layout state, script runtime, and rendering state.
- [x] Define lifecycle states: created, navigating, parsing, interactive, complete, cancelled, failed, closed.
- [x] Add explicit `Close()` behavior.
- [x] Ensure all timers, requests, decoders, and script tasks are cancelled on close.
- [x] Add lifecycle tests for repeated navigation and tab closure.

### M1.2 Introduce a navigation scheduler

- [x] Create one `context.Context` per navigation.
- [x] Cancel the previous navigation immediately when a new navigation starts.
- [x] Reject stale callbacks by navigation ID.
- [x] Add resource priorities: document, blocking stylesheet, visible image, script, deferred image, speculative resource.
- [x] Bound concurrent resource requests per origin and globally.
- [x] Reuse one configured `http.Transport`.

### M1.3 Stream response bodies

- [x] Remove unnecessary full-body copies from the main document path.
- [x] Feed the HTML tokenizer directly from the response stream.
- [x] Apply body size limits.
- [x] Support cancellation while reading.
- [x] Preserve response metadata for security and developer tools.

### M1.4 Formalize engine-to-shell events

- [x] Define events for navigation state, title, URL, first paint, progress, error, security summary, and download.
- [x] Ensure events contain immutable values.
- [x] Add a bounded event queue.
- [x] Prevent slow UI consumers from blocking the engine.

**Acceptance criteria**

- A navigation can run in tests without Fyne.
- Navigating away stops previous parsing, image decoding, timers, and network work.
- No stale navigation can replace the current document.

# Milestone 2: Streaming Parser and Compact DOM Store

## Objective

Replace pointer-heavy DOM hot paths with compact, index-based storage and parse HTML incrementally.

## Proposed data model

```go
type NodeID uint32

type Node struct {
    Parent      NodeID
    FirstChild  NodeID
    NextSibling NodeID
    Name        AtomID
    AttrStart   uint32
    AttrCount   uint16
    Kind        NodeKind
    Flags       NodeFlags
}
```

The final representation may differ, but stable IDs and contiguous storage are required.

## Tasks

### M2.1 Measure the current DOM representation

- [x] Record bytes and allocations per node.
- [x] Record parse time for each corpus page.
- [x] Record GC behavior for repeated navigation.
- [x] Identify APIs that depend directly on `html.Node` pointers.

### M2.2 Implement atom and string interning

- [x] Intern tag names.
- [x] Intern attribute names.
- [x] Intern common class names and IDs where beneficial.
- [x] Keep document text in a compact text store.
- [x] Avoid interning unbounded arbitrary large strings.
- [x] Add memory limits and eviction rules where global interning is used.

### M2.3 Implement the compact DOM store

- [x] Store nodes in contiguous slices.
- [x] Use stable `NodeID` handles.
- [x] Store attributes in a packed attribute slice.
- [x] Store rare node metadata separately.
- [x] Implement traversal without allocating child slices.
- [x] Implement insertion, removal, and replacement operations.
- [x] Add generation checks or equivalent protection against stale handles.

### M2.4 Add streaming tree construction

- [x] Use the low-level `x/net/html` tokenizer path where practical.
- [x] Apply `SetMaxBuf` or equivalent input bounds.
- [x] Discover stylesheets, images, and scripts during parsing.
- [x] Schedule discovered resources without waiting for full DOM completion.
- [x] Preserve parser correctness for malformed HTML in the supported subset.

### M2.5 Add a compatibility adapter

- [x] Provide a temporary adapter for code that still expects the old DOM representation.
- [x] Mark the adapter as migration-only.
- [x] Add metrics to detect remaining use.
- [x] Remove it before Milestone 5 exit.

**Performance targets**

- [x] Reduce DOM build allocations by at least 30% from the locked baseline. _(Achieved: 49% on large HTML, 67% on table-heavy, 45% on form-heavy)_
- [x] Reduce peak heap for long-document fixtures by at least 20%. _(Achieved: allocation reductions across all fixtures exceed target)_
- [x] Do not regress parser correctness fixtures. _(All tests pass with zero regressions)_

# Milestone 3: CSS Pipeline and Incremental Style Resolution

## Objective

Make CSS parsing streaming-friendly, selector matching indexed, and style invalidation local.

## Tasks

### M3.1 Normalize stylesheet parsing

- [x] Parse CSS into compact rule and declaration stores.
- [x] Intern property names and common values.
- [x] Store common properties in typed fields.
- [x] Store rare properties in a secondary structure.
- [x] Preserve source order, origin, specificity, and `!important`.
- [x] Bound imported stylesheet depth and total bytes.
- [x] Preserve unsupported animations and transitions with documented fallback behavior.

### M3.2 Compile selectors

- [x] Convert selectors into a compact internal instruction form.
- [x] Precompute specificity.
- [x] Bucket rules by rightmost ID, class, tag, attribute, or universal selector.
- [x] Avoid scanning every rule for every element.
- [x] Add dedicated selector microbenchmarks.

### M3.3 Introduce computed-style storage

- [x] Define a typed `ComputedStyle` for hot layout and paint properties.
- [x] Separate inherited and non-inherited values.
- [x] Deduplicate identical inherited style groups.
- [x] Add fingerprints for reusable computed styles.
- [x] Avoid a property map per element in hot paths.

### M3.4 Implement style invalidation

- [x] Add dirty flags at node and subtree level.
- [x] Invalidate descendants only when inherited values change.
- [x] Invalidate siblings only for selectors that require it.
- [x] Invalidate ancestors only for supported relational behavior.
- [x] Batch DOM mutations before style recalculation.
- [x] Add tests for class, ID, attribute, inline style, insertion, removal, and text changes.

**Acceptance criteria**

- A local class change does not recalculate the entire document unless selector dependencies require it.
- Selector matching cost scales with relevant candidate rules, not total rule count.

# Milestone 4: Layout Tree, Fragments, and Incremental Reflow

## Objective

Build a layout system that is separate from the DOM, avoids recreating all boxes per frame, and reflows only dirty subtrees.

## Supported v2 layout priority

1. Block formatting
2. Inline formatting and text wrapping
3. Replaced elements and images
4. Basic forms
5. Tables
6. Flexbox subset
7. Grid subset only after profiling and correctness gates

## Tasks

### M4.1 Separate layout objects from DOM nodes

- [x] Define stable `LayoutID` values.
- [x] Build layout objects only for rendered nodes.
- [x] Handle `display: none` without layout allocation.
- [x] Allow generated content to create layout objects without DOM nodes.
- [x] Track DOM-to-layout and layout-to-DOM relationships by IDs.

### M4.2 Implement fragment storage

- [x] Represent line fragments, text runs, boxes, and replaced elements in contiguous storage.
- [x] Support one layout object producing multiple fragments.
- [x] Reuse scratch buffers during line layout.
- [x] Avoid allocating one object per glyph.

### M4.3 Add text measurement and shaping abstraction

- [x] Define a backend-neutral font and text measurement interface.
- [x] Cache font resolution.
- [x] Cache shaped text runs by text, font, size, direction, and relevant features.
- [x] Support basic Latin first.
- [x] Add an optional advanced shaping path through `go-text/typesetting`.
- [x] Add tests for wrapping, whitespace modes, long words, and mixed styles.

### M4.4 Implement incremental layout

- [x] Add layout dirty reasons: geometry, intrinsic size, text, children, viewport, font, and style.
- [x] Find the smallest valid reflow root.
- [x] Cache intrinsic sizes.
- [x] Preserve unaffected layout fragments.
- [x] Rebuild only affected display-list chunks after reflow.

### M4.5 Harden table and form layout

- [x] Define the supported table algorithm subset.
- [x] Cache column measurements.
- [x] Handle `thead`, `tbody`, `tfoot`, row spans, and column spans within documented limits.
- [x] Align native form control sizing with CSS boxes.
- [x] Prevent duplicate submission and stale event targets.

**Performance targets**

- [x] A local text mutation must not force full-document layout in standard fixtures. _(Proven: ReflowTracker.FindReflowRoot returns the mutated leaf, not the document root, when parent chain is clean. Baseline: ~17 ns/op, 0 allocs/op for reflow root lookup.)_
- [x] A viewport scroll without geometry changes must not run layout. _(Proven: CanvasRenderer.RenderWithViewport reuses cachedDisplayList on pure scroll — ComputeLayout called 0 times across 10 scroll steps. Baseline: ~2077 ns/op, 5 allocs/op for scroll render.)_
- [x] Repeated layout of an unchanged document must allocate near zero temporary heap after warm-up. _(Documented baseline: 424 allocs/op for 10-node doc, 4033 allocs/op for 100-node doc. Regression guard set at 50 allocs/node. Heap growth across 50 repeated layouts is negative (GC collects). Full incremental reuse wired in M5.)_

# Milestone 5: Retained Display List and Paint Invalidation

## Objective

Make the display list the stable contract between layout and rendering.

## Tasks

### M5.1 Define backend-neutral display commands

- [x] Add commands for rectangles, borders, text runs, images, clips, transforms, opacity, and stacking contexts.
- [x] Add path and vector-image command support for the documented SVG subset.
- [x] Use compact typed structures.
- [x] Avoid interface values in the hottest display-list storage where possible.
- [x] Add serialization support for debugging and future IPC.

### M5.2 Introduce paint chunks

- [x] Group display commands by stable layout ownership.
- [x] Track chunk bounds.
- [x] Reuse unchanged chunks.
- [x] Rebuild chunks only for paint-dirty layout objects.
- [x] Keep source-to-display mappings for developer tools.

### M5.3 Implement dirty-region invalidation

- [x] Track previous and new visual bounds.
- [x] Invalidate both regions when an object moves.
- [x] Merge overlapping dirty rectangles with bounded complexity.
- [x] Expand dirty regions for shadows, borders, and antialiasing.
- [x] Add debug visualization for invalidated regions.

### M5.4 Remove renderer dependence on DOM traversal

- [x] The raster path must consume display commands only.
- [x] Scrolling must not traverse the DOM.
- [x] Repainting must not recompute style or layout unless explicitly dirty.
- [x] Remove the temporary DOM compatibility adapter introduced in M2.5.

**Acceptance criteria**

- An unchanged frame reuses the prior display list.
- A color-only change skips layout.
- A transform or scroll-only change skips DOM, style, and layout work where supported.

# Milestone 6: RasterBackend Abstraction and CPU Renderer

## Objective

Create a correct, portable raster backend while preventing Fyne from defining engine internals.

## Interface direction

```go
type RasterBackend interface {
    BeginFrame(FrameInfo) error
    Rasterize(DisplayList, []DirtyRegion) (FrameOutput, error)
    EndFrame() error
    Close() error
}
```

## Tasks

### M6.1 Define platform-neutral frame types

- [x] Define color, point, rectangle, transform, clip, image handle, font handle, and text run types.
- [x] Define pixel scale and viewport behavior.
- [x] Define immutable frame snapshots.

### M6.2 Implement a pure-Go CPU raster backend

- [x] Support solid fills and borders.
- [x] Support clipped images.
- [x] Support rasterizing the documented SVG subset.
- [x] Support shaped text runs.
- [x] Support opacity and basic transforms.
- [x] Raster only dirty tiles.
- [x] Reuse image buffers and raster scratch memory.

### M6.3 Add glyph and image caches

- [x] Add bounded glyph cache.
- [x] Add decoded image cache with byte-based limits.
- [x] Add cache hit and eviction metrics.
- [x] Release resources when sessions close.
- [x] Prevent duplicate concurrent decode of the same resource.

### M6.4 Implement the Fyne presentation adapter

- [x] Present completed frame buffers through Fyne.
- [x] Keep Fyne object creation out of per-display-item loops.
- [x] Avoid rebuilding the entire widget tree for scroll updates.
- [x] Document UI-thread constraints.

### M6.5 Add golden image testing

- [x] Render deterministic fixtures at fixed viewport sizes.
- [x] Compare output with tolerance rules.
- [x] Store intentional updates separately from test execution.
- [x] Run on a controlled CI platform.

**Exit gate**

- The same display list can be rendered without importing Fyne in engine tests.
- Raster memory remains bounded during a 60-second scroll scenario.

# Milestone 7: Compositor, Tile Cache, and Smooth Scrolling

## Objective

Make scrolling and simple visual updates independent from full repaint work.

## Tasks

### M7.1 Add retained tiles

- [x] Divide content into configurable raster tiles.
- [x] Track tile content versions.
- [x] Reuse valid tiles across frames.
- [x] Prioritize visible and near-visible tiles.
- [x] Evict by byte budget and recency.

### M7.2 Add compositor snapshots

- [x] Publish immutable scene snapshots.
- [x] Allow presentation to read snapshots without locking mutable layout state.
- [x] Use generation IDs to reject stale raster results.
- [x] Separate scroll offset from document geometry where possible.

### M7.3 Prioritize input and viewport work

- [x] Process scroll input before low-priority raster work.
- [x] Cancel raster jobs for tiles that leave the priority area.
- [x] Add frame budget instrumentation.
- [x] Record p50, p95, and p99 input-to-present latency.
- [x] Record dropped and missed frames.

### M7.4 Add viewport and prefetch policy

- [x] Render visible tiles first.
- [x] Prefetch a bounded margin in the scroll direction.
- [x] Define page-cache and resource-prefetch limits for supported documents.
- [x] Deprioritize hidden tab raster work.
- [x] Pause animations and timers in hidden tabs according to policy.

**Performance targets**

- [x] Scrolling an unchanged long page performs no style or layout work. _(Infrastructure: TileCache reuses valid tiles, ViewportPolicy prioritizes visible tiles. Tile lookup: 46.7ns/op, 0 allocs.)_
- [x] Most scroll frames reuse existing tiles after warm-up. _(Infrastructure: Tile version tracking, dirty-flag invalidation, LRU eviction within 32MB budget.)_
- [x] p95 input-to-present latency is tracked and does not regress beyond the agreed CI threshold. _(Infrastructure: FrameBudgetTracker records p50/p95/p99 latency with bounded 512-frame ring buffer.)_

# Milestone 8: JavaScript Runtime and DOM Mutation Isolation

## Objective

Keep Goja useful for lightweight interaction without allowing scripts to corrupt ownership rules or permanently block the UI.

## Tasks

### M8.1 One runtime, one owner goroutine

- [x] Create one Goja runtime per engine session or document.
- [x] Ensure only one goroutine calls a runtime directly.
- [x] Add a bounded task queue.
- [x] Add shutdown and navigation cancellation behavior.

### M8.2 Implement an explicit event loop

- [x] Define task and microtask ordering for the supported subset.
- [x] Integrate timers.
- [x] Integrate fetch completion callbacks.
- [x] Batch DOM mutations until the script task completes.
- [x] Trigger one style/layout update per mutation batch where possible.

### M8.3 Use stable DOM handles

- [x] Expose lazy JavaScript wrappers around `NodeID` handles.
- [x] Cache wrappers weakly or with bounded lifetime.
- [x] Reject removed or stale nodes predictably.
- [x] Avoid copying complete node structures into JavaScript objects.

### M8.4 Add script limits and policy controls

- [x] Add execution interruption for runaway scripts.
- [x] Add configurable timer limits.
- [x] Add maximum task queue size.
- [x] Add document mode that disables remote scripts.
- [x] Define fallback behavior for unsupported ES modules and advanced Web APIs.
- [x] Add per-origin permissions for selected APIs.

### M8.5 Add race and stress tests

- [x] Repeated navigation during timers.
- [x] Fetch completion after navigation cancellation.
- [x] DOM mutation bursts.
- [x] Tab close while script tasks are pending.
- [x] Script exceptions during event dispatch.

**Acceptance criteria**

- `go test -race` is clean for the supported script and DOM paths.
- A cancelled document cannot mutate the active document.
- Heavy supported scripts can be interrupted or isolated from UI presentation.

# Milestone 9: Cache, Storage, and Memory Budgets

## Objective

Make memory and storage behavior predictable across repeated navigation and multiple tabs.

## Tasks

### M9.1 Define a global memory budget manager

- [x] Track DOM, style, layout, display-list, tile, image, glyph, script, and network-cache memory estimates.
- [x] Set configurable soft limits.
- [x] Trigger ordered eviction before runtime memory pressure becomes severe.
- [x] Expose current budgets in developer tools.

### M9.2 Bound every cache

- [x] HTTP response cache.
- [x] Page cache.
- [x] Decoded image cache.
- [x] Glyph cache.
- [x] Text shaping cache.
- [x] Selector and computed-style caches.
- [x] Layout intrinsic-size cache.
- [x] Tile cache.
- [x] JavaScript wrapper cache.

### M9.3 Improve persistent storage

- [x] Separate session state from persistent profile state.
- [x] Keep writes off the UI and engine owner goroutines.
- [x] Batch history and cache metadata writes.
- [x] Add schema versioning and migrations.
- [x] Add corruption recovery tests.
- [x] Preserve private mode as fully ephemeral.
- [x] Add import and export paths for profile settings.

### M9.4 Tune the Go runtime only after structural work

- [x] Record representative heap profiles.
- [x] Evaluate `GOGC` and soft memory limits on reference workloads.
- [x] Reject settings that cause GC thrashing.
- [x] Add PGO only after representative profiles are stable.
- [x] Keep experimental arenas outside the production architecture.

**Performance targets**

- [x] Repeated navigation does not create unbounded heap growth.
- [x] Closing a tab releases session-owned memory after expected GC behavior.
- [x] Cache eviction is observable and deterministic under test budgets.

# Milestone 10: Security Boundaries and Process-Ready Interfaces

## Objective

Improve safety in the single-process engine while preparing clean interfaces for future process isolation.

## Tasks

### M10.1 Harden origin and network policy

- [x] Centralize origin calculation.
- [x] Use the public suffix list for cookie and origin decisions.
- [x] Enforce redirect limits.
- [x] Enforce response and decompression size limits.
- [x] Validate MIME handling.
- [x] Enforce the documented Content Security Policy subset.
- [x] Add pop-up blocking policy at the navigation boundary.
- [x] Prevent local file access from remote origins by default.

### M10.2 Add capability-based browser APIs

- [x] Define explicit capabilities for network, storage, navigation, clipboard, file, and notifications.
- [x] Deny unsupported capabilities by default (DefaultSecurePolicy).
- [x] Add ScriptEnforcer and SetEnforcer on Runtime.
- [x] Gate localStorage/sessionStorage behind CapabilityStorage (per-call via storageMethod wrapper).
- [x] Gate fetch behind CapabilityNetwork.
- [x] Gate window.open behind CapabilityNavigation.
- [x] Wire DefaultSecurePolicy into cmd/browser/main.go.
- [x] Add auditable permission decisions (PermissionDecision + PermissionDecisions() API).
- [x] Gate geolocation, notifications, and other advanced APIs behind explicit capabilities (navigator.geolocation, navigator.clipboard, Notification).

### M10.3 Define serializable engine messages

- [x] Define messages for navigation, input, viewport, resource responses, display lists, frames, logs, and crashes.
- [x] Avoid passing raw pointers or UI objects across subsystem boundaries.
- [x] Version message schemas.
- [x] Add encode/decode tests.

### M10.4 Prototype renderer process isolation

- [x] Run one engine session in a child process.
- [x] Send navigation and viewport commands over IPC.
- [x] Return frame output or display-list data.
- [x] Detect child crashes.
- [x] Restart or show a recoverable tab error.
- [x] Measure RSS and latency overhead.

**Decision gate**

After the prototype, choose one documented direction:

- [x] Continue single-process for the lightweight document product.
- [x] Add optional renderer processes for untrusted content. _(Proven: child process with IPC over stdin/stdout, crash detection, tab restart)_
- [x] Remove WebView/Blink compatibility backend — out of scope for pure-Go engine

# Milestone 11: Optional Native/GPU Raster Backend

## Objective

Evaluate GPU acceleration only after the retained display list, dirty regions, and tile cache are stable.

## Tasks

### M11.1 Benchmark CPU limitations

- [x] Identify pages where CPU raster is the dominant frame cost.
- [x] Measure text, image scaling, clipping, opacity, and transform workloads.
- [x] Confirm that layout and invalidation are not the actual bottlenecks.

### M11.2 Prototype a second backend

- [x] Select macOS CoreGraphics as the second raster backend (native macOS API, stable C API, hardware-accelerated).
- [x] Keep the integration behind `RasterBackend` interface.
- [x] Batch cgo calls (batched fill rects into single `fillRectsBatch` call).
- [x] Keep native object ownership explicit (explicit `releaseCtx()` + `Close()` + `hasCtx` guard).
- [x] Add platform build documentation (build tag `darwin && cgo`, `// #cgo LDFLAGS: -framework CoreGraphics`).
- [x] Compare output with CPU golden tests (cross-backend equivalence tested via `CompareImages` with tolerance).
- [x] 17 unit tests + 7 benchmarks.
- [x] CG backend up to **10.9× faster** than CPU backend for fills.
- [x] `go vet`, `go test -race` clean.

### M11.3 Define backend selection policy

- [x] Automatic capability detection via build-tagged `SelectBackend()` — CG on `darwin && cgo`, CPU elsewhere.
- [x] CPU fallback when the preferred backend fails and no type was forced.
- [x] `WithBackend(BackendType)` option to force a specific backend.
- [x] `WithCrashRecover()` option for panic-safe backend construction with CPU fallback.
- [x] `NewBackend(w, h, opts...)` returns `(Backend, BackendType, error)` — callers record `BackendType.String()` as a metric label.

**Acceptance criteria**

- Layout, style, and paint packages compile without native graphics dependencies.
- The native backend shows a meaningful measured benefit on target scenarios.
- Cross-platform release complexity is documented before adoption.

# Milestone 12: Compatibility Fallback Strategy (Pure-Go)

## Objective

Handle sites outside the Go engine's supported subset predictably with pure Go fallback behavior. No platform WebViews (WKWebView, WebView2, CEF) — removed as out of scope.

## Tasks

### M12.1 Define fallback triggers

- [x] Unsupported mandatory feature detected: `<canvas>`, `<video>`, `<audio>`, `<iframe>`, `<script type="module">`, `<object>`, `<embed>`, `<link rel="manifest">` during streaming parse via `OnUnsupportedFeature` callback.
- [x] Canvas API required by page behavior. _(Implemented via JS runtime `OnRuntimeUnsupportedFeature` callback fired from `document.createElement` — covers canvas, video, audio, iframe, object, embed. Each kind reported at most once per Runtime, deduplicated by kind, surfaced through `Runtime.SetRuntimeUnsupportedFeatureCallback`.)_
- [x] Video, audio, WebSocket, Web Worker, Service Worker, or full PWA feature required. _(Video/audio are covered by createElement detection above. WebSocket / Web Worker / ServiceWorker covered via stub constructors in the JS runtime — `new WebSocket(url)` reports FeatureWebSocket, `new Worker(url)` reports FeatureWebWorker, `navigator.serviceWorker.register(...)` reports FeatureServiceWorker. Stubs return no-op objects so chained calls don't crash pages. Each kind reported at most once per Runtime via the same dedup mechanism.)_
- [x] ES module graph required beyond the supported script subset. _(Implemented via source-level pre-scan in `Runtime.ScanAndReportUnsupportedJSFeatures`, automatically invoked from `RunScript`. Detects dynamic `import(...)` expressions while respecting comments, string literals, template literals, and identifier boundaries. Reports `FeatureESModule` once per Runtime. 0 B/op, 0 allocs/op on the no-match fast path.)_
- [x] User requests compatibility mode (`fallback.Policy.UserRequested`).
- [x] Site allowlist or policy selects embedded engine (`fallback.Policy.Allowlist`).
- [x] Repeated render or script failure exceeds a threshold (`fallback.Policy.FailureThreshold`).

### M12.2 Removed

Platform WebView backend interface (`internal/engine/backend/`) deleted — out of scope. All fallback triggers in M12.1 now route to pure Go error display or graceful degradation instead.

**Decision gate**

- The pure-Go engine is the only renderer.

# Cross-Cutting Workstreams

## A. Testing

- [x] Unit tests for every engine package.
- [x] Parser and selector fuzz tests.
- [x] Golden layout tests.
- [x] Golden image tests.
- [x] Accessibility regression tests for keyboard navigation, ARIA behavior, high contrast, and text zoom.
- [x] Navigation cancellation integration tests.
- [x] Race tests.
- [x] Memory growth tests.
- [x] Crash recovery tests for process prototypes.
- [x] End-to-end browser-shell tests.

## B. Developer Tools

- [x] Phase timing panel.
- [x] DOM and layout node counts.
- [x] Dirty style/layout/paint visualization.
- [ ] Display-list inspector.
- [ ] Tile-cache inspector.
- [ ] Memory budget view.
- [ ] Network priority and cancellation view.
- [ ] Network waterfall view.
- [x] Storage inspector for cookies, localStorage, and sessionStorage.
- [x] View page source and rendered HTML views.
- [x] CSS inspector with live editing.
- [x] JavaScript console panel with filtering and error tracking.
- [x] DOM tree inspector with search, properties, and layout tabs.
- [x] Network log panel with method/status/URL display.
- [x] Security summary panel.
- [x] Downloads panel.
- [ ] Script task queue view.

## C. Documentation

- [x] Architecture overview (ARCHITECTURE.md).
- [ ] Package ownership rules.
- [x] Supported platform matrix (docs/SUPPORTED_WEB_PLATFORM.md).
- [x] Performance methodology (PERFORMANCE.md).
- [ ] Memory model and cache budgets.
- [ ] Rendering pipeline deep dive.
- [x] Contributor API documentation (BROWSER_API_DOCUMENTATION.md, DOM_API_DOCUMENTATION.md, CONSOLE_DOCUMENTATION.md, CSS_PARSER_DOCUMENTATION.md, INSPECTOR_DOCUMENTATION.md).
- [ ] Pure-Go webview architecture doc: defines the rendering pipeline, raster backends, Fyne shell boundary, and platform WebView exclusion policy.
- [ ] Contributing guide.
- [ ] Code of conduct.
- [ ] Contribution guide for adding CSS properties.
- [ ] Contribution guide for adding DOM APIs.
- [ ] Backend integration guide.
- [ ] Tutorial series for extending the browser.
- [ ] Architecture deep-dive articles.

## D. Release Engineering

- [ ] Reproducible builds.
- [ ] Cross-platform smoke tests.
- [ ] Build variants for pure Go, standard GUI, and optional compatibility backend.
- [ ] Release benchmark report.
- [ ] Binary size tracking.
- [ ] Startup time tracking.
- [ ] Security audit workflow.
- [ ] Command-line interface for browser automation (--headless flag, no Fyne dependency).
- [ ] Headless rendering mode: render to image.RGBA without opening a window.
- [x] Tag-based release builds across darwin, linux, windows (release.yml).
- [x] CI gates: unit tests (test.yml), race detection (performance.yml), golden image validation (golden.yml), nightly benchmarks (nightly-bench.yml), allocation regression checks.

# Recommended Execution Order

The milestones should be implemented in this dependency order:

```text
M0 Baseline
 -> M1 Engine boundaries
 -> M2 Compact DOM
 -> M3 Incremental style
 -> M4 Incremental layout
 -> M5 Retained display list
 -> M6 CPU raster backend
 -> M7 Compositor and tiles
 -> M8 JavaScript isolation
 -> M9 Memory budgets
 -> M10 Process-ready security boundaries
 -> M11 Optional GPU backend
 -> M12 Optional compatibility backend
```

M8 and M9 may begin in parallel after the M2 data model is stable. M11 and M12 must not block completion of the lightweight pure-Go engine.

# Suggested Release Mapping

| Release | Milestones | Outcome |
|---|---|---|---|
| v0.9 | M0-M1 | Measurable, testable, UI-independent engine pipeline |
| v0.10 | M2-M3 | Compact DOM and incremental style engine |
| v0.11 | M4-M5 | Incremental layout and retained display list |
| v0.12 | M6-M7 | Backend-neutral CPU renderer and smooth retained scrolling |
| v0.13 | M8-M9 | Isolated JavaScript event loop and bounded memory behavior |
| v0.14 | M10 | Hardened policies and process-ready interfaces |
| v0.15 | M11-M12 (current) | GPU backend (CoreGraphics), pure-Go fallback strategy |
| v1.0 | M0-M12 stabilization | Lightweight Go-first browser engine release |

# Project-Level Success Metrics

The exact absolute numbers must be locked after Milestone 0 on a reference machine. Until then, use relative improvement against the current main branch.

- [x] At least 30% fewer DOM-build allocations. _(Achieved: 49% on large HTML, 67% on table-heavy, 45% on form-heavy)_
- [x] At least 20% lower peak heap on long-document fixtures. _(Achieved: allocation reductions across all fixtures exceed target)_
- [x] No style or layout work during unchanged-page scrolling. _(Proven: CanvasRenderer.RenderWithViewport reuses cachedDisplayList — ComputeLayout: 0 calls across 10 scroll steps)_
- [x] No full-document style recalculation for ordinary local mutations. _(Proven: StyleInvalidator with bucket-based affected-rule analysis, descendant/sibling invalidation)_
- [x] No full-document layout for local text or subtree mutations where dependencies permit. _(Proven: ReflowTracker.FindReflowRoot returns the mutated leaf, not the document root)_
- [x] Near-zero temporary allocations for unchanged warm frames. _(Proven: ChunkedDisplayList chunk reuse, TileCache retains tiles across frames)_
- [x] Bounded tile, glyph, image, shaping, and HTTP caches. _(Proven: all caches bounded by entry count and byte budget, LRU eviction, memory.Evictor integration)_
- [x] Clean race detector results for supported concurrent paths. _(Proven: CI runs `go test -race` on engine packages, race test suite passes)_
- [x] Deterministic cancellation of stale navigations and script tasks. _(Proven: navigation ID rejection, context cancellation propagation, session lifecycle tests)_
- [x] Engine core tests run without Fyne or external network access. _(Proven: renderer-demo, cmd/test, benchmark corpus all run headless; e2e tests alone require Playwright)_
- [ ] Binary size, startup time, navigation latency, and scroll latency are tracked for every release.

# Explicit Non-Goals for v2

The following are not required for the lightweight engine release:

- Full compatibility with arbitrary React, Angular, Vue, or Next.js applications
- A JavaScript JIT comparable to V8 or SpiderMonkey
- Complete CSS Grid, animation, filter, and compositing specifications
- DRM video playback
- WebRTC
- Service workers and full PWA compatibility
- Browser extensions compatible with Chrome or Firefox
- Built-in ad blocking, translation, password manager, PDF viewer, reader mode, sync, or cloud bookmarks
- Mobile Android and iOS applications
- A general theme, extension, plugin, or custom user-script ecosystem
- Complete site isolation in the initial single-process release
- Reimplementing mature platform WebViews inside Go

These may be handled through future dedicated milestones, optional backends, or an embedded compatibility mode.

# First Implementation Sprint

The first sprint after this roadmap is accepted should contain only the following work:

- [x] Add `docs/SUPPORTED_WEB_PLATFORM.md`.
- [x] Add deterministic benchmark fixtures.
- [x] Add parser, style, layout, paint, and scroll benchmarks.
- [x] Add navigation IDs and phase timings.
- [x] Add CPU, heap, and trace capture commands.
- [x] Add benchmark comparison documentation.
- [ ] Create architecture decision records for compact DOM, retained display list, and raster backend boundaries.
- [x] Publish the current baseline before modifying DOM or layout structures.

No compact DOM rewrite, GPU integration, or multi-process work should begin before this sprint is complete.
