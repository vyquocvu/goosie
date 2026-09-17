package main

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// toolbarWindow sits between the platform window and surface.Loop. It draws the
// toolbar overlay onto the backing store after content composition and routes
// input: pointer hits in the toolbar area go to toolbar, key events go to toolbar
// when the address bar is focused, and everything else passes through to the
// content pipeline unchanged.
//
// The pattern is the same one driver uses: embed the window, replace Events and
// Present. The pump goroutine reads the real window's events, decides whether
// toolbar wants each one, and forwards the rest.
type toolbarWindow struct {
	surface.Window
	toolbar *toolbar.State
	events  chan surface.Event
}

func newToolbarWindow(w surface.Window, tb *toolbar.State) *toolbarWindow {
	tw := &toolbarWindow{
		Window:  w,
		toolbar: tb,
		events:  make(chan surface.Event, 64),
	}
	go tw.pump()
	return tw
}

// Present implements surface.Window. It draws the toolbar overlay on the backing
// store and then hands the buffer to the real window. The damage list is widened
// to include the toolbar area so the platform presents the toolbar pixels too.
func (tw *toolbarWindow) Present(buf *frame.Bitmap, damage []frame.Rect) error {
	tw.toolbar.Draw(buf)
	tbRect := frame.Rect4(0, 0, int32(buf.W), toolbar.ToolbarHeight)
	merged := append(damage[:len(damage):len(damage)], tbRect)
	return tw.Window.Present(buf, merged)
}

// Events implements surface.Window, returning the toolbar-filtered channel.
func (tw *toolbarWindow) Events() <-chan surface.Event {
	return tw.events
}

// Name forwards the underlying window's name so the report identifies the real
// backend, not the toolbar wrapper.
func (tw *toolbarWindow) Name() string {
	if n, ok := tw.Window.(interface{ Name() string }); ok {
		return n.Name()
	}
	return ""
}

// Run implements platform.Runner by forwarding to the underlying window's Run,
// if it has one. This is required for native backends (e.g. AppKit/darwin) whose
// run loop must be turned on the main thread: without this, the type assertion in
// main.go fails and the NSApplication run loop never starts, so the window is
// created but never shown on screen.
func (tw *toolbarWindow) Run() {
	if r, ok := tw.Window.(platform.Runner); ok {
		r.Run()
	}
}

func (tw *toolbarWindow) pump() {
	defer close(tw.events)
	for ev := range tw.Window.Events() {
		if tw.intercept(ev) {
			continue
		}
		tw.events <- ev
	}
}

// intercept returns true if toolbar consumed the event. Pointer events in the
// toolbar area become clicks; key events go to the address bar when focused;
// resize events update the toolbar bounds. Everything else passes through.
func (tw *toolbarWindow) intercept(ev surface.Event) bool {
	switch ev.Kind {
	case surface.EvPointer:
		if ev.Pos.Y < toolbar.ToolbarHeight && ev.Button == surface.ButtonLeft {
			tw.toolbar.HandleClick(ev.Pos, ev.Button)
			return true
		}
		if ev.Pos.Y < toolbar.ToolbarHeight {
			return true
		}
		// A click outside the toolbar while the address bar is focused: let toolbar
		// defocus, then forward the event to content.
		if tw.toolbar.Focus == toolbar.FocusAddress {
			tw.toolbar.HandleClick(ev.Pos, ev.Button)
		}
		return false
	case surface.EvKey:
		if tw.toolbar.Focus == toolbar.FocusAddress {
			tw.toolbar.HandleKey(ev.Key)
			return true
		}
		return false
	case surface.EvResize:
		tw.toolbar.SetBounds(ev.Size.W)
		return false
	default:
		return false
	}
}

var _ surface.Window = (*toolbarWindow)(nil)


