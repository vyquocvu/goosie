// Package compositor — frame budget and latency tracking (M7.3)
//
// Provides FrameBudgetTracker for measuring frame timing, input-to-present
// latency percentiles, and dropped/missed frame detection.
package compositor

import (
	"sort"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// FrameBudget — target frame timing
// ---------------------------------------------------------------------------

// FrameBudget defines the target frame duration and thresholds.
type FrameBudget struct {
	// TargetDuration is the desired frame interval (e.g. 16.67ms for 60fps).
	TargetDuration time.Duration

	// BudgetThreshold is the fraction of TargetDuration at which we
	// consider the frame "at risk" (default 0.9).
	BudgetThreshold float64
}

// DefaultFrameBudget returns a 60fps target (16.67ms).
func DefaultFrameBudget() FrameBudget {
	return FrameBudget{
		TargetDuration:  time.Second / 60,
		BudgetThreshold: 0.9,
	}
}

// ---------------------------------------------------------------------------
// FrameRecord — per-frame timing data
// ---------------------------------------------------------------------------

// FrameRecord captures timing for a single frame.
type FrameRecord struct {
	// FrameNumber is a monotonically increasing counter.
	FrameNumber uint64

	// InputTime is when the input event (scroll, etc.) was received.
	InputTime time.Time

	// PresentTime is when the frame was presented to the screen.
	PresentTime time.Time

	// Duration is the total frame processing time.
	Duration time.Duration

	// Dropped is true if the frame was skipped (e.g. cancelled raster).
	Dropped bool

	// Missed is true if the frame exceeded the target budget.
	Missed bool
}

// Latency returns the input-to-present duration.
func (r FrameRecord) Latency() time.Duration {
	if r.InputTime.IsZero() {
		return r.Duration
	}
	return r.PresentTime.Sub(r.InputTime)
}

// ---------------------------------------------------------------------------
// FrameBudgetTracker — bounded frame timing recorder
// ---------------------------------------------------------------------------

const maxFrameRecords = 512

// FrameBudgetTracker records frame timing and computes latency percentiles.
// It uses a bounded ring buffer to avoid unbounded memory growth.
type FrameBudgetTracker struct {
	mu      sync.Mutex
	budget  FrameBudget
	records [maxFrameRecords]FrameRecord
	head    int // next write position
	count   int // number of valid records (≤ maxFrameRecords)

	// frameCounter is a monotonically increasing frame number.
	frameCounter uint64

	// Counters.
	totalFrames   uint64
	droppedFrames uint64
	missedFrames  uint64
}

// NewFrameBudgetTracker creates a tracker with the given frame budget.
func NewFrameBudgetTracker(budget FrameBudget) *FrameBudgetTracker {
	if budget.TargetDuration <= 0 {
		budget.TargetDuration = time.Second / 60
	}
	if budget.BudgetThreshold <= 0 || budget.BudgetThreshold > 1 {
		budget.BudgetThreshold = 0.9
	}
	return &FrameBudgetTracker{budget: budget}
}

// BeginFrame records the start of a frame and returns the frame number.
func (t *FrameBudgetTracker) BeginFrame() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.frameCounter++
	return t.frameCounter
}

// RecordFrame records a completed frame. If inputTime is zero, the frame
// had no associated input event.
func (t *FrameBudgetTracker) RecordFrame(frameNum uint64, inputTime time.Time, presentTime time.Time, duration time.Duration, dropped bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	rec := FrameRecord{
		FrameNumber: frameNum,
		InputTime:   inputTime,
		PresentTime: presentTime,
		Duration:    duration,
		Dropped:     dropped,
		Missed:      duration > t.budget.TargetDuration,
	}

	// Write to ring buffer.
	t.records[t.head] = rec
	t.head = (t.head + 1) % maxFrameRecords
	if t.count < maxFrameRecords {
		t.count++
	}

	t.totalFrames++
	if dropped {
		t.droppedFrames++
	}
	if rec.Missed {
		t.missedFrames++
	}
}

// RecordInput records an input event timestamp for latency tracking.
// Returns the input time to be passed to RecordFrame later.
func (t *FrameBudgetTracker) RecordInput() time.Time {
	return time.Now()
}

// Stats returns summary statistics for recorded frames.
type FrameBudgetStats struct {
	TotalFrames   uint64
	DroppedFrames uint64
	MissedFrames  uint64

	// Latency percentiles (nanoseconds). Computed from non-dropped frames.
	P50Latency time.Duration
	P95Latency time.Duration
	P99Latency time.Duration

	// Average frame duration.
	AvgDuration time.Duration
}

