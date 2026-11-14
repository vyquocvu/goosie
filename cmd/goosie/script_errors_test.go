package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/net"
)

// qc:BR-190
func TestReportScriptErrorsNamesTheFailingScript(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	sess, err := engine.NewSession(
		`<html><body><script>var ce; ce.createElement("fieldset")</script></body></html>`,
		nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	var buf bytes.Buffer
	reportScriptErrors(&buf, "https://example.test/page", sess)
	got := buf.String()
	if got == "" {
		t.Fatal("a document whose script threw reported nothing: this is the silence GOOSIE-LIVE-001 is filed against")
	}
	if strings.Count(got, "\n") != 1 {
		t.Fatalf("report = %q, want exactly one line: a page's stack trace must not add lines to the report", got)
	}
	if !strings.HasPrefix(got, "goosie: ") || !strings.HasSuffix(got, "\n") {
		t.Fatalf("report line = %q, want one goosie-prefixed newline-terminated line", got)
	}
	if !strings.Contains(got, "1 uncaught script error") {
		t.Fatalf("report line = %q, want the count of errors the document produced", got)
	}
	if !strings.Contains(got, "inline") || !strings.Contains(got, "createElement") {
		t.Fatalf("report line = %q, want the source and the property the page reached for", got)
	}
	if !strings.Contains(got, "https://example.test/page") {
		t.Fatalf("report line = %q, want the document it came from", got)
	}
}

// qc:BR-190
func TestReportScriptErrorsIsSilentOnACleanDocument(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	sess, err := engine.NewSession(`<html><body><script>ok = 1;</script></body></html>`, nil, 800,
		engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	var buf bytes.Buffer
	reportScriptErrors(&buf, "https://example.test/clean", sess)
	if buf.Len() != 0 {
		t.Fatalf("a clean document wrote %q to stderr; every working page would start reporting", buf.String())
	}
}

// qc:BR-190
func TestReportRefusedDocumentNamesTheStatusAndTheBody(t *testing.T) {
	var buf bytes.Buffer
	reportRefusedDocument(&buf, &net.Response{
		StatusCode: 403,
		URL:        "https://apnews.com/",
		Body:       []byte(strings.Repeat("x", 5581)),
	})
	got := buf.String()
	if got == "" {
		t.Fatal("a 403 answer reported nothing: this is the silence GOOSIE-LIVE-001 is filed against")
	}
	if strings.Count(got, "\n") != 1 {
		t.Fatalf("report = %q, want exactly one line", got)
	}
	for _, want := range []string{"goosie: ", "https://apnews.com/", "HTTP 403", "5581 bytes"} {
		if !strings.Contains(got, want) {
			t.Errorf("report line = %q, want it to contain %q", got, want)
		}
	}
}
