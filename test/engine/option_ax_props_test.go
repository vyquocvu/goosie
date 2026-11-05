package engine_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

// What Chromium answers in the `properties` list of a select, its option nodes
// and its optgroup groups - and which of those keys disappear when the control or
// the option is disabled. Every expectation here is copied out of
// scripts/verify-optionax-props.js (Chromium 153.0.8010.12, 46 fixtures, 324
// checks, run 2026-11-06), which reads the tree over CDP and compares the FULL
// property set shape by shape, so a key the reference does not answer reddens the
// anchor rather than passing a subset test. These tests hold the same sets against
// Goosie's tree.
//
//	AX closed-enabled        -> {expanded:false, focusable:true, hasPopup:"menu", invalid:"false"}
//	AX open-single           -> {focusable:true, invalid:"false", multiselectable:false,
//	                             orientation:"vertical", required:false}
//	AX closed-disabled       -> {disabled:true, expanded:false, hasPopup:"menu", invalid:"false"}
//	AX open-disabled         -> {disabled:true, invalid:"false", multiselectable:false,
//	                             orientation:"vertical", required:false} + activedescendant KEPT
//	AX every disabled case   -> options answer {disabled:true, focusable:true} with NO selected
//	AX open-option-disabled  -> {disabled:true} alone, not even on the selected option
//	AX closed-required-empty -> invalid:"true" and NO required key at all
//	AX focused-closed-*      -> focused:true, and the activedescendant names the selection
//
// Two halves of the reference's answer stay outside these tests and are disclosed
// rather than guessed: the activedescendant of a CLOSED combobox hangs on a
// MenuListPopup container node that Goosie does not emit (so the property is
// asserted on the control, which is the only node there is), and the AppKit
// attribute each property would become needs a display host with VoiceOver. Both
// are GOOSIE-OPTIONAX-005 in docs/qa-open-defects.json.

// propSet renders a node's property list the way the anchor prints it: sorted
// "name=value" pairs, so a full-set comparison pins a key the reference does not
// answer as firmly as one it answers wrongly.
//
// The activedescendant is left out, exactly as the anchor's shape() lifts it out of
// the property list into its own check: it names a node rather than answering a
// state, and activeDescendant below asserts it separately.
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

// optionProps renders every option under a control as "name: prop prop", in
// document order. The walk is recursive so the same helper reads a listbox, whose
// options are direct children, and a closed combobox, whose options the reference
// hangs one container layer lower.
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

// activeDescendant returns the name the activedescendant of the control or of any
// node under it points at, and "" when nothing in that subtree answers one. The
// target is resolved through an index of the whole subtree because the option it
// names can sit one layer lower, inside an optgroup's group node.
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

// selectIn returns the control node of a session's tree, failing when the tree
// carries more or fewer than one. axSelect (option_ax_surfaces_test.go) is the
// same search over a fresh session; this one takes a session so a caller can focus
// the control or run script against it first.
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

// focusedSelect returns the control node of a one-select document whose select has
// focus, the state the reference answers with a `focused` property and - on a
// closed box - an activedescendant.
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

// checkProps compares a full property set, so a missing key and an extra one fail
// the same way: the reference prunes keys rather than answering them false.
func checkProps(t *testing.T, label string, got, want []string) {
	t.Helper()
	sorted := append([]string(nil), want...)
	sort.Strings(sorted)
	if strings.Join(got, " ") != strings.Join(sorted, " ") {
		t.Errorf("%s properties = [%s], want [%s]", label, strings.Join(got, " "), strings.Join(sorted, " "))
	}
}

// qc:BR-139
//
// The two shapes answer two disjoint property sets, and the closed box's set is
// the same whether or not `multiple` is present - the used display size decides,
// exactly as it decides the role. Measured closed-enabled /
// closed-multiple-size1 / open-single / open-multiple-two.
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
		// An empty select is still the closed widget: the set does not depend on
		// having anything to show.
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
//
// aria-haspopup replaces the native "menu" only when it names a kind of popup:
// measured dialog, listbox, tree and grid verbatim, while "true", "false" and an
// unusable token all leave the answer at "menu".
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
//
// A closed box has no orientation of its own - measured closed-enabled answers no
// orientation key at all - so only an authored token can add one, and an unusable
// token adds nothing. An open list defaults to "vertical" and an unusable token
// leaves it there.
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
//
// An author-set aria-expanded cannot open the native popup, so the closed box
// answers expanded:false either way - measured closed-aria-expanded-true.
func TestAccessibilityClosedBoxStaysCollapsedWhateverAriaExpandedSays(t *testing.T) {
	for _, expanded := range []string{"true", "false"} {
		checkProps(t, "aria-expanded="+expanded,
			propsOf(t, `<select id="w" aria-expanded="`+expanded+`"><option value="a" selected>Aye</option></select>`),
			[]string{"expanded=false", "focusable=true", "hasPopup=menu", "invalid=false"})
	}
}

