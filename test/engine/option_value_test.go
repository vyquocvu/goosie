package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/engine"
)

// TASK-037: the value contract of <option>. Hand-written markup indents its
// options, so an option's text content arrives at the form layer carrying
// newlines and runs of spaces - "?c=%0A++Charcoal%0A" is a value no server can
// match (GOOSIE-FORMVALUE-001). Every expectation below quotes a line of
// `node scripts/verify-task037-option-value.js`, run against Chromium
// 153.0.8010.12 on this host through Playwright 1.63.0; the script is in the
// same commit as this file, so a rule that was measured can be re-measured.
//
// The two rules the measurements settled:
//
//	text content  -> leading/trailing ASCII whitespace stripped, every internal
//	                       run of it collapsed to one U+0020 (TAB, LF, FF, CR and
//	                       SPACE collapse; U+000B and U+00A0 do not).
//	value attribute -> used verbatim, whitespace and all, including the empty
//	                       string, which is NOT the absent attribute.
//
// Submission, the keyboard path and the accessibility tree all read optionValue
// in internal/engine/focus.go, so one control cannot answer with two values.

// charcoalFormHTML is the markup from the defect report: one option written
// across three lines, selected, plus a second option with a value attribute.
const charcoalFormHTML = `<html><body><form id="f"><select id="s" name="c">
<option selected>
  Charcoal
</option>
<option value="b">B</option>
</select></form></body></html>`

func submittedValue(t *testing.T, s *engine.Session, formID, name string) string {
	t.Helper()
	form := s.Doc.ElementByID(formID)
	if form == nil {
		t.Fatalf("form %q not found", formID)
	}
	got := ""
	seen := 0
	for _, d := range engine.CollectFormData(form, nil) {
		if d.Name == name {
			seen++
			got = d.Value
		}
	}
	if seen != 1 {
		t.Fatalf("form data carried %d entries named %q, want exactly 1", seen, name)
	}
	return got
}

// TestSubmittedOptionValueStripsSurroundingWhitespace is the defect itself: the
// indented option must reach the form as "Charcoal", and the encoding must not
// carry the newlines. Chromium:
//
//	OPTION name=charcoal -> {"textContent":"\n    Charcoal\n  ","value":"Charcoal","label":"Charcoal"}
//	FORMDATA -> [["charcoal","Charcoal"],...]
//	ENCODED  -> "charcoal=Charcoal&..."
func TestSubmittedOptionValueStripsSurroundingWhitespace(t *testing.T) {
	s := newFocusSession(t, charcoalFormHTML)

	if got := submittedValue(t, s, "f", "c"); got != "Charcoal" {
		t.Fatalf("submitted c = %q, want %q: the option's text content is indented over three lines and Chromium reports the stripped word", got, "Charcoal")
	}
	encoded := engine.EncodeFormURLEncoded(engine.CollectFormData(s.Doc.ElementByID("f"), nil))
	if encoded != "c=Charcoal" {
		t.Fatalf("encoded form = %q, want c=Charcoal", encoded)
	}
	if strings.Contains(encoded, "%0A") {
		t.Fatalf("encoded form %q still carries an encoded newline: a GET handler cannot match it against any option", encoded)
	}
}

// TestSubmittedOptionValueCollapsesInnerWhitespaceRuns covers the part a
// strip-only implementation gets wrong. Chromium collapses every internal run:
//
//	OPTION name=hiruns       -> {"textContent":"  Hi   there ","value":"Hi there"}
//	OPTION name=tabinside    -> {"textContent":"Tab\tin\tside","value":"Tab in side"}
//	OPTION name=crlfinside   -> {"textContent":"a\n   b","value":"a b"}
//	OPTION name=formfeed     -> {"textContent":"a\f\fb","value":"a b"}
//	OPTION name=elementkids  -> {"textContent":"a bold   c","value":"a bold c"}
//
// and the two it deliberately does NOT touch:
//
//	OPTION name=vtabinside   -> {"textContent":"a\u000b\u000bb","value":"a\u000b\u000bb"}
//	OPTION name=nbsp         -> {"textContent":"\u00a0nbsp edge\u00a0","value":"\u00a0nbsp edge\u00a0"}
func TestSubmittedOptionValueCollapsesInnerWhitespaceRuns(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"inner run of spaces", "  Hi   there ", "Hi there"},
		{"tabs inside", "Tab\tin\tside", "Tab in side"},
		{"newline and spaces inside", "a\n   b", "a b"},
		{"form feed inside", "a\f\fb", "a b"},
		{"tabs and newline inside", "a\t\n b", "a b"},
		{"text split over element children", "a <b>bold</b>   c", "a bold c"},
		{"only whitespace", "   ", ""},
		{"vertical tab is not ASCII whitespace", "a\u000b\u000bb", "a\u000b\u000bb"},
		{"no-break space is not stripped", "\u00a0nbsp edge\u00a0", "\u00a0nbsp edge\u00a0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newFocusSession(t, `<html><body><form id="f"><select id="s" name="c"><option selected>`+
				tc.body+`</option></select></form></body></html>`)
			got := submittedValue(t, s, "f", "c")
			if got != tc.want {
				t.Fatalf("submitted value = %q, want %q (text content was %q)", got, tc.want, tc.body)
			}
		})
	}
}

