package layout_test

import (
	"testing"
)

func TestGridNamedAreasMatchCaseSensitively(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: grid; width: 720px; grid-template-columns: 200px 1fr; grid-template-areas: 'topARE topARE' 'sideARE mainARE';">
  <div style="grid-area: topARE; height: 40px;">T</div>
  <div style="grid-area: sideARE; height: 300px;">S</div>
  <div style="grid-area: mainARE; height: 300px;">M</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 4 {
		t.Fatalf("found %d divs, want 4 (container + 3 items)", len(divs))
	}
	top, side, main := divs[1], divs[2], divs[3]
	if !almostEqual(top.W, 720) || !almostEqual(top.X, 0) {
		t.Errorf("top = {X:%v W:%v}, want the full 720px first row", top.X, top.W)
	}
	if !almostEqual(side.X, 0) || !almostEqual(side.W, 200) || !almostEqual(side.Y, 40) {
		t.Errorf("side = {X:%v W:%v Y:%v}, want the 200px column below the banner", side.X, side.W, side.Y)
	}
	if !almostEqual(main.X, 200) || !almostEqual(main.W, 520) || !almostEqual(main.Y, 40) {
		t.Errorf("main = {X:%v W:%v Y:%v}, want the 1fr column beside the sidebar", main.X, main.W, main.Y)
	}
}
