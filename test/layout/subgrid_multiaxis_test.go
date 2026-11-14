package layout_test

import (
	"testing"
)

func TestSubgridMultiAxisInheritance(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div id="parent" style="display: grid; width: 600px; grid-template-columns: 200px 400px; grid-template-rows: 80px 120px;">
  <div id="subgrid" style="grid-column: 1 / span 2; grid-row: 1 / span 2; display: grid; grid-template-columns: subgrid; grid-template-rows: subgrid;">
    <div id="cell1" style="grid-column: 1; grid-row: 1;">Cell 1</div>
    <div id="cell2" style="grid-column: 2; grid-row: 2;">Cell 2</div>
  </div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) < 4 {
		t.Fatalf("expected at least 4 divs, got %d", len(divs))
	}

	subgrid := divs[1]
	cell1 := divs[2]
	cell2 := divs[3]

	if !almostEqual(subgrid.X, 0) || !almostEqual(subgrid.W, 600) {
		t.Errorf("subgrid X/W = {%v, %v}, want {0, 600}", subgrid.X, subgrid.W)
	}

	if !almostEqual(cell1.X, 0) || !almostEqual(cell1.W, 200) {
		t.Errorf("cell1 X/W = {%v, %v}, want {0, 200}", cell1.X, cell1.W)
	}
	if !almostEqual(cell1.Y, 0) || !almostEqual(cell1.H, 80) {
		t.Errorf("cell1 Y/H = {%v, %v}, want {0, 80}", cell1.Y, cell1.H)
	}

	if !almostEqual(cell2.X, 200) || !almostEqual(cell2.W, 400) {
		t.Errorf("cell2 X/W = {%v, %v}, want {200, 400}", cell2.X, cell2.W)
	}
	if !almostEqual(cell2.Y, 80) || !almostEqual(cell2.H, 120) {
		t.Errorf("cell2 Y/H = {%v, %v}, want {80, 120}", cell2.Y, cell2.H)
	}
}
