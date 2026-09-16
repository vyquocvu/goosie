//go:build darwin && cgo

package darwin

/*
// The Objective-C runtime is linked for us - a package with a .m file gets -lobjc from
// cmd/go - so this list is the frameworks the shim imports and nothing else.
#cgo LDFLAGS: -framework AppKit -framework Foundation -framework CoreGraphics -framework CoreVideo -framework QuartzCore
#cgo CFLAGS: -I${SRCDIR} -Wno-deprecated-declarations
#include "shim.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
)

// Name is this backend's name. It is a method's return value as much as a constant: a
// report that says "darwin" is claiming a real NSWindow was on screen for these frames,
// so the same string has to come from the window that opened as from the table that
// chose it.
const Name = "darwin"

// Config is the window a caller asks for.
type Config struct {
	// Title is the window's title bar text.
	Title string
	// Size is the surface in device pixels, not points: the units the frame path
	// composes in, so a 2x display asked for at 2880x1800 gets a 1440x900pt window.
	Size frame.Size
	// Scale is the ratio the caller sized its buffer for. It is a request rather than a
	// fact - the window reports the ratio the screen it landed on actually uses - and it
	// is what a 1x-external-monitor run and a Retina run disagree about.
	Scale float32
}

// Window is a macOS surface.Window: an NSWindow plus the goroutine that turns its
// display link and its input handlers into the one event channel the loop reads.
//
// Three threads reach a Window. The main thread owns it - AppKit will not be called
// from anywhere else, and so Open and Run are both main-thread-only. The UI thread
// calls Present and SetCursor. A pump goroutine calls Events' machinery and nothing
// else. No Go field is shared between them: Present's scratch belongs to the UI thread
// alone, and everything another thread reads comes out of the shim's atomics.
type Window struct {
	gw       *C.GoosieWindow
	events   chan surface.Event
	detach   chan struct{}
	pumpDone chan struct{}
	closing  sync.Once

	// dmg is the present's damage list flattened into C ints, kept in the window so a
	// warm frame allocates nothing. It grows to the high-water mark of the list the
	// composer reports and then stops, which is the same argument the scheduler makes
	// about its own scratch. Only the goroutine that calls Present touches it, and the
	// frame path guarantees there is exactly one of those.
	dmg [][4]C.int
}

// Available reports whether this process can reach a window server.
//
// It asks CoreGraphics rather than AppKit on purpose: a capability probe that needed
// the main thread could only be answered by a program already committed to opening a
// window, and go test on a macOS runner has to be able to ask without popping one.
func Available() bool { return C.GoosieHasDisplay() != 0 }

// Open creates the window. It must run on the process's main thread, on a goroutine
// pinned there with runtime.LockOSThread; anything else is ErrNotMainThread rather
// than the abort AppKit would otherwise raise.
func Open(cfg Config) (*Window, error) {
	if C.GoosieIsMainThread() == 0 {
		return nil, ErrNotMainThread
	}
	if cfg.Size.W <= 0 || cfg.Size.H <= 0 {
		return nil, fmt.Errorf("platform/darwin: a window needs a positive size, got %dx%d",
			cfg.Size.W, cfg.Size.H)
	}
	title := C.CString(cfg.Title)
	defer C.free(unsafe.Pointer(title))

	var cerr C.int
	gw := C.GoosieWindowCreate(title, C.int(cfg.Size.W), C.int(cfg.Size.H), &cerr)
	if gw == nil {
		return nil, createErr(int(cerr), cfg)
	}
	w := &Window{
		gw:       gw,
		events:   make(chan surface.Event),
		detach:   make(chan struct{}),
		pumpDone: make(chan struct{}),
	}
	go w.pump()
	return w, nil
}

// createErr turns the shim's failure code into one of the named errors. The codes are
// spelled out in GoosieWindowCreate; a bare "create failed" would leave a caller
// unable to tell "no display" from "wrong thread", which are two different fixes.
func createErr(code int, cfg Config) error {
	switch code {
	case 1:
		return ErrNotMainThread
	case 2:
		return fmt.Errorf("platform/darwin: a window needs a positive size, got %dx%d",
			cfg.Size.W, cfg.Size.H)
	case 3:
		return fmt.Errorf("platform/darwin: out of memory allocating a %dx%d window",
			cfg.Size.W, cfg.Size.H)
	case 4:
		return ErrNoDisplay
	case 5:
		return ErrNoDisplayLink
	}
	return fmt.Errorf("platform/darwin: window create failed with code %d", code)
}

// Events implements surface.Window.
//
// The channel is unbuffered and the shim's ring is the buffer, which puts the drop
// point and the drop count in exactly one place: a window whose reader has stopped is
// visible in the counters rather than in a Go queue that grew.
func (w *Window) Events() <-chan surface.Event { return w.events }

// pump is the one goroutine allowed to block in the platform. GoosieNextEvent waits on
// a condition variable rather than consuming CPU, so an idle window costs nothing, and
// the frame path's single input source stays a channel.
func (w *Window) pump() {
	defer close(w.events)
	defer close(w.pumpDone)

	var ce C.GoosieEvent
	for {
		select {
		case <-w.detach:
			return
		default:
		}
		if C.GoosieNextEvent(w.gw, &ce) == 0 {
			return
		}
		if ev, ok := translate(ce); ok {
			select {
			case w.events <- ev:
			case <-w.detach:
				return
			}
		}
	}
}

// translate maps one shim event onto the frame path's Event, and reports whether the
// shim's kind meant anything. An unmapped kind is dropped rather than turned into a
// zero Event, because a zero Event reads as a vsync and would draw a frame nobody
// asked for.
//
// At is stamped here rather than lifted from the shim's own stamp. The two clocks have
// no common epoch in this build - the shim reads CLOCK_MONOTONIC_RAW, Go's monotonic
// clock reads mach_absolute_time - and a FrameMark whose VsyncAt came from one and
// whose PlanAt came from the other subtracts to nonsense. What the recorder's chain
// measures is therefore the frame path from the tick's arrival, and the shim's stamp is
// left for a measure that wants input-to-pixel and translates the base first.
func translate(ce C.GoosieEvent) (surface.Event, bool) {
	ev := surface.Event{
		Scale: float32(ce.scale),
		At:    time.Now(),
	}
	switch ce.kind {
	case C.GOOSIE_EV_VSYNC:
		ev.Kind = surface.EvVsync
	case C.GOOSIE_EV_SCROLL:
		ev.Kind = surface.EvScroll
		ev.Delta = frame.Point{X: int32(ce.dx), Y: int32(ce.dy)}
	case C.GOOSIE_EV_POINTER:
		// The shim's action distinguishes a press from a release, and surface.Event
		// has no field for it: M1's frame path has nothing to point at, so the action
		// is dropped rather than faked. M2's hit testing needs it, and when it does the
		// right change is a field on Event, not a press smuggled in as a button.
		ev.Kind = surface.EvPointer
		ev.Pos = frame.Point{X: int32(ce.px), Y: int32(ce.py)}
		switch ce.action {
		case C.GOOSIE_POINTER_PRESS:
			ev.Button = button(ce.button)
		default:
			ev.Button = surface.ButtonNone
		}
	case C.GOOSIE_EV_KEY:
		ev.Kind = surface.EvKey
		ev.Key = rune(ce.key)
	case C.GOOSIE_EV_RESIZE:
		ev.Kind = surface.EvResize
		ev.Size = frame.Size{W: int32(ce.w), H: int32(ce.h)}
	default:
		// GOOSIE_EV_NONE and anything a future shim adds before this file learns it.
		return ev, false
	}
	return ev, true
}

// button maps the shim's button number; 1 left, 2 middle, 3 right, 0 none.
func button(n C.int) surface.Button {
	switch n {
	case 1:
		return surface.ButtonLeft
	case 2:
		return surface.ButtonMiddle
	case 3:
		return surface.ButtonRight
	}
	return surface.ButtonNone
}

// Present hands the composed buffer to Core Animation.
//
// It does not wait for the commit. The shim copies the damaged rects into its own pixel
// surface and returns; the CGImage that references that copy is built on the main
// thread. A frame path that blocked here would be a frame path whose scroll rate is
// set by how fast the window server drains a queue.
//
// GOOSIE_DROPPED is not an error and does not become one: it means all three of the
// shim's surfaces were still owed to the display, so this tick's pixels were skipped
// and the next frame's own damage covers them.
func (w *Window) Present(buf *frame.Bitmap, damage []frame.Rect) error {
	if buf == nil || buf.Empty() {
		return ErrNoPixels
	}
	if buf.Stride < buf.W*4 {
		return fmt.Errorf("platform/darwin: buffer stride %d is narrower than a %d px row", buf.Stride, buf.W)
	}
	if len(w.dmg) < len(damage) {
		w.dmg = make([][4]C.int, len(damage))
	}
	list := w.dmg[:0]
	for _, r := range damage {
		if r.Empty() {
			continue
		}
		list = append(list, [4]C.int{C.int(r.X0), C.int(r.Y0), C.int(r.X1), C.int(r.Y1)})
	}

	var rects *C.int
	if len(list) > 0 {
		rects = &list[0][0]
	}
	rc := C.GoosiePresent(w.gw,
		(*C.uchar)(unsafe.Pointer(&buf.RGBA[0])),
		C.int(buf.W), C.int(buf.H), C.int(buf.Stride),
		rects, C.int(len(list)))
	switch rc {
	case C.GOOSIE_OK, C.GOOSIE_DROPPED:
		return nil
	case C.GOOSIE_CLOSED:
		return ErrClosed
	}
	return fmt.Errorf("%w: present returned %d", ErrClosed, int(rc))
}

// SetCursor asks the main thread for a pointer shape. It is asynchronous by necessity:
// the frame path calls it mid-draw, and the only thread allowed to call AppKit is the
// one that may be busy elsewhere in the run loop.
func (w *Window) SetCursor(c surface.Cursor) {
	var shape C.int
	switch c {
	case surface.CursorPointer:
		shape = C.GOOSIE_CURSOR_POINTER
	case surface.CursorText:
		shape = C.GOOSIE_CURSOR_TEXT
	case surface.CursorGrab:
		shape = C.GOOSIE_CURSOR_GRAB
	default:
		shape = C.GOOSIE_CURSOR_DEFAULT
	}
	C.GoosieSetCursor(w.gw, shape)
}

// ScaleFactor implements surface.Window. It is the ratio the window actually got,
// which is not necessarily the one Config asked for.
func (w *Window) ScaleFactor() float32 { return float32(C.GoosieScaleFactor(w.gw)) }

// Size returns the surface size in device pixels, which is what a buffer presented to
// this window has to match.
func (w *Window) Size() frame.Size {
	var cw, ch C.int
	C.GoosieSizeDevice(w.gw, &cw, &ch)
	return frame.Size{W: int32(cw), H: int32(ch)}
}

// Name satisfies platform.Namer.
func (w *Window) Name() string { return Name }

// Presents satisfies platform.PresentCounter. Committed is the number that says the
// frame reached Core Animation: a present the loop made and the shim queued is not the
// same claim as a commit that ran, and on a window whose run loop has stopped the two
// differ by everything still in the queue.
func (w *Window) Presents() (queued, dropped, committed int64) {
	var q, d, c C.int
	C.GoosieCounters(w.gw, &q, &d, nil, &c, nil)
	return int64(q), int64(d), int64(c)
}

// DroppedVsyncs satisfies platform.DroppedVsyncer. It counts the ticks the display
// produced and this window could not deliver, which is the number that tells a skipped
// frame apart from a slow one.
func (w *Window) DroppedVsyncs() int64 {
	var shed C.int
	C.GoosieCounters(w.gw, nil, nil, &shed, nil, nil)
	return int64(shed)
}

// Run enters AppKit's run loop and does not return until the window is closed.
//
// It belongs on the main thread, on the goroutine that called Open: the alternative is
// a second thread owning AppKit while the first one sits idle, which is not a shape
// NSApplication supports. A backend with no run loop does not have this method, and
// cmd/goosie finds out by asking for platform.Runner.
func (w *Window) Run() {
	C.GoosieAppRun(w.gw)
	// Whatever ended the loop - a close button, ⌘Q, or GoosieClose - GoosieAppRun has
	// already pushed the quit flag on the way out, so a pump blocked in
	// GoosieNextEvent is released by this return rather than left in it.
}

// Close stops the display link, ends the run loop, and waits for the pump to leave.
// It is idempotent, and it is safe to call from a goroutine that is not the main
// thread - which is the only way to call it while Run is turning the loop.
//
// Every caller waits for the pump, including a second one arriving while the first is
// still waiting: a window that reports itself closed while something is still inside
// the platform is a window whose next Present is a surprise.
func (w *Window) Close() error {
	w.closing.Do(func() {
		close(w.detach)
		C.GoosieClose(w.gw)
	})
	<-w.pumpDone
	return nil
}

var _ surface.Window = (*Window)(nil)
