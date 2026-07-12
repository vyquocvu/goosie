package pagecache

import (
	"fmt"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Unit tests
// ---------------------------------------------------------------------------

func TestPutAndGet(t *testing.T) {
	c := New(10, 0)

	entry := PageEntry{
		URL:   "https://example.com",
		Title: "Example",
	}
	c.Put(entry)

	got, ok := c.Get("https://example.com")
	if !ok {
		t.Fatal("Get returned false for cached URL")
	}
	if got.URL != entry.URL {
		t.Errorf("URL = %q, want %q", got.URL, entry.URL)
	}
	if got.Title != entry.Title {
		t.Errorf("Title = %q, want %q", got.Title, entry.Title)
	}
}

func TestGetMiss(t *testing.T) {
	c := New(10, 0)

	_, ok := c.Get("https://nonexistent.com")
	if ok {
		t.Fatal("Get returned true for uncached URL")
	}
}

func TestPutOverwrite(t *testing.T) {
	c := New(10, 0)

	c.Put(PageEntry{URL: "https://a.com", Title: "v1"})
	c.Put(PageEntry{URL: "https://a.com", Title: "v2"})

	got, ok := c.Get("https://a.com")
	if !ok {
		t.Fatal("Get returned false after overwrite")
	}
	if got.Title != "v2" {
		t.Errorf("Title = %q, want %q", got.Title, "v2")
	}
	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}
}

func TestLRUEvictionByCount(t *testing.T) {
	c := New(3, 0)

	c.Put(PageEntry{URL: "https://1.com", Title: "p1"})
	c.Put(PageEntry{URL: "https://2.com", Title: "p2"})
	c.Put(PageEntry{URL: "https://3.com", Title: "p3"})

	// Cache full. Adding a 4th should evict LRU (https://1.com).
	c.Put(PageEntry{URL: "https://4.com", Title: "p4"})

	if c.Len() != 3 {
		t.Errorf("Len = %d, want 3", c.Len())
	}
	if _, ok := c.Get("https://1.com"); ok {
		t.Error("LRU entry (1.com) should have been evicted")
	}
	if _, ok := c.Get("https://4.com"); !ok {
		t.Error("Newest entry (4.com) should be present")
	}
}

func TestLRUAccessRefreshes(t *testing.T) {
	c := New(3, 0)

	c.Put(PageEntry{URL: "https://1.com", Title: "p1"})
	c.Put(PageEntry{URL: "https://2.com", Title: "p2"})
	c.Put(PageEntry{URL: "https://3.com", Title: "p3"})

	// Access entry 1 to refresh it in LRU order.
	c.Get("https://1.com")

	// Now entry 2 is the LRU. Adding a 4th should evict entry 2.
	c.Put(PageEntry{URL: "https://4.com", Title: "p4"})

	if _, ok := c.Get("https://1.com"); !ok {
		t.Error("Entry 1 should still be present (was recently accessed)")
	}
	if _, ok := c.Get("https://2.com"); ok {
		t.Error("Entry 2 should have been evicted (was LRU)")
	}
}

func TestLRUEvictionByBytes(t *testing.T) {
	// Budget: 300 bytes. Each entry estimated at 100 bytes.
	c := New(100, 300)

	c.Put(PageEntry{URL: "https://1.com", ByteSize: 100})
	c.Put(PageEntry{URL: "https://2.com", ByteSize: 100})
	c.Put(PageEntry{URL: "https://3.com", ByteSize: 100})

	// Full at 300 bytes. Adding a 4th 100-byte entry should evict LRU.
	c.Put(PageEntry{URL: "https://4.com", ByteSize: 100})

	if c.Len() != 3 {
		t.Errorf("Len = %d, want 3", c.Len())
	}
	if _, ok := c.Get("https://1.com"); ok {
		t.Error("LRU entry should have been evicted to fit byte budget")
	}
}

func TestLargeEntrySkipped(t *testing.T) {
	// Budget: 100 bytes. Single entry exceeds budget.
	c := New(10, 100)

	c.Put(PageEntry{URL: "https://big.com", ByteSize: 200})

	if c.Len() != 0 {
		t.Error("Entry exceeding byte budget should not be cached")
	}
	if _, ok := c.Get("https://big.com"); ok {
		t.Error("Oversized entry should not be retrievable")
	}
}

func TestRemove(t *testing.T) {
	c := New(10, 0)

	c.Put(PageEntry{URL: "https://a.com", Title: "a"})
	c.Remove("https://a.com")

	if c.Len() != 0 {
		t.Errorf("Len = %d, want 0 after Remove", c.Len())
	}
	if _, ok := c.Get("https://a.com"); ok {
		t.Error("Get should return false after Remove")
	}
}

func TestRemoveNonexistent(t *testing.T) {
	c := New(10, 0)
	c.Remove("https://nonexistent.com") // should not panic
}

func TestClear(t *testing.T) {
	c := New(10, 0)

	for i := 0; i < 5; i++ {
		c.Put(PageEntry{URL: fmt.Sprintf("https://%d.com", i), Title: fmt.Sprintf("p%d", i)})
	}

	c.Clear()

	if c.Len() != 0 {
		t.Errorf("Len = %d, want 0 after Clear", c.Len())
	}
	if c.Bytes() != 0 {
		t.Errorf("Bytes = %d, want 0 after Clear", c.Bytes())
	}
}

