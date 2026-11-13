package paint_test

import (
	"strconv"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

// A blurred outer box-shadow is a Gaussian field, not a stack of hard rings:
// blurring a rect convolves it with a separable kernel, so the pixel whose centre
// sits at p keeps Phi((hi-p)/sigma) - Phi((lo-p)/sigma) of the declared alpha on
// each axis - sigma = blur/2, lo/hi the shadow rect's edges after offset and
// spread - and the two axes multiply. Ink stays outside the border box, and the
// first declared layer ends up on top of the later ones.
//
// Every expectation below is Chromium's own reading, taken on 2026-11-13 from the
// one-box pages in /tmp/sb3 (560x400 viewport at deviceScaleFactor 1, a
// rgb(240,240,240) page behind rgb(240,240,240) boxes, screenshots read by
// /tmp/sb3/dump.py). Alpha comes from the red channel as (240-R)/240, 240 levels
// per unit, the finest signal a pure-blue shadow gives. The box sits at
// (200,150) so a sweep's pixel phase matches the reading's, and `right`/`left`
// walk from its border box rather than from the shadow rect: k=0 is the first
// pixel outside the element.

const (
	shadowBoxX = 200
	shadowBoxY = 150
)

// shadowFixture lays one box over a background the same colour as its own, so the
// only thing that moves a pixel is the shadow.
func shadowFixture(boxCSS, decl string) string {
	return `<!doctype html><html><head><style>html,body{margin:0;background:rgb(240,240,240)}` +
		`#b{position:absolute;left:` + strconv.Itoa(shadowBoxX) + `px;top:` + strconv.Itoa(shadowBoxY) +
		`px;width:120px;height:60px;background:rgb(240,240,240);` + boxCSS +
		`box-shadow:` + decl +
		`}</style></head><body><div id="b"></div></body></html>`
}

// shadowRasterized builds the frozen list for one box and rasterizes the whole
// 560x400 viewport, which is how wide a blurred field has to be read: the sweep
// walks up to 24px past a blur-16 box and 60px past a blur-40 one.
func shadowRasterized(t *testing.T, boxCSS, decl string) *frame.Bitmap {
	t.Helper()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("raster.NewFonts: %v", err)
	}
	sess, err := engine.NewSession(shadowFixture(boxCSS, decl), nil, 560,
		engine.WithMetrics(fonts), engine.WithViewportH(400))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()
	dl, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	bitmap := frame.NewBitmap(560, 400)
	tile := dl.Build(1).Publish()
	if err := raster.RasterizeTile(tile, frame.Rect4(0, 0, 560, 400), bitmap,
		fonts, raster.NewGlyphAtlas(1<<20, fonts), frame.RGB(240, 240, 240)); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	return bitmap
}

// shadowEdge names one sweep: outward from an edge of the border box, or from a
// corner along the diagonal.
type shadowEdge string

const (
	edgeRight shadowEdge = "right"
	edgeLeft  shadowEdge = "left"
	edgeBelow shadowEdge = "below"
	edgeAbove shadowEdge = "above"
	edgeDiag  shadowEdge = "diag"
)

// shadowPoint is the k-th pixel of a sweep, k=0 being just outside the box.
func shadowPoint(edge shadowEdge, k, w, h int) (int, int) {
	x1, y1 := shadowBoxX+w, shadowBoxY+h
	xmid, ymid := shadowBoxX+w/2, shadowBoxY+h/2
	switch edge {
	case edgeLeft:
		return shadowBoxX - 1 - k, ymid
	case edgeBelow:
		return xmid, y1 + k
	case edgeAbove:
		return xmid, shadowBoxY - 1 - k
	case edgeDiag:
		return x1 + k, y1 + k
	default:
		return x1 + k, ymid
	}
}

func shadowAlpha(t *testing.T, bm *frame.Bitmap, edge shadowEdge, k, w, h int) float64 {
	t.Helper()
	x, y := shadowPoint(edge, k, w, h)
	return float64(240-int(bm.At(x, y).R())) / 240
}

