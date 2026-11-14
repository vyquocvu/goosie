package js_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

const abortShape = `
var F = function (x) {
  return x === undefined ? 'undefined' : (x === null ? 'null' : String(x));
};
var SHAPE = function (v) {
  var empty = v === null || v === undefined;
  return [
    F(typeof v),
    F(v instanceof DOMException),
    F(v instanceof Error),
    F(v instanceof TypeError),
    F(empty ? v : v.name),
    F(empty ? v : v.message),
    F(empty ? v : v.code),
    F(String(v))
  ].join('|');
};
`

func abortMidFlight(t *testing.T, abortArgs string) string {
	t.Helper()
	gf := &gatedFetcher{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
		resp:    &js.FetchResponse{Status: 200, OK: true, Body: []byte("late"), URL: "https://example.test/slow"},
	}
	r := newFetchRuntime(t, gf)
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(gf.release) }) })

	script := abortShape + `
		var reasonShape = 'pending';
		var shapeController = new AbortController();
		fetch('/slow', {signal: shapeController.signal}).then(
			function () { reasonShape = 'resolved'; },
			function (err) { reasonShape = SHAPE(err); }
		);
	`
	if err := r.Run(script, "inline"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gf.started:
	case <-time.After(2 * time.Second):
		t.Fatal("fetch never reached the HTTP client")
	}
	if err := r.Run("shapeController.abort("+abortArgs+");", "inline"); err != nil {
		t.Fatal(err)
	}
	waitAbortedDrain(t, r)
	v, ok := r.Global("reasonShape")
	if !ok {
		t.Fatal("no rejection value was delivered to the page")
	}
	return v.Str
}

// qc:BR-123
func TestFetchAbortDefaultRejectsAsADOMException(t *testing.T) {
	want := "object|true|true|false|AbortError|signal is aborted without reason|20|AbortError: signal is aborted without reason"
	if got := abortMidFlight(t, ""); got != want {
		t.Errorf("default abort rejection = %q, want %q as Chromium measured it", got, want)
	}
}

// qc:BR-123
func TestFetchAbortObjectReasonRejectsWithTheSameValue(t *testing.T) {
	got := abortMidFlight(t, "({code: 7})")
	want := "object|false|false|false|undefined|undefined|7|[object Object]"
	if got != want {
		t.Errorf("object reason rejection = %q, want %q (same object, code kept)", got, want)
	}
	if !strings.HasSuffix(got, "|7|[object Object]") {
		t.Errorf("object reason lost its properties: %q", got)
	}
}

// qc:BR-123
func TestFetchAbortErrorReasonStaysAnError(t *testing.T) {
	got := abortMidFlight(t, "new Error('boom')")
	want := "object|false|true|false|Error|boom|undefined|Error: boom"
	if got != want {
		t.Errorf("Error reason rejection = %q, want %q", got, want)
	}
}

// qc:BR-123
func TestFetchAbortPrimitiveAndNullReasonsKeepTheirType(t *testing.T) {
	cases := []struct {
		args string
		want string
	}{
		{"'stopped'", "string|false|false|false|undefined|undefined|undefined|stopped"},
		{"0", "number|false|false|false|undefined|undefined|undefined|0"},
		{"null", "object|false|false|false|null|null|null|null"},
	}
	for _, c := range cases {
		got := abortMidFlight(t, c.args)
		if got != c.want {
			t.Errorf("abort(%s) rejection = %q, want %q", c.args, got, c.want)
		}
	}
}

// qc:BR-123
func TestFetchAbortUndefinedReasonFallsBackToTheDefault(t *testing.T) {
	want := "object|true|true|false|AbortError|signal is aborted without reason|20|AbortError: signal is aborted without reason"
	if got := abortMidFlight(t, "undefined"); got != want {
		t.Errorf("abort(undefined) rejection = %q, want %q (Chromium: the default AbortError)", got, want)
	}
}

