# Performance Optimizations

This document describes the performance optimizations implemented in the renderer to improve scroll performance and FPS on long pages.

## Overview

The browser uses a multi-layered rendering architecture with several optimizations to ensure smooth scrolling and high frame rates, even on pages with thousands of elements.

## Key Optimizations

### 1. Display List Caching

Instead of traversing the entire DOM tree on every render, we build and cache a **display list** of paint commands:

```go
// Display list is built once
displayList := displayListBuilder.Build(layoutTree, renderTree)

// Cached for subsequent renders
renderer.cachedDisplayList = displayList
```

**Benefits:**
- Eliminates repeated tree traversal
- O(1) access to paint commands
- Reduces memory allocations

**Performance Impact:**
- ~10x faster than tree traversal for medium-sized trees

### 2. Viewport-Based Culling

Only elements visible in the current viewport (plus a buffer zone) are rendered:

```go
// Check if element is in viewport
func (cr *CanvasRenderer) isInViewport(box Rect) bool {
    bufferZone := cr.viewportHeight * 0.5
    viewportTop := cr.viewportY - bufferZone
    viewportBottom := cr.viewportY + cr.viewportHeight + bufferZone
    
    boxBottom := box.Y + box.Height
    return boxBottom >= viewportTop && box.Y <= viewportBottom
}
```

**Benefits:**
- Renders only ~150% of viewport height (viewport + buffer zones)
- Dramatically reduces widget creation for long pages
- Smooth scrolling with minimal janking

**Performance Impact:**
- For a page with 1000 elements and viewport showing 10%:
  - Without culling: Renders 1000 elements
  - With culling: Renders ~150 elements (85% reduction)

### 3. Incremental Layout Engine

The invalidation tracking system ensures only changed subtrees are re-laid out:

```go
ile := NewIncrementalLayoutEngine(width, height)
ile.InvalidateNode(changedNode, DirtyLayout)
newLayout := ile.ComputeIncrementalLayout(renderTree, oldLayout)
```

**Benefits:**
- Avoids full page relayout on small changes
- Tracks dirty flags (DirtyLayout, DirtyPaint, DirtyStyle)
- Propagates changes efficiently up and down the tree

**Performance Impact:**
- For small DOM changes: ~100x faster than full relayout
- For scrolling (no DOM changes): Near-zero layout cost

### 4. Optimized Scroll Updates

Scroll events trigger viewport updates without rebuilding the display list:

```go
// On scroll event
renderer.SetViewport(newScrollY, viewportHeight)
newContent := renderer.UpdateViewport()
```

**Benefits:**
- Reuses cached display list
- Only filters commands by viewport
- No layout recalculation needed

**Performance Impact:**
- Scroll update: ~350 ns/op (65x faster than full pipeline)

## Benchmark Results

Performance measurements on AMD EPYC 7763 64-Core Processor:

### Full Pipeline (Traditional Approach)

| Tree Size | Time/op | Memory/op | Allocs/op |
|-----------|---------|-----------|-----------|
| 10 nodes  | 15.0 μs | 18.9 KB   | 99        |
| 100 nodes | 23.1 μs | 28.2 KB   | 146       |
| 1000 nodes| 23.1 μs | 28.2 KB   | 146       |

### Viewport Rendering (Optimized)

| Tree Size | Time/op | Memory/op | Allocs/op |
|-----------|---------|-----------|-----------|
| 10 nodes  | 413 ns  | 896 B     | 7         |
| 100 nodes | 744 ns  | 1.7 KB    | 11        |
| 1000 nodes| 746 ns  | 1.7 KB    | 11        |
| 5000 nodes| 746 ns  | 1.7 KB    | 11        |

### Viewport Scroll Updates

| Tree Size | Time/op | Memory/op | Allocs/op |
|-----------|---------|-----------|-----------|
| 10 nodes  | 179 ns  | 372 B     | 3         |
| 100 nodes | 355 ns  | 788 B     | 5         |
| 1000 nodes| 350 ns  | 788 B     | 5         |

### Mutation Baselines

These benchmarks establish the performance footprint of mutating the tree structure before flat DOM store implementation in Milestone 2. Measured on the `form_heavy` document:

| Mutation Scenario | Time/op | Memory/op | Allocs/op |
|-------------------|---------|-----------|-----------|
| Class Toggle      | 342 μs  | 47.5 KB   | 650       |
| Append Node       | 332 μs  | 47.6 KB   | 651       |
| Replace Text      | 328 μs  | 47.5 KB   | 650       |
| Resize Viewport   | 139 μs  | 48.0 KB   | 641       |

