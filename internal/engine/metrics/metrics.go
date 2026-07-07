package metrics

import "time"

// Timing records when a single phase started and ended.
type Timing struct {
	Phase   Phase
	Started time.Time
	Ended   time.Time
}

// Duration returns the elapsed time in the phase.
func (t Timing) Duration() time.Duration {
	return t.Ended.Sub(t.Started)
}

// Counters holds changeable counts tracked during a navigation.
type Counters struct {
	NodeCount         int
	RuleCount         int
	SelectorCount     int
	BoxCount          int
	FragmentCount     int
	DisplayItemCount  int
	TileCount         int
	ImageCount        int
	BytesDownloaded   int64
	DecodedImageBytes int64
	CacheHits         int
	CacheMisses       int
	ScriptErrors      int
}

// Metrics is an immutable snapshot of measurements for one navigation.
type Metrics struct {
	NavID      uint64
	URL        string
	StartedAt  time.Time
	EndedAt    time.Time
	Timings    []Timing
	Counters   Counters
	Goroutines int
	HeapAlloc  uint64
}

// TotalDuration returns the wall-clock time from navigation start to end.
func (m Metrics) TotalDuration() time.Duration {
	return m.EndedAt.Sub(m.StartedAt)
}

// PhaseDuration returns the total duration of a specific phase across all
// recorded intervals, or zero if the phase was never recorded.
func (m Metrics) PhaseDuration(p Phase) time.Duration {
	var total time.Duration
	for _, t := range m.Timings {
		if t.Phase == p {
			total += t.Duration()
		}
	}
	return total
}
