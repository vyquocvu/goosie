package engine_test

import "testing"

// qc:BR-106
func TestSubmitterOverridesFormDestination(t *testing.T) {
	s := newFormSession(t, `<form id="f" action="/published" method="get">
		<input name="title" value="draft">
		<button id="save" type="submit" name="action" value="save" formaction="/saved" formmethod="post">Save</button>
		<button id="publish" type="submit" name="action" value="publish">Publish</button>
		<button id="blank" type="submit" name="action" value="blank" formaction="">Blank</button>
	</form>`)
	form := s.Doc.ElementByID("f")

	// The activated button's formaction/formmethod win over the form's.
	s.SubmitForm(form, s.Doc.ElementByID("save"))
	save := popForm(t, s)
	if save.Method != "POST" || save.Action != "/saved" {
		t.Fatalf("save submission = %s %q, want POST /saved", save.Method, save.Action)
	}
	if save.Encoded != "title=draft&action=save" {
		t.Fatalf("save payload = %q, want title=draft&action=save", save.Encoded)
	}

	// Without overrides the form's own attributes still decide.
	s.SubmitForm(form, s.Doc.ElementByID("publish"))
	publish := popForm(t, s)
	if publish.Method != "GET" || publish.Action != "/published" {
		t.Fatalf("publish submission = %s %q, want GET /published", publish.Method, publish.Action)
	}

	// An explicitly empty formaction overrides the form action with the
	// current document's URL, which the host represents as "".
	s.SubmitForm(form, s.Doc.ElementByID("blank"))
	blank := popForm(t, s)
	if blank.Action != "" {
		t.Fatalf("blank formaction = %q, want the empty action", blank.Action)
	}
}

func TestSubmitterMethodFallsBackWhenUnsupported(t *testing.T) {
	s := newFormSession(t, `<form id="f" action="/send" method="post"><input name="q" value="x"><button id="send" type="submit" formmethod="banana">Go</button></form>`)
	form := s.Doc.ElementByID("f")

	s.SubmitForm(form, s.Doc.ElementByID("send"))
	if got := popForm(t, s).Method; got != "POST" {
		t.Fatalf("unsupported formmethod produced %q, want the form's POST", got)
	}
}

func TestNonSubmitButtonDoesNotOverrideForm(t *testing.T) {
	s := newFormSession(t, `<form id="f" action="/send" method="get"><input name="q" value="x"><button id="plain" type="button" formaction="/other" formmethod="post">Plain</button><button id="send" type="submit">Go</button></form>`)
	form := s.Doc.ElementByID("f")

	s.SubmitForm(form, s.Doc.ElementByID("plain"))
	sub := popForm(t, s)
	if sub.Method != "GET" || sub.Action != "/send" {
		t.Fatalf("type=button submission = %s %q, want the form's GET /send", sub.Method, sub.Action)
	}
}

// qc:BR-105
func TestSubmitterNoValidateSkipsConstraintValidation(t *testing.T) {
	s := newFormSession(t, `<form id="f" action="/send"><input id="q" name="q" required><button id="skip" type="submit" name="skip" value="1" formaction="/draft" formnovalidate>Skip</button><button id="send" type="submit" name="go" value="1">Go</button></form>`)
	form, required := s.Doc.ElementByID("f"), s.Doc.ElementByID("q")

	// A normal submit is blocked by the empty required control.
	s.SubmitForm(form, s.Doc.ElementByID("send"))
	assertNoForm(t, s)
	if s.Focused() != required {
		t.Fatalf("focused node = %v, want the empty required control", s.Focused())
	}

	// The activated button's formnovalidate opts this submission out.
	s.SubmitForm(form, s.Doc.ElementByID("skip"))
	sub := popForm(t, s)
	if sub.Action != "/draft" || sub.Encoded != "q=&skip=1" {
		t.Fatalf("formnovalidate submission = %s %q, want /draft with q=&skip=1", sub.Action, sub.Encoded)
	}
}
