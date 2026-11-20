package paint_test

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func bccBorder(t *testing.T, rule string) paint.BorderSpec {
	t.Helper()
	cmds := ibBorders(ibDisplayList(t, `div{box-sizing:border-box;width:120px;height:60px;`+rule+`}`, `<div></div>`, 320))
	if len(cmds) != 1 {
		t.Fatalf("rule %q emitted %d border commands, want exactly one", rule, len(cmds))
	}
	return cmds[0].Border
}

type bccSides struct {
	top, right, bottom, left frame.Color
	widths                   [4]int32
}

func bccAll(c frame.Color) bccSides {
	return bccSides{top: c, right: c, bottom: c, left: c, widths: [4]int32{6, 6, 6, 6}}
}

func bccShow(c frame.Color) string {
	return fmt.Sprintf("rgba(%d,%d,%d,%d)", c.R(), c.G(), c.B(), c.A())
}

func bccCheck(t *testing.T, rule string, want bccSides) {
	t.Helper()
	b := bccBorder(t, rule)
	got := [4]frame.Color{b.Top.Color, b.Right.Color, b.Bottom.Color, b.Left.Color}
	widths := [4]int32{b.Top.Width, b.Right.Width, b.Bottom.Width, b.Left.Width}
	names := [4]string{"top", "right", "bottom", "left"}
	wantColours := [4]frame.Color{want.top, want.right, want.bottom, want.left}
	for i := 0; i < 4; i++ {
		if got[i] != wantColours[i] {
			t.Errorf("rule %q border-%s colour = %s, want %s (measured on Chromium 153.0.8010.12 by scripts/verify-border-currentcolor.js)",
				rule, names[i], bccShow(got[i]), bccShow(wantColours[i]))
		}
		if widths[i] != want.widths[i] {
			t.Errorf("rule %q border-%s width = %d, want %d", rule, names[i], widths[i], want.widths[i])
		}
	}
}

// qc:BR-327
func TestUnspecifiedBorderColorPaintsTheElementEdge(t *testing.T) {
	for _, tc := range []struct {
		rule string
		want bccSides
	}{
		{"border-width:6px;border-style:solid;color:rgb(0,0,255)", bccAll(frame.RGB(0, 0, 255))},
		{"border:6px solid;color:rgb(255,0,0)", bccAll(frame.RGB(255, 0, 0))},
		{"border-width:6px;border-style:solid;border-color:currentcolor;color:rgb(0,200,200)", bccAll(frame.RGB(0, 200, 200))},
		{"border-width:6px;border-style:solid;border-color:initial;color:rgb(10,20,30)", bccAll(frame.RGB(10, 20, 30))},
		{"border-width:6px;border-style:solid;border-color:unset;color:rgb(10,20,30)", bccAll(frame.RGB(10, 20, 30))},
		{"border-width:6px;border-style:solid;border-color:notacolor;color:rgb(0,0,255)", bccAll(frame.RGB(0, 0, 255))},
	} {
		bccCheck(t, tc.rule, tc.want)
	}
}

// qc:BR-327
func TestSidesWithNoEdgeEmitNoBorderPaint(t *testing.T) {
	cmds := ibBorders(ibDisplayList(t, `div{box-sizing:border-box;width:120px;height:60px;border-width:6px;border-style:none;color:rgb(0,0,255)}`, `<div></div>`, 320))
	if len(cmds) != 0 {
		t.Errorf("border-style:none emitted %d border commands, want none", len(cmds))
	}
	b := bccBorder(t, "border-width:6px;border-style:solid;border-color:transparent;color:rgb(0,0,255)")
	for i, side := range [4]paint.SideSpec{b.Top, b.Right, b.Bottom, b.Left} {
		if side.Color.A() != 0 {
			t.Errorf("border-color:transparent left side %d with alpha %d, want 0: the reference paints nothing here, which is what separates a declared transparent from an undeclared colour", i, side.Color.A())
		}
	}
}

