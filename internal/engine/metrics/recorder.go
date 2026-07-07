package metrics

import (
	"context"
	"log/slog"
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

	// debug logging is opt-in. logger is read under mu to stay race-free.
	debug  bool
	logger *slog.Logger
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

// SetDebugLog enables or disables structured debug logging for this recorder.
// When logger is non-nil, debug logging is enabled and a structured record is
// emitted on Finalize and on explicit LogStructured calls. Passing nil disables
// debug logging. Safe to call from any goroutine, but it is expected to be set
// once before a navigation begins.
func (r *Recorder) SetDebugLog(logger *slog.Logger) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.debug = logger != nil
	r.logger = logger
}

// LogStructured emits a structured debug record for the current state of the
// recorder. It is a no-op when debug logging is disabled, when no logger is set,
// or after Finalize has been called. Safe for concurrent use.
func (r *Recorder) LogStructured(ctx context.Context) {
	r.mu.Lock()
	if r.done || !r.debug || r.logger == nil {
		r.mu.Unlock()
		return
	}
	m := r.buildSnapshotLocked()
	logger := r.logger
	r.mu.Unlock()

	if ctx == nil {
		ctx = context.Background()
	}
	logMetrics(logger, m, ctx)
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
	return r.buildSnapshotLocked()
}

// buildSnapshotLocked builds a Metrics snapshot. The caller must hold r.mu.
func (r *Recorder) buildSnapshotLocked() Metrics {
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

// Finalize ends all open phases, captures final runtime state, emits a
// structured debug record when enabled, and returns the complete Metrics.
// After Finalize, the Recorder must not be used.
func (r *Recorder) Finalize() Metrics {
	r.mu.Lock()

	now := time.Now()
	for p, start := range r.open {
		r.timings = append(r.timings, Timing{
			Phase:   p,
			Started: start,
			Ended:   now,
		})
	}
	r.open = nil

	m := r.buildSnapshotLocked()
	m.EndedAt = now
	r.done = true

	debug := r.debug
	logger := r.logger
	r.mu.Unlock()

	if debug && logger != nil {
		logMetrics(logger, m, context.Background())
	}

	return m
}

// logMetrics writes a structured debug record describing one navigation.
func logMetrics(l *slog.Logger, m Metrics, ctx context.Context) {
	attrs := []slog.Attr{
		slog.Int64("nav_id", int64(m.NavID)),
		slog.String("url", m.URL),
		slog.Int64("total_ns", m.TotalDuration().Nanoseconds()),
		slog.Int("goroutines", m.Goroutines),
		slog.Uint64("heap_alloc", m.HeapAlloc),
	}

	c := m.Counters
	counters := slog.Group("counters",
		slog.Int("node_count", c.NodeCount),
		slog.Int("rule_count", c.RuleCount),
		slog.Int("selector_count", c.SelectorCount),
		slog.Int("box_count", c.BoxCount),
		slog.Int("fragment_count", c.FragmentCount),
		slog.Int("display_item_count", c.DisplayItemCount),
		slog.Int("tile_count", c.TileCount),
		slog.Int("image_count", c.ImageCount),
		slog.Int64("bytes_downloaded", c.BytesDownloaded),
		slog.Int64("decoded_image_bytes", c.DecodedImageBytes),
		slog.Int("cache_hits", c.CacheHits),
		slog.Int("cache_misses", c.CacheMisses),
		slog.Int("script_errors", c.ScriptErrors),
	)
	attrs = append(attrs, counters)

	for p := PhaseNavigation; p <= PhasePresent; p++ {
		if d := m.PhaseDuration(p); d > 0 {
			attrs = append(attrs, slog.Int64(p.String()+"_ns", d.Nanoseconds()))
		}
	}

	l.LogAttrs(ctx, slog.LevelInfo, "navigation complete", attrs...)
}
