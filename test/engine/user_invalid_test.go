package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func newUserInvalidSession(t *testing.T, html string) *engine.Session {
	t.Helper()
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func matchesPseudo(t *testing.T, s *engine.Session, id, pseudo string) bool {
	t.Helper()
	got, err := s.EvaluateJS(`document.getElementById("` + id + `").matches("` + pseudo + `")`)
	if err != nil {
		t.Fatalf("matches(%s) on %s: %v", pseudo, id, err)
	}
	return got == "true"
}

// qc:BR-105
// TestUserInvalidWaitsForFailedSubmission pins the difference between :invalid
// and :user-invalid: the field is invalid from the start, but only an attempted
// submission turns that into a state author CSS may style as user-facing.
func TestUserInvalidWaitsForFailedSubmission(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="f" action="/send"><input id="q" name="q" required></form>`)
	form := s.Doc.ElementByID("f")

	if !matchesPseudo(t, s, "q", ":invalid") {
		t.Fatal("empty required control should match :invalid before any interaction")
	}
	if matchesPseudo(t, s, "q", ":user-invalid") {
		t.Fatal(":user-invalid matched before the user attempted a submission")
	}
	if matchesPseudo(t, s, "q", ":user-valid") {
		t.Fatal(":user-valid matched before any interaction")
	}

	s.SubmitForm(form, nil)
	assertNoForm(t, s)

	if !matchesPseudo(t, s, "q", ":user-invalid") {
		t.Fatal("blocked submission should mark the offending control :user-invalid")
	}
	if matchesPseudo(t, s, "q", ":user-valid") {
		t.Fatal("an invalid control must not also match :user-valid")
	}
	if !matchesPseudo(t, s, "f", ":user-invalid") {
		t.Fatal("the blocked form should match :user-invalid")
	}

	// Typing a value satisfies `required`; the user-facing state flips to valid
	// without another submission. The blocked submit already focused the field.
	if s.Focused() != s.Doc.ElementByID("q") {
		t.Fatal("the blocked submission should focus the offending control")
	}
	if !s.Edit(engine.EditRune, 'r') {
		t.Fatal("typing should update the input")
	}
	if matchesPseudo(t, s, "q", ":user-invalid") {
		t.Fatal("filling the required control should clear :user-invalid")
	}
	if !matchesPseudo(t, s, "q", ":user-valid") {
		t.Fatal("a corrected control should match :user-valid")
	}
	if !matchesPseudo(t, s, "f", ":user-valid") {
		t.Fatal("the corrected form should match :user-valid")
	}
}

// qc:BR-105
func TestUserInvalidStaysScopedToTheAttemptedForm(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="one" action="/one"><input id="a" name="a" required></form>
		<form id="two" action="/two"><input id="b" name="b" required></form>`)

	s.SubmitForm(s.Doc.ElementByID("one"), nil)
	assertNoForm(t, s)

	if !matchesPseudo(t, s, "a", ":user-invalid") {
		t.Fatal("the submitted form's control should be :user-invalid")
	}
	if matchesPseudo(t, s, "b", ":user-invalid") {
		t.Fatal("an untouched form's control must not become :user-invalid")
	}
	if matchesPseudo(t, s, "two", ":user-invalid") {
		t.Fatal("an untouched form must not become :user-invalid")
	}
}

