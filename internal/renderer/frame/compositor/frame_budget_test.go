package compositor

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// FrameBudget — defaults
// ---------------------------------------------------------------------------

func TestDefaultFrameBudget(t *testing.T) {
	b := DefaultFrameBudget()
	want := time.Second / 60
	if b.TargetDuration != want {
		t.Errorf("TargetDuration = %v, want %v", b.TargetDuration, want)
	}
	if b.BudgetThreshold != 0.9 {
		t.Errorf("BudgetThreshold = %f, want 0.9", b.BudgetThreshold)
	}
}

// ---------------------------------------------------------------------------
// FrameBudgetTracker — basic recording
// ---------------------------------------------------------------------------

func TestFrameBudgetTracker_RecordFrame(t *testing.T) {
	tr := NewFrameBudgetTracker(DefaultFrameBudget())

	fn := tr.BeginFrame()
	if fn != 1 {
		t.Errorf("first frame number = %d, want 1", fn)
	}

	now := time.Now()
	tr.RecordFrame(fn, now, now.Add(10*time.Millisecond), 10*time.Millisecond, false)

	stats := tr.Stats()
	if stats.TotalFrames != 1 {
		t.Errorf("TotalFrames = %d, want 1", stats.TotalFrames)
	}
	if stats.DroppedFrames != 0 {
		t.Errorf("DroppedFrames = %d, want 0", stats.DroppedFrames)
	}
}

func TestFrameBudgetTracker_MissedFrame(t *testing.T) {
	budget := FrameBudget{TargetDuration: 16 * time.Millisecond, BudgetThreshold: 0.9}
	tr := NewFrameBudgetTracker(budget)

	fn := tr.BeginFrame()
	now := time.Now()
	// Frame takes 20ms — exceeds 16ms budget.
	tr.RecordFrame(fn, now, now.Add(20*time.Millisecond), 20*time.Millisecond, false)

	stats := tr.Stats()
	if stats.MissedFrames != 1 {
		t.Errorf("MissedFrames = %d, want 1", stats.MissedFrames)
	}
}

func TestFrameBudgetTracker_DroppedFrame(t *testing.T) {
	tr := NewFrameBudgetTracker(DefaultFrameBudget())

	fn := tr.BeginFrame()
	var zero time.Time
	tr.RecordFrame(fn, zero, zero, 5*time.Millisecond, true)

	stats := tr.Stats()
	if stats.DroppedFrames != 1 {
		t.Errorf("DroppedFrames = %d, want 1", stats.DroppedFrames)
	}
}

// ---------------------------------------------------------------------------
// FrameBudgetTracker — latency percentiles
// ---------------------------------------------------------------------------

func TestFrameBudgetTracker_LatencyPercentiles(t *testing.T) {
	tr := NewFrameBudgetTracker(DefaultFrameBudget())

	// Record 100 frames with known latencies.
	for i := 0; i < 100; i++ {
		fn := tr.BeginFrame()
		input := time.Now()
		latency := time.Duration(i+1) * time.Millisecond // 1ms..100ms
		tr.RecordFrame(fn, input, input.Add(latency), latency, false)
	}

	stats := tr.Stats()

	// P50 should be around 50ms.
	if stats.P50Latency < 45*time.Millisecond || stats.P50Latency > 55*time.Millisecond {
		t.Errorf("P50Latency = %v, want ~50ms", stats.P50Latency)
	}

	// P95 should be around 95ms.
	if stats.P95Latency < 90*time.Millisecond || stats.P95Latency > 100*time.Millisecond {
		t.Errorf("P95Latency = %v, want ~95ms", stats.P95Latency)
	}

	// P99 should be around 99ms.
	if stats.P99Latency < 95*time.Millisecond {
		t.Errorf("P99Latency = %v, want ~99ms", stats.P99Latency)
	}
}

