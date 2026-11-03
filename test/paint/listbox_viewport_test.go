package paint_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// The visual half of GOOSIE-LISTBOX-001: the rows a user can see are the rows
// the engine hit-tests (test/engine/listbox_viewport_test.go pins that rule and
// quotes scripts/verify-br101-listbox-viewport.js), and what the display list
// draws must agree with them. Before this slice the builder was handed the whole
// option list tiled below the control, so a select near the viewport bottom drew
// rows the compositor had no pixels for, and a long list drew rows past the fold
// while hiding the one the user had selected.
//
// These oracles read the panel fill and the row runs back: a flipped listbox
// paints above its control, a windowed one paints exactly the rows the engine
// reports, and no listbox command lands outside the viewport.

func listboxPaintFixture(top float32, n, selected int) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<html><head><style>html,body{margin:0}</style></head><body>`+
		`<select id="sel" style="position:absolute;left:40px;top:%gpx;width:180px;font-size:16px">`, top)
	for i := 0; i < n; i++ {
		mark := ""
		if i == selected {
			mark = " selected"
		}
		fmt.Fprintf(&b, `<option value="v%d"%s>opt%d</option>`, i, mark, i)
	}
	b.WriteString(`</select></body></html>`)
	return b.String()
}

// textRunsAbove returns every glyph run painted at or above y, in display-list
// order, so a flipped panel's rows are not mistaken for the closed control's own
// label.
func textRunsAbove(cmds []paint.DisplayCmd, y float32) []paint.DisplayCmd {
	var out []paint.DisplayCmd
	for _, cmd := range cmds {
		if cmd.Kind != paint.CmdText || len(cmd.Text.Glyphs) == 0 {
			continue
		}
		if float32(cmd.Text.Glyphs[0].Y) <= y {
			out = append(out, cmd)
		}
	}
	return out
}

func panelFill(t *testing.T, cmds []paint.DisplayCmd) frame.Rect {
	t.Helper()
	panels := fillsOfColor(cmds, paint.ListboxPanelColor)
	if len(panels) != 1 {
		t.Fatalf("the open listbox painted %v panel fills, want exactly 1", panels)
	}
	return panels[0]
}

// controlRect is the box the closed control occupies, in document CSS px.
func controlRect(t *testing.T, s *engine.Session, id string) frame.RectF {
	t.Helper()
	for i := range s.Arena.Objects {
		obj := &s.Arena.Objects[i]
		if obj.Node != nil && obj.Node.GetAttribute("id") == id {
			x0, y0, x1, y1 := obj.BorderRect()
			return frame.RectF4(x0, y0, x1, y1)
		}
	}
	t.Fatalf("element %q not found in the arena", id)
	return frame.RectF{}
}

// A listbox that flipped upward must draw its rows above the control, inside the
// viewport, and in option order.
//
// qc:BR-101
func TestFlippedListboxPaintsItsRowsAboveTheControl(t *testing.T) {
	s, rows := openListbox(t, listboxPaintFixture(540, 12, 3))
	cmds := popupCommands(t, s)
	box := controlRect(t, s, "sel")
	panel := panelFill(t, cmds)
	if float32(panel.Y1) > box.Y0 {
		t.Errorf("the panel spans y [%d,%d], past the control's top edge %v: it painted where it cannot show", panel.Y0, panel.Y1, box.Y0)
	}
	if len(rows) != 12 {
		t.Fatalf("the engine reports %d rows for a list that fits above", len(rows))
	}
	if last := rows[len(rows)-1].Rect.ToDevice(1.0); last.Y1 > panel.Y1 || last.Y0 < panel.Y0 {
		t.Errorf("the last row %v is outside the panel %v", last, panel)
	}
	above := textRunsAbove(cmds, float32(rows[len(rows)-1].Rect.Y1-1))
	got := runStrings(above)
	if len(got) != 12 {
		t.Fatalf("the flipped listbox drew %v above the control, want one run per option", got)
	}
	if got[0] != "opt0" || got[len(got)-1] != "opt11" {
		t.Errorf("the flipped rows read %v..%v, want opt0..opt11 in option order", got[0], got[len(got)-1])
	}
}

// A list longer than the panel must not draw the rows it is not showing: the
// display list carries exactly the window, and the selected row's highlight sits
// on the row the engine marked.
//
// qc:BR-101
func TestWindowedListboxPaintsOnlyTheRowsItShows(t *testing.T) {
	s, rows := openListbox(t, listboxPaintFixture(200, 40, 33))
	cmds := popupCommands(t, s)
	panel := panelFill(t, cmds)
	below := textRunsBelow(cmds, float32(rows[0].Rect.ToDevice(1.0).Y0)-1)
	got := runStrings(below)
	if len(got) != len(rows) {
		t.Fatalf("the listbox drew %d row runs, want the %d rows the engine reports (not all 40 options): %v",
			len(got), len(rows), got)
	}
	for i, r := range rows {
		if want := r.Label; got[i] != want {
			t.Errorf("row %d drew %q, want %q", i, got[i], want)
		}
	}
	if float32(panel.Y1) > 600 {
		t.Errorf("the panel ends at y %d, past the 600px viewport: it painted rows off screen", panel.Y1)
	}
	highlights := fillsOfColor(cmds, paint.ListboxHighlightColor)
	if len(highlights) != 1 {
		t.Fatalf("the windowed listbox painted %v highlights, want exactly the selected row", highlights)
	}
	sel := -1
	for i, r := range rows {
		if r.Selected {
			sel = i
		}
	}
	if sel < 0 {
		t.Fatalf("the window %v..%v hides the selected option", rows[0].Index, rows[len(rows)-1].Index)
	}
	if want := rows[sel].Rect.ToDevice(1.0); highlights[0] != want {
		t.Errorf("the highlight is at %v, want the selected row %v", highlights[0], want)
	}
}

// The panel and its rows always land inside the viewport, whichever side they
// open on and however much of the list they show. This is the invariant the
// defect was written for, checked on the display list rather than the model.
//
// qc:BR-101
func TestListboxPaintsInsideTheViewportOnEverySide(t *testing.T) {
	for _, tc := range []struct {
		top      float32
		options  int
		selected int
	}{
		{60, 12, 3}, {200, 40, 33}, {300, 40, 5}, {500, 12, 3}, {540, 40, 39},
	} {
		s, rows := openListbox(t, listboxPaintFixture(tc.top, tc.options, tc.selected))
		panel := panelFill(t, popupCommands(t, s))
		if panel.Y0 < 0 || panel.Y1 > 600 {
			t.Errorf("top %v, %d options: the panel spans y [%d,%d], outside the 600px viewport",
				tc.top, tc.options, panel.Y0, panel.Y1)
		}
		for i, r := range rows {
			d := r.Rect.ToDevice(1.0)
			if d.Y0 < panel.Y0 || d.Y1 > panel.Y1 {
				t.Errorf("top %v, %d options: row %d %v is outside the panel %v", tc.top, tc.options, i, d, panel)
			}
		}
	}
}
