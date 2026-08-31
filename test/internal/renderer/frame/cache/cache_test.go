package cache_test

import (
	"errors"
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame/cache"
)

// ---------------------------------------------------------------------------
// cache.Metrics tests
// ---------------------------------------------------------------------------

func TestMetricsSnapshot(t *testing.T) {
	var m cache.Metrics
	m.Hits.Store(10)
	m.Misses.Store(5)
	m.Evictions.Store(2)
	snap := m.Snapshot()
	if snap.Hits != 10 || snap.Misses != 5 || snap.Evictions != 2 {
		t.Errorf("snapshot = %+v, want {10,5,2}", snap)
	}
}

func TestMetricsHitRate(t *testing.T) {
	snap := cache.MetricsSnapshot{Hits: 80, Misses: 20}
	rate := snap.HitRate()
	if rate < 0.79 || rate > 0.81 {
		t.Errorf("HitRate = %f, want ~0.8", rate)
	}
}

func TestMetricsHitRateZeroAccess(t *testing.T) {
	snap := cache.MetricsSnapshot{}
	if snap.HitRate() != 0 {
		t.Errorf("HitRate with no access = %f, want 0", snap.HitRate())
	}
}

func TestMetricsReset(t *testing.T) {
	var m cache.Metrics
	m.Hits.Store(10)
	m.Misses.Store(5)
	m.Reset()
	if m.Hits.Load() != 0 || m.Misses.Load() != 0 {
		t.Error("Reset should zero counters")
	}
}

// ---------------------------------------------------------------------------
// cache.GlyphCache tests
// ---------------------------------------------------------------------------

func TestGlyphCachePutGet(t *testing.T) {
	c := cache.NewGlyphCache(10)
	key := cache.GlyphKey{FontID: 1, GlyphID: 42, FontSize: 1600}
	val := cache.GlyphValue{Advance: 8.5, Width: 7.0, Height: 12.0}
	c.Put(key, val)

	got, ok := c.Get(key)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Advance != 8.5 || got.Width != 7.0 || got.Height != 12.0 {
		t.Errorf("got %+v, want %+v", got, val)
	}
}

func TestGlyphCacheMiss(t *testing.T) {
	c := cache.NewGlyphCache(10)
	_, ok := c.Get(cache.GlyphKey{FontID: 1, GlyphID: 99})
	if ok {
		t.Error("expected cache miss")
	}
}

func TestGlyphCacheEviction(t *testing.T) {
	c := cache.NewGlyphCache(3)
	for i := 0; i < 5; i++ {
		c.Put(cache.GlyphKey{FontID: 1, GlyphID: uint32(i)}, cache.GlyphValue{Advance: float32(i)})
	}
	if c.Len() != 3 {
		t.Errorf("Len() = %d, want 3 (capacity)", c.Len())
	}
	// First two should be evicted (LRU).
	_, ok := c.Get(cache.GlyphKey{FontID: 1, GlyphID: 0})
	if ok {
		t.Error("entry 0 should be evicted")
	}
	_, ok = c.Get(cache.GlyphKey{FontID: 1, GlyphID: 1})
	if ok {
		t.Error("entry 1 should be evicted")
	}
	// Last three should be present.
	_, ok = c.Get(cache.GlyphKey{FontID: 1, GlyphID: 2})
	if !ok {
		t.Error("entry 2 should be present")
	}
}

func TestGlyphCacheLRUOrder(t *testing.T) {
	c := cache.NewGlyphCache(3)
	k0 := cache.GlyphKey{GlyphID: 0}
	k1 := cache.GlyphKey{GlyphID: 1}
	k2 := cache.GlyphKey{GlyphID: 2}
	k3 := cache.GlyphKey{GlyphID: 3}

	c.Put(k0, cache.GlyphValue{})
	c.Put(k1, cache.GlyphValue{})
	c.Put(k2, cache.GlyphValue{})

	// Access k0 to make it recently used.
	c.Get(k0)

	// Insert k3 — should evict k1 (LRU), not k0.
	c.Put(k3, cache.GlyphValue{})

	_, ok := c.Get(k0)
	if !ok {
		t.Error("k0 should survive (recently accessed)")
	}
	_, ok = c.Get(k1)
	if ok {
		t.Error("k1 should be evicted (LRU)")
	}
}

