// Package debug provides rendering pipeline debugging and visualization tools.
package debug

import (
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/vyquocvu/goosie/internal/dom"
)

// RenderCounts tracks how many times each pipeline stage ran for a node.
type RenderCounts struct {
	StyleCount  uint32 // Number of times style was resolved for this node
	LayoutCount uint32 // Number of times layout was computed for this node
	PaintCount  uint32 // Number of times this node was painted
}

// PhaseDelta records a change in a node's render state between frames.
type PhaseDelta struct {
	NodeID      dom.NodeID
	NodeTag     string
	NodeClass   string
	OldStyle    uint32
	NewStyle    uint32
	OldLayout   uint32
	NewLayout   uint32
	OldPaint    uint32
	NewPaint    uint32
	StyleDelta  int32
	LayoutDelta int32
	PaintDelta  int32
	Reason      string // "hover", "click", "select", "scroll", "js", "animation"
}

// PhaseTracker tracks render counts per node and detects phase offsets.
type PhaseTracker struct {
	Enabled    bool
	counts     map[dom.NodeID]*RenderCounts
	snapshots  []map[dom.NodeID]RenderCounts // historical snapshots
	nodeInfo   map[dom.NodeID]NodeInfo       // tag, class, etc.
	mu         sync.Mutex
	maxHistory int
}

// NodeInfo holds metadata about a node for debugging.
type NodeInfo struct {
	Tag   string
	Class string
	ID    string
}

// NewPhaseTracker returns a tracker for render counts and phase detection.
func NewPhaseTracker(enabled bool) *PhaseTracker {
	return &PhaseTracker{
		Enabled:    enabled,
		counts:     make(map[dom.NodeID]*RenderCounts),
		snapshots:  make([]map[dom.NodeID]RenderCounts, 0, 10),
		nodeInfo:   make(map[dom.NodeID]NodeInfo),
		maxHistory: 10,
	}
}

