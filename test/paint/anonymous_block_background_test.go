package paint_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

// These are the oracles for GOOSIE-ANONBG-001: an anonymous block box re-emitted its
// generating element's gradient and background-image layers, so a translucent ramp was
// composited a second time behind every run of raw text layout had to wrap.
//
// The shape is a 300x76 block declaring
// `background:linear-gradient(rgba(200,0,0,.5),rgba(200,0,0,.5))` and holding raw text,
// a 20px-tall div sibling, then more raw text. cmd/dump-layout splices exactly two
// anonymous boxes there, (0,0,300,18) and (0,38,300,18), and both carry a verbatim copy
// of the block's computed style (style.AnonymousBlockStyle: `s = *parent`). Paint emits
// gradient layers for any box whose display is not inline, so an uncleared ramp on those
// two wrappers painted over the block's own.
//
// Measured on this host with cmd/goosie-headless at 400x120 DPR 1 over the block's
// 300x76 area (22,800 px). On the reproduction document, whose lines are
// `one two three four` and `five six seven`: Chromium 13,160 px of (227,127,127) plus
// 8,443 px of (228,127,127) - one composite, no doubled pixel; Goosie at main 12,000 px
// of (227,127,127) plus 9,693 px of (213,63,63); Goosie after the style-layer collapse
// 21,693 px once and 0 doubled. On the document the tests below rasterize, whose lines
// are `one two` and `three four`: Chromium 13,465 plus 8,640 of the two roundings and no
// doubled pixel; Goosie at main 12,000 once plus 10,172 doubled; after the collapse
// 22,172 once and 0 doubled.
// rgba(200,0,0,.5) over white is (227,127,127) once - 0.5*200+0.5*255 - and (213,63,63)
// twice, so those are arithmetic, not tuned constants. The 12,000 figure is exactly the
// block's area minus the two anonymous bands (22,800 - 2*300*18), which is what pins the
// defect to the wrappers rather than to the text runs: the run path was already gated by
// TASK-048.
//
// What must not collapse is the element's own background and the fragmentation of an
// inline one, so every case pairs a collapse with a declaring box that still paints.

// abGradientRects names every linear or radial gradient command in list order, so a
// re-emission shows up as an extra entry no matter which wrapper produced it.
func abGradientRects(cmds []paint.DisplayCmd) []string {
	var out []string
	for _, c := range cmds {
		if c.Kind == paint.CmdGradient || c.Kind == paint.CmdRadialGradient {
			out = append(out, fmt.Sprintf("(%d,%d,%d,%d)", c.Rect.X0, c.Rect.Y0, c.Rect.X1, c.Rect.Y1))
		}
	}
	return out
}

func abKindCount(cmds []paint.DisplayCmd, k paint.CmdKind) int {
	var n int
	for _, c := range cmds {
		if c.Kind == k {
			n++
		}
	}
	return n
}

