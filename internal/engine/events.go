package engine

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

func (s *Session) FireScrollEvent() {
	if s.Doc == nil {
		return
	}
	s.opMu.Lock()
	rt := s.jsRT
	s.opMu.Unlock()
	if rt != nil {
		rt.QueueViewportScroll()
		return
	}
	ev := dom.NewTrustedEvent("scroll", true, false)
	dom.DispatchEvent(&s.Doc.Node, ev)
}

func (s *Session) FireResizeEvent() {
	if s.Doc == nil {
		return
	}
	ev := dom.NewTrustedEvent("resize", false, false)
	dom.DispatchEvent(&s.Doc.Node, ev)
}

type heldKey struct {
	key       string
	node      *dom.Node
	prevented bool
}

func (s *Session) KeyDown(key string, shift, ctrl, alt, meta bool) bool {
	if s.Doc == nil {
		return false
	}
	s.pendingKey = nil
	s.clearKeyboardActiveLocked()
	active := s.keyboardActivationTarget(key)
	if active != nil {
		s.keyboardActive = active
		s.keyboardActiveMarked = markKeyboardActive(active)
	}

	down := dom.NewKeyboardEvent("keydown", true, true, key, "")
	down.ShiftKey, down.CtrlKey, down.AltKey, down.MetaKey = shift, ctrl, alt, meta
	prevented := !dom.DispatchEvent(s.keyTarget(), &down.Event)
	if active != nil {
		s.pendingKey = &heldKey{key: key, node: active, prevented: prevented}
	}
	if prevented {
		s.clearKeyboardActiveLocked()
		return false
	}
	if active != nil {
		return false
	}
	return s.keyDefaultAction(key, shift)
}

func (s *Session) KeyUp(key string, shift, ctrl, alt, meta bool) bool {
	if s.Doc == nil {
		return false
	}
	pending := s.pendingKey
	s.pendingKey = nil

	up := dom.NewKeyboardEvent("keyup", true, true, key, "")
	up.ShiftKey, up.CtrlKey, up.AltKey, up.MetaKey = shift, ctrl, alt, meta
	dom.DispatchEvent(s.keyTarget(), &up.Event)

	activated := pending != nil && pending.key == key && !pending.prevented &&
		pending.node == s.focus && s.activateKeyboardTarget(pending.node)
	s.clearKeyboardActiveLocked()
	return activated
}

func (s *Session) HasHeldKey(key string) bool {
	return s != nil && s.pendingKey != nil && s.pendingKey.key == key
}

func (s *Session) HeldKeyPrevented(key string) bool {
	return s != nil && s.pendingKey != nil && s.pendingKey.key == key && s.pendingKey.prevented
}

func (s *Session) PressKey(key string, shift, ctrl, alt, meta bool) bool {
	down := s.KeyDown(key, shift, ctrl, alt, meta)
	up := s.KeyUp(key, shift, ctrl, alt, meta)
	return down || up
}

func (s *Session) keyDefaultAction(key string, shift bool) bool {
	if isRangeInput(s.focus) {
		var action EditAction
		switch key {
		case "ArrowLeft", "ArrowDown":
			action = EditLeft
		case "ArrowRight", "ArrowUp":
			action = EditRight
		case "Home":
			action = EditHome
		case "End":
			action = EditEnd
		case "PageUp":
			action = EditPageUp
		case "PageDown":
			action = EditPageDown
		default:
			action = EditRune
		}
		if action != EditRune {
			s.opMu.Lock()
			changed := s.adjustRangeLocked(s.focus, action)
			s.opMu.Unlock()
			return changed
		}
	}
	switch key {
	case "Tab":
		if shift {
			s.FocusPrev()
		} else {
			s.FocusNext()
		}
	case "Enter":
		if n := s.focus; n != nil && IsActivatable(n) {
			return s.activateKeyboardTarget(n)
		}
	case " ":
		if n := s.focus; IsSpaceActivatable(n) {
			return s.activateKeyboardTarget(n)
		}
	case "Escape":
		if s.selectPopupNode() != nil {
			s.CloseSelectPopup()
			return true
		}
		s.Edit(EditEscape, 0x1b)
	}
	return false
}

func (s *Session) keyboardActivationTarget(key string) *dom.Node {
	n := s.focus
	if n == nil || dom.IsFormControlDisabled(n) {
		return nil
	}
	if key == "Enter" && IsActivatable(n) || key == " " && IsSpaceActivatable(n) {
		return n
	}
	return nil
}

func IsSpaceActivatable(n *dom.Node) bool {
	if n == nil || !n.Element() || dom.IsFormControlDisabled(n) {
		return false
	}
	switch n.Data {
	case "button", "summary":
		return true
	case "input":
		switch strings.ToLower(n.GetAttribute("type")) {
		case "button", "submit", "reset", "checkbox", "radio", "image", "file":
			return true
		}
	}
	return false
}

func markKeyboardActive(n *dom.Node) []*dom.Node {
	var marked []*dom.Node
	for cur := n; cur != nil && cur.Element(); cur = cur.Parent {
		if !css.IsActive(cur) {
			css.SetActive(cur, true)
			marked = append(marked, cur)
		}
	}
	return marked
}

func (s *Session) SetKeyboardActive(n *dom.Node, active bool) {
	if s == nil {
		return
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if active {
		if n == nil || !n.Element() || s.keyboardActive == n {
			return
		}
		s.clearKeyboardActiveLocked()
		s.keyboardActive = n
		for cur := n; cur != nil && cur.Element(); cur = cur.Parent {
			if !css.IsActive(cur) {
				css.SetActive(cur, true)
				s.keyboardActiveMarked = append(s.keyboardActiveMarked, cur)
			}
		}
		return
	}
	if n != nil && s.keyboardActive != n {
		return
	}
	s.clearKeyboardActiveLocked()
}

func (s *Session) clearKeyboardActiveLocked() {
	for _, node := range s.keyboardActiveMarked {
		css.SetActive(node, false)
	}
	s.keyboardActive = nil
	s.keyboardActiveMarked = nil
}

func (s *Session) activateKeyboardTarget(n *dom.Node) bool {
	click := dom.NewTrustedEvent("click", true, true)
	if !dom.DispatchEvent(n, click) {
		return false
	}
	s.runClickDefault(n)
	return true
}

func (s *Session) keyTarget() *dom.Node {
	if s.focus != nil {
		return s.focus
	}
	if s.Doc.Body != nil {
		return s.Doc.Body
	}
	return &s.Doc.Node
}
