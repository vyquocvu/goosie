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

// TestFlexColumnDeclaredBasisWinsOverMeasure guards column main-axis sizing:
// a declared flex-basis (or height) is written to the item even when content
// would measure taller or shorter. Before, the place pass advanced the cursor
// by the basis but left the measured height on the box.
func TestFlexColumnDeclaredBasisWinsOverMeasure(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-direction: column; height: 200px;">
  <div style="width: 20px; flex: 0 10px;">a</div>
  <div style="width: 20px; flex: 0 50px;">b</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 3 {
		t.Fatalf("found %d divs, want 3 (container + 2 items)", len(divs))
	}
	if !almostEqual(divs[1].H, 10) {
		t.Errorf("first.H = %v, want 10 (flex-basis)", divs[1].H)
	}
	if !almostEqual(divs[2].H, 50) {
		t.Errorf("second.H = %v, want 50 (flex-basis)", divs[2].H)
	}
	if !almostEqual(divs[2].Y, 10) {
		t.Errorf("second.Y = %v, want 10 (stacked after the 10px basis)", divs[2].Y)
	}
}

// TestFlexEmptyFixedAtomicMeasuresItsBox guards max-content measurement of an
// empty fixed-size atomic: a 15px spacer with no words still occupies 15px.
// Before, it measured zero and the probe-width fallback upstream committed a
// 10000px item into the finished row.
func TestFlexEmptyFixedAtomicMeasuresItsBox(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; width: 200px;">
  <div style="flex: 1 0 30px;">a</div>
  <div style="flex: none;"><div style="display: inline-block; width: 15px; height: 15px;"></div></div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) < 3 {
		t.Fatalf("found %d divs, want at least 3", len(divs))
	}
	// divs[0] is the container; divs[1] the growing item; divs[2] the spacer holder.
	if divs[2].W > 200 {
		t.Errorf("spacer holder W = %v, want the 15px spacer (probe width leaked)", divs[2].W)
	}
	if !almostEqual(divs[2].W, 15) {
		t.Errorf("spacer holder W = %v, want 15", divs[2].W)
	}
}

// TestFlexAlignStartShrinkWrapsLikeFlexStart guards logical alignment in
// fit-content measurement: `align-self: start` shrink-wraps exactly like
// flex-start. Before, only the physical keywords measured, so a start-aligned
// item kept the full line width.
func TestFlexAlignStartShrinkWrapsLikeFlexStart(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-direction: column;">
  <div style="align-self: start;">
    <div style="width: 100px; height: 50px;"></div>
    <div style="padding-bottom: 50%;"></div>
  </div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) < 2 {
		t.Fatalf("found %d divs, want at least 2", len(divs))
	}
	// divs[0] is the container; divs[1] the item, shrink-wrapped to 100px.
	if !almostEqual(divs[1].W, 100) {
		t.Errorf("item.W = %v, want 100 (shrink-wrapped, not the 800 line)", divs[1].W)
	}
	if !almostEqual(divs[1].H, 100) {
		t.Errorf("item.H = %v, want 100 (50 content + 50%% padding of 100)", divs[1].H)
	}
}

// TestFlexColumnWrapBreaksLines guards multi-line column flex: items wrap
// past the container height into lines that pack across, each line laying
// its items bottom-up for column-reverse. Before, every item piled into one
// overflowing line.
func TestFlexColumnWrapBreaksLines(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-direction: column-reverse; flex-wrap: wrap; height: 300px; width: 300px;">
  <div style="height: 90px;">a</div>
  <div style="height: 90px;">b</div>
  <div style="height: 90px;">c</div>
  <div style="height: 140px;">d</div>
  <div style="height: 140px;">e</div>
  <div style="height: 290px;">f</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 7 {
		t.Fatalf("found %d divs, want 7 (container + 6 items)", len(divs))
	}
	// Three lines of 100px across: f alone left, d/e middle, a/b/c right.
	if !almostEqual(divs[6].X, 0) || !almostEqual(divs[6].W, 100) {
		t.Errorf("f = {X:%v W:%v}, want the 100px first line", divs[6].X, divs[6].W)
	}
	xs := map[float32]bool{}
	for _, d := range divs[1:] {
		xs[d.X] = true
	}
	if len(xs) != 3 {
		t.Errorf("items span %d distinct columns, want 3", len(xs))
	}
	// Column-reverse stacks bottom-up: a (first in DOM) sits lowest.
	var aY, cY float32
	for i, d := range divs[1:] {
		if i == 0 {
			aY = d.Y
		}
		if i == 2 {
			cY = d.Y
		}
	}
	if !(aY > cY) {
		t.Errorf("a.Y = %v, c.Y = %v, want a below c (column-reverse)", aY, cY)
	}
}
