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
