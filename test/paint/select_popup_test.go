package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// BR-101's paint half: when a pointer has opened a <select>, the rows drawn on
// screen must be the rows the engine lets the pointer hit. The engine computes
// the listbox once (test/engine/select_popup_test.go pins its geometry against
// scripts/verify-br101-select-popup.js) and hands the builder the panel rect,
// one rect per option, each option's label, and which row carries the selection
// and which are disabled. These oracles read the display list back and check it
// drew exactly that, so a row cannot be visible where the pointer lands on
// another one.

const popupHTML = `<html><head><style>` +
	`html,body{margin:0}` +
	`#sel{position:absolute;left:40px;top:60px;width:180px;font-size:16px}` +
	`</style></head><body>` +
	`<select id="sel" name="pick">` +
	`<option id="a" value="a">Alpha</option>` +
	`<option id="b" value="b" selected>Bravo</option>` +
	`<option id="c" value="c" disabled>Charlie</option>` +
	`</select></body></html>`

func openListbox(t *testing.T, html string) (*engine.Session, []paint.ListBoxRow) {
	t.Helper()
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	for i := range s.Arena.Objects {
		obj := &s.Arena.Objects[i]
		if obj.Node == nil || obj.Node.GetAttribute("id") != "sel" {
			continue
		}
		x0, y0, x1, y1 := obj.BorderRect()
		if !s.ClickAt((x0+x1)/2, (y0+y1)/2) {
			t.Fatal("a press on the closed select did not open its listbox")
		}
		rows := s.SelectPopupRows()
		if len(rows) == 0 {
			t.Fatal("the listbox opened with no rows")
		}
		return s, rows
	}
	t.Fatal("select#sel not found in the arena")
	return nil, nil
}

func popupCommands(t *testing.T, s *engine.Session) []paint.DisplayCmd {
	t.Helper()
	dl, err := s.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	return dl.Build(1).All()
}

func fillsOfColor(cmds []paint.DisplayCmd, c frame.Color) []frame.Rect {
	var out []frame.Rect
	for _, cmd := range cmds {
		if cmd.Kind == paint.CmdFill && cmd.Color == c {
			out = append(out, cmd.Rect)
		}
	}
	return out
}

// textRunsBelow returns every glyph run whose baseline sits at or below y, in
// display-list order, so the closed control's own label is not mistaken for a
// listbox row.
func textRunsBelow(cmds []paint.DisplayCmd, y float32) []paint.DisplayCmd {
	var out []paint.DisplayCmd
	for _, cmd := range cmds {
		if cmd.Kind != paint.CmdText || len(cmd.Text.Glyphs) == 0 {
			continue
		}
		if float32(cmd.Text.Glyphs[0].Y) >= y {
			out = append(out, cmd)
		}
	}
	return out
}

func runString(cmd paint.DisplayCmd) string {
	s := ""
	for _, g := range cmd.Text.Glyphs {
		s += string(g.Rune)
	}
	return s
}