func TestFrameBudgetTracker_EmptyStats(t *testing.T) {
	tr := NewFrameBudgetTracker(DefaultFrameBudget())
	stats := tr.Stats()
	if stats.TotalFrames != 0 {
		t.Errorf("TotalFrames = %d, want 0", stats.TotalFrames)
	}
	if stats.P50Latency != 0 || stats.P95Latency != 0 || stats.P99Latency != 0 {
		t.Error("percentiles should be 0 for empty tracker")
	}
}

// ---------------------------------------------------------------------------
// FrameBudgetTracker — ring buffer overflow
// ---------------------------------------------------------------------------

func TestFrameBudgetTracker_RingBufferOverflow(t *testing.T) {
	tr := NewFrameBudgetTracker(DefaultFrameBudget())

	// Record more than maxFrameRecords.
	var zero time.Time
	for i := 0; i < 600; i++ {
		fn := tr.BeginFrame()
		tr.RecordFrame(fn, zero, zero, time.Millisecond, false)
	}

	stats := tr.Stats()
	if stats.TotalFrames != 600 {
		t.Errorf("TotalFrames = %d, want 600", stats.TotalFrames)
	}
	// AvgDuration should still be computed from the last 512 records.
	if stats.AvgDuration != time.Millisecond {
		t.Errorf("AvgDuration = %v, want 1ms", stats.AvgDuration)
	}
}

// ---------------------------------------------------------------------------
// FrameBudgetTracker — Reset
// ---------------------------------------------------------------------------

func TestFrameBudgetTracker_Reset(t *testing.T) {
	tr := NewFrameBudgetTracker(DefaultFrameBudget())
	fn := tr.BeginFrame()
	var zero time.Time
	tr.RecordFrame(fn, zero, zero, time.Millisecond, false)

	tr.Reset()
	stats := tr.Stats()
	if stats.TotalFrames != 0 {
		t.Errorf("TotalFrames after Reset = %d, want 0", stats.TotalFrames)
	}
}

func TestFrameBudgetTracker_RecordInput(t *testing.T) {
	tr := NewFrameBudgetTracker(DefaultFrameBudget())
	inputTime := tr.RecordInput()
	if inputTime.IsZero() {
		t.Error("RecordInput should return non-zero time")
	}
}

func TestFrameBudgetTracker_SanitizesConfig(t *testing.T) {
	tr := NewFrameBudgetTracker(FrameBudget{TargetDuration: -1, BudgetThreshold: 2.0})
	cfg := tr.Config()
	if cfg.TargetDuration != time.Second/60 {
		t.Errorf("sanitized TargetDuration = %v, want %v", cfg.TargetDuration, time.Second/60)
	}
	if cfg.BudgetThreshold != 0.9 {
		t.Errorf("sanitized BudgetThreshold = %f, want 0.9", cfg.BudgetThreshold)
	}
}

// ---------------------------------------------------------------------------
// RasterJobQueue — basic operations
// ---------------------------------------------------------------------------

func TestRasterJobQueue_EnqueueDequeue(t *testing.T) {
	q := NewRasterJobQueue(10)

	job := RasterJob{Coord: TileCoord{Col: 1, Row: 2}, Generation: 1}
	if !q.Enqueue(job) {
		t.Fatal("Enqueue should succeed")
	}

	got, ok := q.Dequeue()
	if !ok {
		t.Fatal("Dequeue should succeed")
	}
	if got.Coord != job.Coord {
		t.Errorf("Coord = %v, want %v", got.Coord, job.Coord)
	}
}

func TestRasterJobQueue_Bounded(t *testing.T) {
	q := NewRasterJobQueue(2)
	q.Enqueue(RasterJob{Coord: TileCoord{Col: 0, Row: 0}})
	q.Enqueue(RasterJob{Coord: TileCoord{Col: 1, Row: 0}})

	if q.Enqueue(RasterJob{Coord: TileCoord{Col: 2, Row: 0}}) {
		t.Error("Enqueue should fail when full")
	}
}

