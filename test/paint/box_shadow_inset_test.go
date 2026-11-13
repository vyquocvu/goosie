package paint_test

import (
	"strconv"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

// An inset box-shadow is the inverse of the ring these other oracles pin: its ink
// goes inside the box, over the background and under the border (CSS Backgrounds
// §4.4), clipped to the padding box, with a falloff whose shape Chromium gets by
// convolving the box's inner rect with a Gaussian.
//
// Every expectation below is Chromium's own reading, taken on 2026-11-13 from
// /tmp/is/probe6.html at deviceScaleFactor 1 (200x100 boxes on rgb(240,240,240),
// screenshots read by /tmp/is/read7.py); the narrow boxes the crossing oracle
// reads come from /tmp/is/probe8.html, read by read9.py. Alpha is derived from
// the red channel as (240-R)/240, which is 240 levels per unit and the finest
// signal a pure-blue shadow gives; the blue channel agrees to its own 1/15
// granularity.

// insetFixture lays one w x h box at the origin over a background the same
// colour as its own, so the only thing that moves a pixel is the shadow.
func insetFixture(w, h int, decl string) string {
	return `<!doctype html><html><head><style>html,body{margin:0;background:rgb(240,240,240)}` +
		`#b{position:absolute;left:0;top:0;width:` + strconv.Itoa(w) + `px;height:` + strconv.Itoa(h) +
		`px;background:rgb(240,240,240);box-shadow:` + decl +
		`}</style></head><body><div id="b"></div></body></html>`
}

func insetDisplayList(t *testing.T, decl string) []paint.DisplayCmd {
	t.Helper()
	return visDisplayList(t, insetFixture(200, 100, decl))
}

// insetRasterized builds the frozen list for `decl` and rasterizes the box's own
// w x h area exactly as the tile path does, so the answer is pixels rather than
// commands.
func insetRasterized(t *testing.T, w, h int, decl string) *frame.Bitmap {
	t.Helper()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("raster.NewFonts: %v", err)
	}
	sess, err := engine.NewSession(insetFixture(w, h, decl), nil, 800,
		engine.WithMetrics(fonts), engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()
	dl, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	bitmap := frame.NewBitmap(w, h)
	tile := dl.Build(1).Publish()
	if err := raster.RasterizeTile(tile, frame.Rect4(0, 0, int32(w), int32(h)), bitmap,
		fonts, raster.NewGlyphAtlas(1<<20, fonts), frame.RGB(240, 240, 240)); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	return bitmap
}

// insetAlpha reads one pixel back as the alpha the shadow carried there.
func insetAlpha(t *testing.T, bitmap *frame.Bitmap, x, y int) float64 {
	t.Helper()
	return float64(240-int(bitmap.At(x, y).R())) / 240
}

func diff(a, b float64) float64 {
	if a < b {
		return b - a
	}
	return a - b
}

// TestInsetShadowSweepsTheChromiumProfile is the pixel half of the defect: it
// walks each inset layer's band and reads it back, where the oracle it replaces
// only asserted that no ring appeared. The four declarations cover the four laws
// the builder implements: a signed per-side sharp depth (spread + the offset
// aimed at that edge), a Gaussian tail at sigma = blur/2, the same tail from a
// negative depth, and the corner's product of two tails.
//
// The tolerance is the residual actually measured against the readings above:
// mid-edge samples land within 0.017, corner samples within 0.034, because the
// separable product over-inks a translucent corner slightly (Chromium reads
// 0.708 at the `inset 0 0 40px` corner, the model 0.742). The blur-free case is
// exact: a hard edge either carries the declared alpha or carries nothing.
//
// qc:BR-277
func TestInsetShadowSweepsTheChromiumProfile(t *testing.T) {
	cases := []struct {
		decl             string
		midL, midR, midT []float64
		diag             []float64
		edgeTol          float64
	}{
		{
			// Chromium: 6px of 0.5 along the left and top, nothing along the right
			// and bottom, and the 6x6 corner at the same 0.5 rather than the 0.75
			// two stacked bands would composite.
			decl: `inset 4px 4px 0 2px rgba(0,0,255,0.5)`,
			midL: []float64{0.500, 0.500, 0.500, 0.500, 0.500, 0.500, 0, 0, 0, 0},
			midR: []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			midT: []float64{0.500, 0.500, 0.500, 0.500, 0.500, 0.500, 0, 0, 0, 0},
			diag: []float64{0.500, 0.500, 0.500, 0.500, 0.500, 0.500, 0, 0},
			// The blur-free bands are exact, so a miss here is a wrong depth, not
			// a fit error.
			edgeTol: 0.004,
		},
		{
			// Chromium: no offset and no spread leaves a symmetric 0.221 at each
			// edge, dying over 8px; the corner is the product of two tails.
			decl: `inset 0 0 8px rgba(0,0,255,0.5)`,
			midL: []float64{0.221, 0.183, 0.142, 0.108, 0.075, 0.054, 0.033, 0.021, 0.008, 0.004},
			midR: []float64{0.221, 0.183, 0.142, 0.108, 0.075, 0.054, 0.033, 0.021, 0.008, 0.004},
			midT: []float64{0.221, 0.183, 0.142, 0.108, 0.075, 0.054, 0.033, 0.021, 0.008, 0.004},
			diag: []float64{0.338, 0.296, 0.246, 0.188, 0.138, 0.096, 0.062, 0.037},
		},
		{
			// Chromium: the offset's own edge carries 0.442 at its border, the
			// opposite edge only the 0.142 a negative depth leaves behind, and the
			// un-offset pair sits at 0.312.
			decl: `inset 4px 0 8px 2px rgba(0,0,255,0.5)`,
			midL: []float64{0.442, 0.425, 0.396, 0.362, 0.317, 0.275, 0.229, 0.188, 0.142, 0.108},
			midR: []float64{0.142, 0.108, 0.075, 0.054, 0.033, 0.021, 0.008, 0.004, 0, 0},
			midT: []float64{0.312, 0.271, 0.229, 0.188, 0.142, 0.108, 0.075, 0.054, 0.033, 0.021},
			diag: []float64{0.467, 0.458, 0.442, 0.412, 0.371, 0.321, 0.267, 0.217},
		},
		{
			// Chromium: a blur wider than the box's half-height never reaches the
			// declared alpha, and stays within 0.16 of 0.47 across 10px.
			decl: `inset 0 0 40px rgb(0,0,255)`,
			midL: []float64{0.475, 0.458, 0.442, 0.425, 0.408, 0.388, 0.371, 0.354, 0.333, 0.317},
			midR: []float64{0.475, 0.458, 0.442, 0.425, 0.408, 0.388, 0.371, 0.354, 0.333, 0.317},
			midT: []float64{0.475, 0.458, 0.442, 0.425, 0.408, 0.388, 0.371, 0.354, 0.333, 0.317},
			diag: []float64{0.708, 0.692, 0.679, 0.662, 0.642, 0.625, 0.600, 0.579},
		},
	}
	for _, tc := range cases {
		bitmap := insetRasterized(t, 200, 100, tc.decl)
		tol := tc.edgeTol
		if tol == 0 {
			tol = 0.02
		}
		profiles := []struct {
			name  string
			at    func(int) (int, int)
			want  []float64
			toler float64
		}{
			{"midL", func(i int) (int, int) { return i, 50 }, tc.midL, tol},
			{"midR", func(i int) (int, int) { return 199 - i, 50 }, tc.midR, tol},
			{"midT", func(i int) (int, int) { return 100, i }, tc.midT, tol},
			{"diag", func(i int) (int, int) { return i, i }, tc.diag, 0.04},
		}
		for _, p := range profiles {
			for i, want := range p.want {
				x, y := p.at(i)
				got := insetAlpha(t, bitmap, x, y)
				if d := diff(got, want); d > p.toler {
					t.Errorf("%s: alpha at (%d,%d) = %.3f, Chromium measured %.3f, off by %.3f (tol %.3f)",
						tc.decl, x, y, got, want, d, p.toler)
				}
			}
		}
	}
}

func insetCells(cmds []paint.DisplayCmd) (ramps, masked []paint.DisplayCmd) {
	for _, c := range cmds {
		switch {
		case c.Kind == paint.CmdGradient:
			ramps = append(ramps, c)
		case c.Kind == paint.CmdFill && c.Mask != nil:
			masked = append(masked, c)
		}
	}
	return ramps, masked
}

// TestInsetShadowCellsTileThePaddingBox is the structural half. A blurred layer
// goes out as four edge ramps that stop short of the corners plus one masked
// fill per corner, so no pixel is inked twice and no cell leaves the padding
// box - the interior clip the defect asked for, and the reason a translucent
// inset layer no longer composites its own alpha twice where two bands met.
//
// The rects are what the law predicts for `inset 4px 0 8px 2px` at scale 1:
// depths left 6, right -2, top 2, bottom 2 and sigma 4, so each cell reaches
// depth + 2*sigma and the bands trim each other to (0,10)-(14,90) and friends.
//
// qc:BR-277
func TestInsetShadowCellsTileThePaddingBox(t *testing.T) {
	cmds := insetDisplayList(t, `inset 4px 0 8px 2px rgba(0,0,255,0.5)`)
	wantRamps := []struct {
		rect  frame.Rect
		angle float32
	}{
		{frame.Rect4(0, 10, 14, 90), 90},
		{frame.Rect4(194, 10, 200, 90), 270},
		{frame.Rect4(14, 0, 194, 10), 180},
		{frame.Rect4(14, 90, 194, 100), 0},
	}
	wantCorners := []frame.Rect{
		frame.Rect4(0, 0, 14, 10),
		frame.Rect4(194, 0, 200, 10),
		frame.Rect4(0, 90, 14, 100),
		frame.Rect4(194, 90, 200, 100),
	}
	ramps, corners := insetCells(cmds)
	if len(ramps) != len(wantRamps) {
		t.Fatalf("inset ramps = %d %v, want %d", len(ramps), rampRects(ramps), len(wantRamps))
	}
	if len(corners) != len(wantCorners) {
		t.Fatalf("masked corner fills = %d %v, want %d", len(corners), fillRects(corners), len(wantCorners))
	}
	for i, w := range wantRamps {
		if ramps[i].Rect != w.rect {
			t.Errorf("ramp %d rect = %v, want %v", i, ramps[i].Rect, w.rect)
		}
		if ramps[i].Gradient.Angle != w.angle {
			t.Errorf("ramp %d angle = %v, want %v", i, ramps[i].Gradient.Angle, w.angle)
		}
		if len(ramps[i].Gradient.Stops) != 5 {
			t.Errorf("ramp %d has %d stops, want the 5-sample fitted falloff", i, len(ramps[i].Gradient.Stops))
		}
	}
	for i, w := range wantCorners {
		if corners[i].Rect != w {
			t.Errorf("corner %d rect = %v, want %v", i, corners[i].Rect, w)
		}
	}
	// The eight cells must not overlap: that double-composite is the defect.
	cells := append(append([]paint.DisplayCmd{}, ramps...), corners...)
	padding := frame.Rect4(0, 0, 200, 100)
	for i, a := range cells {
		if a.Rect.Intersection(padding) != a.Rect {
			t.Errorf("cell %d %v escapes the padding box %v", i, a.Rect, padding)
		}
		for j := i + 1; j < len(cells); j++ {
			if area := intersectArea(a.Rect, cells[j].Rect); area != 0 {
				t.Errorf("cells %d %v and %d %v overlap on %d px, which composites the layer twice",
					i, a.Rect, j, cells[j].Rect, area)
			}
		}
	}
}

func rampRects(cmds []paint.DisplayCmd) []frame.Rect {
	out := make([]frame.Rect, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, c.Rect)
	}
	return out
}

