package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// BR-101's pointer half: a mouse user must be able to choose an option in a
// <select>. Before this slice a press on the control only focused it - the
// engine had no listbox state at all, so the only way to change a selection was
// the keyboard path in select.go.
//
// The rules below are read off scripts/verify-br101-select-popup.js, which
// drives Chromium's in-page form of the same control (<select size="3">, whose
// rows the page can measure and click, unlike the OS-native popup of a closed
// select). Lines quoted here are that script's output on this host:
//
//	RECTS   -> lb [40,60,180,62.5625], a [41,61,178,20.1875], b [41,81.1875,...], c [41,101.375,...]
//	INITIAL -> {"value":"b","index":1,"selectedAttr":[false,true,false],"active":""}
//	CLOSED-CLICK      -> {"active":"s","value":"2","events":["mousedown","focus","mouseup","click",...]}
//	CLICK-A           -> {"value":"a","index":0,"selectedAttr":[false,true,false],"active":"lb"}
//	EVENTS-CLICK-A    -> ["...mousedown","lb:focus","...mouseup value=a","lb:input","lb:change","...click"]
//	CLICK-C-DISABLED  -> value stays "a"; EVENTS-CLICK-C -> [] (the press is swallowed whole)
//	CLICK-OUTSIDE     -> {"value":"b","active":""} (the listbox keeps its value)
//	AFTER-ESCAPE      -> {"value":"a","active":"lb"} (Escape changes nothing and keeps focus)
//
// qc:BR-101

// selectPopupHTML mirrors the Chromium fixture in the anchor script: same
// position, width and font size, the same three options with "b" authored
// selected and "c" disabled, so the measured row geometry applies verbatim.
const selectPopupHTML = `<html><head><style>` +
	`html,body{margin:0}` +
	`#sel{position:absolute;left:40px;top:60px;width:180px;font-size:16px}` +
	`#other{position:absolute;left:40px;top:300px;width:200px;height:40px}` +
	`</style></head><body>` +
	`<select id="sel" name="pick">` +
	`<option id="a" value="a">Alpha</option>` +
	`<option id="b" value="b" selected>Bravo</option>` +
	`<option id="c" value="c" disabled>Charlie</option>` +
	`</select>` +
	`<div id="other"></div></body></html>`

