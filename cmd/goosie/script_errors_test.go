package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

// The sweep's report has one field for "what the engine said" and for nine real
// pages it was empty while the engine held 21 exceptions it had thrown away.
// reportScriptErrors is the place that line gets written, so that is where the
// contract is pinned: a page that broke names the script, a page that did not
// breaks the silence of its own stderr.
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
