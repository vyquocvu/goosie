package paint_test

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/paint"
)

func paintList(t *testing.T, sess *engine.Session) *paint.List {
	t.Helper()
	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	return list
}

func newPaintSession(t *testing.T, html string) *engine.Session {
	t.Helper()
	sess, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)
	return sess
}

// qc:BR-071
func TestZIndexOrdersPositionedSiblingsByTheirDeclaredLayer(t *testing.T) {
	const doc = `<!doctype html><html><head><style>
html,body{margin:0}
.box{position:absolute;top:0;left:0;width:100px;height:100px}
</style></head><body>
<div class="box" style="background:rgb(255,0,0);%s"></div>
<div class="box" style="background:rgb(0,0,255);%s"></div>
</body></html>`

	red := frame.RGB(255, 0, 0)
	blue := frame.RGB(0, 0, 255)

	// The red box is declared first but carries the higher layer, so it must
	// paint last: document order alone would leave blue on top.
	list := paintList(t, newPaintSession(t, fmt.Sprintf(doc, "z-index:2", "z-index:1")))
	redAt, blueAt := -1, -1
	for i, cmd := range list.All() {
		if cmd.Kind != paint.CmdFill {
			continue
		}
		switch cmd.Color {
		case red:
			redAt = i
		case blue:
			blueAt = i
		}
	}
	if redAt < 0 || blueAt < 0 {
		t.Fatalf("missing layered fills: red=%d blue=%d", redAt, blueAt)
	}
	if redAt < blueAt {
		t.Errorf("red (z-index:2) painted at %d before blue (z-index:1) at %d; the higher layer must be on top", redAt, blueAt)
	}

	// Without a z-index the flow is document order, which is the contract the
	// sort must not disturb: blue is declared last and must still be on top.
	list = paintList(t, newPaintSession(t, fmt.Sprintf(doc, "", "")))
	redAt, blueAt = -1, -1
	for i, cmd := range list.All() {
		if cmd.Kind != paint.CmdFill {
			continue
		}
		switch cmd.Color {
		case red:
			redAt = i
		case blue:
			blueAt = i
		}
	}
	if redAt < 0 || blueAt < 0 {
		t.Fatalf("missing auto-layer fills: red=%d blue=%d", redAt, blueAt)
	}
	if blueAt < redAt {
		t.Errorf("blue painted at %d before red at %d without z-index; auto keeps document order", blueAt, redAt)
	}
}

// qc:BR-072
func TestOverflowHiddenClipsEveryCommandInTheSubtree(t *testing.T) {
	sess := newPaintSession(t, `<!doctype html><html><body style="margin:0">
<div id="box" style="overflow:hidden;width:100px;height:100px;position:relative">
<div style="position:absolute;left:50px;top:50px;width:200px;height:200px;background:rgb(255,0,0)"></div>
<span style="position:absolute;left:0;top:150px;white-space:nowrap;color:rgb(0,0,255)">SECRET</span>
</div>
</body></html>`)

	container := frame.Rect4(0, 0, 100, 100)
	list := paintList(t, sess)
	var sawRed bool
	for i, cmd := range list.All() {
		if cmd.Kind == paint.CmdFill && cmd.Color == frame.RGB(255, 0, 0) {
			sawRed = true
			if cmd.Clip.Empty() {
				t.Fatalf("red descendant fill %d carries no clip; it would paint to (%d,%d)-(%d,%d)",
					i, cmd.Rect.X0, cmd.Rect.Y0, cmd.Rect.X1, cmd.Rect.Y1)
			}
		}
		visible, ok := cmd.Bounds()
		if !ok {
			continue
		}
		if !cmd.Clip.Empty() {
			visible = visible.Intersection(cmd.Clip)
		}
		if visible.Empty() {
			continue
		}
		if visible.Intersection(container) != visible {
			t.Fatalf("command %d paints (%d,%d)-(%d,%d), outside the overflow:hidden box (0,0)-(100,100)",
				i, visible.X0, visible.Y0, visible.X1, visible.Y1)
		}
	}
	if !sawRed {
		t.Fatal("the red descendant background was not painted at all; the clip must cut it, not drop it")
	}
}

// qc:BR-072
func TestOverflowScrollMovesAndClipsDescendantPaint(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := engine.NewSession(`<!doctype html><html><body style="margin:0">
<div id="box" style="overflow:auto;width:100px;height:100px">
<div style="height:40px"></div>
<div style="height:20px;background:rgb(0,255,0)"></div>
<div style="height:200px"></div>
</div>
</body></html>`, nil, 800, engine.WithViewportH(600), engine.WithJS(rt))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	green := frame.RGB(0, 255, 0)
	marker := func() (frame.Rect, frame.Rect, bool) {
		list, err := sess.PaintChecked(1)
		if err != nil {
			t.Fatalf("PaintChecked: %v", err)
		}
		for _, cmd := range list.All() {
			if cmd.Kind == paint.CmdFill && cmd.Color == green {
				return cmd.Rect, cmd.Clip, true
			}
		}
		return frame.Rect{}, frame.Rect{}, false
	}

	before, _, ok := marker()
	if !ok {
		t.Fatal("marker background not painted before scrolling")
	}
	if before.Y0 != 40 || before.Y1 != 60 {
		t.Fatalf("marker rect before scroll = y%v..%v, want 40..60", before.Y0, before.Y1)
	}

	if _, err := sess.EvaluateJS(`document.getElementById('box').scrollTop = 50;`); err != nil {
		t.Fatalf("scroll: %v", err)
	}
	after, clip, ok := marker()
	if !ok {
		t.Fatal("marker background not painted after scrolling")
	}
	if after.Y0 != -10 || after.Y1 != 10 {
		t.Errorf("marker rect after scroll = y%v..%v, want -10..10 (content moved up by 50)", after.Y0, after.Y1)
	}
	if clip.Empty() {
		t.Error("scrolled marker carries no clip; the overflow must cut the part above the scrollport")
	} else if visible := after.Intersection(clip); visible.Y0 != 0 || visible.Y1 != 10 {
		t.Errorf("visible marker = y%v..%v, want 0..10 inside the 100px scrollport", visible.Y0, visible.Y1)
	}
}
