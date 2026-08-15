package renderer

import (
	"sync"
	"sync/atomic"
	"time"
)

// FrameMetrics aggregates per-frame timing samples and event counters so
// the DevTools performance panel and the on-screen FPS HUD can show
// *actionable* numbers rather than the single-counter FPS the original
// implementation reported.
//
// The original FPSCounter labelled any gap between RenderWithViewport
// calls as a "dropped frame". Because Goosie is event-driven (not
// continuously presenting), idle periods inflate that counter and make
// the HUD misleading. FrameMetrics keeps the FPS reading for backwards
// compatibility but also tracks:
//
//   - RenderDuration:        time spent inside the most recent render
//   - InputToPresent:        time from a scroll/mutation trigger to the
//     next presented frame (when set)
//   - UIQueueWait:           time the work spent waiting on the Fyne
//     main thread (when set)
//   - LongFrames:            count of frames whose RenderDuration
//     exceeded the frame budget
//   - CoalescedScrollEvents: count of scroll events collapsed into one
//     render
//   - CoalescedMutations:    count of mutation batches collapsed into
//     one render
//   - StaleFramesDropped:    count of frame requests superseded before
//     they reached the canvas
//
// FrameMetrics is goroutine-safe. The hot path (Observe / Add*) uses
// atomics where possible and a mutex only for the timestamp ring buffer.
type FrameMetrics struct {
	now func() time.Time

	target time.Duration // frame budget; default 16.67ms
	window time.Duration // moving-window span for FPS averaging

	// --- frame interval samples (for FPS / dropped-frames display) ---
	mu      sync.Mutex
	samples []time.Time
	maxSamp int
	prev    time.Time
	total   int64
	dropped int64
	lastInt time.Duration
	minInt  time.Duration
	maxInt  time.Duration

	// --- per-frame duration samples ---
	lastRenderNs  atomic.Int64
	sumRenderNs   atomic.Int64
	maxRenderNs   atomic.Int64
	renderCount   atomic.Int64
	longFrames    atomic.Int64
	longThreshold time.Duration

	// --- input-to-present (set by callers; observed by HUD) ---
	lastInputToPresentNs atomic.Int64
	maxInputToPresentNs  atomic.Int64

	// --- UI queue wait (set by callers; observed by HUD) ---
	lastUIQueueWaitNs atomic.Int64
	maxUIQueueWaitNs  atomic.Int64

	// --- coalesced event counters ---
	coalescedScroll    atomic.Int64
	coalescedMutations atomic.Int64
	coalescedImages    atomic.Int64
	staleFramesDropped atomic.Int64
}

// FrameMetricsSnapshot is the immutable view returned to callers.
type FrameMetricsSnapshot struct {
	// Legacy FPS fields (same semantics as FPSStats) for backwards
	// compatibility with the existing on-screen HUD.
	Frames       int64
	CurrentFPS   float64
	AverageFPS   float64
	MinFPS       float64
	MaxFPS       float64
	Dropped      int64
	SampleWindow time.Duration

	// New actionable fields.
	RenderDuration        time.Duration
	MaxRenderDuration     time.Duration
	InputToPresent        time.Duration
	MaxInputToPresent     time.Duration
	UIQueueWait           time.Duration
	MaxUIQueueWait        time.Duration
	LongFrames            int64
	CoalescedScrollEvents int64
	CoalescedMutations    int64
	CoalescedImages       int64
	StaleFramesDropped    int64
	LongThreshold         time.Duration
}

// NewFrameMetrics creates a metrics struct with the default 60Hz budget.
func NewFrameMetrics() *FrameMetrics {
	return &FrameMetrics{
		now:           time.Now,
		target:        time.Second / 60,
		window:        2 * time.Second,
		maxSamp:       128,
		longThreshold: 20 * time.Millisecond,
	}
}

// NewFrameMetricsWithClock creates a metrics struct driven by a custom
// clock. Used by tests.
func NewFrameMetricsWithClock(now func() time.Time) *FrameMetrics {
	m := NewFrameMetrics()
	m.now = now
	return m
}

// SetLongThreshold configures the duration above which a render counts
// as a "long frame". Default is 20ms.
func (m *FrameMetrics) SetLongThreshold(d time.Duration) {
	if d > 0 {
		m.longThreshold = d
	}
}

// SetTargetFPS overrides the frame budget for drop counting.
func (m *FrameMetrics) SetTargetFPS(fps float64) {
	if fps > 0 {
		m.target = time.Duration(float64(time.Second) / fps)
	}
}

// ObserveFrame records that a frame was presented at the current clock.
// The duration parameter is the wall-clock time spent inside the render
// function; it is added to the rolling statistics.
func (m *FrameMetrics) ObserveFrame(duration time.Duration) {
	now := m.now()
	m.mu.Lock()
	if !m.prev.IsZero() {
		dt := now.Sub(m.prev)
		if dt > 0 {
			m.lastInt = dt
			if m.minInt == 0 || dt < m.minInt {
				m.minInt = dt
			}
			if dt > m.maxInt {
				m.maxInt = dt
			}
			if dt > m.target {
				m.dropped++
			}
		}
	}
	// Always append the timestamp so the rolling window accumulates
	// even when the interval is zero (same-tick re-records).
	m.samples = append(m.samples, now)
	if len(m.samples) > m.maxSamp {
		m.samples = m.samples[len(m.samples)-m.maxSamp:]
	}
	m.prev = now
	m.total++
	m.mu.Unlock()

	ns := duration.Nanoseconds()
	m.lastRenderNs.Store(ns)
	if ns > m.maxRenderNs.Load() {
		m.maxRenderNs.Store(ns)
	}
	m.sumRenderNs.Add(ns)
	m.renderCount.Add(1)
	if duration > m.longThreshold {
		m.longFrames.Add(1)
	}
}

