package paint_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/style"
)

func apFillKey(c paint.DisplayCmd) string {
	return fmt.Sprintf("(%d,%d,%d,%d)|%d|%v", c.Rect.X0, c.Rect.Y0, c.Rect.X1, c.Rect.Y1, c.Color, c.Opacity)
}

func apAddedFills(with, without []paint.DisplayCmd) []string {
	have := make(map[string]int, len(without))
	for _, c := range without {
		if c.Kind == paint.CmdFill {
			have[apFillKey(c)]++
		}
	}
	out := make([]string, 0, len(with))
	for _, c := range with {
		if c.Kind != paint.CmdFill {
			continue
		}
		k := apFillKey(c)
		if have[k] > 0 {
			have[k]--
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func apAnonymousBlocks(t *testing.T, css, body string) int {
	t.Helper()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("raster.NewFonts: %v", err)
	}
	html := `<!doctype html><html><head><style>html,body{margin:0;font:16px serif}` + css +
		`</style></head><body>` + body + `</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithMetrics(fonts), engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()
	n := 0
	for i := range sess.Arena.Objects {
		o := &sess.Arena.Objects[i]
		if o.Node == nil && o.Style != nil && o.Style.Display == style.DisplayBlock && o.W > 0 && o.H > 0 {
			n++
		}
	}
	return n
}

const (
	apBoxShape = `.box{width:300px;height:76px;margin:20px}`
	apOneLayer = `.box{width:300px;height:76px;margin:20px;box-shadow:0 0 10px 6px rgba(200,0,0,0.5)}`
	apTwoLayer = `.box{width:300px;height:76px;margin:20px;box-shadow:1px 1px 2px rgb(200,0,0),0 0 4px rgb(0,0,200)}`

	apNoInline = `<div class="box"></div>`
	apTwoBands = `<div class="box"><span>one two</span><div style="height:20px"></div><span>three four</span></div>`
	apNoBands  = `<div class="box"><span>one two</span><span>three four</span></div>`
)

// qc:BR-231
func TestAnonymousBlockEmitsNoBoxShadowCommands(t *testing.T) {
	for _, tc := range []struct {
		name      string
		css       string
		body      string
		anon      int
		wantFills int
	}{
		{
			name:      "one layer, no inline content",
			css:       apOneLayer,
			body:      apNoInline,
			anon:      0,
			wantFills: 16,
		},
		{
			name:      "one layer, two anonymous bands",
			css:       apOneLayer,
			body:      apTwoBands,
			anon:      2,
			wantFills: 16,
		},
		{
			name:      "one layer, inline content with no block sibling",
			css:       apOneLayer,
			body:      apNoBands,
			anon:      0,
			wantFills: 16,
		},
		{
			name:      "two layers, two anonymous bands",
			css:       apTwoLayer,
			body:      apTwoBands,
			anon:      2,
			wantFills: 28,
		},
		{
			name:      "two layers, no inline content",
			css:       apTwoLayer,
			body:      apNoInline,
			anon:      0,
			wantFills: 28,
		},
		{
			name:      "no declaration",
			css:       apBoxShape,
			body:      apTwoBands,
			anon:      2,
			wantFills: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := apAnonymousBlocks(t, tc.css, tc.body); got != tc.anon {
				t.Fatalf("layout generated %d anonymous block boxes, want %d: without them this case proves nothing", got, tc.anon)
			}
			with := ibDisplayList(t, tc.css, tc.body, 800)
			without := ibDisplayList(t, apBoxShape, tc.body, 800)
			added := apAddedFills(with, without)
			if len(added) != tc.wantFills {
				t.Errorf("the declaration added %d fill(s) %v, want %d: "+
					"the fills a declared layer adds belong to the element box and nothing to the generated ones "+
					"(two anonymous bands measured 48 fills for one layer before the collapse and 16 after; "+
					"a blurred layer is the masked cell field BR-281 measures, 28 cells for these two layers)",
					len(added), added, tc.wantFills)
			}
		})
	}

	base := apAddedFills(ibDisplayList(t, apOneLayer, apNoInline, 800), ibDisplayList(t, apBoxShape, apNoInline, 800))
	for _, shape := range []struct {
		name string
		body string
		anon int
	}{
		{"two anonymous bands", apTwoBands, 2},
		{"inline content with no block sibling", apNoBands, 0},
	} {
		if got := apAnonymousBlocks(t, apOneLayer, shape.body); got != shape.anon {
			t.Fatalf("%s: layout generated %d anonymous block boxes, want %d", shape.name, got, shape.anon)
		}
		added := apAddedFills(ibDisplayList(t, apOneLayer, shape.body, 800), ibDisplayList(t, apBoxShape, shape.body, 800))
		if strings.Join(added, "\n") != strings.Join(base, "\n") {
			t.Errorf("%s: the declaration emits %d fill(s), the element-only shape emits %d; "+
				"the first difference is not the element's own ring:\n got %v\nwant %v",
				shape.name, len(added), len(base), added, base)
		}
	}
}

func apRaster(t *testing.T, css, body string) *frame.Bitmap {
	t.Helper()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("raster.NewFonts: %v", err)
	}
	html := `<!doctype html><html><head><style>html,body{margin:0;padding:0;font:16px serif}` + css +
		`</style></head><body>` + body + `</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithMetrics(fonts), engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()
	dl, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	bitmap := frame.NewBitmap(340, 120)
	if err := raster.RasterizeTile(dl.Build(1).Publish(), frame.Rect4(0, 0, 340, 120), bitmap,
		fonts, raster.NewGlyphAtlas(1<<20, fonts), frame.RGB(255, 255, 255)); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	return bitmap
}

func apChanged(a, b *frame.Bitmap) map[[2]int]struct{} {
	out := map[[2]int]struct{}{}
	w, h := a.W, a.H
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if a.At(x, y) != b.At(x, y) {
				out[[2]int{x, y}] = struct{}{}
			}
		}
	}
	return out
}