// TestOptionValueAttributeIsSubmittedVerbatim pins the branch that must NOT be
// normalised. Chromium hands the content attribute through untouched:
//
//	OPTION name=attrpad     -> {"value":"  pad  me  "}     (attribute, not text)
//	OPTION name=attrnewline -> {"value":"a\n b\tc"}
//	OPTION name=attrnbsp    -> {"value":"\u00a0x\u00a0"}
//	OPTION name=attrempty   -> {"value":""}   with text "ignored text"
//	FORMDATA -> [...,["attrpad","  pad  me  "],["attrnewline","a\n b\tc"],["attrempty",""],...]
//
// So `value=""` submits the empty string rather than falling back to the text,
// and an indented attribute keeps its indentation. A server that received
// "pad me" instead of "  pad  me  " would be reading a value the author never
// wrote.
func TestOptionValueAttributeIsSubmittedVerbatim(t *testing.T) {
	cases := []struct {
		name, attr, want string
	}{
		{"padded attribute", "  pad  me  ", "  pad  me  "},
		{"attribute with newline and tab", "a\n b\tc", "a\n b\tc"},
		{"attribute with no-break spaces", "\u00a0x\u00a0", "\u00a0x\u00a0"},
		{"empty attribute is not the absent one", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newFocusSession(t, `<html><body><form id="f"><select id="s" name="c"><option selected value="`+
				tc.attr+`">ignored text</option></select></form></body></html>`)
			if got := submittedValue(t, s, "f", "c"); got != tc.want {
				t.Fatalf("submitted value = %q, want %q verbatim from the value attribute", got, tc.want)
			}
		})
	}
	// The encoding keeps what the attribute held, which is the observable
	// difference from the text-content branch.
	s := newFocusSession(t, `<html><body><form id="f"><select id="s" name="c"><option selected value="  pad  me  ">t</option></select></form></body></html>`)
	encoded := engine.EncodeFormURLEncoded(engine.CollectFormData(s.Doc.ElementByID("f"), nil))
	if encoded != "c=++pad++me++" {
		t.Fatalf("encoded form = %q, want %q: Chromium sends %s for this attribute", encoded, "c=++pad++me++", `"  pad  me  "`)
	}
}

// TestLastAuthoredSelectedOptionWins is the tie-break. Two options both carry
// `selected`; Chromium resolves it at read time in favour of the last one and
// leaves the markup alone:
//
//	TIEBREAK two-selected -> {"selectedIndex":1,"value":"b","optionFlags":[false,true,false],
//	                         "attrFlags":[true,true,false],"formdata":[["c","b"]]}
//
// Goosie submitted "a" here because SelectedOption returned at the first
// `selected` it saw.
func TestLastAuthoredSelectedOptionWins(t *testing.T) {
	html := `<html><body><form id="f"><select id="s" name="c">` +
		`<option id="a" value="a" selected>A</option>` +
		`<option id="b" value="b" selected>B</option>` +
		`<option id="cc" value="c">C</option>` +
		`</select></form></body></html>`
	s := newFocusSession(t, html)

	if got := submittedValue(t, s, "f", "c"); got != "b" {
		t.Fatalf("submitted c = %q, want %q: the last option authored selected wins", got, "b")
	}
	sel := s.Doc.ElementByID("s")
	if got := engine.SelectedOption(sel); got == nil || got.GetAttribute("id") != "b" {
		t.Fatalf("SelectedOption returned %v, want option#b", got)
	}
	// The document is not rewritten: both content attributes survive, exactly as
	// attrFlags [true,true,false] shows in Chromium.
	for _, id := range []string{"a", "b"} {
		if n := s.Doc.ElementByID(id); n == nil || !n.HasAttribute("selected") {
			t.Fatalf("option#%q lost the selected attribute it was authored with", id)
		}
	}
	// Three authored selections still collapse to the last, and the fallback when
	// nothing is authored selected stays the first option (TestSelectedOption-
	// Default, select_keyboard_test.go).
	three := newFocusSession(t, `<html><body><form id="f"><select id="s" name="c">`+
		`<option id="a" selected>one</option>`+
		`<option id="b" selected>two</option>`+
		`<option id="cc" selected>three</option>`+
		`</select></form></body></html>`)
	if got := submittedValue(t, three, "f", "c"); got != "three" {
		t.Fatalf("three options authored selected submit %q, want %q", got, "three")
	}
}

