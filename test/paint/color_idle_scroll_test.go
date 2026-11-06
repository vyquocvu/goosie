package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

// BR-074's colour half: what a page of solid fills, one opacity group and
// simple gradients puts on the screen, and the guarantee that a page nobody
// touched keeps those pixels.
//
// The reference numbers come from scripts/verify-br074-color-idle-scroll.js
// (Chromium 1.63, this host, 2026-11-04), which reads the composited device
// pixels back out of its own PNG rather than recomputing a style:
//
//	PIXELS solid #e7004c             rgb(231,0,76)
//	PIXELS #f00 at opacity .5 over #00f  rgb(128,0,127)
//	PIXELS linear-gradient(90deg,#000,#fff) rgb(3,3,3) rgb(64,64,64) rgb(128,128,128) rgb(192,192,192) rgb(253,253,253)
//	PIXELS linear-gradient(#111 0 50%,#eee 50% 100%)  rgb(17,17,17) above the mid line, rgb(238,238,238) below
//	PIXELS radial-gradient(circle,#f00,#00f) centre rgb(252,0,2)
//	PIXELS linear-gradient(180deg,#0a0 0,#a00 100%) at 10/100/250/390 px into a 1600px box
//	       rgb(1,168,0) rgb(10,159,0) rgb(26,143,0) rgb(41,128,0)
//
// Chromium DITHERS gradient ramps - the ends of a black-to-white line read 3
// and 253 two pixels inside, not 0 and 255 - and rounds a composited group
// asymmetrically (128,0,127 for what arithmetic calls 127.5), so the ramp and
// group samples carry a one-unit band while the flat fills and the hard-stop
// bands are pinned exactly. Goosie's own rasterizer is deterministic, so an
// idle repaint changing a pixel is a defect, never noise: that is what the
// last two oracles here hold.

// cisFixture is the anchor's page: the same boxes at the same document
// coordinates, the same colours, the same gradient syntaxes.
const cisFixture = `<!doctype html><html><head><style>
html,body{margin:0;padding:0;background:#fff}
.box{position:absolute}
#solid{left:10px;top:10px;width:100px;height:100px;background:#e7004c}
#backdrop{left:120px;top:10px;width:100px;height:100px;background:#00f}
#over{position:absolute;left:20px;top:20px;width:60px;height:60px;background:#f00;opacity:.5}
#lin{left:240px;top:10px;width:200px;height:40px;background:linear-gradient(90deg,#000,#fff)}
#hard{left:10px;top:130px;width:200px;height:40px;background:linear-gradient(#111 0 50%,#eee 50% 100%)}
#rad{left:230px;top:70px;width:100px;height:100px;background:radial-gradient(circle,#f00,#00f)}
#tall{left:0;top:200px;width:800px;height:1600px;background:linear-gradient(180deg,#0a0 0,#a00 100%)}
#pxa{left:450px;top:20px;width:120px;height:60px;background:linear-gradient(180deg,#0a0 0px,#a00 60px)}
#pxm{left:590px;top:20px;width:120px;height:60px;background:linear-gradient(180deg,#f00,#0f0 30px,#00f 60px)}
#pxd{left:450px;top:90px;width:120px;height:60px;background:linear-gradient(180deg,#f00 45px,#0f0,#00f)}
#pxc{left:590px;top:90px;width:120px;height:60px;background:linear-gradient(180deg,#f00 25%,#0f0 30px,#00b)}
</style></head><body>
<div id="solid" class="box"></div>
<div id="backdrop" class="box"><div id="over"></div></div>
<div id="lin" class="box"></div>
<div id="hard" class="box"></div>
<div id="rad" class="box"></div>
<div id="tall" class="box"></div>
<div id="pxa" class="box"></div>
<div id="pxm" class="box"></div>
<div id="pxd" class="box"></div>
<div id="pxc" class="box"></div>
</body></html>`

func cisRaster(t *testing.T, s *engine.Session, fonts *raster.Fonts) *frame.Bitmap {
	t.Helper()
	dl, err := s.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	bm := frame.NewBitmap(800, 600)
	if err := raster.RasterizeTile(dl.Build(1).Publish(), frame.Rect4(0, 0, 800, 600), bm, fonts,
		raster.NewGlyphAtlas(1<<20, fonts), frame.RGB(255, 255, 255)); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	return bm
}

// cisPage loads the fixture with the shipped metrics, so the numbers below are
// the ones the shipped rasterizer produces.
func cisPage(t *testing.T) (*engine.Session, *raster.Fonts) {
	t.Helper()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("raster.NewFonts: %v", err)
	}
	s, err := engine.NewSession(cisFixture, nil, 800, engine.WithMetrics(fonts), engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, fonts
}

