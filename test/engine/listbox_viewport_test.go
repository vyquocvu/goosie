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

// GOOSIE-LISTBOX-001: every row a listbox shows must be a row the user can see,
// and the row a pointer hits must be the row that is on screen. Before this
// slice listBoxLocked placed the panel at the control's bottom edge with the
// whole option list tiled below it, so a select near the bottom of the viewport
// painted and hit-tested rows off screen, and a list longer than the panel showed
// only its first rows.
//
// The row-window rule is anchored on scripts/verify-br101-listbox-viewport.js,
// which measures Chromium 1.63 on this host (2026-11-04). A closed select's
// popup is NOT observable from a headless host at all - the same script diffs a
// before/after page.screenshot around a click on a select at y=540 of a 600px
// viewport and prints POPUP-PIXELS control [40,540,180,21] whose measured diff
// is CHANGED [40,540,219,560], exactly the control's own rect - so the window
// rule comes from the observable form of the same widget, <select size=N>, where
// scrollTop/rowHeight IS the index of the top visible row. Its lines:
//
//	ROW-WINDOW size 3 of 20, selected 0  -> scrollTop 0    topRow 0.000  sel-visible+1 = 0
//	ROW-WINDOW size 3 of 20, selected 15 -> scrollTop 262  topRow 12.978 sel-visible+1 = 13
//	ROW-WINDOW size 5 of 40, selected 37 -> scrollTop 666  topRow 32.991 sel-visible+1 = 33
//
// that is: the box keeps its own height, the list scrolls inside it, and the
// scroll moves by the smallest amount that brings the selection into view.
// The same script's GEO-CLAMP lines show the in-page listbox does NOT clamp to
// the viewport (size 60 of 70 measures a 1213.25px box in a 600px viewport), so
// the panel's clamp and side choice are this engine's own rule, forced by the
// rows-ink-pointer invariant, and are recorded as such in
// docs/qa-open-defects.json.

// listboxFixture is the select of the anchor script's fixture (same left, width
// and font size), placed at a caller-chosen top, holding n options and with
// option `selected` authored selected.
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

// newListViewport returns a session with a JS runtime, so a test can move the
// document's scroll offset the way the host does and the listbox has to answer
// from it.
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

// assertOnScreen pins the invariant the whole slice exists for: no row the
// pointer can hit lies outside the viewport the user is looking at.
func assertOnScreen(t *testing.T, rows []paint.ListBoxRow, scrollY float32) {
	t.Helper()
	for i, r := range rows {
		if r.Rect.Y0 < scrollY || r.Rect.Y1 > scrollY+600 {
			t.Errorf("row %d (%s) spans y [%v,%v], outside the viewport [%v,%v]",
				i, r.Label, r.Rect.Y0, r.Rect.Y1, scrollY, scrollY+600)
		}
	}
}

// A list that fits in the space below the control still opens below it: the fix
// must not turn every popup upside down.
//
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

// A select near the bottom of the viewport has no room for its list below the
// control and plenty above it, so the panel must open upward rather than paint
// rows the user cannot see.
//
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

// A list longer than any space the viewport offers shows a window of rows, as
// tall as it can be without overflowing, positioned by the smallest scroll that
// brings the selected option into view - the rule the anchor reads off
// <select size=3>'s scrollTop.
//
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

	// The window is maximal: the space it was given holds no further row.
	visible := float32(len(rows))*rowH + 2
	if visible+rowH <= 600-box.Y1 {
		t.Errorf("the window is %v tall for %v px of room below the control: a row still fits", visible, 600-box.Y1)
	}

	// Contiguous options, in tree order.
	for i, r := range rows {
		if r.Index != rows[0].Index+i {
			t.Fatalf("row %d carries option %d, want %d: the window must be contiguous", i, r.Index, rows[0].Index+i)
		}
		if want := fmt.Sprintf("opt%d", r.Index); r.Label != want {
			t.Errorf("row %d is labelled %q, want %q", i, r.Label, want)
		}
	}
	// The selection is visible, and the window sits at the smallest scroll that
	// shows it: the selected row is the last one when it is below the window.
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

	// A selection that already fits at the top needs no scroll at all.
	early := openRowsOn(t, listboxFixture(200, 40, 1))
	if early[0].Index != 0 {
		t.Errorf("a listbox whose selection is the second option starts at option %d, want 0", early[0].Index)
	}
}

func openRowsOn(t *testing.T, html string) []paint.ListBoxRow {
	t.Helper()
	return openRows(t, newListViewport(t, html))
}

// optionLabel keeps a failure message readable.
func optionLabel(n *dom.Node) string {
	if n == nil {
		return "<nil>"
	}
	return n.OptionLabel()
}

// The rows the pointer hits are the rows the panel shows, so a press on the last
// visible row must pick the option that row is labelled with - not whatever sits
// at that position in the full option list.
//
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
	// The row the panel drew is the option the control now carries.
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

// The panel is placed against the live scroll offset, not against the document:
// the same control opens upward while it sits near the viewport bottom and
// downward once the page has scrolled it back to the top.
//
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

// A page gesture can carry the control out of view while its listbox is open.
// There is then no side with room, and the panel must not answer by tiling the
// whole list into space nobody can see: it keeps one row, and that row is the
// selection.
//
// qc:BR-101
func TestListboxKeepsOneRowHoldingTheSelectionWhenTheControlLeavesTheViewport(t *testing.T) {
	s := newListViewport(t, listboxFixture(500, 40, 3))
	openRows(t, s)
	s.SyncViewportScroll(0, 550) // the control's bottom edge is now above the viewport

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

// A host that never told the session how tall its viewport is has no room to
// measure against, so the listbox keeps its full list rather than showing one
// row. This pins that fallback instead of leaving it implicit.
//
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
