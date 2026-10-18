package main

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestFindInPageLifecycle(t *testing.T) {
	f := newNavTestPath(t)
	f.toolbar = toolbar.NewState(800, nil)

	var nextCalled, closeCalled bool
	var queryReceived string
	f.toolbar.OnFindChanged = func(q string) {
		queryReceived = q
	}
	f.toolbar.OnFindNext = func(back bool) {
		nextCalled = true
	}
	f.toolbar.OnFindClose = func() {
		closeCalled = true
	}

	// 1. Open Find
	f.toolbar.OpenFind()
	active, total, idx := f.toolbar.FindCounts()
	if !active {
		t.Fatal("FindActive should be true after OpenFind")
	}
	if total != 0 || idx != 0 {
		t.Errorf("initial find counts = (%d, %d), want (0, 0)", total, idx)
	}

	// 2. Type query
	f.toolbar.HandleKeyEvent('t', 0)
	f.toolbar.HandleKeyEvent('e', 0)
	f.toolbar.HandleKeyEvent('s', 0)
	f.toolbar.HandleKeyEvent('t', 0)

	if queryReceived != "test" {
		t.Errorf("queryReceived = %q, want 'test'", queryReceived)
	}

	// 3. Set match counts
	f.toolbar.SetFindResult(5, 0)
	active, total, idx = f.toolbar.FindCounts()
	if !active || total != 5 || idx != 0 {
		t.Errorf("FindCounts() = (%v, %d, %d), want (true, 5, 0)", active, total, idx)
	}

	// 4. Enter steps to next match
	f.toolbar.HandleKeyEvent('\r', 0)
	if !nextCalled {
		t.Fatal("Enter should trigger OnFindNext")
	}

	// 5. Esc closes find
	f.toolbar.HandleKeyEvent(0x1b, 0) // Esc
	if !closeCalled {
		t.Fatal("Esc should trigger OnFindClose")
	}
	active, _, _ = f.toolbar.FindCounts()
	if active {
		t.Fatal("FindActive should be false after close")
	}
}

func TestZoomBadgeIndicatorAndReset(t *testing.T) {
	tb := toolbar.NewState(800, nil)
	var resetTriggered bool
	tb.OnResetZoom = func() {
		resetTriggered = true
	}

	// Initial zoom = 1.0, badge is not drawn
	buf := frame.NewBitmap(800, toolbar.ToolbarHeight)
	tb.Draw(buf, 0)

	// Set Zoom to 1.25 (125%)
	tb.SetZoom(1.25)
	if tb.Zoom != 1.25 {
		t.Errorf("tb.Zoom = %f, want 1.25", tb.Zoom)
	}

	// Badge should be drawn on toolbar
	tb.Draw(buf, 0)

	// In normal mode, badge background is frame.RGB(232, 240, 254)
	hasBadgePixels := false
	addrRect := toolbar.AddressBarRect(800)
	for y := addrRect.Y0; y < addrRect.Y1; y++ {
		for x := addrRect.X1 - 50; x < addrRect.X1; x++ {
			c := buf.At(int(x), int(y)).RGBA()
			if c.R == 232 && c.G == 240 && c.B == 254 {
				hasBadgePixels = true
				break
			}
		}
		if hasBadgePixels {
			break
		}
	}
	if !hasBadgePixels {
		t.Error("expected light blue badge pixels frame.RGB(232, 240, 254) for 125% zoom")
	}

	// Hover over badge gives pointer cursor
	badgeX := addrRect.X1 - 20
	badgeY := (addrRect.Y0 + addrRect.Y1) / 2
	cur := tb.CursorAt(frame.Point{X: badgeX, Y: badgeY})
	if cur != surface.CursorPointer {
		t.Errorf("CursorAt zoom badge = %v, want CursorPointer", cur)
	}

	// Click on badge resets zoom to 1.0
	tb.HandleClick(frame.Point{X: badgeX, Y: badgeY}, surface.ButtonLeft)
	if !resetTriggered {
		t.Error("clicking zoom badge did not trigger OnResetZoom")
	}
	if tb.Zoom != 1.0 {
		t.Errorf("tb.Zoom after click = %f, want 1.0", tb.Zoom)
	}
}

func TestZoomKeyboardShortcuts(t *testing.T) {
	mgr := tabs.NewManager(nil)
	tb := toolbar.NewState(800, nil)

	var zoomDelta float64
	var zoomCalled bool
	cw := &chromeWindow{
		toolbar: tb,
		tabMgr:  mgr,
		onZoom: func(delta float64) {
			zoomCalled = true
			zoomDelta = delta
		},
	}

	// Cmd++
	zoomCalled = false
	cw.handleTabShortcut('=', surface.ModCommand)
	if !zoomCalled || zoomDelta != 0.1 {
		t.Errorf("Cmd+= failed: called=%v, delta=%f", zoomCalled, zoomDelta)
	}

	// Cmd+-
	zoomCalled = false
	cw.handleTabShortcut('-', surface.ModCommand)
	if !zoomCalled || zoomDelta != -0.1 {
		t.Errorf("Cmd+- failed: called=%v, delta=%f", zoomCalled, zoomDelta)
	}

	// Cmd+0 (reset)
	zoomCalled = false
	cw.handleTabShortcut('0', surface.ModCommand)
	if !zoomCalled || zoomDelta != 0 {
		t.Errorf("Cmd+0 failed: called=%v, delta=%f", zoomCalled, zoomDelta)
	}
}

func TestFramePathHandleZoom(t *testing.T) {
	f := newNavTestPath(t)
	f.config.width = 800
	f.config.height = 600
	f.toolbar = toolbar.NewState(800, nil)

	// Zoom in
	f.handleZoom(0.2)
	if f.zoom < 1.19 || f.zoom > 1.21 {
		t.Errorf("f.zoom = %f, want ~1.2", f.zoom)
	}
	if f.toolbar.Zoom < 1.19 || f.toolbar.Zoom > 1.21 {
		t.Errorf("f.toolbar.Zoom = %f, want ~1.2", f.toolbar.Zoom)
	}

	// Reset zoom
	f.handleZoom(0)
	if f.zoom != 1.0 {
		t.Errorf("f.zoom after reset = %f, want 1.0", f.zoom)
	}
	if f.toolbar.Zoom != 1.0 {
		t.Errorf("f.toolbar.Zoom after reset = %f, want 1.0", f.toolbar.Zoom)
	}
}