### Performance Improvements

- **Viewport Rendering**: 30x faster than full pipeline (746 ns vs 23 μs)
- **Scroll Updates**: 65x faster than full pipeline (350 ns vs 23 μs)
- **Memory**: 94% reduction (1.7 KB vs 28.2 KB for 1000 nodes)
- **Allocations**: 92% reduction (11 vs 146 allocations)

## Scalability

The optimizations scale extremely well:

### Time Complexity
- **Without optimization**: O(n) where n = total elements
- **With viewport culling**: O(v) where v = visible elements (~constant)
- **With display list cache**: O(1) for scroll updates

### Real-World Example

A long blog post with 5000 elements:
- **Traditional rendering**: 23 μs per frame → 43,000 FPS theoretical max
- **Optimized rendering**: 746 ns per frame → 1,340,000 FPS theoretical max
- **Scroll updates**: 350 ns per update → 2,850,000 FPS theoretical max

Even at 60 FPS, the optimized renderer uses:
- **Rendering**: 0.0045% CPU time per frame
- **Scrolling**: 0.0021% CPU time per frame

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────┐
│                    HTML Document                         │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
            ┌──────────────────────┐
            │   Parse & Build      │
            │   Render Tree        │  [One-time: ~23 μs]
            └──────────┬───────────┘
                       │
                       ▼
            ┌──────────────────────┐
            │   Compute Layout     │  [One-time: ~20 μs]
            │   Tree               │
            └──────────┬───────────┘
                       │
                       ▼
            ┌──────────────────────┐
            │   Build Display      │  [One-time: ~2 μs]
            │   List (Cached)      │
            └──────────┬───────────┘
                       │
                       ▼
            ┌──────────────────────┐
            │   Viewport Culling   │  [Per scroll: ~350 ns]
            │   + Render           │
            └──────────────────────┘
```

## Best Practices

### For Developers

1. **Use viewport rendering for all content**:
   ```go
   canvasObject, err := renderer.RenderHTML(htmlContent)
   ```

2. **Update viewport on scroll events**:
   ```go
   renderer.SetViewport(scrollY, viewportHeight)
   renderer.UpdateViewport()
   ```

3. **Clear cache when content changes**:
   ```go
   renderer.canvasRenderer.ClearCache()
   ```

### For Future Enhancements

1. **Texture Caching**: Cache rendered subtrees as textures for even faster repaints
2. **GPU Acceleration**: Translate display list to GPU operations
3. **Virtual Scrolling**: Use infinite scroll with dynamic content loading
4. **Layer Compositing**: Separate static and dynamic content into layers

## Monitoring Performance

### Run Benchmarks

```bash
# CSS parser benchmarks (all sizes and selector types)
go test -bench=. -benchmem ./internal/css/

# DOM parser benchmarks (text extraction, markdown, queries, selectors)
go test -bench=. -benchmem ./internal/dom/

# Deterministic engine corpus benchmarks (article, documentation, table, form, image, JavaScript-light, scrolling pages)
go test -bench=. -benchmem ./internal/engine/testpages/

# Renderer benchmarks (layout, display list, viewport, scroll)
go test -bench=. -benchmem ./internal/renderer/

# Viewport-specific benchmarks
go test ./internal/renderer -bench=Viewport -benchmem

# Scroll-specific benchmarks
go test ./internal/renderer -bench=Scroll -benchmem

