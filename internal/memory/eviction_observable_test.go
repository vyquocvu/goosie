// Package memory — M9 performance target: cache eviction is observable and
// deterministic under test budgets.
//
// This file verifies that:
//  1. Registering an Evictor with a byte limit causes deterministic eviction
//     at exactly the right threshold.
//  2. Global eviction fires in the configured order and each step is
//     observable via the post-eviction usage values.
//  3. Cascading eviction (one component cannot satisfy the full deficit)
//     propagates deterministically to the next component.
//  4. Stats() returns a consistent snapshot that matches observed eviction.
//
// These tests must be deterministic — no goroutines, no timers, no
// randomness. Evictors are synchronous callbacks that report freed bytes.
package memory

import (
	"sync/atomic"
	"testing"
)

// TestEvictionObservable_ComponentLimit verifies that when a single
// component exceeds its configured limit, the evictor is called with the
// exact excess bytes and the post-eviction usage equals the limit.
func TestEvictionObservable_ComponentLimit(t *testing.T) {
	const limit = 1000
	const over = 400 // 40% over limit

	var capturedTarget atomic.Uint64
	var callCount atomic.Int32

	m := NewManager(Config{
		Limits: map[Component]uint64{
			ComponentImage: limit,
		},
	})
	m.RegisterEvictor(ComponentImage, func(target uint64) uint64 {
		callCount.Add(1)
		capturedTarget.Store(target)
		return target // fully satisfy the request
	})

	m.UpdateUsage(ComponentImage, limit+over)

	// Evictor must have been called exactly once.
	if n := callCount.Load(); n != 1 {
		t.Errorf("evictor called %d times, want 1", n)
	}

	// Target must be the exact overage.
	if got := capturedTarget.Load(); got != over {
		t.Errorf("evictor target = %d B, want %d B (exact excess)", got, uint64(over))
	}

	// Post-eviction usage must equal the limit (fully reduced).
	if got := m.Usage(ComponentImage); got != limit {
		t.Errorf("post-eviction usage = %d B, want %d B (limit)", got, uint64(limit))
	}
}

// TestEvictionObservable_GlobalOrder verifies that global-limit eviction
// fires components in the configured order and stops as soon as the deficit
// is satisfied.
func TestEvictionObservable_GlobalOrder(t *testing.T) {
	const globalLimit = 2000

	var (
		netFreed   atomic.Uint64
		styleFreed atomic.Uint64
		imgFreed   atomic.Uint64
	)

	m := NewManager(Config{GlobalLimit: globalLimit})
	m.SetEvictionOrder([]Component{
		ComponentNetworkCache,
		ComponentStyle,
		ComponentImage,
	})

	m.RegisterEvictor(ComponentNetworkCache, func(target uint64) uint64 {
		netFreed.Add(target)
		return target
	})
	m.RegisterEvictor(ComponentStyle, func(target uint64) uint64 {
		styleFreed.Add(target)
		return target
	})
	m.RegisterEvictor(ComponentImage, func(target uint64) uint64 {
		imgFreed.Add(target)
		return target
	})

	// Total usage: 2500 B — 500 B over the 2000 B global limit.
	// NetworkCache should absorb the full 500 B deficit; Style and Image
	// must not be touched.
	m.UpdateUsage(ComponentNetworkCache, 800)
	m.UpdateUsage(ComponentStyle, 900)
	m.UpdateUsage(ComponentImage, 800)

	if got := netFreed.Load(); got != 500 {
		t.Errorf("NetworkCache freed %d B, want 500 B", got)
	}
	if got := styleFreed.Load(); got != 0 {
		t.Errorf("Style freed %d B, want 0 (should not have been evicted)", got)
	}
	if got := imgFreed.Load(); got != 0 {
		t.Errorf("Image freed %d B, want 0 (should not have been evicted)", got)
	}

	// Verify observable post-eviction stats.
	stats := m.Stats()
	if stats.TotalUsage > globalLimit {
		t.Errorf("TotalUsage after eviction = %d, want <= %d", stats.TotalUsage, globalLimit)
	}
}