func shadowRGB(bm *frame.Bitmap, x, y int) (int, int, int) {
	c := bm.At(x, y)
	return int(c.R()), int(c.G()), int(c.B())
}

type blurProfile struct {
	edge shadowEdge
	want []float64
}

// TestBlurredOuterShadowSweepsTheChromiumProfile is the defect's pixel oracle:
// the engine used to answer a blur with up to four concentric hard rings, which
// put 0.83 of the declared alpha on the pixel beside a blur-8 box where Chromium
// reads 0.229, and left a blur-16 box ringed by nothing at all past its fourth
// layer. Each case walks the falloff Chromium paints and reads the same pixels
// back; the four sweeps of a symmetric box agree to the level, which is what
// catches a field that only got one axis right.
//
// The tolerance is 0.02 - 4.8 levels of the 240-step ramp - because that is the
// residual the separable model has against the readings above: across the 2,261
// pixels this table and its source sweep cover, the worst single pixel is 0.0132
// and the rms is 0.0036. A hard layer is exact, so it gets 0.004.
//
// qc:BR-281
func TestBlurredOuterShadowSweepsTheChromiumProfile(t *testing.T) {
	cases := []struct {
		name   string
		boxCSS string
		decl   string
		w, h   int
		tol    float64
		profs  []blurProfile
	}{
		{
			// Chromium: one pixel of ink and then nothing. A blur of 1 is a
			// half-sigma tail, so the falloff is over before it starts.
			name: "blur 1",
			decl: `0 0 1px rgba(0,0,255,0.5)`,
			w:    120, h: 60,
			tol:   0.02,
			profs: []blurProfile{{edgeRight, []float64{0.0875, 0, 0, 0}}, {edgeDiag, []float64{0.0167, 0, 0}}},
		},
		{
			// Chromium: 0.163, 0.038, 0.004 - the steepest falloff that still
			// reads three pixels deep.
			name: "blur 2",
			decl: `0 0 2px rgba(0,0,255,0.5)`,
			w:    120, h: 60,
			tol:   0.02,
			profs: []blurProfile{{edgeRight, []float64{0.1625, 0.0375, 0.0042, 0}}, {edgeBelow, []float64{0.1625, 0.0375, 0.0042, 0}}, {edgeDiag, []float64{0.0542, 0.0042, 0}}},
		},
		{
			name: "blur 4",
			decl: `0 0 4px rgba(0,0,255,0.5)`,
			w:    120, h: 60,
			tol:   0.02,
			profs: []blurProfile{{edgeRight, []float64{0.2042, 0.1250, 0.0625, 0.0250, 0.0083, 0.0042, 0}}, {edgeDiag, []float64{0.0833, 0.0333, 0.0083, 0}}},
		},
		{
			name: "blur 8",
			decl: `0 0 8px rgba(0,0,255,0.5)`,
			w:    120, h: 60,
			tol: 0.02,
			profs: []blurProfile{
				{edgeRight, []float64{0.2292, 0.1875, 0.1417, 0.1042, 0.0708, 0.0500, 0.0292, 0.0167, 0.0083, 0.0042, 0.0042, 0.0042, 0, 0}},
				{edgeBelow, []float64{0.2292, 0.1875, 0.1417, 0.1042, 0.0708, 0.0500, 0.0292, 0.0167, 0.0083, 0.0042, 0.0042, 0.0042, 0, 0}},
				{edgeDiag, []float64{0.1083, 0.0667, 0.0417, 0.0208, 0.0125, 0.0042, 0.0042, 0}},
			},
		},
		{
			name: "blur 16",
			decl: `0 0 16px rgba(0,0,255,0.5)`,
			w:    120, h: 60,
			tol: 0.02,
			profs: []blurProfile{
				{edgeRight, []float64{0.2417, 0.2167, 0.1917, 0.1750, 0.1500, 0.1292, 0.1125, 0.0958, 0.0792, 0.0625, 0.0542, 0.0458, 0.0333, 0.0292, 0.0208, 0.0167, 0.0125, 0.0083, 0.0042, 0.0042, 0.0042, 0.0042, 0.0042, 0.0042}},
				{edgeDiag, []float64{0.1167, 0.0958, 0.0750, 0.0625, 0.0500, 0.0375, 0.0250, 0.0208, 0.0125, 0.0083, 0.0083, 0.0042, 0.0042, 0.0042, 0}},
			},
		},
		{
			// The wide blur is where the two axes differ: 60px past the box the
			// tail is still 0.05, and mid-height reads less than mid-width
			// because the 60px-tall box has given away part of its own profile.
			name: "blur 40",
			decl: `0 0 40px rgba(0,0,255,0.5)`,
			w:    120, h: 60,
			tol: 0.02,
			profs: []blurProfile{
				{edgeRight, []float64{0.2167, 0.2083, 0.2000, 0.1875, 0.1833, 0.1750, 0.1667, 0.1583, 0.1500, 0.1417, 0.1333, 0.1250, 0.1208, 0.1125, 0.1083, 0.1000, 0.0917, 0.0833, 0.0792, 0.0708, 0.0667, 0.0625, 0.0583, 0.0542}},
				{edgeBelow, []float64{0.2500, 0.2375, 0.2292, 0.2167, 0.2083, 0.1958, 0.1875, 0.1833, 0.1708, 0.1625, 0.1542, 0.1417, 0.1333, 0.1250, 0.1208, 0.1125, 0.1042, 0.0958, 0.0917, 0.0833, 0.0750, 0.0708, 0.0625, 0.0625}},
				{edgeDiag, []float64{0.1250, 0.1125, 0.1042, 0.0958, 0.0875, 0.0792, 0.0708, 0.0625, 0.0625, 0.0542, 0.0500, 0.0417, 0.0375, 0.0333, 0.0292, 0.0250, 0.0250, 0.0208, 0.0167, 0.0167, 0.0125, 0.0125, 0.0083, 0.0083}},
			},
		},
		{
			// A positive spread starts the field inside the box's edge, so the
			// first pixel past it still has most of the alpha.
			name: "blur 8 spread 4",
			decl: `0 0 8px 4px rgba(0,0,255,0.5)`,
			w:    120, h: 60,
			tol:   0.02,
			profs: []blurProfile{{edgeRight, []float64{0.4042, 0.3667, 0.3208, 0.2750, 0.2292, 0.1875, 0.1417, 0.1042, 0.0708, 0.0500, 0.0292, 0.0167, 0.0083, 0.0042, 0.0042, 0.0042, 0}}, {edgeDiag, []float64{0.3250, 0.2625, 0.2042, 0.1542, 0.1083, 0.0667, 0.0417, 0.0208, 0.0125, 0.0042, 0.0042, 0}}},
		},
		{
			// A negative spread pulls the shadow rect inside the box: 4px of the
			// falloff is already spent before the border, so the ring starts at
			// 0.071 instead of 0.229.
			name: "blur 8 spread -4",
			decl: `0 0 8px -4px rgba(0,0,255,0.5)`,
			w:    120, h: 60,
			tol:   0.02,
			profs: []blurProfile{{edgeRight, []float64{0.0708, 0.0500, 0.0292, 0.0167, 0.0083, 0.0042, 0.0042, 0.0042, 0, 0}}, {edgeDiag, []float64{0.0125, 0.0042, 0.0042, 0}}},
		},
		{
			// An offset is a spread that differs per side: the edge it points at
			// reads the spread-4 profile, the opposite edge the spread -4 one.
			name: "blur 8 offset 4,4",
			decl: `4px 4px 8px rgba(0,0,255,0.5)`,
			w:    120, h: 60,
			tol: 0.02,
			profs: []blurProfile{
				{edgeRight, []float64{0.4042, 0.3667, 0.3208, 0.2750, 0.2292, 0.1875, 0.1417, 0.1042, 0.0708, 0.0500, 0.0292, 0.0167, 0.0083, 0.0042, 0.0042, 0.0042, 0}},
				{edgeLeft, []float64{0.0708, 0.0500, 0.0292, 0.0167, 0.0083, 0.0042, 0.0042, 0.0042, 0}},
				{edgeBelow, []float64{0.4042, 0.3667, 0.3208, 0.2750, 0.2292, 0.1875, 0.1417, 0.1042, 0.0708, 0.0500, 0.0292, 0.0167, 0.0083, 0.0042, 0.0042, 0.0042, 0}},
				{edgeAbove, []float64{0.0708, 0.0500, 0.0292, 0.0167, 0.0083, 0.0042, 0.0042, 0.0042, 0}},
			},
		},
		{
			// An offset wider than the blur leaves the shadow rect clear of an
			// edge: the side it moved away from gets nothing at all, and the
			// side it moved toward keeps the declared alpha across the overlap.
			name: "blur 8 offset -20,0",
			decl: `-20px 0 8px rgba(0,0,255,0.5)`,
			w:    120, h: 60,
			tol: 0.02,
			profs: []blurProfile{
				{edgeRight, []float64{0, 0, 0, 0, 0, 0, 0, 0}},
				{edgeLeft, []float64{0.5000, 0.5000, 0.5000, 0.5000, 0.5000, 0.5000, 0.5000, 0.5000, 0.5000, 0.5000, 0.5000, 0.4958, 0.4875, 0.4750, 0.4583, 0.4333, 0.4042, 0.3667, 0.3208, 0.2750, 0.2292, 0.1875, 0.1417, 0.1042}},
				{edgeBelow, []float64{0.2292, 0.1875, 0.1417, 0.1042, 0.0708, 0.0500, 0.0292, 0.0167, 0.0083, 0.0042, 0.0042, 0.0042, 0}},
			},
		},
		{
			// A fully opaque layer reads the coverage itself, so the peak at the
			// border is 0.454 rather than half of it.
			name: "blur 8 alpha 1",
			decl: `0 0 8px rgba(0,0,255,1)`,
			w:    120, h: 60,
			tol:   0.02,
			profs: []blurProfile{{edgeRight, []float64{0.4542, 0.3667, 0.2792, 0.2000, 0.1417, 0.0917, 0.0583, 0.0333, 0.0167, 0.0083, 0.0042, 0.0042, 0}}, {edgeDiag, []float64{0.2083, 0.1333, 0.0792, 0.0417, 0.0208, 0.0083, 0.0042, 0}}},
		},
		{
			// A quarter-alpha layer is the same profile scaled: the falloff is a
			// property of the blur, not of how much ink the blur carries.
			name: "blur 4 spread 2 alpha .25",
			decl: `0 0 4px 2px rgba(0,0,255,0.25)`,
			w:    120, h: 60,
			tol:   0.02,
			profs: []blurProfile{{edgeRight, []float64{0.1917, 0.1500, 0.1042, 0.0625, 0.0292, 0.0125, 0.0042, 0}}, {edgeDiag, []float64{0.1458, 0.0917, 0.0417, 0.0167, 0.0042, 0}}},
		},
		{
			// A box shorter than the blur has already lost most of its vertical
			// profile by the time the field reaches the sides, so its left and
			// right rings are dimmer than its top and bottom ones.
			name:   "blur 8 on a 12px-tall box",
			boxCSS: `height:12px;`,
			decl:   `0 0 8px rgba(0,0,255,0.5)`,
			w:      120, h: 12,
			tol: 0.02,
			profs: []blurProfile{
				{edgeRight, []float64{0.1917, 0.1542, 0.1250, 0.0917, 0.0625, 0.0458, 0.0292, 0.0167, 0.0083, 0.0042, 0}},
				{edgeBelow, []float64{0.2292, 0.1875, 0.1417, 0.1083, 0.0750, 0.0542, 0.0333, 0.0208, 0.0083, 0.0042, 0}},
				{edgeDiag, []float64{0.1083, 0.0667, 0.0417, 0.0250, 0.0125, 0.0042, 0.0042, 0}},
			},
		},
		{
			name:   "blur 8 on a 10px-wide box",
			boxCSS: `width:10px;`,
			decl:   `0 0 8px rgba(0,0,255,0.5)`,
			w:      10, h: 60,
			tol: 0.02,
			profs: []blurProfile{
				{edgeRight, []float64{0.2292, 0.1875, 0.1417, 0.1083, 0.0750, 0.0542, 0.0333, 0.0208, 0.0083, 0.0042, 0}},
				{edgeBelow, []float64{0.1750, 0.1417, 0.1125, 0.0792, 0.0583, 0.0417, 0.0250, 0.0167, 0.0083, 0.0042, 0}},
			},
		},
		{
			// Blur 0 is the shape the ring path already owned: 4px at the declared
			// alpha and nothing beyond, on the diagonal too.
			name: "hard spread 4",
			decl: `0 0 0 4px rgba(0,0,255,0.5)`,
			w:    120, h: 60,
			tol:   0.004,
			profs: []blurProfile{{edgeRight, []float64{0.5, 0.5, 0.5, 0.5, 0, 0}}, {edgeDiag, []float64{0.5, 0.5, 0.5, 0.5, 0, 0}}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bm := shadowRasterized(t, c.boxCSS, c.decl)
			worst, worstAt := 0.0, ""
			for _, p := range c.profs {
				for k, want := range p.want {
					got := shadowAlpha(t, bm, p.edge, k, c.w, c.h)
					if d := diff(got, want); d > worst {
						worst, worstAt = d, string(p.edge)+" at k="+strconv.Itoa(k)
					}
					if d := diff(got, want); d > c.tol {
						t.Errorf("%s sweep pixel %d = %.4f, want %.4f (+-%.3f)", p.edge, k, got, want, c.tol)
					}
				}
			}
			t.Logf("%s: worst pixel %.4f off at %s (%d samples)", c.name, worst, worstAt,
				func() int {
					n := 0
					for _, p := range c.profs {
						n += len(p.want)
					}
					return n
				}())
		})
	}
}

