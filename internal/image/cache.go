package image

import (
	"container/list"
	"sync"
)

const (
	// DefaultCacheMaxBytes is the default byte limit for the image cache (32 MB).
	DefaultCacheMaxBytes int64 = 32 * 1024 * 1024
	// BudgetCacheMaxBytes is the upper budget threshold for the image cache (64 MB).
	BudgetCacheMaxBytes int64 = 64 * 1024 * 1024
)

// Cache implements a byte-bounded LRU (Least Recently Used) cache for images.
type Cache struct {
	mu            sync.RWMutex
	capacity      int
	maxBytes      int64
	currentBytes  int64
	items         map[string]*list.Element
	lruList       *list.List
	onUsageChange func(currentBytes uint64)
}

// cacheEntry represents an entry in the cache.
type cacheEntry struct {
	key      string
	value    *ImageData
	byteSize int64
}

// EstimateImageBytes returns the estimated byte footprint of an ImageData.
// For bitmap/raster images, this is approximately Width * Height * 4 (RGBA).
func EstimateImageBytes(data *ImageData) int64 {
	if data == nil {
		return 0
	}
	w, h := data.Width, data.Height
	if data.Image != nil {
		bounds := data.Image.Bounds()
		if bounds.Dx() > 0 && bounds.Dy() > 0 {
			w, h = bounds.Dx(), bounds.Dy()
		}
	}
	if w <= 0 || h <= 0 {
		return 1024 // Minimal placeholder footprint for metadata/errors
	}
	return int64(w) * int64(h) * 4
}

// NewCache creates a new LRU cache with the specified capacity and default byte limit (32MB).
func NewCache(capacity int) *Cache {
	if capacity <= 0 {
		capacity = 100 // Default capacity
	}
	return &Cache{
		capacity: capacity,
		maxBytes: DefaultCacheMaxBytes,
		items:    make(map[string]*list.Element),
		lruList:  list.New(),
	}
}

// NewCacheWithByteLimit creates an image cache with an explicit byte limit and capacity.
func NewCacheWithByteLimit(maxBytes int64, capacity int) *Cache {
	if maxBytes <= 0 {
		maxBytes = DefaultCacheMaxBytes
	}
	if capacity <= 0 {
		capacity = 100
	}
	return &Cache{
		capacity: capacity,
		maxBytes: maxBytes,
		items:    make(map[string]*list.Element),
		lruList:  list.New(),
	}
}

// Get retrieves an image from the cache.
func (c *Cache) Get(key string) *ImageData {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		// Move to front (most recently used)
		c.lruList.MoveToFront(elem)
		return elem.Value.(*cacheEntry).value
	}
	return nil
}

// Put adds or updates an image in the cache.
func (c *Cache) Put(key string, value *ImageData) {
	byteSize := EstimateImageBytes(value)

	var cb func(uint64)
	var newBytes uint64

	c.mu.Lock()
	// If a single item strictly exceeds the maximum byte limit, do not retain it
	if c.maxBytes > 0 && byteSize > c.maxBytes {
		c.mu.Unlock()
		return
	}

	// Check if key already exists
	if elem, ok := c.items[key]; ok {
		entry := elem.Value.(*cacheEntry)
		c.currentBytes -= entry.byteSize
		entry.value = value
		entry.byteSize = byteSize
		c.currentBytes += byteSize
		c.lruList.MoveToFront(elem)
	} else {
		// Add new entry
		entry := &cacheEntry{key: key, value: value, byteSize: byteSize}
		elem := c.lruList.PushFront(entry)
		c.items[key] = elem
		c.currentBytes += byteSize
	}

	// Evict least recently used if over capacity or byte budget
	for c.lruList.Len() > 0 && ((c.capacity > 0 && c.lruList.Len() > c.capacity) || (c.maxBytes > 0 && c.currentBytes > c.maxBytes)) {
		c.evict()
	}

	cb = c.onUsageChange
	newBytes = uint64(c.currentBytes)
	c.mu.Unlock()

	if cb != nil {
		cb(newBytes)
	}
}

// evict removes the least recently used item from the cache. Must be called with c.mu locked.
func (c *Cache) evict() *cacheEntry {
	elem := c.lruList.Back()
	if elem != nil {
		c.lruList.Remove(elem)
		entry := elem.Value.(*cacheEntry)
		delete(c.items, entry.key)
		c.currentBytes -= entry.byteSize
		if c.currentBytes < 0 {
			c.currentBytes = 0
		}
		return entry
	}
	return nil
}

// Evict removes LRU entries until at least targetBytes have been freed or the cache is empty.
// It returns the total number of bytes actually freed. Satisfies the memory.Evictor signature.
func (c *Cache) Evict(targetBytes uint64) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	var freed uint64
	for freed < targetBytes && c.lruList.Len() > 0 {
		entry := c.evict()
		if entry != nil {
			freed += uint64(entry.byteSize)
		}
	}
	return freed
}

// Clear removes all items from the cache and resets byte usage to zero.
func (c *Cache) Clear() {
	var cb func(uint64)
	c.mu.Lock()
	c.items = make(map[string]*list.Element)
	c.lruList = list.New()
	c.currentBytes = 0
	cb = c.onUsageChange
	c.mu.Unlock()

	if cb != nil {
		cb(0)
	}
}

// Len returns the current number of items in the cache.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lruList.Len()
}

// Bytes returns the current total estimated memory usage of cached images.
func (c *Cache) Bytes() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentBytes
}

// MaxBytes returns the configured maximum byte limit for the cache.
func (c *Cache) MaxBytes() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.maxBytes
}

// SetMaxBytes updates the maximum byte limit of the cache and evicts items if necessary.
func (c *Cache) SetMaxBytes(maxBytes int64) {
	if maxBytes <= 0 {
		maxBytes = DefaultCacheMaxBytes
	}

	var cb func(uint64)
	var newBytes uint64

	c.mu.Lock()
	c.maxBytes = maxBytes
	for c.lruList.Len() > 0 && c.currentBytes > c.maxBytes {
		c.evict()
	}
	cb = c.onUsageChange
	newBytes = uint64(c.currentBytes)
	c.mu.Unlock()

	if cb != nil {
		cb(newBytes)
	}
}

// SetCapacity updates the cache item capacity.
func (c *Cache) SetCapacity(capacity int) {
	if capacity <= 0 {
		capacity = 100
	}

	var cb func(uint64)
	var newBytes uint64

	c.mu.Lock()
	c.capacity = capacity
	for c.lruList.Len() > 0 && c.capacity > 0 && c.lruList.Len() > c.capacity {
		c.evict()
	}
	cb = c.onUsageChange
	newBytes = uint64(c.currentBytes)
	c.mu.Unlock()

	if cb != nil {
		cb(newBytes)
	}
}

// SetUsageCallback sets a callback invoked whenever the cache's byte usage changes.
// The callback is invoked immediately with the current byte usage.
func (c *Cache) SetUsageCallback(cb func(currentBytes uint64)) {
	c.mu.Lock()
	c.onUsageChange = cb
	bytes := uint64(c.currentBytes)
	c.mu.Unlock()

	if cb != nil {
		cb(bytes)
	}
}

