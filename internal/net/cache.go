// Package net provides HTTP caching for the Goosie browser engine.
//
// M9.2: Bound every cache — HTTP response cache.
// M9.3.3: Batch history and cache metadata writes.
//
// HTTPCache is a write-through disk cache for HTTP GET responses with an
// optional in-memory LRU index that enforces entry-count and byte-size limits.
//
// All cache metadata is stored in a single index.json file, which is loaded
// into memory at startup. Writes to index.json are batched and coalesced
// in the background using a timer to minimize disk I/O and avoid blocking
// UI and fetch threads.
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

type HTTPCacheConfig struct {
	Root       string
	Private    bool
	MaxEntries int
	MaxBytes   int64
}

// ---------------------------------------------------------------------------
// HTTPCacheMetrics
// ---------------------------------------------------------------------------

type HTTPCacheMetrics struct {
	Hits      int64
	Misses    int64
	Evictions int64
}

// ---------------------------------------------------------------------------
// httpLRUEntry
// ---------------------------------------------------------------------------

type httpLRUEntry struct {
	key      string
	byteSize int64
	prev     *httpLRUEntry
	next     *httpLRUEntry
}

// ---------------------------------------------------------------------------
// HTTPCache
// ---------------------------------------------------------------------------

type HTTPCache struct {
	root    string
	private bool

	mu sync.Mutex

	// LRU index
	cfg          HTTPCacheConfig
	lruItems     map[string]*httpLRUEntry
	lruHead      *httpLRUEntry
	lruTail      *httpLRUEntry
	currentBytes int64

	// In-memory index of cache entries
	entries map[string]CacheEntry
	dirty   bool

	// Timer for debounced background writing
	writeTimer *time.Timer

	// metrics
	hits      int64
	misses    int64
	evictions int64
}

func NewHTTPCache(root string, private bool) *HTTPCache {
	return NewHTTPCacheWithConfig(HTTPCacheConfig{
		Root:    root,
		Private: private,
	})
}

func NewHTTPCacheWithConfig(cfg HTTPCacheConfig) *HTTPCache {
	c := &HTTPCache{
		root:    cfg.Root,
		private: cfg.Private,
		cfg:     cfg,
		entries: make(map[string]CacheEntry),
	}
	if cfg.MaxEntries > 0 || cfg.MaxBytes > 0 {
		c.lruItems = make(map[string]*httpLRUEntry)
	}

	c.loadIndex()
	return c
}

