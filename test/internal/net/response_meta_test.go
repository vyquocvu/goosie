package net_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"github.com/vyquocvu/goosie/internal/net"
)

// --- ResponseMeta unit tests ---

func TestResponseMetaFromResponseCapturesStatusAndHeaders(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/page", nil)
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/2.0",
		ContentLength: 1234,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader("")),
		Request:       req,
	}
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	resp.Header.Set("X-Custom", "value")
	resp.Header.Set("Content-Encoding", "gzip")

	meta := net.ResponseMetaFromResponse(resp)
	if meta.Status != http.StatusOK {
		t.Errorf("Status = %d, want 200", meta.Status)
	}
	if meta.ContentType != "text/html; charset=utf-8" {
		t.Errorf("ContentType = %q", meta.ContentType)
	}
	if meta.ContentLength != 1234 {
		t.Errorf("ContentLength = %d, want 1234", meta.ContentLength)
	}
	if meta.ContentEncoding != "gzip" {
		t.Errorf("ContentEncoding = %q, want gzip", meta.ContentEncoding)
	}
	if meta.Protocol != "HTTP/2.0" {
		t.Errorf("Protocol = %q, want HTTP/2.0", meta.Protocol)
	}
	if meta.Charset != "utf-8" {
		t.Errorf("Charset = %q, want utf-8", meta.Charset)
	}
	if meta.Header.Get("X-Custom") != "value" {
		t.Errorf("Header X-Custom = %q, want value", meta.Header.Get("X-Custom"))
	}
}

func TestResponseMetaFromResponseNilResponse(t *testing.T) {
	meta := net.ResponseMetaFromResponse(nil)
	if meta.Status != 0 {
		t.Errorf("Status = %d, want 0", meta.Status)
	}
	if meta.ContentType != "" {
		t.Errorf("ContentType = %q, want empty", meta.ContentType)
	}
	if meta.Header == nil {
		t.Fatal("Header should be non-nil even for nil response")
	}
}

func TestResponseMetaFromResponseEmptyHeaders(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp := &http.Response{
		StatusCode: http.StatusNotFound,
		Proto:      "HTTP/1.1",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}
	meta := net.ResponseMetaFromResponse(resp)
	if meta.Status != http.StatusNotFound {
		t.Errorf("Status = %d, want 404", meta.Status)
	}
	if meta.Protocol != "HTTP/1.1" {
		t.Errorf("Protocol = %q, want HTTP/1.1", meta.Protocol)
	}
	if meta.Charset != "" {
		t.Errorf("Charset = %q, want empty for missing content-type", meta.Charset)
	}
}

func TestResponseMetaCharsetExtraction(t *testing.T) {
	tests := []struct {
		contentType string
		wantCharset string
	}{
		{"text/html; charset=utf-8", "utf-8"},
		{"text/html; charset=ISO-8859-1", "ISO-8859-1"},
		{"text/html", ""},
		{"application/json; charset=utf-8", "utf-8"},
		{"text/html;CHARSET=utf-8", "utf-8"},
		{"", ""},
	}
	for _, tc := range tests {
		req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
		resp := &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}
		if tc.contentType != "" {
			resp.Header.Set("Content-Type", tc.contentType)
		}
		meta := net.ResponseMetaFromResponse(resp)
		if meta.Charset != tc.wantCharset {
			t.Errorf("ContentType=%q: Charset=%q, want %q", tc.contentType, meta.Charset, tc.wantCharset)
		}
	}
}

// --- Service.FetchWithMeta tests ---

