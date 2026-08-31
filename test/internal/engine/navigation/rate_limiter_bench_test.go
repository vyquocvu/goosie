package navigation_test

import (
	"context"
	"sync"
	"testing"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
)

func BenchmarkAcquireReleaseUncontended(b *testing.B) {
	rl := navigation.NewRateLimiter(6, 24)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = rl.Acquire(ctx, "example.com", navigation.PriorityScript)
		rl.Release("example.com")
	}
}

func BenchmarkAcquireReleaseContention(b *testing.B) {
	rl := navigation.NewRateLimiter(6, 24)
	ctx := context.Background()

	const goroutines = 32
	var wg sync.WaitGroup

	b.ReportAllocs()
	b.ResetTimer()

	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range b.N / goroutines {
				_ = rl.Acquire(ctx, "example.com", navigation.PriorityScript)
				rl.Release("example.com")
			}
		}()
	}
	wg.Wait()
}
