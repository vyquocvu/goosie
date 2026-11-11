# Debug Tools for Render/Paint Testing

This document describes the debug tools available for testing and profiling Goosie's rendering pipeline.

## Overview

The debug toolkit provides:

1. **Render Debug** (`internal/debug/render_debug.go`) - Frame-by-frame tracking of pipeline work
2. **Visual Overlay** (`internal/debug/overlay.go`) - Draw damage rects, tiles, hover/focus areas
3. **Profiler** (`internal/debug/profiler.go`) - Timing for each pipeline stage
4. **debug-render** (`cmd/debug-render/`) - Interactive debug window with overlays
5. **test-render** (`cmd/test-render/`) - Automated pipeline testing and profiling

## Quick Start

### Test a single HTML file

```bash
# Automated test with profiling
go run ./cmd/test-render -profile testdata/render/debug-test.html

# Interactive debug with overlay
go run ./cmd/debug-render -overlay -profile testdata/render/debug-test.html
```

### Run multiple iterations

```bash
# Average timing over 10 iterations
go run ./cmd/test-render -profile -iterations 10 testdata/render/debug-test.html
```

### Test synthetic scenes

```bash
# Checkerboard pattern (100 cells)
go run ./cmd/debug-render -scene checkerboard -overlay

# Hover test (50 boxes with :hover effects)
go run ./cmd/debug-render -scene hover -overlay

# Selection test (long text paragraph)
go run ./cmd/debug-render -scene select -overlay

# Focus test (20 input fields)
go run ./cmd/debug-render -scene focus -overlay
```

## Tools

### test-render

Automated pipeline testing without opening a window. Useful for CI and performance regression testing.

**Flags:**
- `-profile` - Print pipeline profiling stats
- `-iterations N` - Run N iterations (default 1)
- `-width N` - Viewport width (default 1024)
- `-height N` - Viewport height (default 768)
- `-dpr N` - Device pixel ratio (default 2.0)

**Output:**
```
test-render: testing testdata/render/debug-test.html (3456 bytes)
test-render: iteration 0:
  nodes: 156
  layout objects: 89
  paint commands: 342
  style: 12.5ms
  layout: 8.3ms
  paint: 5.2ms
  damage: 0.8ms
  damage rect: {X0:100 Y0:100 X1:250 Y1:250}

=== Pipeline Profiler ===
Stage                   Calls      Total        Avg        Min        Max
--------------------------------------------------------------------------------
style                       1   12.5ms     12.5ms     12.5ms     12.5ms
layout                      1    8.3ms      8.3ms      8.3ms      8.3ms
paint                       2    6.0ms      3.0ms      2.8ms      3.2ms
content_damage              1    0.8ms      0.8ms      0.8ms      0.8ms
restyle                     1    4.2ms      4.2ms      4.2ms      4.2ms

Histogram (bucket counts):
Stage                  <1ms    1-5ms   5-10ms  10-50ms 50-100ms   >100ms
--------------------------------------------------------------------------------
style                     0        0        1        0        0        0
layout                    0        1        0        0        0        0
paint                     0        2        0        0        0        0
===========================

test-render: OK
```

### debug-render

Interactive debug window with visual overlays. Shows damage rects, tile boundaries, hover/focus areas in real-time.

**Flags:**
- `-overlay` - Show debug overlay (tiles, damage, hover, focus)
- `-profile` - Print pipeline profiling stats on exit
- `-log` - Log frames and interactions to `/tmp/goosie-debug.log`
- `-frames N` - Run N frames in bench mode (default 0 = interactive)
- `-width N` - Viewport width (default 1024)
- `-height N` - Viewport height (default 768)
- `-dpr N` - Device pixel ratio (default 2.0)
- `-scene name` - Use synthetic scene instead of HTML file

**Interactive mode:**
- Move mouse → see cyan hover rect around hovered element
- Click → see yellow focus rect around focused element
- Select text → see blue selection rect
- Scroll → see red damage rects showing what was repainted
- Ctrl-C → exit and print stats