// qc:BR-327
func TestDeclaredBorderColorWinsOverTheElementColour(t *testing.T) {
	blue := frame.RGB(0, 0, 255)
	red := frame.RGB(255, 0, 0)
	for _, tc := range []struct {
		rule string
		want bccSides
	}{
		{"border:6px solid rgb(0,0,255);color:rgb(255,0,0)", bccSides{top: blue, right: blue, bottom: blue, left: blue, widths: [4]int32{6, 6, 6, 6}}},
		{"border-width:6px;border-style:solid;border-color:rgb(255,0,0) rgb(0,0,255);color:rgb(0,128,0)", bccSides{top: red, right: blue, bottom: red, left: blue, widths: [4]int32{6, 6, 6, 6}}},
		{"border-width:6px;border-style:solid;border-color:rgb(255,0,0) rgb(0,200,0) rgb(0,0,255) rgb(200,200,0);color:rgb(9,9,9)", bccSides{top: red, right: frame.RGB(0, 200, 0), bottom: blue, left: frame.RGB(200, 200, 0), widths: [4]int32{6, 6, 6, 6}}},
		{"border:6px solid;border-left-color:rgb(255,0,255);color:rgb(0,0,255)", bccSides{top: blue, right: blue, bottom: blue, left: frame.RGB(255, 0, 255), widths: [4]int32{6, 6, 6, 6}}},
		{"border-width:6px;border-style:solid;border-color:rgb(255,0,0);border-color:notacolor;color:rgb(0,128,0)", bccAll(red)},
	} {
		bccCheck(t, tc.rule, tc.want)
	}
}

// qc:BR-327
func TestCollapsedTableOuterBorderTakesTheWinningCellColour(t *testing.T) {
	// Measured on Chromium 153.0.8010.12: a collapsed table's right/bottom frame
	// paints the edge cells' declared rgb(51,51,51), never the borderless table
	// box's own currentcolor. scripts/verify-border-currentcolor.js records the law.
	cell := frame.RGB(51, 51, 51)
	black := frame.RGB(0, 0, 0)
	cmds := ibBorders(ibDisplayList(t,
		`table{border-collapse:collapse;width:120px}td{border:1px solid rgb(51,51,51)}`,
		`<table><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></table>`, 320))

	var right, bottom *paint.DisplayCmd
	for i := range cmds {
		c := &cmds[i]
		if c.Border.Right.Width > 0 && right == nil {
			right = c
		}
		if c.Border.Bottom.Width > 0 && bottom == nil {
			bottom = c
		}
	}
	if right == nil || bottom == nil {
		t.Fatalf("collapsed table emitted no delegated right/bottom border command: %+v", cmds)
	}
	if right.Border.Right.Color != cell {
		t.Errorf("collapsed table delegated right border colour = %s, want the winning cell's %s; rgb(0,0,0) means paint read the borderless table box's currentcolor instead of the edge cell (the regression this test guards)", bccShow(right.Border.Right.Color), bccShow(cell))
	}
	if bottom.Border.Bottom.Color != cell {
		t.Errorf("collapsed table delegated bottom border colour = %s, want the winning cell's %s; rgb(0,0,0) = %s is the currentcolor regression", bccShow(bottom.Border.Bottom.Color), bccShow(cell), bccShow(black))
	}
}

// qc:BR-327
func TestLogicalBorderColourShorthandPaintsBothEndsOfItsAxis(t *testing.T) {
	blue := frame.RGB(0, 0, 255)
	green := frame.RGB(0, 128, 0)
	red := frame.RGB(255, 0, 0)
	six := [4]int32{6, 6, 6, 6}
	for _, tc := range []struct {
		rule string
		want bccSides
	}{
		{"border-width:6px;border-style:solid;border-block-color:rgb(255,0,0);color:rgb(0,128,0)", bccSides{top: red, right: green, bottom: red, left: green, widths: six}},
		{"border-width:6px;border-style:solid;border-block-color:rgb(255,0,0) rgb(0,0,255);color:rgb(0,128,0)", bccSides{top: red, right: green, bottom: blue, left: green, widths: six}},
		{"border-width:6px;border-style:solid;border-inline-color:rgb(0,0,255);color:rgb(0,128,0)", bccSides{top: green, right: blue, bottom: green, left: blue, widths: six}},
		{"border-width:6px;border-style:solid;border-inline-color:rgb(255,0,0) rgb(0,0,255);color:rgb(0,128,0)", bccSides{top: green, right: blue, bottom: green, left: red, widths: six}},
	} {
		bccCheck(t, tc.rule, tc.want)
	}
}
