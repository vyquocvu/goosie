package layout_test

import (
	"testing"
)

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
	if !almostEqual(main.X, 300) {
		t.Errorf("main.X = %v, want 300", main.X)
	}
	if !almostEqual(main.W, 950) {
		t.Errorf("main.W = %v, want 950", main.W)
	}
}

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
	if !almostEqual(main.W, 980) {
		t.Errorf("main.W = %v, want 980", main.W)
	}
}
