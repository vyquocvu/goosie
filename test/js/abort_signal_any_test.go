package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

func anyRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

// qc:BR-293
func TestAbortSignalAnyForwardsFirstAbortedSignalReasonAndEvent(t *testing.T) {
	r := anyRuntime(t)
	got, err := r.Evaluate(`
		const c1 = new AbortController(), c2 = new AbortController();
		const out = AbortSignal.any([c1.signal, c2.signal]);
		let fired = 0;
		out.addEventListener("abort", function () { fired++; });
		const before = out.aborted;
		c2.abort("stop-reason");
		const afterSecond = out.reason;
		c1.abort("second-never-wins");
		JSON.stringify({ before: before, aborted: out.aborted, afterSecond: afterSecond, fired: fired, finalReason: out.reason });
	`)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	want := `{"before":false,"aborted":true,"afterSecond":"stop-reason","fired":1,"finalReason":"stop-reason"}`
	if got != want {
		t.Errorf("any composite = %s, want %s", got, want)
	}
}

// qc:BR-293
func TestAbortSignalAnyIsAbortedUpFrontWhenAnInputAlreadyAborted(t *testing.T) {
	r := anyRuntime(t)
	got, err := r.Evaluate(`
		const c = new AbortController(); c.abort("early");
		const d = new AbortController();
		const out = AbortSignal.any([d.signal, c.signal]);
		JSON.stringify({ aborted: out.aborted, reason: out.reason });
	`)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	want := `{"aborted":true,"reason":"early"}`
	if got != want {
		t.Errorf("any already-aborted = %s, want %s", got, want)
	}
}

// qc:BR-293
func TestAbortSignalAnyEmptyListAndBadArgument(t *testing.T) {
	r := anyRuntime(t)
	got, err := r.Evaluate(`
		const empty = AbortSignal.any([]);
		let elementErr = "no-throw";
		try { AbortSignal.any([1]); } catch (e) { elementErr = e.name; }
		let zeroErr = "no-throw";
		try { AbortSignal.any(); } catch (e) { zeroErr = e.name; }
		JSON.stringify({ emptyAborted: empty.aborted, elementErr: elementErr, zeroErr: zeroErr });
	`)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	want := `{"emptyAborted":false,"elementErr":"TypeError","zeroErr":"TypeError"}`
	if got != want {
		t.Errorf("any empty/bad = %s, want %s", got, want)
	}
}

// qc:BR-293
func TestAbortSignalAnyComposesWithTimeoutSignal(t *testing.T) {
	r := anyRuntime(t)
	got, err := r.Evaluate(`
		const c = new AbortController();
		const t = AbortSignal.timeout(1000000);
		const out = AbortSignal.any([c.signal, t]);
		c.abort();
		JSON.stringify({ aborted: out.aborted, reasonName: out.reason.name });
	`)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	want := `{"aborted":true,"reasonName":"AbortError"}`
	if got != want {
		t.Errorf("any with timeout = %s, want %s", got, want)
	}
}
