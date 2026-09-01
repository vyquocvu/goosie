package metrics_test

import (
	"io"
	"log/slog"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine/metrics"
)

func BenchmarkRecorderBeginEndPhase(b *testing.B) {
	r := metrics.NewRecorder(1, "https://example.com/bench")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r.BeginPhase(metrics.PhaseParse)
		r.EndPhase(metrics.PhaseParse)
	}
}

func BenchmarkRecorderBeginEndPhaseSequential(b *testing.B) {
	r := metrics.NewRecorder(1, "https://example.com/bench")
	phases := []metrics.Phase{metrics.PhaseNavigation, metrics.PhaseDNSResolve, metrics.PhaseConnect, metrics.PhaseFirstByte,
		metrics.PhaseBodyRead, metrics.PhaseParse, metrics.PhaseStyle, metrics.PhaseLayout, metrics.PhasePaint, metrics.PhaseRaster, metrics.PhasePresent}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, p := range phases {
			r.BeginPhase(p)
			r.EndPhase(p)
		}
	}
}

func BenchmarkRecorderAddCounters(b *testing.B) {
	r := metrics.NewRecorder(1, "https://example.com/bench")
	c := metrics.Counters{NodeCount: 100, RuleCount: 25, BytesDownloaded: 4096}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r.AddCounters(c)
	}
}

func BenchmarkRecorderSnapshot(b *testing.B) {
	r := metrics.NewRecorder(1, "https://example.com/bench")
	r.BeginPhase(metrics.PhaseParse)
	r.EndPhase(metrics.PhaseParse)
	r.AddCounters(metrics.Counters{NodeCount: 500})

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r.Snapshot()
	}
}

func BenchmarkRecorderFinalize(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := metrics.NewRecorder(1, "https://example.com/bench")
		r.BeginPhase(metrics.PhaseParse)
		r.EndPhase(metrics.PhaseParse)
		r.AddCounters(metrics.Counters{NodeCount: 500})
		r.Finalize()
	}
}

func BenchmarkRecorderParallel(b *testing.B) {
	r := metrics.NewRecorder(1, "https://example.com/bench")
	b.ResetTimer()

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r.BeginPhase(metrics.PhaseParse)
			r.EndPhase(metrics.PhaseParse)
			r.AddCounters(metrics.Counters{NodeCount: 1})
		}
	})
}

// BenchmarkRecorderDebugLogDisabled measures the cost when debug logging is
// off (the common path). It must allocate near zero.
func BenchmarkRecorderDebugLogDisabled(b *testing.B) {
	r := metrics.NewRecorder(1, "https://example.com/bench")
	r.BeginPhase(metrics.PhaseParse)
	r.EndPhase(metrics.PhaseParse)
	r.AddCounters(metrics.Counters{NodeCount: 500})

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r.Finalize()
		r = metrics.NewRecorder(1, "https://example.com/bench")
		r.BeginPhase(metrics.PhaseParse)
		r.EndPhase(metrics.PhaseParse)
	}
}

// BenchmarkRecorderDebugLogEnabled measures the structured-logging cost when
// enabled. It reuses one logger writing to a discard buffer.
func BenchmarkRecorderDebugLogEnabled(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := metrics.NewRecorder(1, "https://example.com/bench")
	r.SetDebugLog(logger)
	r.BeginPhase(metrics.PhaseParse)
	r.EndPhase(metrics.PhaseParse)
	r.AddCounters(metrics.Counters{NodeCount: 500})

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r.Finalize()
		r = metrics.NewRecorder(1, "https://example.com/bench")
		r.SetDebugLog(logger)
		r.BeginPhase(metrics.PhaseParse)
		r.EndPhase(metrics.PhaseParse)
	}
}
