// Package net — bounded HTTP response cache tests (M9.2).
//
// These tests are written first (TDD) and verify:
//   - Entry-count limit with LRU eviction
//   - Byte-size limit with LRU eviction
//   - Hit/miss/eviction metrics
//   - Concurrent safety (race detector)
//   - Backward compatibility with the original NewHTTPCache signature
//   - Cancellation / early return on zero-limit config
package net

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newBoundedCache creates a bounded HTTPCache for tests.
// maxEntries=0 and maxBytes=0 means "no limit" for backward compat.
func newBoundedCache(t *testing.T, maxEntries int, maxBytes int64) *HTTPCache {
	t.Helper()
	cfg := HTTPCacheConfig{
		Root:       t.TempDir(),
		Private:    false,
		MaxEntries: maxEntries,
		MaxBytes:   maxBytes,
	}
	return NewHTTPCacheWithConfig(cfg)
}

func putEntry(t *testing.T, c *HTTPCache, url string, body string, maxAge int) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp := newTestResponse(req, http.StatusOK, body)
	resp.Header.Set("Content-Type", "text/plain")
	resp.Header.Set("Cache-Control", fmt.Sprintf("max-age=%d", maxAge))
	c.Put(url, resp, body)
}

func assertHit(t *testing.T, c *HTTPCache, url string, wantBody string) {
	t.Helper()
	body, _, ok := c.Get(url)
	if !ok {
		t.Fatalf("expected cache HIT for %s, got miss", url)
	}
	if body != wantBody {
		t.Fatalf("body = %q, want %q", body, wantBody)
	}
}

func assertMiss(t *testing.T, c *HTTPCache, url string) {
	t.Helper()
	_, _, ok := c.Get(url)
	if ok {
		t.Fatalf("expected cache MISS for %s, got hit", url)
	}
}

// ---------------------------------------------------------------------------
// M9.2 bounded HTTP cache tests
// ---------------------------------------------------------------------------

// TestHTTPCacheBoundedMaxEntriesEvictsLRU verifies that when MaxEntries is
// exceeded the least-recently-used entry is evicted from disk.
func TestHTTPCacheBoundedMaxEntriesEvictsLRU(t *testing.T) {
	c := newBoundedCache(t, 3, 0)

	putEntry(t, c, "https://a.test/1", "body1", 3600)
	putEntry(t, c, "https://a.test/2", "body2", 3600)
	putEntry(t, c, "https://a.test/3", "body3", 3600)

	// All three must be present.
	assertHit(t, c, "https://a.test/1", "body1")
	assertHit(t, c, "https://a.test/2", "body2")
	assertHit(t, c, "https://a.test/3", "body3")

	// Access /1 and /3 to make /2 the LRU.
	assertHit(t, c, "https://a.test/1", "body1")
	assertHit(t, c, "https://a.test/3", "body3")

	// Insert a 4th entry — /2 should be evicted.
	putEntry(t, c, "https://a.test/4", "body4", 3600)

	assertHit(t, c, "https://a.test/1", "body1")
	assertMiss(t, c, "https://a.test/2") // evicted
	assertHit(t, c, "https://a.test/3", "body3")
	assertHit(t, c, "https://a.test/4", "body4")
}

// TestHTTPCacheBoundedMaxEntriesZeroMeansUnlimited verifies that MaxEntries=0
// does not evict anything (backward compat).
func TestHTTPCacheBoundedMaxEntriesZeroMeansUnlimited(t *testing.T) {
	c := newBoundedCache(t, 0, 0) // no limits

	for i := 0; i < 20; i++ {
		url := fmt.Sprintf("https://b.test/%d", i)
		putEntry(t, c, url, fmt.Sprintf("body%d", i), 3600)
	}

	// All entries must still be present.
	for i := 0; i < 20; i++ {
		url := fmt.Sprintf("https://b.test/%d", i)
		assertHit(t, c, url, fmt.Sprintf("body%d", i))
	}
}

