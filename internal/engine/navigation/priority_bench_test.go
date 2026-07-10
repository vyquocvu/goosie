package navigation

import (
	"context"
	"testing"
)

func BenchmarkSchedulerBeginWithPriority(b *testing.B) {
	sched := NewScheduler()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = sched.BeginWithPriority(b.Context(), "https://example.com/bench", PriorityScript)
	}
}

func BenchmarkAddResource(b *testing.B) {
	sched := NewScheduler()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = sched.AddResource(context.Background(), "https://example.com/res", PriorityBlockingCSS)
	}
}

func BenchmarkPendingLoads(b *testing.B) {
	sched := NewScheduler()
	// Seed with a mix of priorities
	for i := 0; i < 20; i++ {
		prio := Priority(i%int(PrioritySpeculative) + 1)
		_, _ = sched.AddResource(context.Background(), "https://example.com/bench", prio)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = sched.PendingLoads()
	}
}
