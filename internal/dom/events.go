package dom

import (
	"reflect"
	"sync"
	"time"
)

// funcEqual reports whether two function values refer to the same function.
// Go does not allow direct comparison of function values with ==, so we
// compare their code pointers via reflect.
func funcEqual(a, b func(*Event)) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

// EventPhase identifies the current phase of event propagation.
type EventPhase int

const (
	NonePhase EventPhase = iota
	CapturingPhase
	AtTarget
	BubblingPhase
)

// Event is a DOM event. It carries a type, propagation state, and references
// to the target and current-target nodes.
type Event struct {
	Type             string
	target           *Node
	currentTarget    *Node
	phase            EventPhase
	bubbles          bool
	cancelable       bool
	defaultPrevented bool
	stopped          bool // stopPropagation called
	immediateStopped bool // stopImmediatePropagation called
	timeStamp        time.Time

	// payload points at the concrete subtype (e.g. *KeyboardEvent) that embeds
	// this Event. Listeners and dispatch only ever pass *Event, so a type
	// assertion on that pointer cannot recover the subtype; this back-pointer is
	// how the JS bridge reads key/mouse fields. Constructors that build a
	// subtype set it; a plain NewEvent leaves it nil.
	payload any
}

// NewEvent creates a new Event with the given type and options.
func NewEvent(typ string, bubbles, cancelable bool) *Event {
	return &Event{
		Type:       typ,
		bubbles:    bubbles,
		cancelable: cancelable,
		timeStamp:  time.Now(),
	}
}

// StopPropagation prevents further propagation of the event.
func (e *Event) StopPropagation() {
	e.stopped = true
}

// StopImmediatePropagation prevents further propagation and any other
// listeners on the same target from firing.
func (e *Event) StopImmediatePropagation() {
	e.stopped = true
	e.immediateStopped = true
}

// PreventDefault marks the event as cancelled, preventing the default action.
func (e *Event) PreventDefault() {
	if e.cancelable {
		e.defaultPrevented = true
	}
}

// Target returns the node to which the event was originally dispatched.
func (e *Event) Target() *Node { return e.target }

// CurrentTarget returns the node whose listeners are currently being invoked.
func (e *Event) CurrentTarget() *Node { return e.currentTarget }

// Phase returns the current propagation phase.
func (e *Event) Phase() EventPhase { return e.phase }

// Bubbles reports whether the event bubbles up through the tree.
func (e *Event) Bubbles() bool { return e.bubbles }

// Cancelable reports whether the event's default action can be prevented.
func (e *Event) Cancelable() bool { return e.cancelable }

// DefaultPrevented reports whether PreventDefault was called.
func (e *Event) DefaultPrevented() bool { return e.defaultPrevented }

// TimeStamp returns the time at which the event was created.
func (e *Event) TimeStamp() time.Time { return e.timeStamp }

// MouseEvent is an event for mouse interactions.
type MouseEvent struct {
	Event
	ClientX  int32
	ClientY  int32
	ScreenX  int32
	ScreenY  int32
	Button   int // 0=left, 1=middle, 2=right
	AltKey   bool
	CtrlKey  bool
	ShiftKey bool
	MetaKey  bool
}

// NewMouseEvent creates a new mouse event with the given parameters.
func NewMouseEvent(typ string, bubbles, cancelable bool, clientX, clientY, screenX, screenY int32, button int) *MouseEvent {
	me := &MouseEvent{
		Event: Event{
			Type:       typ,
			bubbles:    bubbles,
			cancelable: cancelable,
			timeStamp:  time.Now(),
		},
		ClientX: clientX,
		ClientY: clientY,
		ScreenX: screenX,
		ScreenY: screenY,
		Button:  button,
	}
	me.Event.payload = me
	return me
}

// KeyboardEvent is an event for keyboard interactions.
type KeyboardEvent struct {
	Event
	Key      string
	Code     string
	AltKey   bool
	CtrlKey  bool
	ShiftKey bool
	MetaKey  bool
	Repeat   bool
}

// NewKeyboardEvent creates a new keyboard event with the given parameters.
func NewKeyboardEvent(typ string, bubbles, cancelable bool, key, code string) *KeyboardEvent {
	ke := &KeyboardEvent{
		Event: Event{
			Type:       typ,
			bubbles:    bubbles,
			cancelable: cancelable,
			timeStamp:  time.Now(),
		},
		Key:  key,
		Code: code,
	}
	ke.Event.payload = ke
	return ke
}

// AsKeyboard returns the concrete *KeyboardEvent behind this event, or nil when
// the event is not a keyboard event. Listeners receive a *Event, so the subtype
// fields are recovered through the payload back-pointer rather than a type
// assertion on the dispatched pointer.
func (e *Event) AsKeyboard() *KeyboardEvent {
	ke, _ := e.payload.(*KeyboardEvent)
	return ke
}

// AsMouse returns the concrete *MouseEvent behind this event, or nil when the
// event is not a mouse event.
func (e *Event) AsMouse() *MouseEvent {
	me, _ := e.payload.(*MouseEvent)
	return me
}

// EventListener is a registered callback for a specific event type.
type EventListener struct {
	Type     string
	Callback func(*Event)
	Capture  bool
	dedupKey interface{} // when non-nil, used for dedup instead of Callback pointer
}

// ---------------------------------------------------------------------------
// Listener registry
// ---------------------------------------------------------------------------
//
// listeners stores event listeners per node. The integration step will move
// these to a field on Node itself. A pointer to sync.Map is used so that the
// package-level variable is never reassigned (only method calls through the
// pointer), satisfying the no-mutable-globals architecture rule.