func TestServiceFetchWithMetaCapturesMetadata(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusOK, "hello")
		resp.Header.Set("Content-Type", "text/html; charset=utf-8")
		resp.Header.Set("X-Request-Id", "abc123")
		resp.ContentLength = 5
		resp.Proto = "HTTP/1.1"
		return resp, nil
	})}
	service := net.NewService(net.ServiceOptions{Client: client})

	body, meta, err := service.FetchWithMeta(context.Background(), "https://example.test/page", nil)
	if err != nil {
		t.Fatalf("FetchWithMeta returned error: %v", err)
	}
	if body != "hello" {
		t.Errorf("body = %q, want hello", body)
	}
	if meta.Status != http.StatusOK {
		t.Errorf("Status = %d, want 200", meta.Status)
	}
	if meta.ContentType != "text/html; charset=utf-8" {
		t.Errorf("ContentType = %q", meta.ContentType)
	}
	if meta.Charset != "utf-8" {
		t.Errorf("Charset = %q, want utf-8", meta.Charset)
	}
	if meta.ContentLength != 5 {
		t.Errorf("ContentLength = %d, want 5", meta.ContentLength)
	}
	if meta.Protocol != "HTTP/1.1" {
		t.Errorf("Protocol = %q", meta.Protocol)
	}
	if meta.Header.Get("X-Request-Id") != "abc123" {
		t.Errorf("X-Request-Id = %q", meta.Header.Get("X-Request-Id"))
	}
}

func TestServiceFetchWithMetaCacheHitSynthesizesMeta(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		resp := newTestResponse(req, http.StatusOK, "cached body")
		resp.Header.Set("Content-Type", "text/html")
		resp.Header.Set("Cache-Control", "max-age=60")
		return resp, nil
	})}
	cache := net.NewHTTPCache(t.TempDir(), false)
	service := net.NewService(net.ServiceOptions{Client: client, Cache: cache})

	// First fetch populates cache
	_, _, err := service.FetchWithMeta(context.Background(), "https://example.test/cache", nil)
	if err != nil {
		t.Fatalf("first FetchWithMeta returned error: %v", err)
	}

	// Second fetch hits cache
	body, meta, err := service.FetchWithMeta(context.Background(), "https://example.test/cache", nil)
	if err != nil {
		t.Fatalf("second FetchWithMeta returned error: %v", err)
	}
	if body != "cached body" {
		t.Errorf("body = %q, want cached body", body)
	}
	if calls != 1 {
		t.Errorf("transport calls = %d, want 1", calls)
	}
	if meta.Status != http.StatusOK {
		t.Errorf("Status = %d, want 200", meta.Status)
	}
	if meta.ContentType != "text/html" {
		t.Errorf("ContentType = %q, want text/html", meta.ContentType)
	}
	if !meta.Cached {
		t.Error("Cached = false, want true for cache hit")
	}
}

func TestServiceFetchWithMetaErrorResponse(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusInternalServerError, "error body")
		resp.Header.Set("Content-Type", "text/plain")
		return resp, nil
	})}
	service := net.NewService(net.ServiceOptions{Client: client})

	body, meta, err := service.FetchWithMeta(context.Background(), "https://example.test/error", nil)
	if err != nil {
		t.Fatalf("FetchWithMeta returned error: %v", err)
	}
	if body != "error body" {
		t.Errorf("body = %q, want error body", body)
	}
	if meta.Status != http.StatusInternalServerError {
		t.Errorf("Status = %d, want 500", meta.Status)
	}
}

func TestServiceFetchWithMetaNetworkError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})}
	service := net.NewService(net.ServiceOptions{Client: client})

	_, meta, err := service.FetchWithMeta(context.Background(), "https://example.test/timeout", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if meta.Status != 0 {
		t.Errorf("Status = %d on error, want 0", meta.Status)
	}
}

func TestServiceFetchWithMetaCancelledContext(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, req.Context().Err()
	})}
	service := net.NewService(net.ServiceOptions{Client: client})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, meta, err := service.FetchWithMeta(ctx, "https://example.test/cancel", nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if meta.Status != 0 {
		t.Errorf("Status = %d, want 0", meta.Status)
	}
}

