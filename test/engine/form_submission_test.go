package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

func newFormSession(t *testing.T, html string) *engine.Session {
	t.Helper()
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	return s
}

func popForm(t *testing.T, s *engine.Session) *engine.FormSubmission {
	t.Helper()
	sub, ok := s.PopSubmittedForm()
	if !ok || sub == nil {
		t.Fatal("form submission not queued")
	}
	return sub
}

func assertNoForm(t *testing.T, s *engine.Session) {
	t.Helper()
	if sub, ok := s.PopSubmittedForm(); ok || sub != nil {
		t.Fatalf("invalid form queued submission: %#v", sub)
	}
}

// qc:BR-105
func TestFormRequiredValidation(t *testing.T) {
	s := newFormSession(t, `<form id="f" action="/send"><input id="required" name="q" required><input id="keep" name="other" value="intact"><button id="send" type="submit">go</button></form>`)
	form, required, keep := s.Doc.ElementByID("f"), s.Doc.ElementByID("required"), s.Doc.ElementByID("keep")
	invalidCount, submitCount := 0, 0
	dom.AddEventListener(required, "invalid", func(*dom.Event) { invalidCount++ }, false)
	dom.AddEventListener(form, "submit", func(*dom.Event) { submitCount++ }, false)
	s.SubmitForm(form, s.Doc.ElementByID("send"))
	assertNoForm(t, s)
	if invalidCount != 1 || submitCount != 0 {
		t.Fatalf("invalid events=%d, submit events=%d; want 1 and 0", invalidCount, submitCount)
	}
	if s.Focused() != required {
		t.Fatalf("focused node=%v, want first invalid control", s.Focused())
	}
	if keep.GetAttribute("value") != "intact" {
		t.Fatalf("other field value=%q, want preserved value intact", keep.GetAttribute("value"))
	}
	required.SetAttribute("value", "ready")
	s.SubmitForm(form, s.Doc.ElementByID("send"))
	sub := popForm(t, s)
	if invalidCount != 1 || submitCount != 1 {
		t.Fatalf("after valid submit invalid events=%d, submit events=%d; want 1 and 1", invalidCount, submitCount)
	}
	if sub.Encoded != "q=ready&other=intact" {
		t.Fatalf("payload=%q, want q=ready&other=intact", sub.Encoded)
	}
}

// qc:BR-105
func TestFormCheckboxRequiredValidation(t *testing.T) {
	s := newFormSession(t, `<form id="f"><input id="cb" type="checkbox" name="agree" required><button id="send" type="submit">go</button></form>`)
	form, cb := s.Doc.ElementByID("f"), s.Doc.ElementByID("cb")
	invalidCount := 0
	dom.AddEventListener(cb, "invalid", func(*dom.Event) { invalidCount++ }, false)
	s.SubmitForm(form, s.Doc.ElementByID("send"))
	assertNoForm(t, s)
	if invalidCount != 1 || s.Focused() != cb {
		t.Fatalf("invalid events=%d, focused=%v; want 1 and checkbox", invalidCount, s.Focused())
	}
	cb.SetAttribute("checked", "")
	s.SubmitForm(form, s.Doc.ElementByID("send"))
	if got := popForm(t, s).Encoded; got != "agree=on" {
		t.Fatalf("payload=%q, want agree=on", got)
	}
}

// qc:BR-105
func TestFormRadioRequiredValidation(t *testing.T) {
	s := newFormSession(t, `<form id="f"><input id="r1" type="radio" name="choice" required value="a"><input id="r2" type="radio" name="choice" value="b"><button id="send" type="submit">go</button></form>`)
	form, first, second := s.Doc.ElementByID("f"), s.Doc.ElementByID("r1"), s.Doc.ElementByID("r2")
	firstInvalid, secondInvalid := 0, 0
	dom.AddEventListener(first, "invalid", func(*dom.Event) { firstInvalid++ }, false)
	dom.AddEventListener(second, "invalid", func(*dom.Event) { secondInvalid++ }, false)
	s.SubmitForm(form, s.Doc.ElementByID("send"))
	assertNoForm(t, s)
	if firstInvalid != 1 || secondInvalid != 0 || s.Focused() != first {
		t.Fatalf("invalid events=(%d,%d), focused=%v; want (1,0) and first radio", firstInvalid, secondInvalid, s.Focused())
	}
	second.SetAttribute("checked", "")
	s.SubmitForm(form, s.Doc.ElementByID("send"))
	if got := popForm(t, s).Encoded; got != "choice=b" {
		t.Fatalf("payload=%q, want choice=b", got)
	}
}

// qc:BR-105
func TestFormValidationTypes(t *testing.T) {
	t.Run("email", func(t *testing.T) {
		s := newFormSession(t, `<form id="f"><input id="email" type="email" name="email" value="abc"><button type="submit">go</button></form>`)
		form, email := s.Doc.ElementByID("f"), s.Doc.ElementByID("email")
		s.SubmitForm(form, nil)
		assertNoForm(t, s)
		if s.Focused() != email {
			t.Fatal("invalid email did not receive focus")
		}
		email.SetAttribute("value", "")
		s.SubmitForm(form, nil)
		if got := popForm(t, s).Encoded; got != "email=" {
			t.Fatalf("empty optional email payload=%q, want email=", got)
		}
	})
	t.Run("number", func(t *testing.T) {
		s := newFormSession(t, `<form id="f"><input id="n" type="number" name="n" value="not-a-number"><button type="submit">go</button></form>`)
		form, number := s.Doc.ElementByID("f"), s.Doc.ElementByID("n")
		s.SubmitForm(form, nil)
		assertNoForm(t, s)
		if s.Focused() != number {
			t.Fatal("invalid number did not receive focus")
		}
		number.SetAttribute("value", "-12.5")
		s.SubmitForm(form, nil)
		if got := popForm(t, s).Encoded; got != "n=-12.5" {
			t.Fatalf("valid number payload=%q, want n=-12.5", got)
		}
	})
}

