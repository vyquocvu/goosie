# Architecture Deep-Dive Articles

This document indexes architectural notes for specific Goosie subsystems, focusing on internal design, trade-offs, and performance characteristics.

---

## Article 1: The Compact DOM Store — Why Index-Based Storage Wins

**File:** `adr/0001-use-compact-dom-store.md`

**Covers:**
- Pointer-density problem with `*html.Node`
- 32-byte `nodeRecord` layout and field-level design decisions
- Linked-list child structure vs. child arrays
- Generation-based staleness detection
- Zero-allocation traversal iterators
- Benchmarks: 49-67% allocation reduction across fixtures

---

## Article 2: The Retained Display List — From Layout to Pixel

**File:** `adr/0002-retained-display-list-design.md`

**Covers:**
- Why a flat command list instead of a recording canvas
- Paint chunks as the invalidation unit
- Dirty-region merging algorithm
- Two-level command representation (12 renderer commands → 7 raster commands)
- Scroll optimization: zero layout work on unchanged pages
- Serialization for dev tools and IPC

---

## Article 3: Raster Backend Abstraction

**File:** `adr/0003-raster-backend-boundaries.md`

**Covers:**
- Backend interface lifecycle design
- CPU vs. CoreGraphics: performance comparison (4-10× for fills)
- Build-tag isolation strategy
- Backend selection with crash recovery
- Cross-backend golden equivalence testing

---

## Article 4: Memory Budget Manager and Eviction Cascade

**File:** `memory-model.md`

**Covers:**
- Two-phase eviction (per-component → global)
- Ordered eviction priority (network cache → DOM → script)
- Per-cache bounding strategies (entry count, byte budget, or both)
- LRU eviction implementation patterns
- GC tuning philosophy (soft limits over `GOMEMLIMIT`)
- Session memory lifecycle verification tests

---

## Article 5: Incremental Layout and Invalidation

**File:** `internal/renderer/incremental_layout.go`

**Covers:**
- Layout dirty reason classification (geometry, intrinsic size, text, children, viewport, font, style)
- Reflow root finding — smallest valid subtree
- Intrinsic size caching
- Fragment store: contiguous inline layout storage
- Table layout: column measurement caching, row/col span handling
- Performance: local text mutation does not force full-document layout

---

## Article 6: Style Invalidation — From Class Toggle to Repaint

**File:** `internal/css/invalidation.go`

**Covers:**
- Mutation classification (class, ID, attribute, inline, text, insert, remove)
- Bucket-based affected-rule analysis
- Descendant invalidation for inherited property changes
- Sibling invalidation for `+` and `~` combinators
- Mutation batching with target deduplication
- Performance: 2× faster matching, 95% less memory per match

---

## Article 7: Compositor, Tiles, and Smooth Scrolling

**File:** `internal/renderer/frame/compositor/tiles.go`

**Covers:**
- Tile grid: configurable tile size (256×256 default), version tracking
- Scene snapshots with generation IDs
- Frame budget tracking (p50/p95/p99 input-to-present latency)
- Viewport policy: visible tiles first, prefetch margin in scroll direction
- Hidden tab deprioritization
- Performance: 0 allocs/op tile lookup, 46.7ns/op

---

## Article 8: JavaScript Runtime Isolation

**File:** `internal/js/session.go`

**Covers:**
- Single-owner goroutine pattern with `ErrWrongOwner`
- Bounded task queue with navigation cancellation
- Event loop: task/microtask ordering
- `NodeHandle` — lazy JS wrapper around `NodeID` with staleness detection
- Script interruption and timer limits
- Capability-based API gating (`CapabilityNetwork`, `CapabilityStorage`, `CapabilityNavigation`)

---

## Article 9: Process Isolation Prototype

**Files:** `internal/engine/renderer/` and `internal/engine/message/`

**Covers:**
- IPC protocol over stdin/stdout
- Serializable message schema (navigation, viewport, display list, frame, crash)
- Child process lifecycle: spawn, detect crash, restart tab
- RSS and latency overhead measurements
- Decision gate: single-process for lightweight documents, optional process isolation for untrusted content

---

## Article 10: Streaming Parser and Resource Discovery

**File:** `internal/dom/treebuilder.go`

**Covers:**
- Token-by-token tree construction
- Early resource discovery (CSS, scripts, images) during parse
- Cancellation via context: stop parsing mid-token on navigation
- Unsupported feature detection (`<canvas>`, `<video>`, `<iframe>`) during parse for fallback decisions
- Body size limits and malformed HTML handling
