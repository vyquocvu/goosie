package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

// Keyboard selection for <select> (BR-101). Before this the control was
// focusable but immovable: nothing in the engine changed the "selected"
// attribute, so a user could never pick an option that the document did not
// already ship as selected.

const selectKeyboardHTML = `<html><body style="margin: 0;">` +
	`<form action="/s" method="GET">` +
	`<select id="sel" name="opt">` +
	`<option id="o1" value="1">One</option>` +
	`<option id="o2" value="2">Two</option>` +
	`<option id="o3" value="3">Three</option>` +
	`</select>` +
	`</form></body></html>`

func newSelectSession(t *testing.T, html string) (*engine.Session, *dom.Node) {
	t.Helper()
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	sel := s.Doc.ElementByID("sel")
	if sel == nil {
		t.Fatal("select#sel not found")
	}
	if !s.FocusNode(sel) {
		t.Fatal("FocusNode(select) returned false; a select must be focusable")
	}
	if s.Focused() != sel {
		t.Fatalf("focus is %v, want the select", s.Focused())
	}
	return s, sel
}

func optionIDs(s *engine.Session) []string {
	sel := s.Doc.ElementByID("sel")
	var ids []string
	for c := sel.FirstChild; c != nil; c = c.NextSibling {
		if c.Element() && c.Data == "option" {
			ids = append(ids, c.GetAttribute("id"))
		}
	}
	return ids
}

func selectedID(s *engine.Session) string {
	for _, id := range []string{"o1", "o2", "o3"} {
		if n := s.Doc.ElementByID(id); n != nil && n.HasAttribute("selected") {
			return id
		}
	}
	return ""
}

// selectedCount catches the failure mode where a move sets the new option but
// leaves the old one selected: two selected options make controlValue and
// SelectedOption disagree about which one is chosen, silently.
func selectedCount(s *engine.Session) int {
	n := 0
	for _, id := range []string{"o1", "o2", "o3"} {
		if o := s.Doc.ElementByID(id); o != nil && o.HasAttribute("selected") {
			n++
		}
	}
	return n
}

func countEvents(t *testing.T, s *engine.Session, id string, kinds ...string) map[string]int {
	t.Helper()
	n := s.Doc.ElementByID(id)
	if n == nil {
		t.Fatalf("element %q not found", id)
	}
	seen := map[string]int{}
	for _, kind := range kinds {
		k := kind
		seen[k] = 0
		dom.AddEventListener(n, k, func(*dom.Event) { seen[k]++ }, false)
	}
	return seen
}

// qc:BR-101
func TestSelectArrowKeysMoveSelection(t *testing.T) {
	s, sel := newSelectSession(t, selectKeyboardHTML)
	events := countEvents(t, s, "sel", "input", "change")

	if got := engine.SelectedOption(sel); got == nil || got.GetAttribute("id") != "o1" {
		t.Fatalf("a select with no authored selected attribute reports %v, want o1: selectedIndex starts at 0", got)
	}
	if got := selectedCount(s); got != 0 {
		t.Fatalf("%d options carry the selected attribute before any key, want 0", got)
	}
	if !s.MoveSelect(engine.EditDown) {
		t.Fatal("MoveSelect(EditDown) on a focused select returned false; the key was not consumed")
	}
	if got := selectedID(s); got != "o2" {
		t.Fatalf("first EditDown selected %q, want o2: the first option was already selected, so a move steps to the next one", got)
	}
	if !s.MoveSelect(engine.EditDown) {
		t.Fatal("second MoveSelect(EditDown) returned false")
	}
	if got := selectedID(s); got != "o3" {
		t.Fatalf("second EditDown selected %q, want o3", got)
	}
	if got := selectedCount(s); got != 1 {
		t.Fatalf("%d options carry selected after a move, want exactly 1", got)
	}
	if events["input"] != 2 || events["change"] != 2 {
		t.Fatalf("after two moves the select saw %d input and %d change events, want 2 and 2", events["input"], events["change"])
	}
	if s.Focused() != sel {
		t.Fatal("moving the selection moved focus off the select")
	}
}