func TestGlyphCacheUpdate(t *testing.T) {
	c := cache.NewGlyphCache(10)
	key := cache.GlyphKey{GlyphID: 1}
	c.Put(key, cache.GlyphValue{Advance: 5})
	c.Put(key, cache.GlyphValue{Advance: 10})

	got, ok := c.Get(key)
	if !ok || got.Advance != 10 {
		t.Errorf("update failed: got %+v, ok=%v", got, ok)
	}
	if c.Len() != 1 {
		t.Errorf("Len() = %d, want 1 after update", c.Len())
	}
}

func TestGlyphCacheMetrics(t *testing.T) {
	c := cache.NewGlyphCache(10)
	key := cache.GlyphKey{GlyphID: 1}
	c.Put(key, cache.GlyphValue{})
	c.Get(key)                  // hit
	c.Get(key)                  // hit
	c.Get(cache.GlyphKey{GlyphID: 2}) // miss

	m := c.Metrics()
	snap := m.Snapshot()
	if snap.Hits != 2 {
		t.Errorf("Hits = %d, want 2", snap.Hits)
	}
	if snap.Misses != 1 {
		t.Errorf("Misses = %d, want 1", snap.Misses)
	}
}

func TestGlyphCacheClear(t *testing.T) {
	c := cache.NewGlyphCache(10)
	c.Put(cache.GlyphKey{GlyphID: 1}, cache.GlyphValue{})
	c.Put(cache.GlyphKey{GlyphID: 2}, cache.GlyphValue{})
	c.Clear()
	if c.Len() != 0 {
		t.Errorf("Len() after Clear = %d, want 0", c.Len())
	}
}

func TestGlyphCacheDefaultCapacity(t *testing.T) {
	c := cache.NewGlyphCache(0)
	if c.Capacity != 256 {
		t.Errorf("default capacity = %d, want 256", c.Capacity)
	}
}

// ---------------------------------------------------------------------------
// cache.GlyphCache — M9.2 byte budget and memory.Evictor integration
// ---------------------------------------------------------------------------

func TestGlyphCacheBytes(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(10, 1<<20)
	if c.Bytes() != 0 {
		t.Errorf("empty cache Bytes() = %d, want 0", c.Bytes())
	}
	c.Put(cache.GlyphKey{GlyphID: 1}, cache.GlyphValue{ByteSize: 100})
	if c.Bytes() != 100 {
		t.Errorf("Bytes() = %d, want 100", c.Bytes())
	}
	c.Put(cache.GlyphKey{GlyphID: 2}, cache.GlyphValue{ByteSize: 200})
	if c.Bytes() != 300 {
		t.Errorf("Bytes() = %d, want 300", c.Bytes())
	}
}

func TestGlyphCacheBytesDefaultEstimate(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(10, 1<<20)
	c.Put(cache.GlyphKey{GlyphID: 1}, cache.GlyphValue{}) // ByteSize=0 → cache.DefaultGlyphEntrySize
	if c.Bytes() != cache.DefaultGlyphEntrySize {
		t.Errorf("Bytes() = %d, want %d (default estimate)", c.Bytes(), cache.DefaultGlyphEntrySize)
	}
}

func TestGlyphCacheBytesUpdate(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(10, 1<<20)
	key := cache.GlyphKey{GlyphID: 1}
	c.Put(key, cache.GlyphValue{ByteSize: 100})
	c.Put(key, cache.GlyphValue{ByteSize: 200}) // update
	if c.Bytes() != 200 {
		t.Errorf("Bytes() after update = %d, want 200", c.Bytes())
	}
}

func TestGlyphCacheByteLimitEviction(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(100, 500) // 500 byte limit
	c.Put(cache.GlyphKey{GlyphID: 0}, cache.GlyphValue{ByteSize: 200})
	c.Put(cache.GlyphKey{GlyphID: 1}, cache.GlyphValue{ByteSize: 200})
	c.Put(cache.GlyphKey{GlyphID: 2}, cache.GlyphValue{ByteSize: 200}) // should evict entry 0

	if c.Bytes() > 500 {
		t.Errorf("Bytes() = %d, exceeds limit 500", c.Bytes())
	}
	_, ok := c.Get(cache.GlyphKey{GlyphID: 0})
	if ok {
		t.Error("entry 0 should be evicted by byte limit")
	}
}

