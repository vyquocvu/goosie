package engine_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

func propSet(n *ax.Node) []string {
	out := make([]string, 0, len(n.Props))
	for _, p := range n.Props {
		if p.Name == "activedescendant" {
			continue
		}
		out = append(out, p.Name+"="+p.Value)
	}
	sort.Strings(out)
	return out
}

func optionProps(control *ax.Node) []string {
	var out []string
	var walk func([]ax.Node)
	walk = func(list []ax.Node) {
		for i := range list {
			if list[i].Role == ax.RoleOption {
				out = append(out, list[i].Label+": "+strings.Join(propSet(&list[i]), " "))
			}
			walk(list[i].Children)
		}
	}
	walk(control.Children)
	return out
}

func activeDescendant(control *ax.Node) string {
	names := make(map[uint64]string)
	var index func([]ax.Node)
	index = func(list []ax.Node) {
		for i := range list {
			names[list[i].DOMNodeID] = list[i].Label
			index(list[i].Children)
		}
	}
	index(control.Children)
	found := ""
	var walk func(*ax.Node)
	walk = func(n *ax.Node) {
		for _, p := range n.Props {
			if p.Name == "activedescendant" && p.Ref != 0 {
				found = names[p.Ref]
			}
		}
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(control)
	return found
}

func selectIn(t *testing.T, s *engine.Session) *ax.Node {
	t.Helper()
	nodes := axFlatten(s.AccessibilityTree())
	var found []*ax.Node
	for i := range nodes {
		if nodes[i].Role == ax.RoleComboBox || nodes[i].Role == ax.RoleListBox {
			found = append(found, &nodes[i])
		}
	}
	if len(found) != 1 {
		t.Fatalf("select control nodes = %d, want 1", len(found))
	}
	return found[0]
}

func focusedSelect(t *testing.T, html string) *ax.Node {
	t.Helper()
	s := newFocusSession(t, html)
	w := s.Doc.ElementByID("w")
	if w == nil {
		t.Fatalf("no select#w in %s", html)
	}
	if !s.FocusNode(w) {
		t.Fatalf("select#w did not take focus in %s", html)
	}
	return selectIn(t, s)
}

func propsOf(t *testing.T, html string) []string {
	t.Helper()
	return propSet(axSelect(t, `<html><body style="margin:0">`+html+`</body></html>`))
}

func optionsOf(t *testing.T, html string) []string {
	t.Helper()
	return optionProps(axSelect(t, `<html><body style="margin:0">`+html+`</body></html>`))
}

func checkProps(t *testing.T, label string, got, want []string) {
	t.Helper()
	sorted := append([]string(nil), want...)
	sort.Strings(sorted)
	if strings.Join(got, " ") != strings.Join(sorted, " ") {
		t.Errorf("%s properties = [%s], want [%s]", label, strings.Join(got, " "), strings.Join(sorted, " "))
	}
}

// qc:BR-139
func TestAccessibilitySelectPropertiesFollowTheUsedDisplaySize(t *testing.T) {
	closed := []string{"expanded=false", "focusable=true", "hasPopup=menu", "invalid=false"}
	open := []string{"focusable=true", "invalid=false", "multiselectable=false",
		"orientation=vertical", "required=false"}
	cases := []struct {
		html string
		want []string
	}{
		{`<select id="w"><option value="a">Aye</option><option value="b" selected>Bee</option></select>`, closed},
		{`<select id="w" multiple size="1"><option value="a" selected>Aye</option><option value="b">Bee</option></select>`, closed},
		{`<select id="w" size="0"><option value="a" selected>Aye</option></select>`, closed},
		{`<select id="w"></select>`, closed},
		{`<select id="w" size="2"><option value="a" selected>Aye</option><option value="b">Bee</option></select>`, open},
		{`<select id="w" multiple><option value="a" selected>Aye</option><option value="b" selected>Bee</option></select>`,
			[]string{"focusable=true", "invalid=false", "multiselectable=true",
				"orientation=vertical", "required=false"}},
		{`<select id="w" multiple size="0"><option value="a" selected>Aye</option></select>`,
			[]string{"focusable=true", "invalid=false", "multiselectable=true",
				"orientation=vertical", "required=false"}},
	}
	for _, c := range cases {
		checkProps(t, c.html, propsOf(t, c.html), c.want)
	}
}

// qc:BR-139
func TestAccessibilityHasPopupFollowsOnlyAPopupKindToken(t *testing.T) {
	cases := []struct {
		token string
		want  string
	}{
		{"dialog", "dialog"},
		{"listbox", "listbox"},
		{"tree", "tree"},
		{"grid", "grid"},
		{"menu", "menu"},
		{"true", "menu"},
		{"false", "menu"},
		{"bogus", "menu"},
	}
	for _, c := range cases {
		html := `<select id="w" aria-haspopup="` + c.token + `"><option value="a" selected>Aye</option></select>`
		checkProps(t, "aria-haspopup="+c.token, propsOf(t, html),
			[]string{"expanded=false", "focusable=true", "hasPopup=" + c.want, "invalid=false"})
	}
}

// qc:BR-139
func TestAccessibilityOrientationIsAuthoredOnAClosedBoxAndDefaultedOnAnOpenOne(t *testing.T) {
	closed := []string{"expanded=false", "focusable=true", "hasPopup=menu", "invalid=false"}
	cases := []struct {
		token  string
		closed []string
		open   []string
	}{
		{"", closed, []string{"focusable=true", "invalid=false", "multiselectable=false",
			"orientation=vertical", "required=false"}},
		{"horizontal", append(append([]string(nil), closed...), "orientation=horizontal"),
			[]string{"focusable=true", "invalid=false", "multiselectable=false",
				"orientation=horizontal", "required=false"}},
		{"vertical", append(append([]string(nil), closed...), "orientation=vertical"),
			[]string{"focusable=true", "invalid=false", "multiselectable=false",
				"orientation=vertical", "required=false"}},
		{"bogus", closed, []string{"focusable=true", "invalid=false", "multiselectable=false",
			"orientation=vertical", "required=false"}},
	}
	for _, c := range cases {
		attr := ""
		if c.token != "" {
			attr = ` aria-orientation="` + c.token + `"`
		}
		checkProps(t, "closed aria-orientation="+c.token,
			propsOf(t, `<select id="w"`+attr+`><option value="a" selected>Aye</option></select>`), c.closed)
		checkProps(t, "open aria-orientation="+c.token,
			propsOf(t, `<select id="w" size="2"`+attr+`><option value="a" selected>Aye</option></select>`), c.open)
	}
}

// qc:BR-139
func TestAccessibilityClosedBoxStaysCollapsedWhateverAriaExpandedSays(t *testing.T) {
	for _, expanded := range []string{"true", "false"} {
		checkProps(t, "aria-expanded="+expanded,
			propsOf(t, `<select id="w" aria-expanded="`+expanded+`"><option value="a" selected>Aye</option></select>`),
			[]string{"expanded=false", "focusable=true", "hasPopup=menu", "invalid=false"})
	}
}

// qc:BR-139
func TestAccessibilityRequiredReachesOnlyTheOpenList(t *testing.T) {
	closed := []string{"expanded=false", "focusable=true", "hasPopup=menu"}
	openList := []string{"focusable=true", "multiselectable=false", "orientation=vertical"}
	cases := []struct {
		html string
		want []string
	}{
		{`<select id="w" required><option value="">Pick</option><option value="a">Aye</option></select>`,
			append(append([]string(nil), closed...), "invalid=true")},
		{`<select id="w" aria-required="true"><option value="a" selected>Aye</option></select>`,
			append(append([]string(nil), closed...), "invalid=false")},
		{`<select id="w" size="2" required><option value="">Pick</option><option value="a">Aye</option></select>`,
			append(append([]string(nil), openList...), "invalid=true", "required=true")},
		{`<select id="w" size="2" required><option value="">Pick</option><option value="a" selected>Aye</option></select>`,
			append(append([]string(nil), openList...), "invalid=false", "required=true")},
		{`<select id="w" size="2" aria-required="true"><option value="a" selected>Aye</option></select>`,
			append(append([]string(nil), openList...), "invalid=false", "required=true")},
		{`<select id="w" size="2" required aria-required="false"><option value="a" selected>Aye</option></select>`,
			append(append([]string(nil), openList...), "invalid=false", "required=true")},
		{`<select id="w" multiple required><option value="a">Aye</option><option value="b">Bee</option></select>`,
			[]string{"focusable=true", "invalid=true", "multiselectable=true",
				"orientation=vertical", "required=true"}},
		{`<select id="w" size="2"><option value="a" selected>Aye</option></select>`,
			append(append([]string(nil), openList...), "invalid=false", "required=false")},
	}
	for _, c := range cases {
		checkProps(t, c.html, propsOf(t, c.html), c.want)
	}
}

// qc:BR-139
func TestAccessibilityInvalidIsTheConstraintUnlessAriaSaysOtherwise(t *testing.T) {
	one := `<option value="a" selected>Aye</option>`
	missing := `<option value="">Pick</option><option value="a">Aye</option>`
	cases := []struct {
		label string
		html  string
		want  string
	}{
		{"a valid closed box", `<select id="w">` + one + `</select>`, "false"},
		{"a required box left on its placeholder", `<select id="w" required>` + missing + `</select>`, "true"},
		{"a required open list with nothing selected", `<select id="w" size="2" required>` + missing + `</select>`, "true"},
		{"aria-invalid=false over a failing constraint", `<select id="w" required aria-invalid="false">` + missing + `</select>`, "false"},
		{"an empty aria-invalid over a failing constraint", `<select id="w" required aria-invalid="">` + missing + `</select>`, "true"},
		{"an empty aria-invalid over a holding constraint", `<select id="w" aria-invalid="">` + one + `</select>`, "false"},
		{"aria-invalid=true over a holding constraint", `<select id="w" size="2" aria-invalid="true">` + one + `</select>`, "true"},
		{"an unusable aria-invalid token", `<select id="w" aria-invalid="bogus">` + one + `</select>`, "true"},
		{"aria-invalid=grammar", `<select id="w" size="2" aria-invalid="grammar">` + one + `</select>`, ""},
		{"aria-invalid=spelling", `<select id="w" size="2" aria-invalid="spelling">` + one + `</select>`, ""},
	}
	for _, c := range cases {
		got := propsOf(t, c.html)
		invalid := ""
		for _, p := range got {
			if strings.HasPrefix(p, "invalid=") {
				invalid = strings.TrimPrefix(p, "invalid=")
			}
		}
		if invalid != c.want {
			t.Errorf("%s answers invalid=%q, want %q in [%s]", c.label, invalid, c.want, strings.Join(got, " "))
		}
	}
}

// qc:BR-139
func TestAccessibilityCustomValidityMakesASelectInvalid(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0">`+
		`<select id="w"><option value="a" selected>Aye</option></select></body></html>`)
	if _, err := s.EvaluateJS(`document.getElementById("w").setCustomValidity("nope")`); err != nil {
		t.Fatalf("setCustomValidity: %v", err)
	}
	control := selectIn(t, s)
	invalid := ""
	for _, p := range propSet(control) {
		if strings.HasPrefix(p, "invalid=") {
			invalid = strings.TrimPrefix(p, "invalid=")
		}
	}
	if invalid != "true" {
		t.Errorf("a select with a custom error answers invalid=%q, want %q", invalid, "true")
	}
}

