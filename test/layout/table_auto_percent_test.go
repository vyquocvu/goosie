package layout_test

import "testing"

func TestAutoTableResolvesPercentCellsAgainstTableWidth(t *testing.T) {
	arena := session(t, `<html><body style="margin:0"><center><form><table cellpadding="0" cellspacing="0"><tr><td width="25%">&nbsp;</td><td align="center"><div style="width:506px;height:34px"></div><br><span style="display:inline-block;width:100px;height:10px"></span></td><td width="25%">&nbsp;</td></tr></table></form></center></body></html>`, 1280)

	tables := findAllByTag(arena, "table")
	cells := findAllByTag(arena, "td")
	if len(tables) != 1 || len(cells) != 3 {
		t.Fatalf("found %d tables and %d cells, want 1 and 3", len(tables), len(cells))
	}
	table := tables[0]
	if !almostEqual(table.W, 1012) || !almostEqual(table.X, 134) {
		t.Errorf("table = {X:%v W:%v}, want {X:134 W:1012}", table.X, table.W)
	}
	if !almostEqual(cells[0].W, 253) || !almostEqual(cells[1].W, 506) || !almostEqual(cells[2].W, 253) {
		t.Errorf("cell widths = %v/%v/%v, want 253/506/253", cells[0].W, cells[1].W, cells[2].W)
	}
}