// qc:BR-101
func TestSelectKeysAreConsumedAtTheEnds(t *testing.T) {
	s, _ := newSelectSession(t, selectKeyboardHTML)
	for i := 0; i < 4; i++ {
		if !s.MoveSelect(engine.EditDown) {
			t.Fatalf("MoveSelect(EditDown) #%d returned false", i+1)
		}
	}
	if got := selectedID(s); got != "o3" {
		t.Fatalf("after four EditDown the selection is %q, want it to stop at o3", got)
	}
	events := countEvents(t, s, "sel", "input", "change")
	if !s.MoveSelect(engine.EditDown) {
		t.Fatal("EditDown at the last option returned false; the shell would scroll the page instead")
	}
	if got := selectedID(s); got != "o3" {
		t.Fatalf("EditDown at the last option moved the selection to %q, want o3", got)
	}
	if events["input"] != 0 || events["change"] != 0 {
		t.Fatalf("a no-op move fired %d input and %d change events, want 0 and 0", events["input"], events["change"])
	}
}

// qc:BR-101
func TestSelectSkipsDisabledOptions(t *testing.T) {
	html := `<html><body style="margin: 0;">` +
		`<select id="sel" name="opt">` +
		`<option id="o1" value="1">One</option>` +
		`<option id="o2" value="2" disabled>Two</option>` +
		`<option id="o3" value="3">Three</option>` +
		`</select></body></html>`
	s, _ := newSelectSession(t, html)
	if !s.MoveSelect(engine.EditDown) {
		t.Fatal("MoveSelect(EditDown) returned false")
	} // o1 -> must skip disabled o2
	if got := selectedID(s); got != "o3" {
		t.Fatalf("EditDown with o2 disabled landed on %q, want o3: disabled options must be skipped", got)
	}
	if o := s.Doc.ElementByID("o2"); o != nil && o.HasAttribute("selected") {
		t.Fatal("the disabled option carries the selected attribute")
	}
	if !s.MoveSelect(engine.EditUp) {
		t.Fatal("MoveSelect(EditUp) returned false")
	}
	if got := selectedID(s); got != "o1" {
		t.Fatalf("EditUp from o3 landed on %q, want o1 (o2 is disabled)", got)
	}
}

// qc:BR-101
func TestSelectHomeEndGoToTheEdges(t *testing.T) {
	s, _ := newSelectSession(t, selectKeyboardHTML)
	s.MoveSelect(engine.EditDown) // o1 -> o2
	if !s.MoveSelect(engine.EditEnd) {
		t.Fatal("MoveSelect(EditEnd) returned false")
	}
	if got := selectedID(s); got != "o3" {
		t.Fatalf("EditEnd selected %q, want o3", got)
	}
	if !s.MoveSelect(engine.EditHome) {
		t.Fatal("MoveSelect(EditHome) returned false")
	}
	if got := selectedID(s); got != "o1" {
		t.Fatalf("EditHome selected %q, want o1", got)
	}
	if got := selectedCount(s); got != 1 {
		t.Fatalf("%d options carry selected after EditHome, want 1", got)
	}
}

// qc:BR-101
func TestSelectMoveChangesSubmittedValue(t *testing.T) {
	s, _ := newSelectSession(t, selectKeyboardHTML)
	form := s.Doc.ElementByID("sel").Parent
	if form == nil || form.Data != "form" {
		t.Fatalf("the select's parent is %v, want the form", form)
	}
	if got := s.FocusedSelectValue(); got != "1" {
		t.Fatalf("FocusedSelectValue before any key is %q, want 1: the shell reads the label through this accessor", got)
	}
	s.MoveSelect(engine.EditDown) // o1 -> o2
	data := engine.CollectFormData(form, nil)
	got := ""
	for _, p := range data {
		if p.Name == "opt" {
			if got != "" {
				t.Fatalf("form data carries two opt values (%q and %q), want one", got, p.Value)
			}
			got = p.Value
		}
	}
	if got != "2" {
		t.Fatalf("form data sends opt=%q after one EditDown, want opt=2", got)
	}
	if want := s.FocusedSelectValue(); want != got {
		t.Fatalf("FocusedSelectValue reports %q but the form submits %q: the value a user sees must be the value that is sent", want, got)
	}
}

