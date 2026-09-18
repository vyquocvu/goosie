package surface

import (
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
)

// EventKind names what happened. Vsync is an event rather than a callback
// because the UI thread has exactly one input source - a channel - and a
// platform that delivered vsync some other way would need a second select arm
// and a second ordering question.
type EventKind uint8

const (
	// EvVsync is the pacing tick. Every frame starts from one.
	EvVsync EventKind = iota
	// EvScroll carries a device-pixel delta in Delta.
	EvScroll
	// EvPointer carries a press, release, or move at Pos.
	EvPointer
	// EvKey carries a typed rune or control key in Key.
	EvKey
	// EvResize carries a new surface size in Size.
	EvResize
)

func (k EventKind) String() string {
	switch k {
	case EvVsync:
		return "vsync"
	case EvScroll:
		return "scroll"
	case EvPointer:
		return "pointer"
	case EvKey:
		return "key"
	case EvResize:
		return "resize"
	}
	return "unknown"
}

// Button distinguishes pointer actions that share an event kind.
type Button uint8

const (
	ButtonNone Button = iota
	ButtonLeft
	ButtonMiddle
	ButtonRight
)

// Cursor is the platform pointer shape. It is an enum rather than a platform
// handle so the engine can ask for a cursor without knowing what the toolkit
// calls one, which is the whole point of the window contract being an interface.
type Cursor uint8

const (
	CursorDefault Cursor = iota
	CursorPointer
	CursorText
	CursorGrab
)

// KeyMod is a bitmask of modifier keys held during a key event.
// The values match NSEvent modifier flags so the darwin shim passes them through.
type KeyMod uint32

const (
	ModShift   KeyMod = 1 << 17
	ModControl KeyMod = 1 << 18
	ModOption  KeyMod = 1 << 19
	ModCommand KeyMod = 1 << 20
)

// Event is one platform input. All positions and deltas are device pixels, so
// no handler needs the scale factor to interpret an event, and a DPR change
// cannot silently rescale a queued scroll.
type Event struct {
	Kind   EventKind
	Delta  frame.Point // EvScroll
	Pos    frame.Point // EvPointer
	Button Button      // EvPointer
	Key    rune        // EvKey
	Mods   KeyMod      // EvKey: modifier flags
	Size   frame.Size  // EvResize
	Scale  float32     // current device pixel ratio
	At     time.Time
}

// Window is what a platform must provide. Present takes the composed backing
// store and the rects that changed; a backend that cannot do partial updates
// ignores damage and uploads the whole buffer.
//
// Present must not block on rasterization, and Events must be the only way out
// of the platform. Anything else - a callback into the engine, a synchronous
// resize - would put platform code on the UI thread's critical path.
type Window interface {
	Events() <-chan Event
	Present(buf *frame.Bitmap, damage []frame.Rect) error
	SetCursor(Cursor)
	ScaleFactor() float32
	Close() error
}
