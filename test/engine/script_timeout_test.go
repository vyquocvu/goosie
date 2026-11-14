package engine_test

import (
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func newTimeoutRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	rt, err := js.New(js.Options{URL: "https://example.test/slow", Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

// qc:BR-167
func TestParserScriptTimeoutIsRecorded(t *testing.T) {
	html := `<html><body><p>before</p><script>while (true) {}</script><p>after</p></body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(newTimeoutRuntime(t)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })

	if got := sess.ScriptTimeoutCount(); got != 1 {
		t.Fatalf("ScriptTimeoutCount = %d, want 1", got)
	}
	if !sess.HasScriptTimeout() {
		t.Fatal("HasScriptTimeout = false after a parser script timed out")
	}
	var found bool
	for _, e := range sess.ScriptErrors() {
		if e.TimedOut {
			found = true
		}
	}
	if !found {
		t.Fatalf("ScriptErrors = %+v, want one TimedOut entry", sess.ScriptErrors())
	}
	if text := sess.Doc.Node.TextContent(); !strings.Contains(text, "after") {
		t.Fatalf("document text = %q, want parsing to continue past the stopped script", text)
	}
	if _, err := sess.PaintChecked(1); err != nil {
		t.Fatalf("PaintChecked after a script timeout: %v", err)
	}
}

// qc:BR-167
func TestTimerCallbackTimeoutIsRecordedByPump(t *testing.T) {
	html := `<html><body><script>setTimeout(function () { while (true) {} }, 0);</script></body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(newTimeoutRuntime(t)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })

	if sess.HasScriptTimeout() {
		t.Fatal("timeout reported before the timer callback ran")
	}
	sess.PumpJS(800)
	if got := sess.ScriptTimeoutCount(); got != 1 {
		t.Fatalf("ScriptTimeoutCount after pump = %d, want 1", got)
	}
	sess.PumpJS(800)
	if got := sess.ScriptTimeoutCount(); got != 1 {
		t.Fatalf("ScriptTimeoutCount after second pump = %d, want 1 (one stop counted once)", got)
	}
}

// qc:BR-167
func TestCleanDocumentHasNoScriptTimeout(t *testing.T) {
	html := `<html><body><script>document.title = "fast";</script></body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(newTimeoutRuntime(t)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })

	if sess.HasScriptTimeout() {
		t.Fatalf("clean document reported %d timeouts", sess.ScriptTimeoutCount())
	}
}
