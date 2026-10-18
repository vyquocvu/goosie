package layout_test

import (
	"testing"
)

func TestContentVisibilityHiddenLayoutSizing(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div id="box" style="content-visibility: hidden; contain-intrinsic-size: 300px 150px; width: auto; height: auto;">
  <div style="width: 1000px; height: 1000px;">Huge Children Should Be Skipped</div>
</div>
</body></html>`, 800)

	box := findByTag(arena, "div")
	if box == nil {
		t.Fatal("box not found")
	}
	// Because content-visibility is hidden and width/height are auto,
	// box sizes to the contain-intrinsic-size (300 x 150), ignoring the huge 1000x1000 child.
	if !almostEqual(box.W, 300) || !almostEqual(box.H, 150) {
		t.Errorf("box size = (%v, %v), want (300, 150)", box.W, box.H)
	}
}

func TestCSSAnchorPositioning(t *testing.T) {
	arena := positionedSession(t, `<html><body style="margin: 0;">
<div id="btn" style="anchor-name: --my-btn; width: 120px; height: 40px; margin-top: 50px; margin-left: 100px;">Button</div>
<div id="tooltip" style="position: absolute; position-anchor: --my-btn; top: anchor(bottom); left: anchor(left); width: anchor-size(width); height: 25px;">Tooltip</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) < 2 {
		t.Fatalf("expected at least 2 divs, got %d", len(divs))
	}
	btn := divs[0]
	tooltip := divs[1]

	// Button position: X=100, Y=50, W=120, H=40
	if !almostEqual(btn.X, 100) || !almostEqual(btn.Y, 50) || !almostEqual(btn.W, 120) || !almostEqual(btn.H, 40) {
		t.Errorf("btn = {X:%v Y:%v W:%v H:%v}, want {100, 50, 120, 40}", btn.X, btn.Y, btn.W, btn.H)
	}

	// Tooltip: top: anchor(bottom) -> 50 + 40 = 90
	//          left: anchor(left) -> 100
	//          width: anchor-size(width) -> 120
	//          height: 25
	if !almostEqual(tooltip.X, 100) || !almostEqual(tooltip.Y, 90) || !almostEqual(tooltip.W, 120) || !almostEqual(tooltip.H, 25) {
		t.Errorf("tooltip = {X:%v Y:%v W:%v H:%v}, want {100, 90, 120, 25}", tooltip.X, tooltip.Y, tooltip.W, tooltip.H)
	}
}

func TestCSSAnchorPositioningExplicitAnchorAndCenter(t *testing.T) {
	arena := positionedSession(t, `<html><body style="margin: 0;">
<div style="anchor-name: --boxA; width: 100px; height: 100px; margin-top: 100px; margin-left: 200px;">BoxA</div>
<div style="position: absolute; top: anchor(--boxA center); left: anchor(--boxA right); width: 50px; height: 30px;">Popped</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) < 2 {
		t.Fatalf("expected at least 2 divs, got %d", len(divs))
	}
	popped := divs[1]

	// BoxA: X=200, Y=100, W=100, H=100
	// Popped: top: anchor(center) -> 100 + 100/2 = 150
	//         left: anchor(right) -> 200 + 100 = 300
	if !almostEqual(popped.X, 300) || !almostEqual(popped.Y, 150) {
		t.Errorf("popped = {X:%v Y:%v}, want {300, 150}", popped.X, popped.Y)
	}
}
