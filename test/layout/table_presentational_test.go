package layout_test

import (
	"testing"

	"sort"

	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

func rightOf(o *layout.Object) float32 { return o.X + o.W }

func words(arena *layout.Arena) []*layout.Object {
	var out []*layout.Object
	for i := range arena.Objects {
		obj := &arena.Objects[i]
		if obj.Node != nil && obj.Node.Data == "" && obj.Style != nil && obj.Style.Display == style.DisplayInline && obj.W > 0 && obj.H > 0 {
			out = append(out, obj)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].X < out[j].X })
	return out
}

func TestCenteredTableTakesItsPresentationalWidth(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;"><center>
<table width="85%"><tr><td>one</td></tr></table>
</center></body></html>`, 800)
	tbl := findByTag(arena, "table")
	if tbl == nil {
		t.Fatal("no table")
	}
	if !almostEqual(tbl.W, 680) {
		t.Errorf("table.W = %v, want 680 (85%% of 800)", tbl.W)
	}
	if !almostEqual(tbl.X, 60) {
		t.Errorf("table.X = %v, want 60 (centred by <center>)", tbl.X)
	}
}

func TestCenterAlignsTableCellText(t *testing.T) {
	t.Run("inherited centring applies to table cells", func(t *testing.T) {
		arena := session(t, `<html><body style="margin: 0;"><center>
<table width="100%"><tr><td width="100%">left edge text</td></tr></table>
</center></body></html>`, 800)
		td := findByTag(arena, "td")
		ws := words(arena)
		if td == nil || len(ws) == 0 {
			t.Fatal("no cell or no words")
		}
		cellCenter := td.X + td.W/2
		textCenter := ws[0].X + ws[0].W/2
		if diff := textCenter - cellCenter; diff > 50 || diff < -50 {
			t.Errorf("text center %v is not near cell center %v (diff %v): <center> should center table cell text", textCenter, cellCenter, diff)
		}
	})

	t.Run("an explicit alignment on the cell still applies", func(t *testing.T) {
		arena := session(t, `<html><body style="margin: 0;"><center>
<table width="100%"><tr><td width="100%" style="text-align: right">right edge text</td></tr></table>
</center></body></html>`, 800)
		td := findByTag(arena, "td")
		ws := words(arena)
		if td == nil || len(ws) == 0 {
			t.Fatal("no cell or no words")
		}
		if right := rightOf(ws[len(ws)-1]); right < td.X+td.W-10 {
			t.Errorf("cell text ends at %v, cell right edge at %v: text-align: right did not reach the cell", right, td.X+td.W)
		}
	})
}

func TestCellPresentationalAttributes(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<table width="100%" cellpadding="10" cellspacing="0"><tr>
<td width="50%" align="right">right aligned</td>
<td width="50%">left aligned</td>
</tr></table></body></html>`, 800)
	tds := findAllByTag(arena, "td")
	if len(tds) != 2 {
		t.Fatalf("found %d cells, want 2", len(tds))
	}
	for i, td := range tds {
		if !almostEqual(td.PaddingLeft, 10) || !almostEqual(td.PaddingRight, 10) {
			t.Errorf("cell %d padding = %v/%v, want 10/10 from cellpadding", i, td.PaddingLeft, td.PaddingRight)
		}
	}
	var left, right []*layout.Object
	for _, w := range words(arena) {
		if w.X < tds[1].X {
			left = append(left, w)
		} else {
			right = append(right, w)
		}
	}
	if len(left) == 0 || len(right) == 0 {
		t.Fatalf("words did not split between the cells: %d/%d", len(left), len(right))
	}
	if end, want := rightOf(left[len(left)-1]), tds[0].X+tds[0].W+tds[0].PaddingRight; end < want-1 || end > want+1 {
		t.Errorf(`align="right" text ends at %v, want the cell's right padding edge %v`, end, want)
	}
	if start, want := right[0].X, tds[1].X+tds[1].PaddingLeft; start < want-1 || start > want+1 {
		t.Errorf("plain cell text starts at %v, want its left padding edge %v", start, want)
	}
}
