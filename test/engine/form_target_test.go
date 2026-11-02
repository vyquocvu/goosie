package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// qc:BR-106
func TestFormTargetResolvesSubmitterOverride(t *testing.T) {
	s := newFormSession(t, `<form id="f" action="/send" target="_self">
		<button type="submit" id="blank" formtarget="_blank">Open</button>
		<button type="submit" id="self">Stay</button>
		<button type="button" id="plain" formtarget="_blank">Plain</button>
	</form>`)

	form := s.Doc.ElementByID("f")
	if got := engine.FormTargetFor(form, s.Doc.ElementByID("blank")); got != "_blank" {
		t.Fatalf("formtarget on submit button = %q, want _blank", got)
	}
	if got := engine.FormTargetFor(form, s.Doc.ElementByID("self")); got != "_self" {
		t.Fatalf("submit button without override = %q, want the form target _self", got)
	}
	if got := engine.FormTargetFor(form, s.Doc.ElementByID("plain")); got != "_self" {
		t.Fatalf("plain button must not override formtarget = %q, want _self", got)
	}
}

// qc:BR-106
func TestSubmitFormSubmissionCarriesTarget(t *testing.T) {
	s := newFormSession(t, `<form id="f" action="/send" method="POST" target="_blank">
		<input name="q" value="hi">
		<button type="submit" id="send">Send</button>
	</form>`)

	s.SubmitForm(s.Doc.ElementByID("f"), s.Doc.ElementByID("send"))
	sub := popForm(t, s)
	if sub.Target != "_blank" {
		t.Fatalf("submission target = %q, want _blank", sub.Target)
	}
	if sub.Method != "POST" || sub.Encoded != "q=hi" {
		t.Fatalf("submission = %+v, want POST q=hi", sub)
	}
}