func TestServiceFetchWithMetaBodyTooLarge(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusOK, "body that is too large")
		resp.Header.Set("Content-Type", "text/plain")
		return resp, nil
	})}
	service := net.NewService(net.ServiceOptions{Client: client, MaxBodySize: 5})

	_, meta, err := service.FetchWithMeta(context.Background(), "https://example.test/large", nil)
	if err == nil {
		t.Fatal("expected error for body too large")
	}
	// Metadata should still capture response info
	if meta.Status != http.StatusOK {
		t.Errorf("Status = %d, want 200", meta.Status)
	}
	if meta.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want text/plain", meta.ContentType)
	}
}

func TestServiceFetchBackwardCompatible(t *testing.T) {
	// Ensure existing Fetch/FetchWithContext still work unchanged
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusOK, "hello")
		return resp, nil
	})}
	service := net.NewService(net.ServiceOptions{Client: client})

	body, err := service.Fetch("https://example.test/page")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if body != "hello" {
		t.Fatalf("body = %q, want hello", body)
	}
}

// --- Fetcher.Meta() tests ---

func TestFetcherMetaReturnsLastResponseMetadata(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusOK, "hello")
		resp.Header.Set("Content-Type", "text/html")
		resp.Proto = "HTTP/1.1"
		return resp, nil
	})}
	fetcher := net.NewFetcherWithClient(client)

	// Before any fetch, Meta returns zero-value
	initial := fetcher.Meta()
	if initial.Status != 0 {
		t.Errorf("initial Status = %d, want 0", initial.Status)
	}

	_, err := fetcher.Fetch("https://example.test/page")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	meta := fetcher.Meta()
	if meta.Status != http.StatusOK {
		t.Errorf("Status = %d, want 200", meta.Status)
	}
	if meta.ContentType != "text/html" {
		t.Errorf("ContentType = %q", meta.ContentType)
	}
	if meta.Protocol != "HTTP/1.1" {
		t.Errorf("Protocol = %q", meta.Protocol)
	}
}

func TestFetcherMetaConcurrentSafety(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusOK, "ok")
		resp.Header.Set("Content-Type", "text/plain")
		return resp, nil
	})}
	fetcher := net.NewFetcherWithClient(client)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			_, _ = fetcher.Fetch("https://example.test/page")
		}
		close(done)
	}()
	// Concurrent reads of Meta should not race
	for i := 0; i < 100; i++ {
		_ = fetcher.Meta()
	}
	<-done
}

func TestFetcherMetaOnCacheHit(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		resp := newTestResponse(req, http.StatusOK, "cached")
		resp.Header.Set("Content-Type", "text/html")
		resp.Header.Set("Cache-Control", "max-age=60")
		return resp, nil
	})}
	cache := net.NewHTTPCache(t.TempDir(), false)
	fetcher := net.NewFetcherWithService(net.NewService(net.ServiceOptions{Client: client, Cache: cache}))

	// First fetch
	_, _ = fetcher.Fetch("https://example.test/page")
	// Second fetch (cache hit)
	_, _ = fetcher.Fetch("https://example.test/page")

	meta := fetcher.Meta()
	if meta.Status != http.StatusOK {
		t.Errorf("Status = %d, want 200", meta.Status)
	}
	if !meta.Cached {
		t.Error("Cached = false, want true for cache hit")
	}
}

// --- Benchmark for metadata capture overhead ---

func BenchmarkResponseMetaFromResponse(b *testing.B) {
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/page", nil)
	resp := &http.Response{
		StatusCode:    200,
		Proto:         "HTTP/1.1",
		ContentLength: 1000,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader("")),
		Request:       req,
	}
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	resp.Header.Set("X-Custom", "value")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = net.ResponseMetaFromResponse(resp)
	}
}

func BenchmarkFetchWithMeta(b *testing.B) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusOK, "benchmark body")
		resp.Header.Set("Content-Type", "text/html")
		return resp, nil
	})}
	service := net.NewService(net.ServiceOptions{Client: client})
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := service.FetchWithMeta(ctx, "https://example.test/page", nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func init() {
	// Ensure default transport is available for tests
	_ = time.Now()
}
