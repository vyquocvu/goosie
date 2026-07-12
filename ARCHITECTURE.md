# Goosie Browser Architecture

## Overview

Goosie's browser architecture uses a modern multi-tree rendering system that separates concerns between DOM parsing, styling, layout computation, and painting. This design enables maintainable, testable, and performant rendering with support for incremental updates.

## Navigation ID Flow

Every page load receives a monotonic navigation ID from `internal/engine/navigation`. The browser shell uses a `Scheduler` to:

1. Cancel the previous load context when a new navigation starts
2. Assign a unique `navigation.ID` to the new load
3. Attach the ID to the load's `context.Context` for downstream phases
4. Reject stale load callbacks when a superseded navigation completes

### Resource Priorities

Each navigation and sub-resource load carries a `Priority` value indicating its
loading urgency. Priorities are ordered from highest (`PriorityDocument`) to
lowest (`PrioritySpeculative`):

- `PriorityDocument` — main document navigation (default for `Begin`)
- `PriorityBlockingCSS` — render-blocking stylesheet
- `PriorityVisibleImage` — image in or near the viewport
- `PriorityScript` — synchronous or async script
- `PriorityDeferredImage` — below-fold or lazy image
- `PrioritySpeculative` — prefetch, prerender, dns-prefetch

The `Scheduler` tracks all pending loads (main navigation + sub-resources) and
returns a priority-sorted snapshot via `PendingLoads()`. Sub-resources are
registered with `AddResource()` and removed with `RemoveResource()`. Each
sub-resource receives its own cancellable context derived from its parent,
and all are cleaned up when the main navigation is cancelled or superseded.

Priority is propagated through `context.Context` and can be retrieved with
`PriorityFromContext()` by downstream network layers for admission control.

### Concurrency Bounding

The `navigation.Scheduler` supports application-level concurrency bounding
via `SchedulerOptions`. A `RateLimiter` enforces per-origin (default 6) and
global (default 24) concurrent request limits at the application level,
complementing the transport-level `MaxConnsPerHost`. When slots are contended,
the limiter uses a `container/heap`-based priority queue so higher-priority
resources (document, blocking CSS) are admitted before lower-priority ones
(speculative, deferred images). A zero-value `SchedulerOptions` means
unlimited, preserving backward compatibility with existing code that uses
`NewScheduler()`.

## Shared HTTP Transport

The `internal/engine/session.Session` owns one configured `http.Transport` that is reused
across all HTTP requests within a browsing context. The transport is created with sensible
connection limits (MaxIdleConns=100, MaxConnsPerHost=6), timeouts (dial 30s, TLS 10s),
and HTTP/2 support. Callers obtain a client sharing the transport via `Session.HTTPClient()`
(which provides a fresh `http.Client` with a cookie jar) or access the raw transport via
`Session.Transport()`.

The transport lifecycle matches the session lifecycle — `Session.Close()` calls
`CloseIdleConnections()` to release pooled connections. In-flight requests complete
normally even after Close. This prevents connection leaks across repeated navigations
and gives the engine explicit control over network resource limits.

### Response Metadata

Every HTTP response captured by `Service.FetchWithMeta` produces an immutable
`ResponseMeta` struct containing status code, headers, content type, content length,
content encoding, protocol version, charset, and cache-hit status. The `Fetcher`
exposes the most recent response metadata via `Fetcher.Meta()`, which is safe for
concurrent reads. Cache hits synthesize metadata from the stored `CacheEntry`.
This preserves response information for security inspection and developer tools
without retaining the live `http.Response` after the body is consumed.

### Streaming Response Bodies (M1.3)

The main document path uses `Service.FetchStream` and `Fetcher.FetchStreamWithContext`
to return the response body as an `io.ReadCloser` without buffering the entire body
into an intermediate `bytes.Buffer`. This eliminates one full-body copy from the
fetch-to-parse pipeline:

```text
User enters URL
  -> Scheduler.Begin(url)
       -> navigation ID assigned
       -> previous context cancelled
  -> FetchStream (ctx carries navigation ID)
       -> returns io.ReadCloser + ResponseMeta
  -> dom.ParseDocument(stream) feeds tokenizer directly from response
  -> UI update only if Scheduler.IsActive(id)
```

The streaming path:
- Returns the body as `io.ReadCloser` — caller must close when done
- Wraps the body with `limitedContextReader` for context cancellation and size limits
- Does not populate the HTTP cache (caching requires the full body)
- Preserves `ResponseMeta` for security and developer tools
- Benchmarks show 26% less memory and 15% fewer allocations vs the buffered path

The DOM parser's `ParseDocument(io.Reader)` method accepts any `io.Reader`, enabling
the HTML tokenizer to consume the response stream directly without an intermediate
string copy.

```
User enters URL
  -> Scheduler.Begin(url)
       -> navigation ID assigned
       -> previous context cancelled
  -> Fetch (ctx carries navigation ID)
  -> Parse / Render
  -> UI update only if Scheduler.IsActive(id)
```

This keeps navigation tracing UI-independent and prepares phase-level metrics in Milestone 0.3.

## Component Flow

