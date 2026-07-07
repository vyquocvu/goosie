package metrics

import (
	"sync"
	"testing"
	"time"
)

func TestNewRecorder(t *testing.T) {
	r := NewRecorder(42, "https://example.com")
	if r == nil {
		t.Fatal("NewRecorder returned nil")
	}

	snap := r.Snapshot()
	if snap.NavID != 42 {
		t.Fatalf("NavID = %d, want 42", snap.NavID)
	}
	if snap.URL != "https://example.com" {
		t.Fatalf("URL = %q, want %q", snap.URL, "https://example.com")
	}
	if snap.StartedAt.IsZero() {
		t.Fatal("StartedAt should not be zero")
	}
	if !snap.EndedAt.IsZero() {
		t.Fatal("EndedAt should be zero before finalize")
	}
}

func TestRecorderBeginEndPhase(t *testing.T) {
	r := NewRecorder(1, "http://test.dev")
	r.BeginPhase(PhaseParse)
	r.EndPhase(PhaseParse)

	snap := r.Snapshot()
	if len(snap.Timings) != 1 {
		t.Fatalf("got %d timings, want 1", len(snap.Timings))
	}
	if snap.Timings[0].Phase != PhaseParse {
		t.Fatalf("Phase = %v, want %v", snap.Timings[0].Phase, PhaseParse)
	}
	if snap.Timings[0].Duration() <= 0 {
		t.Fatal("phase duration should be positive")
	}
}

func TestRecorderBeginEndPhaseReturnsDuration(t *testing.T) {
	r := NewRecorder(1, "http://test.dev")
	r.BeginPhase(PhaseLayout)
	time.Sleep(time.Millisecond)
	r.EndPhase(PhaseLayout)

	snap := r.Snapshot()
	dur := snap.PhaseDuration(PhaseLayout)
	if dur < time.Millisecond {
		t.Fatalf("PhaseDuration = %v, want >= 1ms", dur)
	}
}

func TestRecorderEndPhaseWithoutBeginIsNoOp(t *testing.T) {
	r := NewRecorder(1, "http://test.dev")
	r.EndPhase(PhaseParse)
	snap := r.Snapshot()
	if len(snap.Timings) != 0 {
		t.Fatalf("got %d timings, want 0 for no-op EndPhase", len(snap.Timings))
	}
}

func TestRecorderDoubleBeginEndsFirst(t *testing.T) {
	r := NewRecorder(1, "http://test.dev")
	r.BeginPhase(PhaseParse)
	time.Sleep(time.Microsecond)
	r.BeginPhase(PhaseParse)
	r.EndPhase(PhaseParse)

	snap := r.Snapshot()
	if len(snap.Timings) != 2 {
		t.Fatalf("got %d timings, want 2 for double BeginPhase", len(snap.Timings))
	}
	if snap.Timings[0].Phase != PhaseParse {
		t.Fatalf("first timing Phase = %v, want PhaseParse", snap.Timings[0].Phase)
	}
	if snap.Timings[1].Phase != PhaseParse {
		t.Fatalf("second timing Phase = %v, want PhaseParse", snap.Timings[1].Phase)
	}
	if snap.Timings[0].Duration() <= 0 {
		t.Fatal("first phase duration should be positive")
	}
}

func TestRecorderFinalizeClosesOpenPhases(t *testing.T) {
	r := NewRecorder(1, "http://test.dev")
	r.BeginPhase(PhaseParse)
	r.BeginPhase(PhaseLayout)

	metrics := r.Finalize()
	if len(metrics.Timings) != 2 {
		t.Fatalf("got %d timings, want 2 after finalize", len(metrics.Timings))
	}
	if metrics.EndedAt.IsZero() {
		t.Fatal("EndedAt should be set after finalize")
	}
	if metrics.TotalDuration() <= 0 {
		t.Fatal("TotalDuration should be positive")
	}
}

func TestRecorderFinalizeReturnsCounters(t *testing.T) {
	r := NewRecorder(1, "http://test.dev")
	r.AddCounters(Counters{NodeCount: 100, RuleCount: 25})
	r.AddCounters(Counters{BytesDownloaded: 4096})

	metrics := r.Finalize()
	if metrics.Counters.NodeCount != 100 {
		t.Fatalf("NodeCount = %d, want 100", metrics.Counters.NodeCount)
	}
	if metrics.Counters.RuleCount != 25 {
		t.Fatalf("RuleCount = %d, want 25", metrics.Counters.RuleCount)
	}
	if metrics.Counters.BytesDownloaded != 4096 {
		t.Fatalf("BytesDownloaded = %d, want 4096", metrics.Counters.BytesDownloaded)
	}
}

func TestRecorderFinalizeCapturesRuntimeState(t *testing.T) {
	r := NewRecorder(1, "http://test.dev")
	metrics := r.Finalize()

	if metrics.Goroutines <= 0 {
		t.Fatalf("Goroutines = %d, want > 0", metrics.Goroutines)
	}
	if metrics.HeapAlloc <= 0 {
		t.Fatalf("HeapAlloc = %d, want > 0", metrics.HeapAlloc)
	}
}

