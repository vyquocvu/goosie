package navigation_test

import (
	"context"
	"sync"
	"testing"
	"time"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
)

// TestSchedulerWithRateLimiter_AddResourceAcquires verifies that AddResource
// succeeds and returns a non-zero Load when the rate limiter has capacity.
func TestSchedulerWithRateLimiter_AddResourceAcquires(t *testing.T) {
	sched := navigation.NewSchedulerWithOptions(navigation.SchedulerOptions{
		MaxConnsPerOrigin: 4,
		MaxConnsGlobal:    8,
	})

	// Start a navigation first so there is an active context.
	sched.Begin(context.Background(), "https://example.com")

	load, ctx := sched.AddResource(context.Background(), "https://example.com/style.css", navigation.PriorityBlockingCSS)
	if !load.ID.IsValid() {
		t.Fatal("AddResource returned zero Load; want valid ID")
	}
	if load.Origin.Host() != "example.com" {
		t.Errorf("load.Origin.Host() = %q, want %q", load.Origin.Host(), "example.com")
	}
	if load.Priority != navigation.PriorityBlockingCSS {
		t.Errorf("load.Priority = %v, want PriorityBlockingCSS", load.Priority)
	}

	// Context should carry the resource ID.
	gotID, ok := navigation.IDFromContext(ctx)
	if !ok || gotID != load.ID {
		t.Errorf("context ID = %v, want %v", gotID, load.ID)
	}

	// PendingLoads should include the resource.
	pending := sched.PendingLoads()
	found := false
	for _, l := range pending {
		if l.ID == load.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("PendingLoads does not contain the added resource")
	}
}

// TestSchedulerWithRateLimiter_CancelReleasesSlots verifies that Cancel()
// releases all limiter slots so new navigations can proceed immediately.
func TestSchedulerWithRateLimiter_CancelReleasesSlots(t *testing.T) {
	sched := navigation.NewSchedulerWithOptions(navigation.SchedulerOptions{
		MaxConnsPerOrigin: 2,
		MaxConnsGlobal:    2,
	})

	sched.Begin(context.Background(), "https://example.com")

	// Fill both per-origin and global slots.
	r1, _ := sched.AddResource(context.Background(), "https://example.com/a.css", navigation.PriorityBlockingCSS)
	r2, _ := sched.AddResource(context.Background(), "https://example.com/b.css", navigation.PriorityScript)
	if !r1.ID.IsValid() || !r2.ID.IsValid() {
		t.Fatal("expected both resources to be admitted")
	}

	// Cancel everything — this should release both slots.
	sched.Cancel()

	// Start a new navigation and add resources; should not block.
	sched.Begin(context.Background(), "https://example.com/page2")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	r3, _ := sched.AddResource(ctx, "https://example.com/c.css", navigation.PriorityBlockingCSS)
	if !r3.ID.IsValid() {
		t.Fatal("AddResource after Cancel returned zero Load; slots were leaked")
	}

	r4, _ := sched.AddResource(ctx, "https://example.com/d.css", navigation.PriorityScript)
	if !r4.ID.IsValid() {
		t.Fatal("second AddResource after Cancel returned zero Load; slots were leaked")
	}
}

// TestSchedulerWithRateLimiter_NewNavigationReleasesSlots verifies that
// Begin() supersedes the previous navigation and releases its sub-resource
// slots through cancelPreviousLocked.
func TestSchedulerWithRateLimiter_NewNavigationReleasesSlots(t *testing.T) {
	sched := navigation.NewSchedulerWithOptions(navigation.SchedulerOptions{
		MaxConnsPerOrigin: 2,
		MaxConnsGlobal:    2,
	})

	// First navigation: fill all slots.
	sched.Begin(context.Background(), "https://example.com/page1")
	r1, _ := sched.AddResource(context.Background(), "https://example.com/a.css", navigation.PriorityBlockingCSS)
	r2, _ := sched.AddResource(context.Background(), "https://example.com/b.css", navigation.PriorityScript)
	if !r1.ID.IsValid() || !r2.ID.IsValid() {
		t.Fatal("expected both resources to be admitted")
	}

	// Second navigation should cancel the first and release its sub-resource slots.
	sched.Begin(context.Background(), "https://example.com/page2")

	// We should now be able to add 2 new resources without blocking.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	r3, _ := sched.AddResource(ctx, "https://example.com/c.css", navigation.PriorityBlockingCSS)
	if !r3.ID.IsValid() {
		t.Fatal("AddResource after new Begin returned zero Load; slots were leaked")
	}

	r4, _ := sched.AddResource(ctx, "https://example.com/d.css", navigation.PriorityScript)
	if !r4.ID.IsValid() {
		t.Fatal("second AddResource after new Begin returned zero Load; slots were leaked")
	}
}

