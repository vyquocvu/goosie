package paint_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/paint"
)

// opFill returns the single opaque-background fill of the requested colour and
// reports its carried opacity. The oracle values come from the Chromium
// measurements in TASK-044 section 2: opacity:0 paints nothing, 0.5 reaches the
// rasterizer as a real fade, 5 clamps to 1, and an ancestor's fraction applies
// to its whole subtree.

func opFill(t *testing.T, cmds []paint.DisplayCmd, r, g, b uint8) paint.DisplayCmd {
	t.Helper()
	got := visSolidFills(cmds, r, g, b)
	if len(got) != 1 {
		t.Fatalf("expected exactly one fill of rgb(%d,%d,%d), got %d", r, g, b, len(got))
	}
	return got[0]
}

func opClose(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-6 }

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

// TestOpacityFractionReachesCommand proves the value survives the whole
// style->paint trip as an actual fraction, not as the opaque sentinel 0/1.
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
	cmds := visDisplayList(t, html)

	if op := opFill(t, cmds, 20, 20, 120).Opacity; !opClose(op, 0.5) {
		t.Errorf("opacity:0.5 carried %v in the display command, want 0.5", op)
	}
	if op := opFill(t, cmds, 120, 20, 20).Opacity; !opClose(op, 0.5) {
		t.Errorf("opacity:.5 carried %v in the display command, want 0.5", op)
	}
}

// TestOpacityAboveOnePaintsOpaque pins the clamp measured in section 2: Chromium
// reads opacity:5 back as 1. The box is painted, and it carries exactly 1.0, so
// the out-of-range value must not survive to the rasterizer as 5.
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
// one faded group, so an opaque child of an opacity:0.5 parent must itself reach
// the rasterizer at 0.5, not at 1.
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
	cmds := visDisplayList(t, html)

	parent := opFill(t, cmds, 200, 0, 0)
	child := opFill(t, cmds, 0, 0, 200)
	if !opClose(parent.Opacity, 0.5) {
		t.Errorf("opacity:0.5 parent carried %v, want 0.5", parent.Opacity)
	}
	if !opClose(child.Opacity, 0.5) {
		t.Errorf("opaque child of an opacity:0.5 parent carried %v, want 0.5 (group opacity applies to the subtree)", child.Opacity)
	}
}

// opPaintedOpacity returns the opacity carried by every text and fill command,
// which is the whole product the rasterizer will apply. A shape whose commands
// disagree has faded some part of it more than once.
func opPaintedOpacity(cmds []paint.DisplayCmd) []float32 {
	var out []float32
	for _, c := range cmds {
		if c.Kind == paint.CmdText || c.Kind == paint.CmdFill {
			out = append(out, c.Opacity)
		}
	}
	return out
}

// TestGroupOpacityCountedOncePerElement is the regression oracle. Accumulating
// opacity down the box tree is right, but a box layout synthesized carries a
// verbatim copy of the generating element's computed style
// (style.AnonymousBlockStyle and style.inheritStyle both do `s = *parent`), and
// that element already contributed. Multiplying the copy faded raw text once per
// generated box: the first pass of this fix measured 0.25 for
// `opacity:0.5` + raw text and 0.125 for the same text beside a block sibling,
// where Chromium applies the declaration once no matter how many boxes layout
// inserts.
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
			// The control shape: an element child owns its own box, so even the
			// buggy walk faded it exactly once.
			name: "span", want: 0.5, chromium: "min_lum 126.0 -> glyph alpha 0.506",
			body: `<div style="opacity:0.5"><span>anonymous text</span></div>`,
		},
		{
			// Raw text: layout puts it in a box of its own whose style is the
			// faded div's copy. This is the shape the first pass regressed;
			// before the whole fix it was right by accident.
			name: "anon", want: 0.5, chromium: "min_lum 126.0 -> glyph alpha 0.506",
			body: `<div style="opacity:0.5">anonymous text</div>`,
		},
		{
			// Raw text beside a block sibling adds the anonymous block wrapper
			// layout invents to stack the run against it, so the copy was
			// multiplied a third time. The sibling's own fill belongs to an
			// element and fades once.
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
			// style is a copy of that cell's (style.AnonymousTableStyle), so the
			// declaration was counted once per wrapper.
			name: "tableAnon", want: 0.5, chromium: "min_lum 126.0 -> glyph alpha 0.506",
			body: `<table><tbody><tr><td style="opacity:0.5">raw cell text<div>block</div></td></tr></tbody></table>`,
		},
		{
			// Generated content is the exception, not a copy: a ::before box owns
			// its own cascade result, so its declared opacity must still fade
			// exactly once. Chromium measured 0.506 for this markup with no other
			// text on the page, and 1.000 for the same pseudo without the
			// declaration.
			name: "pseudoOwn", want: 0.5, chromium: "min_lum 126.0 -> glyph alpha 0.506",
			css:  `#t::before{content:"gen";opacity:0.5}`,
			body: `<div id="t"></div>`,
		},
	}
	for _, s := range shapes {
		t.Run(s.name, func(t *testing.T) {
			html := `<!DOCTYPE html><html><head><style>html,body{margin:0}body{font:16px serif;color:#000}` +
				s.css + `</style></head><body>` + s.body + `</body></html>`
			got := opPaintedOpacity(visDisplayList(t, html))
			if len(got) == 0 {
				t.Fatalf("shape painted nothing; want text or fill commands to check")
			}
			for i, op := range got {
				if !opClose(op, s.want) {
					t.Errorf("command %d carried opacity %v, want %v (Chromium %s: one declaration is counted once, whatever boxes layout inserts)",
						i, op, s.want, s.chromium)
				}
			}
		})
	}
}