```
┌─────────────────────────────────────────────────────────────┐
│                        Main Browser                          │
│                    (cmd/browser/main.go)                     │
└───────────────────────────┬─────────────────────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        │                   │                   │
        ▼                   ▼                   ▼
┌──────────────┐   ┌──────────────┐   ┌──────────────┐
│   HTTP       │   │   HTML       │   │ JavaScript   │
│   Fetcher    │──▶│   Parser     │──▶│   Runtime    │
│ (internal/   │   │ (internal/   │   │ (internal/   │
│    net)      │   │    dom)      │   │    js)       │
└──────────────┘   └──────┬───────┘   └──────────────┘
                          │
                          ▼
                   ┌──────────────┐
                   │   HTML       │
                   │  Renderer    │
                   │ (internal/   │
                   │  renderer)   │
                   └──────┬───────┘
                          │
                          ▼
                   ┌──────────────┐
                   │  GUI Browser │
                   │ (internal/   │
                   │    ui)       │
                   │ [Fyne Window]│
                   └──────┬───────┘
                          │
                          ▼
                   ┌──────────────┐
                   │ Browser State│
                   │  - History   │
                   │  - Bookmarks │
                   └──────────────┘
```

### Atom and String Interning (M2.2)

The `internal/dom/atom` package provides compact uint32 handles (`Atom`) for
interned strings, reducing allocation pressure and pointer density in the
engine's hot paths.

**Static atoms** are pre-assigned constants for all common HTML tag names
(112 tags: `div`, `span`, `p`, `a`, etc.) and attribute names (48 attrs:
`id`, `class`, `href`, `src`, etc.). Static atom lookup is O(1) with zero
allocations.

**Dynamic atoms** are interned into a bounded LRU-evicted `Table` with
configurable entry count and byte limits. Strings exceeding the byte limit
are rejected to prevent unbounded memory growth. The default table supports
1024 entries and 64 KB of string data.

The atom table is safe for concurrent use and is designed as foundation
infrastructure for the compact DOM store (M2.3) and CSS pipeline (M3.1).

### Compact DOM Store (M2.3)

The `internal/dom` package provides a compact DOM store (`Store`) that
replaces pointer-heavy `*html.Node` trees with index-based storage using
stable `NodeID` handles.

**Data model:**

- `NodeID` (uint32) is an index into a contiguous `[]nodeRecord` slice
- Stale handle detection via `Kind` field (Kind == 0 means freed)
- Each node record is 32 bytes with first-child/next-sibling links
- Attributes stored in a packed `[]Attr` slice (8 bytes per attr)
- Text content stored in a separate `[]byte` buffer
- Rare metadata (namespace, etc.) in a dedicated map

**Traversal:**

Zero-allocation iterators for children, subtree (pre-order DFS),
reverse children, siblings, and ancestors.

**Operations:**

- `Allocate`, `Remove`, `Replace` for node lifecycle
- `AppendChild`, `PrependChild`, `InsertBefore`, `RemoveChild` for tree mutation
- `SetAttrs`, `SetText` for content with automatic offset updates
- `SetFlag`, `ClearFlag`, `SetRareData` for metadata

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| AppendChild | 542 | 215 | 0 |
| SetAttrs | 22 | 0 | 0 |
| SetText | 22 | 0 | 0 |
| ChildIterator (100 children) | 436 | 0 | 0 |
| SubtreeIterator (111 nodes) | 746 | 0 | 0 |

The store is additive infrastructure — existing `*html.Node`-based
consumers (parser, renderer, JS runtime) are unaffected until M2.4
(streaming tree construction) and M2.5 (compatibility adapter).

### Compatibility Adapter (M2.5) — Removed in M5.4

The `NodeAdapter` was a temporary migration path that converted compact
`Store` subtrees (rooted at `NodeID`) back to `*html.Node` trees for unmigrated
consumers. It was removed in M5.4 after all consumers migrated to NodeID-based
APIs. The compact DOM store now provides complete tree access via `NodeID`
handles and zero-allocation iterators.

### CSS Pipeline (M3.1)

The `internal/css` package normalizes stylesheet parsing with compact
declaration stores, property name interning, and source order tracking.

**Key features:**
- Property names interned via `atom.Table` (bounded LRU, 256 entries, 16KB)
- Hot/cold property classification: ~100 common properties (display, color,
  font-size, margin, padding, etc.) classified as hot; rare properties
  (vendor prefixes, animation, transition, filter) classified as cold
- Source order tracking: each rule receives a monotonic `SourceOrder` value
- Origin tracking: rules tagged with `OriginUserAgent`, `OriginUser`, or
  `OriginAuthor` for cascade resolution
- Specificity field: `[3]uint16` for (id, class, tag) specificity
- `ParseConfig` with `MaxBytes` and `MaxImportDepth` bounds
- Backward compatible: `Declaration.Property` string field preserved

**Unsupported animations and transitions:** `@keyframes`, `animation-*`,
and `transition-*` properties are parsed and preserved in the declaration
store but are not yet consumed by the style engine. They will fail
predictably (no visual effect) until M3.4 (style invalidation) and
M5 (display list) add animation support.

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| PropertyTableIntern (10 props) | 411 | 0 | 0 |
| PropertyTableLookup (10 props) | 390 | 0 | 0 |

Property interning and lookup are zero-allocation operations after the
initial table population.

### Compiled Selectors (M3.2)

The `internal/css` package provides selector compilation via
`CompileStyleSheet()` which converts a `StyleSheet` into a
`CompiledStyleSheet` with precomputed specificity and bucketed rules
for O(1) candidate lookup.

**Key features:**
- `CompiledSelector`: flat slice-based representation of selector chains
  (replaces linked `SelectorSequence` for matching)
