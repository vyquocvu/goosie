# Memory Model and Cache Budgets

This document describes the Goosie engine's memory management architecture: how the memory budget manager coordinates eviction across all caches, how individual caches are bounded, how Go runtime GC parameters are tuned, and how memory behavior is verified.

---

## 1. Design Principles

1. **Predictable memory usage.** Memory must not grow unbounded during repeated navigation, scrolling, or script execution.
2. **Soft limits everywhere.** Every cache has configurable entry-count and/or byte-budget limits. Exceeding a limit triggers ordered eviction, not allocation failure.
3. **Coordinated eviction.** When memory pressure rises, the central `memory.Manager` orchestrates eviction across all caches in priority order — the least critical caches (network cache, page cache) are drained first.
4. **Session-isolated memory.** Each engine session owns its DOM, style, layout, display-list, tile, image, glyph, and script memory. Closing a session releases this memory.
5. **Measured, not assumed.** Every memory constraint has a corresponding test (heap growth, eviction determinism, cache bounds).

---

## 2. Architecture Overview

```
┌────────────────────────────────────────────────────────────────────┐
│                        memory.Manager                              │
│  ┌─────────────┐  ┌──────────────┐  ┌───────────────────────────┐ │
│  │ per-component│  │ evictionOrder│  │ UpdateUsage(comp, bytes) │ │
│  │ limits       │  │ []Component  │  │ RegisterEvictor(comp, fn)│ │
│  └─────────────┘  └──────────────┘  └───────────────────────────┘ │
└───────────────────────┬────────────────────────────────────────────┘
                        │
          ┌─────────────┼──────────────────┐
          ▼             ▼                    ▼
    ┌──────────┐  ┌──────────┐        ┌──────────┐
    │ Glyph    │  │ Image    │  ...    │ Tile     │
    │ Cache    │  │ Cache    │        │ Cache    │
    │ Evictor  │  │ Evictor  │        │ Evictor  │
    └──────────┘  └──────────┘        └──────────┘
          │             │                    │
          ▼             ▼                    ▼
    ┌───────────────────────────────────────────┐
    │          Ordered eviction cascade          │
    │  Phase 1: per-component limit check        │
    │  Phase 2: global limit check (evictionOrder│
    │           walk)                            │
    └───────────────────────────────────────────┘
```

---

## 3. Memory Components and Default Limits

The `memory.Manager` tracks 11 components. Default limits are configured in `cmd/browser/main.go`:

| Component | Constant | Default Limit | Example eviction target |
|---|---|---|---|
| Network Cache | `ComponentNetworkCache` | 50 MB | Cached HTTP responses |
| Page Cache | `ComponentPageCache` | 20 MB | Cached page snapshots |
| Style | `ComponentStyle` | 50 MB | `StylePool`, `MatchCache` |
| Glyph | `ComponentGlyph` | 10 MB | Shaped glyph data |
| Image | `ComponentImage` | 30 MB | Decoded image bitmaps |
| Tile | `ComponentTile` | 50 MB | Raster tiles |
| Display List | `ComponentDisplayList` | 20 MB | Paint chunks |
| Layout Intrinsic Size | `ComponentLayoutIntrinsicSize` | 2 MB | Cached layout sizes |
| Layout | `ComponentLayout` | 50 MB | Layout tree |
| DOM | `ComponentDOM` | 100 MB | DOM node store |
| Script | `ComponentScript` | 20 MB | JS runtime heap |

**Global limit:** 512 MB total across all components.

Limits are configurable via `Manager.SetLimit(component, bytes)` and `Manager.SetGlobalLimit(bytes)`. Components report their usage via `Manager.UpdateUsage(component, estimatedBytes)`.

---

## 4. Eviction Order and Cascade

### Eviction Priority (least critical first)

The default eviction order is designed to evict the most expendable data first:

```
NetworkCache → PageCache → Style → Glyph → Image → Tile → DisplayList
  → LayoutIntrinsicSize → Layout → DOM → Script
```

This order prioritizes preserving JavaScript runtime state and DOM structure (hardest to reconstruct) over network caches (cheapest to reconstruct).

### Two-Phase Eviction

When `UpdateUsage` is called (or any time `checkLimitsAndEvict` is triggered), eviction runs in two phases:

