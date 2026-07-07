package metrics

import (
	"runtime"
	"sync"
	"time"
)

// Recorder collects metrics for one navigation. Safe for concurrent use.
// After Finalize is called, all subsequent operations become no-ops.
type Recorder struct {
	mu       sync.Mutex
	navID    uint64
	url      string
	start    time.Time
	timings  []Timing
	open     map[Phase]time.Time
	counters Counters
	done     bool
}

// NewRecorder creates a Recorder for the given navigation ID and URL.
func NewRecorder(navID uint64, url string) *Recorder {
	return &Recorder{
		navID: navID,
		url:   url,
		start: time.Now(),
		open:  make(map[Phase]time.Time, 8),
	}
}

// BeginPhase records the start time of a phase. If the phase was already
// started without a matching EndPhase, the first interval is closed and a
// new interval begins. Safe to call from any goroutine.
func (r *Recorder) BeginPhase(p Phase) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return
	}
	if prevStart, exists := r.open[p]; exists {
		r.timings = append(r.timings, Timing{
			Phase:   p,
			Started: prevStart,
			Ended:   time.Now(),
		})
	}
	r.open[p] = time.Now()
}

// EndPhase records the end time of a phase. If the phase was not started
// (or was already closed), this is a no-op.
func (r *Recorder) EndPhase(p Phase) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return
	}
	if start, exists := r.open[p]; exists {
		r.timings = append(r.timings, Timing{
			Phase:   p,
			Started: start,
			Ended:   time.Now(),
		})
		delete(r.open, p)
	}
}

// AddCounters atomically adds the given counter values to the running totals.
func (r *Recorder) AddCounters(c Counters) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return
	}
	r.counters.NodeCount += c.NodeCount
	r.counters.RuleCount += c.RuleCount
	r.counters.SelectorCount += c.SelectorCount
	r.counters.BoxCount += c.BoxCount
	r.counters.FragmentCount += c.FragmentCount
	r.counters.DisplayItemCount += c.DisplayItemCount
	r.counters.TileCount += c.TileCount
	r.counters.ImageCount += c.ImageCount
	r.counters.BytesDownloaded += c.BytesDownloaded
	r.counters.DecodedImageBytes += c.DecodedImageBytes
	r.counters.CacheHits += c.CacheHits
	r.counters.CacheMisses += c.CacheMisses
	r.counters.ScriptErrors += c.ScriptErrors
}

// Snapshot returns a point-in-time copy of the current metrics for
// diagnostic display. This calls runtime.ReadMemStats and should not
// be used on hot paths.
func (r *Recorder) Snapshot() Metrics {
	r.mu.Lock()
	defer r.mu.Unlock()

	timings := make([]Timing, len(r.timings))
	copy(timings, r.timings)

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return Metrics{
		NavID:      r.navID,
		URL:        r.url,
		StartedAt:  r.start,
		Timings:    timings,
		Counters:   r.counters,
		Goroutines: runtime.NumGoroutine(),
		HeapAlloc:  m.Alloc,
	}
}

// Finalize ends all open phases, captures final runtime state, and returns
// the complete Metrics. After Finalize, the Recorder must not be used.
func (r *Recorder) Finalize() Metrics {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for p, start := range r.open {
		r.timings = append(r.timings, Timing{
			Phase:   p,
			Started: start,
			Ended:   now,
		})
	}
	r.open = nil

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	r.done = true

	return Metrics{
		NavID:      r.navID,
		URL:        r.url,
		StartedAt:  r.start,
		EndedAt:    now,
		Timings:    r.timings,
		Counters:   r.counters,
		Goroutines: runtime.NumGoroutine(),
		HeapAlloc:  m.Alloc,
	}
}
