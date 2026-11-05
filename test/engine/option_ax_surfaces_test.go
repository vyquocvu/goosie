package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
)

// One option, two spellings: the same label attribute is collapsed to
// " My Label " in the option's name and only trimmed to "My   Label" in the
// control's value. Every string here is copied out of
// scripts/verify-optionax-surfaces.js (Chromium 153.0.8010.12, 124 checks, run
// 2026-11-05), which reads the tree over CDP - Accessibility.getFullAXTree is
// the only headless seam that sees the announced name - and checks the rules it
// encodes against Chromium's answer shape by shape.
//
//	AX edges+inner -> {"role":"combobox","name":"","value":"My   Label"}
//	                    {"role":"option","name":" My Label "}
//	AX three-spaces-> {"role":"combobox","name":""}   (no value at all)
//	                    {"role":"option","name":"  "}
//	AX vt-edges    -> combobox value "My" / option name "\u000bMy\u000b"
//	AX nbsp-edges  -> combobox value "\u00a0My\u00a0" / option name "\u00a0My\u00a0"
//	AX bare        -> {"role":"combobox","name":"","value":"Alpha Beta"} while
//	                    document.querySelector('#bare').value === "v1"
//	AX labeled     -> name "Author label", value "Gam Ma"
//	AX aria        -> name "Aria name", value "Delta"
//	AX size2, multi-> {"role":"listbox","name":""} - no value on an open control
//	AX nobody      -> {"role":"combobox","name":""}   (an empty select)

// axSelect returns the control node for a one-select or one-authored-list
// document, failing the test when the tree carries more or fewer. It accepts both
// roles because the reference answers one control with either: scripts/
// verify-optionax-roles.js measures listbox for every used size above 1.
func axSelect(t *testing.T, html string) *ax.Node {
	t.Helper()
	s := newFocusSession(t, html)
	nodes := axFlatten(s.AccessibilityTree())
	var found []*ax.Node
	for i := range nodes {
		if nodes[i].Role == ax.RoleComboBox || nodes[i].Role == ax.RoleListBox {
			found = append(found, &nodes[i])
		}
	}
	if len(found) != 1 {
		t.Fatalf("select control nodes = %d, want 1 in %s", len(found), html)
	}
	return found[0]
}

// optionNames lists the option nodes' names in tree order.
func optionNames(combo *ax.Node) []string {
	var out []string
	var walk func([]ax.Node)
	walk = func(nodes []ax.Node) {
		for _, n := range nodes {
			if n.Role == ax.RoleOption {
				out = append(out, n.Label)
			}
			walk(n.Children)
		}
	}
	walk(combo.Children)
	return out
}

// qc:BR-139
//
// The label attribute's whitespace is spelled two ways at once, and a single
// normaliser cannot produce both: the option keeps an edge space (" My Label ")
// while the control strips the edges and keeps the interior run verbatim
// ("My   Label"). Goosie used to answer "My Label" on both, which is neither.
func TestAccessibilityOptionNameAndControlValueSpellWhitespaceDifferently(t *testing.T) {
	combo := axSelect(t, `<html><body style="margin:0"><select id="s">`+
		`<option selected label="  My   Label  ">WWWWW</option></select></body></html>`)
	if combo.Value != "My   Label" {
		t.Errorf("control value = %q, want %q as Chromium spells it (three interior spaces, no edges)",
			combo.Value, "My   Label")
	}
	names := optionNames(combo)
	if len(names) != 1 || names[0] != " My Label " {
		t.Errorf("option name = %q, want %q (edge spaces kept, interior collapsed)", names, " My Label ")
	}
	// Interior runs are the whole difference: "A  B" stays two spaces on the
	// control and becomes one in the name.
	inner := axSelect(t, `<html><body style="margin:0"><select id="s">`+
		`<option selected label="A  B">T</option></select></body></html>`)
	if inner.Value != "A  B" {
		t.Errorf("control value = %q, want %q", inner.Value, "A  B")
	}
	if names := optionNames(inner); len(names) != 1 || names[0] != "A B" {
		t.Errorf("option name = %q, want %q", names, "A B")
	}
}

// qc:BR-139
//
// A whitespace-only label is not the absent one: it names the option with two
// spaces (one per edge run) and leaves the control with nothing to announce,
// which the reference reports as no value at all.
func TestAccessibilityWhitespaceOnlyLabelNamesTheOptionAndEmptyValue(t *testing.T) {
	for _, label := range []string{"   ", "  ", " "} {
		combo := axSelect(t, `<html><body style="margin:0"><select id="s">`+
			`<option selected label="`+label+`">WWWWW</option></select></body></html>`)
		if combo.Value != "" {
			t.Errorf("label=%q control value = %q, want the empty display Chromium reports as absent", label, combo.Value)
		}
		names := optionNames(combo)
		if len(names) != 1 || names[0] != "  " {
			t.Errorf("label=%q option name = %q, want \"  \" (one space per edge run)", label, names)
		}
	}
}

