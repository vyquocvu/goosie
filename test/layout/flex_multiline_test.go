package layout_test

import (
	"testing"
)

// TestFlexWrapRow verifies that flex items wrap to new lines when they exceed
// the container width with flex-wrap: wrap.
func TestFlexWrapRow(t *testing.T) {
	// Three 200px items in a 300px container: each item gets its own line.
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-wrap: wrap; width: 300px; height: 300px;">
  <div style="width: 200px; height: 50px;"></div>
  <div style="width: 200px; height: 50px;"></div>
  <div style="width: 200px; height: 50px;"></div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 4 {
		t.Fatalf("found %d divs, want 4 (container + 3 items)", len(divs))
	}

	// Each item should be on its own line, stacked vertically.
	item1, item2, item3 := divs[1], divs[2], divs[3]
	if item2.Y <= item1.Y {
		t.Errorf("item2.Y = %v should be below item1.Y = %v", item2.Y, item1.Y)
	}
	if item3.Y <= item2.Y {
		t.Errorf("item3.Y = %v should be below item2.Y = %v", item3.Y, item2.Y)
	}
	// All items should start at x=0 (each is alone on its line).
	if !almostEqual(item1.X, 0) {
		t.Errorf("item1.X = %v, want 0", item1.X)
	}
	if !almostEqual(item2.X, 0) {
		t.Errorf("item2.X = %v, want 0", item2.X)
	}
	if !almostEqual(item3.X, 0) {
		t.Errorf("item3.X = %v, want 0", item3.X)
	}
}

// TestFlexWrapTwoPerLine verifies that items that exactly fit share a line,
// and the next item wraps.
func TestFlexWrapTwoPerLine(t *testing.T) {
	// 4 items of 150px in a 300px container: 2 per line (150+150=300 fits).
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-wrap: wrap; width: 300px; height: 300px;">
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 5 {
		t.Fatalf("found %d divs, want 5 (container + 4 items)", len(divs))
	}

	item1, item2, item3, item4 := divs[1], divs[2], divs[3], divs[4]
	// Items 1 and 2 should be on the same line.
	if !almostEqual(item1.Y, item2.Y) {
		t.Errorf("item1.Y = %v and item2.Y = %v should be on the same line", item1.Y, item2.Y)
	}
	// Items 3 and 4 should be on the same line, below items 1 and 2.
	if !almostEqual(item3.Y, item4.Y) {
		t.Errorf("item3.Y = %v and item4.Y = %v should be on the same line", item3.Y, item4.Y)
	}
	if item3.Y <= item1.Y {
		t.Errorf("line 2 (Y=%v) should be below line 1 (Y=%v)", item3.Y, item1.Y)
	}
	// Item 2 should be right after item 1.
	if !almostEqual(item2.X, 150) {
		t.Errorf("item2.X = %v, want 150", item2.X)
	}
	// Item 3 should start a new line at x=0.
	if !almostEqual(item3.X, 0) {
		t.Errorf("item3.X = %v, want 0 (start of new line)", item3.X)
	}
}

// TestFlexWrapReverseRow verifies that wrap-reverse reverses the line order
// in a row flex container.
func TestFlexWrapReverseRow(t *testing.T) {
	// 3 items of 200px in a 300px container: 1 item per line, 3 lines.
	// wrap-reverse should put line 3 (item 3) at the top.
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-wrap: wrap-reverse; width: 300px; height: 300px;">
  <div style="width: 200px; height: 40px;"></div>
  <div style="width: 200px; height: 40px;"></div>
  <div style="width: 200px; height: 40px;"></div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 4 {
		t.Fatalf("found %d divs, want 4", len(divs))
	}

	item1, item2, item3 := divs[1], divs[2], divs[3]
	// With wrap-reverse, item 3 (logical line 2) is at the top,
	// item 2 (logical line 1) is in the middle,
	// item 1 (logical line 0) is at the bottom.
	if item1.Y <= item2.Y {
		t.Errorf("wrap-reverse: item1.Y = %v should be below item2.Y = %v", item1.Y, item2.Y)
	}
	if item2.Y <= item3.Y {
		t.Errorf("wrap-reverse: item2.Y = %v should be below item3.Y = %v", item2.Y, item3.Y)
	}
}

