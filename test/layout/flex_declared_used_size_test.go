package layout_test

import (
	"testing"
)

// qc:BR-318
func TestFlexDeclaredWidthItemShrinksToResolvedBasis(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; width: 300px;">
  <div style="width: 200px; white-space: nowrap;">AAA</div>
  <div style="width: 200px; white-space: nowrap;">BBB</div>
  <div style="width: 200px; white-space: nowrap;">CCC</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 4 {
		t.Fatalf("found %d divs, want 4 (container + 3 items)", len(divs))
	}
	wantX := []float32{0, 100, 200}
	for k := 0; k < 3; k++ {
		item := divs[1+k]
		if !almostEqual(item.W, 100) {
			t.Errorf("item %d W = %v, want 100 (Chromium shrinks each 200px basis to 300/3)", k, item.W)
		}
		if !almostEqual(item.X, wantX[k]) {
			t.Errorf("item %d X = %v, want %v", k, item.X, wantX[k])
		}
	}
}

// qc:BR-318
func TestFlexDeclaredWidthItemGrowsToResolvedBasis(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; width: 400px;">
  <div style="width: 100px; flex-grow: 1;">GROWS</div>
  <div style="width: 100px;">FIXED</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 3 {
		t.Fatalf("found %d divs, want 3 (container + 2 items)", len(divs))
	}
	if !almostEqual(divs[1].W, 300) {
		t.Errorf("growing item W = %v, want 300 (100 basis + 200 free space)", divs[1].W)
	}
	if !almostEqual(divs[2].X, 300) {
		t.Errorf("fixed item X = %v, want 300", divs[2].X)
	}
	if !almostEqual(divs[2].W, 100) {
		t.Errorf("fixed item W = %v, want 100", divs[2].W)
	}
}

// qc:BR-318
func TestFlexDeclaredWidthItemAutomaticMinimumFloor(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; width: 200px;">
  <div style="width: 150px; min-width: 80px;">MMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMM</div>
  <div style="width: 150px;">ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 3 {
		t.Fatalf("found %d divs, want 3 (container + 2 items)", len(divs))
	}
	if !almostEqual(divs[1].W, 80) {
		t.Errorf("explicit min-width item W = %v, want 80 (Chromium floors at the explicit min-width)", divs[1].W)
	}
	if !almostEqual(divs[2].X, 80) {
		t.Errorf("second item X = %v, want 80", divs[2].X)
	}
	if !almostEqual(divs[2].W, 150) {
		t.Errorf("auto min-width item W = %v, want 150 (Chromium keeps the declared size because its content min-width exceeds it)", divs[2].W)
	}
}

// qc:BR-318
func TestFlexDeclaredWidthItemStaysAtBasisWhenContentFloorIsHigher(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; width: 200px;">
  <div style="width: 150px;">WWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWW</div>
  <div style="width: 150px;">YYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYY</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 3 {
		t.Fatalf("found %d divs, want 3 (container + 2 items)", len(divs))
	}
	if !almostEqual(divs[1].W, 150) || !almostEqual(divs[1].X, 0) {
		t.Errorf("first item box = X %v W %v, want X 0 W 150 (Chromium does not shrink below the declared basis when the content floor is higher)", divs[1].X, divs[1].W)
	}
	if !almostEqual(divs[2].W, 150) || !almostEqual(divs[2].X, 150) {
		t.Errorf("second item box = X %v W %v, want X 150 W 150", divs[2].X, divs[2].W)
	}
}