**Phase 1 — Per-component limit check:**
1. Scan every component.
2. For any component exceeding its per-component limit, call its registered `Evictor` with the excess as the target byte amount.
3. Repeat until all components are within their limits or no evictor made progress.

**Phase 2 — Global limit check:**
1. If total usage still exceeds the global limit, walk the `evictionOrder` list sequentially.
2. For each component, call its `Evictor` with the remaining global deficit.
3. Skip components whose evictor returns 0 (nothing to evict).
4. Repeat until total usage is within the global limit or no evictor made progress.

### Evictor Contract

```go
// Evictor is a function that evicts at least targetBytes from a cache
// and returns the actual number of bytes freed.
type Evictor func(targetBytes uint64) uint64
```

Each cache is responsible for implementing its evictor to actually free memory. The manager relies on the returned byte count to update its tracked usage.

---

## 5. Individual Cache Bounds

### GlyphCache — `internal/renderer/frame/cache`

| Property | Value |
|---|---|
| Entry limit | 256 |
| Byte limit | 4 MB |
| Eviction | LRU (doubly-linked list) |
| Reject oversized entry | Yes (single glyph > 4 MB → not cached) |
| `memory.Evictor` | Yes |
| Concurrency | `sync.Mutex` |

### ImageCache — `internal/renderer/frame/cache`

| Property | Value |
|---|---|
| Entry limit | None (byte budget only) |
| Byte limit | 64 MB |
| Eviction | LRU (doubly-linked list) |
| Reject oversized entry | Yes |
| `memory.Evictor` | Yes |
| Concurrency | `sync.Mutex` |
| Notes | `GetOrLoad` prevents duplicate concurrent decode of the same URL |

### TileCache — `internal/renderer/frame/compositor`

| Property | Value |
|---|---|
| Entry limit | 1024 tiles |
| Byte limit | 32 MB |
| Eviction | Scan-based LRU (finds tile with lowest `LastUsed` frame) |
| `memory.Evictor` | Yes |
| Concurrency | `sync.Mutex` |
| Notes | Tiles are 256×256 pixels by default; version-tracked per frame |

### PageCache — `internal/engine/pagecache`

| Property | Value |
|---|---|
| Entry limit | 3 pages |
| Byte limit | 32 MB |
| Eviction | LRU (doubly-linked list) |
| `memory.Evictor` | Yes |
| Concurrency | `sync.Mutex` |

### HTTPCache — `internal/net`

| Property | Value |
|---|---|
| Entry limit | Configurable (not set by default) |
| Byte limit | Configurable (not set by default) |
| Eviction | LRU (doubly-linked list, batched index writes) |
| `memory.Evictor` | No direct evictor (managed internally; future: register with manager) |

### MatchCache — `internal/css`

| Property | Value |
|---|---|
| Entry limit | 512 |
| Byte limit | Tracked only (not enforced) |
| Eviction | LRU (doubly-linked list) |
| `memory.Evictor` | Yes |
| Key | `ElementKey{tag, id, sortedClasses}` |

### StylePool — `internal/css`

| Property | Value |
|---|---|
| Entry limit | 1024 |
| Byte limit | Tracked only (not enforced) |
| Eviction | LRU (doubly-linked list) |
| `memory.Evictor` | Yes |
| Notes | Deduplicates `InheritedStyle` via fingerprint + equality |

### IntrinsicSizeCache — `internal/renderer`

| Property | Value |
|---|---|
| Entry limit | None |
| Byte limit | 2 MB (default 4096 entries) |
| Eviction | LRU (doubly-linked list) |
| `memory.Evictor` | Yes |

### TextShaper — `internal/renderer`

| Property | Value |
|---|---|
| Entry limit | 1024 |
| Byte limit | None |
| Eviction | LRU (doubly-linked list) |
| `memory.Evictor` | No |

### HandleCache — `internal/js`

| Property | Value |
|---|---|
| Entry limit | 1024 |
| Byte limit | None |
| Eviction | Slice-based LRU |
| `memory.Evictor` | No |

### atom.Table — `internal/dom/atom`

| Property | Value |
|---|---|
| Entry limit | 1024 |
| Byte limit | 64 KB string data |
| Eviction | LRU (`container/list`) |
| Static atoms | 500 (never evicted) |
| `memory.Evictor` | No |

### PropertyTable — `internal/css`

| Property | Value |
|---|---|
| Entry limit | 256 |
| Byte limit | 16 KB |
| Eviction | LRU (`atom.NewTable`) |
| `memory.Evictor` | No |

