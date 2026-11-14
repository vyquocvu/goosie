package layout_test

import (
	"testing"
)

func TestFlexWrapRow(t *testing.T) {
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

	item1, item2, item3 := divs[1], divs[2], divs[3]
	if item2.Y <= item1.Y {
		t.Errorf("item2.Y = %v should be below item1.Y = %v", item2.Y, item1.Y)
	}
	if item3.Y <= item2.Y {
		t.Errorf("item3.Y = %v should be below item2.Y = %v", item3.Y, item2.Y)
	}
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

func TestFlexWrapTwoPerLine(t *testing.T) {
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
	if !almostEqual(item1.Y, item2.Y) {
		t.Errorf("item1.Y = %v and item2.Y = %v should be on the same line", item1.Y, item2.Y)
	}
	if !almostEqual(item3.Y, item4.Y) {
		t.Errorf("item3.Y = %v and item4.Y = %v should be on the same line", item3.Y, item4.Y)
	}
	if item3.Y <= item1.Y {
		t.Errorf("line 2 (Y=%v) should be below line 1 (Y=%v)", item3.Y, item1.Y)
	}
	if !almostEqual(item2.X, 150) {
		t.Errorf("item2.X = %v, want 150", item2.X)
	}
	if !almostEqual(item3.X, 0) {
		t.Errorf("item3.X = %v, want 0 (start of new line)", item3.X)
	}
}

func TestFlexWrapReverseRow(t *testing.T) {
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
	if item1.Y <= item2.Y {
		t.Errorf("wrap-reverse: item1.Y = %v should be below item2.Y = %v", item1.Y, item2.Y)
	}
	if item2.Y <= item3.Y {
		t.Errorf("wrap-reverse: item2.Y = %v should be below item3.Y = %v", item2.Y, item3.Y)
	}
}

func TestAlignContentCenterRow(t *testing.T) {
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

	if !almostEqual(item1.Y, 110) {
		t.Errorf("item1.Y = %v, want 110 (centered)", item1.Y)
	}
	if !almostEqual(item3.Y, 150) {
		t.Errorf("item3.Y = %v, want 150", item3.Y)
	}
}

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

	if !almostEqual(item1.Y, 220) {
		t.Errorf("item1.Y = %v, want 220 (flex-end)", item1.Y)
	}
	if !almostEqual(item3.Y, 260) {
		t.Errorf("item3.Y = %v, want 260", item3.Y)
	}
}

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

	if !almostEqual(item1.Y, 0) {
		t.Errorf("item1.Y = %v, want 0 (space-between first line at start)", item1.Y)
	}
	if !almostEqual(item3.Y, 260) {
		t.Errorf("item3.Y = %v, want 260 (space-between second line)", item3.Y)
	}
}

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

	if !almostEqual(item1.Y, 0) {
		t.Errorf("item1.Y = %v, want 0", item1.Y)
	}
	if !almostEqual(item3.Y, 150) {
		t.Errorf("item3.Y = %v, want 150 (stretched line height)", item3.Y)
	}
}

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

	item1 := divs[1]
	if !almostEqual(item1.H, 150) {
		t.Errorf("item1.H = %v, want 150 (stretched to line height)", item1.H)
	}
}

func TestFlexWrapColumn(t *testing.T) {
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
	if !almostEqual(item1.X, 0) {
		t.Errorf("item1.X = %v, want 0", item1.X)
	}
	if !almostEqual(item2.X, 0) {
		t.Errorf("item2.X = %v, want 0 (same column)", item2.X)
	}
	if item3.X <= item1.X {
		t.Errorf("item3.X = %v, want > %v (wrapped to second column)", item3.X, item1.X)
	}
	if !almostEqual(item3.Y, 0) {
		t.Errorf("item3.Y = %v, want 0 (top of new column)", item3.Y)
	}
}

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

	for i := 1; i < len(divs); i++ {
		if !almostEqual(divs[i].Y, 0) {
			t.Errorf("item%d.Y = %v, want 0 (nowrap keeps all on one line)", i, divs[i].Y)
		}
	}
}

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

	if !almostEqual(item1.Y, 55) {
		t.Errorf("item1.Y = %v, want 55 (space-around)", item1.Y)
	}
	if !almostEqual(item3.Y, 205) {
		t.Errorf("item3.Y = %v, want 205 (space-around)", item3.Y)
	}
}

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

	if !almostEqual(item1.Y, 220.0/3.0) {
		t.Errorf("item1.Y = %v, want %.2f (space-evenly)", item1.Y, 220.0/3.0)
	}
	if !almostEqual(item3.Y, 220.0/3.0+40+220.0/3.0) {
		t.Errorf("item3.Y = %v, want %.2f (space-evenly)", item3.Y, 220.0/3.0+40+220.0/3.0)
	}
}

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
	if !almostEqual(item1.Y, item2.Y) {
		t.Errorf("items should be on same line: item1.Y=%v, item2.Y=%v", item1.Y, item2.Y)
	}
	if !almostEqual(item2.X, 110) {
		t.Errorf("item2.X = %v, want 110 (100 + 10px gap)", item2.X)
	}
}
