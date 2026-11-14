package js_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

// qc:BR-167
func TestInfiniteLoopIsStoppedByItsTimeout(t *testing.T) {
	const bound = 100 * time.Millisecond
	r := newRuntime(t, js.Options{URL: "https://example.test/a", Timeout: bound})

	start := time.Now()
	err := r.Run(`var i = 0; while (true) { i++; }`, "inline")
	elapsed := time.Since(start)

	if !errors.Is(err, js.ErrInterrupted) {
		t.Fatalf("an endless script returned %v, want js.ErrInterrupted", err)
	}
	if elapsed < bound/2 {
		t.Errorf("stopped after %v, before the %v bound: the script never got to run", elapsed, bound)
	}
	if elapsed > 5*time.Second {
		t.Errorf("the %v bound took %v to fire: the tab was frozen for that long", bound, elapsed)
	}
}

// qc:BR-167
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

// qc:BR-167
func TestDefaultTimeoutIsBoundedAndNotZero(t *testing.T) {
	if js.DefaultTimeout <= 0 || js.DefaultTimeout > 10*time.Second {
		t.Errorf("DefaultTimeout = %v, want a positive bound no longer than 10s", js.DefaultTimeout)
	}
}

// qc:BR-167
func TestFastScriptIsNotReportedAsInterrupted(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/a", Timeout: 2 * time.Second})
	if err := r.Run(`var sum = 0; for (var i = 0; i < 1000; i++) { sum += i; }`, "inline"); err != nil {
		t.Fatalf("a 1000-iteration script returned %v, want nil", err)
	}
	if v, ok := r.Global("sum"); !ok || v.Num != 499500 {
		t.Errorf("sum = %+v %v, want 499500", v, ok)
	}
}