func fillRects(cmds []paint.DisplayCmd) []frame.Rect { return rampRects(cmds) }

// TestInsetShadowPaintsOverBackgroundUnderBorder pins the layer's place in the
// paint order: CSS Backgrounds §4.4 puts an inset shadow above the background it
// was drawn on and below the border, which is the only order in which a
// translucent border can hide it (the clipping oracle above).
//
// qc:BR-277
func TestInsetShadowPaintsOverBackgroundUnderBorder(t *testing.T) {
	const html = `<!doctype html><html><head><style>html,body{margin:0}` +
		`#b{position:absolute;left:20px;top:20px;width:120px;height:60px;` +
		`background:rgb(0,200,0);border:4px solid rgb(255,0,0);` +
		`box-shadow:inset 0 0 8px rgba(128,0,128,0.5)}` +
		`</style></head><body><div id="b"></div></body></html>`
	cmds := visDisplayList(t, html)
	var bg, border = -1, -1
	for i, c := range cmds {
		if c.Kind == paint.CmdFill && c.Color == frame.RGB(0, 200, 0) && bg < 0 {
			bg = i
		}
		if c.Kind == paint.CmdBorder && border < 0 {
			border = i
		}
	}
	if bg < 0 || border < 0 {
		t.Fatalf("missing the box's background (%d) or border (%d)", bg, border)
	}
	ramps, corners := 0, 0
	for _, c := range cmds[bg+1 : border] {
		switch {
		case c.Kind == paint.CmdGradient:
			ramps++
		case c.Kind == paint.CmdFill && c.Mask != nil:
			corners++
		default:
			t.Errorf("command %v (%s) sits between the background and the border, want only inset cells",
				c.Rect, c.Kind)
		}
	}
	if ramps != 4 || corners != 4 {
		t.Errorf("inset ink between background and border = %d ramps and %d corners, want 4 and 4", ramps, corners)
	}
}

