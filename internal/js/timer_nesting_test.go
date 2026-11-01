package js

import (
	"testing"
	"time"
)

func newTimerTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	r, err := New(Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func onlyTimer(t *testing.T, r *Runtime) *timer {
	t.Helper()
	if len(r.timers) != 1 {
		t.Fatalf("pending timers = %d, want 1", len(r.timers))
	}
	for _, tm := range r.timers {
		return tm
	}
	return nil
}

// TestNestedTimersClampZeroDelayTo4ms pins FR-JS-04: once a timer chain passes
// five levels of nesting, a zero-delay timer is promoted to the 4ms minimum
// event-loop resolution.
func TestNestedTimersClampZeroDelayTo4ms(t *testing.T) {
	r := newTimerTestRuntime(t)
	r.timerNesting = maxTimerNestingLevel + 1

	if err := r.Run(`setTimeout(function() {}, 0);`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := onlyTimer(t, r).interval; got != 4*time.Millisecond {
		t.Fatalf("deeply nested setTimeout(0) interval = %v, want 4ms", got)
	}
}

// TestShallowNestedTimersKeepZeroDelay: below the nesting threshold the delay
// is untouched, so ordinary nested scheduling stays instantaneous.
func TestShallowNestedTimersKeepZeroDelay(t *testing.T) {
	r := newTimerTestRuntime(t)
	r.timerNesting = maxTimerNestingLevel - 1

	if err := r.Run(`setTimeout(function() {}, 0);`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := onlyTimer(t, r).interval; got != 0 {
		t.Fatalf("shallow nested setTimeout(0) interval = %v, want 0", got)
	}
}

// TestNestedIntervalClampsTo4ms applies the same floor to setInterval, whose
// zero-delay clamp was previously 1ms.
func TestNestedIntervalClampsTo4ms(t *testing.T) {
	r := newTimerTestRuntime(t)
	r.timerNesting = maxTimerNestingLevel + 1

	if err := r.Run(`setInterval(function() {}, 0);`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := onlyTimer(t, r).interval; got != 4*time.Millisecond {
		t.Fatalf("deeply nested setInterval(0) interval = %v, want 4ms", got)
	}
}

// TestTimerCallbackPropagatesNestingLevel proves the level advances through
// the event loop: a timer scheduled by another timer's callback is one level
// deeper than the callback that created it.
func TestTimerCallbackPropagatesNestingLevel(t *testing.T) {
	r := newTimerTestRuntime(t)

	if err := r.Run(`
		setTimeout(function() {
			setTimeout(function() {}, 0);
		}, 0);
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !r.Tick() {
		t.Fatal("root timer did not fire")
	}

	probe := onlyTimer(t, r)
	if probe.nesting != 2 {
		t.Fatalf("nested timer nesting = %d, want 2", probe.nesting)
	}
	if probe.interval != 0 {
		t.Fatalf("nested timer interval = %v, want 0 at level 2", probe.interval)
	}
}
