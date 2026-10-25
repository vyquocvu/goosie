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
