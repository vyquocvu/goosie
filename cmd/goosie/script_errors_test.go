package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/net"
)

// The sweep's report has one field for "what the engine said" and for nine real
// pages it was empty while the engine held 21 exceptions it had thrown away.
// reportScriptErrors is the place that line gets written, so that is where the
// contract is pinned: a page that broke names the script, a page that did not
// breaks the silence of its own stderr.
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

// The other half of the silence: a server that refuses the request answers with
// a small interstitial document, the pipeline renders it as any other page, and
// the run looks exactly like a page that is genuinely empty. The status is the
// only fact that tells the two apart, and it was recorded nowhere.
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

// A 200 answer is not a refusal. The ">= 400" decision lives at the call site
// in loadURLCtx, not in the helper, so it is pinned end to end in
// ./test/entrypoints against a server that answers both ways; re-implementing
// the threshold here would only test the test.
