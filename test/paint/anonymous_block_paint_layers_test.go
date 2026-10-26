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

// These are the paint-stage oracles for GOOSIE-SHADOWRUN-001's anonymous-box
// multiplier: layout wraps a run of inline content in anonymous block boxes
// (internal/layout/anonymous.go), each starts from a verbatim copy of the
// generating element's ComputedStyle (style.AnonymousBlockStyle: `s = *parent`),
// and paintBoxShadow has no owner test, so a block that generates two anonymous
// boxes emitted the element's shadow three times.
//
// Every number below was measured before it was encoded, with Chromium 131.0.6778.33
// through Playwright and cmd/goosie-headless on this host at 400x200 DPR 1, on the
// document the tests rasterize: a 300x76 block at (20,20) declaring
// `box-shadow:0 0 10px 6px rgba(200,0,0,0.5)` and holding `one two` and `three four`
// in inline spans, optionally split by a 20px-tall div. cmd/dump-layout puts the two
// anonymous boxes at (20,20,300,18) and (20,58,300,18) when the div is present, and
// generates none when it is not.
//
//	what the declaration changes (with-shadow minus without-shadow pixels)
//	  Chromium    plain 16144   anon 16144   symmetric difference 0
//	  Goosie pre  plain 13056   anon 23856   symmetric difference 10800
//	  Goosie post plain 13056   anon 13056   symmetric difference 0
//	pixels (270,48), in the 20px div's band past the text
//	  Chromium white, Goosie pre (202,13,13), Goosie post white
//	goosie's ring outside the box, (330,60)
//	  Chromium (250,231,231), Goosie (213,59,59) before and after: the blur model
//	  is Goosie's own and this task does not touch it
//	shadow-coloured fills on the display list, one declared layer
//	  no inline content 16 / one anonymous band 16 / two anonymous bands 48 pre,
//	  16 post; two declared layers 12 and 36 pre, 12 post
//
// The shape deliberately wraps the text in inline spans. Raw text directly inside
// the shadowed block is a SECOND multiplier of the same defect - the run's style
// comes from the element through style.inheritStyle, box-shadow included - and it
// is measured but not fixed here (TASK-053 §6, proposal 2 in §9). Spans make the
// anonymous-box contribution the only one on the list: a span does not declare the
// shadow and box-shadow does not inherit, so a span's runs emit none.

// apFillKey names a fill command by everything that decides its pixels, so two
// documents that differ only by a declaration can be subtracted.
func apFillKey(c paint.DisplayCmd) string {
	return fmt.Sprintf("(%d,%d,%d,%d)|%d|%v", c.Rect.X0, c.Rect.Y0, c.Rect.X1, c.Rect.Y1, c.Color, c.Opacity)
}

// apAddedFills returns the fill commands present in with but not in without, sorted
// so a shape comparison is order-independent.
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

// apAnonymousBlocks counts the block boxes layout generated that have no DOM node,
// which is what an anonymous block box is. It is the vacuity guard for this file:
// a shape that stopped generating the wrappers would make the shadow assertions
// trivially true.
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
	// Two anonymous boxes: the spans are inline content with a block sibling between
	// them.
	apTwoBands = `<div class="box"><span>one two</span><div style="height:20px"></div><span>three four</span></div>`
	// No anonymous box: inline content only, so nothing needs a block wrapper.
	apNoBands = `<div class="box"><span>one two</span><span>three four</span></div>`
)

// TestAnonymousBlockEmitsNoBoxShadowCommands pins the display list: the fills a
// box-shadow declaration adds are the element's own, and their number and geometry
// do not change when layout has to generate one or two anonymous blocks around the
// content. N fills per declaring element, zero per generated box.
func TestAnonymousBlockEmitsNoBoxShadowCommands(t *testing.T) {
	for _, tc := range []struct {
		name string
		css  string
		body string
		// anon is how many anonymous block boxes the body generates; wantFills is
		// the number of fills the declaration adds to the no-anon shape.
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
			wantFills: 12,
		},
		{
			name:      "two layers, no inline content",
			css:       apTwoLayer,
			body:      apNoInline,
			anon:      0,
			wantFills: 12,
		},
		{
			// Control: nothing declared, nothing emitted. A mechanism that suppresses
			// the copy by suppressing shadows altogether fails the cases above, not
			// this one, which is here so "zero fills" is never the expected answer for
			// a declared layer.
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
					"one ring per declared layer belongs to the element box and nothing to the generated ones "+
					"(two anonymous bands measured 48 fills for one layer before the collapse and 16 after)",
					len(added), added, tc.wantFills)
			}
		})
	}

	// The geometry, not only the count: the element sits at the same place in all
	// three shapes, so the fills it adds must be the same rects.
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

