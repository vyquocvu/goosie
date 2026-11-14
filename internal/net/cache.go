package net

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxCacheBytes = 64 << 20

type cacheEntry struct {
	resp      *Response
	expiresAt time.Time
	size      int
}

func (e *cacheEntry) fresh(now time.Time) bool {
	return now.Before(e.expiresAt)
}

type responseCache struct {
	mu      sync.Mutex
	entries map[string]*cacheEntry
	order   []string
	bytes   int
}

func newResponseCache() *responseCache {
	return &responseCache{entries: make(map[string]*cacheEntry)}
}

func (c *responseCache) get(key string, now time.Time) (*Response, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if !e.fresh(now) {
		c.removeLocked(key)
		return nil, false
	}
	return e.resp, true
}

func (c *responseCache) store(key string, resp *Response, now time.Time) {
	lifetime, ok := freshnessLifetime(resp, now)
	if !ok {
		return
	}
	e := &cacheEntry{
		resp:      resp,
		expiresAt: now.Add(lifetime),
		size:      len(resp.Body) + len(key) + 64,
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, exists := c.entries[key]; exists {
		c.bytes -= old.size
		delete(c.entries, key)
		c.dropOrderLocked(key)
	}
	c.entries[key] = e
	c.order = append(c.order, key)
	c.bytes += e.size
	for c.bytes > maxCacheBytes && len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		if victim, ok := c.entries[oldest]; ok {
			c.bytes -= victim.size
			delete(c.entries, oldest)
		}
	}
}

func (c *responseCache) removeLocked(key string) {
	if e, ok := c.entries[key]; ok {
		c.bytes -= e.size
		delete(c.entries, key)
		c.dropOrderLocked(key)
	}
}

func (c *responseCache) dropOrderLocked(key string) {
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
}

func (c *responseCache) clear() {
	c.mu.Lock()
	c.entries = make(map[string]*cacheEntry)
	c.order = nil
	c.bytes = 0
	c.mu.Unlock()
}

func freshnessLifetime(resp *Response, now time.Time) (time.Duration, bool) {
	if resp.StatusCode != http.StatusOK {
		return 0, false
	}
	if _, seen := lookupHeader(resp.Headers, "Set-Cookie"); seen {
		return 0, false
	}
	cc, _ := lookupHeader(resp.Headers, "Cache-Control")
	var maxAge time.Duration
	haveMaxAge := false
	for _, tok := range strings.Split(strings.ToLower(cc), ",") {
		tok = strings.TrimSpace(tok)
		switch {
		case tok == "no-store" || tok == "no-cache" || tok == "private":
			return 0, false
		case strings.HasPrefix(tok, "max-age="):
			if secs, err := strconv.Atoi(strings.TrimPrefix(tok, "max-age=")); err == nil && secs >= 0 {
				maxAge = time.Duration(secs) * time.Second
				haveMaxAge = true
			}
		}
	}
	if haveMaxAge {
		if maxAge == 0 {
			return 0, false
		}
		return maxAge, true
	}
	if exp, ok := lookupHeader(resp.Headers, "Expires"); ok {
		if t, err := http.ParseTime(exp); err == nil {
			if d := t.Sub(now); d > 0 {
				return d, true
			}
		}
	}
	return 0, false
}

func lookupHeader(headers map[string]string, name string) (string, bool) {
	if v, ok := headers[name]; ok {
		return v, true
	}
	for k, v := range headers {
		if strings.EqualFold(k, name) {
			return v, true
		}
	}
	return "", false
}
