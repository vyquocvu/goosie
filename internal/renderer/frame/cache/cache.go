// Package cache provides bounded caches for the raster backend.
//
// M6.3: Glyph and image caches with byte-based limits, LRU eviction,
// hit/eviction metrics, and duplicate-decode prevention.
package cache

import (
	"image"
	"sync"
	"sync/atomic"
)

// ---------------------------------------------------------------------------
// Metrics — cache hit and eviction counters
// ---------------------------------------------------------------------------

// Metrics tracks cache hit and eviction statistics.
type Metrics struct {
	Hits      atomic.Int64
	Misses    atomic.Int64
	Evictions atomic.Int64
}

// Snapshot returns a point-in-time copy of the metrics.
func (m *Metrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		Hits:      m.Hits.Load(),
		Misses:    m.Misses.Load(),
		Evictions: m.Evictions.Load(),
	}
}

// Reset zeroes all counters.
func (m *Metrics) Reset() {
	m.Hits.Store(0)
	m.Misses.Store(0)
	m.Evictions.Store(0)
}

// MetricsSnapshot is an immutable copy of cache metrics.
type MetricsSnapshot struct {
	Hits      int64
	Misses    int64
	Evictions int64
}

// HitRate returns the hit rate as a fraction [0,1]. Returns 0 if no accesses.
func (s MetricsSnapshot) HitRate() float64 {
	total := s.Hits + s.Misses
	if total == 0 {
		return 0
	}
	return float64(s.Hits) / float64(total)
}

// ---------------------------------------------------------------------------
// GlyphCache — bounded cache for shaped glyph data
// ---------------------------------------------------------------------------

// GlyphKey uniquely identifies a cached glyph.
type GlyphKey struct {
	FontID   uint32
	GlyphID  uint32
	FontSize uint32 // Font size encoded as fixed-point (e.g. size*100)
}

// GlyphValue holds cached glyph metrics.
type GlyphValue struct {
	Advance float32
	Width   float32
	Height  float32
}

// glyphEntry is an LRU list node.
type glyphEntry struct {
	key   GlyphKey
	value GlyphValue
	prev  *glyphEntry
	next  *glyphEntry
}

// GlyphCache is a bounded LRU cache for glyph metrics.
// All operations are safe for concurrent use.
type GlyphCache struct {
	mu       sync.Mutex
	capacity int
	items    map[GlyphKey]*glyphEntry
	head     *glyphEntry // most recently used
	tail     *glyphEntry // least recently used
	metrics  Metrics
}

// NewGlyphCache creates a glyph cache with the given maximum entry count.
func NewGlyphCache(capacity int) *GlyphCache {
	if capacity <= 0 {
		capacity = 256
	}
	return &GlyphCache{
		capacity: capacity,
		items:    make(map[GlyphKey]*glyphEntry, capacity),
	}
}

// Get retrieves a glyph from the cache. Returns the value and true on hit.
func (c *GlyphCache) Get(key GlyphKey) (GlyphValue, bool) {
	c.mu.Lock()
	e, ok := c.items[key]
	if ok {
		c.moveToFront(e)
		c.metrics.Hits.Add(1)
		v := e.value
		c.mu.Unlock()
		return v, true
	}
	c.metrics.Misses.Add(1)
	c.mu.Unlock()
	return GlyphValue{}, false
}

// Put inserts or updates a glyph in the cache.
func (c *GlyphCache) Put(key GlyphKey, value GlyphValue) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.items[key]; ok {
		e.value = value
		c.moveToFront(e)
		return
	}

	// Evict if at capacity.
	if len(c.items) >= c.capacity {
		c.evictLRU()
	}

	e := &glyphEntry{key: key, value: value}
	c.items[key] = e
	c.pushFront(e)
}

// Len returns the current number of entries.
func (c *GlyphCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// Metrics returns the cache metrics.
func (c *GlyphCache) Metrics() *Metrics { return &c.metrics }

// Clear removes all entries and resets metrics.
func (c *GlyphCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[GlyphKey]*glyphEntry, c.capacity)
	c.head = nil
	c.tail = nil
	c.metrics.Reset()
}

func (c *GlyphCache) evictLRU() {
	if c.tail == nil {
		return
	}
	delete(c.items, c.tail.key)
	c.removeEntry(c.tail)
	c.metrics.Evictions.Add(1)
}

func (c *GlyphCache) moveToFront(e *glyphEntry) {
	if c.head == e {
		return
	}
	c.removeEntry(e)
	c.pushFront(e)
}

func (c *GlyphCache) pushFront(e *glyphEntry) {
	e.prev = nil
	e.next = c.head
	if c.head != nil {
		c.head.prev = e
	}
	c.head = e
	if c.tail == nil {
		c.tail = e
	}
}

func (c *GlyphCache) removeEntry(e *glyphEntry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		c.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		c.tail = e.prev
	}
	e.prev = nil
	e.next = nil
}

// ---------------------------------------------------------------------------
// ImageCache — bounded decoded image cache with byte-based limits
// ---------------------------------------------------------------------------

// ImageKey uniquely identifies a cached image by URL.
type ImageKey string

// ImageValue holds a decoded image and its byte cost.
type ImageValue struct {
	Image    image.Image
	ByteSize int64 // Approximate memory cost in bytes
}

