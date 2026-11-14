package paint_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/paint"
)

func opFill(t *testing.T, cmds []paint.DisplayCmd, r, g, b uint8) paint.DisplayCmd {
	t.Helper()
	got := visSolidFills(cmds, r, g, b)
	if len(got) != 1 {
		t.Fatalf("expected exactly one fill of rgb(%d,%d,%d), got %d", r, g, b, len(got))
	}
	return got[0]
}

func opClose(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-6 }

func partialFade(c paint.DisplayCmd) bool {
	return (c.Kind == paint.CmdText || c.Kind == paint.CmdFill) && c.Opacity > 0 && c.Opacity < 1
}

// qc:BR-196
func TestOpacityZeroDropsBox(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
	body{margin:0}
	.a{opacity:0;width:100px;height:50px;background:rgb(120,10,10)}
	.b{opacity:0%;width:100px;height:50px;background:rgb(10,120,10)}
	</style></head><body>
	<div class="a"></div>
	<div class="b"></div>
	</body></html>`
	cmds := visDisplayList(t, html)

	if got := visSolidFills(cmds, 120, 10, 10); len(got) != 0 {
		t.Errorf("opacity:0 box emitted %d fill(s); want 0 (a fully transparent element paints nothing)", len(got))
	}
	if got := visSolidFills(cmds, 10, 120, 10); len(got) != 0 {
		t.Errorf("opacity:0%% box emitted %d fill(s); want 0", len(got))
	}
}

// qc:BR-196
func TestOpacityFractionReachesCommand(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
	body{margin:0}
	.half{opacity:0.5;width:100px;height:50px;background:rgb(20,20,120)}
	.dot{opacity:.5;width:100px;height:50px;background:rgb(120,20,20)}
	</style></head><body>
	<div class="half"></div>
	<div class="dot"></div>
	</body></html>`
	dl := gpLayer(t, html)

	gs := dl.Groups()
	if len(gs) != 2 {
		t.Fatalf("Groups() = %d, want 2 (two sibling faded boxes, one clean group each)", len(gs))
	}
	for _, g := range gs {
		if !opClose(g.OwnOpacity, 0.5) {
			t.Errorf("group OwnOpacity = %v, want 0.5: the fraction must survive as a real fade, not the opaque sentinel", g.OwnOpacity)
		}
	}
	opFill(t, dl.All(), 20, 20, 120)
	opFill(t, dl.All(), 120, 20, 20)
}

// qc:BR-196
func TestOpacityAboveOnePaintsOpaque(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
	body{margin:0}
	.big{opacity:5;width:100px;height:50px;background:rgb(30,90,30)}
	</style></head><body>
	<div class="big"></div>
	</body></html>`
	cmds := visDisplayList(t, html)

	cmd := opFill(t, cmds, 30, 90, 30)
	if !opClose(cmd.Opacity, 1) {
		t.Errorf("opacity:5 carried %v, want 1 (clamped): the box must paint like an opaque one", cmd.Opacity)
	}
}

// qc:BR-196
func TestGroupOpacityAppliesToSubtree(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
	body{margin:0}
	.parent{opacity:0.5;width:200px;height:100px;background:rgb(200,0,0)}
	.child{width:100px;height:50px;background:rgb(0,0,200)}
	</style></head><body>
	<div class="parent">
		<div class="child"></div>
	</div>
	</body></html>`
	dl := gpLayer(t, html)

	gs := dl.Groups()
	if len(gs) != 1 {
		t.Fatalf("Groups() = %d, want 1 (the opacity:0.5 parent is one clean group spanning its subtree)", len(gs))
	}
	if !opClose(gs[0].OwnOpacity, 0.5) {
		t.Errorf("parent group OwnOpacity = %v, want 0.5", gs[0].OwnOpacity)
	}
	parent := opFill(t, dl.All(), 200, 0, 0)
	child := opFill(t, dl.All(), 0, 0, 200)
	for _, c := range []paint.DisplayCmd{parent, child} {
		if partialFade(c) {
			t.Errorf("subtree member fill carried per-command opacity %v, want the group to hold the fade once", c.Opacity)
		}
	}
}

// qc:BR-196
func TestGroupOpacityCountedOncePerElement(t *testing.T) {
	shapes := []struct {
		name     string
		want     float32
		chromium string
		css      string
		body     string
	}{
		{
			name: "span", want: 0.5, chromium: "min_lum 126.0 -> glyph alpha 0.506",
			body: `<div style="opacity:0.5"><span>anonymous text</span></div>`,
		},
		{
			name: "anon", want: 0.5, chromium: "min_lum 126.0 -> glyph alpha 0.506",
			body: `<div style="opacity:0.5">anonymous text</div>`,
		},
		{
			name: "anondiv", want: 0.5, chromium: "min_lum 126.0 -> glyph alpha 0.506",
			body: `<div style="opacity:0.5">anonymous text<div style="background:rgb(10,10,120)">bg</div></div>`,
		},
		{
			name: "quarter", want: 0.25, chromium: "min_lum 190.0 -> glyph alpha 0.255",
			body: `<div style="opacity:0.25">anonymous text</div>`,
		},
		{
			name: "nested", want: 0.5, chromium: "min_lum 126.0 -> glyph alpha 0.506",
			body: `<div style="opacity:0.5"><div style="opacity:1">anonymous text</div></div>`,
		},
		{
			name: "tableAnon", want: 0.5, chromium: "min_lum 126.0 -> glyph alpha 0.506",
			body: `<table><tbody><tr><td style="opacity:0.5">raw cell text<div>block</div></td></tr></tbody></table>`,
		},
		{
			name: "pseudoOwn", want: 0.5, chromium: "min_lum 126.0 -> glyph alpha 0.506",
			css:  `#t::before{content:"gen";opacity:0.5}`,
			body: `<div id="t"></div>`,
		},
	}
	for _, s := range shapes {
		t.Run(s.name, func(t *testing.T) {
			html := `<!DOCTYPE html><html><head><style>html,body{margin:0}body{font:16px serif;color:#000}` +
				s.css + `</style></head><body>` + s.body + `</body></html>`
			dl := gpLayer(t, html)
			gs := dl.Groups()
			if len(gs) != 1 {
				t.Fatalf("Groups() = %d, want 1 (one faded declaration is one clean group): %s", len(gs), s.chromium)
			}
			if !opClose(gs[0].OwnOpacity, s.want) {
				t.Errorf("group OwnOpacity = %v, want %v (Chromium %s)", gs[0].OwnOpacity, s.want, s.chromium)
			}
			for _, c := range dl.All() {
				if partialFade(c) {
					t.Errorf("member command carried per-command opacity %v; the declaration would be counted twice on top of the group fade (Chromium %s)", c.Opacity, s.chromium)
				}
			}
		})
	}
}
