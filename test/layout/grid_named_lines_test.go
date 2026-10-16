package layout_test

import (
	"testing"
)

// TestGridNamedLinesPinItems guards custom-ident line placement: a "[name]"
// group in the track list names the line before the next track, and
// "start-name / end-name" resolves to those lines. Before line names existed
// each "[...]" group parsed as a phantom auto track - shifting every column
// after it - and every named placement fell back to a single auto-placed
// cell, which is what scattered the named-lines section of the grid-complete
// parity fixture (72.6% against Chromium).
func TestGridNamedLinesPinItems(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: grid; width: 760px; grid-template-columns: [sidebar-start] 200px [sidebar-end content-start] 1fr [content-end];">
  <div style="grid-column: sidebar-start / content-end; height: 40px;">H</div>
  <div style="grid-column: sidebar-start / sidebar-end; height: 300px;">S</div>
  <div style="grid-column: content-start / content-end; height: 300px;">C</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 4 {
		t.Fatalf("found %d divs, want 4 (container + 3 items)", len(divs))
	}
	header, side, content := divs[1], divs[2], divs[3]
	if !almostEqual(header.X, 0) || !almostEqual(header.W, 760) {
		t.Errorf("header = {X:%v W:%v}, want the full 760px first row", header.X, header.W)
	}
	if !almostEqual(side.X, 0) || !almostEqual(side.W, 200) {
		t.Errorf("side = {X:%v W:%v}, want the 200px sidebar column", side.X, side.W)
	}
	if !almostEqual(content.X, 200) || !almostEqual(content.W, 560) {
		t.Errorf("content = {X:%v W:%v}, want the 1fr column beside the sidebar", content.X, content.W)
	}
	if !almostEqual(side.Y, content.Y) {
		t.Errorf("side.Y = %v, content.Y = %v, want both on the second row", side.Y, content.Y)
	}
}

// TestGridNumericLinePlacementPinsItems guards the numbered form of the same
// mechanism: "1 / 3" starts at line 1 and spans two tracks instead of taking
// an auto-placed span of two. The explicit-placement parity section happened
// to score 99.6% through placement luck; this pins the behavior directly.
func TestGridNumericLinePlacementPinsItems(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: grid; width: 800px; grid-template-columns: repeat(4, 1fr);">
  <div style="grid-column: 3 / 5; height: 40px;">B</div>
  <div style="grid-column: 1 / 3; height: 40px;">A</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 3 {
		t.Fatalf("found %d divs, want 3 (container + 2 items)", len(divs))
	}
	// Source order is B then A, but line placement beats document order: A
	// takes columns 1-2 of row 0 and B takes columns 3-4.
	a, b := divs[2], divs[1]
	if !almostEqual(a.X, 0) || !almostEqual(a.W, 400) {
		t.Errorf("a = {X:%v W:%v}, want columns 1-2 (400px at X 0)", a.X, a.W)
	}
	if !almostEqual(b.X, 400) || !almostEqual(b.W, 400) {
		t.Errorf("b = {X:%v W:%v}, want columns 3-4 (400px at X 400)", b.X, b.W)
	}
	if !almostEqual(a.Y, b.Y) {
		t.Errorf("a.Y = %v, b.Y = %v, want both on the first row", a.Y, b.Y)
	}
}

// TestGridNamedRowLines guards the row axis: named row lines pin the item's
// row the same way column lines pin its column.
func TestGridNamedRowLines(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: grid; width: 400px; grid-template-rows: [top-start] 60px [top-end bottom-start] 1fr [bottom-end];">
  <div style="grid-row: bottom-start / bottom-end; height: 30px;">B</div>
  <div style="grid-row: top-start / top-end; height: 30px;">T</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 3 {
		t.Fatalf("found %d divs, want 3 (container + 2 items)", len(divs))
	}
	top, bottom := divs[2], divs[1]
	if !almostEqual(top.Y, 0) {
		t.Errorf("top.Y = %v, want 0 (first row)", top.Y)
	}
	if top.Y >= bottom.Y {
		t.Errorf("top.Y = %v, bottom.Y = %v, want top above bottom", top.Y, bottom.Y)
	}
}
