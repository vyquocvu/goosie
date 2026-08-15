package renderer

// MouseInputKind identifies the type of mouse input posted from the canvas
// widgets to the owning Tab, which routes it into the engine event loop.
type MouseInputKind uint8

const (
	// MouseInputMove is a pointer-move event. The loop's latest-wins
	// mouse slot collapses a burst into one drained position.
	MouseInputMove MouseInputKind = iota
	// MouseInputClick is a discrete pointer press/release (button 1 left,
	// 2 right). It is routed through the loop's ordered FIFO so click
	// intent stays ordered relative to keys.
	MouseInputClick
	// MouseInputLinkTap is a tap on a hyperlink carrying its resolved URL.
	// It dispatches navigation without a hit test.
	MouseInputLinkTap
)

// MouseInput is an immutable, UI-agnostic mouse event posted from the
// canvas. The renderer never dispatches directly when a poster is wired:
// the Tab owns the dispatch policy (coalescing, throttling, hit testing)
// and runs it in the event loop's drain.
type MouseInput struct {
	Kind MouseInputKind
	// X, Y is the pointer position in widget space; the drain adds the
	// current scroll offset to hit-test in content coordinates.
	X, Y float32
	// AbsX, AbsY is the canvas-absolute cursor position (context menus).
	AbsX, AbsY float32
	// Button is 1 for left click, 2 for right click (MouseInputClick).
	Button int
	// URL is the navigation target for MouseInputLinkTap.
	URL string
}

// mouseInputPoster routes canvas mouse events to the owning Tab. Nil when
// no owner is wired; widgets then fall back to their legacy direct
// dispatch so renderer-only usage (and tests) keeps working.
type mouseInputPoster func(MouseInput)

// postMouseInput forwards m to the wired poster, if any. It returns true
// when a poster consumed the event.
func (cr *CanvasRenderer) postMouseInput(m MouseInput) bool {
	if cr.mousePoster == nil {
		return false
	}
	cr.mousePoster(m)
	return true
}