// TestAnonymousBlockGradientIsEmittedOncePerElement pins the display list: exactly one
// gradient per declaring element box, and nothing contributed by the anonymous boxes
// layout splices around raw text.
func TestAnonymousBlockGradientIsEmittedOncePerElement(t *testing.T) {
	linear := `background-image:linear-gradient(rgb(10,120,20),rgb(10,120,20))`
	radial := `background-image:radial-gradient(rgb(10,120,20),rgb(10,120,20))`
	magenta := `background-image:linear-gradient(rgb(200,0,200),rgb(200,0,200))`
	for _, tc := range []struct {
		name string
		body string
		w    int
		want []string
		// wantKinds, when set, is the whole histogram for shapes whose emission is
		// not expressible as a rect list alone.
		wantKinds map[paint.CmdKind]int
	}{
		{
			// The defect: two anonymous boxes ride inside this block.
			name: "raw text beside a block sibling, linear gradient",
			body: `<div style="width:300px;height:76px;` + linear + `">one two<div style="height:20px"></div>three four</div>`,
			want: []string{"(0,0,300,76)"},
		},
		{
			// Control: the same declaration with no block sibling generates no
			// anonymous box at all, so its one gradient is the element's own. An
			// over-collapse that drops the declaring element's background dies here.
			name: "block with no anonymous children",
			body: `<div style="width:300px;height:60px;` + linear + `">one two three four</div>`,
			want: []string{"(0,0,300,60)"},
		},
		{
			name: "raw text beside a block sibling, radial gradient",
			body: `<div style="width:300px;height:76px;` + radial + `">one two<div style="height:20px"></div>three four</div>`,
			want: []string{"(0,0,300,76)"},
		},
		{
			// Two declaring elements nested: each owns exactly one ramp, and the
			// anonymous boxes inside the outer one own none.
			name: "two nested declaring elements",
			body: `<div style="width:300px;height:96px;` + linear + `">one two<div style="height:20px;` + magenta + `">inner</div>three four</div>`,
			want: []string{"(0,0,300,96)", "(0,18,300,38)"},
		},
		{
			// The legitimate fragmentation TASK-048 protects: an inline element's
			// own box paints no background, its text-run fragment does. The ramp the
			// anonymous block must not add is separate from the one the fragment
			// must keep.
			name: "inline fragment inside a block with anonymous children",
			body: `<div style="width:300px;height:76px;` + linear + `">one <span style="` + magenta + `">two</span><div style="height:20px"></div>three four</div>`,
			want: []string{"(0,0,300,76)", "(27,0,52,18)"},
		},
		{
			// Shorthand declaring a colour under the ramp: the colour layer was
			// already collapsed for anonymous boxes, the ramp was not, so both must
			// now appear exactly once and for the element only.
			name: "colour and gradient in one shorthand",
			body: `<div style="width:300px;height:76px;background:rgb(10,120,20) linear-gradient(rgb(200,0,200),rgb(200,0,200))">one two<div style="height:20px"></div>three four</div>`,
			wantKinds: map[paint.CmdKind]int{
				paint.CmdGradient: 1,
				paint.CmdFill:     1,
			},
			want: []string{"(0,0,300,76)"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := tc.w
			if w == 0 {
				w = 800
			}
			cmds := ibDisplayList(t, "", tc.body, w)
			got := abGradientRects(cmds)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("gradient layers emitted %d %v, want %d %v: an anonymous block re-composited its element's background, or the element lost its own",
					len(got), got, len(tc.want), tc.want)
			}
			for k, want := range tc.wantKinds {
				if n := abKindCount(cmds, k); n != want {
					t.Errorf("command kind %s = %d, want %d", k, n, want)
				}
			}
		})
	}
}

// TestAnonymousBlockImageLayerIsNotEmittedPerWrapper covers the fourth background layer,
// background-image. Its url() pixels never reach a node-less box in this engine -
// internal/engine/engine.go only attaches decoded background pixels to objects with a
// DOM node, and cmd/goosie-headless installs no image fetcher at all - so the layer is
// asserted here as a command that must not appear for either box, and the style-layer
// collapse of the url string itself is pinned in test/style/. Stated plainly rather than
// dressed up as a pixel oracle: a url() double-composite is not reachable headlessly.
func TestAnonymousBlockImageLayerIsNotEmittedPerWrapper(t *testing.T) {
	body := `<div style="width:300px;height:76px;background-image:url(/tile.png)">one two<div style="height:20px"></div>three four</div>`
	cmds := ibDisplayList(t, "", body, 800)
	if n := abKindCount(cmds, paint.CmdImage); n != 0 {
		t.Errorf("background-image url emitted %d image command(s), want 0: no background pixels are decoded for this document at all, so a wrapper cannot be adding a second copy", n)
	}
	if n := len(abGradientRects(cmds)); n != 0 {
		t.Errorf("url background produced %d gradient command(s), want 0 (url() is not a ramp)", n)
	}
	// The element still owns the layer as a declaration; the anonymous copy must not.
	// A url() with nothing decoded behind it paints no fill either, so the block's
	// only solid paint is the page.
	if n := abKindCount(cmds, paint.CmdFill); n > 1 {
		t.Errorf("document emitted %d solid fill(s), want at most 1 (the page background)", n)
	}
}

