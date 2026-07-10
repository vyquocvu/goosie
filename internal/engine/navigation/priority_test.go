package navigation

import (
	"context"
	"sort"
	"sync"
	"testing"
)

func TestPriorityConstants(t *testing.T) {
	// Verify ordering: Document is highest priority (lowest value),
	// Speculative is lowest priority (highest value).
	priorities := []Priority{
		PriorityDocument,
		PriorityBlockingCSS,
		PriorityVisibleImage,
		PriorityScript,
		PriorityDeferredImage,
		PrioritySpeculative,
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
		p    Priority
		want string
	}{
		{PriorityDocument, "document"},
		{PriorityBlockingCSS, "blocking_css"},
		{PriorityVisibleImage, "visible_image"},
		{PriorityScript, "script"},
		{PriorityDeferredImage, "deferred_image"},
		{PrioritySpeculative, "speculative"},
		{Priority(99), "unknown_priority_99"},
	}
	for _, c := range cases {
		got := c.p.String()
		if got != c.want {
			t.Errorf("Priority(%d).String() = %q, want %q", c.p, got, c.want)
		}
	}
}

func TestSchedulerBeginDefaultsToDocumentPriority(t *testing.T) {
	sched := NewScheduler()
	load, _ := sched.Begin(context.Background(), "https://example.com")
	if load.Priority != PriorityDocument {
		t.Fatalf("Begin() load.Priority = %v, want PriorityDocument", load.Priority)
	}
}

func TestSchedulerBeginWithPriority(t *testing.T) {
	sched := NewScheduler()
	tests := []struct {
		prio Priority
		url  string
	}{
		{PriorityBlockingCSS, "https://example.com/style.css"},
		{PriorityVisibleImage, "https://example.com/hero.png"},
		{PriorityScript, "https://example.com/app.js"},
		{PriorityDeferredImage, "https://example.com/thumb.png"},
		{PrioritySpeculative, "https://example.com/prefetch"},
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
		got, ok := PriorityFromContext(ctx)
		if !ok {
			t.Errorf("PriorityFromContext returned false for %v", tc.prio)
		}
		if got != tc.prio {
			t.Errorf("PriorityFromContext() = %v, want %v", got, tc.prio)
		}
	}
}

func TestPriorityFromContextMissing(t *testing.T) {
	_, ok := PriorityFromContext(context.Background())
	if ok {
		t.Fatal("PriorityFromContext should return false without priority")
	}
}

func TestPriorityContextRoundTrip(t *testing.T) {
	ctx := WithPriority(context.Background(), PriorityScript)
	got, ok := PriorityFromContext(ctx)
	if !ok {
		t.Fatal("PriorityFromContext returned false")
	}
	if got != PriorityScript {
		t.Fatalf("PriorityFromContext() = %v, want PriorityScript", got)
	}
}

func TestSchedulerPendingLoads(t *testing.T) {
	sched := NewScheduler()
	// Start a main navigation (Begin adds to pending with PriorityDocument)
	docLoad, _ := sched.Begin(context.Background(), "https://example.com/doc")
	// Add sub-resources with different priorities
	cssLoad, _ := sched.AddResource(context.Background(), "https://example.com/style.css", PriorityBlockingCSS)
	specLoad, _ := sched.AddResource(context.Background(), "https://example.com/spec", PrioritySpeculative)

	pending := sched.PendingLoads()
	if len(pending) != 3 {
		t.Fatalf("PendingLoads() returned %d loads, want 3", len(pending))
	}

	// Should be sorted by priority (lowest value = highest priority first)
	expected := []ID{docLoad.ID, cssLoad.ID, specLoad.ID}
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
	sched := NewScheduler()
	sched.AddResource(context.Background(), "https://example.com/a", PriorityScript)
	sched.AddResource(context.Background(), "https://example.com/b", PriorityVisibleImage)

	if got := len(sched.PendingLoads()); got != 2 {
		t.Fatalf("PendingLoads() = %d, want 2", got)
	}

	sched.Cancel()

	if got := len(sched.PendingLoads()); got != 0 {
		t.Fatalf("PendingLoads() after Cancel() = %d, want 0", got)
	}
}

