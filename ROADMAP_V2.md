# Goosie Engine Roadmap v2

> Status: Proposed  
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

### 2.5 Fyne is the shell, not the engine contract

The engine must not expose Fyne canvas objects as its retained rendering representation.

Fyne may remain the browser shell and presentation adapter, but the core renderer must emit backend-neutral display commands.

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

- [ ] Public interfaces are documented.
- [ ] Unit tests cover normal, malformed, empty, and cancellation cases.
- [ ] Benchmarks include `ns/op`, `B/op`, and `allocs/op`.
- [ ] Relevant paths pass `go test -race`.
- [ ] CPU and heap profiles have been reviewed.
- [ ] No unbounded cache, queue, goroutine, timer, or retained document state exists.
- [ ] Engine work does not directly depend on Fyne types unless it is inside the platform adapter.
- [ ] Golden rendering tests are updated intentionally.
- [ ] Performance results are recorded in the PR description.
- [ ] Architecture documentation is updated when ownership or data flow changes.

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
- [ ] Add JavaScript-light interactive pages.
- [ ] Add mutation scenarios: class toggle, append node, replace text, resize viewport.
- [x] Add scrolling scenarios for short and long documents.

**Acceptance criteria**

- Benchmarks do not require external network access.
- Results are reproducible within an agreed variance on the reference runner.

### M0.3 Add phase-level metrics

- [ ] Assign a navigation ID to every load.
- [ ] Record DNS, connection, first byte, body read, parse, style, layout, paint, raster, and present durations.
- [ ] Record node, rule, selector, box, fragment, display item, tile, and image counts.
- [ ] Record bytes downloaded, decoded image bytes, cache hits, and cache misses.
- [ ] Record live goroutine count and heap size after navigation.
- [ ] Add structured logs behind a debug flag.

**Acceptance criteria**

- A single navigation can be traced from URL entry to first presented frame.
- Metrics can be exported without importing UI packages.

### M0.4 Add benchmark and profiling commands

- [ ] Add package-level `testing.B` benchmarks.
- [ ] Add `-benchmem` documentation.
- [ ] Add optional CPU and heap profile output.
- [ ] Add runtime trace capture for scenario benchmarks.
- [ ] Add a command that runs the full local performance suite.
- [ ] Add a benchmark comparison script using `benchstat`.

**Acceptance criteria**

- A contributor can compare a branch against `main` with one documented workflow.

### M0.5 Add performance CI gates

- [ ] Run parser, selector, layout, and display-list microbenchmarks on relevant PRs.
- [ ] Run longer navigation and scrolling scenarios nightly.
- [ ] Store benchmark artifacts.
- [ ] Fail CI on significant allocation regressions.
- [ ] Warn on timing regressions above the accepted variance.
- [ ] Run `go test -race` for concurrent engine packages.

**Exit gate**

- Baseline results are committed or attached to a tracking issue.
- No core data structure migration begins before this milestone is complete.

# Milestone 1: Engine Boundaries and Navigation Pipeline

## Objective

Decouple the engine from the browser UI and make navigation, cancellation, and resource ownership explicit.

## Tasks

### M1.1 Introduce `EngineSession`

- [ ] Define a session that owns one active document, style state, layout state, script runtime, and rendering state.
- [ ] Define lifecycle states: created, navigating, parsing, interactive, complete, cancelled, failed, closed.
- [ ] Add explicit `Close()` behavior.
- [ ] Ensure all timers, requests, decoders, and script tasks are cancelled on close.
- [ ] Add lifecycle tests for repeated navigation and tab closure.

### M1.2 Introduce a navigation scheduler

- [ ] Create one `context.Context` per navigation.
- [ ] Cancel the previous navigation immediately when a new navigation starts.
- [ ] Reject stale callbacks by navigation ID.
- [ ] Add resource priorities: document, blocking stylesheet, visible image, script, deferred image, speculative resource.
- [ ] Bound concurrent resource requests per origin and globally.
- [ ] Reuse one configured `http.Transport`.

### M1.3 Stream response bodies

- [ ] Remove unnecessary full-body copies from the main document path.
- [ ] Feed the HTML tokenizer directly from the response stream.
- [ ] Apply body size limits.
- [ ] Support cancellation while reading.
- [ ] Preserve response metadata for security and developer tools.

