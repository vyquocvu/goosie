package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

// qc:BR-267
func TestScreenIsDefinedWithoutADOM(t *testing.T) {
	r := newRuntime(t, jsOpts())
	src := `
var reached = false;
var out = typeof screen + "/" + (window.screen === screen);
reached = true;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatalf("reading screen: %v", err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "object/true" {
		t.Fatalf("typeof screen / identity = %+v, want object/true", v)
	}
	if v, _ := r.Global("reached"); !v.Bool {
		t.Fatal("the script stopped at the screen read: the reference still throws")
	}
}

// qc:BR-267
func TestScreenReportsTheConfiguredViewport(t *testing.T) {
	r := newDOMRuntime(t, domtest.Parse("<html><body></body></html>"))
	r.SetViewportRect(0, 0, 640, 480)
	src := `var out = [screen.width, screen.height, screen.availWidth, screen.availHeight,
	screen.colorDepth, screen.pixelDepth, window.innerWidth, window.innerHeight].join(",");`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "640,480,640,480,24,24,640,480" {
		t.Fatalf("screen/innerWidth = %+v, want 640,480,640,480,24,24,640,480", v)
	}
}

// qc:BR-267
func TestScreenTracksAViewportChange(t *testing.T) {
	r := newDOMRuntime(t, domtest.Parse("<html><body></body></html>"))
	r.SetViewportRect(0, 0, 800, 600)
	if err := r.Run(`var before = screen.width;`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	r.SetViewportRect(0, 0, 500, 600)
	if err := r.Run(`var out = before + "," + screen.width;`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "800,500" {
		t.Fatalf("screen.width before/after a resize = %+v, want 800,500", v)
	}
}

// qc:BR-267
func TestScreenWidthIsReadOnly(t *testing.T) {
	r := newRuntime(t, jsOpts())
	r.SetViewportRect(0, 0, 1024, 768)
	src := `screen.width = 1; var out = String(screen.width);`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "1024" {
		t.Fatalf("screen.width after assignment = %+v, want the unchanged 1024", v)
	}
}

// qc:BR-267
func TestScreenOrientationIsAbsent(t *testing.T) {
	r := newRuntime(t, jsOpts())
	src := `var out = typeof screen.orientation;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "undefined" {
		t.Fatalf("typeof screen.orientation = %+v, want undefined (see screen.go)", v)
	}
}
