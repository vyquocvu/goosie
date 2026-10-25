package engine_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/userscript"
)

// GOOSIE-LIVE-001 is nine real pages that paint nothing and say nothing: rc 0,
// empty stderr, no field anywhere naming a reason. The reason existed - www.gov.vn
// throws 21 uncaught exceptions on load, the first one naming the page's
// jQuery bundle - and the engine discarded every one of
// them. Isolation is the behaviour under test first (a broken script must still
// not take the page down); recording is the new half.
// qc:BR-189
func TestUncaughtScriptErrorsAreRecordedAndIsolated(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	fetcher := func(base, src string) ([]byte, error) {
		// The shape measured on www.gov.vn: jQuery's saved document handle
		// stayed undefined past its setup gate, so the library's own line
		// threw "TypeError: Cannot read property 'createElement' of undefined".
		return []byte("var ce; ce.createElement(\"fieldset\");"), nil
	}
	html := `<html><body>
<script src="/jquery-3.6.0.min.js"></script>
<script>document.getElementById("missing").style.color = "red";</script>
<script>survived = 1;</script>
</body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt), engine.WithScripts("https://example.test/", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	// Isolation, unchanged: the last script still ran.
	if v, ok := rt.Global("survived"); !ok || v.Num != 1 {
		t.Fatalf("scripts after a throwing one did not run (survived=%+v): recording must not change control flow", v)
	}
	if n := sess.ScriptErrorCount(); n != 2 {
		t.Fatalf("ScriptErrorCount() = %d, want 2 (the bundle and the inline script)", n)
	}
	errs := sess.ScriptErrors()
	if len(errs) != 2 {
		t.Fatalf("ScriptErrors() = %d entries, want 2", len(errs))
	}
	// The source names the file, which is the whole point of recording: an
	// operator reading the line can go find it.
	if errs[0].Source != "https://example.test/jquery-3.6.0.min.js" {
		t.Errorf("first error source = %q, want the bundle URL it came from", errs[0].Source)
	}
	if !strings.Contains(errs[0].Message, "createElement") {
		t.Errorf("first error message = %q, want the property the page reached for", errs[0].Message)
	}
	if errs[1].Source != "inline" {
		t.Errorf("second error source = %q, want %q", errs[1].Source, "inline")
	}
	if line := errs[0].String(); !strings.HasPrefix(line, errs[0].Source+":") {
		t.Errorf("String() = %q, want it to lead with the source", line)
	}
	// goja appends a stack trace to these messages and the text is the page's
	// own. One error must stay one line, or a document writes extra report
	// lines and can forge the shape of the browser's own diagnostics.
	for i, e := range errs {
		if strings.ContainsAny(e.Message, "\n\r") {
			t.Fatalf("error %d message carries a newline (%q): a page must not be able to add report lines", i, e.Message)
		}
	}
}

// A page controls how often it throws and how long each message is, so neither
// may grow the session or the report line. This is the cap that keeps the new
// reporting safe against the input it reports on.
// qc:BR-189
func TestScriptErrorWindowAndMessageAreBounded(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	var sb strings.Builder
	sb.WriteString("<html><body>")
	const throws = 40
	for i := 0; i < throws; i++ {
		sb.WriteString(`<script>throw new Error("` + strings.Repeat("x", 4096) + `");</script>`)
	}
	sb.WriteString("</body></html>")

	sess, err := engine.NewSession(sb.String(), nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if n := sess.ScriptErrorCount(); n != throws {
		t.Fatalf("ScriptErrorCount() = %d, want the full %d: the window must not hide how often a page broke", n, throws)
	}
	errs := sess.ScriptErrors()
	if len(errs) != engine.MaxScriptErrors {
		t.Fatalf("retained %d errors, want the window capped at %d", len(errs), engine.MaxScriptErrors)
	}
	for i, e := range errs {
		if utf8.RuneCountInString(e.Message) > 300 {
			t.Fatalf("error %d message is %d runes, want a truncated one", i, utf8.RuneCountInString(e.Message))
		}
		if !utf8.ValidString(e.Message) {
			t.Fatalf("error %d message is not valid UTF-8; a report line must be", i)
		}
	}
}

// Silence means silence: a document that does not throw must record nothing and
// free the caller from printing anything. This is the case that keeps the new
// line off the stderr of every page that already works.
// qc:BR-189
func TestCleanDocumentRecordsNoScriptErrors(t *testing.T) {
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

	if n := sess.ScriptErrorCount(); n != 0 {
		t.Fatalf("ScriptErrorCount() = %d, want 0", n)
	}
	if errs := sess.ScriptErrors(); len(errs) != 0 {
		t.Fatalf("ScriptErrors() = %d entries, want none", len(errs))
	}
}

// The second call site of the recorder is the userscript stage, and it is the
// one an extension author sees: a broken userscript must name itself the same
// way a broken page script does, and must still not stop the userscripts after
// it. Without this test the userscript half of the recording is unobserved and
// can be removed without anything failing.
// qc:BR-189
func TestUserScriptErrorsAreRecordedAndLaterScriptsStillRun(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	scripts := []*userscript.Script{
		{Name: "broken", RunAt: userscript.RunAtDocumentEnd,
			Code: "var ce; ce.createElement(\"fieldset\");", Enabled: true},
		{Name: "later", RunAt: userscript.RunAtDocumentEnd,
			Code: "window.reachedLater = 1;", Enabled: true},
	}
	sess, err := engine.NewSession(`<html><body></body></html>`, nil, 800,
		engine.WithJS(rt), engine.WithUserScripts(scripts))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if v, ok := rt.Global("reachedLater"); !ok || v.Num != 1 {
		t.Fatalf("the userscript after the throwing one did not run (reachedLater=%+v): recording must not change control flow", v)
	}
	if n := sess.ScriptErrorCount(); n != 1 {
		t.Fatalf("ScriptErrorCount() = %d, want 1 (the broken userscript)", n)
	}
	errs := sess.ScriptErrors()
	if len(errs) != 1 {
		t.Fatalf("ScriptErrors() = %d entries, want 1", len(errs))
	}
	if errs[0].Source != "userscript:broken" {
		t.Errorf("source = %q, want %q: an extension author has to be able to tell which userscript broke",
			errs[0].Source, "userscript:broken")
	}
	if !strings.Contains(errs[0].Message, "createElement") {
		t.Errorf("message = %q, want it to name the property the userscript reached for", errs[0].Message)
	}
}