func TestGlyphCacheByteLimitOversizedItem(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(100, 50)
	c.Put(cache.GlyphKey{GlyphID: 1}, cache.GlyphValue{ByteSize: 100}) // exceeds limit
	if c.Len() != 0 {
		t.Error("oversized item should not be cached")
	}
	if c.Bytes() != 0 {
		t.Errorf("Bytes() = %d, want 0", c.Bytes())
	}
}

func TestGlyphCacheByteLimitLRUOrder(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(100, 300)                  // 300 byte limit
	c.Put(cache.GlyphKey{GlyphID: 0}, cache.GlyphValue{ByteSize: 100}) // oldest
	c.Put(cache.GlyphKey{GlyphID: 1}, cache.GlyphValue{ByteSize: 100})
	c.Put(cache.GlyphKey{GlyphID: 2}, cache.GlyphValue{ByteSize: 100})
	c.Get(cache.GlyphKey{GlyphID: 0}) // make recently used

	c.Put(cache.GlyphKey{GlyphID: 3}, cache.GlyphValue{ByteSize: 100}) // should evict 1 (LRU)

	_, ok := c.Get(cache.GlyphKey{GlyphID: 0})
	if !ok {
		t.Error("entry 0 should survive (recently accessed)")
	}
	_, ok = c.Get(cache.GlyphKey{GlyphID: 1})
	if ok {
		t.Error("entry 1 should be evicted (LRU)")
	}
}

func TestGlyphCacheNewWithBytesDefaultMax(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(0, 0)
	if c.Capacity != 256 {
		t.Errorf("default capacity = %d, want 256", c.Capacity)
	}
	if c.MaxBytes != 4<<20 {
		t.Errorf("default maxBytes = %d, want %d", c.MaxBytes, 4<<20)
	}
}

// ---------------------------------------------------------------------------
// cache.GlyphCache.Evict — M9.2 memory.Evictor integration
// ---------------------------------------------------------------------------

func TestGlyphCacheEvictNothing(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(10, 1<<20)
	c.Put(cache.GlyphKey{GlyphID: 1}, cache.GlyphValue{ByteSize: 100})

	freed := c.Evict(0)
	if freed != 0 {
		t.Errorf("Evict(0) freed %d, want 0", freed)
	}
	if c.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (nothing should be evicted)", c.Len())
	}
}

func TestGlyphCacheEvictExact(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(10, 1<<20)
	c.Put(cache.GlyphKey{GlyphID: 0}, cache.GlyphValue{ByteSize: 200})
	c.Put(cache.GlyphKey{GlyphID: 1}, cache.GlyphValue{ByteSize: 300})

	freed := c.Evict(200)
	if freed < 200 {
		t.Errorf("Evict(200) freed %d, want >= 200", freed)
	}
	if c.Bytes() > 300 {
		t.Errorf("Bytes() = %d, want <= 300 after eviction", c.Bytes())
	}
}

func TestGlyphCacheEvictAll(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(10, 1<<20)
	c.Put(cache.GlyphKey{GlyphID: 0}, cache.GlyphValue{ByteSize: 200})
	c.Put(cache.GlyphKey{GlyphID: 1}, cache.GlyphValue{ByteSize: 300})

	freed := c.Evict(1 << 20) // request more than total
	if freed != 500 {
		t.Errorf("Evict(1MB) freed %d, want 500 (all entries)", freed)
	}
	if c.Len() != 0 {
		t.Errorf("Len() = %d, want 0 (all evicted)", c.Len())
	}
	if c.Bytes() != 0 {
		t.Errorf("Bytes() = %d, want 0", c.Bytes())
	}
}

func TestGlyphCacheEvictEmpty(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(10, 1<<20)
	freed := c.Evict(1000)
	if freed != 0 {
		t.Errorf("Evict on empty cache freed %d, want 0", freed)
	}
}