// RecordInputToPresent records the time from a user-input event (scroll,
// mutation) to the frame that presented its result.
func (m *FrameMetrics) RecordInputToPresent(d time.Duration) {
	ns := d.Nanoseconds()
	m.lastInputToPresentNs.Store(ns)
	for {
		cur := m.maxInputToPresentNs.Load()
		if ns <= cur || m.maxInputToPresentNs.CompareAndSwap(cur, ns) {
			break
		}
	}
}

// RecordUIQueueWait records how long a piece of work waited on the Fyne
// main thread. High values here are a direct signal of UI contention.
func (m *FrameMetrics) RecordUIQueueWait(d time.Duration) {
	ns := d.Nanoseconds()
	m.lastUIQueueWaitNs.Store(ns)
	for {
		cur := m.maxUIQueueWaitNs.Load()
		if ns <= cur || m.maxUIQueueWaitNs.CompareAndSwap(cur, ns) {
			break
		}
	}
}

// IncCoalescedScroll bumps the scroll-coalescing counter. Owners call
// this when they collapse N scroll events into one render.
func (m *FrameMetrics) IncCoalescedScroll(n int) {
	if n > 0 {
		m.coalescedScroll.Add(int64(n))
	}
}

// IncCoalescedMutations bumps the mutation-coalescing counter.
func (m *FrameMetrics) IncCoalescedMutations(n int) {
	if n > 0 {
		m.coalescedMutations.Add(int64(n))
	}
}

// IncCoalescedImages bumps the image-load-coalescing counter. Owners call
// this when they collapse N image-loaded callbacks into one render.
func (m *FrameMetrics) IncCoalescedImages(n int) {
	if n > 0 {
		m.coalescedImages.Add(int64(n))
	}
}

// IncStaleFramesDropped bumps the stale-frame counter. Owners call
// this when a render request is superseded by a newer one before the
// Fyne thread got to run it.
func (m *FrameMetrics) IncStaleFramesDropped() {
	m.staleFramesDropped.Add(1)
}

// Snapshot returns a copy of the current metrics.
func (m *FrameMetrics) Snapshot() FrameMetricsSnapshot {
	m.mu.Lock()
	s := FrameMetricsSnapshot{
		Frames:       m.total,
		Dropped:      m.dropped,
		SampleWindow: m.window,
	}
	if m.total > 0 && len(m.samples) >= 2 {
		s.CurrentFPS = fpsFromInterval(m.lastInt)
		s.MinFPS = fpsFromInterval(m.maxInt)
		s.MaxFPS = fpsFromInterval(m.minInt)
		var sum time.Duration
		n := 0
		for i := 1; i < len(m.samples); i++ {
			dt := m.samples[i].Sub(m.samples[i-1])
			if dt > 0 {
				sum += dt
				n++
			}
		}
		if n > 0 {
			s.AverageFPS = fpsFromInterval(sum / time.Duration(n))
		}
		s.SampleWindow = m.samples[len(m.samples)-1].Sub(m.samples[0])
	}
	m.mu.Unlock()

	s.RenderDuration = time.Duration(m.lastRenderNs.Load())
	s.MaxRenderDuration = time.Duration(m.maxRenderNs.Load())
	s.InputToPresent = time.Duration(m.lastInputToPresentNs.Load())
	s.MaxInputToPresent = time.Duration(m.maxInputToPresentNs.Load())
	s.UIQueueWait = time.Duration(m.lastUIQueueWaitNs.Load())
	s.MaxUIQueueWait = time.Duration(m.maxUIQueueWaitNs.Load())
	s.LongFrames = m.longFrames.Load()
	s.CoalescedScrollEvents = m.coalescedScroll.Load()
	s.CoalescedMutations = m.coalescedMutations.Load()
	s.CoalescedImages = m.coalescedImages.Load()
	s.StaleFramesDropped = m.staleFramesDropped.Load()
	s.LongThreshold = m.longThreshold
	return s
}

// Reset clears all counters. Used by tests and on navigation.
func (m *FrameMetrics) Reset() {
	m.mu.Lock()
	m.samples = nil
	m.prev = time.Time{}
	m.total = 0
	m.dropped = 0
	m.lastInt = 0
	m.minInt = 0
	m.maxInt = 0
	m.mu.Unlock()
	m.lastRenderNs.Store(0)
	m.sumRenderNs.Store(0)
	m.maxRenderNs.Store(0)
	m.renderCount.Store(0)
	m.longFrames.Store(0)
	m.lastInputToPresentNs.Store(0)
	m.maxInputToPresentNs.Store(0)
	m.lastUIQueueWaitNs.Store(0)
	m.maxUIQueueWaitNs.Store(0)
	m.coalescedScroll.Store(0)
	m.coalescedMutations.Store(0)
	m.coalescedImages.Store(0)
	m.staleFramesDropped.Store(0)
}