// TestBlurredOuterShadowInksNothingInsideTheBorderBox is the clip: CSS Backgrounds
// §7.1.1 puts an outer shadow behind the element, and Chromium's interior reads
// the element's own background at every blur. The overlapping cases are the ones
// that catch a leaked fill - a shadow rect grown past the border by spread, or
// slid under the box by an offset, still has to stop at the border.
//
// qc:BR-281
func TestBlurredOuterShadowInksNothingInsideTheBorderBox(t *testing.T) {
	cases := []struct {
		name   string
		boxCSS string
		decl   string
		w, h   int
	}{
		{"blur 8", "", `0 0 8px rgba(0,0,255,0.5)`, 120, 60},
		{"blur 40", "", `0 0 40px rgba(0,0,255,0.5)`, 120, 60},
		{"blur 8 spread 4", "", `0 0 8px 4px rgba(0,0,255,0.5)`, 120, 60},
		{"blur 8 offset -20,0", "", `-20px 0 8px rgba(0,0,255,0.5)`, 120, 60},
		{"blur 8 alpha 1", "", `0 0 8px rgba(0,0,255,1)`, 120, 60},
	}
	// Centre, 1px inside each edge and 3px inside a corner - the pixels a field
	// that forgot its clip would ink darkest.
	spots := [][2]int{{0, 0}, {-1, 0}, {1, 0}, {0, -1}, {0, 1}, {-3, -3}, {3, 3}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bm := shadowRasterized(t, c.boxCSS, c.decl)
			cx, cy := shadowBoxX+c.w/2, shadowBoxY+c.h/2
			for _, s := range spots {
				x, y := cx+s[0], cy+s[1]
				alpha := float64(240-int(bm.At(x, y).R())) / 240
				if alpha > 0.004 {
					t.Errorf("pixel (%d,%d) inside the box reads alpha %.4f: an outer shadow paints behind its element", x, y, alpha)
				}
			}
		})
	}
}

