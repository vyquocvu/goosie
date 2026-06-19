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
	"time"
)

type HTTPCache struct {
	root    string
	private bool
}

type CacheEntry struct {
	URL         string
	Status      int
	ContentType string
	StoredAt    time.Time
	ExpiresAt   time.Time
	BodyFile    string
}

func NewHTTPCache(root string, private bool) *HTTPCache {
	return &HTTPCache{root: root, private: private}
}

func (c *HTTPCache) Get(rawURL string) (string, CacheEntry, bool) {
	if c == nil || c.private {
		return "", CacheEntry{}, false
	}

	entryPath, bodyPath := c.paths(rawURL)
	data, err := os.ReadFile(entryPath)
	if err != nil {
		return "", CacheEntry{}, false
	}
	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return "", CacheEntry{}, false
	}
	if entry.ExpiresAt.IsZero() || !time.Now().Before(entry.ExpiresAt) {
		return "", CacheEntry{}, false
	}
	if entry.BodyFile != "" {
		bodyPath = filepath.Join(c.root, entry.BodyFile)
	}
	body, err := os.ReadFile(bodyPath)
	if err != nil {
		return "", CacheEntry{}, false
	}
	return string(body), entry, true
}

func (c *HTTPCache) Put(rawURL string, resp *http.Response, body string) {
	if c == nil || c.private || resp == nil {
		return
	}
	if resp.StatusCode != http.StatusOK || hasNonEmptyHeader(resp.Header, "Vary") || len(resp.Header.Values("Set-Cookie")) > 0 {
		return
	}
	maxAge, ok := cacheMaxAge(strings.Join(resp.Header.Values("Cache-Control"), ","))
	if !ok {
		return
	}
	storedAt := time.Now()
	key := c.key(rawURL)
	bodyFile := key + ".body"
	entry := CacheEntry{
		URL:         normalizeCacheURL(rawURL),
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
	if err := os.WriteFile(bodyPath, []byte(body), 0o644); err != nil {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	entryPath, _ := c.paths(rawURL)
	_ = os.WriteFile(entryPath, data, 0o644)
}

func hasNonEmptyHeader(header http.Header, name string) bool {
	for _, value := range header.Values(name) {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func (c *HTTPCache) paths(rawURL string) (string, string) {
	key := c.key(rawURL)
	return filepath.Join(c.root, key+".json"), filepath.Join(c.root, key+".body")
}

func (c *HTTPCache) key(rawURL string) string {
	sum := sha256.Sum256([]byte(normalizeCacheURL(rawURL)))
	return hex.EncodeToString(sum[:])
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