// TestInsetShadowCrossingBandsSweepTheChromiumProfile is the same pixel oracle
// for the boxes whose opposite bands overlap - a 12px-wide box with an 8px blur
// reaches 8px in from each side and meets in the middle - where the strip
// partition leaves no middle cell and the stacked full-length ramps it fell back
// on over-inked every pixel two bands crossed: Chromium reads 0.400 at the top
// of n2's mid column where the ramps measured 0.479.
//
// The tolerance is wider than the 200x100 sweep's because these samples are the
// far tail of the Gaussian. Skia keeps slightly more ink three standard
// deviations out than sigma = blur/2 predicts, which reads as Goosie 0.067
// against Chromium's 0.079 in the middle of a crossing box - an under-ink of
// 0.017 where the ramps over-inked by 0.079.
//
// qc:BR-277
func TestInsetShadowCrossingBandsSweepTheChromiumProfile(t *testing.T) {
	cases := []struct {
		w, h   int
		decl   string
		rowMid []float64
		colMid []float64
	}{
		{12, 60, `inset 0 0 8px rgba(0,0,255,0.5)`,
			[]float64{0.221, 0.183, 0.146, 0.117, 0.092, 0.079, 0.079, 0.092, 0.117, 0.146, 0.183, 0.221},
			[]float64{0.263, 0.233, 0.196, 0.171, 0.142, 0.125, 0.108, 0.096, 0.087, 0.083, 0.079, 0.079}},
		{12, 60, `inset 4px 0 8px 2px rgba(0,0,255,0.5)`,
			[]float64{0.442, 0.425, 0.396, 0.362, 0.321, 0.283, 0.250, 0.217, 0.192, 0.179, 0.179, 0.192},
			[]float64{0.400, 0.379, 0.362, 0.342, 0.317, 0.300, 0.283, 0.271, 0.263, 0.254, 0.250, 0.250}},
		// 30x20 with a 6px blur stops one pixel short of crossing, so it keeps
		// the strip partition and the corners' own cells.
		{30, 20, `inset 3px 3px 6px rgba(0,0,255,0.5)`,
			[]float64{0.379, 0.338, 0.283, 0.225, 0.167, 0.117, 0.071, 0.046, 0.025, 0.013, 0.004, 0.004},
			[]float64{0.379, 0.338, 0.283, 0.221, 0.167, 0.117, 0.071, 0.042, 0.021, 0.008, 0.004, 0.000}},
	}
	for _, tc := range cases {
		bitmap := insetRasterized(t, tc.w, tc.h, tc.decl)
		sweeps := []struct {
			name  string
			at    func(int) (int, int)
			want  []float64
			toler float64
		}{
			{"row-mid", func(i int) (int, int) { return i, tc.h / 2 }, tc.rowMid, 0.03},
			{"col-mid", func(i int) (int, int) { return tc.w / 2, i }, tc.colMid, 0.03},
		}
		for _, p := range sweeps {
			for i, want := range p.want {
				x, y := p.at(i)
				got := insetAlpha(t, bitmap, x, y)
				if d := diff(got, want); d > p.toler {
					t.Errorf("%s %s: alpha at (%d,%d) = %.3f, Chromium measured %.3f, off by %.3f (tol %.3f)",
						tc.decl, p.name, x, y, got, want, d, p.toler)
				}
			}
		}
	}
}