// qc:BR-139
//
// The two spellings differ by one byte class: U+000B is trimmed off the
// display string and survives in the name, while U+00A0 is in neither set.
// Quoted from the same run: value "My" for label="\u000bMy\u000b" against name
// "\u000bMy\u000b", and both surfaces answering "\u00a0My\u00a0".
func TestAccessibilityVerticalTabIsTrimmedOnlyOnTheDisplaySpelling(t *testing.T) {
	vt := axSelect(t, "<html><body style=\"margin:0\"><select id=\"s\">"+
		"<option selected label=\"\u000bMy\u000b\">T</option></select></body></html>")
	if vt.Value != "My" {
		t.Errorf("control value = %q, want %q", vt.Value, "My")
	}
	if names := optionNames(vt); len(names) != 1 || names[0] != "\u000bMy\u000b" {
		t.Errorf("option name = %q, want the vertical tabs kept", names)
	}
	nbsp := axSelect(t, "<html><body style=\"margin:0\"><select id=\"s\">"+
		"<option selected label=\"\u00a0My\u00a0\">T</option></select></body></html>")
	if nbsp.Value != "\u00a0My\u00a0" {
		t.Errorf("control value = %q, want U+00A0 untouched", nbsp.Value)
	}
	if names := optionNames(nbsp); len(names) != 1 || names[0] != "\u00a0My\u00a0" {
		t.Errorf("option name = %q, want U+00A0 untouched", names)
	}
	// An interior vertical tab survives on both surfaces, so the difference is
	// the edge set and not a general collapse.
	innerVT := axSelect(t, "<html><body style=\"margin:0\"><select id=\"s\">"+
		"<option selected label=\"My\u000b\u000bLabel\">T</option></select></body></html>")
	if innerVT.Value != "My\u000b\u000bLabel" {
		t.Errorf("control value = %q, want the interior vertical tabs kept", innerVT.Value)
	}
}

// qc:BR-139
//
// A closed control announces the display string, not the submitted value:
// measured {"role":"combobox","name":"","value":"Alpha Beta"} for
// <option value="v1" selected>Alpha Beta</option>, and its accessible name is
// empty because nothing in the document names it. The form still submits "v1",
// so the two answers are two surfaces - that is the reference's model, and the
// older expectation here (value == form value) was written from memory.
func TestAccessibilityComboBoxAnnouncesTheDisplayWhileTheFormSubmitsTheValue(t *testing.T) {
	combo := axSelect(t, `<html><body style="margin:0"><form id="f"><select id="s" name="c">`+
		`<option value="v1" selected>Alpha Beta</option><option value="v2">Cee Dee</option>`+
		`</select></form></body></html>`)
	if combo.Label != "" {
		t.Errorf("control name = %q, want %q: the selected option never names the control", combo.Label, "")
	}
	if combo.Value != "Alpha Beta" {
		t.Errorf("control value = %q, want the display string %q", combo.Value, "Alpha Beta")
	}
	s := newFocusSession(t, `<html><body style="margin:0"><form id="f"><select id="s" name="c">`+
		`<option value="v1" selected>Alpha Beta</option></select></form></body></html>`)
	if got := submittedValue(t, s, "f", "c"); got != "v1" {
		t.Errorf("submitted value = %q, want %q: the display spelling does not leak into the form", got, "v1")
	}
}

// qc:BR-139
//
// Who names the control, measured in the same run: a label[for] gives
// "Author label", aria-label gives "Aria name", and an author-less select gets
// nothing - the option text moves to the value in every case.
func TestAccessibilityControlNameComesFromTheAuthorOnly(t *testing.T) {
	cases := []struct{ html, name, value string }{
		{`<label for="s">Author label</label><select id="s"><option selected>Gam Ma</option></select>`, "Author label", "Gam Ma"},
		{`<select id="s" aria-label="Aria name"><option selected>Delta</option></select>`, "Aria name", "Delta"},
		{`<select id="s"><option selected>Delta</option></select>`, "", "Delta"},
	}
	for _, c := range cases {
		combo := axSelect(t, `<html><body style="margin:0">`+c.html+`</body></html>`)
		if combo.Label != c.name {
			t.Errorf("control name = %q, want %q for %s", combo.Label, c.name, c.html)
		}
		if combo.Value != c.value {
			t.Errorf("control value = %q, want %q for %s", combo.Value, c.value, c.html)
		}
	}
}
