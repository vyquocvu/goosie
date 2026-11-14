package layout_test

import (
	"testing"
)

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

func TestTableEmptyFixedWidthCellHoldsColumnOpen(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<table><tr><td style="width: 200px; height: 200px;"></td></tr></table>
</body></html>`, 800)

	tables := findAllByTag(arena, "table")
	if len(tables) != 1 {
		t.Fatalf("found %d tables, want 1", len(tables))
	}
	if !almostEqual(tables[0].W, 204) {
		t.Errorf("table.W = %v, want 204 (the empty fixed-width cell holds its column open)", tables[0].W)
	}
}

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

func TestAnonymousTableBoxesWrapStrayContent(t *testing.T) {
	arena := session(t, `<html style="display: table; border: 10px solid green; border-spacing: 0; padding: 0; margin: auto;"><body style="padding: 0; margin: 0;"><div style="width:200px;height:300px;display:inline-block;"></div><div style="width:80px;height:300px;display:inline-block;"></div></body></html>`, 800)

	html := findAllByTag(arena, "html")
	if len(html) != 1 {
		t.Fatalf("found %d html elements, want 1", len(html))
	}
	if !almostEqual(html[0].W, 280) {
		t.Errorf("table.W = %v, want 280 (both inline-blocks side by side)", html[0].W)
	}
	if !almostEqual(html[0].X, 250) {
		t.Errorf("table.X = %v, want 250 (margin auto centres)", html[0].X)
	}
}

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
