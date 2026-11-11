# Phase Tracking Debug Guide

## Overview

Phase tracking monitors **render counts per node** and detects **phase offsets** (lệch pha) when elements are re-rendered unnecessarily during interactions.

This helps identify:
- Which nodes are re-styled/re-laid-out/re-painted on each interaction
- Whether hover/click/scroll cause excessive repaints
- Which elements are "out of phase" with expected rendering behavior

## Quick Start

```bash
# Enable phase tracking
go run ./cmd/test-render -phase -profile testdata/render/debug-test.html

# Output shows:
# - Phase offset detected after hover
# - Which nodes changed
# - Render counts per node
```

## Understanding Phase Offsets

**Phase offset** = when a node is re-rendered (style/layout/paint) but didn't need to be.

**Example:**
```
Node 139: style=+0 layout=+1 paint=+71
```

This means after hover:
- Style count: no change (good!)
- Layout count: +1 (reflow happened)
- Paint count: +71 (repainted 71 times!)

**Problem:** Node 139 was painted 71 times even though only hover state changed. This causes flickering and performance issues.

## Using Phase Tracking

### 1. Enable Phase Tracking

```bash
go run ./cmd/test-render -phase testdata/render/debug-test.html
```

### 2. Take Snapshot Before Interaction

The tool automatically takes a snapshot before simulating hover. You can also do this manually:

```go
phaseTracker.Snapshot()  // Save current render counts
```

### 3. Detect Phase Offset After Interaction

```go
deltas := phaseTracker.DetectPhaseOffset(0, "hover")
// Compare current counts with snapshot 0
// Return list of nodes that changed
```

### 4. Analyze Results

**Output:**
```
Phase offset detected after hover:
  nodes changed: 186
  most changed: node 139 (.) style=+0 layout=+1 paint=+71
```

**Interpretation:**
- `nodes changed: 186` - 186 nodes were re-rendered
- `most changed: node 139` - this node had the most changes
- `style=+0` - style count didn't change (good)
- `layout=+1` - layout was recomputed once
- `paint=+71` - paint was called 71 times (BAD!)

### 5. View Render Count Report

```
=== Render Count Report ===
Tracked nodes: 186
Snapshots: 1

Aggregate counts:
  Style:  185
  Layout: 558
  Paint:  1048

Top 10 most-rendered nodes:
NodeID     Tag             Class                   Style   Layout    Paint
---------------------------------------------------------------------------
139                                                    1        3      142
142                                                    1        3      130
174                                                    1        3       46
```

**Interpretation:**
- Node 139 was painted 142 times total
- Node 142 was painted 130 times total
- These are the "hot spots" causing performance issues

## API Reference

### PhaseTracker

```go
// Create tracker
tracker := debug.NewPhaseTracker(true) // enabled

// Track render counts
tracker.TrackStyle(nodeID)
tracker.TrackLayout(nodeID)
tracker.TrackPaint(nodeID)

// Batch tracking
tracker.TrackStyleBatch([]dom.NodeID{1, 2, 3})
tracker.TrackLayoutBatch([]dom.NodeID{1, 2, 3})
tracker.TrackPaintBatch([]dom.NodeID{1, 2, 3})

// Register node metadata
tracker.RegisterNode(nodeID, "div", "box", "main")

// Take snapshot
tracker.Snapshot()

// Detect phase offset
deltas := tracker.DetectPhaseOffset(0, "hover")

// Print reports
tracker.PrintReport()
tracker.PrintPhaseDeltas(deltas)
```

### Session Integration

```go
// Enable phase tracking in session
sess, _ := engine.NewSession(html, nil, 1024,
    engine.WithPhaseTracker(tracker),
)

// Track renders (automatic if tracker is set)
sess.TrackNodeRender(nodeID, "style")
sess.TrackNodeRender(nodeID, "layout")
sess.TrackNodeRender(nodeID, "paint")

// Snapshot and detect
sess.SnapshotRenderCounts()
deltas := sess.DetectPhaseOffset(0, "hover")

// Reports
sess.PrintRenderReport()
sess.PrintPhaseReport(deltas)
```

### PhaseDelta

```go
type PhaseDelta struct {
    NodeID      dom.NodeID
    NodeTag     string      // "div", "span", etc.
    NodeClass   string      // "box", "container", etc.
    OldStyle    uint32      // style count before
    NewStyle    uint32      // style count after
    OldLayout   uint32      // layout count before
    NewLayout   uint32      // layout count after
    OldPaint    uint32      // paint count before
    NewPaint    uint32      // paint count after
    StyleDelta  int32       // change in style count
    LayoutDelta int32       // change in layout count
    PaintDelta  int32       // change in paint count
    Reason      string      // "hover", "click", "select", etc.
}
```