func newPopupSession(t *testing.T, html string) *engine.Session {
	t.Helper()
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// eventLog records the types `kinds` arrives in, so an oracle can pin the order
// and not just the count: Chromium fires input, then change, and both land
// before the click event reaches the option.
func eventLog(t *testing.T, s *engine.Session, id string, kinds ...string) *[]string {
	t.Helper()
	n := s.Doc.ElementByID(id)
	if n == nil {
		t.Fatalf("element %q not found", id)
	}
	log := &[]string{}
	for _, k := range kinds {
		kind := k
		dom.AddEventListener(n, kind, func(*dom.Event) { *log = append(*log, kind) }, false)
	}
	return log
}

// openPopup presses the middle of the control, the way a pointer user does, and
// returns the rows the listbox is showing.
func openPopup(t *testing.T, s *engine.Session) []paint.ListBoxRow {
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

func rowCentre(rows []paint.ListBoxRow, i int) (float32, float32) {
	r := rows[i].Rect
	return (r.X0 + r.X1) / 2, (r.Y0 + r.Y1) / 2
}

// qc:BR-101
func TestPressOnAClosedSelectOpensTheListboxWithoutChangingTheValue(t *testing.T) {
	s := newPopupSession(t, selectPopupHTML)
	if s.SelectPopupOpen() {
		t.Fatal("a fresh session reports an open select listbox")
	}
	events := eventLog(t, s, "sel", "input", "change")

	x, y := clickPoint(t, s, "sel")
	if !s.ClickAt(x, y) {
		t.Fatal("ClickAt on a closed <select> returned false; the shell would not repaint the listbox")
	}
	if !s.SelectPopupOpen() {
		t.Fatal("the listbox did not open")
	}
	if got := len(s.SelectPopupRows()); got != 3 {
		t.Fatalf("the listbox shows %d rows, want one per option", got)
	}
	// CLOSED-CLICK: pressing a select focuses it and leaves the value alone.
	if n := s.Focused(); n == nil || n.GetAttribute("id") != "sel" {
		t.Errorf("focus after the press is %v, want the select", n)
	}
	if got := engine.SelectedOption(s.Doc.ElementByID("sel")); got == nil || got.GetAttribute("id") != "b" {
		t.Errorf("opening the listbox moved the selection to %v, want b: only a press on a row picks", got)
	}
	if len(*events) != 0 {
		t.Errorf("opening the listbox fired %v, want no input or change (CLOSED-CLICK)", *events)
	}
}

// The rows a pointer can hit must be the rows paint draws, and they must be
// laid out the way Chromium lays out the in-page listbox: inset 1px from the
// control's border box on x, uniform height, stacked with no gap, and one row
// per option in tree order (RECTS).
//
// qc:BR-101
func TestListboxRowsAreInsetUniformAndStacked(t *testing.T) {
	s := newPopupSession(t, selectPopupHTML)
	rows := openPopup(t, s)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	box := boxRect(t, s, "sel")
	x0, x1, y1 := box.X0, box.X1, box.Y1
	for i, row := range rows {
		r := row.Rect
		if r.X0 != x0+1 || r.X1 != x1-1 {
			t.Errorf("row %d spans x [%v,%v], want [%v,%v] inset 1px inside the control", i, r.X0, r.X1, x0+1, x1-1)
		}
		if r.Y1 <= r.Y0 {
			t.Errorf("row %d has no height: %v", i, r)
		}
		if r.Y0 < y1 {
			t.Errorf("row %d starts at y %v, above the control's bottom edge %v: the listbox must not sit on top of the closed box", i, r.Y0, y1)
		}
	}
	if rows[0].Rect.Y1-rows[0].Rect.Y0 != rows[1].Rect.Y1-rows[1].Rect.Y0 || rows[1].Rect.Y1-rows[1].Rect.Y0 != rows[2].Rect.Y1-rows[2].Rect.Y0 {
		t.Errorf("row heights differ (%v, %v, %v); a listbox rows are uniform",
			rows[0].Rect.Y1-rows[0].Rect.Y0, rows[1].Rect.Y1-rows[1].Rect.Y0, rows[2].Rect.Y1-rows[2].Rect.Y0)
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].Rect.Y0 != rows[i-1].Rect.Y1 {
			t.Errorf("row %d starts at %v, want it to abut row %d ending at %v", i, rows[i].Rect.Y0, i-1, rows[i-1].Rect.Y1)
		}
	}
}

func boxRect(t *testing.T, s *engine.Session, id string) frame.RectF {
	t.Helper()
	for i := range s.Arena.Objects {
		obj := &s.Arena.Objects[i]
		if obj.Node != nil && obj.Node.GetAttribute("id") == id {
			x0, y0, x1, y1 := obj.BorderRect()
			return frame.RectF4(x0, y0, x1, y1)
		}
	}
	t.Fatalf("element %q not found in arena", id)
	return frame.RectF{}
}

// CLICK-A is the whole point of the slice: a press on a row makes that option
// the value, exactly once, in Chromium's event order.
//
// qc:BR-101
func TestPressOnARowPicksThatOptionAndClosesTheListbox(t *testing.T) {
	s := newPopupSession(t, selectPopupHTML)
	rows := openPopup(t, s)
	events := eventLog(t, s, "sel", "input", "change")

	sel := s.Doc.ElementByID("sel")
	x, y := rowCentre(rows, 0)
	if !s.ClickAt(x, y) {
		t.Fatal("a press on a listbox row was not consumed")
	}
	if got := engine.SelectedOption(sel); got == nil || got.GetAttribute("id") != "a" {
		t.Fatalf("after a press on row 0 the selection is %v, want a", got)
	}
	if got := sel.GetAttribute("id"); got != "sel" {
		t.Fatalf("the fixture select lost its id: %q", got)
	}
	if v := s.FocusedSelectValue(); v != "a" {
		t.Errorf("value after the pick = %q, want %q (CLICK-A reports value a)", v, "a")
	}
	if s.SelectPopupOpen() {
		t.Error("the listbox stayed open after the row was picked")
	}
	if n := s.Focused(); n == nil || n.GetAttribute("id") != "sel" {
		t.Errorf("focus after the pick is %v, want the select (CLICK-A reports active lb)", n)
	}
	// EVENTS-CLICK-A: input, then change, one of each.
	if got := *events; len(got) != 2 || got[0] != "input" || got[1] != "change" {
		t.Errorf("the select saw %v, want [input change] (EVENTS-CLICK-A)", got)
	}
	// CLICK-A reports selectedAttr [false,true,false] after picking a: the press
	// changes current selectedness and never rewrites the selected attribute.
	if got := s.Doc.ElementByID("a").HasAttribute("selected"); got {
		t.Error("picking a row wrote a selected content attribute onto the option")
	}
	if got := s.Doc.ElementByID("b").HasAttribute("selected"); !got {
		t.Error("picking a row removed b's selected content attribute")
	}
}

// CLICK-C-DISABLED fires no event at all and leaves the value alone.
//
// qc:BR-101
func TestPressOnADisabledRowIsSwallowed(t *testing.T) {
	s := newPopupSession(t, selectPopupHTML)
	rows := openPopup(t, s)
	events := eventLog(t, s, "sel", "input", "change")

	x, y := rowCentre(rows, 2)
	s.ClickAt(x, y)
	if got := engine.SelectedOption(s.Doc.ElementByID("sel")); got == nil || got.GetAttribute("id") != "b" {
		t.Errorf("a press on the disabled row moved the selection to %v, want b", got)
	}
	if len(*events) != 0 {
		t.Errorf("a press on the disabled row fired %v, want no events (EVENTS-CLICK-C)", *events)
	}
	if !s.SelectPopupOpen() {
		t.Error("the listbox closed on a press that selected nothing")
	}
}

// The listbox is not modal in this engine, so the press that dismisses it is not
// captured by it: CLICK-OUTSIDE shows the value surviving, and the page still
// receives the click the user aimed at.
//
// qc:BR-101
func TestPressOutsideTheListboxClosesItAndStillClicksWhatItLandsOn(t *testing.T) {
	s := newPopupSession(t, selectPopupHTML)
	openPopup(t, s)
	events := eventLog(t, s, "sel", "input", "change")
	otherClicks := eventLog(t, s, "other", "click")

	x, y := clickPoint(t, s, "other")
	if !s.ClickAt(x, y) {
		t.Fatal("ClickAt reported nothing to do after dismissing a listbox; the shell would keep painting rows that are gone")
	}
	if s.SelectPopupOpen() {
		t.Error("the listbox stayed open after a press outside it")
	}
	if got := engine.SelectedOption(s.Doc.ElementByID("sel")); got == nil || got.GetAttribute("id") != "b" {
		t.Errorf("dismissing the listbox moved the selection to %v, want b (CLICK-OUTSIDE keeps the value)", got)
	}
	if len(*events) != 0 {
		t.Errorf("dismissing the listbox fired %v, want no events", *events)
	}
	if got := *otherClicks; len(got) != 1 {
		t.Errorf("the element under the dismissing press saw %v, want exactly one click", got)
	}
}

// Pressing the control whose listbox is already showing closes it. Without the
// dismiss-does-not-reopen rule the second press would leave the rows up, so a
// user could never get rid of the popup by the way they raised it.
//
// qc:BR-101
func TestPressOnTheOpenSelectClosesItsListbox(t *testing.T) {
	s := newPopupSession(t, selectPopupHTML)
	openPopup(t, s)

	x, y := clickPoint(t, s, "sel")
	if !s.ClickAt(x, y) {
		t.Fatal("the press that closes an open listbox reported nothing to do")
	}
	if s.SelectPopupOpen() {
		t.Error("pressing the select again re-opened its listbox instead of closing it")
	}
	if got := engine.SelectedOption(s.Doc.ElementByID("sel")); got == nil || got.GetAttribute("id") != "b" {
		t.Errorf("closing the listbox moved the selection to %v, want b", got)
	}
}

// AFTER-ESCAPE: Escape closes the listbox, changes nothing and keeps focus on
// the control, so the keyboard user can carry on with the arrows.
//
// qc:BR-101
func TestEscapeClosesTheListboxAndKeepsFocusAndValue(t *testing.T) {
	s := newPopupSession(t, selectPopupHTML)
	openPopup(t, s)
	events := eventLog(t, s, "sel", "input", "change")

	if !s.PressKey("Escape", false, false, false, false) {
		t.Fatal("Escape with an open listbox reported nothing consumed; the shell would scroll the page")
	}
	if s.SelectPopupOpen() {
		t.Error("Escape left the listbox open")
	}
	if got := engine.SelectedOption(s.Doc.ElementByID("sel")); got == nil || got.GetAttribute("id") != "b" {
		t.Errorf("Escape moved the selection to %v, want b", got)
	}
	if n := s.Focused(); n == nil || n.GetAttribute("id") != "sel" {
		t.Errorf("focus after Escape is %v, want the select (AFTER-ESCAPE reports active lb)", n)
	}
	if len(*events) != 0 {
		t.Errorf("Escape fired %v, want no events", *events)
	}
}

// A <select multiple> and a <select size="3"> are already listboxes: they show
// their rows inline, so a press on them must not raise a second copy.
//
// qc:BR-101
func TestAlreadyListboxSelectsDoNotOpenAPopup(t *testing.T) {
	for _, html := range []string{
		`<html><body style="margin:0"><select id="sel" multiple>` +
			`<option id="a" value="a">Alpha</option><option id="b" value="b">Bravo</option>` +
			`</select></body></html>`,
		`<html><body style="margin:0"><select id="sel" size="3">` +
			`<option id="a" value="a">Alpha</option><option id="b" value="b">Bravo</option>` +
			`</select></body></html>`,
	} {
		s := newPopupSession(t, html)
		x, y := clickPoint(t, s, "sel")
		s.ClickAt(x, y)
		if s.SelectPopupOpen() {
			t.Errorf("%s: a press opened a listbox over a control that already shows its rows inline", html)
		}
	}
}

// qc:BR-101
func TestPressOnADisabledSelectOpensNothing(t *testing.T) {
	s := newPopupSession(t, `<html><body style="margin:0">`+
		`<select id="sel" disabled><option id="a" value="a">Alpha</option></select></body></html>`)
	x, y := clickPoint(t, s, "sel")
	if s.ClickAt(x, y) {
		t.Error("ClickAt consumed a press on a disabled select")
	}
	if s.SelectPopupOpen() {
		t.Error("a disabled select opened a listbox")
	}
}

// The listbox belongs to the control that raised it: Tab (or any other focus
// move) dismisses it rather than leaving rows hanging over a blurred control.
//
// qc:BR-101
func TestFocusMovingAwayDismissesTheListbox(t *testing.T) {
	s := newPopupSession(t, `<html><body style="margin:0">`+
		`<select id="sel"><option id="a" value="a">Alpha</option><option id="b" value="b">Bravo</option></select>`+
		`<input id="txt" value="hi">`+
		`</body></html>`)
	openPopup(t, s)

	txt := s.Doc.ElementByID("txt")
	if txt == nil || !s.FocusNode(txt) {
		t.Fatal("the sibling input would not take focus")
	}
	if s.SelectPopupOpen() {
		t.Error("the listbox stayed open over a blurred select")
	}
	if got := engine.SelectedOption(s.Doc.ElementByID("sel")); got == nil || got.GetAttribute("id") != "a" {
		t.Errorf("losing focus changed the selection to %v, want a", got)
	}
}
