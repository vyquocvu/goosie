package engine

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

// FireScrollEvent dispatches a "scroll" event on the document.
// This should be called when the viewport scroll position changes.
func (s *Session) FireScrollEvent() {
	if s.Doc == nil {
		return
	}
	// Scroll events fire on the document and bubble to window.
	// They do not have a specific target element.
	ev := dom.NewEvent("scroll", false, false)
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
	if n == nil || n.HasAttribute("disabled") {
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
	if n == nil || !n.Element() || n.HasAttribute("disabled") {
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
