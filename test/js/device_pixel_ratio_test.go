package js_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

// qc:BR-297
func TestDevicePixelRatioIsANumberByDefault(t *testing.T) {
	r := newDOMRuntime(t, domtest.Parse("<html><body></body></html>"))
	if err := r.Run(`var t = typeof window.devicePixelRatio; var v = String(window.devicePixelRatio);`, "inline"); err != nil {
		t.Fatalf("reading devicePixelRatio: %v", err)
	}
	if v, ok := r.Global("t"); !ok || v.Str != "number" {
		t.Fatalf("typeof window.devicePixelRatio = %+v, want number", v)
	}
	if v, ok := r.Global("v"); !ok || v.Str != "1" {
		t.Fatalf("default window.devicePixelRatio = %+v, want 1", v)
	}
}

// qc:BR-297
func TestDevicePixelRatioTracksTheConfiguredRatio(t *testing.T) {
	r := newDOMRuntime(t, domtest.Parse("<html><body></body></html>"))
	r.SetDevicePixelRatio(2)
	if err := r.Run(`var before = String(window.devicePixelRatio);`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	r.SetDevicePixelRatio(1.5)
	if err := r.Run(`var after = String(window.devicePixelRatio);`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("before"); !ok || v.Str != "2" {
		t.Fatalf("devicePixelRatio at 2x = %+v, want 2", v)
	}
	if v, ok := r.Global("after"); !ok || v.Str != "1.5" {
		t.Fatalf("devicePixelRatio after a zoom-like change to 1.5 = %+v, want 1.5", v)
	}
}

// qc:BR-297
func TestDevicePixelRatioRejectsInvalidValues(t *testing.T) {
	r := newDOMRuntime(t, domtest.Parse("<html><body></body></html>"))
	r.SetDevicePixelRatio(2)
	r.SetDevicePixelRatio(0)
	r.SetDevicePixelRatio(-3)
	r.SetDevicePixelRatio(math.NaN())
	if err := r.Run(`var v = String(window.devicePixelRatio);`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("v"); !ok || v.Str != "2" {
		t.Fatalf("devicePixelRatio after invalid SetDevicePixelRatio calls = %+v, want the last valid 2", v)
	}
}

// qc:BR-297
func TestDevicePixelRatioIsReadOnly(t *testing.T) {
	r := newDOMRuntime(t, domtest.Parse("<html><body></body></html>"))
	r.SetDevicePixelRatio(2)
	if err := r.Run(`window.devicePixelRatio = 99; var v = String(window.devicePixelRatio);`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("v"); !ok || v.Str != "2" {
		t.Fatalf("devicePixelRatio after a script assignment = %+v, want the unchanged 2", v)
	}
}
