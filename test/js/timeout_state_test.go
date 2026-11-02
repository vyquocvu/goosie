package js_test

import (
	"errors"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

// FR-JS-06 needs to tell the host that the deadline, not a navigation, stopped
// the script: only the former is an unresponsive page worth prompting about.

func TestTimeoutIsReportedOnce(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/slow", Timeout: 50 * time.Millisecond})
	if err := r.Run(`while (true) {}`, "inline"); !errors.Is(err, js.ErrInterrupted) {
		t.Fatalf("run = %v, want js.ErrInterrupted", err)
	}
	if !r.TimedOut() {
		t.Fatal("a script stopped by its deadline did not set TimedOut")
	}
	if !r.TakeTimedOut() {
		t.Fatal("TakeTimedOut = false after a deadline stop")
	}
	if r.TakeTimedOut() {
		t.Fatal("TakeTimedOut reported the same stop twice")
	}
}

func TestHostInterruptIsNotReportedAsTimeout(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/stop", Timeout: 30 * time.Second})
	done := make(chan error, 1)
	go func() {
		done <- r.Run(`while (true) {}`, "inline")
	}()
	time.Sleep(20 * time.Millisecond)
	r.Interrupt()
	if err := <-done; !errors.Is(err, js.ErrInterrupted) {
		t.Fatalf("run = %v, want js.ErrInterrupted", err)
	}
	if r.TimedOut() {
		t.Fatal("a host Interrupt was reported as a timeout")
	}
}

func TestCleanRunDoesNotReportTimeout(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/fast", Timeout: 50 * time.Millisecond})
	if err := r.Run(`var x = 1 + 1;`, "inline"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if r.TimedOut() {
		t.Fatal("a script that finished before its deadline reported a timeout")
	}
}

func TestTimerCallbackTimeoutIsReported(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/timer", Timeout: 50 * time.Millisecond})
	if err := r.Run(`setTimeout(function () { while (true) {} }, 0);`, "inline"); err != nil {
		t.Fatalf("run: %v", err)
	}
	r.Tick()
	if !r.TimedOut() {
		t.Fatal("a timer callback stopped by its deadline did not set TimedOut")
	}
	if !r.TakeTimedOut() {
		t.Fatal("TakeTimedOut = false after a timer deadline stop")
	}
}
