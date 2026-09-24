package main

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// recWin is a window fake that records the cursor shapes it is asked for.
type recWin struct {
	cursors []surface.Cursor
}

func (r *recWin) Events() <-chan surface.Event              { return nil }
func (r *recWin) Present(*frame.Bitmap, []frame.Rect) error { return nil }
func (r *recWin) SetCursor(c surface.Cursor)                { r.cursors = append(r.cursors, c) }
func (r *recWin) SetIME(bool)                               {}
func (r *recWin) ScaleFactor() float32                      { return 1 }
func (r *recWin) Close() error                              { return nil }

func hoverMotion(x, y int32) surface.Event {
	return surface.Event{
		Kind:   surface.EvPointer,
		Action: surface.PointerMotion,
		Button: surface.ButtonNone,
		Pos:    frame.Point{X: x, Y: y},
	}
}

func newHoverWindow(rec surface.Window) *chromeWindow {
	tb := toolbar.NewState(800, nil)
	return &chromeWindow{
		Window:  rec,
		toolbar: tb,
		events:  make(chan surface.Event, 8),
		scale:   1,
	}
}

func TestInterceptHoverToolbarShapes(t *testing.T) {
	rec := &recWin{}
	cw := newHoverWindow(rec)

	back := toolbar.ButtonRect(toolbar.ButtonBack, 800)
	cw.intercept(hoverMotion(back.X0+1, int32(tabs.TabBarHeight)+back.Y0+1))
	if len(rec.cursors) != 1 || rec.cursors[0] != surface.CursorPointer {
		t.Fatalf("hover over Back recorded %v, want one CursorPointer", rec.cursors)
	}

	addr := toolbar.AddressBarRect(800)
	cw.intercept(hoverMotion(addr.X0+1, int32(tabs.TabBarHeight)+addr.Y0+1))
	if len(rec.cursors) != 2 || rec.cursors[1] != surface.CursorText {
		t.Fatalf("hover over the address bar recorded %v, want then CursorText", rec.cursors)
	}
}

func TestInterceptHoverContentUsesResolver(t *testing.T) {
	rec := &recWin{}
	cw := newHoverWindow(rec)
	cw.onHoverCursor = func(contentX, contentY int32) surface.Cursor {
		if contentX == 50 && contentY == 5 {
			return surface.CursorPointer
		}
		return surface.CursorDefault
	}

	cw.intercept(hoverMotion(50, int32(totalChromeHeight)+5))
	if len(rec.cursors) != 1 || rec.cursors[0] != surface.CursorPointer {
		t.Fatalf("hover over content recorded %v, want one CursorPointer", rec.cursors)
	}

	// The same shape twice must not spam the platform: only a change calls.
	cw.intercept(hoverMotion(50, int32(totalChromeHeight)+5))
	if len(rec.cursors) != 1 {
		t.Fatalf("repeat hover recorded %v, want the unchanged shape dropped", rec.cursors)
	}
}

func TestInterceptHoverWithoutWindowDoesNotPanic(t *testing.T) {
	// Existing tests build chromeWindow with no Window embedded; a hover
	// over chrome or content must still be routable without one.
	cw := newHoverWindow(nil)
	if !cw.intercept(hoverMotion(10, 10)) {
		t.Fatal("tab-bar hover not consumed")
	}
	cw.intercept(hoverMotion(50, int32(totalChromeHeight)+5))
}