// apRaster renders one document through the same seam the tile path uses and
// returns the bitmap; the tile covers the block, its margin and the whole ring.
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

// apChanged returns the pixels where two renders of the same size differ, which is
// exactly the set a declaration paints.
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

// TestAnonymousBlockBoxShadowPaintIsShapeInvariant is the rasterised half. The
// declaration must paint the same pixels whether or not layout generated anonymous
// boxes, because the boxes are not the element and css-backgrounds-3 §7.1 makes
// box-shadow non-inherited: the generated box owns no shadow of its own.
//
// Measured for this document (see the file comment): Chromium's changed-pixel sets
// are identical in both shapes (16144 px); Goosie's differed by 10800 px before the
// collapse, and the extra pixels were the two 300x18 rings the wrappers painted,
// 3600 of them inside the block's own box between the text and its right edge where
// Chromium and Goosie-without-wrappers are white.
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
	// Anti-vacuity: the declaration must still paint a ring. Goosie measures 13056 px
	// and Chromium 16144 on this document; a floor of 12000 is below both and far
	// above zero, so a mechanism that simply drops box-shadow cannot pass.
	if len(anon) < 12000 {
		t.Errorf("the declaration changes only %d px, want >=12000: box-shadow must still paint the element's own ring (Goosie 13056, Chromium 16144)", len(anon))
	}

	// The wrappers' rings landed here: inside the block's own box, right of both
	// text lines, where nothing but the element paints. Chromium and the correct
	// render are white for every one of these pixels; before the collapse Goosie
	// changed 3600 of them.
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

	// One point in the 20px div's band, past both text lines, stated as a pixel so a
	// future refactor cannot satisfy the set comparison by painting nothing there for
	// a different reason.
	withAnon := apRaster(t, apOneLayer, apTwoBands)
	if got, want := withAnon.At(270, 48), frame.RGB(255, 255, 255); got != want {
		t.Errorf("pixel (270,48) in the sibling block's band = %v, want %v: Chromium is white there and so is Goosie without wrapper boxes; with them it read (202,13,13)",
			ibColorName(got), ibColorName(want))
	}
	// And the element's own ring must still be there: (330,60) is 10 px right of the
	// block's edge, inside the 6 px spread plus blur.
	if got := withAnon.At(330, 60); int(got.R())-int(got.B()) <= 20 {
		t.Errorf("pixel (330,60) just outside the block = %v, want a red-tinted ring pixel: the element's shadow is supposed to paint there (Goosie (213,59,59), Chromium (250,231,231))",
			ibColorName(got))
	}
}

// TestAnonymousBlockTextKeepsItsShadow is the oracle for the direction a blanket
// collapse gets wrong. text-shadow inherits (css-text-decor-3 §3), so glyphs inside
// an anonymous block are supposed to carry the element's shadow, and Chromium paints
// it: 655 changed px in the plain document and 648 in the split one, all inside the
// two bands. Goosie paints 610 in both, 482 of them blue-tinted.
//
// The document uses raw text on purpose: this engine gives a text run its style
// through style.inheritStyle, so raw text is where text-shadow actually reaches the
// glyphs. A blanket zero in AnonymousBlockStyle would not show up here - paint reads
// the run's own style - which is why the cascade assertion lives in test/style/
// (TestAnonymousBlockStyleKeepsInheritedTextShadow). What this test kills is the
// other mistake: gating the run's text-shadow along with the box's.
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
	// Control: without the declaration there is no blue at all.
	plainBitmap := apRaster(t, tsNone, rawAnon)
	for y := 0; y < 96; y++ {
		for x := 0; x < 340; x++ {
			if c := plainBitmap.At(x, y); int(c.B())-int(c.R()) > 20 {
				t.Fatalf("pixel (%d,%d) of the no-text-shadow control is blue-tinted %v: the blue count above would not be measuring the declaration", x, y, ibColorName(c))
			}
		}
	}
}
