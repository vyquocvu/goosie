package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
)

// qc:BR-139
func TestAccessibilitySelectRoleFollowsTheUsedDisplaySize(t *testing.T) {
	cases := []struct {
		html string
		role ax.Role
	}{
		{`<select id="s"><option selected>Alpha</option></select>`, ax.RoleComboBox},
		{`<select id="s" size="1"><option selected>One</option></select>`, ax.RoleComboBox},
		{`<select id="s" size="0"><option selected>Zero</option></select>`, ax.RoleComboBox},
		{`<select id="s" size="less"><option selected>Bogus</option></select>`, ax.RoleComboBox},
		{`<select id="s" size="-1"><option selected>Minus</option></select>`, ax.RoleComboBox},
		{`<select id="s" multiple size="1"><option selected>Mm One</option></select>`, ax.RoleComboBox},
		{`<select id="s" size="2"><option selected>Eve Lyn</option><option>Five</option></select>`, ax.RoleListBox},
		{`<select id="s" multiple><option selected>Ex Aequo</option></select>`, ax.RoleListBox},
		{`<select id="s" multiple size="0"><option selected>Nn One</option></select>`, ax.RoleListBox},
		{`<select id="s" multiple size="less"><option selected>Rr One</option></select>`, ax.RoleListBox},
		{`<select id="s" multiple size="-1"><option selected>Tt One</option></select>`, ax.RoleListBox},
	}
	for _, c := range cases {
		control := axSelect(t, `<html><body style="margin:0">`+c.html+`</body></html>`)
		if control.Role != c.role {
			t.Errorf("%s announces %q, want %q (the used size decides it)", c.html, control.Role, c.role)
		}
	}
}

// qc:BR-139
func TestAccessibilityOpenSelectAnnouncesNoValueAndClosedOneTheDisplay(t *testing.T) {
	open := axSelect(t, `<html><body style="margin:0">`+
		`<select id="s" size="2"><option value="v4" selected>Eve Lyn</option><option value="v5">Five</option></select></body></html>`)
	if open.Role != ax.RoleListBox {
		t.Fatalf("size=2 role = %q, want listbox", open.Role)
	}
	if open.Value != "" {
		t.Errorf("open control value = %q, want %q (Chromium exposes none)", open.Value, "")
	}
	if got := optionNames(open); len(got) != 2 || got[0] != "Eve Lyn" || got[1] != "Five" {
		t.Errorf("open control options = %q, want [Eve Lyn Five]", got)
	}
	multi := axSelect(t, `<html><body style="margin:0">`+
		`<select id="s" multiple><option value="v6" selected>Ex Aequo</option><option value="v7">Seven</option></select></body></html>`)
	if multi.Role != ax.RoleListBox || multi.Value != "" {
		t.Errorf("multiple control = %q value %q, want listbox with no value", multi.Role, multi.Value)
	}
	if got := optionNames(multi); len(got) != 2 || got[0] != "Ex Aequo" || got[1] != "Seven" {
		t.Errorf("multiple control options = %q, want [Ex Aequo Seven]", got)
	}
	closed := axSelect(t, `<html><body style="margin:0">`+
		`<select id="s" multiple size="1"><option value="m1" selected>Mm One</option><option value="m2">Mm Two</option></select></body></html>`)
	if closed.Role != ax.RoleComboBox {
		t.Fatalf("multiple size=1 role = %q, want combobox", closed.Role)
	}
	if closed.Value != "Mm One" {
		t.Errorf("multiple size=1 value = %q, want the display string %q", closed.Value, "Mm One")
	}
}

// qc:BR-139
func TestAccessibilityOptionNodesAnnounceNoValue(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin:0"><form id="f"><select id="s" name="c">`+
		`<option value="v1" selected>Alpha Beta</option><option value="v2">Cee Dee</option>`+
		`<option>Eee Eff</option></select></form></body></html>`)
	control := axSelect(t, `<html><body style="margin:0">`+
		`<select id="s"><option value="v1" selected>Alpha Beta</option><option value="v2">Cee Dee</option>`+
		`<option>Eee Eff</option></select></body></html>`)
	for i, option := range control.Children {
		if option.Role != ax.RoleOption {
			t.Fatalf("child %d role = %q, want option", i, option.Role)
		}
		if option.Value != "" {
			t.Errorf("option %d (%q) announces value %q, want none at all", i, option.Label, option.Value)
		}
	}
	if got := submittedValue(t, s, "f", "c"); got != "v1" {
		t.Errorf("submitted value = %q, want %q: the form keeps answering even when the tree does not", got, "v1")
	}
}

// qc:BR-139
func TestAccessibilityMultipleSelectMarksEverySelectedOption(t *testing.T) {
	control := axSelect(t, `<html><body style="margin:0">`+
		`<select id="s" multiple><option value="a" selected>Aye</option>`+
		`<option value="b" selected>Bee</option><option value="c">Cee</option></select></body></html>`)
	want := []bool{true, true, false}
	if len(control.Children) != len(want) {
		t.Fatalf("option children = %d, want %d", len(control.Children), len(want))
	}
	for i, selected := range want {
		option := control.Children[i]
		if option.Label != []string{"Aye", "Bee", "Cee"}[i] {
			t.Errorf("option %d name = %q, want %q", i, option.Label, []string{"Aye", "Bee", "Cee"}[i])
		}
		if option.Selected != selected {
			t.Errorf("option %d (%q) selected = %v, want %v", i, option.Label, option.Selected, selected)
		}
	}
}

// qc:BR-139
func TestAccessibilityAuthoredListboxTokenAnnouncesListbox(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0">`+
		`<div id="w" role="listbox" aria-label="Authored listbox">`+
		`<div role="option" aria-label="Authored option">x</div></div></body></html>`)
	var control *ax.Node
	flat := axFlatten(s.AccessibilityTree())
	for i := range flat {
		if flat[i].ID == "w" {
			control = &flat[i]
			break
		}
	}
	if control == nil {
		t.Fatalf("authored listbox missing from the tree: %+v", flat)
	}
	if control.Role != ax.RoleListBox {
		t.Errorf("authored listbox role = %q, want listbox", control.Role)
	}
	if control.Label != "Authored listbox" {
		t.Errorf("authored listbox name = %q, want %q", control.Label, "Authored listbox")
	}
	if len(control.Children) != 1 || control.Children[0].Role != ax.RoleOption ||
		control.Children[0].Label != "Authored option" {
		t.Fatalf("authored listbox children = %+v, want the one authored option", control.Children)
	}
}
