# Package Ownership Rules

This document defines the ownership boundaries, responsibilities, and import rules for every package in the Goosie engine. Contributors changing the engine should consult this document to determine which package owns a given concern and where to add new code.

---

## 1. Core Principles

1. **Single owner per concern.** Every major engine capability is owned by exactly one package. Related capabilities that share data structures live in subpackages of the same parent.
2. **No circular imports.** Packages may import only packages with an equal or lower layer number (see Layer map below).
3. **Engine must not import Fyne.** Core engine packages (`internal/dom/*`, `internal/css`, `internal/renderer/frame/*`, `internal/engine/*`, `internal/js`, `internal/net`) must never import `fyne.io/fyne/v2` or any of its subpackages. Fyne is the window/presentation shell only.
4. **Package doc comments** must state the package's responsibility, the milestone it was introduced in, and any ownership constraints (e.g., "single-owner goroutine").
5. **Evictor registration.** Any package that owns a bounded cache with byte-based limits must register a `memory.Evictor` with the global `memory.Manager` (see `memory-model.md`).

---

## 2. Layer Map

Packages are organized in layers. A package may import from its own layer or any lower-numbered layer, but never from a higher-numbered layer.

| Layer | Package | Import restiction | Contains |
|-------|---------|-------------------|----------|
| 0 (foundation) | `internal/dom`, `internal/dom/atom`, `internal/css` | None | DOM store, HTML parser, atom interning, CSS parser, selectors, style |
| 1 (layout) | `internal/renderer` (top-level) | May import layer 0 | Layout tree, fragments, text shaping, display list, paint chunks |
| 2 (raster) | `internal/renderer/frame/*` | May import layers 0-1 | Backend-neutral types, raster backends, caches, compositor |
| 3 (engine) | `internal/engine/*`, `internal/js` | May import layers 0-2 | Session lifecycle, navigation, metrics, IPC, JS runtime |
| 4 (shell) | `internal/ui`, `internal/profile` | May import layers 0-3 | Fyne window, browser chrome, developer tools, profile storage |
| 4 (utilities) | `internal/memory`, `internal/net`, `internal/form`, `internal/image`, `internal/testutil` | May import layer 0 | Memory manager, HTTP networking, form state, image decoding, test helpers |
| 5 (testing) | `internal/test_suite/*`, `test/e2e` | May import any | Cross-cutting test suites |

---

## 3. Package Responsibilities

### `internal/dom` — DOM Store and Parsing (M2)

**Owner:** Compact DOM store and streaming HTML parser.

- Owns the `NodeID` type, `Store`, and all traversal iterators.
- Owns `treebuilder` for streaming HTML tree construction (M2.4).
- Owns the DOM mutation API.
- Owns the compatibility adapter for `*html.Node` (removed in M5.4).
- Must expose only `NodeID`-native APIs externally.

**Subpackages:**
- `internal/dom/atom` — String interning via `atom.Table`. Owns the global atom table, static atoms, and LRU-evicted dynamic atoms.

**Import rules:** Must not import any engine-level or shell-level packages.

### `internal/css` — CSS Pipeline and Style Resolution (M3)

**Owner:** CSS parsing, selector compilation, computed-style storage, and incremental style invalidation.

- Owns CSS tokenizer and parser (compact rule/declaration stores).
- Owns selector compilation into internal instruction form (M3.2).
- Owns `ComputedStyle`, `StylePool`, `InheritedStyle` (M3.3).
- Owns `StyleInvalidator` with bucket-based affected-rule analysis (M3.4).
- Owns `MatchCache` and `StylePool` bounded caches.

**Import rules:** May import `internal/dom` (for `NodeID` and node traversal). Must not import layout, engine, or shell packages.

### `internal/renderer` (top-level) — Layout and Display List (M4-M5)

**Owner:** Layout computation, fragment storage, text measurement, display list construction, and paint chunks.

- Owns `LayoutID`, `LayoutBox`, `LayoutStore` (M4.1).
- Owns `FragmentStore` for inline layout (M4.2).
- Owns text measurement and shaping abstraction (`Measurer` interface, M4.3).
- Owns `ReflowTracker` and incremental layout (M4.4).
- Owns table and form layout (M4.5).
- Owns `DisplayCommand`, `DisplayCommandList`, `ChunkedDisplayList` (M5.1-M5.2).
- Owns `DirtyRegionTracker` (M5.3).

**Import rules:** May import `internal/dom` and `internal/css`. Must not import engine-level or shell packages.

### `internal/renderer/frame` — Backend-Neutral Raster Types (M6)

**Owner:** Platform-neutral frame types and the `RasterBackend` boundary.

- Owns `Color`, `Point`, `Rect`, `ImageHandle`, `FontHandle`, `Glyph`, `TextRun`, `PixelScale`, `Viewport`, `FrameSnapshot`.

**Import rules:** Must not import any backend-specific types (CG types, Fyne types). Pure value types only.

