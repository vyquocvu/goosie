package engine

import (
	"sort"

	"github.com/vyquocvu/goosie/internal/dom"
)

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
		delete(s.touches, id)
		end := dom.NewTrustedTouchEvent("touchend", target, s.heldTouchesLocked(), []*dom.Touch{touch})
		s.opMu.Unlock()
		rt.DispatchTrustedTouch(target, end)
	case "cancel":
		if touch == nil {
			s.opMu.Unlock()
			return
		}
		delete(s.touches, id)
		cancel := dom.NewTrustedTouchEvent("touchcancel", target, s.heldTouchesLocked(), []*dom.Touch{touch})
		s.opMu.Unlock()
		rt.DispatchTrustedTouch(target, cancel)
	default:
		s.opMu.Unlock()
	}
}

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