**Overlay colors:**
- **Gray lines** - Tile boundaries (256px grid)
- **Red** - Damage rects (what was repainted)
- **Cyan** - Hover area
- **Yellow** - Focus area
- **Blue** - Selection area

**Bench mode:**
```bash
# Run 100 frames without display, collect stats
go run ./cmd/debug-render -frames 100 -profile testdata/render/debug-test.html
```

## Debug Package API

### Renderer

Tracks frame-by-frame rendering work and interactions.

```go
import "github.com/vyquocvu/Develop/Browser/goosie/internal/debug"

// Create renderer
renderer := debug.NewRenderer(true) // enabled
renderer.LogToFile = true
renderer.LogPath = "/tmp/goosie-debug.log"

// Track frames
renderer.BeginFrame(serial, "hover")
renderer.EndFrame(stylePasses, layoutPasses, paintCalls, tilesReused, tilesRasterized, damageRects)

// Track interactions
renderer.LogInteraction("hover", x, y, targetNode, triggeredStyle, triggeredLayout, triggeredPaint, damageRect)

// Get stats
stats := renderer.Stats()
fmt.Printf("Total frames: %d\n", stats.TotalFrames)
fmt.Printf("Avg frame duration: %v\n", stats.AvgFrameDuration)

// Print report
renderer.PrintStats()
```

### Overlay

Draws debug visualization on top of rendered frames.

```go
// Create overlay
overlay := debug.NewOverlay(true) // enabled
overlay.DrawTiles = true
overlay.DrawDamage = true
overlay.DrawHover = true
overlay.DrawFocus = true
overlay.DrawSelection = true

// Draw onto display list
overlay.Draw(list, scale, viewport, damageRects, hoverRect, focusRect, selectionRect)
```

### Profiler

Measures timing for each pipeline stage.

```go
// Create profiler
profiler := debug.NewProfiler(true) // enabled

// Time a stage
profiler.Begin(debug.StageStyle)
// ... do style work ...
profiler.End()

// Or use Time() helper
profiler.Time(debug.StagePaint, func() {
    // ... do paint work ...
})

// Print report
profiler.Print()

// Get stats
stats := profiler.Stats()
for _, s := range stats {
    fmt.Printf("%s: %d calls, %v total, %v avg\n", s.Name, s.Calls, s.TotalTime, s.AvgTime)
}
```

**Stage constants:**
- `debug.StageStyle` - Style resolution
- `debug.StageLayout` - Layout/reflow
- `debug.StagePaint` - Display list build
- `debug.StageRasterize` - Tile rasterization
- `debug.StageComposite` - Frame composition
- `debug.StagePresent` - Window present
- `debug.StageContentDamage` - Damage rect computation
- `debug.StageReflow` - Full reflow
- `debug.StageRestyle` - Style re-resolution

## What to Look For

### Flickering Issues

**Symptoms:**
- High `PaintCalls` per frame (>1)
- Large damage rects (full viewport)
- Long `content_damage` times (>5ms)
- Many tiles rasterized per frame (>20)

**Debug steps:**
1. Run with `-overlay -profile`
2. Interact with page (hover, click, select)
3. Observe damage rects - should be small and localized
4. Check profile - `restyle` and `paint` should be <10ms
5. If damage rect is full viewport → interaction is triggering full repaint

### Performance Issues

**Symptoms:**
- Frame duration >16ms (60fps target)
- Style/layout passes >5ms each
- Many tiles rasterized per frame

**Debug steps:**
1. Run with `-profile -iterations 10`
2. Check avg times for each stage
3. Look for stages with >5ms avg time
4. Check histogram - many calls in >100ms bucket = problem

### Correctness Issues

**Symptoms:**
- Visual artifacts
- Missing elements
- Wrong colors/positions

**Debug steps:**
1. Run with `-overlay`
2. Check tile boundaries align with content
3. Verify damage rects cover changed areas
4. Compare paint command count with expected

## Example Debug Sessions