### `internal/renderer/frame/raster` — Raster Backends (M6, M11)

**Owner:** `Backend` interface, CPU backend, CoreGraphics backend, and backend selection factory.

- Owns `Backend` interface (`BeginFrame`, `Rasterize`, `EndFrame`, `Close`).
- Owns `CPUBackend` (pure-Go, M6.2).
- Owns `cgBackend` (macOS CoreGraphics via CGo, M11.2).
- Owns `NewBackend` factory and backend selection policy (M11.3).
- Owns per-backend `DisplayCmd` representation (raster-level subset of `DisplayCommand`).

**Import rules:** CPU backend must import only stdlib `image/*` and `golang.org/x/image/*`. CoreGraphics backend is build-tagged `darwin && cgo` and imports CoreGraphics headers. Must never import Fyne.

### `internal/renderer/frame/cache` — Raster Caches (M6.3, M9.2)

**Owner:** Glyph cache, decoded image cache, and cache hit/eviction metrics.

- Owns `GlyphCache` (entry count + byte budget).
- Owns `ImageCache` (byte budget, `GetOrLoad` with duplicate-decode prevention).
- Both caches register `memory.Evictor` callbacks.

### `internal/renderer/frame/compositor` — Compositor and Tiles (M7)

**Owner:** Tile-based retained rendering, scene snapshots, frame budgets, viewport policy.

- Owns `TileCache` (tile raster cache with version tracking, 32 MB / 1024 tile default).
- Owns `Compositor` (drives frame cycle: BeginFrame → Rasterize → EndFrame).
- Owns scene snapshot and snapshot reader with generation IDs (M7.2).
- Owns `FrameBudgetTracker` with p50/p95/p99 latency tracking (M7.3).
- Owns `ViewportPolicy` for visible-tile-first rendering and prefetch margin (M7.4).

### `internal/renderer/frame/golden` — Golden Image Tests (M6.5)

**Owner:** Golden image testing framework for cross-backend equivalence.

### `internal/renderer/layoutgolden` — Golden Layout Tests

**Owner:** Deterministic layout serialization and golden layout snapshot tests.

### `internal/engine/navigation` — Navigation Scheduler (M1.2)

**Owner:** Navigation IDs, resource priorities, concurrency bounding, origin calculation, and file-access security.

- Owns `ID` type (monotonic navigation ID).
- Owns `Priority` enum and `PriorityFromContext` propagation.
- Owns `RateLimiter` (per-origin and global concurrency limits).
- Owns origin calculation (public suffix list) and same-origin policy.
- Owns file-access security (prevent local file access from remote origins).

**Import rules:** May not import any package outside `internal/engine/*` and `internal/net`. Context-only dependency.

### `internal/engine/session` — Engine Session (M1.1)

**Owner:** Document lifecycle state machine, shared HTTP transport, and event notification.

- Owns `Session` with lifecycle states (created → navigating → parsing → interactive → complete → cancelled → failed → closed).
- Owns shared `http.Transport` with connection limits.
- Owns event queue for engine-to-shell events (navigation state, title, URL, first paint, progress, error).
- Owns cookie jar, TLS config, and per-session network policy.

**Import rules:** May import `internal/engine/navigation`, `internal/engine/metrics`, `internal/dom`, `internal/css`, `internal/renderer/frame/*`, `internal/js`, `internal/net`.

### `internal/engine/metrics` — Engine Metrics (M0.3)

**Owner:** Phase-level instrumentation, timing recorder, and debug logging.

- Owns `Phase` enum (all engine phases from DNS through present).
- Owns `Recorder` (concurrency-safe phase timing, counters, runtime state).
- Owns timing panel data for developer tools.

### `internal/engine/fallback` — Compatibility Fallback (M12.1)

**Owner:** Unsupported-feature detection, fallback policy, and decision routing.

- Owns `Policy` enum (`None`, `UserRequested`, `Allowlist`, `FailureThreshold`).
- Owns unsupported-feature detection triggers (parse-time and runtime).

### `internal/engine/renderer` — Process Isolation Proxy (M10.4)

**Owner:** Child-process renderer isolation, IPC protocol, crash detection, and tab restart.

- Owns `Protocol` (IPC message framing over stdin/stdout).
- Owns `Child` (child process lifecycle).
- Owns `Tab` (tab-side proxy to a child renderer).

### `internal/engine/message` — IPC Messages (M10.3)

**Owner:** Serializable message schema for engine IPC.

- Owns navigation, input, viewport, resource response, display list, frame, log, and crash messages.
- Owns encode/decode and schema versioning.

### `internal/engine/pagecache` — Page Cache (M9.2)

**Owner:** Bounded LRU cache for fully-rendered page snapshots (back/forward navigation).

- Default: 3 entries, 32 MB.
- Registers `memory.Evictor` with manager.

### `internal/engine/testpages` — Test Corpus (M0.2)

**Owner:** Deterministic local HTML/CSS documents for engine benchmarks and scenario tests.