var listenerStore = &sync.Map{} // map[*Node][]*EventListener

// getListeners returns the listener slice for a node.
func getListeners(n *Node) []*EventListener {
	v, _ := listenerStore.Load(n)
	if v == nil {
		return nil
	}
	return v.([]*EventListener)
}

// setListeners replaces the listener slice for a node.
func setListeners(n *Node, ls []*EventListener) {
	if len(ls) == 0 {
		listenerStore.Delete(n)
	} else {
		listenerStore.Store(n, ls)
	}
}

// AddEventListener registers a callback for the given event type on node n.
// If capture is true the listener fires during the capture phase; otherwise it
// fires during the bubble phase (or at-target for both).
func AddEventListener(n *Node, typ string, callback func(*Event), capture bool) {
	ls := getListeners(n)
	// Reject exact duplicates (same type, callback pointer, and capture flag).
	for _, l := range ls {
		if l.Type == typ && listenerMatches(l, callback, nil, capture) {
			return
		}
	}
	ls = append(ls, &EventListener{Type: typ, Callback: callback, Capture: capture})
	setListeners(n, ls)
}

// AddEventListenerWithKey is like AddEventListener but uses key for duplicate
// detection instead of the callback pointer. This is needed when the callback
// is a Go closure wrapping a higher-level function reference (e.g. a goja
// function): Go closures with identical code but different captures share the
// same code pointer, so funcEqual cannot distinguish them.
func AddEventListenerWithKey(n *Node, typ string, callback func(*Event), capture bool, key interface{}) {
	ls := getListeners(n)
	for _, l := range ls {
		if l.Type == typ && listenerMatches(l, callback, key, capture) {
			return
		}
	}
	ls = append(ls, &EventListener{Type: typ, Callback: callback, Capture: capture, dedupKey: key})
	setListeners(n, ls)
}

// listenerMatches reports whether listener l matches the given dedup criteria.
// When key is non-nil, comparison uses the listener's dedupKey; otherwise it
// falls back to funcEqual on the callback.
func listenerMatches(l *EventListener, callback func(*Event), key interface{}, capture bool) bool {
	if l.Capture != capture {
		return false
	}
	if key != nil {
		return l.dedupKey == key
	}
	if l.dedupKey != nil {
		return false
	}
	return funcEqual(l.Callback, callback)
}

// RemoveEventListener removes a previously registered listener.
func RemoveEventListener(n *Node, typ string, callback func(*Event), capture bool) {
	ls := getListeners(n)
	for i, l := range ls {
		if l.Type == typ && funcEqual(l.Callback, callback) && l.Capture == capture {
			ls = append(ls[:i], ls[i+1:]...)
			setListeners(n, ls)
			return
		}
	}
}

// GetNodeListeners returns a copy of the listeners registered on node n.
func GetNodeListeners(n *Node) []*EventListener {
	orig := getListeners(n)
	if orig == nil {
		return nil
	}
	cp := make([]*EventListener, len(orig))
	copy(cp, orig)
	return cp
}

// ---------------------------------------------------------------------------
// Event dispatch
// ---------------------------------------------------------------------------

// DispatchEvent dispatches an event through the DOM tree rooted at the
// document above target. It builds the propagation path from target to
// document, runs the capture phase (document -> target's parent), fires
// listeners on the target itself (both capture and bubble), then runs the
// bubble phase (target's parent -> document) if the event bubbles.
//
// Returns true if the event was not cancelled via PreventDefault.
func DispatchEvent(target *Node, event *Event) bool {
	event.target = target

	// Build propagation path: [target, parent, ..., document].
	path := buildPath(target)

	// --- Capture phase: document -> target's parent. ---
	event.phase = CapturingPhase
	for i := len(path) - 1; i > 0; i-- {
		if event.stopped {
			break
		}
		node := path[i]
		event.currentTarget = node
		fireListeners(node, event, true)
	}

	// --- At target. ---
	if !event.stopped {
		event.phase = AtTarget
		event.currentTarget = target
		// At-target fires capture listeners first, then bubble listeners,
		// each group in registration order. This matches Chromium and the
		// DOM spec's dispatch algorithm.
		listeners := getListeners(target)
		for _, l := range listeners {
			if event.immediateStopped {
				break
			}
			if l.Type == event.Type && l.Capture {
				l.Callback(event)
			}
		}
		for _, l := range listeners {
			if event.immediateStopped {
				break
			}
			if l.Type == event.Type && !l.Capture {
				l.Callback(event)
			}
		}
	}

	// --- Bubble phase: target's parent -> document. ---
	if event.bubbles && !event.stopped {
		event.phase = BubblingPhase
		for i := 1; i < len(path); i++ {
			if event.stopped {
				break
			}
			node := path[i]
			event.currentTarget = node
			fireListeners(node, event, false)
		}
	}

	// Reset phase after dispatch completes.
	event.phase = NonePhase
	event.currentTarget = nil

	return !event.defaultPrevented
}

// buildPath returns the chain [target, parent, ..., document].
func buildPath(target *Node) []*Node {
	var path []*Node
	for n := target; n != nil; n = n.Parent {
		path = append(path, n)
	}
	return path
}

// fireListeners invokes matching listeners on node. When capture is true only
// capture listeners fire; when false only bubble listeners fire.
func fireListeners(node *Node, event *Event, capture bool) {
	for _, l := range getListeners(node) {
		if event.immediateStopped {
			break
		}
		if l.Type == event.Type && l.Capture == capture {
			l.Callback(event)
		}
	}
}
