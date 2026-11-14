package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// qc:BR-073
func TestOutlinePaintsARingThatTouchesNoneOfTheBorderBox(t *testing.T) {
	html := `<!doctype html><html><head><style>` +
		`html,body{margin:0}` +
		`#b{position:absolute;left:200px;top:140px;width:120px;height:60px;` +
		`background:rgb(0,200,0);outline:4px solid rgb(204,0,0);outline-offset:6px}` +
		`</style></head><body><div id="b"></div></body></html>`
	borderBox := frame.Rect4(200, 140, 320, 200)
	var ring []frame.Rect
	boxFill := -1
	for i, cmd := range visDisplayList(t, html) {
		if cmd.Kind != paint.CmdFill {
			continue
		}
		switch cmd.Color {
		case frame.RGB(204, 0, 0):
			ring = append(ring, cmd.Rect)
		case frame.RGB(0, 200, 0):
			if boxFill < 0 {
				boxFill = i
			}
		}
	}
	want := []frame.Rect{
		frame.Rect4(190, 130, 330, 134),
		frame.Rect4(190, 206, 330, 210),
		frame.Rect4(190, 134, 194, 206),
		frame.Rect4(326, 134, 330, 206),
	}
	if len(ring) != len(want) {
		t.Fatalf("outline fills = %v, want the %d bands %v", ring, len(want), want)
	}
	for i, w := range want {
		if ring[i] != w {
			t.Errorf("outline band %d = %v, want %v", i, ring[i], w)
		}
		if area := intersectArea(ring[i], borderBox); area != 0 {
			t.Errorf("outline band %v covers %d px of the box, which the box paints over", ring[i], area)
		}
	}
	if boxFill < 0 {
		t.Error("the box's own background disappeared with the outline")
	}
}

// qc:BR-073
func TestNoOutlineStylePaintsNoRing(t *testing.T) {
	cases := []struct{ label, css string }{
		{"none", `outline:none`},
		{"width without a style", `outline-width:6px;outline-color:rgb(204,0,0)`},
		{"zero width", `outline:0px solid rgb(204,0,0)`},
		{"none style in the shorthand", `outline:6px none rgb(204,0,0)`},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			html := `<!doctype html><html><head><style>` +
				`html,body{margin:0}` +
				`#b{position:absolute;left:40px;top:40px;width:120px;height:60px;` +
				`background:rgb(0,200,0);` + c.css + `}` +
				`</style></head><body><div id="b"></div></body></html>`
			for i, cmd := range visDisplayList(t, html) {
				if cmd.Kind == paint.CmdFill && cmd.Color == frame.RGB(204, 0, 0) {
					t.Errorf("fill %d emitted outline ink %v with %q", i, cmd.Rect, c.css)
				}
			}
		})
	}
}
