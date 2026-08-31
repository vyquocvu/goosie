package navigation_test

import (
	"context"
	"sort"
	"sync"
	"testing"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
)

func TestPriorityConstants(t *testing.T) {
	// Verify ordering: Document is highest priority (lowest value),
	// Speculative is lowest priority (highest value).
	priorities := []navigation.Priority{
		navigation.PriorityDocument,
		navigation.PriorityBlockingCSS,
		navigation.PriorityVisibleImage,
		navigation.PriorityScript,
		navigation.PriorityDeferredImage,
		navigation.PrioritySpeculative,
	}
	for i := 1; i < len(priorities); i++ {
		if priorities[i] <= priorities[i-1] {
			t.Errorf("priority %v (%d) should be > %v (%d)",
				priorities[i], priorities[i],
				priorities[i-1], priorities[i-1])
		}
	}
}

func TestPriorityString(t *testing.T) {
	cases := []struct {
		p    navigation.Priority
		want string
	}{
		{navigation.PriorityDocument, "document"},
		{navigation.PriorityBlockingCSS, "blocking_css"},
		{navigation.PriorityVisibleImage, "visible_image"},
		{navigation.PriorityScript, "script"},
		{navigation.PriorityDeferredImage, "deferred_image"},
		{navigation.PrioritySpeculative, "speculative"},
		{navigation.Priority(99), "unknown_priority_99"},
	}
	for _, c := range cases {
		got := c.p.String()
		if got != c.want {
			t.Errorf("Priority(%d).String() = %q, want %q", c.p, got, c.want)
		}
	}
}

func TestSchedulerBeginDefaultsToDocumentPriority(t *testing.T) {
	sched := navigation.NewScheduler()
	load, _ := sched.Begin(context.Background(), "https://example.com")
	if load.Priority != navigation.PriorityDocument {
		t.Fatalf("Begin() load.Priority = %v, want PriorityDocument", load.Priority)
	}
}

func TestSchedulerBeginWithPriority(t *testing.T) {
	sched := navigation.NewScheduler()
	tests := []struct {
		prio navigation.Priority
		url  string
	}{
		{navigation.PriorityBlockingCSS, "https://example.com/style.css"},
		{navigation.PriorityVisibleImage, "https://example.com/hero.png"},
		{navigation.PriorityScript, "https://example.com/app.js"},
		{navigation.PriorityDeferredImage, "https://example.com/thumb.png"},
		{navigation.PrioritySpeculative, "https://example.com/prefetch"},
	}
	for _, tc := range tests {
		load, ctx := sched.BeginWithPriority(context.Background(), tc.url, tc.prio)
		if load.Priority != tc.prio {
			t.Errorf("BeginWithPriority(%v) load.Priority = %v", tc.prio, load.Priority)
		}
		if load.URL != tc.url {
			t.Errorf("BeginWithPriority URL = %q, want %q", load.URL, tc.url)
		}
		// Context should carry the priority
		got, ok := navigation.PriorityFromContext(ctx)
		if !ok {
			t.Errorf("PriorityFromContext returned false for %v", tc.prio)
		}
		if got != tc.prio {
			t.Errorf("PriorityFromContext() = %v, want %v", got, tc.prio)
		}
	}
}

func TestPriorityFromContextMissing(t *testing.T) {
	_, ok := navigation.PriorityFromContext(context.Background())
	if ok {
		t.Fatal("PriorityFromContext should return false without priority")
	}
}

func TestPriorityContextRoundTrip(t *testing.T) {
	ctx := navigation.WithPriority(context.Background(), navigation.PriorityScript)
	got, ok := navigation.PriorityFromContext(ctx)
	if !ok {
		t.Fatal("PriorityFromContext returned false")
	}
	if got != navigation.PriorityScript {
		t.Fatalf("PriorityFromContext() = %v, want PriorityScript", got)
	}
}

func TestSchedulerPendingLoads(t *testing.T) {
	sched := navigation.NewScheduler()
	// Start a main navigation (Begin adds to pending with PriorityDocument)
	docLoad, _ := sched.Begin(context.Background(), "https://example.com/doc")
	// Add sub-resources with different priorities
	cssLoad, _ := sched.AddResource(context.Background(), "https://example.com/style.css", navigation.PriorityBlockingCSS)
	specLoad, _ := sched.AddResource(context.Background(), "https://example.com/spec", navigation.PrioritySpeculative)

	pending := sched.PendingLoads()
	if len(pending) != 3 {
		t.Fatalf("PendingLoads() returned %d loads, want 3", len(pending))
	}

	// Should be sorted by priority (lowest value = highest priority first)
	expected := []navigation.ID{docLoad.ID, cssLoad.ID, specLoad.ID}
	for i, id := range expected {
		if pending[i].ID != id {
			t.Errorf("pending[%d].ID = %d, want %d", i, pending[i].ID, id)
		}
	}

	// Verify sorted by priority
	for i := 1; i < len(pending); i++ {
		if pending[i].Priority < pending[i-1].Priority {
			t.Errorf("pending loads not sorted: pending[%d].Priority=%d < pending[%d].Priority=%d",
				i, pending[i].Priority, i-1, pending[i-1].Priority)
		}
	}
}