// qc:BR-139
func TestAccessibilityDisabledControlPrunesFocusableAndKeepsTheRest(t *testing.T) {
	rows := `<option value="a" selected>Aye</option><option value="b">Bee</option>`
	cases := []struct {
		label string
		html  string
		want  []string
		ad    string
	}{
		{"a disabled closed box", `<select id="w" disabled>` + rows + `</select>`,
			[]string{"disabled=true", "expanded=false", "hasPopup=menu", "invalid=false"}, ""},
		{"a closed box in a disabled fieldset", `<fieldset disabled><select id="w">` + rows + `</select></fieldset>`,
			[]string{"disabled=true", "expanded=false", "hasPopup=menu", "invalid=false"}, ""},
		{"a disabled open list", `<select id="w" size="2" disabled>` + rows + `</select>`,
			[]string{"disabled=true", "invalid=false", "multiselectable=false",
				"orientation=vertical", "required=false"}, "Aye"},
		{"a disabled open list with two selections", `<select id="w" multiple disabled>` +
			`<option value="a" selected>Aye</option><option value="b" selected>Bee</option></select>`,
			[]string{"disabled=true", "invalid=false", "multiselectable=true",
				"orientation=vertical", "required=false"}, "Aye"},
	}
	for _, c := range cases {
		control := axSelect(t, `<html><body style="margin:0">`+c.html+`</body></html>`)
		checkProps(t, c.label, propSet(control), c.want)
		if !control.Disabled {
			t.Errorf("%s: the control's disabled flag is false, want true", c.label)
		}
		if got := activeDescendant(control); got != c.ad {
			t.Errorf("%s: activedescendant names %q, want %q", c.label, got, c.ad)
		}
	}
}

