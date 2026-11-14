package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
)

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
	innerVT := axSelect(t, "<html><body style=\"margin:0\"><select id=\"s\">"+
		"<option selected label=\"My\u000b\u000bLabel\">T</option></select></body></html>")
	if innerVT.Value != "My\u000b\u000bLabel" {
		t.Errorf("control value = %q, want the interior vertical tabs kept", innerVT.Value)
	}
}

// qc:BR-139
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