// TestSchedulerWithRateLimiter_CancelledContextReturnsZeroLoad verifies that
// AddResource with a cancelled parent context returns a zero Load without
// registering the resource.
func TestSchedulerWithRateLimiter_CancelledContextReturnsZeroLoad(t *testing.T) {
	sched := navigation.NewSchedulerWithOptions(navigation.SchedulerOptions{
		MaxConnsPerOrigin: 1,
		MaxConnsGlobal:    1,
	})

	sched.Begin(context.Background(), "https://example.com")

	// Fill the single slot.
	r1, _ := sched.AddResource(context.Background(), "https://example.com/a.css", navigation.PriorityBlockingCSS)
	if !r1.ID.IsValid() {
		t.Fatal("first AddResource should succeed")
	}

	// Try to add another resource with an already-cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	load, retCtx := sched.AddResource(ctx, "https://example.com/b.css", navigation.PriorityScript)
	if load.ID.IsValid() {
		t.Error("AddResource with cancelled ctx should return zero Load")
	}
	if retCtx.Err() == nil {
		t.Error("returned context should carry an error")
	}

	// The cancelled resource should not appear in PendingLoads.
	for _, l := range sched.PendingLoads() {
		if l.URL == "https://example.com/b.css" {
			t.Error("cancelled resource should not be in PendingLoads")
		}
	}

	// Clean up: release the first slot.
	sched.RemoveResource(r1.ID)
}

// TestSchedulerWithRateLimiter_RemoveResourceReleasesSlot verifies that
// RemoveResource frees the limiter slot so a blocked AddResource can proceed.
func TestSchedulerWithRateLimiter_RemoveResourceReleasesSlot(t *testing.T) {
	sched := navigation.NewSchedulerWithOptions(navigation.SchedulerOptions{
		MaxConnsPerOrigin: 1,
		MaxConnsGlobal:    1,
	})

	sched.Begin(context.Background(), "https://example.com")

	// Fill the single slot.
	r1, _ := sched.AddResource(context.Background(), "https://example.com/a.css", navigation.PriorityBlockingCSS)
	if !r1.ID.IsValid() {
		t.Fatal("first AddResource should succeed")
	}

	// Add another resource in a goroutine — it should block until we free a slot.
	type result struct {
		load navigation.Load
		ctx  context.Context
	}
	ch := make(chan result, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		load, retCtx := sched.AddResource(ctx, "https://example.com/b.css", navigation.PriorityScript)
		ch <- result{load, retCtx}
	}()

	// Give the goroutine time to block in Acquire.
	time.Sleep(50 * time.Millisecond)

	// Remove the first resource — this should release the slot.
	sched.RemoveResource(r1.ID)

	select {
	case res := <-ch:
		if !res.load.ID.IsValid() {
			t.Fatal("blocked AddResource should succeed after RemoveResource")
		}
		if res.load.Origin.Host() != "example.com" {
			t.Errorf("load.Origin.Host() = %q, want %q", res.load.Origin.Host(), "example.com")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocked AddResource did not unblock after RemoveResource")
	}
}

// TestSchedulerWithRateLimiter_ConcurrentAddAndCancel exercises concurrent
// AddResource and Cancel calls to detect races or deadlocks.
func TestSchedulerWithRateLimiter_ConcurrentAddAndCancel(t *testing.T) {
	sched := navigation.NewSchedulerWithOptions(navigation.SchedulerOptions{
		MaxConnsPerOrigin: 4,
		MaxConnsGlobal:    8,
	})

	var wg sync.WaitGroup
	const rounds = 20

	for i := range rounds {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			sched.Begin(ctx, "https://example.com")
			for range 4 {
				load, _ := sched.AddResource(ctx, "https://example.com/r.css", navigation.PriorityScript)
				if load.ID.IsValid() {
					sched.RemoveResource(load.ID)
				}
			}
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent AddResource/Cancel test timed out — possible deadlock")
	}
}
