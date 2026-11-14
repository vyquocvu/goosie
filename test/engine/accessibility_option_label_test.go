package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/engine"
)

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
	if got := combo.Children[0].Value; got != "" {
		t.Errorf("option value = %q, want none: scripts/verify-optionax-roles.js reads option nodes carry no value", got)
	}
}

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