// qc:BR-105
// TestUserInvalidFollowsBlur pins the other way a browser reveals validation:
// leaving an empty required control is already a user interaction, so the field
// becomes :user-invalid without a submission attempt.
func TestUserInvalidFollowsBlur(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="f" action="/send"><input id="q" name="q" required><input id="other" name="other"></form>`)
	q := s.Doc.ElementByID("q")
	other := s.Doc.ElementByID("other")

	if !s.FocusNode(q) {
		t.Fatal("the required control should accept focus")
	}
	if matchesPseudo(t, s, "q", ":user-invalid") {
		t.Fatal("a still-focused control must not become :user-invalid")
	}

	if !s.FocusNode(other) {
		t.Fatal("the second control should accept focus")
	}
	if !matchesPseudo(t, s, "q", ":user-invalid") {
		t.Fatal("leaving the empty required control should mark it :user-invalid")
	}
	if matchesPseudo(t, s, "q", ":user-valid") {
		t.Fatal("an invalid control must not also match :user-valid")
	}
	if !matchesPseudo(t, s, "f", ":user-invalid") {
		t.Fatal("the owning form should follow its invalid control")
	}
}

// qc:BR-105
func TestUserValidFollowsBlur(t *testing.T) {
	s := newUserInvalidSession(t, `<input id="q" name="q" required value="filled">`)
	q := s.Doc.ElementByID("q")

	if !s.FocusNode(q) || !s.BlurNode(q) {
		t.Fatal("the control should focus and blur")
	}
	if !matchesPseudo(t, s, "q", ":user-valid") {
		t.Fatal("leaving a satisfied control should mark it :user-valid")
	}
	if matchesPseudo(t, s, "q", ":user-invalid") {
		t.Fatal("a satisfied control must not match :user-invalid")
	}
}

// qc:BR-105
func TestBlurMarksOnlyTheBlurredControlsForm(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="one" action="/one"><input id="a" name="a" required></form>
		<form id="two" action="/two"><input id="b" name="b" required></form>`)

	if !s.FocusNode(s.Doc.ElementByID("a")) {
		t.Fatal("the first form's control should accept focus")
	}
	if !s.FocusNode(s.Doc.ElementByID("b")) {
		t.Fatal("the second form's control should accept focus")
	}
	if !matchesPseudo(t, s, "a", ":user-invalid") || !matchesPseudo(t, s, "one", ":user-invalid") {
		t.Fatal("blur should mark the control that lost focus and its form")
	}
	if matchesPseudo(t, s, "b", ":user-invalid") || matchesPseudo(t, s, "two", ":user-invalid") {
		t.Fatal("the receiving form and its focused control must stay untouched")
	}
}

// qc:BR-105
func TestBlurOfValidControlDoesNotMarkUntouchedErrorInSameForm(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="f" action="/send"><input id="empty" name="empty" required><input id="ok" name="ok" value="ready"></form>`)

	if !s.FocusNode(s.Doc.ElementByID("ok")) || !s.BlurNode(s.Doc.ElementByID("ok")) {
		t.Fatal("the satisfied control should focus and blur")
	}
	if !matchesPseudo(t, s, "ok", ":user-valid") {
		t.Fatal("the satisfied control should be :user-valid after blur")
	}
	if matchesPseudo(t, s, "empty", ":user-invalid") {
		t.Fatal("a control the user never visited must stay untouched")
	}
	if matchesPseudo(t, s, "f", ":user-invalid") {
		t.Fatal("blurring a valid control must not expose an untouched error")
	}
}

// qc:BR-105
func TestTabNavigationMarksUserInvalidOnBlur(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="f" action="/send"><input id="q" name="q" required><input id="other" name="other"></form>`)

	if !s.FocusNode(s.Doc.ElementByID("q")) {
		t.Fatal("the required control should accept focus")
	}
	s.FocusNext()
	if !matchesPseudo(t, s, "q", ":user-invalid") {
		t.Fatal("tabbing away should mark the control :user-invalid")
	}
}

// qc:BR-105
func TestShiftTabNavigationMarksUserInvalidOnBlur(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="f" action="/send"><input id="q" name="q" required><input id="other" name="other"></form>`)

	if !s.FocusNode(s.Doc.ElementByID("q")) {
		t.Fatal("the required control should accept focus")
	}
	s.FocusPrev()
	if !matchesPseudo(t, s, "q", ":user-invalid") {
		t.Fatal("shift-tabbing away should mark the control :user-invalid")
	}
}

// qc:BR-105
func TestFormNoValidateSubmissionDoesNotMarkUserInvalid(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="f" action="/send" novalidate><input id="q" name="q" required></form>`)
	form := s.Doc.ElementByID("f")

	s.SubmitForm(form, nil)
	if _, ok := s.PopSubmittedForm(); !ok {
		t.Fatal("novalidate form should submit despite the empty required control")
	}
	if matchesPseudo(t, s, "q", ":user-invalid") {
		t.Fatal("no constraint validation ran, so :user-invalid must stay false")
	}
}
