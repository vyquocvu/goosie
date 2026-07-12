// Package net provides HTTP caching for the Goosie browser engine.
//
// M9.2: Bound every cache — HTTP response cache.
//
// HTTPCache is a write-through disk cache for HTTP GET responses with an
// optional in-memory LRU index that enforces entry-count and byte-size limits.
//
// When MaxEntries > 0 or MaxBytes > 0, the cache evicts the least-recently-used
// entry before accepting a new one that would exceed the configured limit.
// Eviction removes both the in-memory LRU entry and the corresponding disk
// files, preventing unbounded disk growth across repeated navigations.
//
// When limits are zero (the default), the cache behaves like the original
// disk-only implementation for backward compatibility.
package net

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// HTTPCacheConfig
// ---------------------------------------------------------------------------

// HTTPCacheConfig configures the HTTP response cache.
type HTTPCacheConfig struct {
	// Root is the filesystem directory where response bodies and metadata are
	// stored. Must be non-empty for a functional cache.
	Root string

	// Private, when true, disables all caching (private browsing mode).
	Private bool

	// MaxEntries is the maximum number of entries in the LRU index.
	// Zero means no limit (backward compatible).
	MaxEntries int

	// MaxBytes is the maximum total byte cost of cached response bodies stored
	// on disk. Zero means no limit.
	MaxBytes int64
}

// ---------------------------------------------------------------------------
// HTTPCacheMetrics
// ---------------------------------------------------------------------------

// HTTPCacheMetrics is a point-in-time snapshot of cache activity.
type HTTPCacheMetrics struct {
	Hits      int64
	Misses    int64
	Evictions int64
}

// ---------------------------------------------------------------------------
// internal LRU entry
// ---------------------------------------------------------------------------

// httpLRUEntry is one node in the doubly-linked LRU list maintained by the
// in-memory index.
type httpLRUEntry struct {
	key      string // normalised URL used as the cache key
	byteSize int64  // approximate on-disk body size in bytes
	prev     *httpLRUEntry
	next     *httpLRUEntry
}

// ---------------------------------------------------------------------------
// HTTPCache
// ---------------------------------------------------------------------------

// HTTPCache is a write-through disk cache for HTTP GET responses.
// It is safe for concurrent use.
type HTTPCache struct {
	root    string
	private bool

	// mu guards the LRU index and metrics. Disk I/O is done outside the lock
	// to avoid holding it during slow syscalls.
	mu sync.Mutex

	// LRU index — populated only when MaxEntries > 0 or MaxBytes > 0.
	cfg          HTTPCacheConfig
	lruItems     map[string]*httpLRUEntry // key → LRU node
	lruHead      *httpLRUEntry            // most recently used
	lruTail      *httpLRUEntry            // least recently used
	currentBytes int64

	// metrics
	hits      int64
	misses    int64
	evictions int64
}

// NewHTTPCache creates an HTTPCache with the original two-parameter signature.
// No entry-count or byte-size limit is applied (backward compatible).
func NewHTTPCache(root string, private bool) *HTTPCache {
	return NewHTTPCacheWithConfig(HTTPCacheConfig{
		Root:    root,
		Private: private,
	})
}