func TestGlyphCacheEvictLRUOrder(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(10, 1<<20)
	c.Put(cache.GlyphKey{GlyphID: 0}, cache.GlyphValue{ByteSize: 100}) // oldest
	c.Put(cache.GlyphKey{GlyphID: 1}, cache.GlyphValue{ByteSize: 200})
	c.Put(cache.GlyphKey{GlyphID: 2}, cache.GlyphValue{ByteSize: 300}) // newest

	c.Get(cache.GlyphKey{GlyphID: 0}) // make recently used

	freed := c.Evict(200) // should evict entry 1 (200 bytes, LRU)
	if freed < 200 {
		t.Errorf("Evict(200) freed %d, want >= 200", freed)
	}

	_, ok := c.Get(cache.GlyphKey{GlyphID: 0})
	if !ok {
		t.Error("entry 0 should survive eviction (recently accessed)")
	}
}

func TestGlyphCacheEvictReturnsActualBytes(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(10, 1<<20)
	c.Put(cache.GlyphKey{GlyphID: 0}, cache.GlyphValue{ByteSize: 150})

	freed := c.Evict(1000) // request more than available
	if freed != 150 {
		t.Errorf("Evict(1000) freed %d, want 150 (actual available)", freed)
	}
}

func TestGlyphCacheEvictMetrics(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(10, 1<<20)
	c.Put(cache.GlyphKey{GlyphID: 0}, cache.GlyphValue{ByteSize: 100})
	c.Put(cache.GlyphKey{GlyphID: 1}, cache.GlyphValue{ByteSize: 200})

	c.Evict(100)
	snap := c.Metrics().Snapshot()
	if snap.Evictions < 1 {
		t.Errorf("Evictions = %d, want >= 1", snap.Evictions)
	}
}

func TestGlyphCacheEvictConcurrent(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(100, 1<<20)

	for i := 0; i < 50; i++ {
		c.Put(cache.GlyphKey{GlyphID: uint32(i)}, cache.GlyphValue{ByteSize: 100})
	}

	var wg sync.WaitGroup
	const N = 20
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			c.Evict(500)
		}()
	}
	wg.Wait()

	if c.Len() < 0 {
		t.Error("Len() should be non-negative after concurrent eviction")
	}
	if c.Bytes() < 0 {
		t.Error("Bytes() should be non-negative after concurrent eviction")
	}
}

func TestGlyphCacheClose(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(10, 1<<20)
	c.Put(cache.GlyphKey{GlyphID: 1}, cache.GlyphValue{ByteSize: 100})
	c.Put(cache.GlyphKey{GlyphID: 2}, cache.GlyphValue{ByteSize: 200})
	c.Close()
	if c.Len() != 0 || c.Bytes() != 0 {
		t.Errorf("after Close: Len=%d, Bytes=%d", c.Len(), c.Bytes())
	}
}

func TestGlyphCacheBytesConsistentWithLen(t *testing.T) {
	c := cache.NewGlyphCacheWithBytes(3, 10000) // high byte limit, entry-count limited
	for i := 0; i < 10; i++ {
		c.Put(cache.GlyphKey{GlyphID: uint32(i)}, cache.GlyphValue{ByteSize: 50})
	}
	if c.Len() != 3 {
		t.Errorf("Len() = %d, want 3", c.Len())
	}
	if c.Bytes() != 150 {
		t.Errorf("Bytes() = %d, want 150 (3 entries * 50 bytes)", c.Bytes())
	}
}

func TestGlyphCacheByteLimitAndEntryLimit(t *testing.T) {
	// Entry count should be enforced even when byte limit is not reached.
	c := cache.NewGlyphCacheWithBytes(2, 1<<20)
	c.Put(cache.GlyphKey{GlyphID: 0}, cache.GlyphValue{ByteSize: 10})
	c.Put(cache.GlyphKey{GlyphID: 1}, cache.GlyphValue{ByteSize: 10})
	c.Put(cache.GlyphKey{GlyphID: 2}, cache.GlyphValue{ByteSize: 10}) // evicts entry 0

	if c.Len() != 2 {
		t.Errorf("Len() = %d, want 2 (entry limit)", c.Len())
	}
}

// ---------------------------------------------------------------------------
// cache.ImageCache tests
// ---------------------------------------------------------------------------