// TestTranslucentAnonymousBlockGradientCompositesOnce is the rasterised half: it
// rasterizes the frozen list exactly as the tile path does and reads the colours back,
// so the defect is counted in pixels rather than in commands. The expectation is
// Chromium's own measurement of the same document - the twice-composited (213,63,63)
// appears nowhere in its render of the block.
func TestTranslucentAnonymousBlockGradientCompositesOnce(t *testing.T) {
	html := `<!doctype html><html><head><style>html,body{margin:0;font:16px serif}</style></head>` +
		`<body><div style="width:300px;height:76px;` +
		`background:linear-gradient(rgba(200,0,0,0.5),rgba(200,0,0,0.5))">one two` +
		`<div style="height:20px"></div>three four</div></body></html>`
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("raster.NewFonts: %v", err)
	}
	sess, err := engine.NewSession(html, nil, 800, engine.WithMetrics(fonts), engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()
	dl, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}

	single := frame.RGB(227, 127, 127)
	doubled := frame.RGB(213, 63, 63)
	bitmap := frame.NewBitmap(300, 76)
	tile := dl.Build(1).Publish()
	if err := raster.RasterizeTile(tile, frame.Rect4(0, 0, 300, 76), bitmap,
		fonts, raster.NewGlyphAtlas(1<<20, fonts), frame.RGB(255, 255, 255)); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}

	// Points inside both anonymous bands - (0,0,300,18) and (0,38,300,18) - at x
	// positions clear of their text: cmd/dump-layout puts the last word box of the
	// first line at (27,0,24,17), ending at x=51, and the second's at
	// (36,38,27,17), ending at x=63, so anything from x=140 on is background only.
	// Sampling on a glyph measures ink, not the composite, which is the mistake
	// TASK-048 §10 records.
	for _, p := range []struct {
		x, y  int
		where string
	}{
		{150, 9, "in the first anonymous band, past its text"},
		{250, 9, "in the first anonymous band, at its right edge"},
		{150, 47, "in the second anonymous band, past its text"},
		{250, 47, "in the second anonymous band, at its right edge"},
		{200, 30, "in the 20px sibling block's band, covered only by the element"},
		{200, 65, "below both bands, covered only by the element"},
	} {
		if got := bitmap.At(p.x, p.y); got != single {
			t.Errorf("pixel (%d,%d) %s = %v, want %v: the element's own 0.5 alpha reached that point more than once",
				p.x, p.y, p.where, ibColorName(got), ibColorName(single))
		}
	}

	// Then the whole block, so no point is cherry-picked. Nothing may carry the
	// twice-composited colour, and almost everything must carry the once value.
	// Measured on this host for this exact document at 400x120 DPR 1: Chromium
	// composites 13,465 px as (227,127,127) and 8,640 px as its (228,127,127)
	// rounding, so 22,105 of the block's 22,800 px are on the once value with 695 px
	// of ink; Goosie after the collapse puts 22,172 px on (227,127,127) and 628 px as
	// ink and antialiasing. Before it, Goosie reads 12,000 px once and 10,172 px
	// doubled - 52.6% of the block, which is the block minus the two anonymous bands -
	// so a floor of 21,000 (92.0%) cannot be met by accident.
	var nDoubled, nOnce, nOther int
	for y := 0; y < 76; y++ {
		for x := 0; x < 300; x++ {
			switch c := bitmap.At(x, y); {
			case c == doubled:
				nDoubled++
			case c == single:
				nOnce++
			default:
				nOther++
			}
		}
	}
	if nDoubled != 0 {
		t.Errorf("%d of 22800 block pixels carry the twice-composited gradient %v; Chromium has none",
			nDoubled, ibColorName(doubled))
	}
	if nOnce < 21000 {
		t.Errorf("only %d of 22800 block pixels carry the once-composited gradient %v, want >=21000 (Chromium measures 13465 of that exact value plus 8640 of (228,127,127); before the collapse Goosie measures 12000)",
			nOnce, ibColorName(single))
	}
	if nOther > 1800 {
		t.Errorf("%d of 22800 block pixels are neither composite value, want <=1800 (glyph ink and its antialiasing: this document measures 628 in Goosie and 695 in Chromium), histogram other=%d once=%d doubled=%d",
			nOther, nOther, nOnce, nDoubled)
	}
}