// TestAlignContentCenterRow verifies that align-content: center centers
// flex lines vertically in a row container.
func TestAlignContentCenterRow(t *testing.T) {
	// 4 items of 150px in 300px container: 2 lines of 40px each.
	// Total content = 80px. Remaining = 220 - 80 = 140px (container is 300px, but
	// default align-content: stretch gives each line 150px, so we need explicit).
	// Actually with align-content: center, lines keep natural size.
	// remaining = 300 - 80 = 220. Lead = 110.
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-wrap: wrap; align-content: center; width: 300px; height: 300px;">
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 5 {
		t.Fatalf("found %d divs, want 5 (container + 4 items)", len(divs))
	}

	item1 := divs[1]
	item3 := divs[3]

	// remaining = 300 - 80 = 220, lead = 110
	// Line 1 at y=110, Line 2 at y=110+40=150
	if !almostEqual(item1.Y, 110) {
		t.Errorf("item1.Y = %v, want 110 (centered)", item1.Y)
	}
	if !almostEqual(item3.Y, 150) {
		t.Errorf("item3.Y = %v, want 150", item3.Y)
	}
}

// TestAlignContentFlexEndRow verifies that align-content: flex-end pushes
// lines to the bottom of the container.
func TestAlignContentFlexEndRow(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-wrap: wrap; align-content: flex-end; width: 300px; height: 300px;">
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 5 {
		t.Fatalf("found %d divs, want 5", len(divs))
	}

	item1 := divs[1]
	item3 := divs[3]

	// Total content = 80px, container = 300px, remaining = 220px
	// flex-end: line 1 starts at y=220, line 2 at y=260
	if !almostEqual(item1.Y, 220) {
		t.Errorf("item1.Y = %v, want 220 (flex-end)", item1.Y)
	}
	if !almostEqual(item3.Y, 260) {
		t.Errorf("item3.Y = %v, want 260", item3.Y)
	}
}

// TestAlignContentSpaceBetweenRow verifies that align-content: space-between
// distributes space evenly between lines.
func TestAlignContentSpaceBetweenRow(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-wrap: wrap; align-content: space-between; width: 300px; height: 300px;">
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 5 {
		t.Fatalf("found %d divs, want 5", len(divs))
	}

	item1 := divs[1]
	item3 := divs[3]

	// Total content = 80px, remaining = 220px, 2 lines => 220px between them
	// Line 1 at y=0, line 2 at y=40+220=260
	if !almostEqual(item1.Y, 0) {
		t.Errorf("item1.Y = %v, want 0 (space-between first line at start)", item1.Y)
	}
	if !almostEqual(item3.Y, 260) {
		t.Errorf("item3.Y = %v, want 260 (space-between second line)", item3.Y)
	}
}

// TestAlignContentStretchRow verifies that align-content: stretch (the default)
// stretches lines to fill the container when no other value is set.
func TestAlignContentStretchRow(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-wrap: wrap; width: 300px; height: 300px;">
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 5 {
		t.Fatalf("found %d divs, want 5", len(divs))
	}

	item1 := divs[1]
	item3 := divs[3]

	// Default align-content stretches lines: 300px / 2 = 150px per line.
	// Line 1 at y=0, line 2 at y=150.
	if !almostEqual(item1.Y, 0) {
		t.Errorf("item1.Y = %v, want 0", item1.Y)
	}
	if !almostEqual(item3.Y, 150) {
		t.Errorf("item3.Y = %v, want 150 (stretched line height)", item3.Y)
	}
}

// TestAlignContentStretchAutoHeight verifies that items with auto height
// stretch to fill the line's cross size under the default align-content.
func TestAlignContentStretchAutoHeight(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-wrap: wrap; width: 300px; height: 300px;">
  <div style="width: 150px;">line one</div>
  <div style="width: 150px;">line two</div>
  <div style="width: 150px;">line three</div>
  <div style="width: 150px;">line four</div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 5 {
		t.Fatalf("found %d divs, want 5", len(divs))
	}

	// Auto-height items should stretch to fill the line's cross size (150px).
	item1 := divs[1]
	if !almostEqual(item1.H, 150) {
		t.Errorf("item1.H = %v, want 150 (stretched to line height)", item1.H)
	}
}

