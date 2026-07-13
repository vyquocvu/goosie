package memory

import (
	"io"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"sync"
	"time"
)

// mu protects sequential execution of EvaluateConfig since Go runtime parameters are global.
var tuningMu sync.Mutex

// TuningConfig represents a candidate configuration for Go runtime parameters.
type TuningConfig struct {
	GOGC        int   // GC percent (equivalent to debug.SetGCPercent)
	MemoryLimit int64 // Memory limit in bytes (equivalent to debug.SetMemoryLimit)
}

// WorkloadStats holds runtime GC and memory statistics captured during a workload run.
type WorkloadStats struct {
	Duration       time.Duration
	AllocatedBytes uint64
	NumGC          uint32
	GCCPUFraction  float64
	TotalPauseTime time.Duration
	Thrashing      bool
}

// TuningReport contains the evaluation results of a tuning configuration.
type TuningReport struct {
	Config TuningConfig
	Stats  WorkloadStats
	Passed bool // True if the configuration did not trigger GC thrashing
}

// WriteHeapProfile captures a snapshot of current memory allocations and writes it to w.
func WriteHeapProfile(w io.Writer) error {
	runtime.GC() // Clean slate for accurate active profile
	return pprof.WriteHeapProfile(w)
}

// StartCPUProfile starts CPU profiling and writes output to w.
// It returns a function that must be called to stop the profiling session.
func StartCPUProfile(w io.Writer) (func(), error) {
	if err := pprof.StartCPUProfile(w); err != nil {
		return nil, err
	}
	return pprof.StopCPUProfile, nil
}

// EvaluateConfig runs the workload function under the specified TuningConfig.
// It measures execution stats and returns them. It ensures original settings are restored on exit.
func EvaluateConfig(cfg TuningConfig, workload func()) WorkloadStats {
	tuningMu.Lock()
	defer tuningMu.Unlock()

	// Capture current settings
	oldGOGC := debug.SetGCPercent(cfg.GOGC)
	oldLimit := debug.SetMemoryLimit(cfg.MemoryLimit)
	defer func() {
		debug.SetGCPercent(oldGOGC)
		debug.SetMemoryLimit(oldLimit)
	}()

	// Perform a garbage collection to start with a clean heap
	runtime.GC()

	var startMem, endMem runtime.MemStats
	runtime.ReadMemStats(&startMem)

	var startGCStats, endGCStats debug.GCStats
	debug.ReadGCStats(&startGCStats)

	startTime := time.Now()

	// Execute workload
	workload()

	duration := time.Since(startTime)

	runtime.ReadMemStats(&endMem)
	debug.ReadGCStats(&endGCStats)

	// Calculate delta metrics
	numGC := endMem.NumGC - startMem.NumGC
	allocatedBytes := endMem.TotalAlloc - startMem.TotalAlloc
	gccpu := endMem.GCCPUFraction

	var totalPause time.Duration
	if len(endGCStats.Pause) > len(startGCStats.Pause) {
		diff := len(endGCStats.Pause) - len(startGCStats.Pause)
		for i := 0; i < diff && i < len(endGCStats.Pause); i++ {
			totalPause += endGCStats.Pause[i]
		}
	} else if len(endGCStats.Pause) > 0 {
		totalPause = endGCStats.PauseTotal - startGCStats.PauseTotal
	}

	// Thrashing detection:
	// 1. GC CPU fraction exceeds 20%
	// 2. High frequency of GC cycles relative to time (e.g. >100 GCs/sec)
	thrashing := false
	if gccpu > 0.20 {
		thrashing = true
	}
	if duration > 10*time.Millisecond {
		gcRate := float64(numGC) / duration.Seconds()
		if gcRate > 100.0 {
			thrashing = true
		}
	}

	return WorkloadStats{
		Duration:       duration,
		AllocatedBytes: allocatedBytes,
		NumGC:          numGC,
		GCCPUFraction:  gccpu,
		TotalPauseTime: totalPause,
		Thrashing:      thrashing,
	}
}

// AutoTune evaluates a list of configs on a reference workload and returns reports.
func AutoTune(configs []TuningConfig, workload func()) []TuningReport {
	reports := make([]TuningReport, len(configs))
	for i, cfg := range configs {
		stats := EvaluateConfig(cfg, workload)
		reports[i] = TuningReport{
			Config: cfg,
			Stats:  stats,
			Passed: !stats.Thrashing,
		}
	}
	return reports
}
