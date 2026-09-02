# internal/memory — Agent Constraints & Architecture

## Core Responsibilities

The `internal/memory` package provides centralized memory accounting, soft limit enforcement, component memory budgets, and ordered eviction cascades across the browser engine subsystems.

## Component Allocation Constants

Subsystems register and track memory under explicit `Component` identifiers:
- `ComponentDOM`: Compact DOM store records, attribute arrays, and text data buffers.
- `ComponentStyle`: Parsed stylesheet rules, match caches, and interned style objects.
- `ComponentLayout`: Layout box trees, line boxes, and fragment trees.
- `ComponentDisplayList`: Serialized display list drawing commands and spatial indices.
- `ComponentTile`: Rasterized pixel tile buffers and composition surfaces.
- `ComponentImage`: Decoded raster images and SVG render buffers.
- `ComponentGlyph`: Rasterized font glyph bitmaps and font atlas caches.
- `ComponentScript`: JavaScript compilation program cache and heap memory allocations.
- `ComponentNetworkCache`: In-memory HTTP cache responses and byte slices.
- `ComponentLayoutIntrinsicSize`: Memoized intrinsic sizing tables for boxes and text.
- `ComponentPageCache`: Cached session history pages and DOM snapshots.

## Evictor Registration Contract

- Components register eviction handlers via `Manager.RegisterEvictor(component Component, evictor Evictor)`.
- The evictor signature is: `type Evictor func(targetBytes uint64) uint64`.
- **Evictor Invariants**:
  1. Evictors must be non-blocking and safe for concurrent execution.
  2. Evictors must return the actual number of bytes freed.
  3. Evictors must never panic or perform blocking I/O during memory reclamation.

## Ordered Eviction Cascade

- When global or per-component soft memory limits are exceeded, `Manager` executes an ordered eviction cascade.
- Eviction prioritizes disposable, easily recomputed visual caches before touching fundamental DOM or layout trees:
  1. `ComponentDisplayList` / `ComponentTile` (re-rasterizable surfaces)
  2. `ComponentImage` / `ComponentNetworkCache` / `ComponentPageCache` (re-fetchable assets)
  3. `ComponentGlyph` (re-renderable font glyphs)
  4. `ComponentLayoutIntrinsicSize` / `ComponentStyle` (recomputable layout metrics)
  5. `ComponentDOM` / `ComponentScript` (critical state; only pruned when explicitly requested)

## Memory Tuning & Dynamic Budgets

- `tuning.go` calculates dynamic memory thresholds based on total host RAM and active pressure conditions, adjusting cache limits adaptively.

## Testing & Verification

All memory manager tests reside in `test/internal/memory/...`.

Run the memory subsystem test suite:
```bash
go test -race -short ./test/internal/memory/...
```