// qc:BR-139
func TestAccessibilityOptionsOfADisabledControlKeepFocusableAndLoseSelected(t *testing.T) {
	rows := `<option value="a" selected>Aye</option><option value="b">Bee</option>`
	want := []string{"Aye: disabled=true focusable=true", "Bee: disabled=true focusable=true"}
	for _, html := range []string{
		`<select id="w" disabled>` + rows + `</select>`,
		`<fieldset disabled><select id="w">` + rows + `</select></fieldset>`,
		`<select id="w" size="2" disabled>` + rows + `</select>`,
	} {
		got := optionsOf(t, html)
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%s options = [%s], want [%s]", html, strings.Join(got, " "), strings.Join(want, " "))
		}
		s := newFocusSession(t, `<html><body style="margin:0">`+html+`</body></html>`)
		control := s.Doc.ElementByID("w")
		if !dom.IsFormControlDisabled(control) {
			t.Errorf("%s: the control is not disabled", html)
		}
		selected := engine.SelectedOption(control)
		if selected == nil || !selected.OptionSelected() {
			t.Errorf("%s: pruning the selected property unselected the option in the DOM", html)
		}
	}
}

// qc:BR-139
func TestAccessibilityDisabledOptionKeepsOnlyDisabled(t *testing.T) {
	control := axSelect(t, `<html><body style="margin:0">`+
		`<select id="w" size="2"><option value="a" selected disabled>Aye</option>`+
		`<option value="b">Bee</option></select></body></html>`)
	got := optionProps(control)
	want := []string{"Aye: disabled=true", "Bee: focusable=true selected=false"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("disabled option = [%s], want [%s]", strings.Join(got, " "), strings.Join(want, " "))
	}
	if ad := activeDescendant(control); ad != "Aye" {
		t.Errorf("activedescendant names %q, want the disabled selected option %q", ad, "Aye")
	}

	grouped := axSelect(t, `<html><body style="margin:0">`+
		`<select id="w" size="2"><optgroup label="G" disabled><option value="a" selected>Aye</option></optgroup>`+
		`<option value="b">Bee</option></select></body></html>`)
	got = optionProps(grouped)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("option in a disabled optgroup = [%s], want [%s]", strings.Join(got, " "), strings.Join(want, " "))
	}
	group := axFind(grouped.Children, ax.RoleGroup, "G")
	if group == nil {
		t.Fatalf("no group node named %q under the optgroup select: %+v", "G", grouped.Children)
	}
	checkProps(t, "optgroup group", propSet(group), nil)
}

