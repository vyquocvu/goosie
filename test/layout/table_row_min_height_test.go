package layout_test

import (
	"testing"
)

// TestTableRowDeclaredHeightIsAMinimum covers the empty rows a list uses to gap
// its items: Hacker News separates every story with
// `<tr class="spacer" style="height:5px">` and puts a `height:10px` row under its
// header bar. A row's own height is a floor on the row, whether or not it brings
// any cell to hold it up, so dropping it makes every story 5px too tight and
// slides the bottom of the page up by a story every four rows.
func TestTableRowDeclaredHeightIsAMinimum(t *testing.T) {
	t.Run("cell-less spacer row takes its height", func(t *testing.T) {
		arena := session(t, `<html><body style="margin: 0;">
<table>
<tr><td style="padding: 0;">one</td></tr>
<tr style="height: 20px;"></tr>
<tr><td style="padding: 0;">two</td></tr>
</table>
</body></html>`, 800)
		trs := findAllByTag(arena, "tr")
		if len(trs) != 3 {
			t.Fatalf("found %d rows, want 3", len(trs))
		}
		if !almostEqual(trs[1].H, 20) {
			t.Fatalf("spacer row H = %v, want 20", trs[1].H)
		}
		// The rows are separated by the table's own border-spacing, read off the
		// gap before the spacer rather than hardcoded: Chromium puts row three at
		// 44 with the spacer at 22 and 2px between rows.
		spacing := trs[1].Y - (trs[0].Y + trs[0].H)
		if !almostEqual(trs[2].Y, trs[1].Y+trs[1].H+spacing) {
			t.Errorf("row three Y = %v, want %v (the spacer must push it down)", trs[2].Y, trs[1].Y+trs[1].H+spacing)
		}
	})

	t.Run("declared height floors a row of shorter cells", func(t *testing.T) {
		arena := session(t, `<html><body style="margin: 0;">
<table>
<tr style="height: 60px;"><td style="padding: 0;">short</td></tr>
</table>
</body></html>`, 800)
		tr := findByTag(arena, "tr")
		if tr == nil {
			t.Fatal("missing row")
		}
		if !almostEqual(tr.H, 60) {
			t.Fatalf("row H = %v, want 60", tr.H)
		}
		td := findByTag(arena, "td")
		if td == nil {
			t.Fatal("missing cell")
		}
		if !almostEqual(td.H, 60) {
			t.Errorf("cell H = %v, want 60 (the cell fills the row it is in)", td.H)
		}
	})

	t.Run("a taller cell still wins", func(t *testing.T) {
		arena := session(t, `<html><body style="margin: 0;">
<table>
<tr style="height: 5px;"><td style="padding: 0; height: 40px;">tall</td></tr>
</table>
</body></html>`, 800)
		tr := findByTag(arena, "tr")
		if tr == nil {
			t.Fatal("missing row")
		}
		if !(tr.H > 30) {
			t.Fatalf("row H = %v, want the cell's 40px to hold the row open", tr.H)
		}
	})
}