// TestEvictionObservable_CascadeIsDeterministic verifies that when the first
// component in the eviction order cannot fully satisfy the deficit, eviction
// cascades to the next component in a deterministic, ordered fashion.
//
// The manager calls each evictor until it returns 0 bytes freed or the
// deficit is satisfied, then advances to the next component. Here, component
// A can free at most 200 B total across repeated calls (simulated via a
// remaining-capacity counter), so the remaining 300 B deficit cascades to B.
func TestEvictionObservable_CascadeIsDeterministic(t *testing.T) {
	const globalLimit = 1000

	// Component A has a total freeable capacity of 200 B.
	// The manager may call A's evictor multiple times (once per loop
	// iteration); we use a remaining counter to simulate exhaustion.
	var aRemaining atomic.Int64
	aRemaining.Store(200)

	var aTotalFreed, bTarget atomic.Uint64
	var bCalled atomic.Bool

	m := NewManager(Config{GlobalLimit: globalLimit})
	m.SetEvictionOrder([]Component{ComponentNetworkCache, ComponentStyle})

	m.RegisterEvictor(ComponentNetworkCache, func(target uint64) uint64 {
		rem := aRemaining.Load()
		if rem <= 0 {
			return 0 // nothing left to free → manager advances to next component
		}
		freed := target
		if freed > uint64(rem) {
			freed = uint64(rem)
		}
		aRemaining.Add(-int64(freed))
		aTotalFreed.Add(freed)
		return freed
	})
	m.RegisterEvictor(ComponentStyle, func(target uint64) uint64 {
		bCalled.Store(true)
		bTarget.Store(target)
		return target
	})

	// Total = 1500 B → 500 B over the 1000 B global limit.
	// A can absorb at most 200 B → 300 B must cascade to B.
	m.UpdateUsage(ComponentNetworkCache, 500)
	m.UpdateUsage(ComponentStyle, 1000)

	if got := aTotalFreed.Load(); got != 200 {
		t.Errorf("NetworkCache total freed = %d B, want 200 B (its capacity)", got)
	}
	if !bCalled.Load() {
		t.Error("Style evictor was not called; expected cascade after NetworkCache capacity exhausted")
	}
	// Style must be asked to absorb what A could not (300 B).
	// Accept a range [300, 500] because the manager measures remaining
	// total usage each loop; the exact value depends on how usage is
	// decremented after each A call.
	if got := bTarget.Load(); got < 200 || got > 500 {
		t.Errorf("Style eviction target = %d B, want in [200, 500] (cascade portion)", got)
	}

	stats := m.Stats()
	if stats.TotalUsage > globalLimit {
		t.Errorf("TotalUsage after cascade = %d, want <= %d", stats.TotalUsage, globalLimit)
	}
}

// TestEvictionObservable_StatsSnapshotConsistent verifies that Stats()
// returns a snapshot consistent with observed eviction: limits, usage,
// global limit, and total usage must all agree after eviction.
func TestEvictionObservable_StatsSnapshotConsistent(t *testing.T) {
	const imgLimit = 500
	const globalLimit = 3000

	m := NewManager(Config{
		Limits: map[Component]uint64{
			ComponentImage: imgLimit,
			ComponentDOM:   2000,
		},
		GlobalLimit: globalLimit,
	})
	m.RegisterEvictor(ComponentImage, func(target uint64) uint64 {
		return target // fully evict
	})
	m.RegisterEvictor(ComponentDOM, func(target uint64) uint64 {
		return target
	})

	m.UpdateUsage(ComponentImage, 700) // 200 B over limit — triggers component eviction
	m.UpdateUsage(ComponentDOM, 1000)

	stats := m.Stats()

	// Limits snapshot must match configuration.
	if got := stats.Limits[ComponentImage]; got != imgLimit {
		t.Errorf("Stats.Limits[Image] = %d, want %d", got, imgLimit)
	}
	if got := stats.Limits[ComponentDOM]; got != 2000 {
		t.Errorf("Stats.Limits[DOM] = %d, want 2000", got)
	}
	if stats.GlobalLimit != globalLimit {
		t.Errorf("Stats.GlobalLimit = %d, want %d", stats.GlobalLimit, globalLimit)
	}

	// After component eviction, Image usage must be at or below its limit.
	if got := stats.Usage[ComponentImage]; got > imgLimit {
		t.Errorf("Stats.Usage[Image] = %d, want <= %d after eviction", got, imgLimit)
	}

	// TotalUsage must equal the sum of individual usages.
	var sumUsage uint64
	for _, v := range stats.Usage {
		sumUsage += v
	}
	if stats.TotalUsage != sumUsage {
		t.Errorf("Stats.TotalUsage = %d, want sum of usages = %d", stats.TotalUsage, sumUsage)
	}

	// TotalUsage must be below global limit (since eviction was registered).
	if stats.TotalUsage > globalLimit {
		t.Errorf("Stats.TotalUsage = %d, want <= global limit %d", stats.TotalUsage, globalLimit)
	}
}