// TestCollapsedNegativeSpreadInksNothing is the far half of a negative spread:
// past the point where the shadow rect stops being an area, Chromium paints
// nothing at all. Measured on a 120x60 box at (200,150) - `-30px` leaves a 60x0
// rect, `-40px` a 40x-20 one and `-70px` a collapsed point - each reading the
// whole 560x400 page at rgb(240,240,240), and the hard `0 0 0 -4px` likewise. An
// inverted rect must not reach the falloff formula either, whose CDF difference
// goes negative there and would wrap a mask byte.
//
// qc:BR-281
func TestCollapsedNegativeSpreadInksNothing(t *testing.T) {
	for _, decl := range []string{
		`0 0 16px -30px rgba(0,0,255,0.5)`,
		`0 0 16px -40px rgba(0,0,255,0.5)`,
		`0 0 16px -70px rgba(0,0,255,0.5)`,
		`0 0 0 -4px rgba(0,0,255,0.5)`,
	} {
		bm := shadowRasterized(t, "", decl)
		for y := 0; y < 400; y++ {
			for x := 0; x < 560; x++ {
				if r := int(bm.At(x, y).R()); r != 240 {
					t.Fatalf("%s inked (%d,%d) to R=%d: a shadow with no area paints nothing", decl, x, y, r)
				}
			}
		}
	}
}

