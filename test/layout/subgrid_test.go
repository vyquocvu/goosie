package layout_test

import (
	"testing"
)

// TestSubgridColumnTrackInheritance verifies that an item with grid-template-columns: subgrid
// inherits track slices from its parent grid container and positions its children accordingly.
func TestSubgridColumnTrackInheritance(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div id="parent" style="display: grid; width: 600px; grid-template-columns: 100px 200px 300px;">
  <div id="item1" style="height: 50px;">Item 1</div>
  <div id="subgrid" style="grid-column: 2 / span 2; display: grid; grid-template-columns: subgrid;">
    <div id="sub1" style="height: 40px;">Sub 1</div>
    <div id="sub2" style="height: 40px;">Sub 2</div>
  </div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	// divs: parent (0), item1 (1), subgrid (2), sub1 (3), sub2 (4)
	if len(divs) < 5 {
		t.Fatalf("expected at least 5 divs, got %d", len(divs))
	}

	item1 := divs[1]
	subgrid := divs[2]
	sub1 := divs[3]
	sub2 := divs[4]

	if !almostEqual(item1.X, 0) || !almostEqual(item1.W, 100) {
		t.Errorf("item1 = {X:%v W:%v}, want {X:0 W:100}", item1.X, item1.W)
	}

	// Subgrid spans col 2 and 3 (widths 200px and 300px) -> X = 100, W = 500
	if !almostEqual(subgrid.X, 100) || !almostEqual(subgrid.W, 500) {
		t.Errorf("subgrid = {X:%v W:%v}, want {X:100 W:500}", subgrid.X, subgrid.W)
	}

	// sub1 takes the first subgrid track (parent col 2: 200px wide, starting at X=100)
	if !almostEqual(sub1.X, 100) || !almostEqual(sub1.W, 200) {
		t.Errorf("sub1 = {X:%v W:%v}, want {X:100 W:200}", sub1.X, sub1.W)
	}

	// sub2 takes the second subgrid track (parent col 3: 300px wide, starting at X=300)
	if !almostEqual(sub2.X, 300) || !almostEqual(sub2.W, 300) {
		t.Errorf("sub2 = {X:%v W:%v}, want {X:300 W:300}", sub2.X, sub2.W)
	}
}

// TestSubgridGapInheritance verifies that subgrid inherits parent column-gap when unspecified.
func TestSubgridGapInheritance(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: grid; width: 620px; grid-template-columns: 100px 200px 300px; column-gap: 10px;">
  <div style="height: 50px;">A</div>
  <div style="grid-column: 2 / span 2; display: grid; grid-template-columns: subgrid;">
    <div style="height: 40px;">B1</div>
    <div style="height: 40px;">B2</div>
  </div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) < 5 {
		t.Fatalf("expected at least 5 divs, got %d", len(divs))
	}
	subgrid := divs[2]
	b1 := divs[3]
	b2 := divs[4]

	// Parent tracks: 100px, gap 10px, 200px, gap 10px, 300px
	// Track 0 starts at 0, ends at 100
	// Track 1 starts at 110, ends at 310
	// Track 2 starts at 320, ends at 620
	// Subgrid spans track 1 and 2: starts at 110, width = 200 + 10 + 300 = 510
	if !almostEqual(subgrid.X, 110) || !almostEqual(subgrid.W, 510) {
		t.Errorf("subgrid = {X:%v W:%v}, want {X:110 W:510}", subgrid.X, subgrid.W)
	}

	// b1 takes track 1: starts at 110, width 200
	if !almostEqual(b1.X, 110) || !almostEqual(b1.W, 200) {
		t.Errorf("b1 = {X:%v W:%v}, want {X:110 W:200}", b1.X, b1.W)
	}

	// b2 takes track 2: starts at 110 + 200 + 10 = 320, width 300
	if !almostEqual(b2.X, 320) || !almostEqual(b2.W, 300) {
		t.Errorf("b2 = {X:%v W:%v}, want {X:320 W:300}", b2.X, b2.W)
	}
}

// TestSubgridNamedLines verifies line names defined in subgrid [<name>].
func TestSubgridNamedLines(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: grid; width: 600px; grid-template-columns: 100px 200px 300px;">
  <div style="grid-column: 1 / span 3; display: grid; grid-template-columns: subgrid [head-start] [mid-start] [tail-start] [tail-end];">
    <div style="grid-column: mid-start / tail-end; height: 30px;">Middle to End</div>
  </div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) < 3 {
		t.Fatalf("expected at least 3 divs, got %d", len(divs))
	}
	mid := divs[2]
	// mid spans mid-start (line 2, track 1: 200px) through tail-end (line 4, track 2: 300px)
	// X = 100px, W = 500px
	if !almostEqual(mid.X, 100) || !almostEqual(mid.W, 500) {
		t.Errorf("mid = {X:%v W:%v}, want {X:100 W:500}", mid.X, mid.W)
	}
}