func TestImageCachePutGet(t *testing.T) {
	c := cache.NewImageCache(1 << 20) // 1 MB
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	val := cache.ImageValue{Image: img, ByteSize: 400}
	c.Put("test.png", val)

	got, ok := c.Get("test.png")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.ByteSize != 400 {
		t.Errorf("ByteSize = %d, want 400", got.ByteSize)
	}
}

func TestImageCacheMiss(t *testing.T) {
	c := cache.NewImageCache(1 << 20)
	_, ok := c.Get("missing.png")
	if ok {
		t.Error("expected cache miss")
	}
}

func TestImageCacheByteLimit(t *testing.T) {
	c := cache.NewImageCache(1000) // 1000 byte limit
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))

	c.Put("a.png", cache.ImageValue{Image: img, ByteSize: 400})
	c.Put("b.png", cache.ImageValue{Image: img, ByteSize: 400})
	c.Put("c.png", cache.ImageValue{Image: img, ByteSize: 400}) // Should evict a.png

	if c.Len() > 3 {
		t.Errorf("Len() = %d, should be <= 3", c.Len())
	}
	if c.Bytes() > 1000 {
		t.Errorf("Bytes() = %d, exceeds limit 1000", c.Bytes())
	}
}

func TestImageCacheEvictionMetrics(t *testing.T) {
	c := cache.NewImageCache(500)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	c.Put("a.png", cache.ImageValue{Image: img, ByteSize: 300})
	c.Put("b.png", cache.ImageValue{Image: img, ByteSize: 300}) // Evicts a

	snap := c.Metrics().Snapshot()
	if snap.Evictions < 1 {
		t.Errorf("Evictions = %d, want >= 1", snap.Evictions)
	}
}

func TestImageCacheOversizedItem(t *testing.T) {
	c := cache.NewImageCache(100)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	c.Put("huge.png", cache.ImageValue{Image: img, ByteSize: 200}) // Exceeds limit
	if c.Len() != 0 {
		t.Error("oversized item should not be cached")
	}
}

func TestImageCacheUpdate(t *testing.T) {
	c := cache.NewImageCache(1 << 20)
	img1 := image.NewRGBA(image.Rect(0, 0, 5, 5))
	img2 := image.NewRGBA(image.Rect(0, 0, 10, 10))
	c.Put("a.png", cache.ImageValue{Image: img1, ByteSize: 100})
	c.Put("a.png", cache.ImageValue{Image: img2, ByteSize: 400})

	got, ok := c.Get("a.png")
	if !ok {
		t.Fatal("expected cache hit after update")
	}
	if got.ByteSize != 400 {
		t.Errorf("ByteSize = %d, want 400 after update", got.ByteSize)
	}
	if c.Bytes() != 400 {
		t.Errorf("Bytes() = %d, want 400", c.Bytes())
	}
}

func TestImageCacheClear(t *testing.T) {
	c := cache.NewImageCache(1 << 20)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	c.Put("a.png", cache.ImageValue{Image: img, ByteSize: 100})
	c.Clear()
	if c.Len() != 0 || c.Bytes() != 0 {
		t.Errorf("after Clear: Len=%d, Bytes=%d", c.Len(), c.Bytes())
	}
}

func TestImageCacheClose(t *testing.T) {
	c := cache.NewImageCache(1 << 20)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	c.Put("a.png", cache.ImageValue{Image: img, ByteSize: 100})
	c.Close()
	if c.Len() != 0 {
		t.Error("Close should clear cache")
	}
}

func TestImageCacheDefaultLimit(t *testing.T) {
	c := cache.NewImageCache(0)
	if c.MaxBytes != 64<<20 {
		t.Errorf("default maxBytes = %d, want %d", c.MaxBytes, 64<<20)
	}
}

// ---------------------------------------------------------------------------
// GetOrLoad — duplicate decode prevention
// ---------------------------------------------------------------------------

func TestImageCacheGetOrLoad(t *testing.T) {
	c := cache.NewImageCache(1 << 20)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))

	callCount := 0
	load := func() (cache.ImageValue, error) {
		callCount++
		return cache.ImageValue{Image: img, ByteSize: 100}, nil
	}

	v, err := c.GetOrLoad("test.png", load)
	if err != nil {
		t.Fatal(err)
	}
	if v.ByteSize != 100 {
		t.Errorf("ByteSize = %d, want 100", v.ByteSize)
	}
	if callCount != 1 {
		t.Errorf("load called %d times, want 1", callCount)
	}

	// Second call should hit cache.
	v, err = c.GetOrLoad("test.png", load)
	if err != nil {
		t.Fatal(err)
	}
	if callCount != 1 {
		t.Errorf("load called %d times, want 1 (cached)", callCount)
	}
}

