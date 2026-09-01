# internal/renderer — Agent Constraints

## Threading Contract

**Build phase** (parse, style, layout) is safe off-thread. **Present phase**
(`PresentFrame`, `RenderWithViewport`, `FyneAdapter.PresentFrame`) MUST run on
the Fyne main goroutine — violating this deadlocks `async.EnsureMain`.

## Invalidation Layers

Three layers interact — understand all before modifying invalidation:
1. `DirtyFlag` bitmask per node with upward/downward propagation
2. `PaintChunk` groups commands by LayoutID; dirty chunks rebuilt incrementally
3. `DirtyRegion` tracks bounds (max 64 rects, auto-merged). Always invalidate
   both old+new bounds on move. Call `ExpandForEffects` for shadow/border/AA.

Never drop canvas cache after mutation without rebuilding dirty chunks.

## Canvas API Duality

Two parallel paths: Fyne (`CanvasRenderer` → `fyne.CanvasObject`) and raster
(`raster.Backend` → `image.Image`). Do not mix. Never bypass `ScrollCoalescer`
for scroll events — without it every tick serializes behind the main thread.

## Golden Tests

Raster/layout changes require: `go test ./internal/renderer/frame/golden/...`
