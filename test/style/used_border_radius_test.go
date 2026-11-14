package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/style"
)

// qc:BR-073
func TestUsedBorderRadiusFollowsTheCSSUsedValueRule(t *testing.T) {
	radiusDoc := "<html><body><div>x</div></body></html>"
	for _, c := range []struct {
		decl           string
		w, h           float32
		tl, tr, br, bl frame.Radius
	}{
		{"border-radius: 30px", 120, 60,
			frame.Radius{X: 30, Y: 30}, frame.Radius{X: 30, Y: 30}, frame.Radius{X: 30, Y: 30}, frame.Radius{X: 30, Y: 30}},
		{"border-radius: 50%", 120, 60,
			frame.Radius{X: 60, Y: 30}, frame.Radius{X: 60, Y: 30}, frame.Radius{X: 60, Y: 30}, frame.Radius{X: 60, Y: 30}},
		{"border-radius: 999px", 120, 60,
			frame.Radius{X: 30, Y: 30}, frame.Radius{X: 30, Y: 30}, frame.Radius{X: 30, Y: 30}, frame.Radius{X: 30, Y: 30}},
		{"border-radius: 40px / 10px", 120, 60,
			frame.Radius{X: 40, Y: 10}, frame.Radius{X: 40, Y: 10}, frame.Radius{X: 40, Y: 10}, frame.Radius{X: 40, Y: 10}},
		{"border-radius: 40px 10px", 120, 60,
			frame.Radius{X: 40, Y: 40}, frame.Radius{X: 10, Y: 10}, frame.Radius{X: 40, Y: 40}, frame.Radius{X: 10, Y: 10}},
		{"border-radius: 150px 50px", 200, 100,
			frame.Radius{X: 75, Y: 75}, frame.Radius{X: 25, Y: 25}, frame.Radius{X: 75, Y: 75}, frame.Radius{X: 25, Y: 25}},
	} {
		t.Run(c.decl, func(t *testing.T) {
			cs := computedFor(t, radiusDoc, "div { "+c.decl+" }", "div")
			if cs == nil {
				t.Fatalf("no style computed for the div")
			}
			got := cs.UsedBorderRadius(c.w, c.h, 1)
			want := frame.Corners{TL: c.tl, TR: c.tr, BR: c.br, BL: c.bl}
			if got != want {
				t.Errorf("%q on %vx%v = %+v, want %+v", c.decl, c.w, c.h, got, want)
			}
		})
	}
}

// qc:BR-073
func TestUsedBorderRadiusKeepsSquareBoxesSquare(t *testing.T) {
	radiusDoc := "<html><body><div>x</div></body></html>"
	for _, decl := range []string{"", "border-radius: 0", "border-radius: 0 0"} {
		t.Run(decl, func(t *testing.T) {
			sheet := ""
			if decl != "" {
				sheet = "div { " + decl + " }"
			}
			cs := computedFor(t, radiusDoc, sheet, "div")
			if cs == nil {
				t.Fatalf("no style computed for the div")
			}
			if got := cs.UsedBorderRadius(120, 60, 1); !got.Empty() {
				t.Errorf("%q = %+v, want square corners", decl, got)
			}
		})
	}
	var nilStyle *style.ComputedStyle
	if got := nilStyle.UsedBorderRadius(120, 60, 1); !got.Empty() {
		t.Errorf("nil style = %+v, want square corners", got)
	}
}

// qc:BR-073
func TestUsedBorderRadiusScalesAbsoluteRadiiButNotPercentages(t *testing.T) {
	radiusDoc := "<html><body><div>x</div></body></html>"
	cs := computedFor(t, radiusDoc, "div { border-radius: 30px }", "div")
	if got := cs.UsedBorderRadius(240, 120, 2); got.TL != (frame.Radius{X: 60, Y: 60}) {
		t.Errorf("30px at scale 2 on a device box = %+v, want {60 60}", got.TL)
	}
	pct := computedFor(t, radiusDoc, "div { border-radius: 50% }", "div")
	if got := pct.UsedBorderRadius(240, 120, 2); got.TL != (frame.Radius{X: 120, Y: 60}) {
		t.Errorf("50%% at scale 2 on a device box = %+v, want {120 60}", got.TL)
	}
}
