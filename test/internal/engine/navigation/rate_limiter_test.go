package navigation_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
)

func TestRateLimiter_AcquireRelease(t *testing.T) {
	rl := navigation.NewRateLimiter(6, 24)
	ctx := context.Background()

	if err := rl.Acquire(ctx, "example.com", navigation.PriorityDocument); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	rl.Release("example.com")

	// After release, internal counts should be zero.
	rl.Mu.Lock()
	if rl.GlobalActive != 0 {
		t.Errorf("globalActive = %d, want 0", rl.GlobalActive)
	}
	if _, ok := rl.Origins["example.com"]; ok {
		t.Error("origin entry not cleaned up after release to zero")
	}
	rl.Mu.Unlock()
}

func TestRateLimiter_PerOriginLimit(t *testing.T) {
	rl := navigation.NewRateLimiter(6, 0) // per-origin=6, global=unlimited
	ctx := context.Background()

	for i := range 6 {
		_ = i
		if err := rl.Acquire(ctx, "example.com", navigation.PriorityScript); err != nil {
			t.Fatalf("Acquire %d: %v", i, err)
		}
	}

	// 7th should block.
	ctx2, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	err := rl.Acquire(ctx2, "example.com", navigation.PriorityScript)
	if err == nil {
		t.Fatal("7th Acquire should have blocked")
	}
	if ctx2.Err() == nil {
		t.Fatal("expected context error after blocked acquire")
	}
}

func TestRateLimiter_GlobalLimit(t *testing.T) {
	rl := navigation.NewRateLimiter(0, 24) // per-origin=unlimited, global=24
	ctx := context.Background()

	for i := range 24 {
		origin := "host" + string(rune('a'+i%26)) + ".com"
		if err := rl.Acquire(ctx, origin, navigation.PriorityScript); err != nil {
			t.Fatalf("Acquire %d: %v", i, err)
		}
	}

	ctx2, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	err := rl.Acquire(ctx2, "overflow.com", navigation.PriorityScript)
	if err == nil {
		t.Fatal("25th Acquire should have blocked")
	}
}

func TestRateLimiter_ContextCancellation(t *testing.T) {
	rl := navigation.NewRateLimiter(1, 0)
	ctx := context.Background()

	if err := rl.Acquire(ctx, "example.com", navigation.PriorityDocument); err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	ctx2, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- rl.Acquire(ctx2, "example.com", navigation.PriorityScript)
	}()

	// Give goroutine time to enqueue.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from cancelled context")
		}
	case <-time.After(time.Second):
		t.Fatal("Acquire did not return after context cancellation")
	}
}

func TestRateLimiter_PriorityOrdering(t *testing.T) {
	rl := navigation.NewRateLimiter(1, 0) // only 1 concurrent per origin
	ctx := context.Background()

	// Fill the slot.
	if err := rl.Acquire(ctx, "example.com", navigation.PriorityDocument); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// Enqueue a low-priority waiter.
	lowDone := make(chan struct{})
	go func() {
		rl.Acquire(ctx, "example.com", navigation.PrioritySpeculative)
		close(lowDone)
	}()
	time.Sleep(20 * time.Millisecond)

	// Enqueue a high-priority waiter.
	highDone := make(chan struct{})
	go func() {
		rl.Acquire(ctx, "example.com", navigation.PriorityDocument)
		close(highDone)
	}()
	time.Sleep(20 * time.Millisecond)

	// Release the slot — high-priority should be admitted first.
	rl.Release("example.com")

	select {
	case <-highDone:
		// good
	case <-time.After(time.Second):
		t.Fatal("high-priority waiter not admitted")
	}

	// Low-priority still waiting.
	select {
	case <-lowDone:
		t.Fatal("low-priority waiter should still be blocked")
	default:
	}

	// Release again to unblock the low-priority waiter.
	rl.Release("example.com")
	select {
	case <-lowDone:
	case <-time.After(time.Second):
		t.Fatal("low-priority waiter not admitted after second release")
	}
	rl.Release("example.com")
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := navigation.NewRateLimiter(6, 24)
	ctx := context.Background()

	const goroutines = 50
	const perGoroutine = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range perGoroutine {
				if err := rl.Acquire(ctx, "example.com", navigation.PriorityScript); err != nil {
					t.Errorf("Acquire: %v", err)
					return
				}
				rl.Release("example.com")
			}
		}()
	}
	wg.Wait()

	rl.Mu.Lock()
	if rl.GlobalActive != 0 {
		t.Errorf("globalActive = %d, want 0", rl.GlobalActive)
	}
	rl.Mu.Unlock()
}

func TestRateLimiter_OriginCleanup(t *testing.T) {
	rl := navigation.NewRateLimiter(6, 24)
	ctx := context.Background()

	if err := rl.Acquire(ctx, "example.com", navigation.PriorityScript); err != nil {
		t.Fatal(err)
	}
	rl.Release("example.com")

	rl.Mu.Lock()
	if _, ok := rl.Origins["example.com"]; ok {
		t.Error("origin map entry not cleaned up when count reached zero")
	}
	rl.Mu.Unlock()
}

func TestRateLimiter_ReleaseUnknownOrigin(t *testing.T) {
	rl := navigation.NewRateLimiter(6, 24)
	// Should not panic.
	rl.Release("unknown.com")
	rl.Release("")
}

func TestRateLimiter_ZeroValueUnlimited(t *testing.T) {
	var rl navigation.RateLimiter // zero value
	ctx := context.Background()

	// Should never block.
	for range 1000 {
		if err := rl.Acquire(ctx, "any.com", navigation.PriorityScript); err != nil {
			t.Fatalf("zero-value Acquire: %v", err)
		}
	}
	// Release should be idempotent even though we never really tracked.
	rl.Release("any.com")
}

func TestRateLimiter_AcquireReleaseCounting(t *testing.T) {
	rl := navigation.NewRateLimiter(3, 5)
	ctx := context.Background()

	var active atomic.Int32
	var maxActive atomic.Int32

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			if err := rl.Acquire(ctx, "example.com", navigation.PriorityScript); err != nil {
				return
			}
			cur := active.Add(1)
			for {
				old := maxActive.Load()
				if cur <= old || maxActive.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			active.Add(-1)
			rl.Release("example.com")
		}()
	}
	wg.Wait()

	if got := maxActive.Load(); got > 3 {
		t.Errorf("max concurrent = %d, want <= 3 (per-origin limit)", got)
	}
}