// TestHTTPCacheBoundedMaxBytesEvictsLRU verifies that the byte-based limit
// evicts the LRU entry when adding a new entry would exceed the budget.
func TestHTTPCacheBoundedMaxBytesEvictsLRU(t *testing.T) {
	// Each entry body is 5 bytes; limit = 12 bytes → max 2 entries before 3rd evicts 1st.
	c := newBoundedCache(t, 0, 12)

	putEntry(t, c, "https://c.test/1", "AAAAA", 3600) // 5 bytes
	putEntry(t, c, "https://c.test/2", "BBBBB", 3600) // 5 bytes  (total = 10)

	// Make /1 MRU by reading it.
	assertHit(t, c, "https://c.test/1", "AAAAA")

	// Insert /3 — adding 5 bytes would exceed 12; /2 is LRU, evict it.
	putEntry(t, c, "https://c.test/3", "CCCCC", 3600) // 5 bytes

	assertHit(t, c, "https://c.test/1", "AAAAA")
	assertMiss(t, c, "https://c.test/2") // evicted (was LRU)
	assertHit(t, c, "https://c.test/3", "CCCCC")
}

// TestHTTPCacheBoundedMetricsHitMissEviction verifies that Metrics() returns
// accurate hit, miss, and eviction counts.
func TestHTTPCacheBoundedMetricsHitMissEviction(t *testing.T) {
	c := newBoundedCache(t, 2, 0)

	// Two misses.
	assertMiss(t, c, "https://d.test/1")
	assertMiss(t, c, "https://d.test/2")

	putEntry(t, c, "https://d.test/1", "body1", 3600)
	putEntry(t, c, "https://d.test/2", "body2", 3600)

	// One hit.
	assertHit(t, c, "https://d.test/1", "body1")

	m := c.Metrics()
	if m.Hits != 1 {
		t.Errorf("hits = %d, want 1", m.Hits)
	}
	if m.Misses != 2 {
		t.Errorf("misses = %d, want 2", m.Misses)
	}
	if m.Evictions != 0 {
		t.Errorf("evictions = %d, want 0 (no eviction yet)", m.Evictions)
	}

	// Third entry triggers one eviction (MaxEntries=2).
	putEntry(t, c, "https://d.test/3", "body3", 3600)

	m = c.Metrics()
	if m.Evictions != 1 {
		t.Errorf("evictions = %d, want 1", m.Evictions)
	}
}

// TestHTTPCacheBoundedPrivateModeStillBlocked ensures the private flag
// prevents reads and writes regardless of limits.
func TestHTTPCacheBoundedPrivateModeStillBlocked(t *testing.T) {
	cfg := HTTPCacheConfig{
		Root:       t.TempDir(),
		Private:    true,
		MaxEntries: 100,
	}
	c := NewHTTPCacheWithConfig(cfg)

	putEntry(t, c, "https://priv.test/1", "secret", 3600)
	assertMiss(t, c, "https://priv.test/1")
}

// TestHTTPCacheBoundedUpdateExistingEntryDoesNotDuplicateCount ensures that
// putting the same URL twice counts as one entry, not two.
func TestHTTPCacheBoundedUpdateExistingEntryDoesNotDuplicateCount(t *testing.T) {
	c := newBoundedCache(t, 2, 0)

	putEntry(t, c, "https://e.test/1", "v1", 3600)
	putEntry(t, c, "https://e.test/1", "v2", 3600) // update same URL

	// Still room for one more entry without eviction.
	putEntry(t, c, "https://e.test/2", "w1", 3600)

	// Both URLs present, latest body for /1.
	assertHit(t, c, "https://e.test/1", "v2")
	assertHit(t, c, "https://e.test/2", "w1")

	// Inserting a third unique URL should now evict the LRU (/1 since /2 was just inserted).
	// Access /2 to make /1 LRU, then insert /3.
	assertHit(t, c, "https://e.test/2", "w1")
	putEntry(t, c, "https://e.test/3", "x1", 3600)

	assertMiss(t, c, "https://e.test/1")
	assertHit(t, c, "https://e.test/2", "w1")
	assertHit(t, c, "https://e.test/3", "x1")

	m := c.Metrics()
	if m.Evictions != 1 {
		t.Errorf("evictions = %d, want 1", m.Evictions)
	}
}