// imageEntry is an LRU list node.
type imageEntry struct {
	key   ImageKey
	value ImageValue
	prev  *imageEntry
	next  *imageEntry
}

// ImageCache is a bounded LRU cache for decoded images with byte-based limits.
// All operations are safe for concurrent use.
type ImageCache struct {
	mu           sync.Mutex
	maxBytes     int64
	currentBytes int64
	items        map[ImageKey]*imageEntry
	head         *imageEntry // most recently used
	tail         *imageEntry // least recently used
	metrics      Metrics

	// inFlight prevents duplicate concurrent decode of the same resource.
	inFlight map[ImageKey]*sync.Once
}

// NewImageCache creates an image cache with the given byte limit.
func NewImageCache(maxBytes int64) *ImageCache {
	if maxBytes <= 0 {
		maxBytes = 64 << 20 // 64 MB default
	}
	return &ImageCache{
		maxBytes: maxBytes,
		items:    make(map[ImageKey]*imageEntry),
		inFlight: make(map[ImageKey]*sync.Once),
	}
}

// Get retrieves an image from the cache. Returns the value and true on hit.
func (c *ImageCache) Get(key ImageKey) (ImageValue, bool) {
	c.mu.Lock()
	e, ok := c.items[key]
	if ok {
		c.moveToFront(e)
		c.metrics.Hits.Add(1)
		v := e.value
		c.mu.Unlock()
		return v, true
	}
	c.metrics.Misses.Add(1)
	c.mu.Unlock()
	return ImageValue{}, false
}

// Put inserts or updates an image in the cache. Evicts LRU entries to stay
// within the byte limit.
func (c *ImageCache) Put(key ImageKey, value ImageValue) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove old entry if updating.
	if e, ok := c.items[key]; ok {
		c.currentBytes -= e.value.ByteSize
		c.removeEntry(e)
		delete(c.items, key)
	}

	// Evict until we have room.
	for c.currentBytes+value.ByteSize > c.maxBytes && c.tail != nil {
		c.evictLRU()
	}

	// Don't cache if single item exceeds limit.
	if value.ByteSize > c.maxBytes {
		return
	}

	e := &imageEntry{key: key, value: value}
	c.items[key] = e
	c.currentBytes += value.ByteSize
	c.pushFront(e)
}

// Len returns the current number of cached images.
func (c *ImageCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// Bytes returns the current total byte usage.
func (c *ImageCache) Bytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentBytes
}

// Metrics returns the cache metrics.
func (c *ImageCache) Metrics() *Metrics { return &c.metrics }

// Clear removes all entries and resets metrics.
func (c *ImageCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[ImageKey]*imageEntry)
	c.head = nil
	c.tail = nil
	c.currentBytes = 0
	c.inFlight = make(map[ImageKey]*sync.Once)
	c.metrics.Reset()
}

// GetOrLoad retrieves an image or calls load exactly once per key to prevent
// duplicate concurrent decodes. The load function is called at most once per
// key across all concurrent callers.
func (c *ImageCache) GetOrLoad(key ImageKey, load func() (ImageValue, error)) (ImageValue, error) {
	// Fast path: cache hit.
	if v, ok := c.Get(key); ok {
		return v, nil
	}

	// Slow path: ensure only one goroutine loads this key.
	c.mu.Lock()
	once, exists := c.inFlight[key]
	if !exists {
		once = &sync.Once{}
		c.inFlight[key] = once
	}
	c.mu.Unlock()

	var result ImageValue
	var loadErr error
	once.Do(func() {
		result, loadErr = load()
		if loadErr == nil {
			c.Put(key, result)
		}
		// Clean up inFlight entry.
		c.mu.Lock()
		delete(c.inFlight, key)
		c.mu.Unlock()
	})

	return result, loadErr
}

// Evict removes LRU entries until at least targetBytes have been freed or the
// cache is empty. Returns the number of bytes actually freed.
//
// This satisfies the memory.Evictor interface:
//
//	func(targetBytes uint64) uint64
func (c *ImageCache) Evict(targetBytes uint64) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	var freed uint64
	for freed < targetBytes && c.tail != nil {
		freed += uint64(c.tail.value.ByteSize)
		c.evictLRU()
	}
	return freed
}

// Close releases all cached resources and resets state.
func (c *ImageCache) Close() {
	c.Clear()
}

func (c *ImageCache) evictLRU() {
	if c.tail == nil {
		return
	}
	c.currentBytes -= c.tail.value.ByteSize
	delete(c.items, c.tail.key)
	c.removeEntry(c.tail)
	c.metrics.Evictions.Add(1)
}

func (c *ImageCache) moveToFront(e *imageEntry) {
	if c.head == e {
		return
	}
	c.removeEntry(e)
	c.pushFront(e)
}

func (c *ImageCache) pushFront(e *imageEntry) {
	e.prev = nil
	e.next = c.head
	if c.head != nil {
		c.head.prev = e
	}
	c.head = e
	if c.tail == nil {
		c.tail = e
	}
}

func (c *ImageCache) removeEntry(e *imageEntry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		c.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		c.tail = e.prev
	}
	e.prev = nil
	e.next = nil
}
