package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

// qc:BR-106
func TestEncodeFormPlainText(t *testing.T) {
	data := []engine.FormData{
		{Name: "who", Value: "Ada Lovelace"},
		{Name: "note", Value: "line1\nline2"},
		{Name: "raw", Value: "a=b&c"},
		{Name: "empty", Value: ""},
	}
	got := engine.EncodeFormPlainText(data)
	// text/plain performs no escaping: '=' and '&' pass through verbatim and
	// every entry ends in CRLF, with lone line feeds normalized to CRLF.
	want := "who=Ada Lovelace\r\nnote=line1\r\nline2\r\nraw=a=b&c\r\nempty=\r\n"
	if got != want {
		t.Fatalf("text/plain body = %q, want %q", got, want)
	}
}

// qc:BR-106
func TestFormEnctypeForSubmitter(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.NewElement("form")
	form.SetAttribute("enctype", "text/plain")

	urlencoded := doc.NewElement("button")
	urlencoded.SetAttribute("type", "submit")
	urlencoded.SetAttribute("formenctype", "application/x-www-form-urlencoded")
	if got := engine.FormEnctypeFor(form, urlencoded); got != "application/x-www-form-urlencoded" {
		t.Fatalf("formenctype override = %q, want application/x-www-form-urlencoded", got)
	}

	plain := doc.NewElement("button")
	plain.SetAttribute("type", "submit")
	plain.SetAttribute("formenctype", "text/plain")
	if got := engine.FormEnctypeFor(form, plain); got != "text/plain" {
		t.Fatalf("formenctype text/plain = %q, want text/plain", got)
	}

	multipart := doc.NewElement("button")
	multipart.SetAttribute("type", "submit")
	multipart.SetAttribute("formenctype", "multipart/form-data")
	if got := engine.FormEnctypeFor(form, multipart); got != "multipart/form-data" {
		t.Fatalf("formenctype multipart = %q, want multipart/form-data", got)
	}

	// An unknown formenctype falls back to the form's enctype, as does a
	// button that is not a submitter.
	bogus := doc.NewElement("button")
	bogus.SetAttribute("type", "submit")
	bogus.SetAttribute("formenctype", "application/json")
	if got := engine.FormEnctypeFor(form, bogus); got != "text/plain" {
		t.Fatalf("invalid formenctype = %q, want the form's text/plain", got)
	}

	plainButton := doc.NewElement("button")
	plainButton.SetAttribute("type", "button")
	plainButton.SetAttribute("formenctype", "application/x-www-form-urlencoded")
	if got := engine.FormEnctypeFor(form, plainButton); got != "text/plain" {
		t.Fatalf("non-submit button formenctype = %q, want the form's text/plain", got)
	}

	if got := engine.FormEnctypeFor(form, nil); got != "text/plain" {
		t.Fatalf("no submitter enctype = %q, want the form's text/plain", got)
	}
}

// qc:BR-106
func TestSubmitFormTextPlainEncodesBodyAndEnctype(t *testing.T) {
	s := newFormSession(t, `<form id="f" action="/draft" method="post" enctype="text/plain">
		<input name="who" value="Ada Lovelace">
		<input name="note" value="line1&#10;line2">
		<button id="send" type="submit" name="go" value="1">Save</button>
	</form>`)
	form := s.Doc.ElementByID("f")
	s.SubmitForm(form, s.Doc.ElementByID("send"))
	sub := popForm(t, s)
	if sub.Method != "POST" {
		t.Fatalf("method = %q, want POST", sub.Method)
	}
	if sub.Enctype != "text/plain" {
		t.Fatalf("enctype = %q, want text/plain", sub.Enctype)
	}
	want := "who=Ada Lovelace\r\nnote=line1\r\nline2\r\ngo=1\r\n"
	if sub.Encoded != want {
		t.Fatalf("body = %q, want %q", sub.Encoded, want)
	}
}

// qc:BR-106
func TestSubmitFormGetIgnoresTextPlainEnctype(t *testing.T) {
	s := newFormSession(t, `<form id="f" action="/search" method="get" enctype="text/plain">
		<input name="q" value="Ada Lovelace">
	</form>`)
	form := s.Doc.ElementByID("f")
	s.SubmitForm(form, nil)
	sub := popForm(t, s)
	if sub.Method != "GET" {
		t.Fatalf("method = %q, want GET", sub.Method)
	}
	if sub.Enctype != "application/x-www-form-urlencoded" {
		t.Fatalf("GET enctype = %q, want urlencoded", sub.Enctype)
	}
	if sub.Encoded != "q=Ada+Lovelace" {
		t.Fatalf("GET payload = %q, want q=Ada+Lovelace", sub.Encoded)
	}
}

// qc:BR-106
func TestSubmitterFormenctypeChoosesBodyEncoding(t *testing.T) {
	s := newFormSession(t, `<form id="f" action="/send" method="post">
		<input name="q" value="a b">
		<button id="plain" type="submit" formenctype="text/plain">Plain</button>
	</form>`)
	form := s.Doc.ElementByID("f")
	s.SubmitForm(form, s.Doc.ElementByID("plain"))
	sub := popForm(t, s)
	if sub.Enctype != "text/plain" {
		t.Fatalf("enctype = %q, want text/plain from the submitter", sub.Enctype)
	}
	if sub.Encoded != "q=a b\r\n" {
		t.Fatalf("body = %q, want q=a b\\r\\n", sub.Encoded)
	}
}

// qc:BR-106
func TestUnsuppportedMultipartFallsBackToURLEncoded(t *testing.T) {
	s := newFormSession(t, `<form id="f" action="/send" method="post" enctype="multipart/form-data">
		<input name="q" value="a b">
	</form>`)
	form := s.Doc.ElementByID("f")
	s.SubmitForm(form, nil)
	sub := popForm(t, s)
	if sub.Enctype != "application/x-www-form-urlencoded" {
		t.Fatalf("enctype = %q, want the urlencoded fallback", sub.Enctype)
	}
	if sub.Encoded != "q=a+b" {
		t.Fatalf("body = %q, want q=a+b", sub.Encoded)
	}
}