// TestTwoLayerShadowStacksFirstDeclaredOnTop is the stacking law: CSS paints the
// first declared layer over the later ones, which a hard pair shows without any
// blur to hide behind. `0 0 0 10px red, 0 0 0 4px green` reads red across all
// 10px in Chromium; declared the other way round, green holds its 4px and red
// takes the rest.
//
// qc:BR-281
func TestTwoLayerShadowStacksFirstDeclaredOnTop(t *testing.T) {
	cases := []struct {
		name string
		decl string
		want [][3]int // k=0..11, the first 12 pixels right of the box
	}{
		{
			name: "wide red first",
			decl: `0 0 0 10px rgb(255,0,0), 0 0 0 4px rgb(0,255,0)`,
			want: [][3]int{{255, 0, 0}, {255, 0, 0}, {255, 0, 0}, {255, 0, 0}, {255, 0, 0}, {255, 0, 0}, {255, 0, 0}, {255, 0, 0}, {255, 0, 0}, {255, 0, 0}, {240, 240, 240}, {240, 240, 240}},
		},
		{
			name: "narrow green first",
			decl: `0 0 0 4px rgb(0,255,0), 0 0 0 10px rgb(255,0,0)`,
			want: [][3]int{{0, 255, 0}, {0, 255, 0}, {0, 255, 0}, {0, 255, 0}, {255, 0, 0}, {255, 0, 0}, {255, 0, 0}, {255, 0, 0}, {255, 0, 0}, {255, 0, 0}, {240, 240, 240}, {240, 240, 240}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bm := shadowRasterized(t, "", c.decl)
			for k, want := range c.want {
				x, y := shadowPoint(edgeRight, k, 120, 60)
				r, g, b := shadowRGB(bm, x, y)
				if got := [3]int{r, g, b}; got != want {
					t.Errorf("pixel %d right of the box = %v, want %v", k, got, want)
				}
			}
		})
	}
}