// qc:BR-105
func TestFormNovalidateBypass(t *testing.T) {
	s := newFormSession(t, `<form id="f" novalidate><input id="q" name="q" required><button type="submit">go</button></form>`)
	form, field := s.Doc.ElementByID("f"), s.Doc.ElementByID("q")
	invalidCount := 0
	dom.AddEventListener(field, "invalid", func(*dom.Event) { invalidCount++ }, false)
	s.SubmitForm(form, nil)
	if got := popForm(t, s).Encoded; got != "q=" {
		t.Fatalf("payload=%q, want q=", got)
	}
	if invalidCount != 0 {
		t.Fatalf("invalid events=%d with novalidate, want 0", invalidCount)
	}
}

// qc:BR-107
func TestFormSubmitterClickAndImplicit(t *testing.T) {
	t.Run("clicked submitter", func(t *testing.T) {
		s := newFormSession(t, `<body style="margin:0"><form id="f"><input name="q" value="x"><button id="save" type="submit" name="action" value="save">save</button><input name="tail" value="end"><button id="publish" type="submit" name="action" value="publish">publish</button></form></body>`)
		for _, tc := range []struct{ id, want string }{{"save", "q=x&action=save&tail=end"}, {"publish", "q=x&tail=end&action=publish"}} {
			x, y := clickPoint(t, s, tc.id)
			if !s.ClickAt(x, y) {
				t.Fatalf("ClickAt(%s) did not activate a control", tc.id)
			}
			if got := popForm(t, s).Encoded; got != tc.want {
				t.Fatalf("click %s payload=%q, want %q", tc.id, got, tc.want)
			}
		}
	})
	t.Run("implicit submitter", func(t *testing.T) {
		s := newFormSession(t, `<form id="f"><input id="q" name="q" value="x"><button type="submit" name="action" value="first">first</button><input type="submit" name="action" value="second"></form>`)
		input := s.Doc.ElementByID("q")
		if !s.FocusNode(input) || !s.Edit(engine.EditEnter, 0) {
			t.Fatal("Enter in text input did not submit")
		}
		if got := popForm(t, s).Encoded; got != "q=x&action=first" {
			t.Fatalf("implicit payload=%q, want q=x&action=first", got)
		}
	})
}

// qc:BR-107
func TestFormSubmitterWithoutNameOmitted(t *testing.T) {
	s := newFormSession(t, `<form id="f"><input name="q" value="x"><button id="send" type="submit" value="ignored">go</button></form>`)
	form := s.Doc.ElementByID("f")
	s.SubmitForm(form, s.Doc.ElementByID("send"))
	if got := popForm(t, s).Encoded; got != "q=x" {
		t.Fatalf("payload=%q, want q=x", got)
	}
}

// qc:BR-107
func TestFormButtonDefaultTypeSubmits(t *testing.T) {
	s := newFormSession(t, `<body style="margin:0"><form id="f"><input name="q" value="x"><button id="send" name="action" value="save">go</button></form></body>`)
	x, y := clickPoint(t, s, "send")
	if !s.ClickAt(x, y) {
		t.Fatal("click on button without type was not handled")
	}
	if got := popForm(t, s).Encoded; got != "q=x&action=save" {
		t.Fatalf("default button payload=%q, want q=x&action=save", got)
	}
}

// qc:BR-105
func TestFormTextareaSelectAndDialogValidation(t *testing.T) {
	t.Run("textarea and select", func(t *testing.T) {
		s := newFormSession(t, `<form id="f"><textarea id="ta" name="ta" required></textarea><select id="sel" name="sel" required><option value=""></option></select><button type="submit">go</button></form>`)
		form, ta, sel := s.Doc.ElementByID("f"), s.Doc.ElementByID("ta"), s.Doc.ElementByID("sel")
		taInvalid, selInvalid := 0, 0
		dom.AddEventListener(ta, "invalid", func(*dom.Event) { taInvalid++ }, false)
		dom.AddEventListener(sel, "invalid", func(*dom.Event) { selInvalid++ }, false)
		s.SubmitForm(form, nil)
		assertNoForm(t, s)
		if taInvalid != 1 || selInvalid != 1 || s.Focused() != ta {
			t.Fatalf("invalid events=(%d,%d), focused=%v; want (1,1) and textarea", taInvalid, selInvalid, s.Focused())
		}
	})
	t.Run("method dialog", func(t *testing.T) {
		s := newFormSession(t, `<dialog id="d" open><form id="f" method="dialog"><input id="q" name="q" required><button type="submit">go</button></form></dialog>`)
		dialog, form := s.Doc.ElementByID("d"), s.Doc.ElementByID("f")
		closeCount := 0
		dom.AddEventListener(dialog, "close", func(*dom.Event) { closeCount++ }, false)
		s.SubmitForm(form, nil)
		assertNoForm(t, s)
		if !dialog.HasAttribute("open") || closeCount != 0 {
			t.Fatalf("dialog open=%v close events=%d; want open and no close", dialog.HasAttribute("open"), closeCount)
		}
	})
}
