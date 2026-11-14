package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// qc:BR-101

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
	if got := *events; len(got) != 2 || got[0] != "input" || got[1] != "change" {
		t.Errorf("the select saw %v, want [input change] (EVENTS-CLICK-A)", got)
	}
	if got := s.Doc.ElementByID("a").HasAttribute("selected"); got {
		t.Error("picking a row wrote a selected content attribute onto the option")
	}
	if got := s.Doc.ElementByID("b").HasAttribute("selected"); !got {
		t.Error("picking a row removed b's selected content attribute")
	}
}

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
