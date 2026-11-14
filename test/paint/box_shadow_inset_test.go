package paint_test

import (
	"strconv"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

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

// qc:BR-277
func TestInsetShadowSweepsTheChromiumProfile(t *testing.T) {
	cases := []struct {
		decl             string
		midL, midR, midT []float64
		diag             []float64
		edgeTol          float64
	}{
		{
			decl:    `inset 4px 4px 0 2px rgba(0,0,255,0.5)`,
			midL:    []float64{0.500, 0.500, 0.500, 0.500, 0.500, 0.500, 0, 0, 0, 0},
			midR:    []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			midT:    []float64{0.500, 0.500, 0.500, 0.500, 0.500, 0.500, 0, 0, 0, 0},
			diag:    []float64{0.500, 0.500, 0.500, 0.500, 0.500, 0.500, 0, 0},
			edgeTol: 0.004,
		},
		{
			decl: `inset 0 0 8px rgba(0,0,255,0.5)`,
			midL: []float64{0.221, 0.183, 0.142, 0.108, 0.075, 0.054, 0.033, 0.021, 0.008, 0.004},
			midR: []float64{0.221, 0.183, 0.142, 0.108, 0.075, 0.054, 0.033, 0.021, 0.008, 0.004},
			midT: []float64{0.221, 0.183, 0.142, 0.108, 0.075, 0.054, 0.033, 0.021, 0.008, 0.004},
			diag: []float64{0.338, 0.296, 0.246, 0.188, 0.138, 0.096, 0.062, 0.037},
		},
		{
			decl: `inset 4px 0 8px 2px rgba(0,0,255,0.5)`,
			midL: []float64{0.442, 0.425, 0.396, 0.362, 0.317, 0.275, 0.229, 0.188, 0.142, 0.108},
			midR: []float64{0.142, 0.108, 0.075, 0.054, 0.033, 0.021, 0.008, 0.004, 0, 0},
			midT: []float64{0.312, 0.271, 0.229, 0.188, 0.142, 0.108, 0.075, 0.054, 0.033, 0.021},
			diag: []float64{0.467, 0.458, 0.442, 0.412, 0.371, 0.321, 0.267, 0.217},
		},
		{
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
