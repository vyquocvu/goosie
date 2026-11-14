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

// qc:BR-201
func TestAnonymousBlockGradientIsEmittedOncePerElement(t *testing.T) {
	linear := `background-image:linear-gradient(rgb(10,120,20),rgb(10,120,20))`
	radial := `background-image:radial-gradient(rgb(10,120,20),rgb(10,120,20))`
	magenta := `background-image:linear-gradient(rgb(200,0,200),rgb(200,0,200))`
	for _, tc := range []struct {
		name      string
		body      string
		w         int
		want      []string
		wantKinds map[paint.CmdKind]int
	}{
		{
			name: "raw text beside a block sibling, linear gradient",
			body: `<div style="width:300px;height:76px;` + linear + `">one two<div style="height:20px"></div>three four</div>`,
			want: []string{"(0,0,300,76)"},
		},
		{
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
			name: "two nested declaring elements",
			body: `<div style="width:300px;height:96px;` + linear + `">one two<div style="height:20px;` + magenta + `">inner</div>three four</div>`,
			want: []string{"(0,0,300,96)", "(0,18,300,38)"},
		},
		{
			name: "inline fragment inside a block with anonymous children",
			body: `<div style="width:300px;height:76px;` + linear + `">one <span style="` + magenta + `">two</span><div style="height:20px"></div>three four</div>`,
			want: []string{"(0,0,300,76)", "(27,0,52,18)"},
		},
		{
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

// qc:BR-201
func TestAnonymousBlockImageLayerIsNotEmittedPerWrapper(t *testing.T) {
	body := `<div style="width:300px;height:76px;background-image:url(/tile.png)">one two<div style="height:20px"></div>three four</div>`
	cmds := ibDisplayList(t, "", body, 800)
	if n := abKindCount(cmds, paint.CmdImage); n != 0 {
		t.Errorf("background-image url emitted %d image command(s), want 0: no background pixels are decoded for this document at all, so a wrapper cannot be adding a second copy", n)
	}
	if n := len(abGradientRects(cmds)); n != 0 {
		t.Errorf("url background produced %d gradient command(s), want 0 (url() is not a ramp)", n)
	}
	if n := abKindCount(cmds, paint.CmdFill); n > 1 {
		t.Errorf("document emitted %d solid fill(s), want at most 1 (the page background)", n)
	}
}

// qc:BR-202
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
