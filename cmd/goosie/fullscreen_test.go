package main

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestFullscreenToggle(t *testing.T) {
	cw2 := &capWin{}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("no fonts: %v", err)
	}
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	tb := toolbar.NewState(800, fonts)
	cw := newChromeWindow(cw2, tb, mgr,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fonts, 1)

	initialH := cw.chromeHeight()
	if initialH <= 0 {
		t.Fatalf("expected initial chrome height > 0, got %d", initialH)
	}

	// 1. Toggle fullscreen with Cmd+Shift+F
	evCmdShiftF := surface.Event{
		Kind: surface.EvKey,
		Key:  'f',
		Mods: surface.ModCommand | surface.ModShift,
	}
	if !cw.intercept(evCmdShiftF) {
		t.Fatal("Cmd+Shift+F was not handled")
	}
	if !cw.IsFullscreen() {
		t.Fatal("expected IsFullscreen() = true after Cmd+Shift+F")
	}
	if cw.chromeHeight() != 0 {
		t.Fatalf("expected chromeHeight() = 0 in fullscreen, got %d", cw.chromeHeight())
	}

	// 2. Exit fullscreen with Escape
	evEsc := surface.Event{
		Kind: surface.EvKey,
		Key:  0x1B, // Escape
	}
	if !cw.intercept(evEsc) {
		t.Fatal("Escape was not handled to exit fullscreen")
	}
	if cw.IsFullscreen() {
		t.Fatal("expected IsFullscreen() = false after Escape")
	}
	if cw.chromeHeight() != initialH {
		t.Fatalf("expected chromeHeight() restored to %d, got %d", initialH, cw.chromeHeight())
	}

	// 3. Toggle fullscreen with F11 (0xF70E)
	evF11 := surface.Event{
		Kind: surface.EvKey,
		Key:  0xF70E,
	}
	if !cw.intercept(evF11) {
		t.Fatal("F11 was not handled")
	}
	if !cw.IsFullscreen() {
		t.Fatal("expected IsFullscreen() = true after F11")
	}

	// 4. Toggle off fullscreen with F11 keysym (0xFFC8)
	evF11Keysym := surface.Event{
		Kind: surface.EvKey,
		Key:  0xFFC8,
	}
	if !cw.intercept(evF11Keysym) {
		t.Fatal("F11 keysym was not handled")
	}
	if cw.IsFullscreen() {
		t.Fatal("expected IsFullscreen() = false after second F11")
	}
}

func TestFullscreenPresentAndPointer(t *testing.T) {
	cw2 := &capWin{}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("no fonts: %v", err)
	}
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	tb := toolbar.NewState(800, fonts)

	clickedContentX := int32(-1)
	clickedContentY := int32(-1)
	onFocus := func(contentX, contentY int32) bool {
		clickedContentX = contentX
		clickedContentY = contentY
		return true
	}

	cw := newChromeWindow(cw2, tb, mgr,
		nil, nil, nil, nil, nil, nil, nil, nil, onFocus, nil, nil, nil, nil, nil, nil, fonts, 1)

	// Enter fullscreen
	cw.SetFullscreen(true)
	if !cw.IsFullscreen() {
		t.Fatal("SetFullscreen(true) failed")
	}

	// Present in fullscreen mode
	buf := frame.NewBitmap(800, 600)
	buf.FillRect(frame.Rect4(0, 0, 800, 600), frame.RGB(0, 128, 255), nil)
	if err := cw.Present(buf, nil); err != nil {
		t.Fatalf("Present failed: %v", err)
	}

	if cw2.got == nil {
		t.Fatal("Window.Present was not called")
	}
	if cw2.got.H != 600 || cw2.got.W != 800 {
		t.Fatalf("expected presented bitmap 800x600, got %dx%d", cw2.got.W, cw2.got.H)
	}

	// Click at y=10 (where tab bar would normally be in non-fullscreen)
	clickEv := surface.Event{
		Kind:   surface.EvPointer,
		Pos:    frame.Point{X: 150, Y: 10},
		Button: surface.ButtonLeft,
		Action: surface.PointerPress,
	}
	consumed := cw.intercept(clickEv)
	if !consumed {
		t.Fatal("expected content to consume click in fullscreen")
	}
	if clickedContentX != 150 || clickedContentY != 10 {
		t.Fatalf("expected click at content (150, 10), got (%d, %d)", clickedContentX, clickedContentY)
	}
}
