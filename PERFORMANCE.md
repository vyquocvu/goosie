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

### Profiling

```bash
# CPU profiling
go test ./internal/renderer -bench=ViewportScroll -cpuprofile=cpu.prof
go tool pprof cpu.prof

# Memory profiling
go test ./internal/renderer -bench=ViewportScroll -memprofile=mem.prof
go tool pprof mem.prof
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
