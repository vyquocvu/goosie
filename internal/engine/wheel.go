package engine

import "github.com/vyquocvu/goosie/internal/dom"

func (s *Session) TestDriverWheelAction(x, y float32, deltaX, deltaY float64) {
	s.opMu.Lock()
	rt := s.jsRT
	s.opMu.Unlock()
	if rt == nil {
		return
	}
	target := s.gestureTargetAt(x, y)
	if rt.DispatchTrustedWheel(target, x, y, deltaX, deltaY) {
		rt.ScrollWindowBy(deltaX, deltaY)
	}
}

func (s *Session) gestureTargetAt(x, y float32) *dom.Node {
	target := s.nodeAt(x, y)
	for target != nil && !target.Element() {
		target = target.Parent
	}
	if target != nil {
		return target
	}
	if s.Doc == nil {
		return nil
	}
	if s.Doc.Body != nil {
		return s.Doc.Body
	}
	return s.Doc.HTML
}
