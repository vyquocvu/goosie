package documentloader_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/documentloader"
)

// TestMutationCoalescer_BatchWithinWindow — multiple Trigger calls in
// quick succession produce exactly one render call after the window.
func TestMutationCoalescer_BatchWithinWindow(t *testing.T) {
	var fires int32
	c := documentloader.NewMutationCoalescer(20*time.Millisecond, func(n int) {
		atomic.AddInt32(&fires, 1)
	})
	for i := 0; i < 50; i++ {
		c.Trigger()
		time.Sleep(time.Millisecond)
	}
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
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&fires); got != 1 {
		t.Errorf("idle fires = %d, want 1", got)
	}
}

func TestMutationCoalescer_PassesCountToRender(t *testing.T) {
	var seen int32
	done := make(chan struct{}, 1)
	c := documentloader.NewMutationCoalescer(10*time.Millisecond, func(n int) {
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

func TestMutationCoalescer_FlushImmediate(t *testing.T) {
	var fired int32
	c := documentloader.NewMutationCoalescer(time.Hour, func(n int) {
		atomic.AddInt32(&fired, 1)
	})
	c.Trigger()
	c.Flush()
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
	c.Flush()
	if got := atomic.LoadInt32(&fired); got != 1 {
		t.Errorf("idle Flush fires = %d, want 1", got)
	}
}

func TestMutationCoalescer_StopCancelsPending(t *testing.T) {
	var fired int32
	c := documentloader.NewMutationCoalescer(10*time.Millisecond, func(n int) {
		atomic.AddInt32(&fired, 1)
	})
	c.Trigger()
	c.Stop()
	c.Trigger()
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&fired); got != 0 {
		t.Errorf("fired after Stop = %d, want 0", got)
	}
}

func TestMutationCoalescer_ZeroWindowFiresImmediately(t *testing.T) {
	var fired int32
	c := documentloader.NewMutationCoalescer(0, func(n int) {
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

func TestMutationCoalescer_ConcurrentTrigger(t *testing.T) {
	var fired int32
	c := documentloader.NewMutationCoalescer(10*time.Millisecond, func(n int) {
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
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&fired); got != 1 {
		t.Errorf("concurrent fires = %d, want 1 (coalesced)", got)
	}
	if pending := c.Pending(); pending != 0 {
		t.Errorf("pending = %d after render, want 0", pending)
	}
}

func TestMutationCoalescer_NilRender(t *testing.T) {
	c := documentloader.NewMutationCoalescer(10*time.Millisecond, nil)
	c.Trigger()
	c.Flush()
	time.Sleep(20 * time.Millisecond)
}

func TestMutationCoalescer_PendingReflectsInflight(t *testing.T) {
	c := documentloader.NewMutationCoalescer(time.Hour, func(n int) {})
	for i := 0; i < 5; i++ {
		c.Trigger()
	}
	if got := c.Pending(); got != 5 {
		t.Errorf("Pending = %d, want 5", got)
	}
	c.Stop()
}