func TestRecorderPostFinalizeIsNoOp(t *testing.T) {
	r := NewRecorder(1, "http://test.dev")
	r.Finalize()

	r.BeginPhase(PhaseParse) // should not panic
	r.EndPhase(PhaseParse)   // should not panic
	r.AddCounters(Counters{NodeCount: 1})

	// Snapshot after finalize should return empty data
	// (the implementation should prevent further recording)
}

func TestRecorderSnapshotIsolation(t *testing.T) {
	r := NewRecorder(1, "http://test.dev")
	r.BeginPhase(PhaseParse)
	r.AddCounters(Counters{NodeCount: 50})

	snap1 := r.Snapshot()
	r.EndPhase(PhaseParse)
	r.AddCounters(Counters{NodeCount: 25})

	snap2 := r.Snapshot()

	if snap1.Counters.NodeCount != 50 {
		t.Fatalf("snap1 NodeCount = %d, want 50", snap1.Counters.NodeCount)
	}
	if snap2.Counters.NodeCount != 75 {
		t.Fatalf("snap2 NodeCount = %d, want 75", snap2.Counters.NodeCount)
	}
	if len(snap2.Timings) != 1 {
		t.Fatalf("snap2 timings = %d, want 1", len(snap2.Timings))
	}
}

func TestRecorderMultiplePhases(t *testing.T) {
	r := NewRecorder(1, "http://test.dev")
	phases := []Phase{PhaseNavigation, PhaseDNSResolve, PhaseConnect, PhaseFirstByte, PhaseBodyRead, PhaseParse, PhaseStyle, PhaseLayout, PhasePaint, PhaseRaster, PhasePresent}

	for _, p := range phases {
		r.BeginPhase(p)
		r.EndPhase(p)
	}

	metrics := r.Finalize()
	if len(metrics.Timings) != len(phases) {
		t.Fatalf("got %d timings, want %d", len(metrics.Timings), len(phases))
	}
	for i, p := range phases {
		if metrics.Timings[i].Phase != p {
			t.Fatalf("timing[%d] Phase = %v, want %v", i, metrics.Timings[i].Phase, p)
		}
	}
}

func TestRecorderConcurrentSafe(t *testing.T) {
	r := NewRecorder(1, "http://test.dev")
	var wg sync.WaitGroup

	for _, p := range []Phase{PhaseParse, PhaseStyle, PhaseLayout, PhasePaint} {
		wg.Add(1)
		go func(ph Phase) {
			defer wg.Done()
			r.BeginPhase(ph)
			r.EndPhase(ph)
		}(p)
	}

	wg.Wait()

	// Add some counters concurrently
	wg.Add(3)
	for range 3 {
		go func() {
			defer wg.Done()
			r.AddCounters(Counters{NodeCount: 10, RuleCount: 5})
		}()
	}
	wg.Wait()

	snap := r.Snapshot()
	if snap.Counters.NodeCount != 30 {
		t.Fatalf("NodeCount = %d, want 30", snap.Counters.NodeCount)
	}
	if snap.Counters.RuleCount != 15 {
		t.Fatalf("RuleCount = %d, want 15", snap.Counters.RuleCount)
	}
}

func TestRecorderConcurrentSafeWithFinalize(t *testing.T) {
	r := NewRecorder(1, "http://test.dev")
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.BeginPhase(PhaseParse)
			r.EndPhase(PhaseParse)
			r.AddCounters(Counters{NodeCount: 1})
		}()
	}

	wg.Wait()

	metrics := r.Finalize()
	if metrics.Counters.NodeCount != 20 {
		t.Fatalf("NodeCount = %d, want 20", metrics.Counters.NodeCount)
	}
}

func TestPhaseDurationMissing(t *testing.T) {
	m := Metrics{NavID: 1}
	dur := m.PhaseDuration(PhaseParse)
	if dur != 0 {
		t.Fatalf("PhaseDuration for missing phase = %v, want 0", dur)
	}
}

func TestPhaseString(t *testing.T) {
	tests := []struct {
		p    Phase
		want string
	}{
		{PhaseNavigation, "navigation"},
		{PhaseDNSResolve, "dns_resolve"},
		{PhaseConnect, "connect"},
		{PhaseFirstByte, "first_byte"},
		{PhaseBodyRead, "body_read"},
		{PhaseParse, "parse"},
		{PhaseStyle, "style"},
		{PhaseLayout, "layout"},
		{PhasePaint, "paint"},
		{PhaseRaster, "raster"},
		{PhasePresent, "present"},
		{Phase(99), "phase_99"},
	}

	for _, tt := range tests {
		got := tt.p.String()
		if got != tt.want {
			t.Errorf("Phase(%d).String() = %q, want %q", int(tt.p), got, tt.want)
		}
	}
}