type CacheEntry struct {
	URL         string    `json:"url"`
	Status      int       `json:"status"`
	ContentType string    `json:"content_type"`
	StoredAt    time.Time `json:"stored_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	BodyFile    string    `json:"body_file"`
}

func (c *HTTPCache) Close() error {
	return c.Sync()
}

func (c *HTTPCache) Sync() error {
	if c.private || c.root == "" {
		return nil
	}

	c.mu.Lock()
	if c.writeTimer != nil {
		c.writeTimer.Stop()
		c.writeTimer = nil
	}
	if c.dirty {
		c.dirty = false
		c.saveIndexLocked()
	}
	c.mu.Unlock()
	return nil
}

func (c *HTTPCache) Get(rawURL string) ([]byte, CacheEntry, bool) {
	if c == nil || c.private {
		c.recordMiss()
		return nil, CacheEntry{}, false
	}

	key := normalizeCacheURL(rawURL)

	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok {
		c.misses++
		c.mu.Unlock()
		return nil, CacheEntry{}, false
	}

	if c.lruItems != nil {
		e, inLRU := c.lruItems[key]
		if !inLRU {
			c.misses++
			c.mu.Unlock()
			return nil, CacheEntry{}, false
		}
		c.moveToFrontLocked(e)
	}
	c.mu.Unlock()

	if entry.ExpiresAt.IsZero() || !time.Now().Before(entry.ExpiresAt) {
		c.recordMiss()
		return nil, CacheEntry{}, false
	}

	bodyPath := filepath.Join(c.root, entry.BodyFile)
	body, err := os.ReadFile(bodyPath)
	if err != nil {
		c.recordMiss()
		return nil, CacheEntry{}, false
	}

	c.mu.Lock()
	c.hits++
	c.mu.Unlock()

	return body, entry, true
}

func (c *HTTPCache) Put(rawURL string, resp *http.Response, body []byte) {
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

	if err := os.MkdirAll(c.root, 0o755); err != nil {
		return
	}
	_, bodyPath := c.paths(rawURL)
	if err := os.WriteFile(bodyPath, body, 0o644); err != nil {
		return
	}

	c.mu.Lock()
	c.entries[key] = entry
	c.dirty = true
	if c.lruItems != nil {
		c.upsertLRULocked(key, bodyBytes)
	}

	if c.writeTimer == nil {
		c.writeTimer = time.AfterFunc(200*time.Millisecond, func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.dirty {
				c.dirty = false
				c.saveIndexLocked()
			}
			c.writeTimer = nil
		})
	}
	c.mu.Unlock()
}

func (c *HTTPCache) Metrics() HTTPCacheMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()
	return HTTPCacheMetrics{
		Hits:      c.hits,
		Misses:    c.misses,
		Evictions: c.evictions,
	}
}

func (c *HTTPCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.writeTimer != nil {
		c.writeTimer.Stop()
		c.writeTimer = nil
	}

	for _, entry := range c.entries {
		_ = os.Remove(filepath.Join(c.root, entry.BodyFile))
	}

	c.entries = make(map[string]CacheEntry)
	if c.lruItems != nil {
		c.lruItems = make(map[string]*httpLRUEntry)
	}
	c.lruHead = nil
	c.lruTail = nil
	c.currentBytes = 0
	c.hits = 0
	c.misses = 0
	c.evictions = 0

	c.dirty = false
	c.saveIndexLocked()
}

// loadIndex loads index.json into memory
func (c *HTTPCache) loadIndex() {
	if c.private || c.root == "" {
		return
	}
	indexPath := filepath.Join(c.root, "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return
	}

	var entries map[string]CacheEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return
	}
	if entries == nil {
		entries = make(map[string]CacheEntry)
	}

	c.entries = entries

	if c.lruItems != nil {
		for key, entry := range entries {
			bodyPath := filepath.Join(c.root, entry.BodyFile)
			info, err := os.Stat(bodyPath)
			if err != nil {
				delete(c.entries, key)
				continue
			}
			byteSize := info.Size()
			e := &httpLRUEntry{key: key, byteSize: byteSize}
			c.lruItems[key] = e
			c.currentBytes += byteSize
			c.pushFrontLocked(e)
		}
	}
}

func (c *HTTPCache) saveIndexLocked() {
	if c.private || c.root == "" {
		return
	}
	data, err := json.MarshalIndent(c.entries, "", "  ")
	if err != nil {
		return
	}
	indexPath := filepath.Join(c.root, "index.json")
	tempFile, err := os.CreateTemp(c.root, ".index.*.tmp")
	if err != nil {
		return
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return
	}
	if err := tempFile.Close(); err != nil {
		return
	}
	_ = os.Rename(tempPath, indexPath)
}

// ---------------------------------------------------------------------------
// LRU implementation
// ---------------------------------------------------------------------------

func (c *HTTPCache) upsertLRULocked(key string, byteSize int64) {
	if existing, ok := c.lruItems[key]; ok {
		c.currentBytes -= existing.byteSize
		c.removeLRUEntryLocked(existing)
		delete(c.lruItems, key)
	}

	if c.cfg.MaxEntries > 0 {
		for len(c.lruItems) >= c.cfg.MaxEntries && c.lruTail != nil {
			c.evictLRUTailLocked()
		}
	}

	if c.cfg.MaxBytes > 0 {
		for c.currentBytes+byteSize > c.cfg.MaxBytes && c.lruTail != nil {
			c.evictLRUTailLocked()
		}
	}

	e := &httpLRUEntry{key: key, byteSize: byteSize}
	c.lruItems[key] = e
	c.currentBytes += byteSize
	c.pushFrontLocked(e)
}

func (c *HTTPCache) evictLRUTailLocked() {
	tail := c.lruTail
	if tail == nil {
		return
	}
	c.currentBytes -= tail.byteSize
	delete(c.lruItems, tail.key)
	c.removeLRUEntryLocked(tail)
	c.evictions++

	delete(c.entries, tail.key)
	c.dirty = true

	_ = os.Remove(filepath.Join(c.root, c.hashKey(tail.key)+".body"))
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

func (c *HTTPCache) recordMiss() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.misses++
	c.mu.Unlock()
}

func (c *HTTPCache) paths(rawURL string) (string, string) {
	key := c.keyFor(rawURL)
	return filepath.Join(c.root, key+".json"), filepath.Join(c.root, key+".body")
}

func (c *HTTPCache) keyFor(rawURL string) string {
	return c.hashKey(normalizeCacheURL(rawURL))
}

func (c *HTTPCache) hashKey(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// Helpers
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
