# internal/renderer — Agent Constraints & Architecture

## Threading Contract

- **Build Phase** (DOM parsing, style resolution, box generation, layout computation): Safe to execute off-thread on background worker goroutines.
- **Present Phase** (`PresentFrame`, `RenderWithViewport`, `FyneAdapter.PresentFrame`): MUST execute on the Fyne main goroutine via `async.EnsureMain`. Violating this will deadlock the UI loop.

## Spatial Indexing & Viewport Culling (`yBandHeight = 200`)

- `DisplayListBuilder.Build` partitions non-clip commands into vertical bands of fixed height `yBandHeight = 200` px stored in `displayList.YBands`.
- `RenderWithViewport` activates spatial indexing when `len(displayList.Commands) > 3000`.
- It locates overlapping bands in the expanded vertical range `[viewportY - 2*H, viewportY + 3*H]` to restrict command iteration to the slice `[cmdStart, cmdEnd]`.
- It performs backward scanning from `cmdStart` to ensure all enclosing `PushClip` state entries are preserved on the canvas clipping stack before rasterizing the visible range.

## Scroll-Only Fast Path (`scrollOnlyDirty`)

- In `invalidation.go`, `ApplyMutationBatch` inspects dirty flags. If no mutations affect layout, subtree, or style (`mutation.Flags&(DirtyLayout|DirtySubtree|DirtyStyle) == 0`), the batch is classified as `scrollOnly`.
- Setting `canvasRenderer.scrollOnlyDirty = true` skips full layout recomputation and preserves the cached display list.
- `PresentFromMutationBatch` recognizes `scrollOnlyDirty`, clears the flag, and triggers an immediate repaint directly using the existing display list.

## Tiled Parallel Rasterization (`TiledRasterizer`)

- `internal/renderer/frame/raster/tiled.go` implements `TiledRasterizer`, partitioning frames into horizontal strips (`DefaultTileHeight = 1024` px).
- Tiles are rasterized concurrently across `runtime.GOMAXPROCS(0)` worker goroutines using per-tile `CPUBackend` instances and pixel buffers obtained from `BufferPool`.

## Zero-Allocation Pixel Buffer Pool (`BufferPool`)

- `internal/renderer/frame/raster/pool.go` provides `BufferPool` and `GlobalBufferPool()`.
- Pools manage `*image.RGBA` pixel buffers indexed by dimensions (`width<<32 | height`) using `sync.Map` and `*sync.Pool`.
- Pixel buffers are zeroed on `Get` and recycled on `Put`, eliminating per-frame garbage collector pressure during continuous rendering or scrolling.

## Virtual DOM Reconciler & Incremental Painter

- `Reconciler.Diff` in `reconciler.go` compares old and new `RenderNode` trees using `NodeID` identity.
- It produces a minimal sequence of `DOMPatch` operations:
  - `PatchUpdateText`
  - `PatchUpdateAttr`
  - `PatchUpdateStyle`
  - `PatchInsertChild`
  - `PatchRemoveChild`
  - `PatchReplaceSubtree`
- `incremental_painter.go` computes dirty regions (`DirtyRegionFromPatches`), rasterizes only the modified bounding rectangles, and blits them onto the existing frame buffer.

## Computed Style Interning & Rule Bucketing

- `globalStylePool.Intern` interns immutable computed `*Style` structs to eliminate redundant style instances across DOM nodes.
- `inlineStyleCache` memoizes parsed CSS declarations by raw `style` attribute string.
- `prepared map[*css.StyleSheet][]preparedRule` and `ruleBuckets` partition rules by their right-most compound selector (tag, ID, class, or wildcard) for O(1) candidate rule filtering.
- Custom properties (`CustomProperties map[string]string`) inherit via pointer sharing with copy-on-write semantics.

## Enumerated Property Atoms in Style

- 16 enumerated `uint8` atom types from `internal/css` (`DisplayAtom`, `PositionAtom`, `FloatAtom`, `VisibilityAtom`, `FontStyleAtom`, `TextAlignAtom`, `TextDecorationAtom`, `TextTransformAtom`, `WhiteSpaceAtom`, `BackgroundRepeatAtom`, `BackgroundPositionAtom`, `BackgroundSizeAtom`, `BackgroundAttachmentAtom`, `ListStyleTypeAtom`, `ListStylePositionAtom`, `OverflowAtom`) replace string comparisons in `renderer.Style`.
- Note that other properties (`BoxSizing`, `Clear`, `FlexDirection`, `FlexWrap`, `JustifyContent`, `AlignItems`, `AlignSelf`, `AlignContent`, `GridAutoFlow`, `VerticalAlign`, `WordBreak`) are `string` fields on `ComputedStyle` / `Style`.

## Invalidation Layers

Three invalidation layers interact — understand all three before modifying invalidation logic:
1. `DirtyFlag` bitmask per node with upward and downward propagation.
2. `PaintChunk` groups display commands by `LayoutID`; dirty chunks are rebuilt incrementally.
3. `DirtyRegion` tracks bounding rectangles (bounded to 64 rects with auto-merging). Always invalidate both old and new bounds on geometry change, and apply `ExpandForEffects` for shadows, borders, and anti-aliasing.

## Canvas API Duality

Two parallel rendering backends exist:
- **Fyne Canvas**: `CanvasRenderer` producing `fyne.CanvasObject` trees.
- **Raster Canvas**: `raster.Backend` producing `image.Image` / `*image.RGBA` buffers.
Do not mix backend primitives. Never bypass `ScrollCoalescer` for scroll events.

## Testing & Verification

All renderer tests reside in `test/internal/renderer/...`.

Run the full renderer test suite:
```bash
go test -short ./test/internal/renderer/...
```

Run golden layout and frame raster tests:
```bash
go test ./test/internal/renderer/layoutgolden/...
go test ./test/internal/renderer/frame/golden/...
```

Run subpackage tests:
```bash
go test ./test/internal/renderer/frame/...
go test ./test/internal/renderer/frame/raster/...
go test ./test/internal/renderer/frame/compositor/...
go test ./test/internal/renderer/frame/cache/...
```