// TrackStyle increments the style count for a node.
func (t *PhaseTracker) TrackStyle(nodeID dom.NodeID) {
	if !t.Enabled {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.counts[nodeID] == nil {
		t.counts[nodeID] = &RenderCounts{}
	}
	t.counts[nodeID].StyleCount++
}

// TrackLayout increments the layout count for a node.
func (t *PhaseTracker) TrackLayout(nodeID dom.NodeID) {
	if !t.Enabled {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.counts[nodeID] == nil {
		t.counts[nodeID] = &RenderCounts{}
	}
	t.counts[nodeID].LayoutCount++
}

// TrackPaint increments the paint count for a node.
func (t *PhaseTracker) TrackPaint(nodeID dom.NodeID) {
	if !t.Enabled {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.counts[nodeID] == nil {
		t.counts[nodeID] = &RenderCounts{}
	}
	t.counts[nodeID].PaintCount++
}

// TrackStyleBatch increments style count for multiple nodes.
func (t *PhaseTracker) TrackStyleBatch(nodeIDs []dom.NodeID) {
	if !t.Enabled {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, id := range nodeIDs {
		if t.counts[id] == nil {
			t.counts[id] = &RenderCounts{}
		}
		t.counts[id].StyleCount++
	}
}

// TrackLayoutBatch increments layout count for multiple nodes.
func (t *PhaseTracker) TrackLayoutBatch(nodeIDs []dom.NodeID) {
	if !t.Enabled {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, id := range nodeIDs {
		if t.counts[id] == nil {
			t.counts[id] = &RenderCounts{}
		}
		t.counts[id].LayoutCount++
	}
}

// TrackPaintBatch increments paint count for multiple nodes.
func (t *PhaseTracker) TrackPaintBatch(nodeIDs []dom.NodeID) {
	if !t.Enabled {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, id := range nodeIDs {
		if t.counts[id] == nil {
			t.counts[id] = &RenderCounts{}
		}
		t.counts[id].PaintCount++
	}
}

// RegisterNode stores metadata about a node for debugging output.
func (t *PhaseTracker) RegisterNode(nodeID dom.NodeID, tag, class, id string) {
	if !t.Enabled {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	t.nodeInfo[nodeID] = NodeInfo{Tag: tag, Class: class, ID: id}
}

// Snapshot saves the current render counts for later comparison.
func (t *PhaseTracker) Snapshot() {
	if !t.Enabled {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	snapshot := make(map[dom.NodeID]RenderCounts, len(t.counts))
	for id, counts := range t.counts {
		snapshot[id] = *counts
	}

	t.snapshots = append(t.snapshots, snapshot)
	if len(t.snapshots) > t.maxHistory {
		t.snapshots = t.snapshots[1:]
	}
}

// DetectPhaseOffset compares current counts with a snapshot and returns deltas.
func (t *PhaseTracker) DetectPhaseOffset(snapshotIdx int, reason string) []PhaseDelta {
	if !t.Enabled {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	if snapshotIdx < 0 || snapshotIdx >= len(t.snapshots) {
		return nil
	}

	snapshot := t.snapshots[snapshotIdx]
	deltas := make([]PhaseDelta, 0)

	// Check all current nodes
	for id, current := range t.counts {
		old, existed := snapshot[id]
		if !existed {
			// New node
			old = RenderCounts{}
		}

		styleDelta := int32(current.StyleCount) - int32(old.StyleCount)
		layoutDelta := int32(current.LayoutCount) - int32(old.LayoutCount)
		paintDelta := int32(current.PaintCount) - int32(old.PaintCount)

		// Only report if something changed
		if styleDelta != 0 || layoutDelta != 0 || paintDelta != 0 {
			info := t.nodeInfo[id]
			deltas = append(deltas, PhaseDelta{
				NodeID:      id,
				NodeTag:     info.Tag,
				NodeClass:   info.Class,
				OldStyle:    old.StyleCount,
				NewStyle:    current.StyleCount,
				OldLayout:   old.LayoutCount,
				NewLayout:   current.LayoutCount,
				OldPaint:    old.PaintCount,
				NewPaint:    current.PaintCount,
				StyleDelta:  styleDelta,
				LayoutDelta: layoutDelta,
				PaintDelta:  paintDelta,
				Reason:      reason,
			})
		}
	}

	// Sort by total delta (most changed first)
	sort.Slice(deltas, func(i, j int) bool {
		totalI := abs(deltas[i].StyleDelta) + abs(deltas[i].LayoutDelta) + abs(deltas[i].PaintDelta)
		totalJ := abs(deltas[j].StyleDelta) + abs(deltas[j].LayoutDelta) + abs(deltas[j].PaintDelta)
		return totalI > totalJ
	})

	return deltas
}

// GetCounts returns the current render counts for a node.
func (t *PhaseTracker) GetCounts(nodeID dom.NodeID) RenderCounts {
	t.mu.Lock()
	defer t.mu.Unlock()

	if counts, ok := t.counts[nodeID]; ok {
		return *counts
	}
	return RenderCounts{}
}

// GetAllCounts returns render counts for all tracked nodes.
func (t *PhaseTracker) GetAllCounts() map[dom.NodeID]RenderCounts {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := make(map[dom.NodeID]RenderCounts, len(t.counts))
	for id, counts := range t.counts {
		result[id] = *counts
	}
	return result
}

// PrintReport prints a summary of render counts and phase offsets.
func (t *PhaseTracker) PrintReport() {
	if !t.Enabled {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	fmt.Fprintf(os.Stderr, "\n=== Render Count Report ===\n")
	fmt.Fprintf(os.Stderr, "Tracked nodes: %d\n", len(t.counts))
	fmt.Fprintf(os.Stderr, "Snapshots: %d\n", len(t.snapshots))

	// Aggregate stats
	var totalStyle, totalLayout, totalPaint uint64
	for _, counts := range t.counts {
		totalStyle += uint64(counts.StyleCount)
		totalLayout += uint64(counts.LayoutCount)
		totalPaint += uint64(counts.PaintCount)
	}

	fmt.Fprintf(os.Stderr, "\nAggregate counts:\n")
	fmt.Fprintf(os.Stderr, "  Style:  %d\n", totalStyle)
	fmt.Fprintf(os.Stderr, "  Layout: %d\n", totalLayout)
	fmt.Fprintf(os.Stderr, "  Paint:  %d\n", totalPaint)

	// Top 10 most-rendered nodes
	fmt.Fprintf(os.Stderr, "\nTop 10 most-rendered nodes:\n")
	fmt.Fprintf(os.Stderr, "%-10s %-15s %-20s %8s %8s %8s\n",
		"NodeID", "Tag", "Class", "Style", "Layout", "Paint")
	fmt.Fprintf(os.Stderr, "%s\n", repeat("-", 75))

	type nodeStats struct {
		id     dom.NodeID
		info   NodeInfo
		counts RenderCounts
	}

	stats := make([]nodeStats, 0, len(t.counts))
	for id, counts := range t.counts {
		stats = append(stats, nodeStats{id, t.nodeInfo[id], *counts})
	}

	sort.Slice(stats, func(i, j int) bool {
		totalI := uint64(stats[i].counts.StyleCount + stats[i].counts.LayoutCount + stats[i].counts.PaintCount)
		totalJ := uint64(stats[j].counts.StyleCount + stats[j].counts.LayoutCount + stats[j].counts.PaintCount)
		return totalI > totalJ
	})

	limit := 10
	if len(stats) < limit {
		limit = len(stats)
	}

	for i := 0; i < limit; i++ {
		s := stats[i]
		fmt.Fprintf(os.Stderr, "%-10d %-15s %-20s %8d %8d %8d\n",
			s.id, s.info.Tag, s.info.Class,
			s.counts.StyleCount, s.counts.LayoutCount, s.counts.PaintCount)
	}

	fmt.Fprintf(os.Stderr, "===========================\n\n")
}

// PrintPhaseDeltas prints phase offset deltas.
func (t *PhaseTracker) PrintPhaseDeltas(deltas []PhaseDelta) {
	if !t.Enabled || len(deltas) == 0 {
		return
	}

	fmt.Fprintf(os.Stderr, "\n=== Phase Offset Report ===\n")
	fmt.Fprintf(os.Stderr, "Nodes with changes: %d\n", len(deltas))

	if len(deltas) > 0 {
		fmt.Fprintf(os.Stderr, "Reason: %s\n", deltas[0].Reason)
	}

	fmt.Fprintf(os.Stderr, "\n%-10s %-15s %-20s %10s %10s %10s\n",
		"NodeID", "Tag", "Class", "Style Δ", "Layout Δ", "Paint Δ")
	fmt.Fprintf(os.Stderr, "%s\n", repeat("-", 80))

	limit := 20
	if len(deltas) < limit {
		limit = len(deltas)
	}

	for i := 0; i < limit; i++ {
		d := deltas[i]
		fmt.Fprintf(os.Stderr, "%-10d %-15s %-20s %+10d %+10d %+10d\n",
			d.NodeID, d.NodeTag, d.NodeClass,
			d.StyleDelta, d.LayoutDelta, d.PaintDelta)
	}

	if len(deltas) > limit {
		fmt.Fprintf(os.Stderr, "... and %d more nodes\n", len(deltas)-limit)
	}

	fmt.Fprintf(os.Stderr, "===========================\n\n")
}

// Reset clears all tracked data.
func (t *PhaseTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.counts = make(map[dom.NodeID]*RenderCounts)
	t.snapshots = t.snapshots[:0]
	t.nodeInfo = make(map[dom.NodeID]NodeInfo)
}

func abs(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}