func TestRasterJobQueue_DequeueEmpty(t *testing.T) {
	q := NewRasterJobQueue(10)
	_, ok := q.Dequeue()
	if ok {
		t.Error("Dequeue from empty queue should return false")
	}
}

func TestRasterJobQueue_DequeueSkipsCancelled(t *testing.T) {
	q := NewRasterJobQueue(10)
	coord := TileCoord{Col: 0, Row: 0}
	q.Enqueue(RasterJob{Coord: coord, Generation: 1})
	q.Enqueue(RasterJob{Coord: TileCoord{Col: 1, Row: 0}, Generation: 2})

	q.CancelCoord(coord)

	got, ok := q.Dequeue()
	if !ok {
		t.Fatal("Dequeue should find non-cancelled job")
	}
	if got.Coord != (TileCoord{Col: 1, Row: 0}) {
		t.Errorf("should skip cancelled, got %v", got.Coord)
	}
}

func TestRasterJobQueue_CancelCoord(t *testing.T) {
	q := NewRasterJobQueue(10)
	coord := TileCoord{Col: 3, Row: 4}
	q.Enqueue(RasterJob{Coord: coord})
	q.Enqueue(RasterJob{Coord: coord})
	q.Enqueue(RasterJob{Coord: TileCoord{Col: 0, Row: 0}})

	cancelled := q.CancelCoord(coord)
	if cancelled != 2 {
		t.Errorf("CancelCoord = %d, want 2", cancelled)
	}
	if q.ActiveLen() != 1 {
		t.Errorf("ActiveLen = %d, want 1", q.ActiveLen())
	}
}

func TestRasterJobQueue_CancelOutside(t *testing.T) {
	q := NewRasterJobQueue(10)
	q.Enqueue(RasterJob{Coord: TileCoord{Col: 0, Row: 0}})
	q.Enqueue(RasterJob{Coord: TileCoord{Col: 1, Row: 0}})
	q.Enqueue(RasterJob{Coord: TileCoord{Col: 2, Row: 0}})

	keep := map[TileCoord]bool{
		{Col: 0, Row: 0}: true,
	}
	cancelled := q.CancelOutside(keep)
	if cancelled != 2 {
		t.Errorf("CancelOutside = %d, want 2", cancelled)
	}
	if q.ActiveLen() != 1 {
		t.Errorf("ActiveLen = %d, want 1", q.ActiveLen())
	}
}

func TestRasterJobQueue_Clear(t *testing.T) {
	q := NewRasterJobQueue(10)
	q.Enqueue(RasterJob{Coord: TileCoord{Col: 0, Row: 0}})
	q.Enqueue(RasterJob{Coord: TileCoord{Col: 1, Row: 0}})

	q.Clear()
	if q.Len() != 0 {
		t.Errorf("Len after Clear = %d, want 0", q.Len())
	}
}

func TestRasterJobQueue_DefaultMax(t *testing.T) {
	q := NewRasterJobQueue(0)
	if q.max != 256 {
		t.Errorf("default max = %d, want 256", q.max)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkFrameBudgetTracker_RecordFrame(b *testing.B) {
	tr := NewFrameBudgetTracker(DefaultFrameBudget())
	now := time.Now()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		fn := tr.BeginFrame()
		tr.RecordFrame(fn, now, now.Add(time.Millisecond), time.Millisecond, false)
	}
}

func BenchmarkFrameBudgetTracker_Stats(b *testing.B) {
	tr := NewFrameBudgetTracker(DefaultFrameBudget())
	var zero2 time.Time
	for i := 0; i < 512; i++ {
		fn := tr.BeginFrame()
		tr.RecordFrame(fn, zero2, zero2, time.Millisecond, false)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tr.Stats()
	}
}

func BenchmarkRasterJobQueue_EnqueueDequeue(b *testing.B) {
	q := NewRasterJobQueue(b.N + 1)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		q.Enqueue(RasterJob{Coord: TileCoord{Col: int32(i), Row: 0}})
	}
	for i := 0; i < b.N; i++ {
		q.Dequeue()
	}
}
