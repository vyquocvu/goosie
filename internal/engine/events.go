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
