package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

// TestSetTimeoutFiresAfterDelay verifies that a setTimeout callback
// executes when Tick is called after the delay has elapsed. The callback
// sets a global variable; we read it back through the host API.
func TestSetTimeoutFiresAfterDelay(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})

	if err := r.Run(`var fired = false; setTimeout(function() { fired = true; }, 0);`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Before Tick the callback must not have run: setTimeout is
	// asynchronous, it does not execute inline.
	v, _ := r.Global("fired")
	if v.Bool {
		t.Fatal("setTimeout callback fired synchronously during Run, want it to wait for Tick")
	}

	time.Sleep(5 * time.Millisecond) // ensure the 0ms delay has elapsed
	r.Tick()

	v, ok := r.Global("fired")
	if !ok {
		t.Fatal("global fired not found after Tick")
	}
	if !v.Bool {
		t.Error("setTimeout callback did not fire after Tick")
	}
}

// TestSetTimeoutWithDelay verifies that a timer with a real delay does
// not fire before the delay has elapsed, and does fire after.
func TestSetTimeoutWithDelay(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})

	if err := r.Run(`var fired = false; setTimeout(function() { fired = true; }, 50);`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Tick immediately: the 50ms delay has not elapsed.
	r.Tick()
	v, _ := r.Global("fired")
	if v.Bool {
		t.Fatal("setTimeout fired before its delay elapsed")
	}

	time.Sleep(60 * time.Millisecond)
	r.Tick()

	v, _ = r.Global("fired")
	if !v.Bool {
		t.Error("setTimeout did not fire after delay elapsed")
	}
}

// TestClearTimeoutPreventsFiring verifies that clearTimeout cancels a
// pending setTimeout so its callback never runs.
func TestClearTimeoutPreventsFiring(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})

	if err := r.Run(`
		var fired = false;
		var id = setTimeout(function() { fired = true; }, 0);
		clearTimeout(id);
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	r.Tick()

	v, _ := r.Global("fired")
	if v.Bool {
		t.Error("cleared setTimeout still fired")
	}
}

// TestSetIntervalFiresRepeatedly verifies that setInterval invokes its
// callback on every Tick once the delay has elapsed, not just once.
func TestSetIntervalFiresRepeatedly(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})

	if err := r.Run(`
		var count = 0;
		setInterval(function() { count++; }, 0);
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	r.Tick()
	v, _ := r.Global("count")
	if v.Num < 1 {
		t.Errorf("after first Tick count = %v, want >= 1", v.Num)
	}

	first := v.Num
	time.Sleep(5 * time.Millisecond)
	r.Tick()

	v, _ = r.Global("count")
	if v.Num <= first {
		t.Errorf("after second Tick count = %v, expected it to increase past %v", v.Num, first)
	}
}

// TestClearIntervalStopsRepetition verifies that clearInterval prevents
// further invocations of a repeating timer.
func TestClearIntervalStopsRepetition(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})

	if err := r.Run(`
		var count = 0;
		var id = setInterval(function() { count++; }, 0);
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	r.Tick()

	v, _ := r.Global("count")
	first := v.Num
	if first < 1 {
		t.Fatalf("count = %v after first Tick, want >= 1", first)
	}

	// Cancel the interval.
	if err := r.Run(`clearInterval(id);`, "inline"); err != nil {
		t.Fatalf("Run clearInterval: %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	r.Tick()

	v, _ = r.Global("count")
	if v.Num != first {
		t.Errorf("count = %v after clearInterval + Tick, want %v (no further increments)", v.Num, first)
	}
}

// TestMultipleTimersCoexist verifies that several independent timers all
// fire on the same Tick without interfering with each other.
func TestMultipleTimersCoexist(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})

	if err := r.Run(`
		var a = 0; var b = 0; var c = 0;
		setTimeout(function() { a = 1; }, 0);
		setTimeout(function() { b = 2; }, 0);
		setTimeout(function() { c = 3; }, 0);
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	r.Tick()

	for _, tc := range []struct {
		name string
		want float64
	}{
		{"a", 1},
		{"b", 2},
		{"c", 3},
	} {
		v, ok := r.Global(tc.name)
		if !ok {
			t.Errorf("%s not found", tc.name)
			continue
		}
		if v.Num != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, v.Num, tc.want)
		}
	}
}

// TestTimerCallbackCanAccessGlobals verifies that a timer callback
// executes in the same realm as the script that created it, so it can
// read and write globals.
func TestTimerCallbackCanAccessGlobals(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})

	if err := r.Run(`
		var message = "initial";
		var greeting = "hello";
		setTimeout(function() { message = greeting + " world"; }, 0);
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	r.Tick()

	v, ok := r.Global("message")
	if !ok {
		t.Fatal("message not found")
	}
	if v.Str != "hello world" {
		t.Errorf("message = %q, want %q", v.Str, "hello world")
	}
}

// TestSetTimeoutReturnsNumericID verifies that setTimeout returns a
// number that can be passed to clearTimeout.
func TestSetTimeoutReturnsNumericID(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})

	if err := r.Run(`var tid = setTimeout(function() {}, 1000);`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	v, ok := r.Global("tid")
	if !ok {
		t.Fatal("tid not found")
	}
	if v.Kind != js.KindNumber {
		t.Errorf("tid kind = %v, want KindNumber", v.Kind)
	}
	if v.Num <= 0 {
		t.Errorf("tid = %v, want a positive integer", v.Num)
	}
}

// TestClearTimeoutWithInvalidIDIsHarmless verifies that clearTimeout with
// a non-existent or already-fired timer ID does not throw.
func TestClearTimeoutWithInvalidIDIsHarmless(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})

	// Clear a never-scheduled ID.
	if err := r.Run(`clearTimeout(9999);`, "inline"); err != nil {
		t.Errorf("clearTimeout(9999) returned %v, want nil", err)
	}

	// Clear a timer that already fired.
	if err := r.Run(`
			var done = false;
			var fid = setTimeout(function() { done = true; }, 0);
		`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	r.Tick()

	if err := r.Run(`clearTimeout(fid);`, "inline"); err != nil {
		t.Errorf("clearTimeout on fired timer returned %v, want nil", err)
	}
}

// TestSetTimeoutOnWindowObject verifies that window.setTimeout works the
// same as the bare global setTimeout, matching the browser spec where
// timer functions are properties of the Window interface.
func TestSetTimeoutOnWindowObject(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})

	if err := r.Run(`
		var fired = false;
		window.setTimeout(function() { fired = true; }, 0);
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	r.Tick()

	v, _ := r.Global("fired")
	if !v.Bool {
		t.Error("window.setTimeout callback did not fire")
	}
}

// TestSetTimeoutOneShotDoesNotRepeat verifies that a setTimeout callback
// fires exactly once, not on subsequent Ticks.
func TestSetTimeoutOneShotDoesNotRepeat(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})

	if err := r.Run(`
		var count = 0;
		setTimeout(function() { count++; }, 0);
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	r.Tick()

	v, _ := r.Global("count")
	if v.Num != 1 {
		t.Fatalf("count = %v after first Tick, want 1", v.Num)
	}

	// Second Tick must not re-fire a one-shot timer.
	time.Sleep(5 * time.Millisecond)
	r.Tick()

	v, _ = r.Global("count")
	if v.Num != 1 {
		t.Errorf("count = %v after second Tick, want 1 (setTimeout must not repeat)", v.Num)
	}
}
