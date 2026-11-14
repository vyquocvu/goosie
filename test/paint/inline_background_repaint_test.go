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

func ibDisplayList(t *testing.T, css, body string, viewportW int) []paint.DisplayCmd {
	t.Helper()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("raster.NewFonts: %v", err)
	}
	html := `<!doctype html><html><head><style>html,body{margin:0;font:16px serif}` + css +
		`</style></head><body>` + body + `</body></html>`
	sess, err := engine.NewSession(html, nil, float32(viewportW), engine.WithMetrics(fonts), engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()
	dl, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	return dl.Build(1).All()
}

func ibRectsOf(cmds []paint.DisplayCmd, r, g, b uint8) []string {
	var out []string
	for _, c := range cmds {
		if c.Kind == paint.CmdFill && c.Color.R() == r && c.Color.G() == g && c.Color.B() == b {
			out = append(out, fmt.Sprintf("(%d,%d,%d,%d)", c.Rect.X0, c.Rect.Y0, c.Rect.X1, c.Rect.Y1))
		}
	}
	return out
}

type ibFill struct {
	what      string
	r, g, b   uint8
	wantRects []string
}

// qc:BR-198
func TestBlockBackgroundIsEmittedOnceNotPerTextRun(t *testing.T) {
	for _, tc := range []struct {
		name      string
		css       string
		body      string
		w         int
		want      []ibFill
		wantKinds map[paint.CmdKind]int
	}{
		{
			name: "block with a declared height",
			body: `<div style="width:300px;height:60px;background:rgb(10,120,20)">one two three four</div>`,
			want: []ibFill{{what: "block", r: 10, g: 120, b: 20, wantRects: []string{"(0,0,300,60)"}}},
		},
		{
			name: "block with an auto height",
			body: `<div style="width:300px;background:rgb(10,120,20)">one two three four</div>`,
			want: []ibFill{{what: "block", r: 10, g: 120, b: 20, wantRects: []string{"(0,0,300,18)"}}},
		},
		{
			name: "raw text beside a block sibling",
			body: `<div style="width:300px;background:rgb(10,120,20)">one two<div style="background:rgb(20,20,120)">block child</div>tail three</div>`,
			want: []ibFill{
				{what: "block", r: 10, g: 120, b: 20, wantRects: []string{"(0,0,300,54)"}},
				{what: "sibling", r: 20, g: 20, b: 120, wantRects: []string{"(0,18,300,36)"}},
			},
		},
		{
			name: "inline fragment inside a block",
			body: `<div style="width:300px;background:rgb(10,120,20)">one <span style="background:rgb(200,0,200)">two</span> three four</div>`,
			want: []ibFill{
				{what: "block", r: 10, g: 120, b: 20, wantRects: []string{"(0,0,300,18)"}},
				{what: "span", r: 200, g: 0, b: 200, wantRects: []string{"(27,0,52,18)"}},
			},
		},
		{
			name: "block that wraps onto two lines",
			body: `<div style="width:120px;background:rgb(10,120,20)">one two three four five six</div>`,
			want: []ibFill{{what: "block", r: 10, g: 120, b: 20, wantRects: []string{"(0,0,120,36)"}}},
		},
		{
			name: "table cell",
			body: `<table><tbody><tr><td style="background:rgb(10,120,20)">cell text here</td></tr></tbody></table>`,
			want: []ibFill{{what: "cell", r: 10, g: 120, b: 20, wantRects: []string{"(2,2,89,22)"}}},
		},
		{
			name: "list item",
			body: `<ul style="margin:0;padding:0"><li style="background:rgb(10,120,20)">item one two</li></ul>`,
			want: []ibFill{{what: "item", r: 10, g: 120, b: 20, wantRects: []string{"(0,0,800,18)"}}},
		},
		{
			name: "inline-block",
			body: `<div style="display:inline-block;width:300px;background:rgb(10,120,20)">one two three</div>`,
			want: []ibFill{{what: "block", r: 10, g: 120, b: 20, wantRects: []string{"(0,0,300,18)"}}},
		},
		{
			name: "paragraph",
			body: `<p style="background:rgb(10,120,20);margin:0">one two three four</p>`,
			want: []ibFill{{what: "block", r: 10, g: 120, b: 20, wantRects: []string{"(0,0,800,18)"}}},
		},
		{
			name: "gradient background",
			body: `<div style="width:300px;height:60px;background-image:linear-gradient(rgb(10,120,20),rgb(10,120,20))">one two three four</div>`,
			wantKinds: map[paint.CmdKind]int{
				paint.CmdGradient: 1,
				paint.CmdText:     7,
				paint.CmdFill:     0,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := tc.w
			if w == 0 {
				w = 800
			}
			cmds := ibDisplayList(t, tc.css, tc.body, w)
			wantFills := 0
			for _, f := range tc.want {
				got := ibRectsOf(cmds, f.r, f.g, f.b)
				wantFills += len(f.wantRects)
				if strings.Join(got, ",") != strings.Join(f.wantRects, ",") {
					t.Errorf("%s background rgb(%d,%d,%d) emitted %d fill(s) %v, want %d %v: a text run re-emitted the element's background",
						f.what, f.r, f.g, f.b, len(got), got, len(f.wantRects), f.wantRects)
				}
			}
			var total int
			kinds := map[paint.CmdKind]int{}
			for _, c := range cmds {
				kinds[c.Kind]++
				if c.Kind == paint.CmdFill {
					total++
				}
			}
			if tc.wantKinds == nil && total != wantFills {
				t.Errorf("document emitted %d solid fills in total, want %d (one per declared background)", total, wantFills)
			}
			for k, want := range tc.wantKinds {
				if kinds[k] != want {
					t.Errorf("command kind %s = %d, want %d (full histogram %v)", k, kinds[k], want, kinds)
				}
			}
		})
	}
}

