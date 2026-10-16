package js_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

// Interruptibility is the reason the runtime choice was made (pure-Go Goja with
// a native Interrupt, not a cgo binding), and it is the property the roadmap
// names as gate 6 acceptance evidence. A script that never returns is the
// cheapest denial of service a page can mount on a browser that runs every tab
// in one process, so the bound has to be enforced by the engine rather than
// offered to the page.

func TestInfiniteLoopIsStoppedByItsTimeout(t *testing.T) {
	const bound = 100 * time.Millisecond
	r := newRuntime(t, js.Options{URL: "https://example.test/a", Timeout: bound})

	start := time.Now()
	err := r.Run(`var i = 0; while (true) { i++; }`, "inline")
	elapsed := time.Since(start)

	if !errors.Is(err, js.ErrInterrupted) {
		t.Fatalf("an endless script returned %v, want js.ErrInterrupted", err)
	}
	// The lower bound is the non-vacuity check: a runtime that decided every
	// script was "interrupted" before running it would satisfy the assertion
	// above without having executed a single instruction.
	if elapsed < bound/2 {
		t.Errorf("stopped after %v, before the %v bound: the script never got to run", elapsed, bound)
	}
	if elapsed > 5*time.Second {
		t.Errorf("the %v bound took %v to fire: the tab was frozen for that long", bound, elapsed)
	}
}

// Interrupt is the host's own lever, separate from the timeout: navigation away
// from a page, a tab close, or the user stopping a load all have to be able to
// end a script that is still running. It is called from another goroutine, so
// the runtime must be safe to interrupt from outside the thread that is
// executing it - which is also what -race checks here.
func TestInterruptFromAnotherGoroutineStopsARunningScript(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/a", Timeout: 0})

	var wg sync.WaitGroup
	wg.Add(1)
	var runErr error
	go func() {
		defer wg.Done()
		runErr = r.Run(`var n = 0; while (n < 1e18) { n = n + 1; }`, "inline")
	}()

	time.Sleep(20 * time.Millisecond)
	r.Interrupt()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("Interrupt() did not stop Run: the calling goroutine is still blocked")
	}
	if !errors.Is(runErr, js.ErrInterrupted) {
		t.Errorf("Run returned %v, want js.ErrInterrupted", runErr)
	}
}

// DefaultTimeout is a documented number, not a rumour: the brief's page-responsiveness
// requirement is stated in wall-clock terms, and a bound nobody can read out of
// the API cannot be tested or compared against it.
func TestDefaultTimeoutIsBoundedAndNotZero(t *testing.T) {
	if js.DefaultTimeout <= 0 || js.DefaultTimeout > 10*time.Second {
		t.Errorf("DefaultTimeout = %v, want a positive bound no longer than 10s", js.DefaultTimeout)
	}
}

// A script that finishes normally must not be reported as interrupted because
// the timer was armed and never disarmed. This is the false-positive half of
// the previous two tests.
func TestFastScriptIsNotReportedAsInterrupted(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/a", Timeout: 2 * time.Second})
	if err := r.Run(`var sum = 0; for (var i = 0; i < 1000; i++) { sum += i; }`, "inline"); err != nil {
		t.Fatalf("a 1000-iteration script returned %v, want nil", err)
	}
	if v, ok := r.Global("sum"); !ok || v.Num != 499500 {
		t.Errorf("sum = %+v %v, want 499500", v, ok)
	}
}
