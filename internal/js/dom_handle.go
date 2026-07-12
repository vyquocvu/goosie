package js

import (
	"errors"
	"sync"

	"github.com/vyquocvu/goosie/internal/dom"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	// ErrNodeRemoved is returned when accessing a handle whose node has been
	// removed from the DOM store.
	ErrNodeRemoved = errors.New("js: node removed from DOM")

	// ErrInvalidHandle is returned when using a zero/uninitialized handle.
	ErrInvalidHandle = errors.New("js: invalid node handle")
)

// ---------------------------------------------------------------------------
// NodeHandle — lazy wrapper around dom.NodeID
// ---------------------------------------------------------------------------

// NodeHandle is a lazy JavaScript-facing wrapper around a dom.NodeID.
// It does NOT copy node data — all access resolves through the DOM store
// on demand. Handles are cached weakly with bounded lifetime.
type NodeHandle struct {
	id    dom.NodeID
	store *dom.Store
}

// NewNodeHandle creates a handle for the given node ID.
// The handle is lazy — no data is copied.
func NewNodeHandle(id dom.NodeID, store *dom.Store) NodeHandle {
	return NodeHandle{id: id, store: store}
}

// NodeID returns the underlying stable DOM handle.
func (h NodeHandle) NodeID() dom.NodeID {
	return h.id
}

// IsValid reports whether the handle refers to a live node.
func (h NodeHandle) IsValid() bool {
	if h.id == dom.NodeNone || h.store == nil {
		return false
	}
	return h.store.IsValid(h.id)
}

// Kind returns the node kind, or an error if the node is stale.
func (h NodeHandle) Kind() (dom.NodeKind, error) {
	if err := h.check(); err != nil {
		return 0, err
	}
	return h.store.Kind(h.id), nil
}

// Parent returns the parent handle, or an invalid handle if no parent.
func (h NodeHandle) Parent() (NodeHandle, error) {
	if err := h.check(); err != nil {
		return NodeHandle{}, err
	}
	pid := h.store.Parent(h.id)
	return NodeHandle{id: pid, store: h.store}, nil
}

// FirstChild returns the first child handle, or invalid if leaf.
func (h NodeHandle) FirstChild() (NodeHandle, error) {
	if err := h.check(); err != nil {
		return NodeHandle{}, err
	}
	cid := h.store.FirstChild(h.id)
	return NodeHandle{id: cid, store: h.store}, nil
}

// NextSibling returns the next sibling handle, or invalid if last.
func (h NodeHandle) NextSibling() (NodeHandle, error) {
	if err := h.check(); err != nil {
		return NodeHandle{}, err
	}
	sid := h.store.NextSibling(h.id)
	return NodeHandle{id: sid, store: h.store}, nil
}

// check validates the handle is live. Returns ErrInvalidHandle or
// ErrNodeRemoved as appropriate.
func (h NodeHandle) check() error {
	if h.id == dom.NodeNone || h.store == nil {
		return ErrInvalidHandle
	}
	if !h.store.IsValid(h.id) {
		return ErrNodeRemoved
	}
	return nil
}

// ---------------------------------------------------------------------------
// HandleCache — weak bounded cache of NodeHandle wrappers
// ---------------------------------------------------------------------------

// HandleCacheConfig configures the handle cache.
type HandleCacheConfig struct {
	// MaxEntries is the maximum number of cached handles.
	MaxEntries int
}

// DefaultHandleCacheConfig returns sensible defaults.
func DefaultHandleCacheConfig() HandleCacheConfig {
	return HandleCacheConfig{
		MaxEntries: 1024,
	}
}

// HandleCache is a bounded cache of NodeHandle wrappers keyed by NodeID.
// It avoids creating new Go objects for the same DOM node on repeated
// JavaScript access. Entries are evicted LRU when the cache is full.
type HandleCache struct {
	mu      sync.Mutex
	store   *dom.Store
	entries map[dom.NodeID]*cacheEntry
	order   []dom.NodeID // LRU order (oldest first)
	max     int

	// Metrics.
	hits   uint64
	misses uint64
}

type cacheEntry struct {
	handle NodeHandle
}

// NewHandleCache creates a bounded handle cache.
func NewHandleCache(store *dom.Store, cfg HandleCacheConfig) *HandleCache {
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 1024
	}
	return &HandleCache{
		store:   store,
		entries: make(map[dom.NodeID]*cacheEntry, cfg.MaxEntries/2),
		order:   make([]dom.NodeID, 0, cfg.MaxEntries),
		max:     cfg.MaxEntries,
	}
}

// Get returns a cached handle for the given NodeID, creating one if needed.
// Returns an invalid handle if the node is not in the store.
func (c *HandleCache) Get(id dom.NodeID) NodeHandle {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.entries[id]; ok {
		c.hits++
		// Move to end (most recent).
		c.touchLocked(id)
		return e.handle
	}

	c.misses++

	// Evict if full.
	if len(c.entries) >= c.max {
		c.evictLRU()
	}

	h := NodeHandle{id: id, store: c.store}
	c.entries[id] = &cacheEntry{handle: h}
	c.order = append(c.order, id)
	return h
}

// Invalidate removes a handle from the cache (e.g. when a node is removed).
func (c *HandleCache) Invalidate(id dom.NodeID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, id)
	// Lazy removal from order slice — cleaned up on next eviction.
}

// Len returns the number of cached handles.
func (c *HandleCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Metrics returns cache hit/miss counts.
func (c *HandleCache) Metrics() (hits, misses uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses
}

// Clear removes all cached handles.
func (c *HandleCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[dom.NodeID]*cacheEntry, c.max/2)
	c.order = c.order[:0]
	c.hits = 0
	c.misses = 0
}

// touchLocked moves id to the end of the order slice (most recent).
func (c *HandleCache) touchLocked(id dom.NodeID) {
	for i, oid := range c.order {
		if oid == id {
			copy(c.order[i:], c.order[i+1:])
			c.order[len(c.order)-1] = id
			return
		}
	}
}

// evictLRU removes the oldest entry. Must be called with mu held.
func (c *HandleCache) evictLRU() {
	for len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		if _, ok := c.entries[oldest]; ok {
			delete(c.entries, oldest)
			return
		}
		// Already deleted — skip (lazy cleanup).
	}
}