func TestSchedulerPendingLoadsCleanupOnCancel(t *testing.T) {
	sched := navigation.NewScheduler()
	sched.AddResource(context.Background(), "https://example.com/a", navigation.PriorityScript)
	sched.AddResource(context.Background(), "https://example.com/b", navigation.PriorityVisibleImage)

	if got := len(sched.PendingLoads()); got != 2 {
		t.Fatalf("PendingLoads() = %d, want 2", got)
	}

	sched.Cancel()

	if got := len(sched.PendingLoads()); got != 0 {
		t.Fatalf("PendingLoads() after Cancel() = %d, want 0", got)
	}
}

func TestSchedulerPendingLoadsCleanupOnNewBegin(t *testing.T) {
	sched := navigation.NewScheduler()
	// First navigation creates a pending load
	sched.BeginWithPriority(context.Background(), "https://example.com/first", navigation.PriorityDocument)
	if got := len(sched.PendingLoads()); got != 1 {
		t.Fatalf("PendingLoads() = %d, want 1", got)
	}

	// Second Begin cancels the first; first load should be removed
	sched.Begin(context.Background(), "https://example.com/second")
	pending := sched.PendingLoads()
	if len(pending) != 1 {
		t.Fatalf("PendingLoads() = %d, want 1", len(pending))
	}
	if pending[0].URL != "https://example.com/second" {
		t.Fatalf("pending[0].URL = %q, want second navigation URL", pending[0].URL)
	}
}

func TestSchedulerBeginWithPriorityConcurrent(t *testing.T) {
	sched := navigation.NewScheduler()
	const workers = 16
	const perWorker = 50

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := range workers {
		go func(wid int) {
			defer wg.Done()
			prio := navigation.Priority(wid%int(navigation.PrioritySpeculative) + 1)
			for j := range perWorker {
				_ = j
				load, ctx := sched.AddResource(context.Background(), "https://example.com/concurrent", prio)
				if !load.ID.IsValid() {
					t.Errorf("invalid load ID from concurrent AddResource")
				}
				// Verify context carries priority
				if got, ok := navigation.PriorityFromContext(ctx); !ok || got != prio {
					t.Errorf("AddResource context priority = %v, ok=%v, want %v", got, ok, prio)
				}
			}
		}(i)
	}
	wg.Wait()

	// Should have exactly workers*perWorker pending loads
	pending := sched.PendingLoads()
	want := workers * perWorker
	if len(pending) != want {
		t.Fatalf("PendingLoads() = %d, want %d", len(pending), want)
	}

	// All should be sorted
	if !sort.SliceIsSorted(pending, func(i, j int) bool {
		return pending[i].Priority < pending[j].Priority
	}) {
		t.Error("PendingLoads() not sorted by priority")
	}
}

func TestSchedulerBeginWithPriorityCancelsPrevious(t *testing.T) {
	sched := navigation.NewScheduler()
	_, firstCtx := sched.BeginWithPriority(context.Background(), "https://example.com/first", navigation.PriorityScript)

	select {
	case <-firstCtx.Done():
		t.Fatal("first context cancelled before second navigation")
	default:
	}

	_, secondCtx := sched.BeginWithPriority(context.Background(), "https://example.com/second", navigation.PriorityDocument)
	select {
	case <-firstCtx.Done():
	default:
		t.Fatal("first context was not cancelled by second navigation")
	}
	select {
	case <-secondCtx.Done():
		t.Fatal("second context cancelled immediately")
	default:
	}
}

func TestLoadPriorityIsImmutable(t *testing.T) {
	sched := navigation.NewScheduler()
	load, _ := sched.BeginWithPriority(context.Background(), "https://example.com", navigation.PriorityVisibleImage)
	// Modifying the returned Load should not affect the tracked pending load
	load.Priority = navigation.PrioritySpeculative

	pending := sched.PendingLoads()
	found := false
	for _, p := range pending {
		if p.ID == load.ID {
			found = true
			if p.Priority != navigation.PriorityVisibleImage {
				t.Errorf("pending load priority mutated: got %v, want PriorityVisibleImage", p.Priority)
			}
		}
	}
	if !found {
		t.Error("load not found in PendingLoads after external mutation")
	}
}

