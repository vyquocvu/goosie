package js_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// These are roadmap gate 6, sub-project 1 as executable requirements. They are
// written before the runtime exists on purpose: the point of the round is to
// find out what the contract costs before paying for the interpreter, and a
// contract nobody tried to assert on is a guess.
//
// Everything here currently fails on behaviour, not on compilation - the
// package under test declares the API and returns js.ErrNotImplemented.

func newRuntime(t *testing.T, opts js.Options) *js.Runtime {
	t.Helper()
	r, err := js.New(opts)
	if err != nil {
		t.Fatalf("js.New(%+v): %v", opts, err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return r
}

func TestDynamicClassicScriptRunsThenFiresLoad(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	doc.Node.AppendChild(html)
	doc.HTML = html
	html.AppendChild(body)
	r := newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
	fetchCount := 0
	r.SetDynamicScriptFetcher(func(base, src string) ([]byte, error) {
		fetchCount++
		if base != "https://example.test/page" || src != "/dynamic.js" {
			t.Fatalf("dynamic fetch (%q, %q), want page URL and /dynamic.js", base, src)
		}
		return []byte(`window.dynamicRan = true`), nil
	})
	if err := r.Run(`
		window.dynamicRan = false;
		window.dynamicLoaded = false;
		var script = document.createElement('script');
		script.src = '/dynamic.js';
		script.addEventListener('load', function() {
			window.dynamicLoaded = window.dynamicRan;
		});
		document.body.appendChild(script);
		window.dynamicConnected = script.isConnected;
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fetchCount != 0 {
		t.Fatalf("dynamic fetch happened synchronously during Run (count %d)", fetchCount)
	}
	r.Tick()
	for _, key := range []string{"dynamicRan", "dynamicLoaded", "dynamicConnected"} {
		value, ok := r.Global(key)
		if !ok || value.Kind != js.KindBool || !value.Bool {
			t.Errorf("%s = %+v (present %v), want true (fetch count %d)", key, value, ok, fetchCount)
		}
	}
}

func TestDynamicInlineClassicScriptRunsOnConnection(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	doc.Node.AppendChild(html)
	doc.HTML = html
	doc.Body = body
	html.AppendChild(body)
	r := newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
	if err := r.Run(`
		window.inlineScriptRan = false;
		var script = document.createElement("script");
		script.textContent = "window.inlineScriptRan = true;";
		document.body.appendChild(script);
		if (!window.inlineScriptRan) throw new Error("connected inline script did not run synchronously");
	`, "inline-inserted-script.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestDynamicClassicScriptLimitIsBoundedPerRuntime(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	doc.Node.AppendChild(html)
	doc.HTML = html
	doc.Body = body
	html.AppendChild(body)
	r := newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
	r.SetDynamicScriptLimit(1)
	fetchCount := 0
	r.SetDynamicScriptFetcher(func(_, _ string) ([]byte, error) {
		fetchCount++
		return []byte(`window.firstScriptRan = true`), nil
	})
	if err := r.Run(`
		window.firstScriptRan = false;
		window.loadCount = 0;
		window.errorCount = 0;
		var first = document.createElement('script');
		first.src = '/first.js';
		first.onload = function() { window.loadCount++; };
		var second = document.createElement('script');
		second.src = '/second.js';
		second.onerror = function() { window.errorCount++; };
		document.body.append(first, second);
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	r.Tick()
	r.Tick()
	for key, want := range map[string]string{
		"firstScriptRan": "true", "loadCount": "1", "errorCount": "1",
	} {
		value, err := r.Evaluate(key)
		if err != nil || value != want {
			t.Errorf("%s = %q, err=%v, want %q", key, value, err, want)
		}
	}
	if fetchCount != 1 {
		t.Fatalf("fetch count = %d, want one admitted external script", fetchCount)
	}
}

// A negative timeout is a host that believes it configured a bound and did
// not. Silently running without one is the failure mode this rejects, so the
// constructor refuses it and the zero value means the default.
func TestNewRejectsNegativeTimeout(t *testing.T) {
	_, err := js.New(js.Options{Timeout: -time.Second})
	if err == nil {
		t.Fatalf("New accepted Timeout %v and would have run scripts unbounded", -time.Second)
	}
	// errors.Is alone would be satisfied by the unimplemented stub, so the test
	// would report green for a rule nothing enforces.
	if errors.Is(err, js.ErrNotImplemented) {
		t.Fatalf("New rejected every option, not the negative timeout: %v", err)
	}
}

// Two scripts in one document share one realm, and the second sees what the
// first left behind. This is the whole reason the engine can run <script> tags
// one at a time during a parse instead of concatenating the page.
func TestScriptsRunInDocumentOrderAgainstOneRealm(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/a"})

	steps := []string{
		`var trace = "first";`,
		`trace = trace + ",second";`,
		`trace = trace + ",third";`,
	}
	for i, src := range steps {
		if err := r.Run(src, "inline"); err != nil {
			t.Fatalf("script %d (%q): %v", i+1, src, err)
		}
	}
	v, ok := r.Global("trace")
	if !ok {
		t.Fatalf("global trace is not readable after three scripts set it")
	}
	if v.Kind != js.KindString {
		t.Errorf("trace has kind %v, want %v", v.Kind, js.KindString)
	}
	if v.Str != "first,second,third" {
		t.Errorf("trace = %q, want %q: scripts did not run in the order the parser saw them", v.Str, "first,second,third")
	}
}

// A computed value has to survive the call that produced it, or nothing a page
// does is observable from the host side.
func TestGlobalReadsBackANumber(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/a"})
	if err := r.Run(`var answer = 20 + 22;`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	v, ok := r.Global("answer")
	if !ok {
		t.Fatalf("global answer is missing")
	}
	if v.Kind != js.KindNumber || v.Num != 42 {
		t.Errorf("answer = %v %g, want %v 42", v.Kind, v.Num, js.KindNumber)
	}
}

// One bad script must not end the document: a page with a broken ad tag still
// has to render and still has to run the scripts after it. The error is
// reported, and it names the file and line that threw.
func TestThrowingScriptIsReportedAndDoesNotStopTheRuntime(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/a"})

	err := r.Run("throw new Error('boom');", "https://example.test/ad.js")
	if err == nil {
		t.Fatalf("an uncaught exception returned no error")
	}
	var jsErr *js.Error
	if !errors.As(err, &jsErr) {
		t.Fatalf("error is %T (%v), want *js.Error", err, err)
	}
	if jsErr.SourceURL != "https://example.test/ad.js" {
		t.Errorf("error blames source %q, want the script URL it came from", jsErr.SourceURL)
	}
	if jsErr.Line != 1 {
		t.Errorf("error blames line %d, want 1", jsErr.Line)
	}
	if !strings.Contains(jsErr.Message, "boom") {
		t.Errorf("error message %q drops the exception text", jsErr.Message)
	}
	if !strings.Contains(jsErr.Error(), "https://example.test/ad.js:1:") {
		t.Errorf("Error() = %q, want it to lead with source:line:", jsErr.Error())
	}

	if err := r.Run(`var afterError = "ran";`, "inline"); err != nil {
		t.Fatalf("Run after a throwing script: %v", err)
	}
	if v, ok := r.Global("afterError"); !ok || v.Str != "ran" {
		t.Errorf("global afterError = %+v %v, want %q: the realm died with the exception", v, ok, "ran")
	}
}

// A syntax error is a different failure from an exception - nothing executed at
// all - and it is treated the same way by the loader: reported, and the next
// script still runs.
func TestSyntaxErrorDoesNotPoisonLaterScripts(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/a"})
	if err := r.Run(`var = ;`, "https://example.test/broken.js"); err == nil {
		t.Fatalf("a syntax error returned no error")
	}
	if err := r.Run(`var survivor = 1;`, "inline"); err != nil {
		t.Fatalf("Run after a syntax error: %v", err)
	}
	if _, ok := r.Global("survivor"); !ok {
		t.Errorf("survivor was never set: the parse error took the realm down with it")
	}
}

// Source size is admitted before compilation, not charged to it: MaxScriptBytes
// exists so a page cannot make the engine build a multi-megabyte AST on a tab
// that is already at its document limit.
func TestOversizedScriptIsRefusedBeforeRunning(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/a"})

	src := "var pad = " + strings.Repeat("\"x\"", js.MaxScriptBytes) + ";"
	err := r.Run(src, "https://example.test/big.js")
	if !errors.Is(err, js.ErrTooLarge) {
		t.Fatalf("a %d byte script returned %v, want js.ErrTooLarge", len(src), err)
	}
	if _, ok := r.Global("pad"); ok {
		t.Errorf("the refused script still ran: pad is defined")
	}
}

// Close is the document teardown. A runtime whose page is gone must report that
// rather than execute into a realm nobody owns any more, which is the use-after
// free this API has to make impossible at the Go level.
func TestRunAfterCloseReportsErrClosed(t *testing.T) {
	r, err := js.New(js.Options{URL: "https://example.test/a"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := r.Run(`var x = 1;`, "inline"); !errors.Is(err, js.ErrClosed) {
		t.Errorf("Run after Close returned %v, want js.ErrClosed", err)
	}
}
