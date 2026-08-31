package navigation_test

import "github.com/vyquocvu/goosie/internal/engine/navigation"

import "testing"

func BenchmarkIDGeneratorNext(b *testing.B) {
	gen := navigation.NewIDGenerator()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		gen.Next()
	}
}

func BenchmarkSchedulerBegin(b *testing.B) {
	sched := navigation.NewScheduler()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, ctx := sched.Begin(b.Context(), "https://example.com/bench")
		_ = ctx
	}
}
