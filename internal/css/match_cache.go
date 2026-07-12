// Package css — M9.2: Bounded selector and computed-style caches.
//
// This file implements:
//
//  1. ElementKey — a value-type fingerprint for an element's identity
//     (tag, ID, sorted class list) used as the cache key.
//
//  2. MatchCache — a bounded LRU cache for the result of CompiledStyleSheet.MatchRules.
//     When a style resolution pass calls MatchRules for the same element type
//     (same tag + ID + class set), the cached result is returned without
//     re-running selector matching.
//
//     Scope/invalidation contract:
//     - The cache is keyed by (tag, ID, sorted-classes) only — not by attribute
//     values, pseudo-class state, or structural position.
//     - Callers MUST Clear() or invalidate the cache when the stylesheet changes,
//     when attribute-selector-sensitive attributes change, or when structural
//     pseudo-classes (:first-child etc.) may produce different results.
//     - A MatchCache is safe for concurrent use.
//
//  3. StylePool.Evict — enables the global memory.Manager to drive eviction of
//     the computed-style deduplication pool (see computed.go).
package css

import (
	"sort"
	"strings"
	"sync"
	"unsafe"
)

// ---------------------------------------------------------------------------
// ElementKey — value-type fingerprint
// ---------------------------------------------------------------------------

// ElementKey is a comparable value that identifies an element's selector-relevant
// identity: tag name (lowercase), ID attribute, and sorted class list.
//
// It deliberately omits attribute values, pseudo-class state (hover, focus,
// :nth-child position) and DOM structural position so that elements with the
// same type characteristics share a single cache entry. Callers that need
// per-structural matching must bypass or clear the cache.
//
// ElementKey is a value type — it contains no pointers and is safe to use as a
// map key.
type ElementKey struct {
	// Tag is the lowercase tag name (e.g. "div", "p").
	Tag string

	// ID is the value of the id attribute, or "" if absent.
	ID string

	// ClassKey is the space-joined sorted class list (e.g. "active container nav").
	// Sorting makes the key deterministic regardless of class attribute order.
	ClassKey string
}

// ElementKeyFromElement builds an ElementKey from an Element interface value.
// The class list is sorted alphabetically so the key is order-independent.
func ElementKeyFromElement(elem Element) ElementKey {
	tag := strings.ToLower(elem.TagName())
	id := elem.ID()
	classes := elem.Classes()

	var classKey string
	if len(classes) == 1 {
		classKey = classes[0]
	} else if len(classes) > 1 {
		// Sort a copy so we don't mutate the caller's slice.
		sorted := make([]string, len(classes))
		copy(sorted, classes)
		sort.Strings(sorted)
		classKey = strings.Join(sorted, " ")
	}

	return ElementKey{Tag: tag, ID: id, ClassKey: classKey}
}

// ---------------------------------------------------------------------------
// MatchCache — bounded LRU cache for MatchRules results
// ---------------------------------------------------------------------------

// MatchCacheConfig configures the MatchCache.
type MatchCacheConfig struct {
	// MaxEntries is the maximum number of cached entries.
	// If 0, a default of 512 is used.
	MaxEntries int
}

// DefaultMatchCacheConfig returns a MatchCacheConfig with sensible defaults.
func DefaultMatchCacheConfig() MatchCacheConfig {
	return MatchCacheConfig{MaxEntries: 512}
}

// MatchCacheMetrics is an immutable snapshot of cache hit/miss/eviction counts.
type MatchCacheMetrics struct {
	Hits      int64
	Misses    int64
	Evictions int64
}

// matchEntry is a node in the doubly-linked LRU list.
type matchEntry struct {
	key   ElementKey
	rules []CompiledRule
	prev  *matchEntry
	next  *matchEntry
}

// byteSize returns the approximate memory cost of this entry.
// We count key storage (three string headers + their backing bytes) plus the
// slice header and each CompiledRule value.
func (e *matchEntry) byteSize() uint64 {
	// String header is 16 bytes on 64-bit (ptr + len).
	const stringHeaderSize = 16
	cost := uint64(stringHeaderSize*3) + // Tag, ID, ClassKey headers
		uint64(len(e.key.Tag)) +
		uint64(len(e.key.ID)) +
		uint64(len(e.key.ClassKey)) +
		uint64(unsafe.Sizeof(*e)) // node overhead (key struct + pointers)

	// Each CompiledRule is stored by value. Slice header + elements.
	cost += uint64(cap(e.rules)) * uint64(unsafe.Sizeof(CompiledRule{}))
	return cost
}

