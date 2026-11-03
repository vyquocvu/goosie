package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// These oracles pin the shape of the ink an outer box-shadow puts on the page:
// the shadow is clipped to the OUTSIDE of the border box (CSS Backgrounds and
// Borders §7.1.1), so what reaches the display list is the shadow rect minus the
// box, emitted behind it. Chromium measures the same ring, and it is the reason
// the shadow band is not part of the hit area (test/engine/effects_hit_test.go).

// shadowFills returns the fills painted in the shadow's colour in display-list
// order, the index of the box's own background fill, and the index of the first
// shadow fill.
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

// A symmetric spread of 8px on the 120x60 box at (200,260) grows the shadow to
// (192,252)-(328,328), so the ring is that rect minus the border box: two
// full-width bands of 8px above and below, two 8x60 bands between them.
//
// qc:BR-073
func TestOuterBoxShadowPaintsOnlyTheRingOutsideTheBorderBox(t *testing.T) {
	fills, boxFillAt, shadowAt := shadowFills(t, shadowFixtureHead+`0 0 0 8px`+shadowFixtureTail)
	want := []frame.Rect{
		frame.Rect4(192, 252, 328, 260), // above
		frame.Rect4(192, 320, 328, 328), // below
		frame.Rect4(192, 260, 200, 320), // left
		frame.Rect4(320, 260, 328, 320), // right
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

// The offset case is what catches a shadow emitted as a plain rect: `10px 10px 0
// 8px` moves the shadow rect to (202,262)-(338,338), which no longer reaches
// above or left of the box, so only the below and right bands take ink.
//
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

// Inset shadows are a documented no-op in this builder, so a box declaring only
// an inset layer must emit no shadow ink at all - and must not lose its own
// background either.
//
// qc:BR-073
func TestInsetOnlyShadowPaintsNoRing(t *testing.T) {
	fills, boxFillAt, _ := shadowFills(t, `<!doctype html><html><head><style>`+
		`html,body{margin:0}`+
		`#b{position:absolute;left:40px;top:40px;width:120px;height:60px;`+
		`background:rgb(0,200,0);box-shadow:inset 4px 4px 0 2px rgb(128,0,128)}`+
		`</style></head><body><div id="b"></div></body></html>`)
	if len(fills) != 0 {
		t.Errorf("inset shadow emitted %d fills %v, want none (inset is not painted yet)", len(fills), fills)
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
