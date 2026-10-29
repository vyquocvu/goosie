package engine

import "github.com/vyquocvu/goosie/internal/dom"

// TestDriverWheelAction maps the WPT adapter's wheel "scroll" action onto the
// same gesture path native wheel input takes: the page receives the generated
// events at the point under the cursor, and the viewport moves only when the
// gesture survives them. A page that calls preventDefault(), or that registers
// only passive listeners, cannot be scrolled through this entry point.
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

// gestureTargetAt resolves the node a generated gesture targets at a document
// point. Hit testing finds the innermost box, whose element ancestor is the
// event target; a point outside every box still belongs to the page, so the
// gesture falls back to the viewport's scrolling element. Wheel and touch
// gestures both enter this way.
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
