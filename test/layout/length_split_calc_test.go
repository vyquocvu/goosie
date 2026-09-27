package layout_test

import (
	"testing"
)

// The cascade test pins what `border-spacing: calc(10vh - 4px) 3px` resolves to; this
// pins that the grid moves. At a 900 px frame the axes are 86 and 3, so the first cell
// sits 86 px inside the table's content box and 3 px below its top edge. Splitting on
// every Unicode space handed the parser `calc(10vh` and `-`, both of which read as
// zero, and the whole gap collapsed.

const spacingDoc = `<html><body style="margin:0">
<table style="width:600px"><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></table>
</body></html>`

type cellBox struct{ x, y, w, h float32 }

// gridBoxes returns the four cells in document order. Only the first cell's position is
// asserted absolutely: the columns share whatever the gaps leave of the table's width,
// so a bigger gap is answered by narrower columns.
func gridBoxes(t *testing.T, sheet string) [4]cellBox {
	t.Helper()
	arena := layoutAtViewport(t, spacingDoc, sheet, 1440, 900)
	all := findAllByTag(arena, "td")
	if len(all) != 4 {
		t.Fatalf("found %d tds, want 4 for sheet %q", len(all), sheet)
	}
	var boxes [4]cellBox
	for i, o := range all {
		boxes[i] = cellBox{o.X, o.Y, o.W, o.H}
	}
	return boxes
}

func TestCalcBorderSpacingInAShorthandMovesTheGrid(t *testing.T) {
	boxes := gridBoxes(t, "table { border-spacing: calc(10vh - 4px) 3px; }")
	if !almostEqual(boxes[0].x, 86) {
		t.Errorf("first cell x = %v, want 86 (the shorthand's first argument)", boxes[0].x)
	}
	if !almostEqual(boxes[0].y, 3) {
		t.Errorf("first cell y = %v, want 3 (the shorthand's second argument)", boxes[0].y)
	}
	// The expression and the lengths it evaluates to have to place the whole grid the
	// same way, which is the claim: the function is read as one length per axis.
	if want := gridBoxes(t, "table { border-spacing: 86px 3px; }"); boxes != want {
		t.Errorf("calc grid = %+v, want the 86px grid %+v", boxes, want)
	}
}

// A single value sets both axes, and that has to keep working now that the shorthand
// is split by a function-aware reader rather than by whitespace.
func TestLoneCalcBorderSpacingSetsBothAxes(t *testing.T) {
	boxes := gridBoxes(t, "table { border-spacing: calc(10vh - 4px); }")
	if !almostEqual(boxes[0].x, 86) || !almostEqual(boxes[0].y, 86) {
		t.Errorf("first cell = %v/%v, want 86/86", boxes[0].x, boxes[0].y)
	}
	if want := gridBoxes(t, "table { border-spacing: 86px; }"); boxes != want {
		t.Errorf("calc grid = %+v, want the 86px grid %+v", boxes, want)
	}
}

// Each axis reading its own argument, and reading it as arithmetic rather than as a
// constant that happens to be right for one input: 1 px off in the expression is 1 px
// on the page.
func TestCalcBorderSpacingAxesTrackTheArithmetic(t *testing.T) {
	boxes := gridBoxes(t, "table { border-spacing: calc(10vh - 5px) calc(1vh + 1px); }")
	if !almostEqual(boxes[0].x, 85) {
		t.Errorf("first cell x = %v, want 85 (calc(10vh - 5px))", boxes[0].x)
	}
	if !almostEqual(boxes[0].y, 10) {
		t.Errorf("first cell y = %v, want 10 (calc(1vh + 1px) at a 900 px frame)", boxes[0].y)
	}
}