// qc:BR-101
func TestClosedSelectPaintsNoListbox(t *testing.T) {
	s, err := engine.NewSession(popupHTML, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Close()
	cmds := popupCommands(t, s)
	if got := fillsOfColor(cmds, paint.ListboxPanelColor); len(got) != 0 {
		t.Errorf("a closed select painted %v listbox panels, want none", got)
	}
	if got := fillsOfColor(cmds, paint.ListboxHighlightColor); len(got) != 0 {
		t.Errorf("a closed select painted %v selection highlights, want none", got)
	}
}

// qc:BR-101
func TestOpenListboxPaintsOneRowPerOptionInOrder(t *testing.T) {
	s, rows := openListbox(t, popupHTML)
	cmds := popupCommands(t, s)
	if got := len(fillsOfColor(cmds, paint.ListboxPanelColor)); got != 1 {
		t.Errorf("the open listbox painted %d panel fills, want exactly 1", got)
	}
	wantLabels := []string{"Alpha", "Bravo", "Charlie"}
	below := textRunsBelow(cmds, rows[0].Rect.Y0-1)
	if len(below) != len(wantLabels) {
		t.Fatalf("the listbox drew %d glyph runs below the control (%v), want one per option %v",
			len(below), runStrings(below), wantLabels)
	}
	for i, want := range wantLabels {
		if got := runString(below[i]); got != want {
			t.Errorf("row %d drew %q, want %q", i, got, want)
		}
		wantRect := rows[i].Rect.ToDevice(1.0)
		got := below[i].Rect
		if got.Y0 != wantRect.Y0 || got.Y1 != wantRect.Y1 {
			t.Errorf("row %d text rect %v does not sit in the engine's row %v", i, got, wantRect)
		}
	}
	// Rows paint last, so nothing in the document can cover them.
	lastRow := -1
	for i, cmd := range cmds {
		if cmd.Kind == paint.CmdText && len(cmd.Text.Glyphs) > 0 && float32(cmd.Text.Glyphs[0].Y) >= rows[0].Rect.Y0-1 {
			lastRow = i
		}
	}
	if lastRow < len(cmds)-len(wantLabels) {
		t.Errorf("the row runs start at %d, too early to be the top of the display list", lastRow)
	}
}

func runStrings(cmds []paint.DisplayCmd) []string {
	var out []string
	for _, c := range cmds {
		out = append(out, runString(c))
	}
	return out
}

// qc:BR-101
func TestOpenListboxHighlightsExactlyTheSelectedRow(t *testing.T) {
	s, rows := openListbox(t, popupHTML)
	cmds := popupCommands(t, s)
	highlights := fillsOfColor(cmds, paint.ListboxHighlightColor)
	if len(highlights) != 1 {
		t.Fatalf("the open listbox painted %v highlights, want exactly the selected row", highlights)
	}
	// Option b is the second option and the selected one.
	want := rows[1].Rect.ToDevice(1.0)
	if highlights[0] != want {
		t.Errorf("highlight = %v, want the selected row %v", highlights[0], want)
	}
	below := textRunsBelow(cmds, rows[0].Rect.Y0-1)
	if len(below) != 3 {
		t.Fatalf("want three row runs, got %v", runStrings(below))
	}
	if below[1].Text.Color != paint.ListboxSelectedTextColor {
		t.Errorf("the selected row drew its label in %v, want %v", below[1].Text.Color, paint.ListboxSelectedTextColor)
	}
	if below[0].Text.Color != frame.RGB(0, 0, 0) {
		t.Errorf("an enabled unselected row drew in %v, want black", below[0].Text.Color)
	}
	// A disabled option is shown but greyed: the pointer cannot pick it
	// (test/engine/select_popup_test.go), so it must read as inert.
	if below[2].Text.Color != paint.ListboxDisabledTextColor {
		t.Errorf("the disabled row drew in %v, want %v", below[2].Text.Color, paint.ListboxDisabledTextColor)
	}
}

// The panel must cover every row it is showing, or a row would be drawn outside
// its own background.
//
// qc:BR-101
func TestOpenListboxPanelCoversEveryRow(t *testing.T) {
	s, rows := openListbox(t, popupHTML)
	cmds := popupCommands(t, s)
	panels := fillsOfColor(cmds, paint.ListboxPanelColor)
	if len(panels) != 1 {
		t.Fatalf("panel fills = %v, want one", panels)
	}
	panel := panels[0]
	for i, r := range rows {
		d := r.Rect.ToDevice(1.0)
		if d.X0 < panel.X0 || d.X1 > panel.X1 || d.Y0 < panel.Y0 || d.Y1 > panel.Y1 {
			t.Errorf("row %d %v sticks out of the panel %v", i, d, panel)
		}
	}
	if panel.Y1-panel.Y0 <= 0 || panel.X1-panel.X0 <= 0 {
		t.Errorf("the panel is empty: %v", panel)
	}
}
