package main

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func newZoomTestPath(t *testing.T) *framePath {
	t.Helper()
	f := newNavTestPath(t)
	f.config.width = 800
	f.config.height = 600
	f.toolbar = toolbar.NewState(800, nil)
	return f
}

func TestZoomClampsAtBounds(t *testing.T) {
	f := newZoomTestPath(t)

	for i := 0; i < 100; i++ {
		f.handleZoom(zoomStep)
	}
	if math.IsNaN(f.zoom) || math.IsInf(f.zoom, 0) {
		t.Fatalf("zoom after repeated zoom-in = %v, want a finite value", f.zoom)
	}
	if f.zoom != maxZoom {
		t.Fatalf("zoom after repeated zoom-in = %v, want %v", f.zoom, maxZoom)
	}
	if got, want := f.ZoomPercent(), "500%"; got != want {
		t.Fatalf("ZoomPercent after zoom-in = %q, want %q", got, want)
	}
	if got, want := f.toolbar.ZoomPercent(), "500%"; got != want {
		t.Fatalf("toolbar.ZoomPercent after zoom-in = %q, want %q", got, want)
	}

	// An overflowing step must clamp, not leave the state at infinity.
	f.handleZoom(math.Inf(1))
	if math.IsNaN(f.zoom) || math.IsInf(f.zoom, 0) || f.zoom != maxZoom {
		t.Fatalf("zoom after infinite step = %v, want %v", f.zoom, maxZoom)
	}

	for i := 0; i < 100; i++ {
		f.handleZoom(-zoomStep)
	}
	if math.IsNaN(f.zoom) || math.IsInf(f.zoom, 0) {
		t.Fatalf("zoom after repeated zoom-out = %v, want a finite value", f.zoom)
	}
	if f.zoom != minZoom {
		t.Fatalf("zoom after repeated zoom-out = %v, want %v", f.zoom, minZoom)
	}
	if got, want := f.ZoomPercent(), "25%"; got != want {
		t.Fatalf("ZoomPercent after zoom-out = %q, want %q", got, want)
	}
	if got, want := f.toolbar.ZoomPercent(), "25%"; got != want {
		t.Fatalf("toolbar.ZoomPercent after zoom-out = %q, want %q", got, want)
	}

	// An underflowing step must clamp, not leave the state negative.
	f.handleZoom(math.Inf(-1))
	if math.IsNaN(f.zoom) || math.IsInf(f.zoom, 0) || f.zoom != minZoom {
		t.Fatalf("zoom after negative infinite step = %v, want %v", f.zoom, minZoom)
	}
}

func TestZoomReset(t *testing.T) {
	f := newZoomTestPath(t)

	f.handleZoom(0.4)
	if f.zoom == 1.0 {
		t.Fatalf("zoom before reset = %v, want a non-default value", f.zoom)
	}

	f.ZoomReset()
	if f.zoom != 1.0 {
		t.Fatalf("zoom after ZoomReset = %v, want 1.0", f.zoom)
	}
	if f.toolbar.Zoom != 1.0 {
		t.Fatalf("toolbar zoom after ZoomReset = %v, want 1.0", f.toolbar.Zoom)
	}
	if got, want := f.ZoomPercent(), "100%"; got != want {
		t.Fatalf("ZoomPercent after ZoomReset = %q, want %q", got, want)
	}
}

func TestZoomShortcutChangesZoom(t *testing.T) {
	f := newZoomTestPath(t)
	cw := &chromeWindow{
		toolbar: f.toolbar,
		tabMgr:  tabs.NewManager(nil),
		onZoom:  f.handleZoom,
	}

	if !cw.handleTabShortcut('=', surface.ModCommand) {
		t.Fatal("Cmd+= was not handled")
	}
	if got, want := f.ZoomPercent(), "110%"; got != want {
		t.Fatalf("after Cmd+= zoom = %q, want %q", got, want)
	}

	if !cw.handleTabShortcut('+', surface.ModCommand) {
		t.Fatal("Cmd++ was not handled")
	}
	if got, want := f.ZoomPercent(), "120%"; got != want {
		t.Fatalf("after Cmd++ zoom = %q, want %q", got, want)
	}
	if f.toolbar.Zoom != f.zoom {
		t.Fatalf("toolbar zoom = %v, want frame zoom %v", f.toolbar.Zoom, f.zoom)
	}

	if !cw.handleTabShortcut('-', surface.ModCommand) {
		t.Fatal("Cmd+- was not handled")
	}
	if got, want := f.ZoomPercent(), "110%"; got != want {
		t.Fatalf("after Cmd+- zoom = %q, want %q", got, want)
	}

	if !cw.handleTabShortcut('0', surface.ModCommand) {
		t.Fatal("Cmd+0 was not handled")
	}
	if got, want := f.ZoomPercent(), "100%"; got != want {
		t.Fatalf("after Cmd+0 zoom = %q, want %q", got, want)
	}

	if cw.handleTabShortcut('=', 0) {
		t.Fatal("zoom shortcut fired without the command modifier")
	}
	if got, want := f.ZoomPercent(), "100%"; got != want {
		t.Fatalf("zoom after unmodified '=' = %q, want %q", got, want)
	}
}
