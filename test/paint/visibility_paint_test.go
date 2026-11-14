package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

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

func visSolidFills(cmds []paint.DisplayCmd, r, g, b uint8) []paint.DisplayCmd {
	var out []paint.DisplayCmd
	for _, c := range cmds {
		if c.Kind == paint.CmdFill && c.Color.R() == r && c.Color.G() == g && c.Color.B() == b {
			out = append(out, c)
		}
	}
	return out
}

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

func TestClosedDetailsContentKeepsBoxWithoutPainting(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>body{margin:0}</style></head><body>
	<details><summary>closed</summary>
		<div style="width:100px;height:100px;background:rgb(211,37,91)">hidden</div>
	</details>
	<div style="width:20px;height:20px;background:rgb(17,83,149)"></div>
	</body></html>`
	cmds := visDisplayList(t, html)
	if got := visSolidFills(cmds, 211, 37, 91); len(got) != 0 {
		t.Errorf("closed details content painted %d background fill(s); want none", len(got))
	}
	if got := visSolidFills(cmds, 17, 83, 149); len(got) != 1 {
		t.Errorf("visible content after details produced %d fills; want one", len(got))
	}
}