// cisWant is one measured reference pixel and how far the engine may sit from
// it: 0 for a flat fill the reference itself reads exactly, 1 where Chromium
// dithers a ramp or rounds a composited group.
type cisWant struct {
	x, y         int
	r, g, b, tol int
	what         string
}

func cisCheck(t *testing.T, bm *frame.Bitmap, wants []cisWant) {
	t.Helper()
	var bad []string
	for _, w := range wants {
		c := bm.At(w.x, w.y)
		dr, dg, db := int(c.R())-w.r, int(c.G())-w.g, int(c.B())-w.b
		if abs(dr) > w.tol || abs(dg) > w.tol || abs(db) > w.tol {
			bad = append(bad, w.what+": got rgb("+itoaC(int(c.R()))+","+itoaC(int(c.G()))+","+itoaC(int(c.B()))+
				") at ("+itoaC(w.x)+","+itoaC(w.y)+"), want rgb("+itoaC(w.r)+","+itoaC(w.g)+","+itoaC(w.b)+
				") +-"+itoaC(w.tol))
		}
	}
	if len(bad) != 0 {
		for _, b := range bad {
			t.Error(b)
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func itoaC(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// Flat fills and a composited opacity group, at the points the anchor reads.
//
// qc:BR-074
func TestSolidAndOpacityGroupsPaintTheMeasuredColours(t *testing.T) {
	s, fonts := cisPage(t)
	cisCheck(t, cisRaster(t, s, fonts), []cisWant{
		{x: 60, y: 60, r: 231, g: 0, b: 76, tol: 0, what: "#e7004c flat fill"},
		{x: 130, y: 20, r: 0, g: 0, b: 255, tol: 0, what: "#00f backdrop under the faded box"},
		{x: 170, y: 60, r: 128, g: 0, b: 127, tol: 1, what: "#f00 at opacity .5 over #00f"},
	})
}

// A ramp and a hard stop, at the fractions the anchor samples: the ramp must be
// monotone across the box and the hard stop must keep its edge, both of which
// the exact-value cases below carry.
//
// qc:BR-074
func TestGradientsPaintTheMeasuredRampAndBands(t *testing.T) {
	s, fonts := cisPage(t)
	cisCheck(t, cisRaster(t, s, fonts), []cisWant{
		{x: 242, y: 30, r: 3, g: 3, b: 3, tol: 0, what: "linear 90deg 2px inside the #000 end"},
		{x: 290, y: 30, r: 64, g: 64, b: 64, tol: 0, what: "linear 90deg at 25%"},
		{x: 340, y: 30, r: 128, g: 128, b: 128, tol: 0, what: "linear 90deg at 50%"},
		{x: 390, y: 30, r: 192, g: 192, b: 192, tol: 1, what: "linear 90deg at 75%"},
		{x: 438, y: 30, r: 253, g: 253, b: 253, tol: 0, what: "linear 90deg 2px inside the #fff end"},
		// The radius itself, now that GOOSIE-RADIALGRAD-001 fixed it: the box is
		// 100x100 with centre (280,120), so Chromium's circle reaches the corner
		// at 70.7px. The anchor measured rgb(76,0,178) 50px above the centre,
		// rgb(130,0,125) midway to the corner and rgb(33,0,221) near the corner;
		// a 50px min-side radius would have answered the end colour at the first
		// of them.
		{x: 280, y: 120, r: 252, g: 0, b: 2, tol: 1, what: "radial centre of #f00 to #00f"},
		{x: 280, y: 70, r: 76, g: 0, b: 178, tol: 2, what: "radial 50px above the centre: half the side still blends"},
		{x: 255, y: 95, r: 130, g: 0, b: 124, tol: 2, what: "radial midway to the corner"},
		{x: 236, y: 76, r: 33, g: 0, b: 221, tol: 2, what: "radial near the corner: the ramp reaches 70.7px"},
	})
}

// The syntax that painted nothing until this slice: stop positions written
// with CSS's unitless zero - `#111 0 50%` and `#0a0 0` - which the reference
// keeps and paints as flat bands, and stop positions written as <length>s -
// `#0a0 0px`, `#0f0 30px` - which it also keeps and paints at the authored
// offsets. Measured scripts/verify-br074-color-idle-scroll.js (Chromium 1.63,
// this host, 2026-11-06): px-ends answers rgb(44,126,0) rgb(86,83,0)
// rgb(129,41,0) at 25/50/75% of the 60px ramp and px-mid answers
// rgb(123,132,0) rgb(0,250,4) rgb(0,123,132). The tolerance is wider than the
// tall box's because Chromium dithers with an amplitude scaled to the local
// slope - this ramp runs 255 units over 30px where the 1600px ramp ran 170
// over 1600 - and Goosie's undithered arithmetic sits inside the band.
//
// qc:BR-074
func TestGradientStopsWithUnitlessZeroPositionsPaint(t *testing.T) {
	s, fonts := cisPage(t)
	cisCheck(t, cisRaster(t, s, fonts), []cisWant{
		{x: 100, y: 145, r: 17, g: 17, b: 17, tol: 0, what: "#111 0 50% band above the mid line"},
		{x: 100, y: 155, r: 238, g: 238, b: 238, tol: 0, what: "#eee 50% 100% band below the mid line"},
		{x: 400, y: 210, r: 1, g: 168, b: 0, tol: 0, what: "#0a0 0 at 10px into the ramp"},
		{x: 400, y: 300, r: 10, g: 159, b: 0, tol: 0, what: "#0a0 0 at 100px into the ramp"},
		{x: 400, y: 450, r: 26, g: 143, b: 0, tol: 0, what: "#0a0 0 at 250px into the ramp"},
		{x: 400, y: 590, r: 41, g: 128, b: 0, tol: 0, what: "#0a0 0 at 390px into the ramp"},
		{x: 480, y: 35, r: 44, g: 126, b: 0, tol: 3, what: "#0a0 0px at 25% of the 60px ramp"},
		{x: 480, y: 50, r: 86, g: 83, b: 0, tol: 3, what: "#0a0 0px at 50% of the ramp"},
		{x: 480, y: 65, r: 129, g: 41, b: 0, tol: 3, what: "#0a0 60px at 75% of the ramp"},
		{x: 620, y: 35, r: 123, g: 132, b: 0, tol: 3, what: "#f00 fading out at 25%, the 30px stop pending"},
		{x: 620, y: 50, r: 0, g: 250, b: 4, tol: 3, what: "on the #0f0 30px stop itself"},
		{x: 620, y: 65, r: 0, g: 123, b: 132, tol: 3, what: "fading toward #00f at 75%"},
		// A px anchor with unpositioned stops around it: the tail anchors at the
		// END of the laid-out line - Chromium stays solid #f00 through 50% and
		// puts the middle stop at 52.5px, halfway from the 45px anchor, which
		// even-distribution over the stops would answer (0,255,0) instead.
		{x: 480, y: 105, r: 255, g: 0, b: 0, tol: 0, what: "solid above the 45px px anchor"},
		{x: 480, y: 120, r: 255, g: 0, b: 0, tol: 0, what: "still solid at 50%, the tail waits past the anchor"},
		{x: 480, y: 135, r: 238, g: 17, b: 0, tol: 1, what: "just past the 45px anchor, toward the 52.5px middle"},
		// A percentage mixed with a px length: both convert onto the same
		// measured line, the unpositioned end anchors there too.
		{x: 620, y: 105, r: 247, g: 9, b: 0, tol: 2, what: "just past the 25% stop, toward the 30px stop"},
		{x: 620, y: 120, r: 0, g: 250, b: 3, tol: 2, what: "on the 30px stop"},
		{x: 620, y: 135, r: 0, g: 123, b: 97, tol: 2, what: "midway to the anchored #00b end"},
	})
}

// "No flicker when idle" at the level the frame path consumes: a document
// nobody touched hands the frame path nothing to redraw, and a page rasterized
// twice paints the same pixels both times. Both halves are the engine's own
// claim, not a reference browser's: Goosie has no dither and no animation here,
// so a pixel that moves between two idle rasters is a bug.
//
// qc:BR-074
func TestAnIdlePageRepaintsNothingAndTellsTheFramePathSo(t *testing.T) {
	s, fonts := cisPage(t)
	first := cisRaster(t, s, fonts)
	second := cisRaster(t, s, fonts)
	third := cisRaster(t, s, fonts)
	if bad := cisChanged(first, second); len(bad) != 0 {
		t.Errorf("an untouched page changed %d pixels between two rasters, e.g. %v", len(bad), bad[:min(len(bad), 4)])
	}
	if bad := cisChanged(second, third); len(bad) != 0 {
		t.Errorf("an untouched page changed %d pixels between three rasters, e.g. %v", len(bad), bad[:min(len(bad), 4)])
	}

	before, err := s.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	after, err := s.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	if dmg := paint.ContentDamage(before.Build(1).Publish(), after.Build(1).Publish()); !dmg.Empty() {
		t.Errorf("two paints of an untouched document damage %v, want nothing: an idle vsync must cost no pixel work", dmg)
	}
}

// cisChanged is the device pixels two rasters disagree about.
func cisChanged(a, b *frame.Bitmap) [][2]int {
	var out [][2]int
	for y := 0; y < int(b.H); y++ {
		for x := 0; x < int(b.W); x++ {
			if a.At(x, y) != b.At(x, y) {
				out = append(out, [2]int{x, y})
			}
		}
	}
	return out
}