// TestTwoBlurredLayersCompositeTheTwoFields is the stacking law where both layers
// are blurred: each keeps its own falloff and the pixel is what the two leave of
// each other, 1-(1-a1)(1-a2). The readings below are Chromium's, and the pairs
// are the declared layers - a blur-8 half-alpha over a blur-16 quarter-alpha, and
// an offset blur-16 over an unoffset blur-4.
//
// qc:BR-281
func TestTwoBlurredLayersCompositeTheTwoFields(t *testing.T) {
	cases := []struct {
		name  string
		decl  string
		profs []blurProfile
	}{
		{
			name:  "blur 8 over blur 16",
			decl:  `0 0 8px rgba(0,0,255,0.5), 0 0 16px rgba(0,0,255,0.25)`,
			profs: []blurProfile{{edgeRight, []float64{0.3208, 0.2750, 0.2250, 0.1833, 0.1417, 0.1083, 0.0875, 0.0667, 0.0500, 0.0375, 0.0292, 0.0250, 0.0167, 0.0125, 0.0083, 0.0083, 0.0042, 0.0042, 0}}, {edgeDiag, []float64{0.1583, 0.1167, 0.0792, 0.0500, 0.0375, 0.0208, 0.0167, 0.0083, 0.0042, 0.0042, 0.0042, 0}}},
		},
		{
			name:  "offset blur 16 over blur 4",
			decl:  `4px 4px 16px rgba(0,0,255,0.5), 0 0 4px rgba(0,0,255,0.25)`,
			profs: []blurProfile{{edgeRight, []float64{0.4042, 0.3542, 0.3083, 0.2750, 0.2417, 0.2167, 0.1917, 0.1750, 0.1500, 0.1292, 0.1125, 0.0958, 0.0792, 0.0625, 0.0542, 0.0458, 0.0333, 0.0292, 0.0208, 0.0167, 0.0125, 0.0083, 0.0042, 0.0042}}, {edgeLeft, []float64{0.2375, 0.1875, 0.1375, 0.1083, 0.0833, 0.0625, 0.0542, 0.0458, 0.0333, 0.0292, 0.0208, 0.0167, 0.0125, 0.0083, 0.0042, 0.0042, 0.0042, 0.0042, 0.0042, 0.0042, 0}}, {edgeAbove, []float64{0.2375, 0.1875, 0.1375, 0.1083, 0.0833, 0.0625, 0.0542, 0.0458, 0.0333, 0.0292, 0.0208, 0.0167, 0.0125, 0.0083, 0.0042, 0.0042, 0.0042, 0.0042, 0.0042, 0.0042, 0}}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bm := shadowRasterized(t, "", c.decl)
			for _, p := range c.profs {
				for k, want := range p.want {
					if got := shadowAlpha(t, bm, p.edge, k, 120, 60); diff(got, want) > 0.02 {
						t.Errorf("%s sweep pixel %d = %.4f, want %.4f (+-0.02)", p.edge, k, got, want)
					}
				}
			}
		})
	}
}

