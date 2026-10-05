package layout_test

import (
	"testing"
)

// TestFlexMeasuredItemHonorsMinWidth guards the auto-basis measure path: an
// item with no declared basis measures its max-content width, but min-width
// still floors it. Before the floor, 34px of text in a min-width: 60px item
// measured 34px of content and the whole row packed left of Chromium's row -
// which is what dragged the flexbox-complete parity fixture to 85.88%.
func TestFlexMeasuredItemHonorsMinWidth(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; width: 760px;">
  <div style="min-width: 60px; padding: 15px; margin: 3px;">start</div>
  <div style="min-width: 60px; padding: 15px; margin: 3px;">center</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 3 {
		t.Fatalf("found %d divs, want 3 (container + 2 items)", len(divs))
	}
	first, second := divs[1], divs[2]
	if !almostEqual(first.W, 60) {
		t.Errorf("first.W = %v, want 60 (min-width floors the text measure)", first.W)
	}
	// Second item starts after first's margin box plus its own margin-left.
	if !almostEqual(second.X, 99) {
		t.Errorf("second.X = %v, want 99 (3 + 60 content + 30 padding + 3 + 3 margins)", second.X)
	}
}

// TestFlexMeasuredItemHonorsMaxWidth guards the other side: max-width caps an
// auto-basis item whose text would measure wider.
func TestFlexMeasuredItemHonorsMaxWidth(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; width: 760px;">
  <div style="max-width: 40px; padding: 0px; margin: 0px;">a much longer line of text</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 2 {
		t.Fatalf("found %d divs, want 2 (container + 1 item)", len(divs))
	}
	if !almostEqual(divs[1].W, 40) {
		t.Errorf("item.W = %v, want 40 (max-width caps the text measure)", divs[1].W)
	}
}