- Precomputed specificity per CSS spec: `[3]uint16` for (id, class, tag)
- Rules bucketed by rightmost key: ID, class, tag, attribute, or universal
- `MatchElement(Element)` uses bucket lookup to avoid scanning all rules
- `Element` interface keeps CSS package independent from renderer

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| ComputeSpecificity | 10.7 | 0 | 0 |
| CompileStyleSheet (Medium) | 6,663 | 9,816 | 81 |
| MatchElement (ByID) | 459 | 216 | 4 |
| MatchElement (ByClass) | 442 | 216 | 4 |
| MatchVsLinear (Bucketed) | 765 | 136 | 6 |
| MatchVsLinear (Linear) | 1,578 | 2,930 | 22 |

Bucketed matching is 2x faster and uses 95% less memory than linear
scan on selector-heavy stylesheets.

### Computed-Style Storage (M3.3)

The `internal/css` package provides typed computed-style storage that
replaces per-element property maps with compact structs separating
inherited from non-inherited CSS properties.

**Key features:**
- `InheritedStyle` — typed struct for CSS-inherited properties (color,
  font-size, font-weight, font-family, line-height, text-align, visibility,
  opacity, etc.)
- `NonInheritedStyle` — typed struct for non-inherited properties (display,
  position, margin, padding, border, background, flexbox, grid, etc.)
- `ComputedStyle` — combines both, accessed via `.Inherited` and
  `.NonInherited` fields
- `Fingerprint()` — uint64 FNV-1a hash for style deduplication
- `StylePool` — bounded LRU cache (default 1024 entries) that deduplicates
  identical `InheritedStyle` groups, returning the same pointer for equal
  styles
- `ApplyDeclarationsToInherited` / `ApplyDeclarationsToNonInherited` —
  populate typed structs from `[]Declaration` slices
- `IsInheritedProperty` — classifies properties per CSS spec
- All operations are zero-allocation after initial construction
- `StylePool` is safe for concurrent use

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| InheritedStyleFingerprint | 101 | 0 | 0 |
| InheritedStyleEqual | 30 | 0 | 0 |
| StylePoolInternHit | 247 | 0 | 0 |
| StylePoolInternMiss | 279 | 0 | 0 |
| ApplyDeclarationsInherited | 92 | 0 | 0 |
| ApplyDeclarationsNonInherited | 106 | 0 | 0 |
| ComputedStyleInherit | 0.3 | 0 | 0 |

All computed-style operations are zero-allocation. The `StylePool`
enables sharing identical inherited style groups across elements,
reducing memory for documents with many elements sharing the same
inherited properties (e.g., repeated `<p>` or `<li>` elements).

This is additive infrastructure — the existing `renderer.Style` type
remains in use until the renderer is migrated to consume `ComputedStyle`.

### Style Invalidation (M3.4)

The `internal/css` package provides a `StyleInvalidator` that determines
which elements need style recalculation after DOM mutations, using the
`CompiledStyleSheet` bucket structure for efficient affected-rule lookup.

**Key features:**
- Mutation classification: class, ID, attribute, inline style, text,
  insertion, removal
- Bucket-based affected rule lookup: class changes check class bucket
  for old and new values; ID changes check ID bucket; attribute changes
  check attr bucket
- Descendant invalidation: when affected rules contain inherited CSS
  properties (color, font-size, visibility, etc.), all descendants are
  flagged for recalculation
- Sibling invalidation: adjacent (+) and general sibling (~) combinators
  trigger invalidation of next/following siblings
- Mutation batching: `BeginBatch()` / `RecordMutation()` / `FlushBatch()`
  coalesce multiple DOM changes into a single combined invalidation result
  with deduplicated targets
- Text changes mark layout dirty but not style (text content doesn't
  affect CSS cascade)

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| ComputeInvalidation (class change) | 311 | 72 | 4 |
| ComputeInvalidation (inherited) | 237 | 40 | 3 |
| BatchMutations (3 mutations) | 672 | 112 | 8 |
| AffectedRuleIndices | 189 | 56 | 3 |
| HasSiblingCombinator | 1.5 | 0 | 0 |

The invalidator is conservative — it may over-invalidate rather than
miss nodes. This is safe: extra invalidation is a performance cost,
not a correctness issue.

### Layout Store (M4.1)

The `internal/renderer` package provides a `LayoutStore` that separates
layout objects from DOM nodes using compact, index-based storage with
stable `LayoutID` handles. This replaces the pointer-heavy `*LayoutBox`
tree with cache-friendly contiguous storage.

**Key features:**
- `LayoutID` (uint32) is a stable handle — an index into a contiguous
  `[]LayoutObject` slice
- `LayoutNone` (0) is the invalid/nil layout handle
- First-child/next-sibling links for tree traversal without pointers
- Bidirectional DOM-to-layout and layout-to-DOM mappings
- Generated content (`::before`, `::after`) creates layout objects
  without corresponding DOM nodes
- `display:none` elements map to `LayoutNone` — no allocation
- Free-list reuse of deleted layout IDs

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| Allocate (100 objects) | 4800 | 0 | 0 |
| AppendChild (100 children) | 760 | 0 | 0 |
| DOMMapping (100 set+get) | 4799 | 0 | 0 |
| ChildCount (100 children) | 230 | 0 | 0 |

The `LayoutStore` is additive infrastructure. The existing
`LayoutBox`/`LayoutEngine` continues to work. The store provides
the foundation for M4.2 (fragment storage) and M4.4 (incremental
layout).

### Fragment Store (M4.2)