### M1.4 Formalize engine-to-shell events

- [ ] Define events for navigation state, title, URL, first paint, progress, error, security summary, and download.
- [ ] Ensure events contain immutable values.
- [ ] Add a bounded event queue.
- [ ] Prevent slow UI consumers from blocking the engine.

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

- [ ] Record bytes and allocations per node.
- [ ] Record parse time for each corpus page.
- [ ] Record GC behavior for repeated navigation.
- [ ] Identify APIs that depend directly on `html.Node` pointers.

### M2.2 Implement atom and string interning

- [ ] Intern tag names.
- [ ] Intern attribute names.
- [ ] Intern common class names and IDs where beneficial.
- [ ] Keep document text in a compact text store.
- [ ] Avoid interning unbounded arbitrary large strings.
- [ ] Add memory limits and eviction rules where global interning is used.

### M2.3 Implement the compact DOM store

- [ ] Store nodes in contiguous slices.
- [ ] Use stable `NodeID` handles.
- [ ] Store attributes in a packed attribute slice.
- [ ] Store rare node metadata separately.
- [ ] Implement traversal without allocating child slices.
- [ ] Implement insertion, removal, and replacement operations.
- [ ] Add generation checks or equivalent protection against stale handles.

### M2.4 Add streaming tree construction

- [ ] Use the low-level `x/net/html` tokenizer path where practical.
- [ ] Apply `SetMaxBuf` or equivalent input bounds.
- [ ] Discover stylesheets, images, and scripts during parsing.
- [ ] Schedule discovered resources without waiting for full DOM completion.
- [ ] Preserve parser correctness for malformed HTML in the supported subset.

### M2.5 Add a compatibility adapter

- [ ] Provide a temporary adapter for code that still expects the old DOM representation.
- [ ] Mark the adapter as migration-only.
- [ ] Add metrics to detect remaining use.
- [ ] Remove it before Milestone 5 exit.

**Performance targets**

- [ ] Reduce DOM build allocations by at least 30% from the locked baseline.
- [ ] Reduce peak heap for long-document fixtures by at least 20%.
- [ ] Do not regress parser correctness fixtures.

# Milestone 3: CSS Pipeline and Incremental Style Resolution

## Objective

Make CSS parsing streaming-friendly, selector matching indexed, and style invalidation local.

## Tasks

### M3.1 Normalize stylesheet parsing

- [ ] Parse CSS into compact rule and declaration stores.
- [ ] Intern property names and common values.
- [ ] Store common properties in typed fields.
- [ ] Store rare properties in a secondary structure.
- [ ] Preserve source order, origin, specificity, and `!important`.
- [ ] Bound imported stylesheet depth and total bytes.

### M3.2 Compile selectors

- [ ] Convert selectors into a compact internal instruction form.
- [ ] Precompute specificity.
- [ ] Bucket rules by rightmost ID, class, tag, attribute, or universal selector.
- [ ] Avoid scanning every rule for every element.
- [ ] Add dedicated selector microbenchmarks.

### M3.3 Introduce computed-style storage

- [ ] Define a typed `ComputedStyle` for hot layout and paint properties.
- [ ] Separate inherited and non-inherited values.
- [ ] Deduplicate identical inherited style groups.
- [ ] Add fingerprints for reusable computed styles.
- [ ] Avoid a property map per element in hot paths.

### M3.4 Implement style invalidation

- [ ] Add dirty flags at node and subtree level.
- [ ] Invalidate descendants only when inherited values change.
- [ ] Invalidate siblings only for selectors that require it.
- [ ] Invalidate ancestors only for supported relational behavior.
- [ ] Batch DOM mutations before style recalculation.
- [ ] Add tests for class, ID, attribute, inline style, insertion, removal, and text changes.

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

- [ ] Define stable `LayoutID` values.
- [ ] Build layout objects only for rendered nodes.
- [ ] Handle `display: none` without layout allocation.
- [ ] Allow generated content to create layout objects without DOM nodes.
- [ ] Track DOM-to-layout and layout-to-DOM relationships by IDs.

### M4.2 Implement fragment storage

- [ ] Represent line fragments, text runs, boxes, and replaced elements in contiguous storage.
- [ ] Support one layout object producing multiple fragments.
- [ ] Reuse scratch buffers during line layout.
- [ ] Avoid allocating one object per glyph.

