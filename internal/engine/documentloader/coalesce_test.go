package documentloader

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestMutationCoalescer_BatchWithinWindow — multiple Trigger calls in
// quick succession produce exactly one render call after the window.
func TestMutationCoalescer_BatchWithinWindow(t *testing.T) {
	var fires int32
	c := NewMutationCoalescer(20*time.Millisecond, func(n int) {
		atomic.AddInt32(&fires, 1)
	})
	for i := 0; i < 50; i++ {
		c.Trigger()
		time.Sleep(time.Millisecond)
	}
	// Wait for the timer to fire after the last trigger.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&fires) >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&fires); got != 1 {
		t.Errorf("fires = %d, want 1 (coalesced burst)", got)
	}
	// And no further fires for the idle period.
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&fires); got != 1 {
		t.Errorf("idle fires = %d, want 1", got)
	}
}

// TestMutationCoalescer_PassesCountToRender — the render callback
// receives the mutation count.
func TestMutationCoalescer_PassesCountToRender(t *testing.T) {
	var seen int32
	done := make(chan struct{}, 1)
	c := NewMutationCoalescer(10*time.Millisecond, func(n int) {
		atomic.StoreInt32(&seen, int32(n))
		done <- struct{}{}
	})
	for i := 0; i < 7; i++ {
		c.Trigger()
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("render did not fire")
	}
	if got := atomic.LoadInt32(&seen); got != 7 {
		t.Errorf("render count = %d, want 7", got)
	}
}

// TestMutationCoalescer_FlushImmediate — Flush fires synchronously
// without waiting for the window.
func TestMutationCoalescer_FlushImmediate(t *testing.T) {
	var fired int32
	c := NewMutationCoalescer(time.Hour, func(n int) {
		atomic.AddInt32(&fired, 1)
	})
	c.Trigger()
	c.Flush()
	// The render runs in a goroutine inside Flush; give it a moment.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&fired) >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&fired); got != 1 {
		t.Errorf("fires after Flush = %d, want 1", got)
	}
	// A second Flush with no pending mutations is a no-op.
	c.Flush()
	if got := atomic.LoadInt32(&fired); got != 1 {
		t.Errorf("idle Flush fires = %d, want 1", got)
	}
}

// TestMutationCoalescer_StopCancelsPending — Stop prevents future
// Trigger calls from firing.
func TestMutationCoalescer_StopCancelsPending(t *testing.T) {
	var fired int32
	c := NewMutationCoalescer(10*time.Millisecond, func(n int) {
		atomic.AddInt32(&fired, 1)
	})
	c.Trigger()
	c.Stop()
	c.Trigger() // should be no-op
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&fired); got != 0 {
		t.Errorf("fired after Stop = %d, want 0", got)
	}
}

// TestMutationCoalescer_ZeroWindowFiresImmediately — window=0 means
// no coalescing: every Trigger fires a render.
func TestMutationCoalescer_ZeroWindowFiresImmediately(t *testing.T) {
	var fired int32
	c := NewMutationCoalescer(0, func(n int) {
		atomic.AddInt32(&fired, 1)
	})
	for i := 0; i < 5; i++ {
		c.Trigger()
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&fired) >= 5 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&fired); got != 5 {
		t.Errorf("fires with window=0 = %d, want 5", got)
	}
}

// TestMutationCoalescer_ConcurrentTrigger — concurrent Trigger calls
// are safely coalesced.
func TestMutationCoalescer_ConcurrentTrigger(t *testing.T) {
	var fired int32
	c := NewMutationCoalescer(10*time.Millisecond, func(n int) {
		atomic.AddInt32(&fired, 1)
	})
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Trigger()
			}
		}()
	}
	wg.Wait()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&fired) >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // settle any second fires
	if got := atomic.LoadInt32(&fired); got != 1 {
		t.Errorf("concurrent fires = %d, want 1 (coalesced)", got)
	}
	if pending := c.Pending(); pending != 0 {
		t.Errorf("pending = %d after render, want 0", pending)
	}
}

// TestMutationCoalescer_NilRender — nil render callback is a no-op.
func TestMutationCoalescer_NilRender(t *testing.T) {
	c := NewMutationCoalescer(10*time.Millisecond, nil)
	c.Trigger()
	c.Flush()
	time.Sleep(20 * time.Millisecond)
	// No panic means success.
}

// TestMutationCoalescer_PendingReflectsInflight — Pending returns the
// number of mutations received since the last fire.
func TestMutationCoalescer_PendingReflectsInflight(t *testing.T) {
	c := NewMutationCoalescer(time.Hour, func(n int) {})
	for i := 0; i < 5; i++ {
		c.Trigger()
	}
	if got := c.Pending(); got != 5 {
		t.Errorf("Pending = %d, want 5", got)
	}
	c.Stop()
}