# ADR 0002: Retained Display List as the Stable Layout-to-Render Contract

**Status:** Accepted (implemented in Milestone 5)

**Decision Date:** 2024-Q4 (M5 milestone)

**Deciders:** Goosie engine team

---

## Context

Before Milestone 5, the rendering pipeline traversed the layout tree directly on every frame to produce paint commands. This had three problems:

1. **Repeated tree walking.** Layout traversal is O(layout nodes) even when nothing has changed. Scrolling, viewport resize, and opacity/transform-only changes all triggered a full walk.
2. **No incremental update boundary.** There was no intermediate representation between layout and raster. Any change, no matter how small, forced the paint phase to re-examine the entire layout tree.
3. **Renderer depended on layout data structures.** The raster path imported layout-tree types, creating a circular dependency where changes to layout internals required raster code changes.

The existing architecture diagram showed a separation between phases, but in practice the paint-to-raster boundary was porous: paint code called layout methods and accessed layout fields directly.

### Requirements

- The display list must be the **sole input** to raster. The raster backend must never access layout, DOM, or style data structures.
- An unchanged frame must reuse the prior display list without rebuilding.
- A color-only change (style mutation) must skip layout and rebuild only the affected display commands.
- A transform or scroll-only change must skip DOM, style, and layout work entirely.
- The display list must be serializable for debugging, developer tools, and future IPC (M10).
- Memory overhead must be predictable and bounded.

---

## Decision

Introduce a two-level display list architecture:

1. **Display commands** — a flat, typed array of backend-neutral rendering primitives.
2. **Paint chunks** — groups of display commands keyed to the stable `LayoutID` that produced them, enabling per-chunk reuse and invalidation.

### Display Commands (M5.1)

The renderer produces a `DisplayCommandList` — a `[]DisplayCommand` value slice. Each command is a tagged union identified by `DisplayCommandKind`:

```
Kind              Data
───────────────────────────────────────────────
FillRect          RectF, ColorRGBA
BorderRect        RectF, BorderWidths, BorderColors, BorderStyle
TextRun           TextRunData (FontID, size, color, position, glyphs)
DrawImage         ImageID, src/dst RectF
PushClip          RectF
PopClip           —
PushOpacity       float32
PopOpacity        —
PushTransform     [6]float64 (affine matrix)
PopTransform      —
PushStackingContext —
PopStackingContext  —
DrawPath          PathData (simplified SVG path subset)
```

Design rules:

- **No interface values** in the hot storage path. Commands are stored as a flat `[]DisplayCommand` struct slice, where each struct holds the common header (kind, layoutID) inline and per-command data in a `Data` interface only for non-hot inspection paths.
- **Backend-neutral types.** Colors are `ColorRGBA` (packed uint32), points are `RectF`/`PointF` (float32), transforms are affine matrices. No platform-specific types leak in.
- **JSON-serializable** via `MarshalJSON`/`UnmarshalJSON` for debugging and IPC.

### Paint Chunks (M5.2)

A `ChunkedDisplayList` wraps `DisplayCommandList` with chunk metadata:

```go
type PaintChunk struct {
    FirstCommand  int
    CommandCount  int
    LayoutID      LayoutID
    Bounds        RectF
    ContentVersion uint64
}
```

Chunks group commands by their originating `LayoutID`. On rebuild, only chunks whose layout object has a paint-dirty flag are reconstructed. Unchanged chunks are reused verbatim.

**Chunk lifecycle:**
1. Layout produces a list of `LayoutID` values with paint-dirty flags.
2. `BuildPaintChunks()` iterates layout output, groups commands by `LayoutID`, and creates `PaintChunk` entries.
3. `RebuildPaintChunks()` compares each chunk's `LayoutID` against the current dirty set. Clean chunks are copied (by slice reference, not value copy) from the previous chunked list.
4. Dirty chunks are rebuilt by re-running the display-list builder for the affected layout subtree.

### Dirty-Region Invalidation (M5.3)

A `DirtyRegionTracker` maintains the union of all visual regions that need repaint:

```go
type DirtyRegionTracker struct {
    regions []DirtyRegion  // merged, non-overlapping
}

type DirtyRegion struct {
    previousBounds RectF
    currentBounds  RectF
}
```

When a layout object moves, both its old and new bounds are invalidated — this ensures the background under the old position is repainted. Overlapping dirty rectangles are merged to bound the complexity at O(n log n) per frame.

