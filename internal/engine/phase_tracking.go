package engine

import (
	"github.com/vyquocvu/goosie/internal/debug"
	"github.com/vyquocvu/goosie/internal/dom"
)

// PhaseTracker returns the session's phase tracker, or nil if tracking is disabled.
func (s *Session) PhaseTracker() *debug.PhaseTracker {
	if s == nil {
		return nil
	}
	if tracker, ok := s.phaseTracker.(*debug.PhaseTracker); ok {
		return tracker
	}
	return nil
}

// TrackNodeRender records that a node was processed by a pipeline stage.
// stage is "style", "layout", or "paint".
func (s *Session) TrackNodeRender(nodeID dom.NodeID, stage string) {
	tracker := s.PhaseTracker()
	if tracker == nil {
		return
	}

	switch stage {
	case "style":
		tracker.TrackStyle(nodeID)
	case "layout":
		tracker.TrackLayout(nodeID)
	case "paint":
		tracker.TrackPaint(nodeID)
	}
}

// TrackNodeRenderBatch records that multiple nodes were processed by a pipeline stage.
func (s *Session) TrackNodeRenderBatch(nodeIDs []dom.NodeID, stage string) {
	tracker := s.PhaseTracker()
	if tracker == nil {
		return
	}

	switch stage {
	case "style":
		tracker.TrackStyleBatch(nodeIDs)
	case "layout":
		tracker.TrackLayoutBatch(nodeIDs)
	case "paint":
		tracker.TrackPaintBatch(nodeIDs)
	}
}

// RegisterNodeForDebug stores metadata about a node for debug output.
func (s *Session) RegisterNodeForDebug(node *dom.Node) {
	if node == nil {
		return
	}
	tracker := s.PhaseTracker()
	if tracker == nil {
		return
	}

	tag := node.Data
	class := node.GetAttribute("class")
	id := node.GetAttribute("id")
	tracker.RegisterNode(node.ID, tag, class, id)
}

// SnapshotRenderCounts saves the current render counts for later comparison.
func (s *Session) SnapshotRenderCounts() {
	tracker := s.PhaseTracker()
	if tracker == nil {
		return
	}
	tracker.Snapshot()
}

// DetectPhaseOffset compares current render counts with a snapshot and returns deltas.
// snapshotIdx is the index of the snapshot to compare against (0 = most recent before current).
// reason describes what interaction triggered the comparison.
func (s *Session) DetectPhaseOffset(snapshotIdx int, reason string) []debug.PhaseDelta {
	tracker := s.PhaseTracker()
	if tracker == nil {
		return nil
	}
	return tracker.DetectPhaseOffset(snapshotIdx, reason)
}

// PrintRenderReport prints a summary of render counts per node.
func (s *Session) PrintRenderReport() {
	tracker := s.PhaseTracker()
	if tracker == nil {
		return
	}
	tracker.PrintReport()
}

// PrintPhaseReport prints phase offset deltas.
func (s *Session) PrintPhaseReport(deltas []debug.PhaseDelta) {
	tracker := s.PhaseTracker()
	if tracker == nil {
		return
	}
	tracker.PrintPhaseDeltas(deltas)
}

// GetNodeRenderCounts returns the render counts for a specific node.
func (s *Session) GetNodeRenderCounts(nodeID dom.NodeID) debug.RenderCounts {
	tracker := s.PhaseTracker()
	if tracker == nil {
		return debug.RenderCounts{}
	}
	return tracker.GetCounts(nodeID)
}

// GetAllNodeRenderCounts returns render counts for all tracked nodes.
func (s *Session) GetAllNodeRenderCounts() map[dom.NodeID]debug.RenderCounts {
	tracker := s.PhaseTracker()
	if tracker == nil {
		return nil
	}
	return tracker.GetAllCounts()
}

// ResetRenderCounts clears all tracked render counts.
func (s *Session) ResetRenderCounts() {
	tracker := s.PhaseTracker()
	if tracker == nil {
		return
	}
	tracker.Reset()
}
