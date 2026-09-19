package layout_test

import (
	"testing"
)

// TestFlexBasisPercentage guards CSS Flexbox §7.2.3: a percentage `flex-basis`
// resolves against the container's content box, and it is the item's basis
// before any growing or shrinking. A percentage basis arrives from the style
// layer as a sentinel rather than a length, so the flex row read it as `auto`
// and measured the content instead - which is how Bootstrap's grid, whose
// columns are `flex: 0 0 75%` plus `max-width: 75%`, collapsed its main column
// from 855px to the width of its own text on xresch.com.
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
