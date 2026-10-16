package engine

import (
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
// the focused control. Printable-key text editing is handled separately by the
// Edit path, not here.
func (s *Session) PressKey(key string, shift, ctrl, alt, meta bool) {
	if s.Doc == nil {
		return
	}
	down := dom.NewKeyboardEvent("keydown", true, true, key, "")
	down.ShiftKey, down.CtrlKey, down.AltKey, down.MetaKey = shift, ctrl, alt, meta
	prevented := !dom.DispatchEvent(s.keyTarget(), &down.Event)

	if !prevented {
		s.keyDefaultAction(key, shift)
	}

	up := dom.NewKeyboardEvent("keyup", true, true, key, "")
	up.ShiftKey, up.CtrlKey, up.AltKey, up.MetaKey = shift, ctrl, alt, meta
	dom.DispatchEvent(s.keyTarget(), &up.Event)
}

// keyDefaultAction performs the default action of a key that was not
// preventDefault-ed. Only keys with an engine default action are handled.
func (s *Session) keyDefaultAction(key string, shift bool) {
	switch key {
	case "Tab":
		if shift {
			s.FocusPrev()
		} else {
			s.FocusNext()
		}
	case "Enter":
		if n := s.focus; n != nil && IsActivatable(n) {
			s.runClickDefault(n)
		}
	}
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