The `internal/renderer` package provides a `FragmentStore` that
represents line fragments, text runs, boxes, and replaced elements
in contiguous storage using stable `FragmentID` handles. This replaces
pointer-heavy `[]*LineBox` and `[]*InlineBox` with cache-friendly
storage.

**Key features:**
- `FragmentID` (uint32) is a stable handle — an index into a contiguous
  `[]Fragment` slice
- `FragmentNone` (0) is the invalid/nil fragment handle
- Fragment types: `FragmentLine`, `FragmentTextRun`, `FragmentBox`,
  `FragmentReplaced`
- One layout object can produce multiple fragments (e.g., line breaks)
  via `NextFragment` chains
- Layout objects reference their first fragment via `FirstFragment`
  mapping
- Text runs batch multiple glyphs (not one object per glyph)
- `ScratchBufferPool` reuses buffers for line layout with bounded
  capacity

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| Allocate (100 fragments) | 4800 | 0 | 0 |
| SetGet (100 fragments) | 242 | 0 | 0 |
| Chain (100 fragments) | 354 | 0 | 0 |
| ScratchBufferPool | 27 | 0 | 0 |

All fragment operations are zero-allocation. The contiguous slice
storage provides cache-friendly access patterns. The scratch buffer
pool eliminates per-line allocations.

The `FragmentStore` is additive infrastructure. The existing
`LineBox`/`InlineBox` continues to work. The store provides the
foundation for M4.3 (text measurement) and M4.4 (incremental layout).

### Text Shaping (M4.3)

The `internal/renderer` package provides a `TextShaper` that offers
a backend-neutral interface for measuring and shaping text. It caches
shaped text runs by text, font, size, direction, and relevant features
to avoid redundant computation.

**Key features:**
- `FontKey` uniquely identifies a font configuration (size, weight,
  style, direction, family)
- `ShapedText` contains glyphs with positions and metrics
- Cache keyed by (text, FontKey) to avoid re-shaping identical runs
- Basic Latin support first; advanced shaping via go-text/typesetting
  is optional
- Whitespace-aware text wrapping for line layout
- Direction support (LTR/RTL)

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| Shape (uncached) | 67 | 32 | 2 |
| Shape (cached) | 67 | 32 | 2 |
| MeasureWrapped | 1406 | 560 | 32 |

The text shaper provides consistent performance through caching.
Shape operations are O(1) for cached text. Wrapping is O(words) for
paragraph layout.

The `TextShaper` is additive infrastructure. The existing
`FontMetrics` continues to work. The shaper provides the foundation
for M4.4 (incremental layout).

### Streaming Tree Construction (M2.4)

The parser uses `html.NewTokenizer` to read HTML tokens incrementally and build the DOM tree directly in the compact `Store`. This replaces the intermediate `*html.Node` tree for the new code path.

**Key components:**
- `TreeBuilder` — consumes tokens from `html.Tokenizer`, manages an open-element stack, and writes nodes into `Store`
- `ParseDocumentCtx(ctx, r, cfg)` — context-aware streaming parse entry point on `Parser`
- `Document` — parse result containing `Store` and root `NodeID`
- `Resource` / `OnResource` callback — discovers CSS, scripts, and images during parsing for early scheduling

**Design decisions:**
- Token-by-token processing with `ctx.Done()` checks between tokens for cancellation
- `SetMaxBuf` bounds the tokenizer input buffer (default 1 MB)
- Void elements (br, img, input, etc.) are never pushed onto the open stack
- Malformed HTML tolerance: end tags search the stack from top, popping to match
- Auto-insertion of html/head/body elements to match `html.Parse` behavior
- Append-only `Store.AppendAttrs` and `Store.AppendText` methods for O(1) construction

**Backward compatibility:** The existing `ParseDocument(io.Reader) (*html.Node, error)` is preserved. All downstream consumers (renderer, JS runtime, cmd/browser) continue using `*html.Node` until M2.5 provides a compatibility adapter.

### Backend-Neutral Display Commands (M5.1)

The `internal/renderer` package provides a `DisplayCommand` value type and
`DisplayCommandList` that form the stable contract between layout and
rendering. These are backend-neutral — they contain no references to
`*RenderNode`, `*LayoutBox`, or any UI framework types.

**Key features:**
- `DisplayCommandKind` enum: Rect, Border, Text, Image, PushClip/PopClip,
  PushTransform/PopTransform, PushOpacity/PopOpacity,
  PushStackingContext/PopStackingContext
- Per-command data structs: `RectCommand`, `BorderCommand`, `TextCommand`,
  `ImageCommand`, `ClipCommand`, `TransformCommand`, `OpacityCommand`,
  `StackingContextCommand`
- `TransformMatrix`: 2D affine transform with multiply, inverse, and
  factory functions (translate, scale, rotate)
- `RectF`: float32 axis-aligned rectangle with Contains and Intersects
- `BorderStyle`: enum for none, solid, dashed, dotted
- `DisplayCommandList`: contiguous `[]DisplayCommand` value slice
  (not `[]*DisplayCommand`) — reduces pointer density and GC pressure
- Full JSON serialization for debugging and future IPC
- Zero-allocation command creation (value types, no heap escapes)

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| CommandCreate | 0.32 | 0 | 0 |
| ListAdd (100 cmds) | 12,325 | 97,888 | 8 |
| SerializeRect | 6,916 | 1,800 | 25 |
| TransformMatrixMul | 0.72 | 0 | 0 |
| TransformMatrixInverse | 19.6 | 24 | 1 |