// qc:BR-139
func TestAccessibilityEnabledOptionsAnswerSelectedFalseRatherThanNothing(t *testing.T) {
	got := optionsOf(t, `<select id="w" size="2"><option value="a" selected>Aye</option>`+
		`<option value="b">Bee</option><option value="c" disabled>Cee</option></select>`)
	want := []string{
		"Aye: focusable=true selected=true",
		"Bee: focusable=true selected=false",
		"Cee: disabled=true",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("options = [%s], want [%s]", strings.Join(got, " "), strings.Join(want, " "))
	}
}

// qc:BR-139
func TestAccessibilityActiveDescendantNamesTheFirstSelectionOfTheShapeThatHasOne(t *testing.T) {
	cases := []struct {
		label string
		html  string
		focus bool
		want  string
	}{
		{"an unfocused closed box", `<select id="w"><option value="a">Aye</option><option value="b" selected>Bee</option></select>`, false, ""},
		{"a focused closed box", `<select id="w"><option value="a">Aye</option><option value="b" selected>Bee</option></select>`, true, "Bee"},
		{"a focused closed box with no authored selection", `<select id="w"><option value="a">Aye</option><option value="b">Bee</option></select>`, true, "Aye"},
		{"a focused closed box whose only option is disabled", `<select id="w"><option value="a" disabled>Aye</option></select>`, true, ""},
		{"a focused empty closed box", `<select id="w"></select>`, true, ""},
		{"an open list with one selection", `<select id="w" size="2"><option value="a" selected>Aye</option><option value="b">Bee</option></select>`, false, "Aye"},
		{"an open list with two selections", `<select id="w" multiple><option value="a" selected>Aye</option><option value="b" selected>Bee</option></select>`, false, "Aye"},
		{"an open list with nothing selected", `<select id="w" size="2"><option value="a">Aye</option><option value="b">Bee</option></select>`, false, ""},
	}
	for _, c := range cases {
		html := `<html><body style="margin:0">` + c.html + `</body></html>`
		var control *ax.Node
		if c.focus {
			control = focusedSelect(t, html)
		} else {
			control = axSelect(t, html)
		}
		if got := activeDescendant(control); got != c.want {
			t.Errorf("%s: activedescendant names %q, want %q", c.label, got, c.want)
		}
		focused := false
		for _, p := range propSet(control) {
			if p == "focused=true" {
				focused = true
			}
		}
		if focused != c.focus {
			t.Errorf("%s: focused property = %v, want %v", c.label, focused, c.focus)
		}
		if !c.focus && control.Focused {
			t.Errorf("%s: the control's Focused flag is set without focus", c.label)
		}
	}
}

// qc:BR-139
func TestAccessibilityFocusedOpenListKeepsItsActivedescendantOnTheControl(t *testing.T) {
	control := focusedSelect(t, `<html><body style="margin:0">`+
		`<select id="w" size="2"><option value="a" selected>Aye</option><option value="b">Bee</option></select></body></html>`)
	checkProps(t, "a focused open list", propSet(control),
		[]string{"focusable=true", "focused=true", "invalid=false", "multiselectable=false",
			"orientation=vertical", "required=false"})
	if got := activeDescendant(control); got != "Aye" {
		t.Errorf("a focused open list names activedescendant %q, want %q", got, "Aye")
	}
	if !control.Focused {
		t.Errorf("the focused control's Focused flag is false, want true")
	}
}
