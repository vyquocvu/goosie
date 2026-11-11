package debug

import (
	"fmt"
	"os"
	"sort"
	"time"
)

// Profiler measures timing for each pipeline stage.
type Profiler struct {
	Enabled      bool
	stages       map[string]*StageStats
	order        []string // preserve insertion order
	currentStage string
	currentStart time.Time
}

// StageStats holds timing statistics for one pipeline stage.
type StageStats struct {
	Name      string
	Calls     uint64
	TotalTime time.Duration
	MinTime   time.Duration
	MaxTime   time.Duration
	AvgTime   time.Duration
	LastTime  time.Duration
	Histogram []int // bucket counts: <1ms, 1-5ms, 5-10ms, 10-50ms, 50-100ms, >100ms
}

// NewProfiler returns a pipeline profiler.
func NewProfiler(enabled bool) *Profiler {
	return &Profiler{
		Enabled: enabled,
		stages:  make(map[string]*StageStats),
		order:   make([]string, 0, 20),
	}
}

// Begin starts timing a stage.
func (p *Profiler) Begin(stage string) {
	if !p.Enabled {
		return
	}
	p.currentStage = stage
	p.currentStart = time.Now()
}

// End finishes timing the current stage and records stats.
func (p *Profiler) End() {
	if !p.Enabled || p.currentStage == "" {
		return
	}

	dur := time.Since(p.currentStart)
	stats, ok := p.stages[p.currentStage]
	if !ok {
		stats = &StageStats{
			Name:      p.currentStage,
			MinTime:   dur,
			MaxTime:   dur,
			Histogram: make([]int, 6),
		}
		p.stages[p.currentStage] = stats
		p.order = append(p.order, p.currentStage)
	}

	stats.Calls++
	stats.TotalTime += dur
	stats.LastTime = dur
	if dur < stats.MinTime {
		stats.MinTime = dur
	}
	if dur > stats.MaxTime {
		stats.MaxTime = dur
	}
	stats.AvgTime = stats.TotalTime / time.Duration(stats.Calls)

	// Update histogram
	switch {
	case dur < 1*time.Millisecond:
		stats.Histogram[0]++
	case dur < 5*time.Millisecond:
		stats.Histogram[1]++
	case dur < 10*time.Millisecond:
		stats.Histogram[2]++
	case dur < 50*time.Millisecond:
		stats.Histogram[3]++
	case dur < 100*time.Millisecond:
		stats.Histogram[4]++
	default:
		stats.Histogram[5]++
	}

	p.currentStage = ""
}

// Time measures a function call.
func (p *Profiler) Time(stage string, fn func()) {
	if !p.Enabled {
		fn()
		return
	}
	p.Begin(stage)
	fn()
	p.End()
}

// Stats returns statistics for all stages, sorted by total time.
func (p *Profiler) Stats() []*StageStats {
	p.order = p.order[:0]
	for name := range p.stages {
		p.order = append(p.order, name)
	}

	stats := make([]*StageStats, 0, len(p.stages))
	for _, name := range p.order {
		stats = append(stats, p.stages[name])
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].TotalTime > stats[j].TotalTime
	})

	return stats
}

// Print prints profiling statistics to stderr.
func (p *Profiler) Print() {
	if !p.Enabled {
		return
	}

	stats := p.Stats()
	if len(stats) == 0 {
		return
	}

	fmt.Fprintf(os.Stderr, "\n=== Pipeline Profiler ===\n")
	fmt.Fprintf(os.Stderr, "%-20s %10s %10s %10s %10s %10s\n",
		"Stage", "Calls", "Total", "Avg", "Min", "Max")
	fmt.Fprintf(os.Stderr, "%s\n", repeat("-", 80))

	for _, s := range stats {
		fmt.Fprintf(os.Stderr, "%-20s %10d %10v %10v %10v %10v\n",
			s.Name, s.Calls, s.TotalTime, s.AvgTime, s.MinTime, s.MaxTime)
	}

	fmt.Fprintf(os.Stderr, "\nHistogram (bucket counts):\n")
	fmt.Fprintf(os.Stderr, "%-20s %8s %8s %8s %8s %8s %8s\n",
		"Stage", "<1ms", "1-5ms", "5-10ms", "10-50ms", "50-100ms", ">100ms")
	fmt.Fprintf(os.Stderr, "%s\n", repeat("-", 80))

	for _, s := range stats {
		if len(s.Histogram) >= 6 {
			fmt.Fprintf(os.Stderr, "%-20s %8d %8d %8d %8d %8d %8d\n",
				s.Name, s.Histogram[0], s.Histogram[1], s.Histogram[2],
				s.Histogram[3], s.Histogram[4], s.Histogram[5])
		}
	}

	fmt.Fprintf(os.Stderr, "===========================\n\n")
}

// Reset clears all statistics.
func (p *Profiler) Reset() {
	p.stages = make(map[string]*StageStats)
	p.order = p.order[:0]
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

// Common stage names for consistency.
const (
	StageStyle         = "style"
	StageLayout        = "layout"
	StagePaint         = "paint"
	StageRasterize     = "rasterize"
	StageComposite     = "composite"
	StagePresent       = "present"
	StageContentDamage = "content_damage"
	StageReflow        = "reflow"
	StageRestyle       = "restyle"
)
