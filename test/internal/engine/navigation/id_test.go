package navigation_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/metrics"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
)

func TestInvalidID(t *testing.T) {
	if navigation.Invalid.IsValid() {
		t.Fatal("Invalid ID should not be valid")
	}
	if navigation.Invalid.String() != "0" {
		t.Fatalf("Invalid.String() = %q, want %q", navigation.Invalid.String(), "0")
	}
}

func TestIDGeneratorNextIsMonotonic(t *testing.T) {
	gen := navigation.NewIDGenerator()
	first := gen.Next()
	second := gen.Next()
	if second != first+1 {
		t.Fatalf("second ID = %d, want %d", second, first+1)
	}
}

func TestIDGeneratorNextIsUniqueUnderConcurrency(t *testing.T) {
	gen := navigation.NewIDGenerator()
	const workers = 32
	const perWorker = 100

	seen := make(map[navigation.ID]struct{}, workers*perWorker)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(workers)

	for range workers {
		go func() {
			defer wg.Done()
			for range perWorker {
				id := gen.Next()
				mu.Lock()
				if _, exists := seen[id]; exists {
					t.Errorf("duplicate navigation ID %d", id)
				}
				seen[id] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != workers*perWorker {
		t.Fatalf("generated %d IDs, want %d", len(seen), workers*perWorker)
	}
}

func TestWithIDAndIDFromContext(t *testing.T) {
	ctx := navigation.WithID(context.Background(), navigation.ID(42))
	id, ok := navigation.IDFromContext(ctx)
	if !ok {
		t.Fatal("IDFromContext returned false")
	}
	if id != 42 {
		t.Fatalf("IDFromContext() = %d, want 42", id)
	}
}

func TestIDFromContextMissing(t *testing.T) {
	_, ok := navigation.IDFromContext(context.Background())
	if ok {
		t.Fatal("IDFromContext should return false without navigation ID")
	}
}

func TestIDFromContextCancelledParent(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	ctx := navigation.WithID(parent, navigation.ID(7))
	id, ok := navigation.IDFromContext(ctx)
	if !ok {
		t.Fatal("IDFromContext returned false")
	}
	if id != 7 {
		t.Fatalf("IDFromContext() = %d, want 7", id)
	}
}

func TestSchedulerBeginAssignsUniqueIDs(t *testing.T) {
	sched := navigation.NewScheduler()
	first, _ := sched.Begin(context.Background(), "https://example.com/a")
	second, _ := sched.Begin(context.Background(), "https://example.com/b")

	if !first.ID.IsValid() || !second.ID.IsValid() {
		t.Fatalf("loads should have valid IDs: first=%d second=%d", first.ID, second.ID)
	}
	if first.ID == second.ID {
		t.Fatalf("loads received duplicate IDs: %d", first.ID)
	}
	if first.URL != "https://example.com/a" {
		t.Fatalf("first URL = %q", first.URL)
	}
	if second.URL != "https://example.com/b" {
		t.Fatalf("second URL = %q", second.URL)
	}
	if second.StartedAt.Before(first.StartedAt) {
		t.Fatal("second load started before first load")
	}
}

func TestSchedulerBeginCancelsPreviousContext(t *testing.T) {
	sched := navigation.NewScheduler()
	_, firstCtx := sched.Begin(context.Background(), "https://example.com/first")

	select {
	case <-firstCtx.Done():
		t.Fatal("first context cancelled before second navigation")
	default:
	}

	_, secondCtx := sched.Begin(context.Background(), "https://example.com/second")
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

func TestSchedulerIsActiveRejectsStaleID(t *testing.T) {
	sched := navigation.NewScheduler()
	stale, _ := sched.Begin(context.Background(), "https://example.com/stale")
	current, _ := sched.Begin(context.Background(), "https://example.com/current")

	if !sched.IsActive(current.ID) {
		t.Fatalf("current ID %d should be active", current.ID)
	}
	if sched.IsActive(stale.ID) {
		t.Fatalf("stale ID %d should not be active", stale.ID)
	}
	if sched.ActiveID() != current.ID {
		t.Fatalf("ActiveID() = %d, want %d", sched.ActiveID(), current.ID)
	}
}

func TestSchedulerCancel(t *testing.T) {
	sched := navigation.NewScheduler()
	load, ctx := sched.Begin(context.Background(), "https://example.com")

	sched.Cancel()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("context was not cancelled by Cancel()")
	}
	if sched.IsActive(load.ID) {
		t.Fatalf("ID %d should not be active after Cancel()", load.ID)
	}
	if sched.ActiveID().IsValid() {
		t.Fatalf("ActiveID() = %d after Cancel(), want invalid", sched.ActiveID())
	}
}

func TestSchedulerContextCarriesNavigationID(t *testing.T) {
	sched := navigation.NewScheduler()
	load, ctx := sched.Begin(context.Background(), "https://example.com/id-check")

	id, ok := navigation.IDFromContext(ctx)
	if !ok {
		t.Fatal("navigation context missing ID")
	}
	if id != load.ID {
		t.Fatalf("context ID = %d, load ID = %d", id, load.ID)
	}
}

func TestSchedulerBeginHonorsParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	sched := navigation.NewScheduler()
	_, ctx := sched.Begin(parent, "https://example.com")
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("navigation context was not cancelled when parent is cancelled")
	}
}

func TestSchedulerRepeatedBeginSameURLGetsNewIDs(t *testing.T) {
	sched := navigation.NewScheduler()
	url := "https://example.com/same"
	first, _ := sched.Begin(context.Background(), url)
	second, _ := sched.Begin(context.Background(), url)

	if first.ID == second.ID {
		t.Fatalf("repeated navigation to %q reused ID %d", url, first.ID)
	}
}

func TestLoadHasRecorder(t *testing.T) {
	sched := navigation.NewScheduler()
	load, _ := sched.Begin(context.Background(), "https://example.com/metrics")

	if load.Recorder == nil {
		t.Fatal("Load.Recorder is nil, expected a metrics.Recorder")
	}

	snap := load.Recorder.Snapshot()
	if snap.NavID != uint64(load.ID) {
		t.Fatalf("Recorder NavID = %d, want %d", snap.NavID, load.ID)
	}
	if snap.URL != load.URL {
		t.Fatalf("Recorder URL = %q, want %q", snap.URL, load.URL)
	}
}

func TestLoadRecorderRecordsPhases(t *testing.T) {
	sched := navigation.NewScheduler()
	load, _ := sched.Begin(context.Background(), "https://example.com/phases")

	load.Recorder.BeginPhase(metrics.PhaseParse)
	load.Recorder.EndPhase(metrics.PhaseParse)
	load.Recorder.BeginPhase(metrics.PhaseLayout)
	load.Recorder.EndPhase(metrics.PhaseLayout)

	m := load.Recorder.Finalize()
	if len(m.Timings) != 2 {
		t.Fatalf("got %d timings, want 2", len(m.Timings))
	}
	if m.Timings[0].Phase != metrics.PhaseParse {
		t.Fatalf("timing[0] Phase = %v, want PhaseParse", m.Timings[0].Phase)
	}
	if m.Timings[1].Phase != metrics.PhaseLayout {
		t.Fatalf("timing[1] Phase = %v, want PhaseLayout", m.Timings[1].Phase)
	}
}