### M4.3 Add text measurement and shaping abstraction

- [ ] Define a backend-neutral font and text measurement interface.
- [ ] Cache font resolution.
- [ ] Cache shaped text runs by text, font, size, direction, and relevant features.
- [ ] Support basic Latin first.
- [ ] Add an optional advanced shaping path through `go-text/typesetting`.
- [ ] Add tests for wrapping, whitespace modes, long words, and mixed styles.

### M4.4 Implement incremental layout

- [ ] Add layout dirty reasons: geometry, intrinsic size, text, children, viewport, font, and style.
- [ ] Find the smallest valid reflow root.
- [ ] Cache intrinsic sizes.
- [ ] Preserve unaffected layout fragments.
- [ ] Rebuild only affected display-list chunks after reflow.

### M4.5 Harden table and form layout

- [ ] Define the supported table algorithm subset.
- [ ] Cache column measurements.
- [ ] Handle `thead`, `tbody`, `tfoot`, row spans, and column spans within documented limits.
- [ ] Align native form control sizing with CSS boxes.
- [ ] Prevent duplicate submission and stale event targets.

**Performance targets**

- [ ] A local text mutation must not force full-document layout in standard fixtures.
- [ ] A viewport scroll without geometry changes must not run layout.
- [ ] Repeated layout of an unchanged document must allocate near zero temporary heap after warm-up.

# Milestone 5: Retained Display List and Paint Invalidation

## Objective

Make the display list the stable contract between layout and rendering.

## Tasks

### M5.1 Define backend-neutral display commands

- [ ] Add commands for rectangles, borders, text runs, images, clips, transforms, opacity, and stacking contexts.
- [ ] Use compact typed structures.
- [ ] Avoid interface values in the hottest display-list storage where possible.
- [ ] Add serialization support for debugging and future IPC.

### M5.2 Introduce paint chunks

- [ ] Group display commands by stable layout ownership.
- [ ] Track chunk bounds.
- [ ] Reuse unchanged chunks.
- [ ] Rebuild chunks only for paint-dirty layout objects.
- [ ] Keep source-to-display mappings for developer tools.

### M5.3 Implement dirty-region invalidation

- [ ] Track previous and new visual bounds.
- [ ] Invalidate both regions when an object moves.
- [ ] Merge overlapping dirty rectangles with bounded complexity.
- [ ] Expand dirty regions for shadows, borders, and antialiasing.
- [ ] Add debug visualization for invalidated regions.

### M5.4 Remove renderer dependence on DOM traversal

- [ ] The raster path must consume display commands only.
- [ ] Scrolling must not traverse the DOM.
- [ ] Repainting must not recompute style or layout unless explicitly dirty.
- [ ] Remove the temporary DOM compatibility adapter introduced in M2.5.

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

- [ ] Define color, point, rectangle, transform, clip, image handle, font handle, and text run types.
- [ ] Define pixel scale and viewport behavior.
- [ ] Define immutable frame snapshots.

### M6.2 Implement a pure-Go CPU raster backend

- [ ] Support solid fills and borders.
- [ ] Support clipped images.
- [ ] Support shaped text runs.
- [ ] Support opacity and basic transforms.
- [ ] Raster only dirty tiles.
- [ ] Reuse image buffers and raster scratch memory.

### M6.3 Add glyph and image caches

- [ ] Add bounded glyph cache.
- [ ] Add decoded image cache with byte-based limits.
- [ ] Add cache hit and eviction metrics.
- [ ] Release resources when sessions close.
- [ ] Prevent duplicate concurrent decode of the same resource.

### M6.4 Implement the Fyne presentation adapter

- [ ] Present completed frame buffers through Fyne.
- [ ] Keep Fyne object creation out of per-display-item loops.
- [ ] Avoid rebuilding the entire widget tree for scroll updates.
- [ ] Document UI-thread constraints.

### M6.5 Add golden image testing

- [ ] Render deterministic fixtures at fixed viewport sizes.
- [ ] Compare output with tolerance rules.
- [ ] Store intentional updates separately from test execution.
- [ ] Run on a controlled CI platform.

**Exit gate**

- The same display list can be rendered without importing Fyne in engine tests.
- Raster memory remains bounded during a 60-second scroll scenario.