// TestInsetShadowCrossingBandsMaskTheWholeBox is the structural half of the
// crossing case: with no middle cell left to ink, the field goes out as masked
// cells over the whole padding box - one per fieldMaskCap block - and no ramp at
// all, so no pixel is composited twice.
//
// qc:BR-277
func TestInsetShadowCrossingBandsMaskTheWholeBox(t *testing.T) {
	cases := []struct {
		w, h int
		decl string
		want []frame.Rect
	}{
		{
			w: 12, h: 60, decl: `inset 0 0 8px rgba(0,0,255,0.5)`,
			want: []frame.Rect{frame.Rect4(0, 0, 12, 60)},
		},
		{
			// 200px of height is three full 64px blocks and a remainder, which is
			// how the tiling reaches the box's last row.
			w: 12, h: 200, decl: `inset 4px 0 8px 2px rgba(0,0,255,0.5)`,
			want: []frame.Rect{
				frame.Rect4(0, 0, 12, 64),
				frame.Rect4(0, 64, 12, 128),
				frame.Rect4(0, 128, 12, 192),
				frame.Rect4(0, 192, 12, 200),
			},
		},
	}
	for _, tc := range cases {
		cmds := visDisplayList(t, insetFixture(tc.w, tc.h, tc.decl))
		ramps, masked := insetCells(cmds)
		if len(ramps) != 0 {
			t.Errorf("%s crossed the box yet emitted %d ramps %v, want the masked cells alone",
				tc.decl, len(ramps), rampRects(ramps))
		}
		if len(masked) != len(tc.want) {
			t.Fatalf("%s emitted %d masked cells %v, want %d %v",
				tc.decl, len(masked), fillRects(masked), len(tc.want), tc.want)
		}
		box := frame.Rect4(0, 0, int32(tc.w), int32(tc.h))
		covered := int32(0)
		for i, c := range masked {
			if c.Rect != tc.want[i] {
				t.Errorf("cell %d rect = %v, want %v", i, c.Rect, tc.want[i])
			}
			covered += c.Rect.W() * c.Rect.H()
			for j := i + 1; j < len(masked); j++ {
				if area := intersectArea(c.Rect, masked[j].Rect); area != 0 {
					t.Errorf("cells %d and %d overlap on %d px, which composites the layer twice", i, j, area)
				}
			}
		}
		if covered != box.W()*box.H() {
			t.Errorf("%s cells cover %d px of a %d px box, want the padding box exactly", tc.decl, covered, box.W()*box.H())
		}
	}
}

// TestInsetShadowStopsMaskingPastItsBudget pins the admission limit: a crossing
// box too large to mask pixel by pixel falls back to the four stacked ramps
// rather than paying for the whole interior.
//
// qc:BR-277
func TestInsetShadowStopsMaskingPastItsBudget(t *testing.T) {
	cmds := visDisplayList(t, insetFixture(400, 400, `inset 0 0 200px rgba(0,0,255,0.5)`))
	ramps, masked := insetCells(cmds)
	if len(masked) != 0 {
		t.Fatalf("a 160000 px crossing box emitted %d masked cells %v, want the budget to hold it to ramps",
			len(masked), fillRects(masked))
	}
	if len(ramps) != 4 {
		t.Errorf("budget fallback emitted %d ramps %v, want the four stacked edge ramps", len(ramps), rampRects(ramps))
	}
}
