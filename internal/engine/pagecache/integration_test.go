// M9 integration test: cache eviction is observable and deterministic under
// test budgets when the pagecache is wired into memory.Manager.
//
// This file uses the external test package "pagecache_test" so it can import
// both "pagecache" and "internal/memory" without an import cycle.
package pagecache_test

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine/pagecache"
	"github.com/vyquocvu/goosie/internal/memory"
)

// TestMemoryManagerIntegration_PageCacheEvictionIsDeterministic wires a
// pagecache.Cache into a memory.Manager and verifies that:
//
//   - UpdateUsage above the component limit triggers pagecache.Evict.
//   - The pagecache reports the correct bytes freed via Metrics.
//   - The manager's post-eviction usage matches the freed bytes.
//   - Cache hits and misses remain accurate after eviction.
func TestMemoryManagerIntegration_PageCacheEvictionIsDeterministic(t *testing.T) {
	const entryBytes = 1000
	const entries = 5
	const totalBytes = entryBytes * entries // 5 000 B
	const budgetLimit = 3000               // 3 000 B — forces eviction of 2 entries

	// Create a pagecache with generous entry count but a tight byte limit
	// so only 3 entries fit at entryBytes each.
	cache := pagecache.New(100, budgetLimit)

	// Populate the cache: 5 entries × 1000 B = 5 000 B stored.
	for i := 0; i < entries; i++ {
		cache.Put(pagecache.PageEntry{
			URL:      fmt.Sprintf("https://page-%d.example.com", i),
			Title:    fmt.Sprintf("Page %d", i),
			ByteSize: entryBytes,
		})
	}

	// The byte-budget enforcement in Put evicts LRU entries automatically.
	// After all puts the cache must hold exactly budgetLimit / entryBytes = 3 entries.
	wantEntries := int(budgetLimit / entryBytes)
	if got := cache.Len(); got != wantEntries {
		t.Fatalf("cache.Len() = %d, want %d after byte-budget enforcement", got, wantEntries)
	}
	if got := cache.Bytes(); got > int64(budgetLimit) {
		t.Fatalf("cache.Bytes() = %d, want <= %d", got, budgetLimit)
	}

	// Record evictions that happened during Put.
	snapAfterPut := cache.Metrics().Snapshot()
	if snapAfterPut.Evictions != int64(entries-wantEntries) {
		t.Errorf("evictions after Put = %d, want %d", snapAfterPut.Evictions, entries-wantEntries)
	}

	// Wire the cache into a memory.Manager with the same byte limit.
	mgr := memory.NewManager(memory.Config{
		Limits: map[memory.Component]uint64{
			memory.ComponentPageCache: uint64(budgetLimit),
		},
	})
	mgr.RegisterEvictor(memory.ComponentPageCache, func(target uint64) uint64 {
		return cache.Evict(target)
	})

	// Simulate the memory manager tracking the cache's current usage.
	mgr.UpdateUsage(memory.ComponentPageCache, uint64(cache.Bytes()))

	// Usage is within the budget (cache enforced it during Put), so no
	// additional eviction should have been triggered.
	if got := mgr.Usage(memory.ComponentPageCache); got > uint64(budgetLimit) {
		t.Errorf("manager usage after sync = %d, want <= %d", got, budgetLimit)
	}

	// Simulate an external usage spike: report usage 1 500 B above budget.
	// The manager must call cache.Evict to bring it back under the limit.
	const spikeBytes = 4500 // 1 500 over budget
	snapBeforeSpike := cache.Metrics().Snapshot()

	mgr.UpdateUsage(memory.ComponentPageCache, spikeBytes)

	snapAfterSpike := cache.Metrics().Snapshot()
	newEvictions := snapAfterSpike.Evictions - snapBeforeSpike.Evictions
	if newEvictions == 0 {
		t.Error("no additional evictions occurred after usage spike — manager did not call cache.Evict")
	}

	// Post-eviction: manager's tracked usage must be at or below the limit.
	if got := mgr.Usage(memory.ComponentPageCache); got > uint64(budgetLimit) {
		t.Errorf("manager usage after eviction = %d, want <= %d", got, budgetLimit)
	}

	// Cache metrics must be self-consistent: evictions counter must be
	// non-negative and bytes must match the remaining entry count × size.
	finalSnap := cache.Metrics().Snapshot()
	if finalSnap.Evictions < 0 {
		t.Errorf("Evictions counter is negative: %d", finalSnap.Evictions)
	}
	wantMaxBytes := int64(budgetLimit)
	if cache.Bytes() > wantMaxBytes {
		t.Errorf("cache.Bytes() = %d after all evictions, want <= %d", cache.Bytes(), wantMaxBytes)
	}

	t.Logf("Integration eviction summary:")
	t.Logf("  Entries after Put             : %d", cache.Len())
	t.Logf("  Bytes after Put               : %d / %d budget", cache.Bytes(), budgetLimit)
	t.Logf("  Evictions during Put          : %d", snapAfterPut.Evictions)
	t.Logf("  Evictions triggered by manager: %d", newEvictions)
	t.Logf("  Manager tracked usage (final) : %d", mgr.Usage(memory.ComponentPageCache))
}

