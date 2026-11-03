package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/style"
)

// UsedBorderRadius is the rule both the painter and pointer hit testing resolve
// a declared radius through (CSS Backgrounds & Borders 5.5): a percentage is the
// share of the side it runs along, and when two corners on one side overflow
// that side every radius scales down by the SAME factor f, so an oversized value
// collapses into a pill instead of a clipped arc. The arithmetic is checkable by
// hand on the boxes below: f = min(side / sum-of-that-side).
//
// qc:BR-073
func TestUsedBorderRadiusFollowsTheCSSUsedValueRule(t *testing.T) {
	radiusDoc := "<html><body><div>x</div></body></html>"
	for _, c := range []struct {
		decl           string
		w, h           float32
		tl, tr, br, bl frame.Radius
	}{
		// Nothing overflows: 30+30 is 60 on the 120 width and exactly the 60 height.
		{"border-radius: 30px", 120, 60,
			frame.Radius{X: 30, Y: 30}, frame.Radius{X: 30, Y: 30}, frame.Radius{X: 30, Y: 30}, frame.Radius{X: 30, Y: 30}},
		// 50% of the 120 width is 60, of the 60 height 30: each side sums to itself.
		{"border-radius: 50%", 120, 60,
			frame.Radius{X: 60, Y: 30}, frame.Radius{X: 60, Y: 30}, frame.Radius{X: 60, Y: 30}, frame.Radius{X: 60, Y: 30}},
		// 999px on every corner: f = min(120/1998, 60/1998) = 60/1998, so both axes
		// land on 30 - the semicircular ends of a pill, not a 999 arc.
		{"border-radius: 999px", 120, 60,
			frame.Radius{X: 30, Y: 30}, frame.Radius{X: 30, Y: 30}, frame.Radius{X: 30, Y: 30}, frame.Radius{X: 30, Y: 30}},
		// Elliptical needs the slash: 40 runs along the width, 10 along the height.
		{"border-radius: 40px / 10px", 120, 60,
			frame.Radius{X: 40, Y: 10}, frame.Radius{X: 40, Y: 10}, frame.Radius{X: 40, Y: 10}, frame.Radius{X: 40, Y: 10}},
		// Without a slash a two-value list is the corner list - TL&BR first, then
		// TR&BL - and each corner's vertical radius repeats its horizontal one.
		{"border-radius: 40px 10px", 120, 60,
			frame.Radius{X: 40, Y: 40}, frame.Radius{X: 10, Y: 10}, frame.Radius{X: 40, Y: 40}, frame.Radius{X: 10, Y: 10}},
		// Only the height sides overflow here (150+50 > 100), yet the shared
		// f = 100/200 halves the horizontal radii too, even though 150+50 is
		// exactly the 200 width.
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

// A declared radius of 0 and an undeclared box both resolve to square corners,
// which is the fast path hit testing relies on to stay a plain rect test.
//
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

// The scale multiplies absolute radii because the painter resolves them against
// a device rect, while a percentage is resolved against the side, which already
// carries the scale. A 2x layer of a 50%-radius box therefore keeps the same
// shape, and a 30px radius doubles to 60 device px.
//
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