# Milestone 7: Compositor, Tile Cache, and Smooth Scrolling

## Objective

Make scrolling and simple visual updates independent from full repaint work.

## Tasks

### M7.1 Add retained tiles

- [ ] Divide content into configurable raster tiles.
- [ ] Track tile content versions.
- [ ] Reuse valid tiles across frames.
- [ ] Prioritize visible and near-visible tiles.
- [ ] Evict by byte budget and recency.

### M7.2 Add compositor snapshots

- [ ] Publish immutable scene snapshots.
- [ ] Allow presentation to read snapshots without locking mutable layout state.
- [ ] Use generation IDs to reject stale raster results.
- [ ] Separate scroll offset from document geometry where possible.

### M7.3 Prioritize input and viewport work

- [ ] Process scroll input before low-priority raster work.
- [ ] Cancel raster jobs for tiles that leave the priority area.
- [ ] Add frame budget instrumentation.
- [ ] Record p50, p95, and p99 input-to-present latency.
- [ ] Record dropped and missed frames.

### M7.4 Add viewport and prefetch policy

- [ ] Render visible tiles first.
- [ ] Prefetch a bounded margin in the scroll direction.
- [ ] Deprioritize hidden tab raster work.
- [ ] Pause animations and timers in hidden tabs according to policy.

**Performance targets**

- [ ] Scrolling an unchanged long page performs no style or layout work.
- [ ] Most scroll frames reuse existing tiles after warm-up.
- [ ] p95 input-to-present latency is tracked and does not regress beyond the agreed CI threshold.

# Milestone 8: JavaScript Runtime and DOM Mutation Isolation

## Objective

Keep Goja useful for lightweight interaction without allowing scripts to corrupt ownership rules or permanently block the UI.

## Tasks

### M8.1 One runtime, one owner goroutine

- [ ] Create one Goja runtime per engine session or document.
- [ ] Ensure only one goroutine calls a runtime directly.
- [ ] Add a bounded task queue.
- [ ] Add shutdown and navigation cancellation behavior.

### M8.2 Implement an explicit event loop

- [ ] Define task and microtask ordering for the supported subset.
- [ ] Integrate timers.
- [ ] Integrate fetch completion callbacks.
- [ ] Batch DOM mutations until the script task completes.
- [ ] Trigger one style/layout update per mutation batch where possible.

### M8.3 Use stable DOM handles

- [ ] Expose lazy JavaScript wrappers around `NodeID` handles.
- [ ] Cache wrappers weakly or with bounded lifetime.
- [ ] Reject removed or stale nodes predictably.
- [ ] Avoid copying complete node structures into JavaScript objects.

### M8.4 Add script limits and policy controls

- [ ] Add execution interruption for runaway scripts.
- [ ] Add configurable timer limits.
- [ ] Add maximum task queue size.
- [ ] Add document mode that disables remote scripts.
- [ ] Add per-origin permissions for selected APIs.

### M8.5 Add race and stress tests

- [ ] Repeated navigation during timers.
- [ ] Fetch completion after navigation cancellation.
- [ ] DOM mutation bursts.
- [ ] Tab close while script tasks are pending.
- [ ] Script exceptions during event dispatch.

**Acceptance criteria**

- `go test -race` is clean for the supported script and DOM paths.
- A cancelled document cannot mutate the active document.
- Heavy supported scripts can be interrupted or isolated from UI presentation.

# Milestone 9: Cache, Storage, and Memory Budgets

## Objective

Make memory and storage behavior predictable across repeated navigation and multiple tabs.

## Tasks

### M9.1 Define a global memory budget manager

- [ ] Track DOM, style, layout, display-list, tile, image, glyph, script, and network-cache memory estimates.
- [ ] Set configurable soft limits.
- [ ] Trigger ordered eviction before runtime memory pressure becomes severe.
- [ ] Expose current budgets in developer tools.

### M9.2 Bound every cache

- [ ] HTTP response cache.
- [ ] Decoded image cache.
- [ ] Glyph cache.
- [ ] Text shaping cache.
- [ ] Selector and computed-style caches.
- [ ] Layout intrinsic-size cache.
- [ ] Tile cache.
- [ ] JavaScript wrapper cache.

### M9.3 Improve persistent storage

