package paint_test

import (
	"strconv"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

const (
	shadowBoxX = 200
	shadowBoxY = 150
)

func shadowFixture(boxCSS, decl string) string {
	return `<!doctype html><html><head><style>html,body{margin:0;background:rgb(240,240,240)}` +
		`#b{position:absolute;left:` + strconv.Itoa(shadowBoxX) + `px;top:` + strconv.Itoa(shadowBoxY) +
		`px;width:120px;height:60px;background:rgb(240,240,240);` + boxCSS +
		`box-shadow:` + decl +
		`}</style></head><body><div id="b"></div></body></html>`
}

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

type shadowEdge string

const (
	edgeRight shadowEdge = "right"
	edgeLeft  shadowEdge = "left"
	edgeBelow shadowEdge = "below"
	edgeAbove shadowEdge = "above"
	edgeDiag  shadowEdge = "diag"
)

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
			name: "blur 1",
			decl: `0 0 1px rgba(0,0,255,0.5)`,
			w:    120, h: 60,
			tol:   0.02,
			profs: []blurProfile{{edgeRight, []float64{0.0875, 0, 0, 0}}, {edgeDiag, []float64{0.0167, 0, 0}}},
		},
		{
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
			name: "blur 8 spread 4",
			decl: `0 0 8px 4px rgba(0,0,255,0.5)`,
			w:    120, h: 60,
			tol:   0.02,
			profs: []blurProfile{{edgeRight, []float64{0.4042, 0.3667, 0.3208, 0.2750, 0.2292, 0.1875, 0.1417, 0.1042, 0.0708, 0.0500, 0.0292, 0.0167, 0.0083, 0.0042, 0.0042, 0.0042, 0}}, {edgeDiag, []float64{0.3250, 0.2625, 0.2042, 0.1542, 0.1083, 0.0667, 0.0417, 0.0208, 0.0125, 0.0042, 0.0042, 0}}},
		},
		{
			name: "blur 8 spread -4",
			decl: `0 0 8px -4px rgba(0,0,255,0.5)`,
			w:    120, h: 60,
			tol:   0.02,
			profs: []blurProfile{{edgeRight, []float64{0.0708, 0.0500, 0.0292, 0.0167, 0.0083, 0.0042, 0.0042, 0.0042, 0, 0}}, {edgeDiag, []float64{0.0125, 0.0042, 0.0042, 0}}},
		},
		{
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
			name: "blur 8 alpha 1",
			decl: `0 0 8px rgba(0,0,255,1)`,
			w:    120, h: 60,
			tol:   0.02,
			profs: []blurProfile{{edgeRight, []float64{0.4542, 0.3667, 0.2792, 0.2000, 0.1417, 0.0917, 0.0583, 0.0333, 0.0167, 0.0083, 0.0042, 0.0042, 0}}, {edgeDiag, []float64{0.2083, 0.1333, 0.0792, 0.0417, 0.0208, 0.0083, 0.0042, 0}}},
		},
		{
			name: "blur 4 spread 2 alpha .25",
			decl: `0 0 4px 2px rgba(0,0,255,0.25)`,
			w:    120, h: 60,
			tol:   0.02,
			profs: []blurProfile{{edgeRight, []float64{0.1917, 0.1500, 0.1042, 0.0625, 0.0292, 0.0125, 0.0042, 0}}, {edgeDiag, []float64{0.1458, 0.0917, 0.0417, 0.0167, 0.0042, 0}}},
		},
		{
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

// qc:BR-281
func TestTwoLayerShadowStacksFirstDeclaredOnTop(t *testing.T) {
	cases := []struct {
		name string
		decl string
		want [][3]int
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
			continue
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