// TestAccessibilityOptionLabelAndValueShareNormalization is the cross-check on
// the shape that used to drift: an AX option node whose Label was trimmed while
// its Value kept the newlines. Both now come from the same rule, and the value
// the tree announces is the value the form sends.
//
//	OPTION name=charcoal -> {"value":"Charcoal","label":"Charcoal"}
//	INDENTED initial     -> {"selectedIndex":0,"value":"Charcoal","optionLabel":"Charcoal"}
func TestAccessibilityOptionLabelAndValueShareNormalization(t *testing.T) {
	s := newFocusSession(t, charcoalFormHTML)
	combo := axCombo(t, s, "s")
	if combo.Value != "Charcoal" {
		t.Fatalf("combo box announces value %q, want %q", combo.Value, "Charcoal")
	}
	var charcoal *ax.Node
	for i := range combo.Children {
		if combo.Children[i].Role == ax.RoleOption && combo.Children[i].Selected {
			charcoal = &combo.Children[i]
		}
	}
	if charcoal == nil {
		t.Fatalf("no selected option node under the combo box: %+v", combo.Children)
	}
	if charcoal.Label != "Charcoal" {
		t.Fatalf("option label = %q, want %q", charcoal.Label, "Charcoal")
	}
	if charcoal.Value != "Charcoal" {
		t.Fatalf("option value = %q, want %q", charcoal.Value, "Charcoal")
	}
	if submitted := submittedValue(t, s, "f", "c"); submitted != charcoal.Value {
		t.Fatalf("accessibility announces %q but the form submits %q: one control, two answers",
			charcoal.Value, submitted)
	}
	// The whitespace shape has to agree too, not just the value: an option whose
	// text is "  Hi   there " is labelled and valued "Hi there".
	runs := newFocusSession(t, `<html><body><form id="f"><select id="s" name="c">`+
		`<option selected>  Hi   there </option></select></form></body></html>`)
	runsCombo := axCombo(t, runs, "s")
	if len(runsCombo.Children) != 1 {
		t.Fatalf("combo has %d option nodes, want 1", len(runsCombo.Children))
	}
	if got := runsCombo.Children[0]; got.Label != "Hi there" || got.Value != "Hi there" {
		t.Fatalf("option node is labelled %q and valued %q, want %q for both",
			got.Label, got.Value, "Hi there")
	}
	// The two answers only have to agree where they differ, so the third shape
	// keeps the value attribute distinct from the displayed text: an AX tree that
	// recomputed the select's value from the option's text would announce
	// "Slate blue" here while the form sent "slate-blue".
	//
	//	OPTION name=attrpad -> {"value":"  pad  me  ","label":"ignored text"}
	//	INDENTED initial    -> {"optionLabel":"Charcoal"} (label is text, value is text)
	mixed := newFocusSession(t, `<html><body><form id="f"><select id="s" name="c">`+
		`<option selected value="slate-blue">
   Slate blue
 </option></select></form></body></html>`)
	mixedCombo := axCombo(t, mixed, "s")
	if mixedCombo.Value != "slate-blue" {
		t.Fatalf("combo box announces value %q, want the value attribute %q it submits",
			mixedCombo.Value, "slate-blue")
	}
	if len(mixedCombo.Children) != 1 {
		t.Fatalf("combo has %d option nodes, want 1", len(mixedCombo.Children))
	}
	if got := mixedCombo.Children[0]; got.Label != "Slate blue" || got.Value != "slate-blue" {
		t.Fatalf("option node is labelled %q and valued %q, want %q and %q",
			got.Label, got.Value, "Slate blue", "slate-blue")
	}
	if submitted := submittedValue(t, mixed, "f", "c"); submitted != mixedCombo.Value {
		t.Fatalf("accessibility announces %q but the form submits %q: one control, two answers",
			mixedCombo.Value, submitted)
	}
}

func axCombo(t *testing.T, s *engine.Session, id string) *ax.Node {
	t.Helper()
	flat := axFlatten(s.AccessibilityTree())
	for i := range flat {
		if flat[i].Role == ax.RoleComboBox && flat[i].ID == id {
			return &flat[i]
		}
	}
	t.Fatalf("combo box %q missing from the accessibility tree", id)
	return nil
}

