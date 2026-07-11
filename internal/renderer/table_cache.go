package renderer

import (
	"sync"
)

// TableColumnCache is a bounded, thread-safe cache for table column measurements.
// It maps a table's RenderNode ID and available width to its computed column widths.
// To prevent memory leaks, it maintains a maximum capacity using a FIFO eviction strategy.
type TableColumnCache struct {
	mu       sync.RWMutex
	cache    map[int64]map[float32][]float32
	keys     []int64 // FIFO tracking for eviction
	capacity int
}

// NewTableColumnCache creates a new TableColumnCache with the given capacity.
func NewTableColumnCache(capacity int) *TableColumnCache {
	if capacity <= 0 {
		capacity = 100 // Default capacity
	}
	return &TableColumnCache{
		cache:    make(map[int64]map[float32][]float32),
		capacity: capacity,
	}
}

// Get retrieves cached column widths for a table ID and available width.
func (c *TableColumnCache) Get(tableID int64, availableWidth float32) ([]float32, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	widthsMap, ok := c.cache[tableID]
	if !ok {
		return nil, false
	}
	widths, ok := widthsMap[availableWidth]
	return widths, ok
}

// Set stores computed column widths for a table ID and available width.
func (c *TableColumnCache) Set(tableID int64, availableWidth float32, widths []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	widthsMap, ok := c.cache[tableID]
	if !ok {
		// Evict oldest entry if capacity is reached
		if len(c.cache) >= c.capacity && len(c.keys) > 0 {
			oldest := c.keys[0]
			delete(c.cache, oldest)
			c.keys = c.keys[1:]
		}
		widthsMap = make(map[float32][]float32)
		c.cache[tableID] = widthsMap
		c.keys = append(c.keys, tableID)
	}

	// Copy widths slice to prevent mutations of cached data
	widthsCopy := make([]float32, len(widths))
	copy(widthsCopy, widths)
	widthsMap[availableWidth] = widthsCopy
}

// Invalidate clears the cached entries for a specific table ID.
func (c *TableColumnCache) Invalidate(tableID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.cache[tableID]; ok {
		delete(c.cache, tableID)
		for i, k := range c.keys {
			if k == tableID {
				c.keys = append(c.keys[:i], c.keys[i+1:]...)
				break
			}
		}
	}
}

// Clear resets the entire cache.
func (c *TableColumnCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[int64]map[float32][]float32)
	c.keys = nil
}

// globalTableColumnCache is the package-level cache for table column widths.
var globalTableColumnCache = NewTableColumnCache(100)
