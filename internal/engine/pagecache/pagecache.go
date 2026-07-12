// Package pagecache provides a bounded LRU cache for fully-rendered page
// snapshots, enabling instant back/forward navigation without re-fetching
// and re-parsing.
//
// The cache is bounded by both entry count and byte budget, and integrates
// with the memory.Manager via the Evict method.
//
// M9.2: Bound page cache with LRU eviction and memory.Manager integration.
package pagecache

import (
	"sync"
	"sync/atomic"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// Config holds the page cache limits.
type Config struct {
	MaxEntries int
	MaxBytes   int64
}

// DefaultConfig returns conservative defaults matching
// ResourcePrefetchLimits.MaxCachedPages (3) with a 32 MB byte budget.
func DefaultConfig() Config {
	return Config{
		MaxEntries: 3,
		MaxBytes:   32 << 20, // 32 MB
	}
}

// ---------------------------------------------------------------------------
// PageEntry — immutable snapshot of a fully-loaded page
// ---------------------------------------------------------------------------

// PageEntry stores the data needed to restore a page without re-fetching.
type PageEntry struct {
	URL      string
	Title    string
	ByteSize int64 // approximate memory cost; 0 means use DefaultEntrySize
}

// DefaultEntrySize is the fallback byte cost when ByteSize is zero.
const DefaultEntrySize int64 = 4096

func entryBytes(e PageEntry) int64 {
	if e.ByteSize > 0 {
		return e.ByteSize
	}
	return DefaultEntrySize
}

// ---------------------------------------------------------------------------
// Metrics
// ---------------------------------------------------------------------------

// Metrics tracks cache hit and eviction statistics.
type Metrics struct {
	Hits      atomic.Int64
	Misses    atomic.Int64
	Evictions atomic.Int64
}

// Snapshot returns an immutable point-in-time copy.
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
// LRU list node
// ---------------------------------------------------------------------------

type pageEntry struct {
	key   string
	value PageEntry
	prev  *pageEntry
	next  *pageEntry
}

// ---------------------------------------------------------------------------
// Cache — bounded LRU page cache
// ---------------------------------------------------------------------------

// Cache is a bounded LRU cache for page snapshots keyed by URL.
// All operations are safe for concurrent use.
type Cache struct {
	mu           sync.Mutex
	capacity     int
	maxBytes     int64
	currentBytes int64
	items        map[string]*pageEntry
	head         *pageEntry // most recently used
	tail         *pageEntry // least recently used
	metrics      Metrics
}

// New creates a page cache with the given maximum entry count.
// Byte budget is unlimited; use NewFromConfig for byte-limited caches.
// A zero or negative capacity defaults to 3.
func New(maxEntries int, maxBytes int64) *Cache {
	if maxEntries <= 0 {
		maxEntries = 3
	}
	return &Cache{
		capacity: maxEntries,
		maxBytes: maxBytes,
		items:    make(map[string]*pageEntry, maxEntries),
	}
}

// NewFromConfig creates a page cache from a Config.
func NewFromConfig(cfg Config) *Cache {
	return New(cfg.MaxEntries, cfg.MaxBytes)
}

// Get retrieves a page entry by URL. Returns the entry and true on hit.
func (c *Cache) Get(url string) (PageEntry, bool) {
	c.mu.Lock()
	e, ok := c.items[url]
	if ok {
		c.moveToFront(e)
		c.metrics.Hits.Add(1)
		v := e.value
		c.mu.Unlock()
		return v, true
	}
	c.metrics.Misses.Add(1)
	c.mu.Unlock()
	return PageEntry{}, false
}

// Put inserts or updates a page entry. Evicts LRU entries to respect
// both the entry count and byte budget.
func (c *Cache) Put(entry PageEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry.
	if e, ok := c.items[entry.URL]; ok {
		c.currentBytes -= entryBytes(e.value)
		e.value = entry
		c.currentBytes += entryBytes(entry)
		c.moveToFront(e)
		return
	}

	// Skip entries that exceed the byte budget alone.
	if c.maxBytes > 0 && entryBytes(entry) > c.maxBytes {
		return
	}

	// Evict to make room (count or byte budget).
	for (len(c.items) >= c.capacity) ||
		(c.maxBytes > 0 && c.currentBytes+entryBytes(entry) > c.maxBytes && c.tail != nil) {
		c.evictLRU()
	}

	e := &pageEntry{key: entry.URL, value: entry}
	c.items[entry.URL] = e
	c.currentBytes += entryBytes(entry)
	c.pushFront(e)
}

// Remove deletes a page entry by URL.
func (c *Cache) Remove(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.items[url]
	if !ok {
		return
	}
	c.currentBytes -= entryBytes(e.value)
	delete(c.items, url)
	c.removeEntry(e)
}

// Len returns the current number of entries.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// Bytes returns the current total byte usage.
func (c *Cache) Bytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentBytes
}

// Metrics returns the cache metrics.
func (c *Cache) Metrics() *Metrics { return &c.metrics }

// Clear removes all entries and resets metrics.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*pageEntry, c.capacity)
	c.head = nil
	c.tail = nil
	c.currentBytes = 0
	c.metrics.Reset()
}

// Evict removes LRU entries until at least targetBytes have been freed or
// the cache is empty. Returns the number of bytes actually freed.
//
// This satisfies the memory.Evictor interface:
//
//	func(targetBytes uint64) uint64
func (c *Cache) Evict(targetBytes uint64) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	var freed uint64
	for freed < targetBytes && c.tail != nil {
		freed += uint64(entryBytes(c.tail.value))
		c.evictLRU()
	}
	return freed
}

// Close removes all entries and resets state.
func (c *Cache) Close() {
	c.Clear()
}

// ---------------------------------------------------------------------------
// Internal LRU helpers
// ---------------------------------------------------------------------------

func (c *Cache) evictLRU() {
	if c.tail == nil {
		return
	}
	c.currentBytes -= entryBytes(c.tail.value)
	delete(c.items, c.tail.key)
	c.removeEntry(c.tail)
	c.metrics.Evictions.Add(1)
}

func (c *Cache) moveToFront(e *pageEntry) {
	if c.head == e {
		return
	}
	c.removeEntry(e)
	c.pushFront(e)
}

func (c *Cache) pushFront(e *pageEntry) {
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

func (c *Cache) removeEntry(e *pageEntry) {
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
