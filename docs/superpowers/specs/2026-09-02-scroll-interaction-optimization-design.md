# Stream D: Scroll/Interaction Optimization

## Status
**IMPLEMENTED**

## Goal
Reduce scroll jank and improve interaction responsiveness by optimizing the render pipeline's viewport culling and paint path.

## Scope
- `internal/renderer/canvas.go` — RenderWithViewport, display list caching
- `internal/renderer/display_list.go` — Y-band spatial index
- `internal/renderer/renderer.go` — PresentFrame

## Constraints
- Zero pixel change (same visual output)
- All existing tests pass
- No new dependencies

## Optimizations

### D1: Y-band viewport culling in RenderWithViewport
**Problem:** `RenderWithViewport` iterates all display list commands even when most are off-screen. The Y-band index exists but isn't used during rendering.

**Solution:** In `RenderWithViewport`, use the Y-band index to skip commands outside the visible viewport range. Only iterate commands in bands that overlap `[viewportY, viewportY+viewportHeight]`.

**Implementation:** Before the main command loop, binary search for the first band that overlaps the viewport. Skip commands before `band.CmdStart`. Stop iterating after `band.CmdEnd` when bands exceed viewport.

### D2: Display list pointer reuse
**Problem:** `DisplayList.Commands` is a `[]*PaintCommand`. Every display list rebuild allocates new command pointers. For a page with 1000 elements, that's 1000 allocations per frame.

**Solution:** Pre-allocate a pool of `PaintCommand` structs. On display list rebuild, reset and reuse commands from the pool instead of allocating new ones.

**Trade-off:** Adds complexity. Only worthwhile if display list rebuilds are frequent (they are — every mutation invalidates the cache).

**Alternative:** Skip this and focus on D1 + D3, which have higher impact.

### D3: Scroll-only fast path
**Problem:** Pure scroll (no DOM mutation) still triggers display list cache invalidation in some code paths. The display list is valid; only the viewport offset changed.

**Solution:** Add a `ScrollOnly` flag to the invalidation batch. When only scroll position changed, skip display list rebuild and just re-render with the new viewport offset.

**Implementation:** In `ApplyMutationBatch`, check if all mutations have `DirtyPaint` set but no `DirtyLayout` or `DirtySubtree`. If so, set a flag that tells `PresentFromMutationBatch` to skip display list rebuild.

## Out of Scope
- GPU-accelerated rasterization
- Double buffering (separate concern)
- Tiled rasterizer integration (already exists, just not wired up)

## Testing
- Existing renderer tests
- Scroll performance benchmark (measure frame time during scroll)
- Pixel hash unchanged
