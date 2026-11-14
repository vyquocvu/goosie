package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/paint"
)

func ibBorders(cmds []paint.DisplayCmd) []paint.DisplayCmd {
	var out []paint.DisplayCmd
	for _, c := range cmds {
		if c.Kind == paint.CmdBorder {
			out = append(out, c)
		}
	}
	return out
}

func TestInlineBorderFragments(t *testing.T) {
	css := `div{width:80px;background:rgba(200,0,0,.5)}span{border:3px solid rgba(0,0,200,.5)}`
	body := `<div><span>aa bb cc dd ee ff</span></div>`
	cmds := ibDisplayList(t, css, body, 100)
	borders := ibBorders(cmds)
	if len(borders) < 2 {
		t.Fatalf("wrapping span: got %d border commands, want >= 2 (one per line)", len(borders))
	}
	first := borders[0]
	last := borders[len(borders)-1]
	if first.Border.Left.Width == 0 {
		t.Errorf("first fragment must draw the inline-start (left) edge")
	}
	if first.Border.Right.Width != 0 {
		t.Errorf("first fragment must NOT draw the inline-end (right) edge, got width %d", first.Border.Right.Width)
	}
	if last.Border.Right.Width == 0 {
		t.Errorf("last fragment must draw the inline-end (right) edge")
	}
	if last.Border.Left.Width != 0 {
		t.Errorf("last fragment must NOT draw the inline-start (left) edge, got width %d", last.Border.Left.Width)
	}
	for i, b := range borders {
		if b.Border.Top.Width == 0 || b.Border.Bottom.Width == 0 {
			t.Errorf("fragment %d must draw top and bottom edges", i)
		}
	}
}

func TestInlineBorderSingleLine(t *testing.T) {
	css := `div{width:400px}span{border:3px solid rgba(0,0,200,.5)}`
	body := `<div><span>word</span></div>`
	cmds := ibDisplayList(t, css, body, 400)
	borders := ibBorders(cmds)
	if len(borders) != 1 {
		t.Fatalf("single-line span: got %d border commands, want 1", len(borders))
	}
	b := borders[0]
	if b.Border.Top.Width == 0 || b.Border.Bottom.Width == 0 || b.Border.Left.Width == 0 || b.Border.Right.Width == 0 {
		t.Errorf("single-line span must draw all four edges, got %+v", b.Border)
	}
}

func TestInlineBorderNone(t *testing.T) {
	css := `div{width:80px}span{color:blue}`
	body := `<div><span>aa bb cc dd</span></div>`
	cmds := ibDisplayList(t, css, body, 100)
	if got := len(ibBorders(cmds)); got != 0 {
		t.Fatalf("borderless span: got %d border commands, want 0", got)
	}
}