// TestKeyboardSelectOnIndentedMarkupMovesAndUpdatesAX joins the journey the way
// TASK-032 joined it: a real key press on the indented markup, then the value
// the shell reads, the value the form would send and the tree the screen reader
// walks must all agree.
//
// INDENTED initial -> {"selectedIndex":0,"value":"Charcoal"}
func TestKeyboardSelectOnIndentedMarkupMovesAndUpdatesAX(t *testing.T) {
	s := newFocusSession(t, charcoalFormHTML)
	sel := s.Doc.ElementByID("s")
	if !s.FocusNode(sel) {
		t.Fatal("FocusNode rejected the select")
	}
	if got := s.FocusedSelectValue(); got != "Charcoal" {
		t.Fatalf("FocusedSelectValue before any key = %q, want %q", got, "Charcoal")
	}
	if !s.MoveSelect(engine.EditDown) {
		t.Fatal("MoveSelect(EditDown) did not consume the key")
	}
	if got := s.FocusedSelectValue(); got != "b" {
		t.Fatalf("FocusedSelectValue after EditDown = %q, want %q", got, "b")
	}
	if got := submittedValue(t, s, "f", "c"); got != "b" {
		t.Fatalf("form would submit c=%q after EditDown, want c=b", got)
	}
	combo := axCombo(t, s, "s")
	if combo.Value != "b" {
		t.Fatalf("combo box announces %q after EditDown, want %q", combo.Value, "b")
	}
	selected := 0
	for _, o := range combo.Children {
		if o.Selected {
			selected++
			if o.Value != "b" {
				t.Fatalf("selected option node = %+v, want value b", o)
			}
		}
	}
	if selected != 1 {
		t.Fatalf("%d option nodes report Selected after one move, want 1", selected)
	}
	// Back up: the whitespace option is reached again with its normalised value,
	// not the three lines of markup it was written on.
	if !s.MoveSelect(engine.EditUp) {
		t.Fatal("MoveSelect(EditUp) did not consume the key")
	}
	if got := s.FocusedSelectValue(); got != "Charcoal" {
		t.Fatalf("FocusedSelectValue after EditUp = %q, want %q", got, "Charcoal")
	}
}

// TestMoveSelectStepsFromTheLastAuthoredSelection keeps the keyboard and the
// value on the same tie-break. Chromium steps from the option it treats as
// selected, not from the first one authored:
//
//	LISTBOX two-selected initial        -> {"selectedIndex":1,"value":"b"}
//	LISTBOX two-selected after ArrowDown-> {"selectedIndex":2,"value":"c"}
//	LISTBOX two-selected after ArrowUp  -> {"selectedIndex":1,"value":"b"}
//
// (Measured on size=3 because a closed select ignores arrow keys in headless
// Chromium: CLOSED two-selected after ArrowDown -> {"selectedIndex":1}.)
// Stepping from the first authored selection would make the first key press
// look dead, because the control already carries the last one's value.
func TestMoveSelectStepsFromTheLastAuthoredSelection(t *testing.T) {
	html := `<html><body style="margin: 0;"><select id="sel" name="opt">` +
		`<option id="o1" value="1" selected>One</option>` +
		`<option id="o2" value="2" selected>Two</option>` +
		`<option id="o3" value="3">Three</option>` +
		`</select></body></html>`
	s := newFocusSession(t, html)
	sel := s.Doc.ElementByID("sel")
	if !s.FocusNode(sel) {
		t.Fatal("FocusNode rejected the select")
	}
	if got := s.FocusedSelectValue(); got != "2" {
		t.Fatalf("value before any key = %q, want 2: the last authored selection wins", got)
	}
	if !s.MoveSelect(engine.EditDown) {
		t.Fatal("MoveSelect(EditDown) returned false")
	}
	if got := s.FocusedSelectValue(); got != "3" {
		t.Fatalf("value after EditDown = %q, want 3: a move steps from the winning selection", got)
	}
	if !s.MoveSelect(engine.EditUp) {
		t.Fatal("MoveSelect(EditUp) returned false")
	}
	if got := s.FocusedSelectValue(); got != "2" {
		t.Fatalf("value after EditUp = %q, want 2", got)
	}
	// The move collapses the document to one selection, as it always has.
	n := 0
	for _, id := range []string{"o1", "o2", "o3"} {
		if o := s.Doc.ElementByID(id); o != nil && o.HasAttribute("selected") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d options carry selected after a move, want exactly 1", n)
	}
}