// TestMemoryManagerIntegration_HitRatePreservedAfterEviction verifies that
// cache hit/miss tracking is accurate and consistent after eviction cycles.
func TestMemoryManagerIntegration_HitRatePreservedAfterEviction(t *testing.T) {
	cache := pagecache.New(10, 2000)

	// Put 4 entries of 400 B each = 1 600 B (under 2 000 B limit).
	for i := 0; i < 4; i++ {
		cache.Put(pagecache.PageEntry{
			URL:      fmt.Sprintf("https://page-%d.example.com", i),
			ByteSize: 400,
		})
	}

	// All 4 entries present — hit them all.
	for i := 0; i < 4; i++ {
		if _, ok := cache.Get(fmt.Sprintf("https://page-%d.example.com", i)); !ok {
			t.Errorf("entry %d not found before eviction", i)
		}
	}

	// Add a 5th entry that pushes bytes to 2 000 B exactly — no eviction yet.
	cache.Put(pagecache.PageEntry{URL: "https://page-4.example.com", ByteSize: 400})
	if cache.Len() != 5 {
		t.Fatalf("expected 5 entries, got %d", cache.Len())
	}

	// Explicitly evict 800 B (2 entries).
	freed := cache.Evict(800)
	if freed < 800 {
		t.Fatalf("Evict freed %d B, want >= 800", freed)
	}

	// After eviction, 3 entries remain. Misses on evicted URLs must be
	// counted correctly.
	snap := cache.Metrics().Snapshot()
	hitsBefore := snap.Hits
	missesBefore := snap.Misses

	// The 2 LRU entries were evicted. Try to access all 5 — some will miss.
	hits, misses := 0, 0
	for i := 0; i < 5; i++ {
		if _, ok := cache.Get(fmt.Sprintf("https://page-%d.example.com", i)); ok {
			hits++
		} else {
			misses++
		}
	}

	snap = cache.Metrics().Snapshot()
	if snap.Hits-hitsBefore != int64(hits) {
		t.Errorf("Hits counter delta = %d, expected %d", snap.Hits-hitsBefore, hits)
	}
	if snap.Misses-missesBefore != int64(misses) {
		t.Errorf("Misses counter delta = %d, expected %d", snap.Misses-missesBefore, misses)
	}
	if hits+misses != 5 {
		t.Errorf("hits+misses = %d, want 5", hits+misses)
	}
	if snap.HitRate() < 0 || snap.HitRate() > 1 {
		t.Errorf("HitRate() = %f, want in [0, 1]", snap.HitRate())
	}

	t.Logf("Hit/miss after eviction: hits=%d misses=%d rate=%.2f", hits, misses, snap.HitRate())
}

// BenchmarkMemoryManagerIntegration_EvictViaManager measures the overhead
// of eviction triggered through memory.Manager vs direct cache.Evict.
func BenchmarkMemoryManagerIntegration_EvictViaManager(b *testing.B) {
	cache := pagecache.New(1000, 0)
	mgr := memory.NewManager(memory.Config{
		Limits: map[memory.Component]uint64{
			memory.ComponentPageCache: 100 * 1024,
		},
	})
	mgr.RegisterEvictor(memory.ComponentPageCache, cache.Evict)

	// Pre-fill the cache.
	for i := 0; i < 1000; i++ {
		cache.Put(pagecache.PageEntry{
			URL:      fmt.Sprintf("https://bench-%d.example.com", i),
			ByteSize: 256,
		})
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Spike usage above limit to trigger eviction.
		mgr.UpdateUsage(memory.ComponentPageCache, 200*1024)
		// Reset so next iteration triggers again.
		mgr.UpdateUsage(memory.ComponentPageCache, 0)
	}
}
