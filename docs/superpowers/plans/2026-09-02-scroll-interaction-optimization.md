# Stream D: Scroll/Interaction Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce scroll jank by using the Y-band spatial index for viewport culling and adding a scroll-only fast path.

**Architecture:** Use existing Y-band index in RenderWithViewport to skip off-screen commands. Add scroll-only invalidation flag to skip display list rebuilds on pure scroll.

**Tech Stack:** Go, internal/renderer package

---

## Task D1: Y-band viewport culling in RenderWithViewport

**Files:**
- Modify: `internal/renderer/canvas.go` — RenderWithViewport method

- [x] **Step 1: Read current RenderWithViewport implementation**

Read `internal/renderer/canvas.go` and find the `RenderWithViewport` method. Understand how it iterates display list commands.

- [x] **Step 2: Add Y-band culling logic**

Before the main command iteration loop, add viewport-based culling using the Y-band index:

```go
// Determine visible command range from Y-band index
cmdStart := 0
cmdEnd := len(displayList.Commands)

if len(displayList.YBands) > 0 {
    viewportTop := viewportY
    viewportBottom := viewportY + viewportHeight
    
    // Find first band overlapping viewport
    for i, band := range displayList.YBands {
        if band.YEnd >= viewportTop && band.YStart <= viewportBottom {
            if band.CmdStart >= 0 && (cmdStart == 0 || band.CmdStart < cmdStart) {
                cmdStart = band.CmdStart
            }
            if band.CmdEnd >= 0 && band.CmdEnd > cmdEnd {
                cmdEnd = band.CmdEnd
            }
        }
        // Early exit: no more bands can overlap
        if band.YStart > viewportBottom {
            break
        }
        _ = i
    }
}

// Iterate only visible commands
for i := cmdStart; i < cmdEnd && i < len(displayList.Commands); i++ {
    cmd := displayList.Commands[i]
    // ... existing paint logic ...
}
```

- [x] **Step 3: Handle clip commands correctly**

PushClip/PopClip commands must be balanced. When culling, ensure we don't start in the middle of a clip group. Walk backward from cmdStart to find the nearest PopClip or start of list:

```go
// Ensure clip balance: walk back to find clip boundary
for i := cmdStart - 1; i >= 0; i-- {
    if displayList.Commands[i].Type == PopClip {
        break
    }
    if displayList.Commands[i].Type == PushClip {
        cmdStart = i
        break
    }
}
```

- [x] **Step 4: Run tests**

```bash
go test ./test/internal/renderer/... -v
```

Expected: All tests pass

- [x] **Step 5: Verify pixel output unchanged**

```bash
go test -v ./test/perf -run TestPixelHashManifest
```

Expected: Pass (same visual output, just fewer commands iterated)

- [x] **Step 6: Commit**

```bash
git add internal/renderer/canvas.go
git commit -m "feat(renderer): use Y-band spatial index for viewport culling in RenderWithViewport"
```

---

## Task D2: Scroll-only fast path

**Files:**
- Modify: `internal/renderer/invalidation.go:13-66`
- Modify: `internal/renderer/renderer.go` — PresentFromMutationBatch

- [x] **Step 1: Add scroll-only detection**

In `ApplyMutationBatch`, detect when all mutations are paint-only (no layout/structure changes):

```go
func (r *Renderer) ApplyMutationBatch(batch []MutationInvalidation) int {
    
    scrollOnly := true
    for _, mutation := range batch {
        if mutation.Flags&(DirtyLayout|DirtySubtree|DirtyStyle) != 0 {
            scrollOnly = false
            break
        }
    }
    
    if applied > 0 {
        r.dirty = true
        if scrollOnly {
            // Skip layout recomputation and display list invalidation
            r.canvasRenderer.mu.Lock()
            r.canvasRenderer.scrollOnlyDirty = true
            r.canvasRenderer.mu.Unlock()
            return applied
        }
        // ... existing layout/display list invalidation ...
    }
    return applied
}
```

- [x] **Step 2: Add scrollOnlyDirty flag to CanvasRenderer**

Add the field to `CanvasRenderer`:

```go
type CanvasRenderer struct {
    // ... existing fields ...
    scrollOnlyDirty bool
}
```

- [x] **Step 3: Handle scroll-only in PresentFromMutationBatch**

In `PresentFromMutationBatch`, check the flag and skip display list rebuild:

```go
func (r *Renderer) PresentFromMutationBatch(adapter *FyneAdapter) bool {
    
    r.canvasRenderer.mu.RLock()
    scrollOnly := r.canvasRenderer.scrollOnlyDirty
    r.canvasRenderer.mu.RUnlock()
    
    if scrollOnly {
        // Display list is valid, just re-render with current viewport
        r.canvasRenderer.mu.Lock()
        r.canvasRenderer.scrollOnlyDirty = false
        r.canvasRenderer.mu.Unlock()
        // Trigger a repaint without rebuilding display list
        r.canvasRenderer.mu.Lock()
        r.canvasRenderer.cachedDisplayList = nil // Force re-render
        r.canvasRenderer.mu.Unlock()
        return true
    }
    
    // ... existing full paint path ...
}
```

- [x] **Step 4: Run tests**

```bash
go test ./test/internal/renderer/... -v
```

Expected: All tests pass

- [x] **Step 5: Verify pixel output unchanged**

```bash
go test -v ./test/perf -run TestPixelHashManifest
```

Expected: Pass

- [x] **Step 6: Commit**

```bash
git add internal/renderer/invalidation.go internal/renderer/canvas.go
git commit -m "feat(renderer): add scroll-only fast path for mutation batches"
```

---

## Task D3: Final verification

- [x] **Step 1: Run all renderer tests**

```bash
go test ./test/internal/renderer/... -v
```

Expected: All tests pass

- [x] **Step 2: Run full test suite**

```bash
go test ./...
```

Expected: All tests pass

- [x] **Step 3: Verify pixel hashes unchanged**

```bash
go test -v ./test/perf -run TestPixelHashManifest
```

Expected: Pass

- [x] **Step 4: Benchmark scroll performance (optional)**

If scroll benchmarks exist, run before/after comparison.

Expected: Improved frame time during scroll