### Session 1: Investigate hover flicker

```bash
# Run with overlay and profiling
go run ./cmd/debug-render -overlay -profile testdata/render/debug-test.html

# In the window:
# 1. Move mouse slowly over boxes
# 2. Observe damage rects (should be small, around hovered box)
# 3. If damage rect is full viewport → problem
# 4. Press Ctrl-C to see stats

# Expected output:
# - Damage rects should be ~150x150px (box size)
# - Paint calls: 1-2 per hover
# - Restyle time: <5ms
```

### Session 2: Profile selection performance

```bash
# Run automated test
go run ./cmd/test-render -profile -iterations 20 testdata/render/debug-test.html

# Simulate selection in code:
# 1. Load page
# 2. Call SelectAt(x, y)
# 3. Call SelectTo(x2, y2)
# 4. Measure time for each stage

# Look for:
# - Paint time should be <10ms
# - Damage rect should cover selection only
# - No layout passes (selection is paint-only)
```

### Session 3: Benchmark scroll performance

```bash
# Run bench mode
go run ./cmd/debug-render -frames 600 -profile -scene checkerboard

# Expected:
# - 600 frames in ~10 seconds (60fps)
# - Most tiles reused (>90%)
# - Few tiles rasterized per frame (<5)
# - No style/layout passes (scroll is paint-only)
```

## Integration with Tests

Add debug assertions to tests:

```go
func TestHoverDoesNotTriggerFullRepaint(t *testing.T) {
    renderer := debug.NewRenderer(true)
    profiler := debug.NewProfiler(true)
    
    // Load page
    sess := loadTestSession(t, "hover-test.html")
    
    // Initial paint
    profiler.Begin(debug.StagePaint)
    dl1 := paint(sess)
    profiler.End()
    
    // Hover
    profiler.Begin(debug.StageRestyle)
    sess.SetHover(100, 100)
    sess.RestyleAndReflow(800)
    profiler.End()
    
    // Repaint
    profiler.Begin(debug.StagePaint)
    dl2 := paint(sess)
    profiler.End()
    
    // Compute damage
    profiler.Begin(debug.StageContentDamage)
    damage := paint.ContentDamage(dl1, dl2)
    profiler.End()
    
    // Assert damage is small
    if damage.W > 200 || damage.H > 200 {
        t.Errorf("hover triggered full repaint: damage=%v", damage)
    }
    
    // Assert timing
    stats := profiler.Stats()
    for _, s := range stats {
        if s.Name == debug.StageRestyle && s.AvgTime > 10*time.Millisecond {
            t.Errorf("restyle too slow: %v", s.AvgTime)
        }
    }
}
```

## Future Enhancements

Planned features:

1. **Frame recorder** - Record frames to video for visual inspection
2. **Tile cache visualizer** - Show which tiles are cached/evicted
3. **Memory profiler** - Track allocations per stage
4. **Comparison mode** - Side-by-side with reference browser
5. **Regression detector** - Alert on performance regressions

## Troubleshooting

### Debug tools don't compile

```bash
# Check imports
go list -f '{{.Imports}}' ./internal/debug

# Should include:
# - github.com/vyquocvu/Develop/Browser/goosie/internal/frame
# - github.com/vyquocvu/Develop/Browser/goosie/internal/paint
```

### Overlay doesn't show

- Ensure `-overlay` flag is set
- Check `overlay.Enabled = true`
- Verify display list is not empty

### Profiler shows no stats

- Ensure `-profile` flag is set
- Check `profiler.Enabled = true`
- Verify `Begin()` and `End()` are called in pairs

### Log file not created

- Check `renderer.LogToFile = true`
- Verify path is writable
- Check stderr for error messages

## See Also

- `PROJECT.md` - Architecture overview
- `docs/roadmap-v2.md` - Performance targets
- `internal/engine/engine.go` - Pipeline implementation
- `internal/paint/list.go` - Display list and ContentDamage
- `internal/raster/scheduler.go` - Tile scheduling
