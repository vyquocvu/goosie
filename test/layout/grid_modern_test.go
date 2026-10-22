package layout_test

import (
	"testing"
)

// TestGridSpanPlacements verifies line / span N, span N / line, and bare span N.
func TestGridSpanPlacements(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: grid; width: 600px; grid-template-columns: repeat(3, 200px);">
  <div id="itemA" style="grid-column: 1 / span 2; height: 30px;">A</div>
  <div id="itemB" style="grid-column: span 2 / 4; height: 30px;">B</div>
  <div id="itemC" style="grid-column-start: 1; grid-column-end: span 3; height: 30px;">C</div>
</div>
</body></html>`, 800)

	itemA := findByID(arena, "itemA")
	if itemA == nil {
		t.Fatal("itemA not found")
	}
	if !almostEqual(itemA.X, 0) || !almostEqual(itemA.W, 400) {
		t.Errorf("itemA = {X:%v, W:%v}, want {0, 400}", itemA.X, itemA.W)
	}

	itemB := findByID(arena, "itemB")
	if itemB == nil {
		t.Fatal("itemB not found")
	}
	// span 2 / line 4 ends at line 4 (3rd column right edge = 600), starts at line 2 (200), width = 400
	if !almostEqual(itemB.X, 200) || !almostEqual(itemB.W, 400) {
		t.Errorf("itemB = {X:%v, W:%v}, want {200, 400}", itemB.X, itemB.W)
	}

	itemC := findByID(arena, "itemC")
	if itemC == nil {
		t.Fatal("itemC not found")
	}
	// 1 / span 3 spans all 3 columns: X=0, W=600
	if !almostEqual(itemC.X, 0) || !almostEqual(itemC.W, 600) {
		t.Errorf("itemC = {X:%v, W:%v}, want {0, 600}", itemC.X, itemC.W)
	}
}

// TestGridNegativeLinePlacement verifies that -1 refers to the end of the explicit grid.
func TestGridNegativeLinePlacement(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: grid; width: 800px; grid-template-columns: repeat(4, 200px);">
  <div id="full" style="grid-column: 1 / -1; height: 40px;">Full Width</div>
  <div id="sub" style="grid-column: 2 / -1; height: 40px;">Sub Width</div>
</div>
</body></html>`, 800)

	full := findByID(arena, "full")
	if full == nil {
		t.Fatal("full not found")
	}
	if !almostEqual(full.X, 0) || !almostEqual(full.W, 800) {
		t.Errorf("full = {X:%v, W:%v}, want {0, 800}", full.X, full.W)
	}

	sub := findByID(arena, "sub")
	if sub == nil {
		t.Fatal("sub not found")
	}
	if !almostEqual(sub.X, 200) || !almostEqual(sub.W, 600) {
		t.Errorf("sub = {X:%v, W:%v}, want {200, 600}", sub.X, sub.W)
	}
}

// TestGridFitContent verifies fit-content(length) track sizing.
func TestGridFitContent(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: grid; width: 600px; grid-template-columns: fit-content(150px) 1fr;">
  <div id="small" style="width: 80px; height: 30px;">Small</div>
  <div>Rest</div>
</div>
</body></html>`, 800)

	small := findByID(arena, "small")
	if small == nil {
		t.Fatal("small not found")
	}
	if !almostEqual(small.W, 80) {
		t.Errorf("small.W = %v, want 80", small.W)
	}
}

// TestGridWithLogicalProperties verifies that inline-size, block-size, and logical margins/paddings work in grid.
func TestGridWithLogicalProperties(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: grid; inline-size: 600px; block-size: 200px; grid-template-columns: 1fr 1fr;">
  <div id="boxA" style="margin-inline: 10px 20px; padding-block: 5px;">A</div>
  <div id="boxB" style="inline-size: 100px; block-size: 50px;">B</div>
</div>
</body></html>`, 800)

	boxA := findByID(arena, "boxA")
	if boxA == nil {
		t.Fatal("boxA not found")
	}
	// Column 0 is width 300, boxA has margin-left: 10, so X = 10
	if !almostEqual(boxA.X, 10) {
		t.Errorf("boxA.X = %v, want 10", boxA.X)
	}

	boxB := findByID(arena, "boxB")
	if boxB == nil {
		t.Fatal("boxB not found")
	}
	if !almostEqual(boxB.W, 100) || !almostEqual(boxB.H, 50) {
		t.Errorf("boxB = {W:%v, H:%v}, want {100, 50}", boxB.W, boxB.H)
	}
}