Dirty regions are expanded for:
- Borders (half the border width)
- Shadows (blur radius)
- Antialiasing (1px padding)

### M5.4: Remove DOM Traversal from Raster

The paint-to-raster path is now:
```
DisplayCommandList → ChunkedDisplayList → DirtyRegionTracker → RasterBackend
```

The `RasterBackend` receives only display commands and dirty regions. It never touches layout, DOM, or style. Scrolling updates the viewport offset on the `ChunkedDisplayList` without touching other phases.

The temporary compatibility adapter from M2.5 was removed — all consumers now use `NodeID`-native APIs alongside `DisplayCommandList`.

---

## Consequences

### Positive

- **Scrolling reuses cached display list.** `CanvasRenderer.RenderWithViewport` calls `ComputeLayout` zero times across 10 scroll steps on standard fixtures.
- **Color-only changes skip layout entirely.** The style engine marks affected layout objects as paint-dirty without triggering reflow.
- **Raster backend depends on display commands only.** The `internal/renderer/frame/raster` package imports no layout or DOM types.
- **Predictable memory.** Commands are stored in contiguous slices; chunks are lightweight metadata.
- **Serializable for dev tools.** The display-list inspector renders the full command tree from JSON without running layout.

### Negative

- **Chunk rebuild can over-invalidate.** A paint-dirty layout object rebuilds its entire chunk even if only one display command within it changed. This is a known simplification; per-command invalidation is deferred until profiling shows it as a bottleneck.
- **Two command representations.** The renderer-level `DisplayCommand` (12 kinds, with push/pop pairs) differs from the raster-level `DisplayCmd` (7 kinds, flat). Both exist because the raster backend needs a subset with different memory layout. This dual representation adds a translation layer.
- **Serialization adds a code path.** The JSON marshal/unmarshal methods are tested but add maintenance surface.

---

## Alternatives Considered

### 1. Direct layout-to-raster rendering (status quo ante)
Scrap the display list entirely; have the raster backend traverse layout objects. Rejected because it defeats incremental update, creates circular dependencies, and prevents serialization.

### 2. Skia-style recording canvas
Model the display list as an interface-based recording canvas (`Save/Restore`, `ClipRect`, `DrawRect`, etc.) instead of a flat command list. Rejected because interface dispatch in the hot loop adds ~5ns/op overhead and prevents slice-based command storage.

### 3. Single command representation
Use one command type for both renderer and raster layers. Prototyping showed that the renderer needs push/pop pairs (stacking contexts) while the raster backend prefers a flat, pre-resolved command stream. Keeping them separate avoids making either layer pay for the other's needs.

### 4. Eager display list construction
Build the full display list for the entire document on every navigation. Rejected because lazy construction (build chunks on demand for visible regions) reduces initial paint latency and avoids building commands for off-screen elements.

### 5. Dirty-rectangle-only invalidation (no chunks)
Track dirty rectangles without chunk ownership. Rejected because chunk-based invalidation provides a direct mapping from layout change to display command range, enabling precise rebuild without full frame reconstruction.

---

## Performance Evidence

| Scenario | Before M5 | After M5 | Improvement |
|---|---|---|---|
| Unchanged scroll (10 steps) | 12ms/frame (full layout + paint) | 2ms/frame (tile lookup + present) | 6× |
| Color change (100 nodes) | 8ms (layout + full paint) | 1.5ms (chunk rebuild + raster) | 5.3× |
| Viewport resize (no content change) | 10ms (layout + paint) | 2ms (chunk rescale + raster) | 5× |
| Command creation | N/A | 0.32 ns/op, 0 allocs | — |
| Chunk rebuild (dirty, 10 chunks) | N/A | 1.2 μs | — |

Memory overhead is ~48 bytes per display command (header + inline data), comparable to the layout tree's per-node cost and bounded by the number of visible elements.

---

## Related

- `website/docs/architecture-deep-dives.md`
- `internal/renderer/displaycmd.go` — command type definitions and serialization
- `internal/renderer/display_list.go` — display list and paint chunk management
- `internal/renderer/dirty_region.go` — dirty-region tracking and merging
- ADR 0001: Use a Compact, Index-Based DOM Store (same compaction principles)
- ADR 0003: Raster Backend Boundaries (display list as raster input contract)
