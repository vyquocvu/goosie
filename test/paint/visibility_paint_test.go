package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

// These oracles sit at the style->paint seam: they build a real document, run
// the frozen display list (Session.PaintChecked, the untrusted-input contract),
// and assert on the commands that reach the rasterizer. The values are pinned
// to the Chromium measurements recorded in TASK-044 section 2, not to Goosie's
// own prior behaviour.

func visDisplayList(t *testing.T, html string) []paint.DisplayCmd {
	t.Helper()
	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	dl, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	list := dl.Build(1)
	if list == nil {
		t.Fatalf("Build returned nil list")
	}
	return list.All()
}

// visSolidFills returns every opaque fill command whose premultiplied colour is
// the requested rgb. Background colours here are chosen to be unique per box so
// presence or absence of one colour identifies exactly one box.
func visSolidFills(cmds []paint.DisplayCmd, r, g, b uint8) []paint.DisplayCmd {
	var out []paint.DisplayCmd
	for _, c := range cmds {
		if c.Kind == paint.CmdFill && c.Color.R() == r && c.Color.G() == g && c.Color.B() == b {
			out = append(out, c)
		}
	}
	return out
}

// TestVisibilityHiddenDropsBoxPaint is the core defect: an author hides a box,
// so none of its own paint - background, border, text - may reach the display
// list. Before the fix the box painted anyway because the builder consulted
// Visibility only for list markers.
// qc:BR-195
func TestVisibilityHiddenDropsBoxPaint(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
	body{margin:0}
	.hidden{visibility:hidden;width:120px;height:60px;background:rgb(11,22,33);border:4px solid rgb(44,55,66)}
	</style></head><body>
	<div class="hidden">hidden text</div>
	</body></html>`
	cmds := visDisplayList(t, html)

	if got := visSolidFills(cmds, 11, 22, 33); len(got) != 0 {
		t.Errorf("visibility:hidden background painted %d fill(s); want 0", len(got))
	}
	for _, c := range cmds {
		if c.Kind == paint.CmdBorder {
			t.Errorf("visibility:hidden box emitted a border command at %v; want none", c.Rect)
		}
		if c.Kind == paint.CmdText {
			for _, g := range c.Text.Glyphs {
				if g.Rune == 'h' {
					t.Errorf("visibility:hidden box painted its text; want none")
					break
				}
			}
		}
	}
}

// TestVisibilityHiddenSubtreeInheritsHidden proves hidden cascades to the
// subtree (visibility is an inherited property) - a child that says nothing is
// hidden too - while a child that reasserts visible is painted again. This is
// the shape Chromium showed when measured (section 2.3).
// qc:BR-195
func TestVisibilityHiddenSubtreeInheritsHidden(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
	body{margin:0}
	.parent{visibility:hidden;width:200px;height:100px;background:rgb(100,10,10)}
	.childHidden{width:80px;height:40px;background:rgb(10,100,10)}
	.childShown{visibility:visible;width:80px;height:40px;background:rgb(10,10,100)}
	</style></head><body>
	<div class="parent">
		<div class="childHidden"></div>
		<div class="childShown"></div>
	</div>
	</body></html>`
	cmds := visDisplayList(t, html)

	if got := visSolidFills(cmds, 100, 10, 10); len(got) != 0 {
		t.Errorf("hidden parent background painted %d fill(s); want 0", len(got))
	}
	if got := visSolidFills(cmds, 10, 100, 10); len(got) != 0 {
		t.Errorf("child that inherits hidden painted %d fill(s); want 0", len(got))
	}
	if got := visSolidFills(cmds, 10, 10, 100); len(got) == 0 {
		t.Errorf("child that reasserts visibility:visible painted nothing; want its blue fill restored")
	}
}

// TestVisibilityHiddenKeepsLayout shows the mechanism is not display:none: a
// hidden box still reserves its box in flow, so the block after it starts below
// it. This is the half of the acceptance the fix must not break while dropping
// the paint.
// qc:BR-195
func TestVisibilityHiddenKeepsLayout(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
	body{margin:0}
	.hidden{visibility:hidden;height:50px;background:rgb(1,1,1)}
	.after{height:50px;background:rgb(2,2,2)}
	</style></head><body>
	<div class="hidden"></div>
	<div class="after"></div>
	</body></html>`
	cmds := visDisplayList(t, html)

	// The hidden box emits no fill, yet the following box must be pushed down by
	// the hidden box's 50px height.
	if got := visSolidFills(cmds, 1, 1, 1); len(got) != 0 {
		t.Errorf("hidden box emitted %d fill(s); want 0", len(got))
	}
	after := visSolidFills(cmds, 2, 2, 2)
	if len(after) != 1 {
		t.Fatalf("expected exactly one fill for the visible box after the hidden one, got %d", len(after))
	}
	if after[0].Rect.Y0 != 50 {
		t.Errorf("visible box top is %d, want 50: visibility:hidden must still occupy layout space", after[0].Rect.Y0)
	}
}

// TestDisplayNoneStillCollapses is the contrast case that keeps the two
// mechanisms from being conflated: display:none removes the box from flow, so
// the next block sits at the top, and it paints nothing.
// qc:BR-195
func TestDisplayNoneStillCollapses(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
	body{margin:0}
	.none{display:none;height:50px;background:rgb(3,3,3)}
	.after{height:50px;background:rgb(4,4,4)}
	</style></head><body>
	<div class="none"></div>
	<div class="after"></div>
	</body></html>`
	cmds := visDisplayList(t, html)

	if got := visSolidFills(cmds, 3, 3, 3); len(got) != 0 {
		t.Errorf("display:none box emitted %d fill(s); want 0", len(got))
	}
	after := visSolidFills(cmds, 4, 4, 4)
	if len(after) != 1 {
		t.Fatalf("expected one fill for the box after the display:none one, got %d", len(after))
	}
	if after[0].Rect.Y0 != 0 {
		t.Errorf("visible box top is %d, want 0: display:none must not occupy layout space", after[0].Rect.Y0)
	}
}