// Stats computes summary statistics from recorded frames.
func (t *FrameBudgetTracker) Stats() FrameBudgetStats {
	t.mu.Lock()
	defer t.mu.Unlock()

	stats := FrameBudgetStats{
		TotalFrames:   t.totalFrames,
		DroppedFrames: t.droppedFrames,
		MissedFrames:  t.missedFrames,
	}

	if t.count == 0 {
		return stats
	}

	// Collect valid records.
	latencies := make([]time.Duration, 0, t.count)
	var totalDuration time.Duration
	validCount := 0

	start := 0
	if t.count == maxFrameRecords {
		start = t.head // oldest record
	}

	for i := 0; i < t.count; i++ {
		idx := (start + i) % maxFrameRecords
		rec := t.records[idx]
		totalDuration += rec.Duration
		validCount++
		if !rec.Dropped {
			latencies = append(latencies, rec.Latency())
		}
	}

	if validCount > 0 {
		stats.AvgDuration = totalDuration / time.Duration(validCount)
	}

	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool {
			return latencies[i] < latencies[j]
		})
		stats.P50Latency = percentile(latencies, 50)
		stats.P95Latency = percentile(latencies, 95)
		stats.P99Latency = percentile(latencies, 99)
	}

	return stats
}

// Reset clears all recorded data and counters.
func (t *FrameBudgetTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.head = 0
	t.count = 0
	t.frameCounter = 0
	t.totalFrames = 0
	t.droppedFrames = 0
	t.missedFrames = 0
}

// Config returns the frame budget configuration.
func (t *FrameBudgetTracker) Config() FrameBudget {
	return t.budget
}

// percentile returns the p-th percentile from a sorted slice of durations.
func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := (len(sorted) - 1) * p / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// ---------------------------------------------------------------------------
// RasterJob — cancellable raster work unit
// ---------------------------------------------------------------------------

// RasterJob represents a pending rasterization task for a tile.
// It can be cancelled if the tile leaves the priority area.
type RasterJob struct {
	Coord      TileCoord
	Generation uint64
	Cancelled  bool
}

// RasterJobQueue is a bounded queue of pending raster jobs with
// cancellation support. Jobs for tiles outside the priority area
// can be cancelled to avoid wasted work.
type RasterJobQueue struct {
	mu   sync.Mutex
	jobs []RasterJob
	Max  int
}

// NewRasterJobQueue creates a bounded job queue.
func NewRasterJobQueue(maxJobs int) *RasterJobQueue {
	if maxJobs <= 0 {
		maxJobs = 256
	}
	return &RasterJobQueue{
		jobs: make([]RasterJob, 0, maxJobs),
		Max:  maxJobs,
	}
}

// Enqueue adds a raster job. Returns false if the queue is full.
func (q *RasterJobQueue) Enqueue(job RasterJob) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.jobs) >= q.Max {
		return false
	}
	q.jobs = append(q.jobs, job)
	return true
}

// Dequeue removes and returns the next non-cancelled job.
// Returns ok=false if the queue is empty.
func (q *RasterJobQueue) Dequeue() (RasterJob, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.jobs) > 0 {
		job := q.jobs[0]
		q.jobs = q.jobs[1:]
		if !job.Cancelled {
			return job, true
		}
	}
	return RasterJob{}, false
}

// CancelCoord marks all pending jobs for the given coordinate as cancelled.
// Returns the number of cancelled jobs.
func (q *RasterJobQueue) CancelCoord(coord TileCoord) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	count := 0
	for i := range q.jobs {
		if q.jobs[i].Coord == coord && !q.jobs[i].Cancelled {
			q.jobs[i].Cancelled = true
			count++
		}
	}
	return count
}

// CancelOutside marks all pending jobs whose coordinates are NOT in the
// given set as cancelled. Returns the number of cancelled jobs.
func (q *RasterJobQueue) CancelOutside(keep map[TileCoord]bool) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	count := 0
	for i := range q.jobs {
		if !keep[q.jobs[i].Coord] && !q.jobs[i].Cancelled {
			q.jobs[i].Cancelled = true
			count++
		}
	}
	return count
}

// Len returns the number of pending jobs (including cancelled).
func (q *RasterJobQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.jobs)
}

// ActiveLen returns the number of non-cancelled pending jobs.
func (q *RasterJobQueue) ActiveLen() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	count := 0
	for _, j := range q.jobs {
		if !j.Cancelled {
			count++
		}
	}
	return count
}

// Clear removes all pending jobs.
func (q *RasterJobQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = q.jobs[:0]
}
