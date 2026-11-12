package paint_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/paint"
)

// The paint-side opacity oracles. The Chromium measurements behind the expected
// values come from TASK-044 section 2: opacity:0 paints nothing, a fraction such
// as 0.5 survives the style->paint trip as a real fade, 5 clamps to 1, and an
// ancestor's fraction applies to its whole subtree exactly once.
//
// SP-8 moved the fade off the per-command path: a clean faded box is now recorded
// as a paint.Group carrying its own opacity, and the group fades its subtree once
// at composite. So the "value survived" checks read Group.OwnOpacity, and the
// "counted once" check asserts no member command *also* carries a partial fade
// (that would darken the content twice -- the original regression).

// opFill returns the single opaque-background fill of the requested colour.
func opFill(t *testing.T, cmds []paint.DisplayCmd, r, g, b uint8) paint.DisplayCmd {
	t.Helper()
	got := visSolidFills(cmds, r, g, b)
	if len(got) != 1 {
		t.Fatalf("expected exactly one fill of rgb(%d,%d,%d), got %d", r, g, b, len(got))
	}
	return got[0]
}

func opClose(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-6 }

// partialFade reports whether c is a painted command carrying an opacity strictly
// between 0 and 1 -- a per-command fade that would stack on top of a group fade.
func partialFade(c paint.DisplayCmd) bool {
	return (c.Kind == paint.CmdText || c.Kind == paint.CmdFill) && c.Opacity > 0 && c.Opacity < 1
}

// TestOpacityZeroDropsBox is the primary paint-side defect. Before the fix a
// box with opacity:0 was emitted into the display list with Opacity==0, and the
// rasterizer reads 0 as "unset, therefore opaque" (see DisplayCmd.Opacity), so
// the supposedly-invisible box painted at full strength. Now the box and its
// subtree are dropped before a command is ever built.
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

// TestOpacityFractionReachesCommand proves the fraction survives the whole
// style->paint trip as an actual fade, not as the opaque sentinel. Under the
// group model each faded box is recorded as its own clean group carrying the
// declared value (the leading-dot .5 shorthand included).
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
	// Both boxes still paint.
	opFill(t, dl.All(), 20, 20, 120)
	opFill(t, dl.All(), 120, 20, 20)
}

// TestOpacityAboveOnePaintsOpaque pins the clamp measured in section 2: Chromium
// reads opacity:5 back as 1. The box is painted, and it carries exactly 1.0, so
// the out-of-range value must not survive to the rasterizer as 5. An opacity of
// 1 is not a fade, so it is not recorded as a group either.
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

// TestGroupOpacityAppliesToSubtree is the accumulation half of the fix: opacity
// does not inherit, but a translucent ancestor renders its whole subtree inside
// one faded group. Under SP-8 the parent is a clean group whose OwnOpacity is the
// declared fraction, and both the parent's own fill and the opaque child fill are
// group members carrying no partial fade of their own.
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

// TestGroupOpacityCountedOncePerElement is the regression oracle. Accumulating
// opacity down the box tree must apply one declaration exactly once, whatever
// boxes layout inserts. A box layout synthesizes carries a verbatim copy of the
// generating element's computed style (style.AnonymousBlockStyle and
// style.inheritStyle both do `s = *parent`); the first pass of the original fix
// multiplied that copy and faded raw text twice (0.25 for `opacity:0.5`) or three
// times (0.125 beside a block sibling), where Chromium applies the declaration
// once.
//
// Under the group model the invariant reads two ways: the faded box is recorded as
// a single clean group whose OwnOpacity is the Chromium-measured fraction, and no
// member command carries a partial fade that would stack a second time on top of
// the group's.
//
// Every expectation is a Chromium measurement taken with
// scripts/verify-task044-group-opacity.js on this machine (viewport 400x200,
// DPR 1, black text on white, darkest glyph pixel): min_lum 126.0 is glyph alpha
// 0.506 and 190.0 is 0.255, so a declared 0.5 or 0.25 arrives once.
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
			// The control shape: an element child owns its own box.
			name: "span", want: 0.5, chromium: "min_lum 126.0 -> glyph alpha 0.506",
			body: `<div style="opacity:0.5"><span>anonymous text</span></div>`,
		},
		{
			// Raw text: layout puts it in a box of its own whose style is the
			// faded div's copy. This is the shape the first pass regressed.
			name: "anon", want: 0.5, chromium: "min_lum 126.0 -> glyph alpha 0.506",
			body: `<div style="opacity:0.5">anonymous text</div>`,
		},
		{
			// Raw text beside a block sibling adds the anonymous block wrapper
			// layout invents to stack the run against it, so the copy was
			// multiplied a third time.
			name: "anondiv", want: 0.5, chromium: "min_lum 126.0 -> glyph alpha 0.506",
			body: `<div style="opacity:0.5">anonymous text<div style="background:rgb(10,10,120)">bg</div></div>`,
		},
		{
			// A different fraction, so the oracle is not satisfied by a value
			// that happens to work at one-halves.
			name: "quarter", want: 0.25, chromium: "min_lum 190.0 -> glyph alpha 0.255",
			body: `<div style="opacity:0.25">anonymous text</div>`,
		},
		{
			// opacity:1 is a declaration, not an escape: the child still fades
			// with its ancestor and once.
			name: "nested", want: 0.5, chromium: "min_lum 126.0 -> glyph alpha 0.506",
			body: `<div style="opacity:0.5"><div style="opacity:1">anonymous text</div></div>`,
		},
		{
			// The anonymous-table half of the same rule: raw text beside a block
			// inside a faded cell makes layout splice an anonymous cell whose
			// style is a copy of that cell's (style.AnonymousTableStyle).
			name: "tableAnon", want: 0.5, chromium: "min_lum 126.0 -> glyph alpha 0.506",
			body: `<table><tbody><tr><td style="opacity:0.5">raw cell text<div>block</div></td></tr></tbody></table>`,
		},
		{
			// Generated content is the exception, not a copy: a ::before box owns
			// its own cascade result, so its declared opacity must still fade
			// exactly once.
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
