package engine

import (
	"sort"

	"github.com/vyquocvu/goosie/internal/dom"
)

// TestDriverTouchAction maps one phase of the WPT adapter's touch pointer onto
// the trusted touch events a real gesture raises: "down" puts a contact on the
// node under the point and dispatches touchstart, "move" slides the held contact
// and dispatches touchmove, "up" lifts it and dispatches touchend.
//
// The contact set lives on the session rather than in the page because a page may
// not decide what a generated gesture contains. A "move" before any "down" raises
// nothing: a browser sends no touchmove until a contact is on the screen, which is
// why resources/touching.js presses before it moves.
//
// The gesture's default action is deliberately absent. Chromium pans the nearest
// scrollable box under a touch drag; goosie does not model touch panning yet, and
// scrolling the viewport instead would be a guess rather than a measurement.
func (s *Session) TestDriverTouchAction(id, phase string, x, y float32) {
	s.opMu.Lock()
	rt := s.jsRT
	if rt == nil {
		s.opMu.Unlock()
		return
	}
	if s.touches == nil {
		s.touches = map[string]*dom.Touch{}
	}
	target := s.gestureTargetAt(x, y)
	touch := s.touches[id]
	switch phase {
	case "down":
		if touch != nil {
			// A second "down" for the same source re-targets the contact instead
			// of stacking a phantom finger on top of it.
			touch.Target = target
			touch.ClientX, touch.ClientY = float64(x), float64(y)
		} else {
			s.touchSeq++
			touch = &dom.Touch{Identifier: s.touchSeq, ClientX: float64(x), ClientY: float64(y), Target: target}
			s.touches[id] = touch
		}
		start := dom.NewTrustedTouchEvent("touchstart", target, s.heldTouchesLocked(), []*dom.Touch{touch})
		s.opMu.Unlock()
		rt.DispatchTrustedTouch(target, start)
	case "move":
		if touch == nil {
			s.opMu.Unlock()
			return
		}
		// A drag can slide off the node it started on, so the contact follows the
		// point while touchmove reports the held set unchanged.
		touch.Target = target
		touch.ClientX, touch.ClientY = float64(x), float64(y)
		move := dom.NewTrustedTouchEvent("touchmove", target, s.heldTouchesLocked(), []*dom.Touch{touch})
		s.opMu.Unlock()
		rt.DispatchTrustedTouch(target, move)
	case "up":
		if touch == nil {
			s.opMu.Unlock()
			return
		}
		touch.ClientX, touch.ClientY = float64(x), float64(y)
		// touchend reports the released contact only in changedTouches: every
		// contact still down stays in touches, and this one leaves it.
		delete(s.touches, id)
		end := dom.NewTrustedTouchEvent("touchend", target, s.heldTouchesLocked(), []*dom.Touch{touch})
		s.opMu.Unlock()
		rt.DispatchTrustedTouch(target, end)
	case "cancel":
		if touch == nil {
			s.opMu.Unlock()
			return
		}
		// touchcancel ends the contact: the cancelled touch point leaves the
		// active set and touches reports what remains.
		delete(s.touches, id)
		cancel := dom.NewTrustedTouchEvent("touchcancel", target, s.heldTouchesLocked(), []*dom.Touch{touch})
		s.opMu.Unlock()
		rt.DispatchTrustedTouch(target, cancel)
	default:
		s.opMu.Unlock()
	}
}

// heldTouchesLocked returns the contacts currently down, ordered by the
// identifier each was raised with so a generated gesture's touch lists read the
// same however the map iterates. The caller holds opMu.
func (s *Session) heldTouchesLocked() []*dom.Touch {
	if len(s.touches) == 0 {
		return nil
	}
	held := make([]*dom.Touch, 0, len(s.touches))
	for _, touch := range s.touches {
		held = append(held, touch)
	}
	sort.Slice(held, func(i, j int) bool { return held[i].Identifier < held[j].Identifier })
	return held
}