// MatchCache is a bounded LRU cache for the result of CompiledStyleSheet.MatchRules.
// It is safe for concurrent use.
//
// Invalidation contract: the cache does NOT detect stylesheet changes or
// structural pseudo-class changes. Callers must call Clear() when the
// stylesheet is replaced or when state-sensitive selectors could produce
// different results.
type MatchCache struct {
	mu           sync.Mutex
	max          int
	items        map[ElementKey]*matchEntry
	head         *matchEntry // most recently used
	tail         *matchEntry // least recently used
	currentBytes uint64

	hits      int64
	misses    int64
	evictions int64
}

// NewMatchCache creates a bounded MatchCache with the given configuration.
func NewMatchCache(cfg MatchCacheConfig) *MatchCache {
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 512
	}
	return &MatchCache{
		max:   cfg.MaxEntries,
		items: make(map[ElementKey]*matchEntry, cfg.MaxEntries/2+1),
	}
}

// Get retrieves the cached MatchRules result for the given key.
// Returns (nil, false) on miss. The returned slice must not be mutated.
func (c *MatchCache) Get(key ElementKey) ([]CompiledRule, bool) {
	c.mu.Lock()
	e, ok := c.items[key]
	if ok {
		c.moveToFront(e)
		c.hits++
		rules := e.rules
		c.mu.Unlock()
		return rules, true
	}
	c.misses++
	c.mu.Unlock()
	return nil, false
}

// Put stores the MatchRules result for the given key.
// If an entry already exists for the key, it is updated.
// LRU eviction is triggered when the cache is full.
// The rules slice is stored by reference; callers must not modify it after
// calling Put.
func (c *MatchCache) Put(key ElementKey, rules []CompiledRule) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry.
	if e, ok := c.items[key]; ok {
		c.currentBytes -= e.byteSize()
		e.rules = rules
		c.currentBytes += e.byteSize()
		c.moveToFront(e)
		return
	}

	// Evict until under capacity.
	for len(c.items) >= c.max {
		c.evictLRU()
	}

	e := &matchEntry{key: key, rules: rules}
	c.items[key] = e
	c.currentBytes += e.byteSize()
	c.pushFront(e)
}

// Len returns the number of cached entries.
func (c *MatchCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// ByteSize returns the approximate total byte cost of all cached entries.
func (c *MatchCache) ByteSize() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentBytes
}

// Metrics returns a snapshot of cache hit/miss/eviction counts.
func (c *MatchCache) Metrics() MatchCacheMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()
	return MatchCacheMetrics{
		Hits:      c.hits,
		Misses:    c.misses,
		Evictions: c.evictions,
	}
}

// Clear removes all cached entries and resets metrics.
func (c *MatchCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[ElementKey]*matchEntry, c.max/2+1)
	c.head = nil
	c.tail = nil
	c.currentBytes = 0
	c.hits = 0
	c.misses = 0
	c.evictions = 0
}

// Evict removes LRU entries until at least targetBytes have been freed or the
// cache is empty. Returns the number of bytes actually freed.
//
// This satisfies the memory.Evictor interface:
//
//	func(targetBytes uint64) uint64
func (c *MatchCache) Evict(targetBytes uint64) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	var freed uint64
	for freed < targetBytes && c.tail != nil {
		freed += c.tail.byteSize()
		c.evictLRU()
	}
	return freed
}

// evictLRU removes the least recently used entry. Must be called with mu held.
func (c *MatchCache) evictLRU() {
	if c.tail == nil {
		return
	}
	e := c.tail
	c.currentBytes -= e.byteSize()
	delete(c.items, e.key)
	c.removeEntry(e)
	c.evictions++
}

func (c *MatchCache) pushFront(e *matchEntry) {
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

func (c *MatchCache) moveToFront(e *matchEntry) {
	if c.head == e {
		return
	}
	c.removeEntry(e)
	c.pushFront(e)
}

func (c *MatchCache) removeEntry(e *matchEntry) {
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