// shadowMaskedFills returns the masked fills of a declaration's list and the
// widest rect any plain fill of it covers.
func shadowMaskedFills(t *testing.T, decl string) []paint.DisplayCmd {
	t.Helper()
	var out []paint.DisplayCmd
	for _, c := range visDisplayList(t, shadowFixture("", decl)) {
		if c.Kind == paint.CmdFill && c.Mask != nil {
			out = append(out, c)
		}
	}
	return out
}

// TestBlurredOuterFieldMasksStayOutsideTheBorderBox is the structural half of the
// field: a blurred layer now goes out as masked cells rather than as rings, so
// the clip is only as good as the rects the builder hands them. None may reach
// inside the border box, and none may be larger than one mask cap, which is what
// bounds the memory a blurred shadow can ask the rasterizer for.
//
// qc:BR-281
func TestBlurredOuterFieldMasksStayOutsideTheBorderBox(t *testing.T) {
	borderBox := frame.Rect4(shadowBoxX, shadowBoxY, shadowBoxX+120, shadowBoxY+60)
	for _, decl := range []string{
		`0 0 8px rgba(0,0,255,0.5)`,
		`0 0 40px rgba(0,0,255,0.5)`,
		`4px 4px 16px rgba(0,0,255,0.5), 0 0 4px rgba(0,0,255,0.25)`,
	} {
		fills := shadowMaskedFills(t, decl)
		if len(fills) == 0 {
			t.Fatalf("%s emitted no masked fill: a blurred layer is the field, not a ring", decl)
		}
		for _, c := range fills {
			if area := intersectArea(c.Rect, borderBox); area != 0 {
				t.Errorf("%s masked cell %v covers %d px of the border box", decl, c.Rect, area)
			}
			if w, h := int(c.Rect.W()), int(c.Rect.H()); w > 64 || h > 64 {
				t.Errorf("%s masked cell %v is past the 64px cap", decl, c.Rect)
			}
			b := c.Mask.Bounds()
			if b.Dx() != int(c.Rect.W()) || b.Dy() != int(c.Rect.H()) {
				t.Errorf("%s masked cell %v carries a %dx%d mask: the field is exact per pixel", decl, c.Rect, b.Dx(), b.Dy())
			}
		}
	}
}

