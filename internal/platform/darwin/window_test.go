//go:build darwin && cgo

package darwin

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
)

// Type inference supplies the shim event because test files cannot import C.
func zeroEvent[T any](func(T) (surface.Event, bool)) T {
	var ev T
	return ev
}

func TestTranslateScrollDirection(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reverse bool
		want    frame.Point
	}{
		{"down and right", false, frame.Point{X: 12, Y: 40}},
		{"up and left", true, frame.Point{X: -12, Y: -40}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ce := zeroEvent(translate)
			ce.kind = 1 // GOOSIE_EV_SCROLL
			ce.dx, ce.dy = -12, -40
			if tc.reverse {
				ce.dx, ce.dy = -ce.dx, -ce.dy
			}
			ce.px, ce.py, ce.scale = 100, 160, 2

			ev, ok := translate(ce)
			if !ok || ev.Kind != surface.EvScroll {
				t.Fatalf("translate = %+v, %t; want a scroll event", ev, ok)
			}
			if ev.Delta != tc.want {
				t.Errorf("scroll delta = %v, want %v", ev.Delta, tc.want)
			}
			if ev.Pos != (frame.Point{X: 100, Y: 160}) || ev.Scale != 2 {
				t.Errorf("scroll position/scale changed: %+v", ev)
			}
		})
	}
}
