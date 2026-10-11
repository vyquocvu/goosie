package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

// Microtask checkpoints: promise reactions queue and drain FIFO after each
// task (script or timer callback), never inline. A reaction scheduled during
// Run must not run before the synchronous code after it; a reaction scheduled
// during a timer callback must not run before that callback finishes.

func orderOf(t *testing.T, r *js.Runtime) string {
	t.Helper()
	v, ok := r.Global("orderStr")
	if !ok {
		t.Fatal("orderStr global not found")
	}
	return v.Str
}

// TestMicrotaskRunsAfterSyncCode pins deferred (not inline) reactions: sync
// code following then() runs first.
func TestMicrotaskRunsAfterSyncCode(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	if err := r.Run(`
		var order = [];
		Promise.resolve().then(function() { order.push('micro'); });
		order.push('sync');
		var orderStr = order.join(',');
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The checkpoint at the end of Run has drained by now; re-read live.
	if err := r.Run(`var orderStr = order.join(',');`, "inline2"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := orderOf(t, r); got != "sync,micro" {
		t.Fatalf("order = %q, want sync,micro (reaction deferred past sync code)", got)
	}
}

// TestMicrotaskFIFOChains pins queue order across promises: reactions run in
// attach order, and chained continuations queue behind already-queued work.
func TestMicrotaskFIFOChains(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	if err := r.Run(`
		var order = [];
		Promise.resolve().then(function() { order.push(1); }).then(function() { order.push(3); });
		Promise.resolve().then(function() { order.push(2); });
		var orderStr = order.join(',');
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := r.Run(`var orderStr = order.join(',');`, "inline2"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := orderOf(t, r); got != "1,2,3" {
		t.Fatalf("order = %q, want 1,2,3 (FIFO across chains)", got)
	}
}

// TestMicrotaskRunsBeforeNextTimer pins checkpoint-before-task: a microtask
// queued during Run drains before any timer callback Tick delivers.
func TestMicrotaskRunsBeforeNextTimer(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	if err := r.Run(`
		var order = [];
		setTimeout(function() { order.push('timer'); }, 0);
		Promise.resolve().then(function() { order.push('micro'); });
		var orderStr = order.join(',');
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	r.Tick()
	if err := r.Run(`var orderStr = order.join(',');`, "inline2"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := orderOf(t, r); got != "micro,timer" {
		t.Fatalf("order = %q, want micro,timer", got)
	}
}

// TestMicrotaskQueuedInTimerDrainsAfterTask pins the per-task checkpoint: a
// reaction queued inside a timer callback runs after that callback finishes.
func TestMicrotaskQueuedInTimerDrainsAfterTask(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	if err := r.Run(`
		var order = [];
		setTimeout(function() {
			Promise.resolve().then(function() { order.push('inner'); });
			order.push('timer');
		}, 0);
		var orderStr = order.join(',');
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	r.Tick()
	if err := r.Run(`var orderStr = order.join(',');`, "inline2"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := orderOf(t, r); got != "timer,inner" {
		t.Fatalf("order = %q, want timer,inner (reaction queued mid-task drains after it)", got)
	}
}

// TestRunawayMicrotasksInterrupt pins the DoS bound: a self-perpetuating
// microtask chain ends in ErrInterrupted from the Run timeout, not a hang
// (and not a stack-exhausting inline recursion).
func TestRunawayMicrotasksInterrupt(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/", Timeout: 300 * time.Millisecond})
	err := r.Run(`function loop() { Promise.resolve().then(loop); } loop();`, "inline")
	if err != js.ErrInterrupted {
		t.Fatalf("Run err = %v, want ErrInterrupted", err)
	}
}
