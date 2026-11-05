package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/engine"
)

// The accessibility tree is where these rules are measurable: a browser's
// rendered option text is not readable by page script, so the tree is what
// Chromium exposes as the name of each option node. The lines quoted below come
// from scripts/verify-optionax-surfaces.js on this host (Chromium 153.0.8010.12,
// 2026-11-05), which keeps the candidate rules in JavaScript and compares them
// against Chromium's answer shape by shape - so a wrong guess about the
// reference reddens the anchor, not these tests.
//
//	AX edges+inner   -> {"role":"combobox","name":"","value":"My   Label"}
//	                    {"role":"option","name":" My Label "}
//	                    for label="  My   Label  ", text "WWWWW"
//	AX three-spaces  -> {"role":"combobox","name":""} - no value at all -
//	                    {"role":"option","name":"  "}
//	AX vt-edges      -> combobox value "My" while the option name keeps
//	                    "\u000bMy\u000b": the display trims U+000B, the name does not
//	AX nbsp-edges    -> both surfaces keep U+00A0 verbatim
//	AX bare          -> {"role":"combobox","name":"","value":"Alpha Beta"} for
//	                    <option value="v1" selected>Alpha Beta</option>: the
//	                    control announces the display and the form value stays
//	                    in the form. label[for] -> name "Author label",
//	                    aria-label -> "Aria name", size="2"/multiple -> listbox
//	                    with no value.
//	OPTGROUP-AX      -> group named "Group Label" with BOTH options inside it
//
// The two spellings are two functions on purpose: OptionAccessibleName
// collapses every run to one space and keeps one at each edge, while
// OptionDisplayStyle strips the edges and leaves the interior written as it
// was. Paint and the listbox rows keep the normalised form the BR-101 slice
// shipped; the tree answers each surface the way the reference does.

// comboByID finds the select's node by DOM id: Chromium leaves a combobox's
// accessible name to its author, so a role+label lookup cannot pin it.
func comboByID(nodes []ax.Node, id string) *ax.Node {
	for i := range nodes {
		if nodes[i].Role == ax.RoleComboBox && nodes[i].ID == id {
			return &nodes[i]
		}
		if got := comboByID(nodes[i].Children, id); got != nil {
			return got
		}
	}
	return nil
}

// qc:BR-139
func TestAccessibilityOptionNamesFollowTheLabelAttribute(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin:0">
<select id="sel">
  <option id="lab" label="IIIII">WWWWW</option>
  <option id="empty" label="">BBBBB</option>
  <option id="blank" label="   ">CCCCC</option>
  <option id="valonly" value="V"></option>
  <option id="spaces" label="  My   Label  ">DDDDD</option>
</select></body></html>`)
	options := comboByID(s.AccessibilityTree(), "sel")
	if options == nil {
		t.Fatal("no combo box in the accessibility tree")
	}
	// "  " for label="   " is the measured name, not a typo: a run touching
	// each edge contributes one space, so a whitespace-only label announces two
	// spaces. The old expectation here ("") was written before that shape was
	// measured and is the clause GOOSIE-OPTIONAX-001 was opened for.
	want := []string{"IIIII", "BBBBB", "  ", "", " My Label "}
	if len(options.Children) != len(want) {
		t.Fatalf("combo has %d children, want %d: %+v", len(options.Children), len(want), options.Children)
	}
	for i, w := range want {
		if got := options.Children[i].Label; got != w {
			t.Errorf("option %d announced %q, want %q (Chromium: %q)", i, got, w, w)
		}
	}
}

// A value attribute is a submitted value, never a name: an option with only a
// value and no text announces nothing in Chromium, and the same attribute must
// not reach the display string through the label rule either.
//
// qc:BR-101 BR-139
func TestAccessibilityValueAttributeNeverNamesAnOption(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin:0">
<select id="sel"><option id="a" value="alpha" label="Alpha Label">Text A</option></select></body></html>`)
	if !s.FocusNode(s.Doc.ElementByID("sel")) {
		t.Fatal("the select would not take focus")
	}
	if got := s.FocusedSelectValue(); got != "alpha" {
		t.Errorf("FocusedSelectValue() = %q, want %q: the label attribute never becomes the value", got, "alpha")
	}
	combo := comboByID(s.AccessibilityTree(), "sel")
	if combo == nil || len(combo.Children) != 1 {
		t.Fatalf("combo children = %+v, want the one option", combo)
	}
	if got := combo.Children[0].Label; got != "Alpha Label" {
		t.Errorf("option announced %q, want %q", got, "Alpha Label")
	}
	if got := combo.Children[0].Value; got != "alpha" {
		t.Errorf("option value = %q, want %q", got, "alpha")
	}
}

// Options under an <optgroup> are part of the control: Chromium flattens them
// into .options and exposes them in the tree under a group named by the
// group's label attribute. Before this slice the tree walked only direct
// children of the select, so every option inside a group was unreachable.
//
// qc:BR-101 BR-139
func TestAccessibilityExposesOptionsInsideOptgroup(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin:0">
<select id="sel">
  <option id="top" value="top">Top</option>
  <optgroup id="g" label="  Group  Label  ">
    <option id="in1" value="in1">In one</option>
    <option id="in2" selected value="in2">In two</option>
  </optgroup>
  <option id="tail" value="tail">Tail</option>
</select></body></html>`)
	combo := comboByID(s.AccessibilityTree(), "sel")
	if combo == nil {
		t.Fatal("no combo box in the accessibility tree")
	}
	var names []string
	var group *ax.Node
	for i := range combo.Children {
		child := &combo.Children[i]
		if child.Role == ax.RoleGroup {
			group = child
			names = append(names, "group:"+child.Label)
			continue
		}
		names = append(names, child.Label)
	}
	want := []string{"Top", "group:Group Label", "Tail"}
	if len(names) != len(want) {
		t.Fatalf("combo children = %v, want %v (the group's options nest inside it)", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("child %d = %q, want %q", i, names[i], want[i])
		}
	}
	if group == nil {
		t.Fatal("no group node for the optgroup")
	}
	var groupLabels []string
	for _, child := range group.Children {
		groupLabels = append(groupLabels, child.Label)
	}
	if len(groupLabels) != 2 || groupLabels[0] != "In one" || groupLabels[1] != "In two" {
		t.Errorf("group children = %v, want [In one In two]", groupLabels)
	}
	if engine.SelectedOption(s.Doc.ElementByID("sel")).GetAttribute("id") != "in2" {
		t.Error("the selected option inside the group is not the one the tree marks")
	}
}
