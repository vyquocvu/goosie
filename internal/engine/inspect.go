package engine

import "github.com/vyquocvu/goosie/internal/dom"

func (s *Session) SetInspectHighlight(n *dom.Node) {
	s.opMu.Lock()
	s.inspectNode = n
	s.opMu.Unlock()
}

func (s *Session) ClearInspectHighlight() {
	s.opMu.Lock()
	s.inspectNode = nil
	s.opMu.Unlock()
}

func (s *Session) InspectHighlight() *dom.Node {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.inspectNode
}