The `DisplayCommand` types are additive infrastructure. The existing
`PaintCommand`/`DisplayList` continues to work. M5.2 builds paint chunks
on top of this command list.

### Paint Chunks (M5.2)

The `internal/renderer` package provides `PaintChunk` and `ChunkedDisplayList`
that group display commands by stable layout ownership. This enables chunk
reuse across frames and paint-dirty invalidation.

**Key features:**
- `PaintChunk`: value type with `LayoutID` owner, command range [Start, End),
  bounds (RectF union), and dirty flag
- `PaintChunkList`: contiguous `[]PaintChunk` slice — zero-allocation access
- `BuildPaintChunks()`: groups a `DisplayCommandList` by consecutive
  `LayoutID` ownership, computing union bounds per chunk
- `ChunkedDisplayList`: combines commands + chunks with invalidation by
  `LayoutID`, dirty rect collection, and chunk reuse
- `SourceMapping`: maps `LayoutID` → command range for developer tools
- Non-contiguous same-owner commands produce separate chunks (preserving
  display order for correct painting)
- Zero-allocation invalidation and dirty-rect queries

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| BuildPaintChunks (10 cmds) | 453 | 1,512 | 6 |
| BuildPaintChunks (100 cmds) | 3,565 | 12,264 | 9 |
| BuildPaintChunks (1000 cmds) | 37,830 | 155,624 | 13 |
| BuildPaintChunksSingleOwner (1000) | 8,658 | 72 | 2 |
| ChunkedDisplayListInvalidate | 1,391 | 0 | 0 |
| SourceMappingLookup | 376 | 0 | 0 |
| PaintChunkContains | 0.57 | 0 | 0 |
| PaintChunkIntersects | 0.57 | 0 | 0 |

Chunk building is O(n) where n = command count. Invalidation and
spatial queries (Contains, Intersects) are zero-allocation. The
`ChunkedDisplayList` enables M5.3 (dirty-region invalidation) by
providing per-chunk dirty tracking and bounds.

The paint chunk infrastructure is additive. The existing
`DisplayList`/`PaintCommand` path is unaffected.

### Dirty-Region Invalidation (M5.3)

The `internal/renderer` package provides a `DirtyRegion` and
`DirtyRegionTracker` that track visual bounds across frames and
compute minimal repaint regions.

**Key features:**
- `DirtyRegion`: bounded list of `RectF` rectangles with automatic
  overlap merging when count exceeds `maxRects` (default 64)
- `DirtyRegionTracker`: per-`LayoutID` bounds tracking across frames;
  `InvalidateMove()` marks both old and new regions dirty
- `MergeOverlapping()`: greedy O(n·k) pairwise merge that stays
  within the bounded rect count
- `ExpandForEffects()`: expands dirty rects to account for shadow
  blur, border width, shadow offset, and antialiasing margins
- `DebugDirtyRegionOverlay()`: generates `DisplayCommandList` with
  semi-transparent rects for developer-tools visualization
- `RectF` utility methods: `Area()`, `IsEmpty()`, `Equal()`,
  `NearlyEqual()`, `RectUnion()`, `RectIntersection()`

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| DirtyRegionAdd | 4.1 | 0 | 0 |
| MergeOverlapping (n=100) | 1,351 | 3,072 | 2 |
| DirtyRegionTotalArea (100 rects) | 127 | 0 | 0 |
| TrackerInvalidateMove | 193 | 1,056 | 2 |
| ExpandForEffects | 4.0 | 0 | 0 |
| DebugOverlay (100 rects) | 14,514 | 99,704 | 10 |

All core dirty-region operations are zero-allocation. The tracker
enables M5.4 by providing the bounds information needed to avoid
DOM traversal during repainting.

The dirty-region infrastructure is additive. The existing
`ChunkedDisplayList.DirtyRects()` path continues to work.

### Platform-Neutral Frame Types (M6.1)

The `internal/renderer/frame` package defines the backend-neutral types
that form the contract between the display list builder and any raster
backend (CPU, GPU, or Fyne adapter). No Fyne or backend-specific types
appear in this package.

**Core types:**
- `Color`: packed uint32 RGBA — avoids `color.Color` interface allocation
- `Point`: float32 2D coordinate with Add/Sub/Scale/DistanceTo
- `Rect`: float32 AABB with Contains/Intersects/Intersection/Union/Expand
- `ImageHandle` / `FontHandle`: typed uint32 handles for backend-managed resources
- `Glyph` / `TextRun`: shaped text with per-glyph advance and offsets
- `PixelScale`: DPI-aware layout→device pixel conversion
- `Viewport`: frame dimensions, scroll offset, and pixel scale
- `FrameSnapshot`: immutable frame with generation counter and content hash

**Design principles:**
- All types are value types (no pointer indirection, no GC pressure)
- Zero allocations on all operations (verified by benchmarks)
- `FrameSnapshot` is immutable once created — safe for concurrent reads
- `PixelScale` clamps invalid input (zero/negative DPI → 96 fallback)
- `Viewport` clamps negative dimensions and scroll offsets to zero

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| ColorCreation | 0.32 | 0 | 0 |
| ColorToStd | 0.32 | 0 | 0 |
| RectContains | 0.32 | 0 | 0 |
| RectIntersects | 0.31 | 0 | 0 |
| PixelScaleToPixels | 0.32 | 0 | 0 |
| FrameSnapshotCreation | 19.1 | 0 | 0 |
| TextRunWidth (50 glyphs) | 17.1 | 0 | 0 |

