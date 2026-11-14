package paint_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

const rgFixture = `<!doctype html><html><head><style>
html,body{margin:0;padding:0;background:#fff}
.box{position:absolute}
#fc{left:10px;top:10px;width:100px;height:100px;background:radial-gradient(circle,#f00,#00f)}
#cs{left:120px;top:10px;width:100px;height:100px;background:radial-gradient(circle closest-side at 25% 25%,#f00,#00f)}
#fs{left:230px;top:10px;width:100px;height:100px;background:radial-gradient(circle farthest-side at 25% 25%,#f00,#00f)}
#cc{left:340px;top:10px;width:100px;height:100px;background:radial-gradient(circle closest-corner at 25% 25%,#f00,#00f)}
#ell{left:10px;top:120px;width:200px;height:100px;background:radial-gradient(ellipse,#f00,#00f)}
#ellcs{left:220px;top:120px;width:200px;height:100px;background:radial-gradient(ellipse closest-side,#f00,#00f)}
#ellfs{left:10px;top:230px;width:200px;height:100px;background:radial-gradient(ellipse farthest-side at 25% 25%,#f00,#00f)}
#ellcs2{left:220px;top:230px;width:200px;height:100px;background:radial-gradient(ellipse closest-side at 25% 25%,#f00,#00f)}
#pxat{left:450px;top:10px;width:100px;height:100px;background:radial-gradient(circle at 30px 30px,#f00,#00f)}
</style></head><body>
<div id="fc" class="box"></div>
<div id="cs" class="box"></div>
<div id="fs" class="box"></div>
<div id="cc" class="box"></div>
<div id="ell" class="box"></div>
<div id="ellcs" class="box"></div>
<div id="ellfs" class="box"></div>
<div id="ellcs2" class="box"></div>
<div id="pxat" class="box"></div>
</body></html>`

func rgCommands(t *testing.T) ([]paint.DisplayCmd, *frame.Bitmap, *raster.Fonts) {
	t.Helper()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("raster.NewFonts: %v", err)
	}
	s, err := engine.NewSession(rgFixture, nil, 800, engine.WithMetrics(fonts), engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	dl, err := s.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	bm := frame.NewBitmap(800, 600)
	built := dl.Build(1)
	if err := raster.RasterizeTile(built.Publish(), frame.Rect4(0, 0, 800, 600), bm, fonts,
		raster.NewGlyphAtlas(1<<20, fonts), frame.RGB(255, 255, 255)); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	return built.All(), bm, fonts
}

// qc:BR-074
func TestRadialCommandsCarryTheResolvedSizing(t *testing.T) {
	cmds, _, _ := rgCommands(t)
	type want struct {
		x, y   int
		rx, ry float32
		what   string
	}
	wants := []want{
		{10, 10, 70.71, 70.71, "circle farthest-corner on 100x100: the reach to the farthest corner, not half the side"},
		{120, 10, 25, 25, "circle closest-side at 25% 25%: the nearest side is 25px away"},
		{230, 10, 75, 75, "circle farthest-side at 25% 25%: the farthest side is 75px away"},
		{340, 10, 35.36, 35.36, "circle closest-corner at 25% 25%: the diagonal to the nearest corner"},
		{10, 120, 141.42, 70.71, "ellipse farthest-corner on 200x100: closest-side aspect scaled through the farthest corner"},
		{220, 120, 100, 50, "ellipse closest-side on 200x100: per-axis, not one scalar"},
		{10, 230, 150, 75, "ellipse farthest-side at 25% 25%: the farthest side differs per axis (right 150, below 75)"},
		{220, 230, 50, 25, "ellipse closest-side at 25% 25%: the nearest side differs per axis (right 50, below 25)"},
		{450, 10, 98.99, 98.99, "circle at 30px 30px on 100x100: the farthest corner from an off-centre length position"},
	}
	seen := 0
	for _, w := range wants {
		found := false
		for _, c := range cmds {
			if c.Kind == paint.CmdRadialGradient && c.Rect.X0 == int32(w.x) && c.Rect.Y0 == int32(w.y) {
				found = true
				seen++
				if math.Abs(float64(c.RadialGradient.Rx-w.rx)) > 0.01 || math.Abs(float64(c.RadialGradient.Ry-w.ry)) > 0.01 {
					t.Errorf("%s: the command at (%d,%d) carries radii (%.4f, %.4f), want (%.2f, %.2f)",
						w.what, w.x, w.y, c.RadialGradient.Rx, c.RadialGradient.Ry, w.rx, w.ry)
				}
			}
		}
		if !found {
			t.Errorf("%s: no radial command at (%d,%d)", w.what, w.x, w.y)
		}
	}
	if seen != len(wants) {
		t.Errorf("matched %d radial commands, want exactly %d: an extra emission paints the ramp twice",
			seen, len(wants))
	}
}