- [ ] Separate session state from persistent profile state.
- [ ] Keep writes off the UI and engine owner goroutines.
- [ ] Batch history and cache metadata writes.
- [ ] Add schema versioning and migrations.
- [ ] Add corruption recovery tests.
- [ ] Preserve private mode as fully ephemeral.

### M9.4 Tune the Go runtime only after structural work

- [ ] Record representative heap profiles.
- [ ] Evaluate `GOGC` and soft memory limits on reference workloads.
- [ ] Reject settings that cause GC thrashing.
- [ ] Add PGO only after representative profiles are stable.
- [ ] Keep experimental arenas outside the production architecture.

**Performance targets**

- [ ] Repeated navigation does not create unbounded heap growth.
- [ ] Closing a tab releases session-owned memory after expected GC behavior.
- [ ] Cache eviction is observable and deterministic under test budgets.

# Milestone 10: Security Boundaries and Process-Ready Interfaces

## Objective

Improve safety in the single-process engine while preparing clean interfaces for future process isolation.

## Tasks

### M10.1 Harden origin and network policy

- [ ] Centralize origin calculation.
- [ ] Use the public suffix list for cookie and origin decisions.
- [ ] Enforce redirect limits.
- [ ] Enforce response and decompression size limits.
- [ ] Validate MIME handling.
- [ ] Prevent local file access from remote origins by default.

### M10.2 Add capability-based browser APIs

- [ ] Define explicit capabilities for network, storage, navigation, clipboard, file, and notifications.
- [ ] Deny unsupported capabilities by default.
- [ ] Add per-session policy.
- [ ] Add auditable permission decisions.

### M10.3 Define serializable engine messages

- [ ] Define messages for navigation, input, viewport, resource responses, display lists, frames, logs, and crashes.
- [ ] Avoid passing raw pointers or UI objects across subsystem boundaries.
- [ ] Version message schemas.
- [ ] Add encode/decode tests.

### M10.4 Prototype renderer process isolation

- [ ] Run one engine session in a child process.
- [ ] Send navigation and viewport commands over IPC.
- [ ] Return frame output or display-list data.
- [ ] Detect child crashes.
- [ ] Restart or show a recoverable tab error.
- [ ] Measure RSS and latency overhead.

**Decision gate**

After the prototype, choose one documented direction:

- Continue single-process for the lightweight document product.
- Add optional renderer processes for untrusted content.
- Add a WebView/Blink compatibility backend for unsupported sites.

# Milestone 11: Optional Native/GPU Raster Backend

## Objective

Evaluate GPU acceleration only after the retained display list, dirty regions, and tile cache are stable.

## Tasks

### M11.1 Benchmark CPU limitations

- [ ] Identify pages where CPU raster is the dominant frame cost.
- [ ] Measure text, image scaling, clipping, opacity, and transform workloads.
- [ ] Confirm that layout and invalidation are not the actual bottlenecks.

### M11.2 Prototype a second backend

- [ ] Select Skia, Graphite, or another maintained backend.
- [ ] Keep the integration behind `RasterBackend`.
- [ ] Batch cgo calls.
- [ ] Keep native object ownership explicit.
- [ ] Add platform build documentation.
- [ ] Compare output with CPU golden tests.

### M11.3 Define backend selection policy

- [ ] Automatic capability detection.
- [ ] CPU fallback.
- [ ] Debug flag to force a backend.
- [ ] Crash-safe fallback where possible.
- [ ] Metrics labeled by backend.

**Acceptance criteria**

- Layout, style, and paint packages compile without native graphics dependencies.
- The native backend shows a meaningful measured benefit on target scenarios.
- Cross-platform release complexity is documented before adoption.

# Milestone 12: Compatibility Fallback Strategy

## Objective

Provide a product path for sites outside the Go engine's supported subset without forcing the core engine to implement the entire modern web platform.

## Tasks

### M12.1 Define fallback triggers

- [ ] Unsupported mandatory feature detected.
- [ ] User requests compatibility mode.
- [ ] Site allowlist or policy selects embedded engine.
- [ ] Repeated render or script failure exceeds a threshold.

### M12.2 Define a compatibility backend interface

- [ ] Navigation.
- [ ] Back, forward, reload, and stop.
- [ ] Title and URL updates.
- [ ] Download and permission events.
- [ ] Profile and private-mode behavior.
- [ ] Developer-tools handoff.