// TestHTTPCacheBoundedConcurrentReadWrite exercises the cache under concurrent
// reads and writes; the race detector will catch unsynchronised access.
func TestHTTPCacheBoundedConcurrentReadWrite(t *testing.T) {
	c := newBoundedCache(t, 10, 0)

	// Pre-populate half the capacity.
	for i := 0; i < 5; i++ {
		putEntry(t, c, fmt.Sprintf("https://f.test/%d", i), fmt.Sprintf("v%d", i), 3600)
	}

	var wg sync.WaitGroup
	const goroutines = 8
	const ops = 50

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		g := g
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				// Mix of reads and writes across overlapping keys.
				url := fmt.Sprintf("https://f.test/%d", (g+i)%15)
				if i%3 == 0 {
					putEntry(t, c, url, "concurrent", 3600)
				} else {
					c.Get(url) //nolint:errcheck — result not needed for race test
				}
			}
		}()
	}
	wg.Wait()
}

// TestHTTPCacheBoundedNewHTTPCacheBackwardCompatibility ensures the original
// constructor NewHTTPCache(root, private) still works without limits.
func TestHTTPCacheBoundedNewHTTPCacheBackwardCompatibility(t *testing.T) {
	c := NewHTTPCache(t.TempDir(), false)

	putEntry(t, c, "https://compat.test/1", "compat", 3600)
	assertHit(t, c, "https://compat.test/1", "compat")
}

// TestHTTPCacheBoundedSingleEntryExceedingByteLimitIsNotCached checks that an
// entry whose body is larger than MaxBytes is silently dropped.
func TestHTTPCacheBoundedSingleEntryExceedingByteLimitIsNotCached(t *testing.T) {
	c := newBoundedCache(t, 0, 3) // limit = 3 bytes

	putEntry(t, c, "https://big.test/1", "LONGBODY", 3600) // 8 bytes > 3

	assertMiss(t, c, "https://big.test/1")
}

// TestHTTPCacheBoundedMetricsResetOnClear verifies that calling Clear resets
// the in-memory LRU index and metrics. Metrics are checked immediately after
// Clear and before any subsequent cache operations that would increment them.
func TestHTTPCacheBoundedMetricsResetOnClear(t *testing.T) {
	c := newBoundedCache(t, 5, 0)

	putEntry(t, c, "https://clr.test/1", "b1", 3600)
	putEntry(t, c, "https://clr.test/2", "b2", 3600)
	assertHit(t, c, "https://clr.test/1", "b1")

	// Verify pre-Clear state: 1 hit accumulated.
	preClear := c.Metrics()
	if preClear.Hits != 1 {
		t.Errorf("pre-Clear hits = %d, want 1", preClear.Hits)
	}

	c.Clear()

	// Metrics must be zero immediately after Clear (before any more operations).
	m := c.Metrics()
	if m.Hits != 0 || m.Misses != 0 || m.Evictions != 0 {
		t.Errorf("metrics not reset after Clear: %+v", m)
	}

	// Verify the LRU index is empty: both URLs now miss.
	assertMiss(t, c, "https://clr.test/1")
	assertMiss(t, c, "https://clr.test/2")
}

// BenchmarkHTTPCachePut measures the cost of a Put call with a bounded cache.
func BenchmarkHTTPCachePut(b *testing.B) {
	dir := b.TempDir()
	c := NewHTTPCacheWithConfig(HTTPCacheConfig{
		Root:       dir,
		MaxEntries: 256,
	})
	req, _ := http.NewRequest(http.MethodGet, "https://bench.test/1", nil)
	resp := newTestResponse(req, http.StatusOK, "benchbody")
	resp.Header.Set("Cache-Control", "max-age=3600")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		url := fmt.Sprintf("https://bench.test/%d", i%256)
		c.Put(url, resp, "benchbody")
	}
}

// BenchmarkHTTPCacheGet measures cache hit performance.
func BenchmarkHTTPCacheGet(b *testing.B) {
	dir := b.TempDir()
	c := NewHTTPCacheWithConfig(HTTPCacheConfig{
		Root:       dir,
		MaxEntries: 256,
	})
	req, _ := http.NewRequest(http.MethodGet, "https://bench.test/1", nil)
	resp := newTestResponse(req, http.StatusOK, "benchbody")
	resp.Header.Set("Cache-Control", "max-age=3600")
	c.Put("https://bench.test/hot", resp, "benchbody")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get("https://bench.test/hot")
	}
}