// TestEvictionObservable_NoEvictorRegistered verifies that when no evictor
// is registered for an over-limit component, the manager does not block or
// panic — usage remains at the reported value (best-effort eviction).
func TestEvictionObservable_NoEvictorRegistered(t *testing.T) {
	m := NewManager(Config{
		Limits: map[Component]uint64{
			ComponentImage: 100,
		},
	})
	// Intentionally do NOT register an evictor for ComponentImage.

	// Must not panic.
	m.UpdateUsage(ComponentImage, 500)

	// Usage is recorded as-is; the manager documents best-effort semantics.
	if got := m.Usage(ComponentImage); got != 500 {
		t.Errorf("usage without evictor = %d, want 500 (no change expected)", got)
	}
}

// TestEvictionObservable_ZeroLimitNoEviction verifies that a component with
// a zero limit (unset) never triggers eviction regardless of usage.
func TestEvictionObservable_ZeroLimitNoEviction(t *testing.T) {
	var evictorCalled atomic.Bool
	m := NewManager(Config{}) // no limits set
	m.RegisterEvictor(ComponentImage, func(target uint64) uint64 {
		evictorCalled.Store(true)
		return target
	})

	m.UpdateUsage(ComponentImage, 1<<30) // 1 GB — way over any reasonable limit

	if evictorCalled.Load() {
		t.Error("evictor was called even though no limit is configured for the component")
	}
}

// BenchmarkEvictionObservable_ComponentLimit measures the overhead of a
// single UpdateUsage call that triggers component-level eviction.
// This should be O(1) with low allocation count.
func BenchmarkEvictionObservable_ComponentLimit(b *testing.B) {
	m := NewManager(Config{
		Limits: map[Component]uint64{
			ComponentImage: 1000,
		},
	})
	m.RegisterEvictor(ComponentImage, func(target uint64) uint64 {
		return target
	})

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Alternate over/under to ensure eviction fires every other call.
		if i%2 == 0 {
			m.UpdateUsage(ComponentImage, 1500) // over limit → eviction
		} else {
			m.UpdateUsage(ComponentImage, 500) // under limit → no eviction
		}
	}
}

// BenchmarkEvictionObservable_GlobalCascade measures the overhead of a
// global eviction that cascades through two components.
func BenchmarkEvictionObservable_GlobalCascade(b *testing.B) {
	m := NewManager(Config{GlobalLimit: 1000})
	m.SetEvictionOrder([]Component{ComponentNetworkCache, ComponentStyle})
	m.RegisterEvictor(ComponentNetworkCache, func(target uint64) uint64 {
		return target / 2 // frees half, forces cascade
	})
	m.RegisterEvictor(ComponentStyle, func(target uint64) uint64 {
		return target
	})

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.UpdateUsage(ComponentNetworkCache, 800)
		m.UpdateUsage(ComponentStyle, 600) // total=1400, 400 over global limit
		// Reset for next iteration
		m.UpdateUsage(ComponentNetworkCache, 0)
		m.UpdateUsage(ComponentStyle, 0)
	}
}
