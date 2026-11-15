package layout_test

import (
	"testing"
)

// qc:BR-320
func TestFlexExplicitMinWidthZeroShrinksBelowBasis(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; width: 200px;">
  <div style="width: 150px; min-width: 0; white-space: nowrap;">WWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWW</div>
  <div style="width: 150px; min-width: 0; white-space: nowrap;">WWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWW</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 3 {
		t.Fatalf("found %d divs, want 3 (row + 2 items)", len(divs))
	}
	if !almostEqual(divs[1].W, 100) || !almostEqual(divs[1].X, 0) {
		t.Errorf("item0 box = X %v W %v, want X 0 W 100 (Chromium: explicit min-width:0 lets the declared-width item shrink to 100)", divs[1].X, divs[1].W)
	}
	if !almostEqual(divs[2].W, 100) || !almostEqual(divs[2].X, 100) {
		t.Errorf("item1 box = X %v W %v, want X 100 W 100", divs[2].X, divs[2].W)
	}
}

// qc:BR-320
func TestFlexAutoMinWidthHoldsDeclaredBasis(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; width: 200px;">
  <div style="width: 150px; min-width: auto; white-space: nowrap;">WWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWW</div>
  <div style="width: 150px; min-width: auto; white-space: nowrap;">WWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWW</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 3 {
		t.Fatalf("found %d divs, want 3 (row + 2 items)", len(divs))
	}
	if !almostEqual(divs[1].W, 150) || !almostEqual(divs[2].W, 150) {
		t.Errorf("auto items W = %v/%v, want 150/150 (min-width:auto floors at the declared basis, not 0)", divs[1].W, divs[2].W)
	}
}

// qc:BR-320
func TestFlexMixedMinWidthZeroAndAuto(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; width: 200px;">
  <div style="width: 150px; min-width: 0; white-space: nowrap;">WWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWW</div>
  <div style="width: 150px; min-width: auto; white-space: nowrap;">WWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWW</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 3 {
		t.Fatalf("found %d divs, want 3 (row + 2 items)", len(divs))
	}
	if !almostEqual(divs[1].W, 50) || !almostEqual(divs[1].X, 0) {
		t.Errorf("min-width:0 item box = X %v W %v, want X 0 W 50 (Chromium: auto sibling frozen at 150 pushes all shrink onto the explicit-0 item)", divs[1].X, divs[1].W)
	}
	if !almostEqual(divs[2].W, 150) || !almostEqual(divs[2].X, 50) {
		t.Errorf("min-width:auto item box = X %v W %v, want X 50 W 150", divs[2].X, divs[2].W)
	}
}