---

## 6. GC Tuning

### Go Runtime Parameters

The `internal/memory/tuning.go` package provides evaluation infrastructure for Go GC tuning, but the production engine does not call `debug.SetGCPercent` or `debug.SetMemoryLimit` directly — the memory manager handles memory via soft limits and eviction, which is more predictable than relying on the Go GC for resource management.

| Parameter | Production Value | Notes |
|---|---|---|
| `GOGC` | 100 (default) | Not overridden in production |
| `GOMEMLIMIT` | Not set | Soft limits via memory.Manager instead |
| PGO | Not yet enabled | Waiting for representative production profiles |

### Thrashing Detection

The tuning infrastructure detects GC thrashing via two heuristics:
- **GC CPU fraction > 20%** — more than 20% of CPU time spent in GC
- **GC rate > 100 cycles/sec** — more than 100 GC cycles per second

### Heap Profile Strategy

Heap profiles are captured:
1. After idle (baseline)
2. After loading a reference page
3. After repeated navigation (to detect growth)
4. After scrolling (to detect tile leak)

---

## 7. Session Memory Lifecycle

```
Session created ──→ Session.Owned allocations:
                     ├── DOM store          (internal/dom)
                     ├── CSS data           (internal/css)
                     ├── Layout store       (internal/renderer)
                     ├── Display list       (internal/renderer)
                     ├── Tile cache         (compositor)
                     ├── Image/glyph caches (frame/cache)
                     ├── JS runtime         (internal/js)
                     └── HTTP transport     (internal/net)

Navigation ──→ Previous document released:
                ├── DOM, style, layout freed
                ├── Display list rebuilt
                ├── Tile cache invalidated
                ├── Image/glyph caches trimmed
                └── Script runtime reset

Session.Close ──→ All memory released:
                   ├── All caches drained
                   ├── HTTP transport closed
                   ├── JS runtime terminated
                   └── All goroutines joined
```

### Verification Tests

| Test | File | What it proves |
|---|---|---|
| `TestRepeatedNavigation_NoUnboundedHeapGrowth` | `internal/engine/session/memory_growth_test.go` | 50 navigation cycles retain < 512 KB |
| `TestRepeatedNavigation_HeapDoesNotGrowLinearly` | same file | Growth is sublinear, not O(n) |
| `TestClose_ReleasesSessionOwnedMemory` | same file | Closing session releases heap |

---

## 8. Configuring Limits

### Changing Default Limits

Default limits are set in `cmd/browser/main.go`. To change:

```go
mgr := memory.NewManager(memory.Config{
    Limits: map[memory.Component]uint64{
        memory.ComponentDOM:   200 * 1024 * 1024,  // 200 MB
        memory.ComponentTile:  100 * 1024 * 1024,  // 100 MB
    },
    GlobalLimit: 1024 * 1024 * 1024,  // 1 GB
})
```

### Per-Session Overrides

Each `Session` creates its own `memory.Manager` with defaults from the browser config. Sessions do not share managers — each session tracks and evicts independently.

---

## 9. Debugging Memory

### Developer Tools Panel

The Memory Budget panel displays a live table of all components, their limits, current usage, and eviction activity. Accessible from the developer tools menu ("Memory Budget" button).

### Enabling Trace Logging

```go
mgr := memory.NewManager(config)
mgr.SetDebugLog(true)  // logs every eviction decision
```

### Runtime Inspection

```go
stats := mgr.Stats()
// stats.Limits[memory.ComponentDOM]     → 100 MB
// stats.Usage[memory.ComponentDOM]      → 42 MB
// stats.GlobalLimit                     → 512 MB
// stats.TotalUsage                      → 267 MB
```

---

## 10. Best Practices for Adding a New Cache

1. **Choose bounding strategy.** Decide: entry count, byte budget, or both.
2. **Implement LRU eviction.** Use `container/list` for O(1) doubly-linked list eviction.
3. **Reject oversized entries.** If a single item exceeds the budget, do not cache it.
4. **Implement `Evict(targetBytes uint64) uint64`.** Register it with `memory.Manager` via `RegisterEvictor`.
5. **Track hit/miss/eviction metrics.** Use atomics for low overhead.
6. **Add a memory growth test.** Prove that repeated cache fills reach steady state.
7. **Document the cache** in this document's cache bounds table.
