package engine

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

// FireScrollEvent tells the page that the document viewport moved. The runtime
// queues one event per turn for the viewport, delivered at the next frame flush
// the way a script-initiated scroll is, so several viewport moves in one turn
// reach the page as a single event.
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
	// Without a script runtime there is no frame flush to drain the queue, so
	// the document's own listeners are served the event directly.
	ev := dom.NewTrustedEvent("scroll", true, false)
	dom.DispatchEvent(&s.Doc.Node, ev)
}

// FireResizeEvent dispatches a "resize" event on the window.
// This should be called when the viewport size changes.
func (s *Session) FireResizeEvent() {
	if s.Doc == nil {
		return
	}
	// Resize events fire on the window object. Since we don't have a full
	// window object in the DOM, we fire on the document which is the closest
	// equivalent in our model.
	ev := dom.NewEvent("resize", false, false)
	dom.DispatchEvent(&s.Doc.Node, ev)
}

// PressKey dispatches a keydown, runs the key's default action unless a
// listener called preventDefault, then dispatches keyup. The keyup target is
// re-read after the default action so that a focus move (Tab) lands keyup on
// the newly focused element, matching Chromium's ordering.
//
// Default actions implemented here: Tab/Shift+Tab cycle focus; Enter activates
// the focused control; Space activates buttons and button-like inputs.
// Printable-key text editing is handled separately by the Edit path, not here.
// PressKey reports whether the key's activation default was performed. The
// result lets the browser shell apply defaults such as following an anchor
// after DOM click listeners had the chance to cancel it.
func (s *Session) PressKey(key string, shift, ctrl, alt, meta bool) bool {
	if s.Doc == nil {
		return false
	}
	active := s.keyboardActivationTarget(key)
	markedActive := markKeyboardActive(active)
	defer func() {
		for _, n := range markedActive {
			css.SetActive(n, false)
		}
	}()

	down := dom.NewKeyboardEvent("keydown", true, true, key, "")
	down.ShiftKey, down.CtrlKey, down.AltKey, down.MetaKey = shift, ctrl, alt, meta
	prevented := !dom.DispatchEvent(s.keyTarget(), &down.Event)

	activated := false
	if !prevented {
		activated = s.keyDefaultAction(key, shift)
	}

	up := dom.NewKeyboardEvent("keyup", true, true, key, "")
	up.ShiftKey, up.CtrlKey, up.AltKey, up.MetaKey = shift, ctrl, alt, meta
	dom.DispatchEvent(s.keyTarget(), &up.Event)
	return activated
}

// keyDefaultAction performs the default action of a key that was not
// preventDefault-ed. It reports whether the key performed an activation.
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
		s.Edit(EditEscape, 0x1b)
	}
	return false
}

// keyboardActivationTarget reports the focused element that is pressed during
// a keyboard activation. The state spans keydown, the default action, and
// keyup so event handlers observe :active throughout the key operation.
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

// IsSpaceActivatable reports whether Space invokes the element's keyboard
// activation behavior. Links are intentionally excluded because Space remains
// a page-scroll key when a link has focus.
func IsSpaceActivatable(n *dom.Node) bool {
	if n == nil || !n.Element() || dom.IsFormControlDisabled(n) {
		return false
	}
	switch n.Data {
	case "button", "summary":
		return true
	case "input":
		switch strings.ToLower(n.GetAttribute("type")) {
		case "button", "submit", "reset", "checkbox", "radio", "image":
			return true
		}
	}
	return false
}

// markKeyboardActive sets :active on the target and its element ancestors,
// returning only nodes whose state this key operation introduced so a
// pre-existing pointer activation is not cleared on keyup.
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

// SetKeyboardActive tracks the :active state of a control while a host input
// adapter holds Space. It preserves active state already owned by pointer
// input, matching the overlap behavior used by PressKey.
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
	click := dom.NewEvent("click", true, true)
	if !dom.DispatchEvent(n, click) {
		return false
	}
	s.runClickDefault(n)
	return true
}

// keyTarget returns the node keyboard events target: the focused control, else
// the body, else the document node.
func (s *Session) keyTarget() *dom.Node {
	if s.focus != nil {
		return s.focus
	}
	if s.Doc.Body != nil {
		return s.Doc.Body
	}
	return &s.Doc.Node
}