// qc:BR-198
func TestInlineFragmentsStillCarryTheirOwnBackground(t *testing.T) {
	for _, tc := range []struct {
		name string
		css  string
		body string
		w    int
		want []string
	}{
		{
			name: "span wrapping onto two lines",
			body: `<p style="margin:0;width:120px"><span style="background:rgb(200,0,200)">aaa bbb ccc ddd eee fff</span></p>`,
			w:    120,
			want: []string{
				"(0,0,22,18)", "(21,0,26,18)", "(25,0,50,18)", "(49,0,54,18)", "(53,0,75,18)", "(74,0,79,18)", "(78,0,103,18)",
				"(0,18,22,36)", "(21,18,26,36)", "(25,18,42,36)",
			},
		},
		{
			name: "span inside a block",
			body: `<div style="width:300px;background:rgb(10,120,20)">one <span style="background:rgb(200,0,200)">two</span> three four</div>`,
			want: []string{"(27,0,52,18)"},
		},
		{
			name: "generated ::before content",
			css:  `p::before{content:"aa bb";background:rgb(200,0,200)}`,
			body: `<p style="margin:0">x</p>`,
			want: []string{"(0,0,15,18)", "(14,0,19,18)", "(18,0,35,18)"},
		},
		{
			name: "generated ::first-letter",
			css:  `p::first-letter{background:rgb(200,0,200)}`,
			body: `<p style="margin:0">abc def</p>`,
			want: []string{"(0,0,8,18)"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := tc.w
			if w == 0 {
				w = 800
			}
			got := ibRectsOf(ibDisplayList(t, tc.css, tc.body, w), 200, 0, 200)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("inline fragments painted %d fill(s) %v, want %d %v",
					len(got), got, len(tc.want), tc.want)
			}
		})
	}
}

// qc:BR-199
func TestTranslucentBlockBackgroundCompositesOnce(t *testing.T) {
	html := `<!doctype html><html><head><style>html,body{margin:0;font:16px serif}` +
		`div{width:300px;height:60px;background:rgba(200,0,0,0.5)}</style></head>` +
		`<body><div>one two three four</div></body></html>`
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
	bitmap := frame.NewBitmap(300, 60)
	tile := dl.Build(1).Publish()
	if err := raster.RasterizeTile(tile, frame.Rect4(0, 0, 300, 60), bitmap,
		fonts, raster.NewGlyphAtlas(1<<20, fonts), frame.RGB(255, 255, 255)); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	for _, p := range []struct {
		x, y  int
		where string
	}{
		{5, 17, "inside the first word box"},
		{30, 17, "inside the second word box"},
		{60, 17, "inside the third word box"},
		{95, 17, "inside the fourth word box"},
		{200, 40, "in the block, past the text"},
	} {
		if got := bitmap.At(p.x, p.y); got != single {
			t.Errorf("pixel (%d,%d) %s = %v, want %v: the element's own 0.5 alpha reached that point more than once",
				p.x, p.y, p.where, ibColorName(got), ibColorName(single))
		}
	}

	var nDoubled, nOnce, nOther int
	for y := 0; y < 60; y++ {
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
		t.Errorf("%d of 18000 block pixels carry the twice-composited background %v; Chromium has none",
			nDoubled, ibColorName(doubled))
	}
	if nOnce < 17000 {
		t.Errorf("only %d of 18000 block pixels carry the once-composited background %v, want >=17000 (Chromium measures 17300)",
			nOnce, ibColorName(single))
	}
}

func ibColorName(c frame.Color) string {
	return fmt.Sprintf("(%d,%d,%d)", c.R(), c.G(), c.B())
}