### M12.3 Prototype platform WebView integration

- [ ] Windows WebView2.
- [ ] macOS WKWebView.
- [ ] Linux option evaluation.
- [ ] Document feature and behavioral differences.
- [ ] Keep fallback optional at build time where possible.

**Decision gate**

- The pure-Go engine remains the default for supported lightweight content.
- Compatibility mode must not leak platform-specific types into core engine packages.

# Cross-Cutting Workstreams

## A. Testing

- [ ] Unit tests for every engine package.
- [ ] Parser and selector fuzz tests.
- [ ] Golden layout tests.
- [ ] Golden image tests.
- [ ] Navigation cancellation integration tests.
- [ ] Race tests.
- [ ] Memory growth tests.
- [ ] Crash recovery tests for process prototypes.
- [ ] End-to-end browser-shell tests.

## B. Developer Tools

- [ ] Phase timing panel.
- [ ] DOM and layout node counts.
- [ ] Dirty style/layout/paint visualization.
- [ ] Display-list inspector.
- [ ] Tile-cache inspector.
- [ ] Memory budget view.
- [ ] Network priority and cancellation view.
- [ ] Script task queue view.

## C. Documentation

- [ ] Architecture overview.
- [ ] Package ownership rules.
- [ ] Supported platform matrix.
- [ ] Performance methodology.
- [ ] Memory model and cache budgets.
- [ ] Rendering pipeline deep dive.
- [ ] Contribution guide for adding CSS properties.
- [ ] Contribution guide for adding DOM APIs.
- [ ] Backend integration guide.

## D. Release Engineering

- [ ] Reproducible builds.
- [ ] Cross-platform smoke tests.
- [ ] Build variants for pure Go, standard GUI, and optional compatibility backend.
- [ ] Release benchmark report.
- [ ] Binary size tracking.
- [ ] Startup time tracking.

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
|---|---|---|
| v0.9 | M0-M1 | Measurable, testable, UI-independent engine pipeline |
| v0.10 | M2-M3 | Compact DOM and incremental style engine |
| v0.11 | M4-M5 | Incremental layout and retained display list |
| v0.12 | M6-M7 | Backend-neutral CPU renderer and smooth retained scrolling |
| v0.13 | M8-M9 | Isolated JavaScript event loop and bounded memory behavior |
| v0.14 | M10 | Hardened policies and process-ready interfaces |
| v1.0 | M0-M10 stabilization | Lightweight Go-first browser engine release |
| v1.x optional | M11-M12 | GPU and compatibility backends when justified by measurements |

# Project-Level Success Metrics

The exact absolute numbers must be locked after Milestone 0 on a reference machine. Until then, use relative improvement against the current main branch.

- [ ] At least 30% fewer DOM-build allocations.
- [ ] At least 20% lower peak heap on long-document fixtures.
- [ ] No style or layout work during unchanged-page scrolling.
- [ ] No full-document style recalculation for ordinary local mutations.
- [ ] No full-document layout for local text or subtree mutations where dependencies permit.
- [ ] Near-zero temporary allocations for unchanged warm frames.
- [ ] Bounded tile, glyph, image, shaping, and HTTP caches.
- [ ] Clean race detector results for supported concurrent paths.
- [ ] Deterministic cancellation of stale navigations and script tasks.
- [ ] Engine core tests run without Fyne or external network access.
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
- Complete site isolation in the initial single-process release
- Reimplementing mature platform WebViews inside Go

These may be handled through future dedicated milestones, optional backends, or an embedded compatibility mode.

# First Implementation Sprint

The first sprint after this roadmap is accepted should contain only the following work:

- [ ] Add `docs/SUPPORTED_WEB_PLATFORM.md`.
- [x] Add deterministic benchmark fixtures.
- [x] Add parser, style, layout, paint, and scroll benchmarks.
- [ ] Add navigation IDs and phase timings.
- [ ] Add CPU, heap, and trace capture commands.
- [ ] Add benchmark comparison documentation.
- [ ] Create architecture decision records for compact DOM, retained display list, and raster backend boundaries.
- [ ] Publish the current baseline before modifying DOM or layout structures.

No compact DOM rewrite, GPU integration, or multi-process work should begin before this sprint is complete.