# Mutation-specific benchmarks
go test ./internal/renderer -bench=Mutation -benchmem
```

### Pull Request Benchmark Gate

The `Performance` GitHub Actions workflow runs bounded PR microbenchmarks when
benchmark-sensitive engine paths change:

```bash
go test -run=^$ -bench=BenchmarkParse -benchmem -benchtime=100ms -timeout=10m ./internal/dom
go test -run=^$ -bench=BenchmarkParseSelector -benchmem -benchtime=100ms -timeout=10m ./internal/css
go test -run=^$ -bench=BenchmarkLayout -benchmem -benchtime=100ms -timeout=10m ./internal/renderer
go test -run=^$ -bench=BenchmarkDisplayList -benchmem -benchtime=100ms -timeout=10m ./internal/renderer
```

This gate verifies the parser, selector, layout, and display-list benchmark
suites stay runnable on relevant pull requests. Timing regression thresholds
and artifact storage are tracked as separate M0.5 tasks.

### Response Body Reader Benchmarks

The `internal/net` package includes benchmarks for the context-aware limited reader:

```bash
go test -bench=BenchmarkLimitedContextReader -benchmem ./internal/net/
```

Results (12KB body on Apple Virtual CPU):
- Normal read (no limit, live context): ~5.8 μs, 13 allocs
- With active limit: ~5.7 μs, 13 allocs
- Cancelled context (immediate): ~0.2 μs, 3 allocs

The overhead of the context and size limit check is negligible compared
to the underlying I/O cost.

### Response Metadata Capture Benchmarks

The `ResponseMeta` type preserves immutable HTTP response metadata for security
and developer tools without retaining the live `http.Response`:

```bash
go test -bench=BenchmarkResponseMeta -benchmem ./internal/net/
go test -bench=BenchmarkFetchWithMeta -benchmem ./internal/net/
```

Results (VirtualApple @ 2.50GHz):
- `ResponseMetaFromResponse`: ~1.4 μs, 432 B/op, 4 allocs/op
- `FetchWithMeta` (full fetch + metadata): ~3.1 μs, 5.2 KB/op, 31 allocs/op

Metadata capture adds negligible overhead compared to the network I/O cost.

### Streaming Response Body Benchmarks (M1.3)

The streaming path (`FetchStream`) returns the response body as an `io.ReadCloser`
without buffering into an intermediate `bytes.Buffer`, compared to the buffered
path (`FetchWithMeta`) which reads the entire body into a string:

```bash
go test -bench=BenchmarkFetchStreamVsBuffered -benchmem ./internal/net/
```

Results (VirtualApple @ 2.50GHz, ~6.6KB HTML body):

| Path | Time/op | Memory/op | Allocs/op |
|------|---------|-----------|----------|
| FetchStream | 6.4 μs | 18.1 KB | 29 |
| FetchWithMeta | 8.7 μs | 24.6 KB | 34 |

The streaming path is **26% faster**, uses **26% less memory**, and makes **15% fewer
allocations** by eliminating the `bytes.Buffer` intermediary. The DOM parser's
`ParseDocument(io.Reader)` accepts the stream directly for tokenizer consumption.

### Profiling

```bash
# CPU profiling
go test ./internal/renderer -bench=ViewportScroll -cpuprofile=cpu.prof
go tool pprof cpu.prof