func TestSelectMoveCollapsesMultipleAuthoredSelections(t *testing.T) {
	html := `<html><body style="margin: 0;">` +
		`<select id="sel" name="opt">` +
		`<option id="o1" value="1" selected>One</option>` +
		`<option id="o2" value="2" selected>Two</option>` +
		`<option id="o3" value="3">Three</option>` +
		`</select></body></html>`
	s, _ := newSelectSession(t, html)
	if !s.MoveSelect(engine.EditDown) {
		t.Fatal("MoveSelect(EditDown) returned false")
	}
	if got := selectedCount(s); got != 1 {
		t.Fatalf("%d options still carry selected after a move, want the document collapsed to 1", got)
	}
	// TASK-037: the last authored selection is the selection, so a move steps
	// forward from o2 to o3. Chromium measures the same pair of facts -
	// two selected options give selectedIndex 1, and the next ArrowDown gives
	// selectedIndex 2 (scripts/verify-task037-option-value.js,
	// "LISTBOX two-selected initial"/"after ArrowDown"). Stepping from o1 would
	// land on o2, the option that was already winning, and the key would look
	// dead to the user.
	if got := selectedID(s); got != "o3" {
		t.Fatalf("move from a two-selected document landed on %q, want o3: the last authored selection is what a move steps from", got)
	}
}

func TestSelectKeysIgnoreOtherActions(t *testing.T) {
	s, _ := newSelectSession(t, selectKeyboardHTML)
	for _, a := range []engine.EditAction{engine.EditRune, engine.EditLeft, engine.EditRight, engine.EditBackspace, engine.EditEnter, engine.EditEscape} {
		if s.MoveSelect(a) {
			t.Fatalf("MoveSelect(%v) returned true on a focused select; only up/down/home/end belong to it", a)
		}
	}
	if got := selectedCount(s); got != 0 {
		t.Fatalf("%d options carry selected after unrelated keys, want 0", got)
	}
}

func TestDisabledSelectDoesNotConsumeKeys(t *testing.T) {
	html := `<html><body style="margin: 0;">` +
		`<select id="sel" name="opt" disabled>` +
		`<option id="o1" value="1">One</option>` +
		`<option id="o2" value="2">Two</option>` +
		`</select></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	sel := s.Doc.ElementByID("sel")
	if s.FocusNode(sel) {
		t.Fatal("FocusNode accepted a disabled select")
	}
	if s.MoveSelect(engine.EditDown) {
		t.Fatal("MoveSelect consumed a key with no select focused")
	}
	if got := selectedCount(s); got != 0 {
		t.Fatalf("%d options carry selected, want 0", got)
	}
}

func TestEmptySelectConsumesKeysWithoutChangingAnything(t *testing.T) {
	html := `<html><body style="margin: 0;"><select id="sel" name="opt"></select></body></html>`
	s, _ := newSelectSession(t, html)
	if !s.MoveSelect(engine.EditDown) {
		t.Fatal("MoveSelect on an optionless select returned false; the page would scroll instead")
	}
	if ids := optionIDs(s); len(ids) != 0 {
		t.Fatalf("optionless select ended up with options %v", ids)
	}
}

// qc:BR-101
func TestClickFocusesSelectButNotDisabledControls(t *testing.T) {
	html := `<html><body style="margin: 0;">` +
		`<select id="sel" name="opt"><option id="o1" value="1">One</option><option id="o2" value="2">Two</option></select>` +
		`<select id="dis" name="dis" disabled><option id="o3" value="3">Three</option></select>` +
		`<input id="txt" name="txt" value="hi" disabled>` +
		`</body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	x, y := clickPoint(t, s, "sel")
	if !s.FocusControl(x, y) {
		t.Fatal("clicking a select did not change focus; a keyboard user cannot reach the selection keys")
	}
	if n := s.Focused(); n == nil || n.GetAttribute("id") != "sel" {
		t.Fatalf("focus after clicking the select is %v, want the select", n)
	}

	for _, id := range []string{"dis", "txt"} {
		s.FocusControl(795, 595) // start each case from no focus
		x, y := clickPoint(t, s, id)
		if s.FocusControl(x, y) {
			t.Fatalf("clicking the disabled %q changed focus; disabled controls must not take focus", id)
		}
		if n := s.Focused(); n != nil {
			t.Fatalf("focus is %v after clicking disabled %q, want nothing focused", n, id)
		}
	}
}