func TestAddResourceReturnsUniqueIDs(t *testing.T) {
	sched := navigation.NewScheduler()
	load1, _ := sched.AddResource(context.Background(), "https://example.com/a", navigation.PriorityScript)
	load2, _ := sched.AddResource(context.Background(), "https://example.com/b", navigation.PriorityScript)
	if load1.ID == load2.ID {
		t.Fatalf("AddResource returned duplicate IDs: %d", load1.ID)
	}
}

func TestAddResourceContextCarriesIDAndPriority(t *testing.T) {
	sched := navigation.NewScheduler()
	load, ctx := sched.AddResource(context.Background(), "https://example.com/style.css", navigation.PriorityBlockingCSS)

	gotID, ok := navigation.IDFromContext(ctx)
	if !ok {
		t.Fatal("IDFromContext returned false")
	}
	if gotID != load.ID {
		t.Fatalf("IDFromContext() = %d, want %d", gotID, load.ID)
	}

	gotPrio, ok := navigation.PriorityFromContext(ctx)
	if !ok {
		t.Fatal("PriorityFromContext returned false")
	}
	if gotPrio != navigation.PriorityBlockingCSS {
		t.Fatalf("PriorityFromContext() = %v, want PriorityBlockingCSS", gotPrio)
	}
}

func TestRemoveResource(t *testing.T) {
	sched := navigation.NewScheduler()
	load1, ctx1 := sched.AddResource(context.Background(), "https://example.com/a", navigation.PriorityScript)
	load2, _ := sched.AddResource(context.Background(), "https://example.com/b", navigation.PriorityVisibleImage)

	if got := len(sched.PendingLoads()); got != 2 {
		t.Fatalf("PendingLoads() = %d, want 2", got)
	}

	sched.RemoveResource(load1.ID)

	// load1 should be gone
	pending := sched.PendingLoads()
	if len(pending) != 1 {
		t.Fatalf("PendingLoads() = %d, want 1", len(pending))
	}
	if pending[0].ID != load2.ID {
		t.Fatalf("remaining pending ID = %d, want %d", pending[0].ID, load2.ID)
	}

	// load1's context should be cancelled
	select {
	case <-ctx1.Done():
	default:
		t.Fatal("removed resource's context was not cancelled")
	}
}

func TestRemoveResourceIdempotent(t *testing.T) {
	sched := navigation.NewScheduler()
	load, _ := sched.AddResource(context.Background(), "https://example.com/a", navigation.PriorityScript)

	sched.RemoveResource(load.ID)
	// Second call should not panic
	sched.RemoveResource(load.ID)

	if got := len(sched.PendingLoads()); got != 0 {
		t.Fatalf("PendingLoads() = %d, want 0", got)
	}
}

func TestRemoveResourceUnknownID(t *testing.T) {
	sched := navigation.NewScheduler()
	// Should not panic on unknown ID
	sched.RemoveResource(navigation.ID(9999))
	if got := len(sched.PendingLoads()); got != 0 {
		t.Fatalf("PendingLoads() = %d, want 0", got)
	}
}

func TestAddResourceContextCancelledByParent(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	sched := navigation.NewScheduler()
	_, ctx := sched.AddResource(parent, "https://example.com/a", navigation.PriorityScript)

	select {
	case <-ctx.Done():
		t.Fatal("resource context cancelled before parent")
	default:
	}

	cancel()

	select {
	case <-ctx.Done():
	default:
		t.Fatal("resource context not cancelled when parent cancelled")
	}
}

func TestBeginCancelsPendingSubResources(t *testing.T) {
	sched := navigation.NewScheduler()
	// Start a main navigation
	_, navCtx := sched.Begin(context.Background(), "https://example.com/page1")
	// Add sub-resources under that navigation
	_, resCtx := sched.AddResource(navCtx, "https://example.com/style.css", navigation.PriorityBlockingCSS)

	if got := len(sched.PendingLoads()); got != 2 {
		t.Fatalf("PendingLoads() = %d, want 2", got)
	}

	// Start a new navigation — should cancel the previous nav + sub-resources
	sched.Begin(context.Background(), "https://example.com/page2")

	select {
	case <-resCtx.Done():
	default:
		t.Fatal("sub-resource context not cancelled when new navigation started")
	}

	// Only the new navigation should remain in pending
	pending := sched.PendingLoads()
	if len(pending) != 1 {
		t.Fatalf("PendingLoads() = %d, want 1", len(pending))
	}
	if pending[0].URL != "https://example.com/page2" {
		t.Fatalf("pending URL = %q, want page2", pending[0].URL)
	}
}