All frame type operations are zero-allocation. The packed `Color`
avoids the interface allocation of `color.Color` in hot paths.

### CPU Raster Backend (M6.2)

The `internal/renderer/frame/raster` package provides a pure-Go CPU
raster backend that consumes backend-neutral display commands and
produces pixel frame buffers.

**Key features:**
- `Backend` interface: `BeginFrame` → `Rasterize` → `EndFrame` → `Close`
- `CPUBackend`: pure-Go implementation with zero-allocation rasterization
- `FrameBuffer`: reusable pixel buffer — allocated once, cleared per frame
- Solid fill rasterization with alpha blending (source-over compositing)
- Per-side border rasterization (top/right/bottom/left)
- Nested clip stack with intersection-based clipping
- Opacity stack for group transparency
- Dirty-region-only rasterization — only pixels within dirty bounds are written
- HiDPI support via `PixelScale` device-pixel conversion

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| FillRect 100×100 | 42,215 | 0 | 0 |
| FillRect 800×600 | 1,480,107 | 0 | 0 |
| FillWithClip | 137,249 | 0 | 0 |
| BorderAllSides | 15,937 | 0 | 0 |
| DirtyRegionSmall | 42,814 | 0 | 0 |
| BlendPixel | 6.1 | 0 | 0 |

All raster operations are zero-allocation. The frame buffer is reused
across frames via `Reset()` (memset to zero) without reallocation.

### Glyph and Image Caches (M6.3)

The `internal/renderer/frame/cache` package provides bounded LRU caches
for the raster backend with byte-based limits and duplicate-decode
prevention.

**Key features:**
- `GlyphCache`: bounded by entry count, LRU eviction, zero-alloc Get/Put
- `ImageCache`: bounded by byte budget, LRU eviction by memory cost
- `Metrics`: atomic hit/miss/eviction counters with `HitRate()`
- `GetOrLoad()`: prevents duplicate concurrent decode via `sync.Once`
- `Close()`/`Clear()`: releases all resources on session shutdown

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| GlyphCachePut | 52.0 | 0 | 0 |
| GlyphCacheGet | 41.1 | 0 | 0 |
| ImageCacheGet | 54.1 | 0 | 0 |

Glyph cache operations are zero-allocation. Image cache Get is
zero-allocation; Put allocates one LRU entry node.

### Fyne Presentation Adapter (M6.4)

The `FyneAdapter` in `internal/renderer/fyne_adapter.go` bridges the
CPU raster backend output to Fyne's canvas system.

**Key features:**
- Presents `FrameBuffer` (image.RGBA) via a single `canvas.Image`
- Content object is stable — never rebuilt on scroll or frame updates
- `PresentFrame()` updates the image in-place (no widget allocation)
- `SetViewport()` stores scroll state without triggering rebuilds
- Thread-safe: concurrent frame production + UI-thread presentation
- UI-thread constraint: `PresentFrame()` must be called via `fyne.Do()`

The adapter enables the M6 exit gate: the same display list can be
rendered without importing Fyne in engine tests (via `CPUBackend`
directly), while Fyne remains the presentation shell.

### Golden Image Testing (M6.5)

The `internal/renderer/frame/golden` package provides deterministic
render-to-PNG comparison for regression testing.

**Key features:**
- `AssertGolden()`: renders commands via CPUBackend, compares to stored PNG
- Per-channel tolerance (default 1) for rounding differences
- `GOOSIE_UPDATE_GOLDEN=1` env var writes new reference images
- Diff image generation for visual debugging
- `CompareImages()`: pixel-by-pixel comparison with metrics
- Separate update directory for review before acceptance

### Retained Tile System (M7.1)

The `internal/renderer/frame/compositor` package provides tile-based
retained rendering for smooth scrolling. Content is divided into
configurable raster tiles that are reused across frames when unchanged.

**Key features:**
- `Tile`: retained raster tile with coord, bounds, version, and image
- `TileCache`: bounded LRU cache with byte budget and tile count limits
- `TileCacheConfig`: configurable tile size (default 256×256), max bytes (32MB), max tiles (1024)
- `TilePriority`: Visible/Near/Hidden based on viewport and prefetch margin
- `CoordForPoint()`: maps layout-space point to tile coordinate
- `BoundsForCoord()`: returns layout-space bounds for a tile
- `CoordsInRect()`: returns all tile coordinates overlapping a rect (half-open interval)
- `InvalidateRect()`: marks all overlapping tiles as dirty
- Atomic hit/miss/eviction metrics

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| TileCache Get (hit) | 46.7 | 0 | 0 |
| TileCache Get (miss) | 18.6 | 0 | 0 |
| CoordForPoint | 0.32 | 0 | 0 |
| CoordsInRect (4 tiles) | 43.8 | 96 | 1 |
| InvalidateRect (400 tiles) | 4,033 | 0 | 0 |

All tile lookups and invalidation operations are zero-allocation. The
tile cache enables M7.2-M7.4 by providing the retained raster
infrastructure needed for compositor snapshots, smooth scrolling, and
viewport prefetch policies.

The compositor package is independent of Fyne and the CPU backend.
Tiles hold `*image.RGBA` but the cache logic is backend-neutral.

### Compositor Snapshots (M7.2)

The `internal/renderer/frame/compositor` package provides immutable
`SceneSnapshot` types that capture compositor state at a point in
time. Presentation layers read snapshots without locking mutable
layout state.

