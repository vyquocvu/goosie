package net

import (
	"net/http"
	"testing"
)

func TestHTTPCacheHitMissAndExpiry(t *testing.T) {
	cache := NewHTTPCache(t.TempDir(), false)

	if _, _, ok := cache.Get("https://example.test/miss"); ok {
		t.Fatal("empty cache returned a hit")
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.test/hit", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := newTestResponse(req, http.StatusOK, "fresh")
	resp.Header.Set("Content-Type", "text/plain")
	resp.Header.Set("Cache-Control", "max-age=60")
	cache.Put(req.URL.String(), resp, "fresh")

	body, entry, ok := cache.Get(req.URL.String())
	if !ok {
		t.Fatal("fresh cache entry missed")
	}
	if body != "fresh" {
		t.Fatalf("body = %q, want fresh", body)
	}
	if entry.URL != req.URL.String() {
		t.Errorf("entry URL = %q", entry.URL)
	}
	if entry.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", entry.Status)
	}
	if entry.ContentType != "text/plain" {
		t.Errorf("content type = %q, want text/plain", entry.ContentType)
	}

	expiredReq, err := http.NewRequest(http.MethodGet, "https://example.test/expired", nil)
	if err != nil {
		t.Fatal(err)
	}
	expiredResp := newTestResponse(expiredReq, http.StatusOK, "expired")
	expiredResp.Header.Set("Cache-Control", "max-age=0")
	cache.Put(expiredReq.URL.String(), expiredResp, "expired")
	if _, _, ok := cache.Get(expiredReq.URL.String()); ok {
		t.Fatal("expired cache entry returned a hit")
	}
}

func TestHTTPCachePrivateModeDoesNotWriteOrRead(t *testing.T) {
	cache := NewHTTPCache(t.TempDir(), true)
	req, err := http.NewRequest(http.MethodGet, "https://example.test/private", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := newTestResponse(req, http.StatusOK, "private")
	resp.Header.Set("Cache-Control", "max-age=60")

	cache.Put(req.URL.String(), resp, "private")

	if _, _, ok := cache.Get(req.URL.String()); ok {
		t.Fatal("private cache returned a hit")
	}
}

func TestHTTPCacheDoesNotStoreWithoutMaxAge(t *testing.T) {
	cache := NewHTTPCache(t.TempDir(), false)
	req, err := http.NewRequest(http.MethodGet, "https://example.test/no-cache", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := newTestResponse(req, http.StatusOK, "body")

	cache.Put(req.URL.String(), resp, "body")

	if _, _, ok := cache.Get(req.URL.String()); ok {
		t.Fatal("response without max-age was cached")
	}
}

func TestHTTPCacheVetoDirectivesOverrideMaxAge(t *testing.T) {
	tests := []struct {
		name         string
		cacheControl string
	}{
		{name: "no-store before max-age", cacheControl: "no-store, max-age=60"},
		{name: "no-store after max-age", cacheControl: "max-age=60, no-store"},
		{name: "private before max-age", cacheControl: "private, max-age=60"},
		{name: "private after max-age", cacheControl: "max-age=60, private"},
		{name: "no-cache before max-age", cacheControl: "no-cache, max-age=60"},
		{name: "no-cache after max-age", cacheControl: "max-age=60, no-cache"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewHTTPCache(t.TempDir(), false)
			req, err := http.NewRequest(http.MethodGet, "https://example.test/"+tt.name, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp := newTestResponse(req, http.StatusOK, "body")
			resp.Header.Set("Cache-Control", tt.cacheControl)

			cache.Put(req.URL.String(), resp, "body")

			if _, _, ok := cache.Get(req.URL.String()); ok {
				t.Fatalf("cached response with Cache-Control %q", tt.cacheControl)
			}
		})
	}
}

func TestHTTPCacheRejectsUnsafeResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		header string
		value  string
	}{
		{name: "partial response", status: http.StatusPartialContent},
		{name: "redirect response", status: http.StatusFound},
		{name: "not modified response", status: http.StatusNotModified},
		{name: "vary response", status: http.StatusOK, header: "Vary", value: "Accept-Encoding"},
		{name: "set-cookie response", status: http.StatusOK, header: "Set-Cookie", value: "session=abc; Path=/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewHTTPCache(t.TempDir(), false)
			req, err := http.NewRequest(http.MethodGet, "https://example.test/"+tt.name, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp := newTestResponse(req, tt.status, "body")
			resp.Header.Set("Cache-Control", "max-age=60")
			if tt.header != "" {
				resp.Header.Set(tt.header, tt.value)
			}

			cache.Put(req.URL.String(), resp, "body")

			if _, _, ok := cache.Get(req.URL.String()); ok {
				t.Fatalf("cached unsafe response: status=%d %s=%q", tt.status, tt.header, tt.value)
			}
		})
	}
}
