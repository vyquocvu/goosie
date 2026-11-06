package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

// The radial-gradient header: ending shape, size keyword and position. All of
// it is measured, not recalled: scripts/verify-radialgrad-geometry.js prints
// Chromium 1.63 on this host (2026-11-06) answering
//
//	SYNTAX circle-default         kept=YES computed=radial-gradient(circle, …)
//	SYNTAX circle-closest-side    kept=YES computed=radial-gradient(circle closest-side at 25% 25%, …)
//	SYNTAX circle-farthest-side   kept=YES computed=radial-gradient(circle farthest-side at 25% 25%, …)
//	SYNTAX circle-closest-corner  kept=YES computed=radial-gradient(circle closest-corner at 25% 25%, …)
//	SYNTAX ellipse-default        kept=YES computed=radial-gradient(…) - shape and size default away
//	SYNTAX ellipse-closest-side   kept=YES computed=radial-gradient(closest-side, …) - bare size without a shape
//	SYNTAX px-position            kept=YES computed=radial-gradient(circle at 30px 30px, …)
//	SYNTAX header-invalid         kept=DROPPED computed=none - an unknown token drops the declaration
//	SYNTAX explicit-radii         kept=YES computed=radial-gradient(100px 50px, …) - recorded as a residual, not built
//
// Goosie matched whole header arguments instead of their space-separated
// tokens, so every form but a bare "circle" or "ellipse" failed the stop
// reader and dropped the whole gradient: `circle closest-side at 25% 25%`
// painted nothing at all, not the wrong shape.

func radialOf(t *testing.T, css string) style.RadialGradient {
	t.Helper()
	s := styleFor(t, `<html><body><div>x</div></body></html>`, css, "div")
	return s.BackgroundRadialGradient
}

// qc:BR-074
func TestRadialHeaderParsesShapeSizeAndPosition(t *testing.T) {
	cases := []struct {
		name string
		css  string
		what style.RadialGradient
	}{
		{
			name: "shape and size with a percentage position",
			css:  `div { background: radial-gradient(circle closest-side at 25% 25%,#f00,#00f); }`,
			what: style.RadialGradient{Shape: "circle", Size: "closest-side", Cx: 0.25, Cy: 0.25},
		},
		{
			name: "size keyword without a shape",
			css:  `div { background: radial-gradient(ellipse closest-side,#f00,#00f); }`,
			what: style.RadialGradient{Shape: "ellipse", Size: "closest-side", Cx: 0.5, Cy: 0.5},
		},
		{
			name: "farthest-corner is the default size",
			css:  `div { background: radial-gradient(circle,#f00,#00f); }`,
			what: style.RadialGradient{Shape: "circle", Size: "farthest-corner", Cx: 0.5, Cy: 0.5},
		},
		{
			name: "order-free shape and size before the position",
			css:  `div { background: radial-gradient(closest-side circle at 25% 25%,#f00,#00f); }`,
			what: style.RadialGradient{Shape: "circle", Size: "closest-side", Cx: 0.25, Cy: 0.25},
		},
		{
			name: "keyword position",
			css:  `div { background: radial-gradient(circle at top,#f00,#00f); }`,
			what: style.RadialGradient{Shape: "circle", Size: "farthest-corner", Cx: 0.5, Cy: 0},
		},
	}
	for _, tc := range cases {
		g := radialOf(t, tc.css)
		if g.Empty() {
			t.Errorf("%s: the whole gradient dropped, want shape %q size %q centre (%.2f,%.2f)",
				tc.name, tc.what.Shape, tc.what.Size, tc.what.Cx, tc.what.Cy)
			continue
		}
		if g.Shape != tc.what.Shape || g.Size != tc.what.Size || g.Cx != tc.what.Cx || g.Cy != tc.what.Cy {
			t.Errorf("%s: header = shape %q size %q centre (%.2f,%.2f), want shape %q size %q centre (%.2f,%.2f)",
				tc.name, g.Shape, g.Size, g.Cx, g.Cy, tc.what.Shape, tc.what.Size, tc.what.Cx, tc.what.Cy)
		}
		if len(g.Stops) != 2 {
			t.Errorf("%s: stops = %d, want 2", tc.name, len(g.Stops))
		}
	}
}

// A centre component written as a <length> is a distance from the edge, not a
// fraction: the box it divides by does not exist until layout, so it rides the
// PxSet pair exactly the way length stop positions do.
//
// qc:BR-074
func TestRadialHeaderLengthPositionRidesUnresolved(t *testing.T) {
	g := radialOf(t, `div { background: radial-gradient(circle at 30px 30px,#f00,#00f); }`)
	if g.Empty() {
		t.Fatal("the whole gradient dropped, want a kept ramp with a length position")
	}
	if !g.CxPxSet || g.CxPx != 30 || !g.CyPxSet || g.CyPx != 30 {
		t.Errorf("length position = (%v,%v) set (%v,%v), want (30,30) set (true,true)",
			g.CxPx, g.CyPx, g.CxPxSet, g.CyPxSet)
	}
}

// One percent and one length can share a position, each on its own axis.
//
// qc:BR-074
func TestRadialHeaderMixedPositionComponents(t *testing.T) {
	g := radialOf(t, `div { background: radial-gradient(circle at 25% 40px,#f00,#00f); }`)
	if g.Empty() {
		t.Fatal("the whole gradient dropped, want a kept ramp with a mixed position")
	}
	if g.Cx != 0.25 || !g.CyPxSet || g.CyPx != 40 {
		t.Errorf("mixed position = cx %.2f cy (%v px %v), want cx 0.25 cy (true px 40)",
			g.Cx, g.CyPxSet, g.CyPx)
	}
}

// An unknown header token drops the declaration, the same answer Chromium
// gives: kept=DROPPED computed=none.
//
// qc:BR-074
func TestRadialHeaderInvalidTokenDropsTheGradient(t *testing.T) {
	g := radialOf(t, `div { background: radial-gradient(circle nonsense,#f00,#00f); }`)
	if !g.Empty() {
		t.Errorf("an invalid header token produced a ramp, want nothing: the reference drops the declaration")
	}
}