func TestBytesTracking(t *testing.T) {
	c := New(10, 0)

	c.Put(PageEntry{URL: "https://a.com", ByteSize: 100})
	c.Put(PageEntry{URL: "https://b.com", ByteSize: 200})

	if c.Bytes() != 300 {
		t.Errorf("Bytes = %d, want 300", c.Bytes())
	}

	c.Remove("https://a.com")
	if c.Bytes() != 200 {
		t.Errorf("Bytes = %d after Remove, want 200", c.Bytes())
	}
}

func TestMetrics(t *testing.T) {
	c := New(10, 0)

	c.Put(PageEntry{URL: "https://a.com", Title: "a"})
	c.Get("https://a.com") // hit
	c.Get("https://b.com") // miss

	snap := c.Metrics().Snapshot()
	if snap.Hits != 1 {
		t.Errorf("Hits = %d, want 1", snap.Hits)
	}
	if snap.Misses != 1 {
		t.Errorf("Misses = %d, want 1", snap.Misses)
	}
	if snap.Evictions != 0 {
		t.Errorf("Evictions = %d, want 0", snap.Evictions)
	}
}

func TestEvictMethod(t *testing.T) {
	c := New(10, 0)

	c.Put(PageEntry{URL: "https://a.com", ByteSize: 100})
	c.Put(PageEntry{URL: "https://b.com", ByteSize: 200})
	c.Put(PageEntry{URL: "https://c.com", ByteSize: 300})

	freed := c.Evict(150)
	if freed < 150 {
		t.Errorf("Evict freed %d bytes, want >= 150", freed)
	}
	if c.Len() >= 3 {
		t.Errorf("Len = %d after Evict, should be < 3", c.Len())
	}
}

func TestEvictEmpty(t *testing.T) {
	c := New(10, 0)
	freed := c.Evict(1000)
	if freed != 0 {
		t.Errorf("Evict on empty cache freed %d, want 0", freed)
	}
}

func TestConcurrency(t *testing.T) {
	c := New(100, 10000)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			url := fmt.Sprintf("https://%d.com", n)
			c.Put(PageEntry{URL: url, Title: fmt.Sprintf("p%d", n), ByteSize: 100})
			c.Get(url)
			c.Remove(url)
		}(i)
	}
	wg.Wait()
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxEntries != 3 {
		t.Errorf("MaxEntries = %d, want 3", cfg.MaxEntries)
	}
	if cfg.MaxBytes != 32<<20 {
		t.Errorf("MaxBytes = %d, want %d", cfg.MaxBytes, 32<<20)
	}
}

func TestNewFromConfig(t *testing.T) {
	cfg := Config{MaxEntries: 5, MaxBytes: 1024}
	c := NewFromConfig(cfg)

	c.Put(PageEntry{URL: "https://a.com", ByteSize: 100})

	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}
}

func TestCapacityZero(t *testing.T) {
	c := New(0, 0) // should use default
	c.Put(PageEntry{URL: "https://a.com", Title: "a"})
	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}
}

func TestCapacityOne(t *testing.T) {
	c := New(1, 0)
	c.Put(PageEntry{URL: "https://1.com", Title: "p1"})
	c.Put(PageEntry{URL: "https://2.com", Title: "p2"})

	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}
	if _, ok := c.Get("https://2.com"); !ok {
		t.Error("Only entry should be the most recent")
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkPut(b *testing.B) {
	c := New(b.N, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Put(PageEntry{
			URL:      fmt.Sprintf("https://%d.com", i),
			Title:    fmt.Sprintf("page %d", i),
			ByteSize: 100,
		})
	}
}

func BenchmarkGetHit(b *testing.B) {
	c := New(b.N, 0)
	for i := 0; i < b.N; i++ {
		c.Put(PageEntry{
			URL:      fmt.Sprintf("https://%d.com", i),
			Title:    fmt.Sprintf("page %d", i),
			ByteSize: 100,
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(fmt.Sprintf("https://%d.com", i))
	}
}

func BenchmarkGetMiss(b *testing.B) {
	c := New(10, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(fmt.Sprintf("https://miss-%d.com", i))
	}
}

func BenchmarkPutEvict(b *testing.B) {
	c := New(10, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Put(PageEntry{
			URL:      fmt.Sprintf("https://%d.com", i),
			Title:    fmt.Sprintf("page %d", i),
			ByteSize: 100,
		})
	}
}

func BenchmarkEvict(b *testing.B) {
	c := New(1024, 0)
	for i := 0; i < 1024; i++ {
		c.Put(PageEntry{
			URL:      fmt.Sprintf("https://%d.com", i),
			Title:    fmt.Sprintf("page %d", i),
			ByteSize: 256,
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Clear()
		c.Put(PageEntry{URL: "reset", ByteSize: 0})
		for j := 0; j < 1024; j++ {
			c.Put(PageEntry{
				URL:      fmt.Sprintf("https://%d.com", j),
				Title:    fmt.Sprintf("page %d", j),
				ByteSize: 256,
			})
		}
	}
}

func BenchmarkLen(b *testing.B) {
	c := New(100, 0)
	for i := 0; i < 100; i++ {
		c.Put(PageEntry{URL: fmt.Sprintf("https://%d.com", i)})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Len()
	}
}