func TestSchedulerPendingLoadsCleanupOnNewBegin(t *testing.T) {
	sched := NewScheduler()
	// First navigation creates a pending load
	sched.BeginWithPriority(context.Background(), "https://example.com/first", PriorityDocument)
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
	sched := NewScheduler()
	const workers = 16
	const perWorker = 50

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := range workers {
		go func(wid int) {
			defer wg.Done()
			prio := Priority(wid%int(PrioritySpeculative) + 1)
			for j := range perWorker {
				_ = j
				load, ctx := sched.AddResource(context.Background(), "https://example.com/concurrent", prio)
				if !load.ID.IsValid() {
					t.Errorf("invalid load ID from concurrent AddResource")
				}
				// Verify context carries priority
				if got, ok := PriorityFromContext(ctx); !ok || got != prio {
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
	sched := NewScheduler()
	_, firstCtx := sched.BeginWithPriority(context.Background(), "https://example.com/first", PriorityScript)

	select {
	case <-firstCtx.Done():
		t.Fatal("first context cancelled before second navigation")
	default:
	}

	_, secondCtx := sched.BeginWithPriority(context.Background(), "https://example.com/second", PriorityDocument)
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
	sched := NewScheduler()
	load, _ := sched.BeginWithPriority(context.Background(), "https://example.com", PriorityVisibleImage)
	// Modifying the returned Load should not affect the tracked pending load
	load.Priority = PrioritySpeculative

	pending := sched.PendingLoads()
	found := false
	for _, p := range pending {
		if p.ID == load.ID {
			found = true
			if p.Priority != PriorityVisibleImage {
				t.Errorf("pending load priority mutated: got %v, want PriorityVisibleImage", p.Priority)
			}
		}
	}
	if !found {
		t.Error("load not found in PendingLoads after external mutation")
	}
}

func TestAddResourceReturnsUniqueIDs(t *testing.T) {
	sched := NewScheduler()
	load1, _ := sched.AddResource(context.Background(), "https://example.com/a", PriorityScript)
	load2, _ := sched.AddResource(context.Background(), "https://example.com/b", PriorityScript)
	if load1.ID == load2.ID {
		t.Fatalf("AddResource returned duplicate IDs: %d", load1.ID)
	}
}

func TestAddResourceContextCarriesIDAndPriority(t *testing.T) {
	sched := NewScheduler()
	load, ctx := sched.AddResource(context.Background(), "https://example.com/style.css", PriorityBlockingCSS)

	gotID, ok := IDFromContext(ctx)
	if !ok {
		t.Fatal("IDFromContext returned false")
	}
	if gotID != load.ID {
		t.Fatalf("IDFromContext() = %d, want %d", gotID, load.ID)
	}

	gotPrio, ok := PriorityFromContext(ctx)
	if !ok {
		t.Fatal("PriorityFromContext returned false")
	}
	if gotPrio != PriorityBlockingCSS {
		t.Fatalf("PriorityFromContext() = %v, want PriorityBlockingCSS", gotPrio)
	}
}

func TestRemoveResource(t *testing.T) {
	sched := NewScheduler()
	load1, ctx1 := sched.AddResource(context.Background(), "https://example.com/a", PriorityScript)
	load2, _ := sched.AddResource(context.Background(), "https://example.com/b", PriorityVisibleImage)

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
	sched := NewScheduler()
	load, _ := sched.AddResource(context.Background(), "https://example.com/a", PriorityScript)

	sched.RemoveResource(load.ID)
	// Second call should not panic
	sched.RemoveResource(load.ID)

	if got := len(sched.PendingLoads()); got != 0 {
		t.Fatalf("PendingLoads() = %d, want 0", got)
	}
}

func TestRemoveResourceUnknownID(t *testing.T) {
	sched := NewScheduler()
	// Should not panic on unknown ID
	sched.RemoveResource(ID(9999))
	if got := len(sched.PendingLoads()); got != 0 {
		t.Fatalf("PendingLoads() = %d, want 0", got)
	}
}

func TestAddResourceContextCancelledByParent(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	sched := NewScheduler()
	_, ctx := sched.AddResource(parent, "https://example.com/a", PriorityScript)

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
	sched := NewScheduler()
	// Start a main navigation
	_, navCtx := sched.Begin(context.Background(), "https://example.com/page1")
	// Add sub-resources under that navigation
	_, resCtx := sched.AddResource(navCtx, "https://example.com/style.css", PriorityBlockingCSS)

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