func TestImageCacheGetOrLoadError(t *testing.T) {
	c := cache.NewImageCache(1 << 20)
	loadErr := errors.New("decode failed")
	_, err := c.GetOrLoad("bad.png", func() (cache.ImageValue, error) {
		return cache.ImageValue{}, loadErr
	})
	if err != loadErr {
		t.Errorf("err = %v, want %v", err, loadErr)
	}
	if c.Len() != 0 {
		t.Error("failed load should not cache")
	}
}

func TestImageCacheGetOrLoadConcurrent(t *testing.T) {
	c := cache.NewImageCache(1 << 20)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})

	var callCount int64
	var mu sync.Mutex
	load := func() (cache.ImageValue, error) {
		mu.Lock()
		callCount++
		mu.Unlock()
		return cache.ImageValue{Image: img, ByteSize: 100}, nil
	}

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, err := c.GetOrLoad("concurrent.png", load)
			if err != nil {
				t.Errorf("GetOrLoad error: %v", err)
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	count := callCount
	mu.Unlock()
	if count != 1 {
		t.Errorf("load called %d times, want 1 (deduplicated)", count)
	}
}

// ---------------------------------------------------------------------------
// cache.ImageCache.Evict — M9.2 memory.Evictor integration
// ---------------------------------------------------------------------------

func TestImageCacheEvictNothing(t *testing.T) {
	c := cache.NewImageCache(1 << 20)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	c.Put("a.png", cache.ImageValue{Image: img, ByteSize: 400})

	freed := c.Evict(0)
	if freed != 0 {
		t.Errorf("Evict(0) freed %d, want 0", freed)
	}
	if c.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (nothing should be evicted)", c.Len())
	}
}

func TestImageCacheEvictExact(t *testing.T) {
	c := cache.NewImageCache(1 << 20)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	c.Put("a.png", cache.ImageValue{Image: img, ByteSize: 400})
	c.Put("b.png", cache.ImageValue{Image: img, ByteSize: 300})

	freed := c.Evict(400)
	if freed < 400 {
		t.Errorf("Evict(400) freed %d, want >= 400", freed)
	}
	if c.Bytes() > 300 {
		t.Errorf("Bytes() = %d, want <= 300 after eviction", c.Bytes())
	}
}

func TestImageCacheEvictAll(t *testing.T) {
	c := cache.NewImageCache(1 << 20)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	c.Put("a.png", cache.ImageValue{Image: img, ByteSize: 400})
	c.Put("b.png", cache.ImageValue{Image: img, ByteSize: 300})

	freed := c.Evict(1 << 20) // request more than total
	if freed != 700 {
		t.Errorf("Evict(1MB) freed %d, want 700 (all entries)", freed)
	}
	if c.Len() != 0 {
		t.Errorf("Len() = %d, want 0 (all evicted)", c.Len())
	}
	if c.Bytes() != 0 {
		t.Errorf("Bytes() = %d, want 0", c.Bytes())
	}
}

func TestImageCacheEvictEmpty(t *testing.T) {
	c := cache.NewImageCache(1 << 20)
	freed := c.Evict(1000)
	if freed != 0 {
		t.Errorf("Evict on empty cache freed %d, want 0", freed)
	}
}

func TestImageCacheEvictLRUOrder(t *testing.T) {
	c := cache.NewImageCache(1 << 20)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	c.Put("a.png", cache.ImageValue{Image: img, ByteSize: 100}) // oldest
	c.Put("b.png", cache.ImageValue{Image: img, ByteSize: 200})
	c.Put("c.png", cache.ImageValue{Image: img, ByteSize: 300}) // newest

	// Access a.png to make it recently used.
	c.Get("a.png")

	freed := c.Evict(200) // Should evict b (200 bytes) first, then c if needed
	if freed < 200 {
		t.Errorf("Evict(200) freed %d, want >= 200", freed)
	}

	// a.png should survive (recently accessed).
	_, ok := c.Get("a.png")
	if !ok {
		t.Error("a.png should survive eviction (recently accessed)")
	}
}

