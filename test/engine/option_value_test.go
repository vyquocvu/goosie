package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
)

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

// qc:BR-104
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

// qc:BR-104
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

// qc:BR-106
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
	s := newFocusSession(t, `<html><body><form id="f"><select id="s" name="c"><option selected value="  pad  me  ">t</option></select></form></body></html>`)
	encoded := engine.EncodeFormURLEncoded(engine.CollectFormData(s.Doc.ElementByID("f"), nil))
	if encoded != "c=++pad++me++" {
		t.Fatalf("encoded form = %q, want %q: Chromium sends %s for this attribute", encoded, "c=++pad++me++", `"  pad  me  "`)
	}
}

// qc:BR-101
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
	for _, id := range []string{"a", "b"} {
		if n := s.Doc.ElementByID(id); n == nil || !n.HasAttribute("selected") {
			t.Fatalf("option#%q lost the selected attribute it was authored with", id)
		}
	}
	three := newFocusSession(t, `<html><body><form id="f"><select id="s" name="c">`+
		`<option id="a" selected>one</option>`+
		`<option id="b" selected>two</option>`+
		`<option id="cc" selected>three</option>`+
		`</select></form></body></html>`)
	if got := submittedValue(t, three, "f", "c"); got != "three" {
		t.Fatalf("three options authored selected submit %q, want %q", got, "three")
	}
}

// qc:BR-139
func TestAccessibilityOptionAnnouncesANameAndNoValue(t *testing.T) {
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
	if charcoal.Value != "" {
		t.Fatalf("option value = %q, want none at all", charcoal.Value)
	}
	if submitted := submittedValue(t, s, "f", "c"); submitted != "Charcoal" {
		t.Fatalf("the form submits %q while the tree announces the name %q: the option's text is the value here",
			submitted, charcoal.Label)
	}
	runs := newFocusSession(t, `<html><body><form id="f"><select id="s" name="c">`+
		`<option selected>  Hi   there </option></select></form></body></html>`)
	runsCombo := axCombo(t, runs, "s")
	if len(runsCombo.Children) != 1 {
		t.Fatalf("combo has %d option nodes, want 1", len(runsCombo.Children))
	}
	if got := runsCombo.Children[0]; got.Label != "Hi there" || got.Value != "" {
		t.Fatalf("option node is labelled %q and valued %q, want %q and no value",
			got.Label, got.Value, "Hi there")
	}
	mixed := newFocusSession(t, `<html><body><form id="f"><select id="s" name="c">`+
		`<option selected value="slate-blue">
   Slate blue
 </option></select></form></body></html>`)
	mixedCombo := axCombo(t, mixed, "s")
	if mixedCombo.Value != "Slate blue" {
		t.Fatalf("combo box announces value %q, want the display string %q Chromium announces",
			mixedCombo.Value, "Slate blue")
	}
	if len(mixedCombo.Children) != 1 {
		t.Fatalf("combo has %d option nodes, want 1", len(mixedCombo.Children))
	}
	if got := mixedCombo.Children[0]; got.Label != "Slate blue" || got.Value != "" {
		t.Fatalf("option node is labelled %q and valued %q, want %q and no value",
			got.Label, got.Value, "Slate blue")
	}
	if submitted := submittedValue(t, mixed, "f", "c"); submitted != "slate-blue" {
		t.Fatalf("the form submits %q while the option node carries no value and announces %q: "+
			"the value attribute belongs to the form alone", submitted, mixedCombo.Children[0].Label)
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

// qc:BR-101
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
	if combo.Value != "B" {
		t.Fatalf("combo box announces %q after EditDown, want the display string %q", combo.Value, "B")
	}
	selected := 0
	for _, o := range combo.Children {
		if o.Selected {
			selected++
			if o.Label != "B" || o.Value != "" {
				t.Fatalf("selected option node = %+v, want the name B and no value", o)
			}
		}
	}
	if selected != 1 {
		t.Fatalf("%d option nodes report Selected after one move, want 1", selected)
	}
	if !s.MoveSelect(engine.EditUp) {
		t.Fatal("MoveSelect(EditUp) did not consume the key")
	}
	if got := s.FocusedSelectValue(); got != "Charcoal" {
		t.Fatalf("FocusedSelectValue after EditUp = %q, want %q", got, "Charcoal")
	}
}

// qc:BR-101
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
	n := 0
	for _, id := range []string{"o1", "o2", "o3"} {
		if o := s.Doc.ElementByID(id); o != nil && o.OptionSelected() {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d options carry selected after a move, want exactly 1", n)
	}
}

// qc:BR-101
func TestPaintSelectFollowsTheKeyboardSelection(t *testing.T) {
	s := newFocusSession(t, charcoalFormHTML)
	sel := s.Doc.ElementByID("s")
	if !s.FocusNode(sel) {
		t.Fatal("FocusNode rejected the select")
	}
	if got := string(textRunesIn(t, s.Paint(1).Build(1), frame.RGB(0, 0, 0))); got != "Charcoal" {
		t.Fatalf("painted label before any key = %q, want %q", got, "Charcoal")
	}
	if !s.MoveSelect(engine.EditDown) {
		t.Fatal("MoveSelect(EditDown) did not consume the key")
	}
	if got := string(textRunesIn(t, s.Paint(1).Build(1), frame.RGB(0, 0, 0))); got != "B" {
		t.Fatalf("painted label after EditDown = %q, want %q", got, "B")
	}
	if got := s.FocusedSelectValue(); got != "b" {
		t.Fatalf("value after EditDown = %q, want %q: the painted label and the submitted value must name the same option", got, "b")
	}
}
