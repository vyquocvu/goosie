package navigation

import (
	"context"
	"sync"
	"testing"
)

func BenchmarkAcquireReleaseUncontended(b *testing.B) {
	rl := NewRateLimiter(6, 24)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = rl.Acquire(ctx, "example.com", PriorityScript)
		rl.Release("example.com")
	}
}

func BenchmarkAcquireReleaseContention(b *testing.B) {
	rl := NewRateLimiter(6, 24)
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
				_ = rl.Acquire(ctx, "example.com", PriorityScript)
				rl.Release("example.com")
			}
		}()
	}
	wg.Wait()
}