func TestImageCacheEvictReturnsActualBytes(t *testing.T) {
	c := cache.NewImageCache(1 << 20)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	c.Put("a.png", cache.ImageValue{Image: img, ByteSize: 150})

	freed := c.Evict(1000) // Request more than available
	if freed != 150 {
		t.Errorf("Evict(1000) freed %d, want 150 (actual available)", freed)
	}
}

func TestImageCacheEvictMetrics(t *testing.T) {
	c := cache.NewImageCache(1 << 20)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	c.Put("a.png", cache.ImageValue{Image: img, ByteSize: 100})
	c.Put("b.png", cache.ImageValue{Image: img, ByteSize: 200})

	c.Evict(100)
	snap := c.Metrics().Snapshot()
	if snap.Evictions < 1 {
		t.Errorf("Evictions = %d, want >= 1", snap.Evictions)
	}
}

func TestImageCacheEvictConcurrent(t *testing.T) {
	c := cache.NewImageCache(1 << 20)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))

	// Fill cache.
	for i := 0; i < 50; i++ {
		key := cache.ImageKey("img" + string(rune('A'+i%26)) + ".png")
		c.Put(key, cache.ImageValue{Image: img, ByteSize: 100})
	}

	var wg sync.WaitGroup
	const N = 20
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			c.Evict(500)
		}()
	}
	wg.Wait()

	// Cache should be in a consistent state.
	if c.Len() < 0 {
		t.Error("Len() should be non-negative after concurrent eviction")
	}
	if c.Bytes() < 0 {
		t.Error("Bytes() should be non-negative after concurrent eviction")
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkGlyphCachePut(b *testing.B) {
	c := cache.NewGlyphCache(1024)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Put(cache.GlyphKey{FontID: 1, GlyphID: uint32(i % 500)}, cache.GlyphValue{Advance: 8.0})
	}
}

func BenchmarkGlyphCacheGet(b *testing.B) {
	c := cache.NewGlyphCache(1024)
	for i := uint32(0); i < 500; i++ {
		c.Put(cache.GlyphKey{FontID: 1, GlyphID: i}, cache.GlyphValue{Advance: 8.0})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(cache.GlyphKey{FontID: 1, GlyphID: uint32(i % 500)})
	}
}

func BenchmarkGlyphCachePutWithBytes(b *testing.B) {
	c := cache.NewGlyphCacheWithBytes(1024, 1<<20)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Put(cache.GlyphKey{FontID: 1, GlyphID: uint32(i % 500)}, cache.GlyphValue{Advance: 8.0, ByteSize: 32})
	}
}

func BenchmarkGlyphCacheEvict(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		c := cache.NewGlyphCacheWithBytes(1024, 1<<20)
		for j := 0; j < 100; j++ {
			c.Put(cache.GlyphKey{GlyphID: uint32(j)}, cache.GlyphValue{ByteSize: 32})
		}
		b.StartTimer()
		c.Evict(1600) // evict ~50 entries
	}
}

func BenchmarkImageCachePut(b *testing.B) {
	c := cache.NewImageCache(64 << 20)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := cache.ImageKey("img" + string(rune('A'+i%26)) + ".png")
		c.Put(key, cache.ImageValue{Image: img, ByteSize: 100})
	}
}

func BenchmarkImageCacheGet(b *testing.B) {
	c := cache.NewImageCache(64 << 20)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	for i := 0; i < 100; i++ {
		key := cache.ImageKey("img" + string(rune('A'+i%26)) + ".png")
		c.Put(key, cache.ImageValue{Image: img, ByteSize: 100})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := cache.ImageKey("img" + string(rune('A'+i%100)) + ".png")
		c.Get(key)
	}
}

func BenchmarkImageCacheEvict(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		c := cache.NewImageCache(64 << 20)
		for j := 0; j < 100; j++ {
			key := cache.ImageKey("img" + string(rune('A'+j%26)) + ".png")
			c.Put(key, cache.ImageValue{Image: img, ByteSize: 100})
		}
		b.StartTimer()
		c.Evict(5000) // evict ~50 entries
	}
}