### `internal/js` — JavaScript Runtime (M8)

**Owner:** Goja runtime per session, event loop, DOM handles, script policies, and polyfills.

- Owns per-session Goja runtime with single-owner goroutine (M8.1).
- Owns explicit event loop with task/microtask ordering (M8.2).
- Owns `NodeHandle` (lazy JS wrapper around `dom.NodeID`, M8.3).
- Owns script limits, interruption, timer bounds, and capability gating (M8.4).
- Owns unsupported-JS-feature reporting (dynamic `import()` scan, M12.1).

**Import rules:** May import `internal/dom` (for `NodeID` via `NodeHandle` only). Must not import Fyne or UI packages.

### `internal/net` — HTTP Networking (M1.3)

**Owner:** HTTP fetching (streaming), response caching, MIME validation, CSP enforcement, cookies.

- Owns `Fetcher` / `Service` (async fetch with context cancellation).
- Owns HTTP response cache (write-through disk + in-memory LRU index).
- Owns `ResponseMeta` (immutable response metadata).
- Owns CSP subset enforcement.
- Owns cookie jar and cookie policy.

**Import rules:** May import `internal/dom`. Must not import engine/shell packages.

### `internal/memory` — Memory Budget Manager (M9.1)

**Owner:** Global memory budget tracking, eviction coordination, GC tuning infrastructure.

- Owns `Manager` with per-component limits and ordered eviction.
- Owns `Evictor` callback type (`func(targetBytes uint64) uint64`).
- Owns `TuningConfig`, `EvaluateConfig`, `AutoTune` for GC tuning.
- Does not own any specific cache — caches register `Evictor` callbacks.

### `internal/profile` — Persistent Profiles (M9.3)

**Owner:** Browser profile storage, bookmarks, history, settings, session storage.

- Owns schema versioning and migration.
- Owns corruption recovery.
- Owns import/export for profile settings.
- Owns private mode (fully ephemeral).

**Import rules:** May import `internal/net` (for cookie storage). Should not import engine-core packages.

### `internal/form` — Form State (M4.5)

**Owner:** Form element state, validation, submission prevention, stale event target protection.

### `internal/image` — Image Loading (M6.3)

**Owner:** Image decoding, data URI handling, legacy image cache.

### `internal/ui` — Browser Shell (Fyne)

**Owner:** Fyne-based browser window, address bar, developer tools, panels, theme, shortcuts.

- Owns the browser shell window and tab management.
- Owns developer tools panels: console, DOM inspector, network log, storage, security, downloads, CSS editor.
- Owns Fyne widget tree; engine core never imports Fyne.
- Owns keyboard shortcuts (Cmd+L, Cmd+R, Cmd+T, etc.) and popup blocking UI.

**Import rules:** May import any other `internal/` package. This is the only layer-4 shell package.

### `internal/testutil` — Test Utilities

**Owner:** Shared test helpers, golden fixtures, and test corpus utilities.

### `internal/test_suite/*` — Cross-Cutting Test Suites

Each subpackage owns one dimension of cross-cutting tests:

| Subpackage | Concern |
|---|---|
| `a11y` | Accessibility regression tests (keyboard nav, ARIA, high contrast, text zoom) |
| `e2e` | End-to-end tests (navigation, rendering, JS interaction) |
| `integration` | Integration tests across package boundaries |
| `performance` | Memory growth, allocation pattern, and latency tests |
| `security` | CSP enforcement, origin isolation, capability gating tests |
| `webapi` | Web API compliance tests |

---

## 4. Cross-Cutting Ownership

Some concerns span multiple packages. These have designated coordinators:

| Concern | Coordinator package | Participating packages |
|---|---|---|
| Memory budgets | `internal/memory` | All packages with caches (register `Evictor`) |
| Security boundaries | `internal/engine/navigation` + `internal/js` | `internal/net` (CSP), `internal/engine/session` (policy) |
| Developer tools | `internal/ui` (panels) | `internal/engine/metrics` (timing), `internal/memory` (budget view) |
| Golden tests | `internal/renderer/frame/golden` | `internal/renderer/layoutgolden` |
| IPC / process isolation | `internal/engine/renderer` | `internal/engine/message` (schema) |
| Fallback decisions | `internal/engine/fallback` | `internal/dom` (parse-time), `internal/js` (runtime) |

---

## 5. Adding New Code

When adding new functionality:

1. **Identify the owning package** by matching the concern to the responsibility table above.
2. **Check layer rules.** Your package must not import a package from a higher layer.
3. **Add a package doc comment** stating responsibility and milestone.
4. **If adding a cache**, implement the `Evictor` callback and register it with `memory.Manager`.
5. **If adding a new package**, add it to this document and the layer map.

---

## 6. Ownership Changes

Package ownership should not change without consulting this document. When ownership does change:

1. Update this document.
2. Update package doc comments.
3. Update stale diagrams in `architecture-deep-dives.md`.
4. Ensure import-layer rules are preserved.
