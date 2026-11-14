package engine_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/userscript"
)

// qc:BR-189 BR-091
func TestUncaughtScriptErrorsAreRecordedAndIsolated(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	fetcher := func(base, src string) ([]byte, error) {
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
	for i, e := range errs {
		if strings.ContainsAny(e.Message, "\n\r") {
			t.Fatalf("error %d message carries a newline (%q): a page must not be able to add report lines", i, e.Message)
		}
	}
}

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

// qc:BR-092
func TestScriptErrorFromRemovedNodeKeepsTheRuntimeAlive(t *testing.T) {
	html := `<html><body>
<div id="host"><span id="child">x</span></div>
<script>
var host = document.getElementById('host');
var child = document.getElementById('child');
host.removeChild(child);
child.bogusMethod();
</script>
<script>
setTimeout(function() { document.getElementById('host').textContent = 'alive'; }, 0);
</script>
</body></html>`
	sess, _, _ := pumpSession(t, html, true)
	if n := sess.ScriptErrorCount(); n != 1 {
		t.Fatalf("ScriptErrorCount() = %d, want 1 (the invalid call on the removed node)", n)
	}
	if !sess.PumpJS(800) {
		t.Fatal("PumpJS reported no work after a recovered script error")
	}
	if got := sess.Find("alive"); len(got) == 0 {
		t.Fatal("timer after the throwing script did not mutate the live document")
	}
}
