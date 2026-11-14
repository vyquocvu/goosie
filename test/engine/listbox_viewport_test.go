package engine_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/paint"
)

func listboxFixture(top float32, n, selected int) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<html><head><style>html,body{margin:0}</style></head><body>`+
		`<select id="sel" name="pick" style="position:absolute;left:40px;top:%gpx;width:180px;font-size:16px">`, top)
	for i := 0; i < n; i++ {
		mark := ""
		if i == selected {
			mark = " selected"
		}
		fmt.Fprintf(&b, `<option id="o%d" value="v%d"%s>opt%d</option>`, i, i, mark, i)
	}
	fmt.Fprintf(&b, `</select><div style="position:absolute;left:0;top:%gpx;width:1px;height:1px"></div></body></html>`, 3000+top)
	return b.String()
}

func newListViewport(t *testing.T, html string) *engine.Session {
	t.Helper()
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600), engine.WithJS(rt))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func openRows(t *testing.T, s *engine.Session) []paint.ListBoxRow {
	t.Helper()
	x, y := clickPoint(t, s, "sel")
	if !s.ClickAt(x, y) {
		t.Fatal("a press on a closed <select> did not open its listbox")
	}
	rows := s.SelectPopupRows()
	if len(rows) == 0 {
		t.Fatal("the listbox opened with no rows")
	}
	return rows
}

func assertOnScreen(t *testing.T, rows []paint.ListBoxRow, scrollY float32) {
	t.Helper()
	for i, r := range rows {
		if r.Rect.Y0 < scrollY || r.Rect.Y1 > scrollY+600 {
			t.Errorf("row %d (%s) spans y [%v,%v], outside the viewport [%v,%v]",
				i, r.Label, r.Rect.Y0, r.Rect.Y1, scrollY, scrollY+600)
		}
	}
}

// qc:BR-101
func TestListboxOpensBelowWhileTheWholeListFits(t *testing.T) {
	s := newListViewport(t, listboxFixture(60, 12, 3))
	rows := openRows(t, s)
	box := boxRect(t, s, "sel")
	if len(rows) != 12 {
		t.Fatalf("the listbox shows %d rows for 12 options in a 600px viewport with room below", len(rows))
	}
	rowH := rows[0].Rect.Y1 - rows[0].Rect.Y0
	if rows[0].Rect.Y0 < box.Y1 {
		t.Errorf("the first row starts at %v, above the control's bottom edge %v: a listbox that fits below opens below", rows[0].Rect.Y0, box.Y1)
	}
	if rows[len(rows)-1].Rect.Y1 > 600 {
		t.Errorf("the last row ends at %v, past the viewport bottom", rows[len(rows)-1].Rect.Y1)
	}
	assertOnScreen(t, rows, 0)
	if rowH <= 0 {
		t.Errorf("row height %v, want a positive row", rowH)
	}
	for i, r := range rows {
		if r.Index != i {
			t.Errorf("row %d carries option %d, want %d", i, r.Index, i)
		}
	}
}

// qc:BR-101
func TestListboxFlipsAboveTheControlWhenThePanelWouldOverflowTheViewport(t *testing.T) {
	s := newListViewport(t, listboxFixture(540, 12, 3))
	rows := openRows(t, s)
	box := boxRect(t, s, "sel")
	if len(rows) != 12 {
		t.Fatalf("the flipped listbox shows %d rows, want all 12 (they fit above)", len(rows))
	}
	for i, r := range rows {
		if r.Rect.Y1 > box.Y0 {
			t.Errorf("row %d (%s) ends at %v, below the control's top edge %v: the panel opened downward off screen",
				i, r.Label, r.Rect.Y1, box.Y0)
			break
		}
	}
	assertOnScreen(t, rows, 0)
	if rows[0].Rect.Y0 >= rows[1].Rect.Y0 {
		t.Errorf("rows go upward in the wrong order: %v then %v", rows[0].Rect, rows[1].Rect)
	}
}

// qc:BR-101
func TestListboxWindowsItsRowsToTheViewportAroundTheSelection(t *testing.T) {
	const sel = 33
	s := newListViewport(t, listboxFixture(200, 40, sel))
	rows := openRows(t, s)
	assertOnScreen(t, rows, 0)
	if len(rows) >= 40 {
		t.Fatalf("the listbox shows all %d rows of a 40-option list that cannot fit; want a window", len(rows))
	}
	rowH := rows[0].Rect.Y1 - rows[0].Rect.Y0
	box := boxRect(t, s, "sel")

	visible := float32(len(rows))*rowH + 2
	if visible+rowH <= 600-box.Y1 {
		t.Errorf("the window is %v tall for %v px of room below the control: a row still fits", visible, 600-box.Y1)
	}

	for i, r := range rows {
		if r.Index != rows[0].Index+i {
			t.Fatalf("row %d carries option %d, want %d: the window must be contiguous", i, r.Index, rows[0].Index+i)
		}
		if want := fmt.Sprintf("opt%d", r.Index); r.Label != want {
			t.Errorf("row %d is labelled %q, want %q", i, r.Label, want)
		}
	}
	var selRow = -1
	for i, r := range rows {
		if r.Selected {
			selRow = i
		}
	}
	if selRow < 0 {
		t.Fatalf("the window %v..%v hides the selected option %d", rows[0].Index, rows[len(rows)-1].Index, sel)
	}
	if got := rows[selRow].Index; got != sel {
		t.Errorf("the row marked selected carries option %d, want %d", got, sel)
	}
	if want := sel - len(rows) + 1; rows[0].Index != want && sel >= len(rows) {
		t.Errorf("the window starts at option %d, want %d: the smallest scroll that shows the selection", rows[0].Index, want)
	}

	early := openRowsOn(t, listboxFixture(200, 40, 1))
	if early[0].Index != 0 {
		t.Errorf("a listbox whose selection is the second option starts at option %d, want 0", early[0].Index)
	}
}

func openRowsOn(t *testing.T, html string) []paint.ListBoxRow {
	t.Helper()
	return openRows(t, newListViewport(t, html))
}

func optionLabel(n *dom.Node) string {
	if n == nil {
		return "<nil>"
	}
	return n.OptionLabel()
}

// qc:BR-101
func TestPressOnAWindowedRowPicksTheOptionThatRowShows(t *testing.T) {
	s := newListViewport(t, listboxFixture(200, 40, 33))
	rows := openRows(t, s)
	mid := rows[len(rows)/2]
	if mid.Selected {
		t.Fatalf("the fixture's window leaves no unselected row to press: %v", rows)
	}
	x, y := (mid.Rect.X0+mid.Rect.X1)/2, (mid.Rect.Y0+mid.Rect.Y1)/2
	if !s.ClickAt(x, y) {
		t.Fatal("a press on a visible row was not consumed")
	}
	got := engine.SelectedOption(s.Doc.ElementByID("sel"))
	if got == nil || got.OptionLabel() != mid.Label {
		t.Errorf("a press on the row labelled %q selected option %q, want that row's own option", mid.Label, optionLabel(got))
	}
	want := fmt.Sprintf("v%d", mid.Index)
	if got := s.FocusedSelectValue(); got != want {
		t.Errorf("a press on the row labelled %q set the value to %q, want %q", mid.Label, got, want)
	}
	if s.SelectPopupOpen() {
		t.Error("the listbox stayed open after a row was picked")
	}
}

// qc:BR-101
func TestListboxPlacementFollowsTheLiveScrollOffset(t *testing.T) {
	s := newListViewport(t, listboxFixture(500, 12, 3))
	rows := openRows(t, s)
	box := boxRect(t, s, "sel")
	if rows[len(rows)-1].Rect.Y1 > box.Y0 {
		t.Errorf("with the control at the viewport bottom the panel opens below it (last row ends at %v, control top %v): rows would be off screen",
			rows[len(rows)-1].Rect.Y1, box.Y0)
	}
	assertOnScreen(t, rows, 0)

	s.SyncViewportScroll(0, 300)
	box = boxRect(t, s, "sel")
	after := s.SelectPopupRows()
	if len(after) != 12 {
		t.Fatalf("after scrolling the listbox shows %d rows, want all 12 (they fit below now)", len(after))
	}
	if after[0].Rect.Y0 < box.Y1 {
		t.Errorf("after scrolling, the panel is still above the control at %v, want below its edge %v", after[0].Rect.Y0, box.Y1)
	}
	assertOnScreen(t, after, 300)
}

// qc:BR-101
func TestListboxKeepsOneRowHoldingTheSelectionWhenTheControlLeavesTheViewport(t *testing.T) {
	s := newListViewport(t, listboxFixture(500, 40, 3))
	openRows(t, s)
	s.SyncViewportScroll(0, 550)

	rows := s.SelectPopupRows()
	if len(rows) != 1 {
		t.Fatalf("with no room on either side the listbox shows %d rows, want the one it can still mean", len(rows))
	}
	if rows[0].Index != 3 {
		t.Errorf("the single row shows option %d, want the selected option 3", rows[0].Index)
	}
	if !rows[0].Selected {
		t.Errorf("the single row is not marked selected: %v", rows[0])
	}
}

// qc:BR-101
func TestListboxWithoutAKnownViewportShowsEveryOption(t *testing.T) {
	s, err := engine.NewSession(listboxFixture(540, 12, 3), nil, 800)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	rows := openRows(t, s)
	if len(rows) != 12 {
		t.Fatalf("with no viewport height the listbox shows %d rows, want all 12", len(rows))
	}
	box := boxRect(t, s, "sel")
	if rows[0].Rect.Y0 < box.Y1 {
		t.Errorf("with no viewport height the panel opens above the control at %v, want below its edge %v: nothing measured the room",
			rows[0].Rect.Y0, box.Y1)
	}
}