**Key features:**
- `SceneSnapshot`: immutable struct with generation ID, viewport,
  tile metadata, and content hash
- `SnapshotTile`: immutable view of a single tile (shared image pointer)
- `SnapshotPublisher`: creates snapshots from mutable TileCache with
  atomic generation counter
- `IsStale()` / `RejectStale()`: generation-based stale detection
- `VisibleTiles()`: returns tiles overlapping the snapshot viewport
- `FindTile()`: coordinate-based tile lookup in snapshot
- Content hash from tile versions + scroll position for fast equality

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| Publish (256 tiles) | 6,289 | 16,384 | 1 |
| VisibleTiles (256 tiles) | 3,155 | 16,384 | 1 |
| FindTile (256 tiles) | 105 | 0 | 0 |

Snapshots enable lock-free presentation: the UI thread reads
immutable data while the engine thread mutates the tile cache.
Generation IDs reject stale raster results from cancelled jobs.

### Frame Budget and Input Prioritization (M7.3)

The `internal/renderer/frame/compositor` package provides frame timing
instrumentation and cancellable raster job queues.

**Key features:**
- `FrameBudget`: target frame duration (default 60fps) and threshold
- `FrameBudgetTracker`: bounded ring buffer (512 frames) recording
  per-frame timing, latency, dropped/missed counts
- `Stats()`: computes p50/p95/p99 input-to-present latency percentiles
- `RasterJob`: cancellable rasterization work unit with generation ID
- `RasterJobQueue`: bounded job queue with `CancelCoord()` and
  `CancelOutside()` for cancelling tiles that leave the priority area

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| RecordFrame | 34.0 | 0 | 0 |
| Stats (512 records) | 5,885 | 4,152 | 3 |
| EnqueueDequeue | 28.0 | 0 | 0 |

Frame recording is zero-allocation. Stats computation allocates
only for latency sorting. The bounded ring buffer prevents
unbounded memory growth.

### Viewport and Prefetch Policy (M7.4)

The `internal/renderer/frame/compositor` package provides scroll-
direction-aware tile prioritization and resource prefetch limits.

**Key features:**
- `ViewportPolicy`: tracks scroll direction, computes prefetch rects,
  prioritizes tiles (visible > near > hidden)
- `ScrollDirection`: None/Up/Down/Left/Right estimation from scroll delta
- `PrefetchRect()`: expanded rect in scroll direction with bounded margin
- `PrioritizeTiles()`: sorted tile coords bounded by MaxPrefetchTiles
- `SetHidden()`: reduces prefetch to zero for hidden tabs
- `ResourcePrefetchLimits`: page cache (3 pages), resource prefetch
  (16 resources, 8MB budget)

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| PrioritizeTiles (256 tiles) | 148.6 | 240 | 2 |
| PrefetchRect | 2.9 | 0 | 0 |

Viewport policy enables M7 exit gates: visible tiles are rasterized
first, scroll direction drives prefetch, hidden tabs pause raster
work, and resource limits prevent unbounded memory growth.

### JavaScript Session Ownership (M8.1)

The `internal/js` package provides a `Session` type that wraps the
Goja runtime with single-owner goroutine enforcement, a bounded
task queue, and context-based shutdown.

**Key features:**
- `Session`: wraps Runtime with ownership, task scheduling, shutdown
- One Goja runtime per session (one per document/tab)
- `Run()`: owner loop — blocks calling goroutine, processes tasks
- `Submit(Task)`: enqueue work from any goroutine for owner execution
- Bounded ring buffer task queue (default 256, configurable)
- `Close()`: context cancellation triggers clean shutdown
- `Navigate()`: cancels context, resets runtime for new document
- Atomic metrics: total executed, total dropped

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| Submit | 67.3 | 0 | 0 |
| TaskExecution (drain) | ~0 | 0 | 0 |

Task submission is zero-allocation. The bounded ring buffer prevents
unbounded queue growth. Context cancellation ensures clean shutdown
with no goroutine leaks.

The Session layer is additive — the existing Runtime API is unchanged.

### Explicit Event Loop (M8.2)

The `internal/js` package provides an `EventLoop` type implementing
the HTML event loop processing model with task/microtask ordering,
timer integration, and DOM mutation batching.

**Key features:**
- `EventLoop`: task queue + microtask queue + timer set
- `QueueTask()`: enqueue macrotask (FIFO ring buffer, bounded 256)
- `QueueMicrotask()`: enqueue microtask (FIFO ring buffer, bounded 512)
- `RunOnce()`: execute one macrotask → drain all microtasks → fire
  ready timers → flush DOM mutations
- `SetTimeout()` / `SetInterval()`: bounded timer scheduling (max 128)
- `ClearTimer()`: cancel timer by ID
- `RecordMutation()` / `SetMutationFlush()`: DOM mutation batching
  with one style/layout update per task
- Microtasks enqueued by microtasks are drained in the same cycle
- Atomic metrics: tasks executed, microtasks executed, mutation batches

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| QueueAndRun | 146.2 | 0 | 0 |
| MicrotaskDrain (100 micros) | 140.6 | 0 | 0 |

All event loop operations are zero-allocation. Bounded ring buffers
prevent unbounded queue growth. Timer set is bounded by MaxTimers.

### Stable DOM Handles (M8.3)

The `internal/js` package provides `NodeHandle` — a lazy wrapper
around `dom.NodeID` that resolves through the DOM store on demand
without copying node data.