// qc:BR-139
//
// `required` reaches the open list only: measured closed-required-empty and
// closed-aria-required answer no required key at all, while open-required answers
// required:true. aria-required can turn the key ON but "false" cannot turn the
// content attribute OFF (open-aria-required-false-over-attr answers required:true),
// so the rule is a disjunction of the two attributes.
func TestAccessibilityRequiredReachesOnlyTheOpenList(t *testing.T) {
	closed := []string{"expanded=false", "focusable=true", "hasPopup=menu"}
	openList := []string{"focusable=true", "multiselectable=false", "orientation=vertical"}
	cases := []struct {
		html string
		want []string
	}{
		// The placeholder option is selected, so the constraint fails and the
		// closed box answers invalid:"true" with no required key.
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
//
// `invalid` is a STRING, not a boolean, and aria-invalid overrides the constraint
// except for the two tokens that mean neither true nor false, which drop the key
// entirely. An EMPTY aria-invalid reads as absent rather than as "false": the same
// fixture answers "true" when the constraint fails and "false" when it holds
// (closed-aria-invalid-empty versus closed-aria-invalid-empty-valid).
func TestAccessibilityInvalidIsTheConstraintUnlessAriaSaysOtherwise(t *testing.T) {
	one := `<option value="a" selected>Aye</option>`
	missing := `<option value="">Pick</option><option value="a">Aye</option>`
	cases := []struct {
		label string
		html  string
		want  string // "" means the reference answers no invalid key at all
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
//
// A custom error is a constraint failure like any other: measured
// closed-custom-validity answers invalid:"true" for a select that would otherwise
// be valid. This case needs a JS runtime because setCustomValidity is script.
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
//
// A disabled control answers `disabled` and loses `focusable`, keeping every other
// key of its shape - and a disabled OPEN list keeps its activedescendant even
// though nothing in it can be focused any more. Measured closed-disabled,
// fieldset-disabled (a disabled fieldset is byte-identical to the attribute) and
// open-disabled.
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
//
// An enabled option inside a disabled control answers BOTH disabled and focusable
// and loses only `selected` - measured for the selected and the unselected row of
// closed-disabled, fieldset-disabled and open-disabled. Pruning `selected` is an
// accessibility answer only: the DOM's selectedness is untouched, which the same
// test asserts through engine.SelectedOption.
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
		// The property list answers what assistive technology hears, not what the
		// DOM holds: pruning `selected` must not unselect anything.
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
//
// A disabled option keeps nothing but `disabled` - no focusable, and no `selected`
// even when it is the selected one - while the control's activedescendant still
// names it. The same pruning reaches an option through a disabled optgroup.
// Measured open-option-disabled and open-optgroup-disabled.
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
	// A group node answers no properties in either shape: measured props {} for
	// closed-optgroup and open-optgroup-disabled alike.
	group := axFind(grouped.Children, ax.RoleGroup, "G")
	if group == nil {
		t.Fatalf("no group node named %q under the optgroup select: %+v", "G", grouped.Children)
	}
	checkProps(t, "optgroup group", propSet(group), nil)
}

// qc:BR-139
//
// An enabled option in an enabled control answers focusable plus `selected` with
// BOTH values - measured {focusable:true, selected:false} on the unselected rows,
// so the key is not pruned when it is false, unlike `disabled`.
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
//
// The activedescendant names a selection, and which node carries it and when it
// appears differs by shape: an open list answers it whenever something is
// selected, naming the FIRST selected option, while a closed box answers it only
// while focused. With nothing selected - no authored selection on an open list, or
// no enabled option for a closed box's default to land on - no node answers one.
// Measured open-single, open-multiple-two, open-nothing-selected,
// focused-closed-selected, focused-closed-default and focused-closed-all-disabled.
//
// The reference hangs a closed box's activedescendant on the MenuListPopup node
// between the control and its options; Goosie emits no such layer yet, so the
// property is read off the control (GOOSIE-OPTIONAX-005).
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
		// `focused` is answered only while the control has focus, and only on the
		// control: no option or group node reports it.
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
//
// A focused open list adds `focused` to the property set it already answers, and
// keeps its activedescendant on the control - measured focused-open.
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
