package layout_test

import (
	"testing"
)

// TestFlexGrowStopsAtMaxWidth guards CSS Flexbox §9.7: growing an item stops at
// its max-width, and the space it could not take is offered to the other
// growers. The distribution used to add the whole share onto the item's basis
// with no clamp, while the box it painted had been clamped - so the cursor that
// advances by the basis walked every later sibling past its own edge. Yesterweb's
// `nav{width:370px;max-width:300px;flex-grow:1}` shifted its article column 30px
// right at 1280 and 190px right at 1440, because the error is the free space the
// frozen item refused to take.
func TestFlexGrowStopsAtMaxWidth(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex;">
  <div style="width: 370px; max-width: 300px; flex-grow: 1;">NAVLIST</div>
  <div style="width: 950px;">MANIFESTO</div>
</div>
</body></html>`, 1280)

	boxes := findAllByTag(arena, "div")
	if len(boxes) != 3 {
		t.Fatalf("found %d divs, want 3 (row + nav + main)", len(boxes))
	}
	nav, main := boxes[1], boxes[2]
	if !almostEqual(nav.W, 300) {
		t.Errorf("nav.W = %v, want 300 (its max-width)", nav.W)
	}
	// The 30px the nav refused stays with it: main starts at the nav's edge, not
	// at the 330px the unclamped basis consumed.
	if !almostEqual(main.X, 300) {
		t.Errorf("main.X = %v, want 300", main.X)
	}
	if !almostEqual(main.W, 950) {
		t.Errorf("main.W = %v, want 950", main.W)
	}
}

// TestFlexShrinkSkipsItemClampedByMaxWidth guards the other half of §9.7: an
// item whose base size exceeded its max-width is frozen at the clamped size
// before any shortfall is shared out. Yesterweb's sidebar (`width:370px;
// max-width:300px`) sat next to an article column wider than the viewport, and
// the shrink pass took the overflow out of both, leaving the nav a 54px sliver
// while its box still painted 300 wide - so the article column slid under it
// and covered every nav label but its first letter.
func TestFlexShrinkSkipsItemClampedByMaxWidth(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex;">
  <div style="width: 370px; max-width: 300px; flex-grow: 1;">NAVLIST</div>
  <div>word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word word</div>
</div>
</body></html>`, 1280)

	boxes := findAllByTag(arena, "div")
	if len(boxes) != 3 {
		t.Fatalf("found %d divs, want 3 (row + nav + main)", len(boxes))
	}
	nav, main := boxes[1], boxes[2]
	if !almostEqual(nav.W, 300) {
		t.Errorf("nav.W = %v, want 300 (frozen at its max-width, not shrunk)", nav.W)
	}
	if !almostEqual(main.X, 300) {
		t.Errorf("main.X = %v, want 300 (the nav keeps its clamped size, so the cursor advances by it)", main.X)
	}
	// The whole shortfall is the unclamped sibling's to pay.
	if !almostEqual(main.W, 980) {
		t.Errorf("main.W = %v, want 980", main.W)
	}
}