// qc:BR-232
func TestAnonymousBlockBoxShadowPaintIsShapeInvariant(t *testing.T) {
	plain := apChanged(apRaster(t, apOneLayer, apNoBands), apRaster(t, apBoxShape, apNoBands))
	anon := apChanged(apRaster(t, apOneLayer, apTwoBands), apRaster(t, apBoxShape, apTwoBands))

	extra := make([][2]int, 0)
	for p := range anon {
		if _, ok := plain[p]; !ok {
			extra = append(extra, p)
		}
	}
	for p := range plain {
		if _, ok := anon[p]; !ok {
			extra = append(extra, p)
		}
	}
	if len(extra) != 0 {
		sort.Slice(extra, func(i, j int) bool {
			if extra[i][1] != extra[j][1] {
				return extra[i][1] < extra[j][1]
			}
			return extra[i][0] < extra[j][0]
		})
		t.Errorf("the declaration paints %d px without anonymous boxes and %d with them; %d px differ, first at %v: the wrappers painted the element's ring around themselves (Goosie pre-fix differed by 10800 px, Chromium by 0)",
			len(plain), len(anon), len(extra), extra[:min(8, len(extra))])
	}
	if len(anon) < 12000 {
		t.Errorf("the declaration changes only %d px, want >=12000: box-shadow must still paint the element's own ring (Goosie 15332, Chromium 16636)", len(anon))
	}

	var intruding [][2]int
	for p := range anon {
		if p[0] >= 220 && p[0] < 320 && p[1] >= 20 && p[1] < 96 {
			intruding = append(intruding, p)
		}
	}
	if len(intruding) != 0 {
		t.Errorf("%d px of the block's interior right of its text are changed by the shadow, want 0: "+
			"a 300-wide ring from an anonymous band covers exactly this area and Chromium has none (measured 3600 px before the collapse)",
			len(intruding))
	}

	withAnon := apRaster(t, apOneLayer, apTwoBands)
	if got, want := withAnon.At(270, 48), frame.RGB(255, 255, 255); got != want {
		t.Errorf("pixel (270,48) in the sibling block's band = %v, want %v: Chromium is white there and so is Goosie without wrapper boxes; with them it read (202,13,13)",
			ibColorName(got), ibColorName(want))
	}
	if got := withAnon.At(330, 60); got != frame.RGB(250, 232, 232) {
		t.Errorf("pixel (330,60) just outside the block = %v, want the measured (250,232,232), "+
			"which is within 3 levels of Chromium's (249,230,230); the ring model read (213,59,59) here",
			ibColorName(got))
	}
}

// qc:BR-233
func TestAnonymousBlockTextKeepsItsShadow(t *testing.T) {
	const ts = `.box{width:300px;height:76px;margin:20px;text-shadow:2px 2px 0 rgb(0,0,255)}`
	const tsNone = `.box{width:300px;height:76px;margin:20px}`
	const rawAnon = `<div class="box">one two<div style="height:20px"></div>three four</div>`

	withShadow := apRaster(t, ts, rawAnon)
	without := apRaster(t, tsNone, rawAnon)
	changed := apChanged(withShadow, without)

	var inBands, blue int
	for p := range changed {
		if p[0] >= 20 && p[0] < 320 && ((p[1] >= 20 && p[1] < 38) || (p[1] >= 58 && p[1] < 76)) {
			inBands++
		}
	}
	for y := 0; y < 96; y++ {
		for x := 0; x < 340; x++ {
			if c := withShadow.At(x, y); int(c.B())-int(c.R()) > 20 {
				blue++
			}
		}
	}
	if inBands < 500 {
		t.Errorf("text-shadow changes %d px inside the two anonymous bands, want >=500: the text inside them inherits the shadow and must paint it (Goosie measures 610 in both shapes, Chromium 655 plain / 648 split)", inBands)
	}
	if blue < 400 {
		t.Errorf("%d px are blue-tinted, want >=400: the shadow colour must reach the glyphs inside the anonymous boxes (Goosie 482, Chromium 549 plain / 542 split)", blue)
	}
	plainBitmap := apRaster(t, tsNone, rawAnon)
	for y := 0; y < 96; y++ {
		for x := 0; x < 340; x++ {
			if c := plainBitmap.At(x, y); int(c.B())-int(c.R()) > 20 {
				t.Fatalf("pixel (%d,%d) of the no-text-shadow control is blue-tinted %v: the blue count above would not be measuring the declaration", x, y, ibColorName(c))
			}
		}
	}
}
