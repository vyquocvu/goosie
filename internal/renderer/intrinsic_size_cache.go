package renderer

import (
	"sync"
	"unsafe"
)

// IntrinsicSizeCache is a bounded LRU cache for intrinsic layout sizes.
// It avoids unbound memory growth during long sessions or on dynamic pages.
type IntrinsicSizeCache struct {
	mu           sync.Mutex
	maxBytes     int64
	currentBytes int64
	items        map[LayoutID]*intrinsicSizeEntry
	head         *intrinsicSizeEntry // most recently used
	tail         *intrinsicSizeEntry // least recently used
}

type intrinsicSizeEntry struct {
	id     LayoutID
	width  float32
	height float32
	prev   *intrinsicSizeEntry
	next   *intrinsicSizeEntry
}

// ByteSize returns the memory size of an intrinsicSizeEntry.
func (e *intrinsicSizeEntry) ByteSize() int64 {
	return int64(unsafe.Sizeof(*e))
}

// NewIntrinsicSizeCache creates an intrinsic size cache with a specific byte limit.
func NewIntrinsicSizeCache(maxBytes int64) *IntrinsicSizeCache {
	if maxBytes <= 0 {
		maxBytes = 2 << 20 // 2MB default
	}
	return &IntrinsicSizeCache{
		maxBytes: maxBytes,
		items:    make(map[LayoutID]*intrinsicSizeEntry),
	}
}

// Get retrieves the intrinsic size for the given LayoutID. Returns (0, 0, false) if not found.
func (c *IntrinsicSizeCache) Get(id LayoutID) (width, height float32, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, exists := c.items[id]; exists {
		c.moveToFront(e)
		return e.width, e.height, true
	}
	return 0, 0, false
}

// Put caches the intrinsic size for the given LayoutID.
func (c *IntrinsicSizeCache) Put(id LayoutID, width, height float32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, exists := c.items[id]; exists {
		e.width = width
		e.height = height
		c.moveToFront(e)
		return
	}

	e := &intrinsicSizeEntry{
		id:     id,
		width:  width,
		height: height,
	}

	entrySize := e.ByteSize()

	for c.currentBytes+entrySize > c.maxBytes && c.tail != nil {
		c.evictLRU()
	}

	// Should not happen for small entries, but just in case
	if entrySize > c.maxBytes {
		return
	}

	c.items[id] = e
	c.currentBytes += entrySize
	c.pushFront(e)
}

// Invalidate removes the entry for the given LayoutID.
func (c *IntrinsicSizeCache) Invalidate(id LayoutID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, exists := c.items[id]; exists {
		c.currentBytes -= e.ByteSize()
		c.removeEntry(e)
		delete(c.items, id)
	}
}

// Clear removes all entries.
func (c *IntrinsicSizeCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[LayoutID]*intrinsicSizeEntry)
	c.head = nil
	c.tail = nil
	c.currentBytes = 0
}

// Bytes returns the current bytes used.
func (c *IntrinsicSizeCache) Bytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentBytes
}

// Evict evicts up to targetBytes bytes and returns the actual bytes freed.
func (c *IntrinsicSizeCache) Evict(targetBytes uint64) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	var freed uint64
	for freed < targetBytes && c.tail != nil {
		size := c.tail.ByteSize()
		freed += uint64(size)
		c.evictLRU()
	}
	return freed
}

// evictLRU evicts the least recently used entry. Must be called with lock held.
func (c *IntrinsicSizeCache) evictLRU() {
	if c.tail == nil {
		return
	}
	c.currentBytes -= c.tail.ByteSize()
	delete(c.items, c.tail.id)
	c.removeEntry(c.tail)
}

func (c *IntrinsicSizeCache) moveToFront(e *intrinsicSizeEntry) {
	if c.head == e {
		return
	}
	c.removeEntry(e)
	c.pushFront(e)
}

func (c *IntrinsicSizeCache) pushFront(e *intrinsicSizeEntry) {
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

func (c *IntrinsicSizeCache) removeEntry(e *intrinsicSizeEntry) {
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