// qc:BR-074
func TestRadialGradientsPaintTheMeasuredSizing(t *testing.T) {
	_, bm, _ := rgCommands(t)
	cisCheck(t, bm, []cisWant{
		{x: 60, y: 60, r: 252, g: 0, b: 3, tol: 2, what: "fc centre"},
		{x: 60, y: 10, r: 76, g: 0, b: 179, tol: 2, what: "fc half side: 49.5px of a 70.7px radius still blends"},
		{x: 35, y: 35, r: 130, g: 0, b: 125, tol: 2, what: "fc midway to corner"},
		{x: 14, y: 14, r: 23, g: 0, b: 232, tol: 2, what: "fc near corner"},
		{x: 145, y: 35, r: 248, g: 0, b: 7, tol: 2, what: "cs centre"},
		{x: 170, y: 35, r: 0, g: 0, b: 255, tol: 0, what: "cs 25px right is already the end colour (radius 25)"},
		{x: 162, y: 35, r: 76, g: 0, b: 179, tol: 2, what: "cs 17px right"},
		{x: 125, y: 15, r: 0, g: 0, b: 255, tol: 0, what: "cs corner past the radius"},
		{x: 255, y: 35, r: 253, g: 0, b: 2, tol: 2, what: "fs centre"},
		{x: 280, y: 35, r: 170, g: 0, b: 85, tol: 2, what: "fs 25px right is a third of the ramp (radius 75)"},
		{x: 235, y: 15, r: 161, g: 0, b: 94, tol: 2, what: "fs corner"},
		{x: 255, y: 105, r: 15, g: 0, b: 240, tol: 2, what: "fs 70px below"},
		{x: 365, y: 35, r: 250, g: 0, b: 5, tol: 2, what: "cc centre"},
		{x: 382, y: 35, r: 129, g: 0, b: 126, tol: 2, what: "cc 17px right (radius 35.36)"},
		{x: 345, y: 15, r: 56, g: 0, b: 199, tol: 2, what: "cc corner"},
		{x: 110, y: 170, r: 253, g: 0, b: 2, tol: 2, what: "ell centre"},
		{x: 160, y: 170, r: 164, g: 0, b: 91, tol: 2, what: "ell 50px right: t=.36 of the 141.4px horizontal axis"},
		{x: 110, y: 135, r: 131, g: 0, b: 124, tol: 2, what: "ell 35px above: t=.49 of the 70.7px vertical axis"},
		{x: 181, y: 205, r: 73, g: 0, b: 182, tol: 2, what: "ell diagonal t=.71"},
		{x: 320, y: 170, r: 252, g: 0, b: 3, tol: 2, what: "ellcs centre"},
		{x: 370, y: 170, r: 126, g: 0, b: 129, tol: 2, what: "ellcs half width: t=.5"},
		{x: 320, y: 145, r: 130, g: 0, b: 125, tol: 2, what: "ellcs half height: the SAME t=.5, the per-axis signature"},
		{x: 395, y: 170, r: 62, g: 0, b: 193, tol: 2, what: "ellcs 75px right: t=.75"},
		{x: 320, y: 215, r: 23, g: 0, b: 232, tol: 2, what: "ellcs 45px below: t=.91"},
		{x: 60, y: 255, r: 253, g: 0, b: 2, tol: 2, what: "ellfs centre"},
		{x: 135, y: 255, r: 127, g: 0, b: 128, tol: 2, what: "ellfs 75px right: t=.5 of the 150px horizontal axis"},
		{x: 60, y: 292, r: 127, g: 0, b: 127, tol: 2, what: "ellfs 37px below: t=.5 of the 75px vertical axis"},
		{x: 270, y: 255, r: 249, g: 0, b: 6, tol: 2, what: "ellcs2 centre"},
		{x: 320, y: 255, r: 0, g: 0, b: 255, tol: 0, what: "ellcs2 50px right is already the end colour (radius 50)"},
		{x: 270, y: 280, r: 0, g: 0, b: 255, tol: 0, what: "ellcs2 25px below is already the end colour (radius 25)"},
		{x: 480, y: 40, r: 253, g: 0, b: 2, tol: 2, what: "pxat centre (30,30): red where the default centre would blend"},
		{x: 545, y: 105, r: 16, g: 0, b: 239, tol: 2, what: "pxat near the far corner"},
	})
}
