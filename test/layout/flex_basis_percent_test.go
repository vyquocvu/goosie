package layout_test

import (
	"testing"
)

func TestFlexBasisPercentage(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex;">
  <div style="flex: 0 0 75%;">MAIN</div>
  <div style="flex: 0 0 25%;">SIDE</div>
</div>
</body></html>`, 1140)

	boxes := findAllByTag(arena, "div")
	if len(boxes) != 3 {
		t.Fatalf("found %d divs, want 3 (row + main + side)", len(boxes))
	}
	main, side := boxes[1], boxes[2]
	if !almostEqual(main.W, 855) {
		t.Errorf("main.W = %v, want 855 (75%% of the 1140 content box)", main.W)
	}
	if !almostEqual(main.X, 0) {
		t.Errorf("main.X = %v, want 0", main.X)
	}
	if !almostEqual(side.W, 285) {
		t.Errorf("side.W = %v, want 285 (25%% of the 1140 content box)", side.W)
	}
	if !almostEqual(side.X, 855) {
		t.Errorf("side.X = %v, want 855 (right after the 75%% basis)", side.X)
	}
}
