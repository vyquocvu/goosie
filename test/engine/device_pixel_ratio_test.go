package engine_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func dprSession(t *testing.T, opts ...engine.Option) *engine.Session {
	t.Helper()
	rt, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://test.example/"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { rt.Close() })
	sess, err := engine.NewSession(`<html><body><p>hi</p></body></html>`, nil, 800,
		append([]engine.Option{engine.WithViewportH(600), engine.WithJS(rt)}, opts...)...)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess
}

// qc:BR-297
func TestSessionDevicePixelRatioDefaultsToOne(t *testing.T) {
	s := dprSession(t)
	got, err := s.EvaluateJS(`String(window.devicePixelRatio)`)
	if err != nil {
		t.Fatalf("EvaluateJS: %v", err)
	}
	if got != "1" {
		t.Fatalf("default window.devicePixelRatio = %q, want 1", got)
	}
}

// qc:BR-297
func TestSessionDevicePixelRatioFollowsTheOptionAndZoom(t *testing.T) {
	s := dprSession(t, engine.WithDevicePixelRatio(3))
	if got, _ := s.EvaluateJS(`String(window.devicePixelRatio)`); got != "3" {
		t.Fatalf("window.devicePixelRatio with WithDevicePixelRatio(3) = %q, want 3", got)
	}
	s.SetDevicePixelRatio(2)
	if got, _ := s.EvaluateJS(`String(window.devicePixelRatio)`); got != "2" {
		t.Fatalf("window.devicePixelRatio after SetDevicePixelRatio(2) (zoom) = %q, want 2", got)
	}
	s.SetDevicePixelRatio(0)
	if got, _ := s.EvaluateJS(`String(window.devicePixelRatio)`); got != "2" {
		t.Fatalf("window.devicePixelRatio after an invalid SetDevicePixelRatio(0) = %q, want the unchanged 2", got)
	}
}