**Key features:**
- `NodeHandle`: wraps `dom.NodeID` with lazy resolution through store
- `IsValid()`: checks node liveness via store (Kind != 0)
- `Kind()`, `Parent()`, `FirstChild()`, `NextSibling()`: lazy access
  returning predictable errors (`ErrInvalidHandle`, `ErrNodeRemoved`)
- `HandleCache`: bounded LRU cache (default 1024 entries) of handle
  wrappers to avoid repeated allocations for the same node
- `Invalidate()`: removes handle from cache on node removal
- Atomic hit/miss metrics

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| IsValid | 0.52 | 0 | 0 |
| HandleCache Get (hit) | 19.0 | 0 | 0 |
| HandleCache Get (miss) | 249 | 41 | 1 |

Handles are zero-copy — they store only a NodeID and store pointer.
Cache hits are zero-allocation. Stale nodes are rejected predictably
with typed errors.

### Script Limits and Policy Controls (M8.4)

The `internal/js` package provides `ScriptPolicy` and `ScriptEnforcer`
for configurable execution limits and security controls.

**Key features:**
- `ScriptPolicy`: MaxSteps, MaxExecutionTime, MaxTimers,
  MaxTaskQueueSize, DocumentMode, per-origin API permissions
- `ScriptEnforcer`: runtime enforcement with atomic step counter,
  time tracking, timer/task slot acquisition
- `DocumentMode`: Full, InlineOnly, NoScript — controls script loading
- `AllowScript()`: checks if a script source is permitted
- `CheckAPIPermission()`: per-origin API access control with
  default fallback
- Context cancellation on abort for cooperative interruption
- Typed errors: ErrScriptTimeout, ErrScriptStepLimit,
  ErrTimerLimit, ErrTaskQueueLimit, ErrRemoteScriptBlocked,
  ErrAPIPermissionDenied

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| AddSteps | 7.5 | 0 | 0 |
| CheckAPIPermission | 2.1 | 0 | 0 |

All enforcement checks are zero-allocation. Atomic counters for
step tracking and resource acquisition.

## Navigation State Flow

```
User enters URL → Add to History → Fetch Page → Parse HTML → Render
       │               │
       │               ├─→ Update URL bar
       │               ├─→ Update back/forward buttons
       │               └─→ Update bookmark indicator
       │
       ├─→ Back button → GoBack() → Fetch previous URL
       ├─→ Forward button → GoForward() → Fetch next URL
       ├─→ Refresh button → Reload current URL
       └─→ Bookmark button → Toggle bookmark state
```

## Example Execution Flow

### Initial Startup
1. **GUI Browser** (`internal/ui/browser.go`)
   - Creates Fyne window titled "Goosie"
   - Initializes navigation controls (URL bar, buttons)
   - Creates BrowserState for history/bookmarks
   - Displays welcome message
   - Waits for user input

### Navigation Flow (User enters URL)
1. **User Action**
   - User enters "https://example.com" in URL bar
   - Presses Enter or clicks navigation button

2. **State Management** (`internal/ui/state.go`)
   - Adds URL to navigation history
   - Updates current index
   - Enables/disables back/forward buttons appropriately

3. **HTTP Fetcher** (`internal/net/fetcher.go`)
   - Fetches https://example.com
   - Returns HTML content

4. **HTML Parser** (`internal/dom/parser.go`)
   - Parses HTML using x/net/html
   - Extracts HTML structure for rendering
   - Provides getElementById functionality for JS

5. **HTML Renderer** (`internal/renderer/`)
   - Multi-tree architecture: DOM → Render Tree → Layout Tree → Display List
   - Builds render tree from parsed HTML with unique node IDs
   - Computes layout tree with box model calculations
   - Generates display list for efficient painting
   - Supports incremental updates with invalidation tracking
   - Hit testing for interactive elements
   - Renders to Fyne canvas objects
   - Supports headings, paragraphs, lists, links, images

6. **JavaScript Runtime** (`internal/js/runtime.go`)
   - Sets HTML content for DOM operations
   - Runs: `console.log("Page loaded: " + document.title)`
   - Output: "Page loaded: Example Domain"

7. **GUI Browser** (`internal/ui/browser.go`)
   - Updates URL bar with current URL
   - Updates button states (back/forward/bookmark)
   - Displays rendered content in scrollable canvas
   - Shows bookmark indicator if page is bookmarked

## Window Layout (when GUI is available)

```
┌───────────────────────────────────────────────────────┐
│ Goosie                                       [_][□][X]│
├───────────────────────────────────────────────────────┤
│ ← → ⟳ │ https://example.com                     │ ☆   │
├───────────────────────────────────────────────────────┤
│                                                       │
│  # Example Domain                                     │
│                                                       │
│  This domain is for use in illustrative               │
│  examples in documents. You may use this              │
│  domain in literature without prior                   │
│  coordination or asking for permission.               │
│                                                       │
│  [More information...](https://example.org)           │
│                                                       │
│                                                       │
│                                                       │
└───────────────────────────────────────────────────────┘
```

### Navigation Bar Components
- **← (Back)**: Navigate to previous page in history
- **→ (Forward)**: Navigate to next page in history  
- **⟳ (Refresh)**: Reload current page
- **URL Entry**: Enter web addresses, press Enter to navigate
- **☆/★ (Bookmark)**: Toggle bookmark for current page

## Test Coverage

- internal/net: 36.4%
- internal/dom: 95.0%
- internal/renderer: 100% (65+ tests, including benchmarks)
- internal/js: 92.9%

See [README.md](README.md) for usage and testing instructions.