# Memory profiling
go test ./internal/renderer -bench=ViewportScroll -memprofile=mem.prof
go tool pprof mem.prof
```

### Concurrency Bounding (Rate Limiter)

The navigation scheduler enforces application-level concurrency limits via
`RateLimiter`:

- **Per-origin limit**: 6 concurrent requests per host
- **Global limit**: 24 concurrent requests across all origins
- **Priority-aware admission**: higher-priority resources (document, blocking
  CSS) are admitted before lower-priority ones (speculative, deferred images)
  when slots are contended
- **Backward compatible**: zero-value `SchedulerOptions` means unlimited

These application-level limits complement the transport-level
`MaxConnsPerHost: 6` with priority-based scheduling.

**Benchmark results** (`internal/engine/navigation`):

| Scenario | Time/op | Allocs/op |
|----------|---------|----------|
| Uncontended acquire/release | ~123 ns | 1 |
| Contended (32 goroutines) | ~4.2 µs | 4 |

No regression observed in existing scheduler benchmarks.

```bash
# Rate limiter benchmarks
go test -bench=BenchmarkAcquireRelease -benchmem ./internal/engine/navigation/
```

## DOM Representation Baseline (M2.1)

Measurements of the current `*html.Node` DOM tree before compact store migration.

### Struct Sizes (64-bit)

| Type | Size | Pointer fields |
|------|------|----------------|
| `html.Node` | 104 B | 8 pointer fields (61.5% pointer density) |
| `html.Attribute` | 48 B | 4 string headers |

### Corpus Node Counts and Estimated Heap

| Page | HTML (B) | Nodes | Attrs | Est Heap (KB) |
|------|----------|-------|-------|----------------|
| long_article | 5,064 | 184 | 22 | 19.7 |
| documentation | 1,834 | 136 | 24 | 14.9 |
| table_heavy | 3,998 | 498 | 48 | 52.8 |
| form_heavy | 3,868 | 209 | 101 | 26.0 |
| image_heavy | 3,363 | 96 | 57 | 12.4 |
| javascript_light_todo | 3,287 | 104 | 39 | 12.4 |
| scrolling_short | 950 | 63 | 6 | 6.7 |
| scrolling_long | 11,694 | 333 | 108 | 38.9 |

### GC Pressure (50 parse cycles, scrolling_long)

- Total allocated per cycle: ~59 KB
- GC cycles over 50 parses: ~1
- Avg GC pause: ~69 µs
- Retained heap growth after GC: near zero (GC reclaims fully)
- AllocsPerRun(long_article): ~306 allocs, ~1.66 allocs/node

### APIs depending on `*html.Node`

See `internal/dom/api_inventory.go` for the full inventory and migration plan.

Run the measurement benchmarks:

```bash
go test -v -run="TestMeasure" ./internal/dom/
go test -bench=BenchmarkMeasure -benchmem ./internal/dom/
```

## Atom and String Interning (M2.2)

The `internal/dom/atom` package provides compact uint32 handles for interned
strings, forming the foundation for the compact DOM store (M2.3) and CSS
pipeline (M3.1).

### Static Atoms

112 HTML tag names and 48 attribute names are pre-assigned as static `Atom`
constants. Lookup is O(1) with zero allocations:

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| LookupStaticHit (10 tags) | 124 | 0 | 0 |
| LookupStaticMiss | 9.2 | 0 | 0 |
| StaticAtomString | 0.3 | 0 | 0 |

### Dynamic Table

The bounded LRU-evicted `Table` supports configurable entry count and byte
limits. Hot-path operations achieve zero allocations:

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| TableInternHit | 37.6 | 0 | 0 |
| TableInternStatic | 16.6 | 0 | 0 |
| TableLookupHit | 32.1 | 0 | 0 |
| TableLookupMiss | 21.9 | 0 | 0 |
| TableLookupStatic | 13.3 | 0 | 0 |
| TableEviction | 5,763 | 80 | 3 |
| RealisticWorkload (100 names) | 9,361 | 0 | 0 |
| CorpusClasses (17 names) | 1,116 | 0 | 0 |
| Concurrent (parallel) | 501 | 7 | 1 |

Run the benchmarks:

```bash
go test -bench=. -benchmem ./internal/dom/atom/
```

## Compact DOM Store (M2.3)

The `internal/dom` package provides a compact DOM store (`Store`) that
replaces pointer-heavy `*html.Node` trees with index-based storage.

### Design

- **NodeID** (uint32): index into contiguous `[]nodeRecord` slice
- **Stale detection**: Kind field == 0 means freed/invalid
- **Node record**: 32 bytes per node (first-child/next-sibling links)
- **Packed attributes**: `[]Attr` slice (8 bytes per attr, indexed by AttrStart/AttrCount)
- **Text storage**: separate `[]byte` buffer for text/comment content
- **Rare metadata**: dedicated map for namespace and other infrequent data
- **Zero-allocation iterators**: children, subtree, reverse children, siblings, ancestors

### Mutation Benchmarks

| Operation | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| AppendChild | 542 | 215 | 0 |
| SetAttrs (3 attrs) | 22 | 0 | 0 |
| SetText | 22 | 0 | 0 |
| RemoveChild | 219 | 0 | 0 |
| RemoveSubtree (101 nodes) | 1,854 | 512 | 1 |

### Traversal Benchmarks

| Traversal | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| ChildIterator (100 children) | 436 | 0 | 0 |
| SubtreeIterator (111 nodes) | 746 | 0 | 0 |

### Memory Overhead

| Node Count | B/op | allocs/op |
|------------|------|----------|
| 10 | 13,200 | 6 |
| 100 | 16,848 | 6 |
| 1,000 | 84,432 | 8 |

Run the benchmarks:

```bash
go test -bench=BenchmarkStore -benchmem ./internal/dom/
```

### Compatibility Adapter (M2.5) — Removed in M5.4

The `NodeAdapter` was removed in M5.4 after all consumers migrated to
NodeID-based APIs. The compact DOM store now provides complete tree access
via `NodeID` handles and zero-allocation iterators, eliminating the need
for the `*html.Node` compatibility layer.

### CSS Pipeline (M3.1)

The `internal/css` package normalizes stylesheet parsing with property
name interning via `atom.Table`, hot/cold property classification, source
order tracking, and origin tagging.

#### Property Interning Benchmarks

Property names are interned into a bounded LRU table (256 entries, 16KB).
All hot properties (~100 common CSS properties) are pre-interned at init.

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| PropertyTableIntern (10 props) | 411 | 0 | 0 |
| PropertyTableLookup (10 props) | 390 | 0 | 0 |

Intern and lookup are zero-allocation after initial population.

#### Parse Overhead from M3.1

The property interning adds minimal overhead to parsing. Allocation counts
remain identical because the atom table reuses existing atoms:

| Benchmark | Before (ns/op) | After (ns/op) | allocs/op (unchanged) |
|-----------|----------------|---------------|----------------------|
| ParseSmall | 3,904 | 4,505 | 166 |
| ParseMedium | 43,893 | 52,056 | 1,909 |
| ParseLarge | 210,776 | 256,181 | 9,067 |
| ParseSelectorComplex | 21,378 | 23,778 | 912 |
| ParseSelectorHeavy | 38,233 | 42,714 | 1,609 |
| ParseAtRules | 35,357 | 39,137 | 1,512 |

The ~15% ns/op increase is from atom table lock acquisition and hash
lookup per property. This is an acceptable trade-off for enabling
M3.2 (selector compilation) and M3.3 (computed style storage) which
will use property atoms for O(1) property matching.

Run the benchmarks:

```bash
go test -bench=. -benchmem ./internal/css/
```

### Compiled Selectors (M3.2)

The CSS package compiles selectors into a flat instruction form with
precomputed specificity and bucketed rules for O(1) candidate lookup.

#### Specificity Computation

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| ComputeSpecificity | 10.7 | 0 | 0 |

Specificity computation is zero-allocation.

#### Stylesheet Compilation

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| CompileStyleSheet (Small) | 960 | 1,464 | 16 |
| CompileStyleSheet (Medium) | 6,663 | 9,816 | 81 |
| CompileStyleSheet (Large) | 30,989 | 40,776 | 314 |
| CompileStyleSheet (SelectorComplex) | 5,560 | 7,440 | 61 |
| CompileStyleSheet (SelectorHeavy) | 11,678 | 23,640 | 100 |

Compilation is a one-time cost per stylesheet. The compiled form
enables faster matching for all subsequent element queries.

#### Element Matching: Bucketed vs Linear

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| MatchElement (ByID) | 459 | 216 | 4 |
| MatchElement (ByClass) | 442 | 216 | 4 |
| MatchElement (ByTag) | 466 | 216 | 4 |
| MatchElement (Compound) | 542 | 240 | 4 |
| MatchElement (Descendant) | 306 | 72 | 2 |
| MatchElement (NoMatch) | 354 | 72 | 2 |
| MatchVsLinear (Bucketed) | 765 | 136 | 6 |
| MatchVsLinear (Linear) | 1,578 | 2,930 | 22 |

Bucketed matching is **2x faster** and uses **95% less memory**
than linear scan on selector-heavy stylesheets. The improvement
scales with rule count: more rules = greater benefit from bucketing.

Run the benchmarks:

```bash
go test -bench=BenchmarkMatchVsLinear -benchmem ./internal/css/
go test -bench=BenchmarkMatchElement -benchmem ./internal/css/
```

### Computed-Style Storage (M3.3)

The `internal/css` package provides typed computed-style storage with
inherited/non-inherited separation, fingerprinting, and bounded
deduplication via `StylePool`.

#### Fingerprint and Equality

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| InheritedStyleFingerprint | 101 | 0 | 0 |
| InheritedStyleEqual | 30 | 0 | 0 |

Fingerprinting uses FNV-1a over all fields. Equality is a direct
struct comparison with zero allocations.

#### StylePool Deduplication

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| StylePoolInternHit | 247 | 0 | 0 |
| StylePoolInternMiss | 279 | 0 | 0 |

Pool operations are zero-allocation. The bounded LRU pool (default
1024 entries) evicts least-recently-used styles when full.

#### Declaration Application

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| ApplyDeclarationsInherited | 92 | 0 | 0 |
| ApplyDeclarationsNonInherited | 106 | 0 | 0 |
| ComputedStyleInherit | 0.3 | 0 | 0 |

Applying declarations to typed structs and inheriting from parent
are zero-allocation operations.

Run the benchmarks:

```bash
go test -bench=BenchmarkInheritedStyle -benchmem ./internal/css/
go test -bench=BenchmarkStylePool -benchmem ./internal/css/
go test -bench=BenchmarkApplyDeclarations -benchmem ./internal/css/
go test -bench=BenchmarkComputedStyle -benchmem ./internal/css/
```

### Style Invalidation (M3.4)

The `internal/css` package provides a `StyleInvalidator` that analyzes
DOM mutations against a compiled stylesheet to determine which elements
need style recalculation.

#### Invalidation Analysis

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| ComputeInvalidation (class change) | 311 | 72 | 4 |
| ComputeInvalidation (inherited) | 237 | 40 | 3 |
| BatchMutations (3 mutations) | 672 | 112 | 8 |
| AffectedRuleIndices | 189 | 56 | 3 |
| HasSiblingCombinator | 1.5 | 0 | 0 |

Invalidation analysis is O(affected rules), not O(total rules). The
bucket-based lookup ensures that only rules referencing the changed
class, ID, or attribute are examined. Sibling combinator detection
is cached and near-zero cost.

Run the benchmarks:

```bash
go test -bench=BenchmarkComputeInvalidation -benchmem ./internal/css/
go test -bench=BenchmarkBatchMutations -benchmem ./internal/css/
go test -bench=BenchmarkAffectedRuleIndices -benchmem ./internal/css/
go test -bench=BenchmarkHasSiblingCombinator -benchmem ./internal/css/
```

### Layout Store (M4.1)

The `internal/renderer` package provides a `LayoutStore` that separates
layout objects from DOM nodes using compact, index-based storage with
stable `LayoutID` handles.

#### Store Operations

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| Allocate (100 objects) | 4800 | 0 | 0 |
| AppendChild (100 children) | 760 | 0 | 0 |
| DOMMapping (100 set+get) | 4799 | 0 | 0 |
| ChildCount (100 children) | 230 | 0 | 0 |

All layout store operations are zero-allocation. The contiguous slice
storage provides cache-friendly access patterns. Tree operations use
first-child/next-sibling links without pointer indirection.

Run the benchmarks:

```bash
go test -bench=BenchmarkLayoutStore -benchmem ./internal/renderer/
```

### Fragment Store (M4.2)

The `internal/renderer` package provides a `FragmentStore` that represents
line fragments, text runs, boxes, and replaced elements in contiguous
storage using stable `FragmentID` handles.

#### Fragment Operations

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| Allocate (100 fragments) | 4800 | 0 | 0 |
| SetGet (100 fragments) | 242 | 0 | 0 |
| Chain (100 fragments) | 354 | 0 | 0 |
| ScratchBufferPool | 27 | 0 | 0 |

All fragment operations are zero-allocation. The contiguous slice storage
provides cache-friendly access patterns. The scratch buffer pool eliminates
per-line allocations during inline layout.

Run the benchmarks:

```bash
go test -bench=BenchmarkFragment -benchmem ./internal/renderer/
go test -bench=BenchmarkScratchBufferPool -benchmem ./internal/renderer/
```

### Text Shaping (M4.3)

The `internal/renderer` package provides a `TextShaper` that offers a
backend-neutral interface for measuring and shaping text with caching.

#### Shaping Operations

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| Shape (uncached) | 67 | 32 | 2 |
| Shape (cached) | 67 | 32 | 2 |
| MeasureWrapped | 1406 | 560 | 32 |

The text shaper provides consistent performance through caching. Shape
operations are O(1) for cached text. Wrapping is O(words) for paragraph
layout.

Run the benchmarks:

```bash
go test -bench=BenchmarkTextShaper -benchmem ./internal/renderer/
go test -bench=BenchmarkFontKeyCacheKey -benchmem ./internal/renderer/
```

### Backend-Neutral Display Commands (M5.1)

The `internal/renderer` package provides compact value-type `DisplayCommand`
structures for the retained display list. Commands are stored by value in
a contiguous `[]DisplayCommand` slice (not `[]*PaintCommand`).

#### Command Creation and List Operations

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| CommandCreate | 0.32 | 0 | 0 |
| ListAdd (100 cmds) | 12,325 | 97,888 | 8 |
| ListAddMixed (100 cmds) | 12,830 | 97,888 | 8 |

Command creation is zero-allocation. List additions only allocate
for the backing slice growth.

#### Serialization

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| SerializeRect | 6,916 | 1,800 | 25 |
| SerializeList (100 cmds) | 1,085,013 | 221,676 | 2,203 |
| DeserializeList (100 cmds) | 2,292,808 | 340,872 | 4,921 |

JSON serialization enables debugging output and future IPC between
engine and presentation processes.

#### Transform Matrix

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| MatrixMul | 0.72 | 0 | 0 |
| MatrixInverse | 19.6 | 24 | 1 |

Matrix operations are near-zero cost. Multiply is zero-allocation.

Run the benchmarks:

```bash
go test -bench=BenchmarkDisplayCommand -benchmem ./internal/renderer/
go test -bench=BenchmarkTransformMatrix -benchmem ./internal/renderer/
```

### Paint Chunks (M5.2)

The `internal/renderer` package provides `PaintChunk` and `ChunkedDisplayList`
that group display commands by stable layout ownership for retained display
list invalidation.

#### Chunk Building

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| BuildPaintChunks (10 cmds) | 453 | 1,512 | 6 |
| BuildPaintChunks (100 cmds) | 3,565 | 12,264 | 9 |
| BuildPaintChunks (1000 cmds) | 37,830 | 155,624 | 13 |
| BuildPaintChunksSingleOwner (1000) | 8,658 | 72 | 2 |

Chunk building is O(n) with a single pass over the command list.
Single-owner documents produce one chunk with minimal allocation.

#### Invalidation and Reuse

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| ChunkedDisplayListInvalidate | 1,391 | 0 | 0 |
| ChunkedDisplayListDirtyRects | 2,031 | 4,080 | 7 |

Invalidation by LayoutID is zero-allocation. Only dirty chunk bounds
are collected for repaint.

#### Source Mapping and Spatial Queries

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| SourceMappingBuild (1000 chunks) | 403,892 | 77,824 | 13 |
| SourceMappingLookup | 376 | 0 | 0 |
| PaintChunkContains | 0.57 | 0 | 0 |
| PaintChunkIntersects | 0.57 | 0 | 0 |

Source mapping build is O(chunks). Lookup is O(chunks) linear scan
with zero allocations. Spatial queries are near-zero cost.

Run the benchmarks:

```bash
go test -bench=BenchmarkBuildPaintChunks -benchmem ./internal/renderer/
go test -bench=BenchmarkChunkedDisplayList -benchmem ./internal/renderer/
go test -bench=BenchmarkSourceMapping -benchmem ./internal/renderer/
go test -bench=BenchmarkPaintChunk -benchmem ./internal/renderer/
```

### Dirty-Region Invalidation (M5.3)

The `internal/renderer` package provides `DirtyRegion` and
`DirtyRegionTracker` for tracking visual bounds and computing
minimal repaint regions.

#### Dirty Region Operations

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| DirtyRegionAdd | 4.1 | 0 | 0 |
| MergeOverlapping (n=10) | 222 | 1,024 | 1 |
| MergeOverlapping (n=100) | 1,351 | 3,072 | 2 |
| MergeOverlapping (n=500) | 5,344 | 3,072 | 2 |
| TotalArea (100 rects) | 127 | 0 | 0 |

Add and area queries are zero-allocation. Merge scales with
rect count but stays bounded by the `maxRects` limit.

#### Tracker and Effects Expansion

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| TrackerInvalidateMove | 193 | 1,056 | 2 |
| ExpandForEffects | 4.0 | 0 | 0 |
| DebugOverlay (100 rects) | 14,514 | 99,704 | 10 |

`ExpandForEffects` is zero-allocation. The tracker move
invalidation allocates only for the new `DirtyRegion` returned
by `Finalize()`. Debug overlay is a developer-tools path.

Run the benchmarks:

```bash
go test -bench=BenchmarkDirtyRegion -benchmem ./internal/renderer/
go test -bench=BenchmarkExpandForEffects -benchmem ./internal/renderer/
go test -bench=BenchmarkDebugDirtyRegionOverlay -benchmem ./internal/renderer/
```

### Streaming Parser (M2.4)

| Fixture | ParseDocument (old) | ParseDocumentCtx (stream) | Alloc Reduction |
|---------|---------------------|---------------------------|------------------|
| Small HTML | 2,440 ns/op, 17 allocs | 7,585 ns/op, 15 allocs | 12% fewer |
| Medium HTML | 10,586 ns/op, 96 allocs | 17,465 ns/op, 55 allocs | 43% fewer |
| Large HTML | 89,328 ns/op, 679 allocs | 119,018 ns/op, 348 allocs | 49% fewer |
| Table heavy | 87,709 ns/op, 733 allocs | 113,788 ns/op, 240 allocs | 67% fewer |
| Form heavy | 57,724 ns/op, 449 allocs | 81,250 ns/op, 247 allocs | 45% fewer |

**Key observations:**
- Allocation count is significantly reduced on complex documents (49–67% on large fixtures), exceeding the 30% target.
- Wall-clock time is ~1.3–1.5× higher due to tokenizer overhead vs. batch `html.Parse`.
- The trade-off favors the streaming path: fewer allocations reduce GC pressure, context cancellation enables responsive navigation, and resource discovery enables parallel fetching.
- Store layer shows zero regression on all benchmarks.

### Golden Image Rendering (M6.5)

The golden image test suite provides deterministic CPU raster benchmarks
for the display command pipeline. All raster operations are zero-allocation.

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| RasterFillRect (800×600, 3 fills) | 1,664,187 | 0 | 0 |
| RasterComposite (800×600, 9 cmds) | 2,228,932 | 0 | 0 |
| RasterClipOpacity (400×300, 6 cmds) | 792,145 | 0 | 0 |

Golden comparison benchmarks:

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| CompareImages 100×100 | 731,800 | 80,004 | 20,000 |
| CompareImages 800×600 | 35,343,540 | 3,840,070 | 960,000 |

All raster operations produce zero allocations. Golden comparison allocates
only for the pixel-diff comparison loop (proportional to image size).

Run the benchmarks:

```bash
go test -bench=BenchmarkGolden -benchmem ./internal/renderer/frame/golden/
```

## Known Limitations

1. **Buffer Zone Trade-off**: Larger buffer zones (50% above/below viewport) use more memory but provide smoother scrolling
2. **Initial Render**: First render still requires full pipeline (~23 μs)
3. **DOM Changes**: Modifying the DOM invalidates the cache and requires rebuilding

## Future Work

- [ ] Implement texture atlas for commonly used elements
- [ ] Add GPU-accelerated rendering path
- [ ] Optimize for mobile with touch gestures
- [ ] Implement lazy loading for off-screen images
- [ ] Add virtual scrolling for infinite lists

## References

- [RENDER_ARCHITECTURE.md](RENDER_ARCHITECTURE.md) - Full rendering architecture
- [Chromium Display Lists](https://chromium.googlesource.com/chromium/src/+/master/cc/paint/display_item_list.h)
- [WebKit Viewport Culling](https://webkit.org/blog/6591/scroll-anchoring/)

---

*Last updated: July 2026*

## Running Benchmarks and Profiling

The repository includes a script to simplify running benchmarks, capturing profiles, and comparing results.

### Using `scripts/bench.sh`

The `bench.sh` script provides the following commands:

*   **`run [package]`**: Runs all `testing.B` benchmarks in the specified package (or `./...` by default) with `-benchmem` enabled to report allocations.
    ```bash
    ./scripts/bench.sh run ./internal/renderer
    ```

*   **`suite`**: Runs the full local performance suite across the entire repository and saves the output to `perf-suite.txt`.
    ```bash
    ./scripts/bench.sh suite
    ```

*   **`profile-cpu <package> [regex]`**: Runs benchmarks matching the regex and captures a CPU profile to `cpu.prof`.
    ```bash
    ./scripts/bench.sh profile-cpu ./internal/renderer ViewportScroll
    # To view: go tool pprof cpu.prof
    ```

*   **`profile-mem <package> [regex]`**: Runs benchmarks matching the regex and captures a memory profile to `mem.prof`.
    ```bash
    ./scripts/bench.sh profile-mem ./internal/renderer ViewportScroll
    # To view: go tool pprof mem.prof
    ```

*   **`trace <package> [regex]`**: Captures a runtime execution trace for scenario benchmarks to `trace.out`.
    ```bash
    ./scripts/bench.sh trace ./internal/engine/testpages BenchmarkGetLongArticle
    # To view: go tool trace trace.out
    ```

*   **`compare <old.txt> <new.txt>`**: Compares two benchmark result files using `benchstat`. This is essential for verifying performance improvements or regressions before merging. (Automatically installs `benchstat` if missing).
    ```bash
    # On main branch
    ./scripts/bench.sh suite
    mv perf-suite.txt main-perf.txt

    # On feature branch
    ./scripts/bench.sh suite
    ./scripts/bench.sh compare main-perf.txt perf-suite.txt
    ```