// NewHTTPCacheWithConfig creates an HTTPCache with full configuration.
func NewHTTPCacheWithConfig(cfg HTTPCacheConfig) *HTTPCache {
	c := &HTTPCache{
		root:    cfg.Root,
		private: cfg.Private,
		cfg:     cfg,
	}
	if cfg.MaxEntries > 0 || cfg.MaxBytes > 0 {
		c.lruItems = make(map[string]*httpLRUEntry)
	}
	return c
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// CacheEntry describes a stored HTTP response entry.
type CacheEntry struct {
	URL         string    `json:"url"`
	Status      int       `json:"status"`
	ContentType string    `json:"content_type"`
	StoredAt    time.Time `json:"stored_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	BodyFile    string    `json:"body_file"`
}

// Get retrieves a cached response body for rawURL. Returns the body, the
// cache metadata, and true on a cache hit. Returns false if the cache is
// disabled, the entry is missing, or the entry has expired.
//
// When the LRU index is active (MaxEntries > 0 or MaxBytes > 0), the index is
// the authoritative membership list. A URL not present in the LRU index is
// treated as a miss even if stale disk files happen to exist.
func (c *HTTPCache) Get(rawURL string) (string, CacheEntry, bool) {
	if c == nil || c.private {
		c.recordMiss()
		return "", CacheEntry{}, false
	}

	// When limits are active, gate on LRU membership before touching disk.
	if c.lruItems != nil {
		key := normalizeCacheURL(rawURL)
		c.mu.Lock()
		e, inLRU := c.lruItems[key]
		if !inLRU {
			c.misses++
			c.mu.Unlock()
			return "", CacheEntry{}, false
		}
		c.moveToFrontLocked(e)
		c.mu.Unlock()
	}

	entryPath, _ := c.paths(rawURL)
	data, err := os.ReadFile(entryPath)
	if err != nil {
		c.recordMiss()
		return "", CacheEntry{}, false
	}
	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		c.recordMiss()
		return "", CacheEntry{}, false
	}
	if entry.ExpiresAt.IsZero() || !time.Now().Before(entry.ExpiresAt) {
		c.recordMiss()
		return "", CacheEntry{}, false
	}

	bodyPath := filepath.Join(c.root, entry.BodyFile)
	body, err := os.ReadFile(bodyPath)
	if err != nil {
		c.recordMiss()
		return "", CacheEntry{}, false
	}

	c.mu.Lock()
	c.hits++
	c.mu.Unlock()

	return string(body), entry, true
}

// Put stores a cacheable response for rawURL. The response is only stored when
// the Cache-Control header contains a valid positive max-age and none of the
// veto directives (no-store, private, no-cache) are present. Vary and
// Set-Cookie responses are also excluded.
func (c *HTTPCache) Put(rawURL string, resp *http.Response, body string) {
	if c == nil || c.private || resp == nil {
		return
	}
	if resp.StatusCode != http.StatusOK {
		return
	}
	if hasNonEmptyHeader(resp.Header, "Vary") {
		return
	}
	if len(resp.Header.Values("Set-Cookie")) > 0 {
		return
	}
	maxAge, ok := cacheMaxAge(strings.Join(resp.Header.Values("Cache-Control"), ","))
	if !ok {
		return
	}

	bodyBytes := int64(len(body))

	// Enforce byte limit: if a single entry already exceeds MaxBytes, skip it.
	if c.cfg.MaxBytes > 0 && bodyBytes > c.cfg.MaxBytes {
		return
	}

	key := normalizeCacheURL(rawURL)
	storedAt := time.Now()
	cacheKey := c.keyFor(rawURL)
	bodyFile := cacheKey + ".body"

	entry := CacheEntry{
		URL:         key,
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		StoredAt:    storedAt,
		ExpiresAt:   storedAt.Add(time.Duration(maxAge) * time.Second),
		BodyFile:    bodyFile,
	}

	// Write to disk (outside any lock).
	if err := os.MkdirAll(c.root, 0o755); err != nil {
		return
	}
	_, bodyPath := c.paths(rawURL)
	if err := os.WriteFile(bodyPath, []byte(body), 0o644); err != nil {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	entryPath, _ := c.paths(rawURL)
	if err := os.WriteFile(entryPath, data, 0o644); err != nil {
		return
	}

	// Update LRU index under lock.
	if c.lruItems != nil {
		c.mu.Lock()
		c.upsertLRULocked(key, bodyBytes)
		c.mu.Unlock()
	}
}

// Metrics returns a point-in-time snapshot of cache activity counters.
func (c *HTTPCache) Metrics() HTTPCacheMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()
	return HTTPCacheMetrics{
		Hits:      c.hits,
		Misses:    c.misses,
		Evictions: c.evictions,
	}
}

// Clear removes all in-memory LRU state, deletes the corresponding disk files
// for every tracked entry, and resets metrics. After Clear, the cache is
// logically empty. Entries whose disk files cannot be removed (permission
// errors, already deleted) are ignored.
func (c *HTTPCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lruItems != nil {
		// Remove disk files for every tracked entry.
		for key := range c.lruItems {
			cacheKey := c.hashKey(key)
			_ = os.Remove(filepath.Join(c.root, cacheKey+".json"))
			_ = os.Remove(filepath.Join(c.root, cacheKey+".body"))
		}
		c.lruItems = make(map[string]*httpLRUEntry)
	}
	c.lruHead = nil
	c.lruTail = nil
	c.currentBytes = 0
	c.hits = 0
	c.misses = 0
	c.evictions = 0
}

// ---------------------------------------------------------------------------
// LRU implementation (all methods called with c.mu held)
// ---------------------------------------------------------------------------

// upsertLRULocked inserts or updates an entry in the LRU index and enforces
// limits. Caller must hold c.mu.
func (c *HTTPCache) upsertLRULocked(key string, byteSize int64) {
	// If updating an existing entry, remove it first to recompute bytes.
	if existing, ok := c.lruItems[key]; ok {
		c.currentBytes -= existing.byteSize
		c.removeLRUEntryLocked(existing)
		delete(c.lruItems, key)
	}

	// Enforce MaxEntries.
	if c.cfg.MaxEntries > 0 {
		for len(c.lruItems) >= c.cfg.MaxEntries && c.lruTail != nil {
			c.evictLRUTailLocked()
		}
	}

	// Enforce MaxBytes.
	if c.cfg.MaxBytes > 0 {
		for c.currentBytes+byteSize > c.cfg.MaxBytes && c.lruTail != nil {
			c.evictLRUTailLocked()
		}
	}

	// Insert at head.
	e := &httpLRUEntry{key: key, byteSize: byteSize}
	c.lruItems[key] = e
	c.currentBytes += byteSize
	c.pushFrontLocked(e)
}

// evictLRUTailLocked evicts the LRU (tail) entry and removes its disk files.
// Caller must hold c.mu. Disk I/O happens while the lock is held intentionally
// to keep the LRU state consistent; HTTP cache entries are rare and large, so
// the latency cost is acceptable.
func (c *HTTPCache) evictLRUTailLocked() {
	tail := c.lruTail
	if tail == nil {
		return
	}
	c.currentBytes -= tail.byteSize
	delete(c.lruItems, tail.key)
	c.removeLRUEntryLocked(tail)
	c.evictions++

	// Remove the disk files for the evicted entry. Errors are ignored — a
	// stale disk entry that cannot be read is a cache miss, which is safe.
	cacheKey := c.hashKey(tail.key)
	_ = os.Remove(filepath.Join(c.root, cacheKey+".json"))
	_ = os.Remove(filepath.Join(c.root, cacheKey+".body"))
}

func (c *HTTPCache) moveToFrontLocked(e *httpLRUEntry) {
	if c.lruHead == e {
		return
	}
	c.removeLRUEntryLocked(e)
	c.pushFrontLocked(e)
}

func (c *HTTPCache) pushFrontLocked(e *httpLRUEntry) {
	e.prev = nil
	e.next = c.lruHead
	if c.lruHead != nil {
		c.lruHead.prev = e
	}
	c.lruHead = e
	if c.lruTail == nil {
		c.lruTail = e
	}
}

func (c *HTTPCache) removeLRUEntryLocked(e *httpLRUEntry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		c.lruHead = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		c.lruTail = e.prev
	}
	e.prev = nil
	e.next = nil
}

// ---------------------------------------------------------------------------
// Metrics helpers
// ---------------------------------------------------------------------------

func (c *HTTPCache) recordMiss() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.misses++
	c.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Path and key helpers
// ---------------------------------------------------------------------------

func (c *HTTPCache) paths(rawURL string) (string, string) {
	key := c.keyFor(rawURL)
	return filepath.Join(c.root, key+".json"), filepath.Join(c.root, key+".body")
}

// keyFor returns the hex-encoded SHA-256 hash of the normalised URL.
func (c *HTTPCache) keyFor(rawURL string) string {
	return c.hashKey(normalizeCacheURL(rawURL))
}

// hashKey returns the hex SHA-256 of s.
func (c *HTTPCache) hashKey(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// Package-level helpers (unchanged from original)
// ---------------------------------------------------------------------------

func hasNonEmptyHeader(header http.Header, name string) bool {
	for _, value := range header.Values(name) {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func normalizeCacheURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.String()
}

func cacheMaxAge(header string) (int, bool) {
	maxAge := 0
	hasMaxAge := false
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(strings.ToLower(part))
		name, value, hasValue := strings.Cut(part, "=")
		name = strings.TrimSpace(name)
		if name == "private" || name == "no-store" || name == "no-cache" {
			return 0, false
		}
		if name == "max-age" {
			if !hasValue {
				return 0, false
			}
			seconds, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || seconds < 0 {
				return 0, false
			}
			maxAge = seconds
			hasMaxAge = true
		}
	}
	return maxAge, hasMaxAge
}