## Common Patterns

### Pattern 1: Hover Causes Full Repaint

**Symptom:**
```
Phase offset after hover:
  nodes changed: 186
  most changed: node 139 paint=+71
```

**Cause:** Hover triggers `RestyleAndReflow()` which rebuilds entire display list.

**Fix:** Pre-compute damage rect, skip reflow for geometry-neutral changes.

### Pattern 2: Selection Causes Layout

**Symptom:**
```
Phase offset after select:
  nodes changed: 50
  most changed: node 42 layout=+10 paint=+25
```

**Cause:** Selection triggers reflow even though only paint should change.

**Fix:** Detect selection-only changes, skip layout pass.

### Pattern 3: Scroll Repaints Everything

**Symptom:**
```
Phase offset after scroll:
  nodes changed: 200
  most changed: node 1 paint=+100
```

**Cause:** Scroll invalidates all tiles instead of just damaged ones.

**Fix:** Use tile caching, only repaint exposed areas.

## Debugging Workflow

### Step 1: Baseline Measurement

```bash
# Test without interactions
go run ./cmd/test-render -phase testdata/render/debug-test.html
```

Note the initial render counts.

### Step 2: Test Hover

```bash
# Simulate hover interactions
go run ./cmd/debug-render -phase -frames 100 -scene hover
```

Check phase offset report.

### Step 3: Test Selection

```bash
go run ./cmd/debug-render -phase -frames 100 -scene select
```

### Step 4: Test Click/Focus

```bash
go run ./cmd/debug-render -phase -frames 100 -scene focus
```

### Step 5: Compare Results

**Good:**
- Hover: only hovered node changes (1-2 nodes)
- Selection: only selected text changes (5-10 nodes)
- Scroll: zero nodes change (viewport-only)

**Bad:**
- Hover: 100+ nodes change
- Selection: layout pass triggered
- Scroll: full repaint

### Step 6: Identify Hot Spots

Look at "Top 10 most-rendered nodes" in the report. These are your optimization targets.

## Performance Targets

| Interaction | Expected Nodes Changed | Expected Paint Delta |
|-------------|------------------------|----------------------|
| Hover       | 1-5                    | <10                  |
| Click       | 1-5                    | <10                  |
| Select      | 5-20                   | <50                  |
| Scroll      | 0                      | 0                    |
| Resize      | all                    | all (expected)       |

If your numbers are higher, you have a phase offset problem.

## Integration with Tests

```go
func TestHoverDoesNotCauseExcessiveRepaint(t *testing.T) {
    tracker := debug.NewPhaseTracker(true)
    sess := loadTestSession(t, "hover.html", tracker)
    
    // Baseline
    tracker.Snapshot()
    
    // Hover
    sess.SetHover(100, 100)
    sess.RestyleAndReflow(800)
    
    // Check phase offset
    deltas := tracker.DetectPhaseOffset(0, "hover")
    
    // Assert: only a few nodes should change
    if len(deltas) > 10 {
        t.Errorf("hover changed %d nodes, expected <10", len(deltas))
    }
    
    // Assert: paint delta should be small
    for _, d := range deltas {
        if d.PaintDelta > 20 {
            t.Errorf("node %d paint delta=%d, expected <20", d.NodeID, d.PaintDelta)
        }
    }
}
```

## Visual Debugging (Future)

When integrated with the debug overlay:

```bash
go run ./cmd/debug-render -overlay -phase testdata/render/debug-test.html
```

Will show:
- **Red numbers** on each element: paint count
- **Yellow highlight** on elements with phase offset
- **Green checkmark** on elements that didn't change

## Troubleshooting

### No phase tracking data

- Ensure `-phase` flag is set
- Check `tracker.Enabled = true`
- Verify `WithPhaseTracker(tracker)` option is passed to session

### All nodes show zero changes

- Interaction didn't trigger any rendering
- Check if `HasPseudoClassRule()` returns false
- Verify interaction actually happened

### Too many nodes changed

- This is the problem you're trying to fix!
- Look at "most changed" node
- Check what interaction triggered it
- Optimize to reduce phase offset

### Paint count is very high

- Element is being repainted excessively
- Check if damage rect is too large
- Verify tile caching is working
- Look for unnecessary `RestyleAndReflow()` calls

## See Also

- `docs/debug-tools.md` - General debug tools guide
- `internal/debug/phase_tracker.go` - Implementation
- `internal/engine/phase_tracking.go` - Session integration
- `cmd/test-render/main.go` - Example usage
