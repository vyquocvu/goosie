package engine

import "github.com/vyquocvu/goosie/internal/dom"

// SetInspectHighlight outlines n in the next paint. The inspector uses it to
// show which node it is displaying without mutating the page: only the node is
// stored, and Paint re-resolves its box from the live arena, so a reflow cannot
// leave a stale highlight behind. A node with no box (detached or display:none)
// paints nothing.
func (s *Session) SetInspectHighlight(n *dom.Node) {
	s.opMu.Lock()
	s.inspectNode = n
	s.opMu.Unlock()
}

// ClearInspectHighlight removes the inspector overlay.
func (s *Session) ClearInspectHighlight() {
	s.opMu.Lock()
	s.inspectNode = nil
	s.opMu.Unlock()
}

// InspectHighlight returns the node currently outlined, or nil when none is.
func (s *Session) InspectHighlight() *dom.Node {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.inspectNode
}