// TestFlexWrapColumn verifies that column flex containers wrap items to new
// columns when they exceed the container height.
func TestFlexWrapColumn(t *testing.T) {
	// Three 100px-tall items in a 200px-tall container should wrap to 2 columns.
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-direction: column; flex-wrap: wrap; width: 400px; height: 200px;">
  <div style="width: 100px; height: 100px;"></div>
  <div style="width: 100px; height: 100px;"></div>
  <div style="width: 100px; height: 100px;"></div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 4 {
		t.Fatalf("found %d divs, want 4 (container + 3 items)", len(divs))
	}

	item1, item2, item3 := divs[1], divs[2], divs[3]
	// Items 1 and 2 should be in the first column (x=0).
	if !almostEqual(item1.X, 0) {
		t.Errorf("item1.X = %v, want 0", item1.X)
	}
	if !almostEqual(item2.X, 0) {
		t.Errorf("item2.X = %v, want 0 (same column)", item2.X)
	}
	// Item 3 should wrap to the second column.
	if item3.X <= item1.X {
		t.Errorf("item3.X = %v, want > %v (wrapped to second column)", item3.X, item1.X)
	}
	// Item 3 should be at the top of its column.
	if !almostEqual(item3.Y, 0) {
		t.Errorf("item3.Y = %v, want 0 (top of new column)", item3.Y)
	}
}

// TestFlexNoWrap verifies that flex-wrap: nowrap keeps all items on one line
// even when they overflow the container.
func TestFlexNoWrap(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-wrap: nowrap; width: 200px;">
  <div style="width: 100px; height: 50px; flex-shrink: 0;"></div>
  <div style="width: 100px; height: 50px; flex-shrink: 0;"></div>
  <div style="width: 100px; height: 50px; flex-shrink: 0;"></div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 4 {
		t.Fatalf("found %d divs, want 4", len(divs))
	}

	// All items should be on the same line (y=0).
	for i := 1; i < len(divs); i++ {
		if !almostEqual(divs[i].Y, 0) {
			t.Errorf("item%d.Y = %v, want 0 (nowrap keeps all on one line)", i, divs[i].Y)
		}
	}
}

// TestAlignContentSpaceAroundRow verifies that align-content: space-around
// distributes space evenly around lines.
func TestAlignContentSpaceAroundRow(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-wrap: wrap; align-content: space-around; width: 300px; height: 300px;">
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 5 {
		t.Fatalf("found %d divs, want 5", len(divs))
	}

	item1 := divs[1]
	item3 := divs[3]

	// Total content = 80px, remaining = 220px, 2 lines
	// space-around: per-line space = 220 / 2 = 110px
	// Line 1 lead = 55px, gap between lines = 110px
	// Line 1 at y=55, Line 2 at y=55+40+110=205
	if !almostEqual(item1.Y, 55) {
		t.Errorf("item1.Y = %v, want 55 (space-around)", item1.Y)
	}
	if !almostEqual(item3.Y, 205) {
		t.Errorf("item3.Y = %v, want 205 (space-around)", item3.Y)
	}
}

// TestAlignContentSpaceEvenlyRow verifies that align-content: space-evenly
// distributes space evenly between and around lines.
func TestAlignContentSpaceEvenlyRow(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-wrap: wrap; align-content: space-evenly; width: 300px; height: 300px;">
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
  <div style="width: 150px; height: 40px;"></div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 5 {
		t.Fatalf("found %d divs, want 5", len(divs))
	}

	item1 := divs[1]
	item3 := divs[3]

	// Total content = 80px, remaining = 220px, 2 lines => 3 gaps (220/3 ≈ 73.33px each)
	// Line 1 at y≈73.33, Line 2 at y≈73.33+40+73.33≈186.67
	if !almostEqual(item1.Y, 220.0/3.0) {
		t.Errorf("item1.Y = %v, want %.2f (space-evenly)", item1.Y, 220.0/3.0)
	}
	if !almostEqual(item3.Y, 220.0/3.0+40+220.0/3.0) {
		t.Errorf("item3.Y = %v, want %.2f (space-evenly)", item3.Y, 220.0/3.0+40+220.0/3.0)
	}
}

// TestFlexWrapWithGap verifies that gap is respected when wrapping to new lines.
func TestFlexWrapWithGap(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-wrap: wrap; gap: 10px; width: 210px; height: 200px;">
  <div style="width: 100px; height: 50px;"></div>
  <div style="width: 100px; height: 50px;"></div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 3 {
		t.Fatalf("found %d divs, want 3 (container + 2 items)", len(divs))
	}

	item1, item2 := divs[1], divs[2]
	// Items are 100px each with 10px gap = 210px total, fits on one line.
	if !almostEqual(item1.Y, item2.Y) {
		t.Errorf("items should be on same line: item1.Y=%v, item2.Y=%v", item1.Y, item2.Y)
	}
	if !almostEqual(item2.X, 110) {
		t.Errorf("item2.X = %v, want 110 (100 + 10px gap)", item2.X)
	}
}