// qc:BR-123
func TestFetchNonSignalValueRejectsWithTypeErrorAndNeverRequests(t *testing.T) {
	const wantMessage = "Failed to execute 'fetch' on 'Window': Failed to read the 'signal' " +
		"property from 'RequestInit': Failed to convert value to 'AbortSignal'."
	want := "object|false|true|true|TypeError|" + wantMessage + "|undefined|TypeError: " + wantMessage

	for _, init := range []string{"{signal: 'abc'}", "{signal: {}}", "{signal: 7}"} {
		m := &mockFetcher{responses: map[string]*js.FetchResponse{
			"https://example.test/plain": {Status: 200, StatusText: "OK", OK: true, Body: []byte("plain"), URL: "https://example.test/plain"},
		}}
		r := newFetchRuntime(t, m)
		script := abortShape + `
			var invalidSignalShape = 'pending';
			fetch('/plain', ` + init + `).then(
				function () { invalidSignalShape = 'resolved'; },
				function (err) { invalidSignalShape = SHAPE(err); }
			);
		`
		if err := r.Run(script, "inline"); err != nil {
			t.Fatalf("%s: %v", init, err)
		}
		drainFetch(t, r)
		v, ok := r.Global("invalidSignalShape")
		if !ok {
			t.Fatalf("%s: no rejection delivered", init)
		}
		if v.Str != want {
			t.Errorf("%s rejection = %q, want %q", init, v.Str, want)
		}
		if got := m.callCount(); got != 0 {
			t.Errorf("%s issued %d HTTP calls, want 0 (Chromium never started the request)", init, got)
		}
	}

	m := &mockFetcher{responses: map[string]*js.FetchResponse{}}
	r := newFetchRuntime(t, m)
	if err := r.Run(`
		var invalidSignalReturn = 'unset';
		try {
			var p = fetch('/plain', {signal: 'abc'});
			invalidSignalReturn = (p && typeof p.then === 'function') ? 'returns-thenable' : 'returned-non-thenable';
		} catch (e) { invalidSignalReturn = 'threw-sync'; }
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("invalidSignalReturn"); !ok || v.Str != "returns-thenable" {
		t.Errorf("fetch({signal:'abc'}) %q, want %q (a rejection, not a sync throw)", v.Str, "returns-thenable")
	}

	ok := &js.FetchResponse{Status: 200, OK: true, Body: []byte("plain"), URL: "https://example.test/plain"}
	m2 := &mockFetcher{responses: map[string]*js.FetchResponse{"https://example.test/plain": ok}}
	r2 := newFetchRuntime(t, m2)
	if err := r2.Run(`
		var nullSignalState = 'pending';
		fetch('/plain', {signal: null}).then(
			function (resp) { resp.text().then(function (b) { nullSignalState = 'resolved:' + b; }); },
			function (err) { nullSignalState = 'rejected:' + err; }
		);
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r2)
	if v, ok := r2.Global("nullSignalState"); !ok || v.Str != "resolved:plain" {
		t.Errorf("signal:null state = %q, want %q (Chromium fetches normally)", v.Str, "resolved:plain")
	}
	if got := m2.callCount(); got != 1 {
		t.Errorf("signal:null issued %d requests, want 1", got)
	}
}

// qc:BR-123
func TestAbortSignalReasonIsADOMException(t *testing.T) {
	r := newFetchRuntime(t, &mockFetcher{})
	if err := r.Run(abortShape+`
		var liveController = new AbortController();
		var reasonBefore = SHAPE(liveController.signal.reason);
		liveController.abort();
		var reasonAfter = SHAPE(liveController.signal.reason);
		var timedOut = AbortSignal.abort();
		var abortStatic = SHAPE(timedOut.reason);
		var custom = new AbortController();
		custom.abort('mine');
		var reasonCustom = SHAPE(custom.signal.reason);
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ global, want string }{
		{"reasonBefore", "undefined|false|false|false|undefined|undefined|undefined|undefined"},
		{"reasonAfter", "object|true|true|false|AbortError|signal is aborted without reason|20|AbortError: signal is aborted without reason"},
		{"abortStatic", "object|true|true|false|AbortError|signal is aborted without reason|20|AbortError: signal is aborted without reason"},
		{"reasonCustom", "string|false|false|false|undefined|undefined|undefined|mine"},
	}
	for _, c := range cases {
		v, ok := r.Global(c.global)
		if !ok {
			t.Fatalf("%s: not set", c.global)
		}
		if v.Str != c.want {
			t.Errorf("%s = %q, want %q (measured Chromium)", c.global, v.Str, c.want)
		}
	}
}