// TestRoundedBlurredShadowKeepsTheRingApproximation pins the exception rather
// than the rule: a masked fill drops the corner radius, so a rounded box cannot
// take the field path and still gets concentric rings. The residue is measured,
// not assumed - on the same 120x60 box with `border-radius:20px` and `0 0 8px
// rgba(0,0,255,0.5)`, Chromium reads 0.2292, 0.1875, 0.1417, 0.1083, 0.0750,
// 0.0542 along the first six pixels right of the border box at mid-height and
// 0.0083 at the 45-degree point just past the 20px arc, while the rings put
// 0.7958, 0.7958, 0.7667, 0.7667, 0.6875, 0.6875 and 0.5 there. The worst pixel
// of a 210x130 band around the box is 0.7167 at (320,204), inside the corner's
// square but outside its arc. That gap is GOOSIE-SHADOWARC-001, and the fact
// that pre- and post-fix renders agree to the level is this slice's own bound:
// the field BR-281 adds never runs for a rounded layer.
//
// qc:BR-281
func TestRoundedBlurredShadowKeepsTheRingApproximation(t *testing.T) {
	decl := `0 0 8px rgba(0,0,255,0.5)`
	plain := shadowMaskedFills(t, decl)
	if len(plain) == 0 {
		t.Fatal("the square box stopped emitting a masked field")
	}
	var rounded []paint.DisplayCmd
	for _, c := range visDisplayList(t, shadowFixture(`border-radius:20px;`, decl)) {
		if c.Kind == paint.CmdFill && c.Mask != nil {
			rounded = append(rounded, c)
		}
	}
	if len(rounded) != 0 {
		t.Errorf("a rounded blurred layer emitted %d masked cells: a mask would square off its corners", len(rounded))
	}
}

// TestOversizedBlurredFieldKeepsTheRingApproximation pins the other exception: a
// field wider than one mask budget falls back to the concentric rings, because
// pixel-exact masks are what bounds what a blurred shadow can ask the rasterizer
// for. The cost is measured on /tmp/sb6/budget.html - a 900x200 box at (20,200)
// declaring `0 0 20px rgba(0,0,255,0.5)`, so a field of 69,600 px against the
// 65,536 budget - where Chromium falls from 0.2458 to 0.1375 across the seven
// pixels left of the border box at mid-height, holding 0.0167 at (10,190), and
// Goosie answers 0.7958 then 0.7667 for both, a worst pixel error of 0.75. That
// gap is GOOSIE-SHADOWBUDGET-001.
//
// The control is the same declaration on this file's 120x60 box, whose field is
// 14,400 px: under budget, it must still go out as masked cells, so what this
// test pins is the budget dividing the two paths and not the blur.
//
// qc:BR-281
func TestOversizedBlurredFieldKeepsTheRingApproximation(t *testing.T) {
	const decl = `0 0 20px rgba(0,0,255,0.5)`
	if cells := shadowMaskedFills(t, decl); len(cells) == 0 {
		t.Fatal("the 120x60 box's blur-20 field stopped being masked: the control has to take the field path")
	}
	doc := `<!doctype html><html><head><style>html,body{margin:0;background:rgb(240,240,240)}` +
		`#b{position:absolute;left:20px;top:200px;width:900px;height:200px;background:rgb(240,240,240);` +
		`box-shadow:` + decl + `}</style></head><body><div id="b"></div></body></html>`
	var cells, rings int
	for _, c := range visDisplayList(t, doc) {
		if c.Kind != paint.CmdFill || int(c.Color.B()) <= int(c.Color.R()) {
			continue // the page's own fills carry no blue
		}
		if c.Mask != nil {
			cells++
		} else {
			rings++
		}
	}
	if cells != 0 {
		t.Errorf("the over-budget blur emitted %d masked cells, want 0 while the ring fallback stands", cells)
	}
	if rings == 0 {
		t.Error("the over-budget blur emitted no shadow fill at all: the fallback has to keep painting it")
	}
}
