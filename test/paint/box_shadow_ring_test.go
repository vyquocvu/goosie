package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func shadowFills(t *testing.T, html string) (fills []frame.Rect, boxFillAt, shadowAt int) {
	t.Helper()
	boxFillAt, shadowAt = -1, -1
	for i, cmd := range visDisplayList(t, html) {
		if cmd.Kind != paint.CmdFill {
			continue
		}
		switch cmd.Color {
		case frame.RGB(128, 0, 128):
			fills = append(fills, cmd.Rect)
			if shadowAt < 0 {
				shadowAt = i
			}
		case frame.RGB(0, 200, 0):
			if boxFillAt < 0 {
				boxFillAt = i
			}
		}
	}
	return fills, boxFillAt, shadowAt
}

const shadowFixtureHead = `<!doctype html><html><head><style>` +
	`html,body{margin:0}` +
	`#b{position:absolute;left:200px;top:260px;width:120px;height:60px;` +
	`background:rgb(0,200,0);box-shadow:`

const shadowFixtureTail = ` rgb(128,0,128)}` +
	`</style></head><body><div id="b"></div></body></html>`

// qc:BR-073
func TestOuterBoxShadowPaintsOnlyTheRingOutsideTheBorderBox(t *testing.T) {
	fills, boxFillAt, shadowAt := shadowFills(t, shadowFixtureHead+`0 0 0 8px`+shadowFixtureTail)
	want := []frame.Rect{
		frame.Rect4(192, 252, 328, 260),
		frame.Rect4(192, 320, 328, 328),
		frame.Rect4(192, 260, 200, 320),
		frame.Rect4(320, 260, 328, 320),
	}
	if len(fills) != len(want) {
		t.Fatalf("shadow fills = %v, want the %d strips %v", fills, len(want), want)
	}
	borderBox := frame.Rect4(200, 260, 320, 320)
	for i, w := range want {
		if fills[i] != w {
			t.Errorf("shadow strip %d = %v, want %v", i, fills[i], w)
		}
		if area := intersectArea(fills[i], borderBox); area != 0 {
			t.Errorf("shadow strip %v covers %d px of the box interior", fills[i], area)
		}
	}
	if boxFillAt < 0 || shadowAt < 0 {
		t.Fatalf("missing fills: box=%d shadow=%d", boxFillAt, shadowAt)
	}
	if shadowAt > boxFillAt {
		t.Errorf("shadow emitted at %d after the box fill at %d: a shadow paints behind its element", shadowAt, boxFillAt)
	}
}

// qc:BR-073
func TestOffsetBoxShadowInksOnlyTheSideItIsPushedTo(t *testing.T) {
	fills, _, _ := shadowFills(t, shadowFixtureHead+`10px 10px 0 8px`+shadowFixtureTail)
	want := []frame.Rect{
		frame.Rect4(202, 320, 338, 338),
		frame.Rect4(320, 260, 338, 320),
	}
	if len(fills) != len(want) {
		t.Fatalf("shadow fills = %v, want %v", fills, want)
	}
	for i, w := range want {
		if fills[i] != w {
			t.Errorf("shadow strip %d = %v, want %v", i, fills[i], w)
		}
	}
}

// qc:BR-073
func TestInsetOnlyShadowKeepsItsInkInsideTheBorderBox(t *testing.T) {
	fills, boxFillAt, _ := shadowFills(t, `<!doctype html><html><head><style>`+
		`html,body{margin:0}`+
		`#b{position:absolute;left:40px;top:40px;width:120px;height:60px;`+
		`background:rgb(0,200,0);box-shadow:inset 4px 4px 0 2px rgb(128,0,128)}`+
		`</style></head><body><div id="b"></div></body></html>`)
	borderBox := frame.Rect4(40, 40, 160, 100)
	if len(fills) == 0 {
		t.Fatal("an inset layer painted nothing: it belongs inside the border box, not behind the element")
	}
	for i, f := range fills {
		if area := intersectArea(f, borderBox); area != f.W()*f.H() {
			t.Errorf("inset fill %d %v puts %d px outside the border box", i, f, f.W()*f.H()-area)
		}
	}
	if boxFillAt < 0 {
		t.Error("the box's own background disappeared with the inset layer")
	}
}

func intersectArea(a, b frame.Rect) int32 {
	x0, y0 := max(a.X0, b.X0), max(a.Y0, b.Y0)
	x1, y1 := min(a.X1, b.X1), min(a.Y1, b.Y1)
	if x1 <= x0 || y1 <= y0 {
		return 0
	}
	return (x1 - x0) * (y1 - y0)
}
