package navigation_test

import (
	"context"
	"testing"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
)

func BenchmarkSchedulerBeginWithPriority(b *testing.B) {
	sched := navigation.NewScheduler()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = sched.BeginWithPriority(b.Context(), "https://example.com/bench", navigation.PriorityScript)
	}
}

func BenchmarkAddResource(b *testing.B) {
	sched := navigation.NewScheduler()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = sched.AddResource(context.Background(), "https://example.com/res", navigation.PriorityBlockingCSS)
	}
}

func BenchmarkPendingLoads(b *testing.B) {
	sched := navigation.NewScheduler()
	// Seed with a mix of priorities
	for i := 0; i < 20; i++ {
		prio := navigation.Priority(i%int(navigation.PrioritySpeculative) + 1)
		_, _ = sched.AddResource(context.Background(), "https://example.com/bench", prio)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = sched.PendingLoads()
	}
}
