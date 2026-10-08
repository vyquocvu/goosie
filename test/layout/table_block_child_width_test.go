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

// TestTableEmptyFixedWidthCellHoldsColumnOpen pins the specified-width floor:
// an empty cell with `width: 200px` still holds its column at 200, so the
// auto-width table around it measures 200 instead of collapsing to zero and
// the row's shadow has a box to cast from.
func TestTableEmptyFixedWidthCellHoldsColumnOpen(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<table><tr><td style="width: 200px; height: 200px;"></td></tr></table>
</body></html>`, 800)

	tables := findAllByTag(arena, "table")
	if len(tables) != 1 {
		t.Fatalf("found %d tables, want 1", len(tables))
	}
	// 200 of column plus the default 2px separated spacing on each side.
	if !almostEqual(tables[0].W, 204) {
		t.Errorf("table.W = %v, want 204 (the empty fixed-width cell holds its column open)", tables[0].W)
	}
}

// TestEmptyTableKeepsPaddingBox pins that a column-less table still paints
// its padding and borders: W/H stay content-sized (zero) so BorderRect adds
// the padding exactly once, yielding a 312x312 border box for 155px padding
// and a 1px border instead of a zero-height collapse.
func TestEmptyTableKeepsPaddingBox(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: table; background: green; border: 1px solid black; padding: 155px;"></div>
</body></html>`, 800)

	tables := findAllByTag(arena, "div")
	if len(tables) != 1 {
		t.Fatalf("found %d tables, want 1", len(tables))
	}
	if tables[0].W != 0 || tables[0].H != 0 {
		t.Errorf("table content = %vx%v, want 0x0 (no columns)", tables[0].W, tables[0].H)
	}
	x0, y0, x1, y1 := tables[0].BorderRect()
	if x1-x0 != 312 || y1-y0 != 312 {
		t.Errorf("table border box = %vx%v, want 312x312", x1-x0, y1-y0)
	}
}

// TestAnonymousTableBoxesWrapStrayContent pins missing-structure generation:
// blocks directly under a table gain an anonymous row and cell, so an
// <html display:table> wrapping body content still grids instead of
// collapsing to zero.
func TestAnonymousTableBoxesWrapStrayContent(t *testing.T) {
	arena := session(t, `<html style="display: table; border: 10px solid green; border-spacing: 0; padding: 0; margin: auto;"><body style="padding: 0; margin: 0;"><div style="width:200px;height:300px;display:inline-block;"></div><div style="width:80px;height:300px;display:inline-block;"></div></body></html>`, 800)

	html := findAllByTag(arena, "html")
	if len(html) != 1 {
		t.Fatalf("found %d html elements, want 1", len(html))
	}
	// Content 280 plus the 20px border, centred in the 800 viewport.
	if !almostEqual(html[0].W, 280) {
		t.Errorf("table.W = %v, want 280 (both inline-blocks side by side)", html[0].W)
	}
	if !almostEqual(html[0].X, 250) {
		t.Errorf("table.X = %v, want 250 (margin auto centres)", html[0].X)
	}
}

// TestColDefinitionsSizeTheGrid pins <col> handling two ways: definitions
// floor their columns (three 50px cols hold 150 even with an empty cell) and
// extend the grid past the cells, so a colspan=4 cell in a 3-col grid still
// fits instead of outgrowing it. The void </col> close must not eject the
// table either (pinned in test/dom).
func TestColDefinitionsSizeTheGrid(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<table style="border-collapse: collapse; table-layout: fixed;"><col style="width: 50px;"><col style="width: 50px;"><col style="width: 50px;"><td colspan="1" style="height: 50px;"></td></table>
</body></html>`, 800)

	tables := findAllByTag(arena, "table")
	if len(tables) != 1 {
		t.Fatalf("found %d tables, want 1", len(tables))
	}
	if tables[0].W < 140 {
		t.Errorf("table.W = %v, want ~150 (three cols hold the grid open)", tables[0].W)
	}
}
