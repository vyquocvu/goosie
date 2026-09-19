package layout_test

import (
	"testing"
)

// TestTableCellSizesToBlockChildWidth guards the intrinsic width of a cell whose
// content is a block rather than words. A block child brings its declared width,
// its own box extras and its horizontal margins to the measure; reading only text
// left the cell at zero, and Hacker News' `.votearrow{width:10px;margin:3px 2px
// 6px}` column collapsed so every title on the page slid 18px left of where
// Chromium puts it. Chromium measures the same boxes at 16 and 202 wide: `margin:
// 3px 2px 6px` is 10 of content plus 4 of horizontal margin, and a cell adds its
// own 1px of padding on each side. The table cannot go below the 218 those
// columns need, however narrow it asks to be.
func TestTableCellSizesToBlockChildWidth(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<table style="width: 214px; border-collapse: collapse;">
  <tr>
    <td><div style="width: 10px; margin: 3px 2px 6px;"></div></td>
    <td><div style="width: 200px;"></div></td>
  </tr>
</table>
</body></html>`, 800)

	tds := findAllByTag(arena, "td")
	if len(tds) != 2 {
		t.Fatalf("found %d tds, want 2", len(tds))
	}
	if !almostEqual(tds[0].W, 14) {
		t.Errorf("arrow td.W = %v, want 14 (the block child's width plus its margins)", tds[0].W)
	}
	if !almostEqual(tds[1].X, 16) {
		t.Errorf("title td.X = %v, want 16 (the arrow column and its cell padding have to take room)", tds[1].X)
	}
	if !almostEqual(tds[1].W, 200) {
		t.Errorf("title td.W = %v, want 200", tds[1].W)
	}
}

// TestTableCellHonoursChildMinMaxWidth keeps a child's min- and max-width in the
// column's floor and ceiling: a cell holding a `min-width: 60px` box is 60 wide
// with no text to measure, and one holding a 200px box capped at 40px is 40.
// Neither box can shrink, so the 100px table Chromium is given ends up 104 wide.
func TestTableCellHonoursChildMinMaxWidth(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<table style="width: 100px; border-collapse: collapse;">
  <tr>
    <td><div style="min-width: 60px;"></div></td>
    <td><div style="width: 200px; max-width: 40px;"></div></td>
  </tr>
</table>
</body></html>`, 800)

	tds := findAllByTag(arena, "td")
	if len(tds) != 2 {
		t.Fatalf("found %d tds, want 2", len(tds))
	}
	if !almostEqual(tds[0].W, 60) {
		t.Errorf("min-width td.W = %v, want 60", tds[0].W)
	}
	if !almostEqual(tds[1].W, 40) {
		t.Errorf("max-width td.W = %v, want 40", tds[1].W)
	}
}
