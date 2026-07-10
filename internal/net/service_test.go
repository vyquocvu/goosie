package net

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func TestServiceFetchRecordsRequestLog(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("User-Agent"); got != "Goosie-Test" {
			t.Fatalf("User-Agent = %q, want Goosie-Test", got)
		}
		resp := newTestResponse(req, http.StatusOK, "hello")
		resp.Header.Set("Content-Type", "text/plain")
		return resp, nil
	})}
	service := NewService(ServiceOptions{Client: client, UserAgent: "Goosie-Test"})

	body, err := service.Fetch("https://example.test/page")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if body != "hello" {
		t.Fatalf("body = %q, want hello", body)
	}

	entries := service.Log().Entries()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Method != http.MethodGet {
		t.Errorf("Method = %q, want GET", entry.Method)
	}
	if entry.URL != "https://example.test/page" {
		t.Errorf("URL = %q", entry.URL)
	}
	if entry.Status != http.StatusOK {
		t.Errorf("Status = %d, want 200", entry.Status)
	}
	if entry.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want text/plain", entry.ContentType)
	}
	if entry.Bytes != int64(len("hello")) {
		t.Errorf("Bytes = %d, want %d", entry.Bytes, len("hello"))
	}
	if entry.CacheHit {
		t.Error("CacheHit = true, want false")
	}
	if entry.Error != "" {
		t.Errorf("Error = %q, want empty", entry.Error)
	}
	if entry.StartedAt.IsZero() {
		t.Error("StartedAt was zero")
	}
	if entry.Duration <= 0 {
		t.Error("Duration was not recorded")
	}
}

func TestServiceFetchUsesHTTPMaxAgeCacheOnSecondRequest(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		resp := newTestResponse(req, http.StatusOK, "cached body")
		resp.Header.Set("Content-Type", "text/html")
		resp.Header.Set("Cache-Control", "max-age=60")
		return resp, nil
	})}
	cache := NewHTTPCache(t.TempDir(), false)
	service := NewService(ServiceOptions{Client: client, Cache: cache})

	first, err := service.Fetch("https://example.test/cache")
	if err != nil {
		t.Fatalf("first Fetch returned error: %v", err)
	}
	second, err := service.Fetch("https://example.test/cache")
	if err != nil {
		t.Fatalf("second Fetch returned error: %v", err)
	}
	if first != "cached body" || second != "cached body" {
		t.Fatalf("bodies = %q/%q, want cached body", first, second)
	}
	if calls != 1 {
		t.Fatalf("transport calls = %d, want 1", calls)
	}

	entries := service.Log().Entries()
	if len(entries) != 2 {
		t.Fatalf("log entries = %d, want 2", len(entries))
	}
	if entries[0].CacheHit {
		t.Error("first request should not be cache hit")
	}
	if !entries[1].CacheHit {
		t.Error("second request should be cache hit")
	}
}

func TestServiceDefaultClientHasCookieJar(t *testing.T) {
	service := NewService(ServiceOptions{})

	if service.client == nil {
		t.Fatal("default client was nil")
	}
	if service.client.Jar == nil {
		t.Fatal("default client Jar was nil")
	}
	if _, ok := service.client.Jar.(*cookiejar.Jar); !ok {
		t.Fatalf("default client Jar = %T, want *cookiejar.Jar", service.client.Jar)
	}
}

func TestServiceFetchWithContextHonorsProgressReader(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusOK, "progress")
		resp.ContentLength = int64(len("progress"))
		resp.Header.Set("Content-Length", "8")
		return resp, nil
	})}
	service := NewService(ServiceOptions{Client: client})
	var lastProgress float64
	body, err := service.FetchWithContext(context.Background(), "https://example.test/progress", func(progress float64) {
		lastProgress = progress
	})
	if err != nil {
		t.Fatalf("FetchWithContext returned error: %v", err)
	}
	if body != "progress" {
		t.Fatalf("body = %q, want progress", body)
	}
	if lastProgress != 1 {
		t.Fatalf("last progress = %v, want 1", lastProgress)
	}
}

func TestServiceSecuritySummaryFromTLSResponse(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.test/secure", nil)
	if err != nil {
		t.Fatal(err)
	}
	cert := syntheticCertificate("Example Subject", "Example Issuer")
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusOK, "secure")
		resp.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
		return resp, nil
	})}
	service := NewService(ServiceOptions{Client: client})

	if _, err := service.FetchWithContext(context.Background(), req.URL.String(), nil); err != nil {
		t.Fatalf("FetchWithContext returned error: %v", err)
	}
	summary := service.Security()
	if summary.URL != "https://example.test/secure" {
		t.Errorf("URL = %q", summary.URL)
	}
	if summary.Scheme != "https" {
		t.Errorf("Scheme = %q, want https", summary.Scheme)
	}
	if !summary.Secure {
		t.Error("Secure = false, want true")
	}
	if summary.Subject != "CN=Example Subject" {
		t.Errorf("Subject = %q", summary.Subject)
	}
	if summary.Issuer != "CN=Example Issuer" {
		t.Errorf("Issuer = %q", summary.Issuer)
	}
	if !summary.NotAfter.Equal(cert.NotAfter) || !summary.NotBefore.Equal(cert.NotBefore) {
		t.Error("certificate validity dates were not copied")
	}
}

func TestServiceCookieBearingRequestBypassesCacheLookup(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err := http.NewRequest(http.MethodGet, "https://example.test/private", nil)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(u.URL, []*http.Cookie{{Name: "session", Value: "abc", Path: "/"}})

	cache := NewHTTPCache(t.TempDir(), false)
	cachedResp := newTestResponse(u, http.StatusOK, "stale")
	cachedResp.Header.Set("Cache-Control", "max-age=60")
	cache.Put(u.URL.String(), cachedResp, "stale")
	calls := 0
	client := &http.Client{Jar: jar, Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return newTestResponse(req, http.StatusOK, "fresh"), nil
	})}
	service := NewService(ServiceOptions{Client: client, Cache: cache})

	body, err := service.Fetch(u.URL.String())
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if body != "fresh" {
		t.Fatalf("body = %q, want fresh", body)
	}
	if calls != 1 {
		t.Fatalf("transport calls = %d, want 1", calls)
	}
}

func TestServiceCookieBearingRequestDoesNotPopulateCache(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err := http.NewRequest(http.MethodGet, "https://example.test/private", nil)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(u.URL, []*http.Cookie{{Name: "session", Value: "abc", Path: "/"}})

	cache := NewHTTPCache(t.TempDir(), false)
	client := &http.Client{Jar: jar, Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusOK, "private")
		resp.Header.Set("Cache-Control", "max-age=60")
		return resp, nil
	})}
	service := NewService(ServiceOptions{Client: client, Cache: cache})

	if _, err := service.Fetch(u.URL.String()); err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if _, _, ok := cache.Get(u.URL.String()); ok {
		t.Fatal("cookie-bearing response populated persistent cache")
	}
}

func TestServiceCacheHitReplacesSecuritySummary(t *testing.T) {
	cache := NewHTTPCache(t.TempDir(), false)
	cachedReq, err := http.NewRequest(http.MethodGet, "http://cached.example.test/page", nil)
	if err != nil {
		t.Fatal(err)
	}
	cachedResp := newTestResponse(cachedReq, http.StatusOK, "cached")
	cachedResp.Header.Set("Cache-Control", "max-age=60")
	cache.Put(cachedReq.URL.String(), cachedResp, "cached")

	cert := syntheticCertificate("Origin A", "Issuer A")
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		resp := newTestResponse(req, http.StatusOK, "origin-a")
		resp.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
		return resp, nil
	})}
	service := NewService(ServiceOptions{Client: client, Cache: cache})

	if _, err := service.Fetch("https://origin-a.example.test/"); err != nil {
		t.Fatalf("origin A Fetch returned error: %v", err)
	}
	if _, err := service.Fetch(cachedReq.URL.String()); err != nil {
		t.Fatalf("cached Fetch returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("transport calls = %d, want 1", calls)
	}

	summary := service.Security()
	if summary.URL != cachedReq.URL.String() {
		t.Errorf("URL = %q, want %q", summary.URL, cachedReq.URL.String())
	}
	if summary.Scheme != "http" || summary.Secure {
		t.Errorf("scheme/secure = %q/%v, want http/false", summary.Scheme, summary.Secure)
	}
	if summary.Subject != "" || summary.Issuer != "" || !summary.NotBefore.IsZero() || !summary.NotAfter.IsZero() {
		t.Fatalf("cached summary retained certificate data: %#v", summary)
	}
}

// --- limitedContextReader tests ---

func TestLimitedContextReaderReadsNormally(t *testing.T) {
	body := "hello, world"
	reader := &limitedContextReader{
		ctx:    context.Background(),
		reader: strings.NewReader(body),
		limit:  100,
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if string(out) != body {
		t.Fatalf("read %q, want %q", string(out), body)
	}
}

func TestLimitedContextReaderExceedsLimit(t *testing.T) {
	reader := &limitedContextReader{
		ctx:    context.Background(),
		reader: strings.NewReader("this body is too long"),
		limit:  5,
	}
	_, err := io.ReadAll(reader)
	if err == nil {
		t.Fatal("expected error for exceeded limit, got nil")
	}
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("error = %v, want ErrBodyTooLarge", err)
	}
}

func TestLimitedContextReaderExactLimit(t *testing.T) {
	// A body that's exactly the limit should fail — we can't distinguish
	// between "body equals limit" and "body exceeds limit" without reading more.
	body := "exactly"
	reader := &limitedContextReader{
		ctx:    context.Background(),
		reader: strings.NewReader(body),
		limit:  int64(len(body)),
	}
	_, err := io.ReadAll(reader)
	if err == nil {
		t.Fatal("expected error when body == limit, got nil")
	}
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("error = %v, want ErrBodyTooLarge", err)
	}
}

func TestLimitedContextReaderContextCancelledBeforeRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reader := &limitedContextReader{
		ctx:    ctx,
		reader: strings.NewReader("data"),
		limit:  100,
	}
	_, err := io.ReadAll(reader)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestLimitedContextReaderContextCancelledDuringRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	reader := &limitedContextReader{
		ctx:    ctx,
		reader: strings.NewReader(strings.Repeat("x", 1000)),
		limit:  0,
	}

	buf := make([]byte, 100)
	n, err := reader.Read(buf)
	if err != nil {
		t.Fatalf("first read failed: %v", err)
	}
	if n != 100 {
		t.Fatalf("first read returned %d bytes, want 100", n)
	}

	// Cancel — next Read should detect immediately
	cancel()

	_, err = reader.Read(buf)
	if err == nil {
		t.Fatal("expected error after cancel, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestLimitedContextReaderUnlimited(t *testing.T) {
	body := strings.Repeat("a", 10000)
	reader := &limitedContextReader{
		ctx:    context.Background(),
		reader: strings.NewReader(body),
		limit:  0, // unlimited
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(out) != len(body) {
		t.Fatalf("read %d bytes, want %d", len(out), len(body))
	}
}

func TestLimitedContextReaderEmptyBody(t *testing.T) {
	reader := &limitedContextReader{
		ctx:    context.Background(),
		reader: strings.NewReader(""),
		limit:  100,
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("read %d bytes, want 0", len(out))
	}
}

func TestLimitedContextReaderZeroLimitAndCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reader := &limitedContextReader{
		ctx:    ctx,
		reader: strings.NewReader("data"),
		limit:  0, // unlimited
	}
	_, err := io.ReadAll(reader)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

// --- Service FetchWithContext body-size-limit integration tests ---

func TestServiceFetchWithContextBodyTooLarge(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return newTestResponse(req, http.StatusOK, "body that is too large"), nil
	})}
	service := NewService(ServiceOptions{
		Client:      client,
		MaxBodySize: 10, // only allow 10 bytes
	})

	_, err := service.FetchWithContext(context.Background(), "https://example.test/page", nil)
	if err == nil {
		t.Fatal("expected error for body too large, got nil")
	}
	if !errors.Is(err, ErrBodyTooLarge) && !strings.Contains(err.Error(), "body too large") {
		t.Fatalf("error = %v, want ErrBodyTooLarge", err)
	}
}

func TestServiceFetchWithContextCancelledDuringBodyRead(t *testing.T) {
	// Server that sends headers + partial body, then waits indefinitely
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("partial "))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(started)
		// Block until client disconnects
		<-r.Context().Done()
	}))
	defer server.Close()

	service := NewService(ServiceOptions{Client: &http.Client{}})

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, err := service.FetchWithContext(ctx, server.URL, nil)
		errCh <- err
	}()

	// Wait for server to start sending body
	<-started

	// Cancel while body read is in progress
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error for cancelled context during body read, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fetch did not complete within timeout after cancel")
	}
}

func TestServiceFetchWithContextBodySizeBackwardCompat(t *testing.T) {
	// Default MaxBodySize = 0 means unlimited — existing behavior preserved
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return newTestResponse(req, http.StatusOK, strings.Repeat("x", 50000)), nil
	})}
	service := NewService(ServiceOptions{Client: client})

	body, err := service.FetchWithContext(context.Background(), "https://example.test/large", nil)
	if err != nil {
		t.Fatalf("FetchWithContext returned error: %v", err)
	}
	if len(body) != 50000 {
		t.Fatalf("body length = %d, want 50000", len(body))
	}
}

func TestServiceFetchWithContextBodySizePreservesProgressCallback(t *testing.T) {
	bodyContent := "progress check"
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusOK, bodyContent)
		resp.ContentLength = int64(len(bodyContent))
		resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(bodyContent)))
		return resp, nil
	})}
	service := NewService(ServiceOptions{Client: client, MaxBodySize: 100})

	var lastProgress float64
	body, err := service.FetchWithContext(context.Background(), "https://example.test/progress", func(p float64) {
		lastProgress = p
	})
	if err != nil {
		t.Fatalf("FetchWithContext returned error: %v", err)
	}
	if body != bodyContent {
		t.Fatalf("body = %q, want %q", body, bodyContent)
	}
	if lastProgress != 1 {
		t.Fatalf("last progress = %v, want 1", lastProgress)
	}
}

func TestServiceFetchWithContextBodySizeLogsEntryOnError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusOK, "this body exceeds the limit")
		resp.Header.Set("Content-Type", "text/plain")
		return resp, nil
	})}
	service := NewService(ServiceOptions{Client: client, MaxBodySize: 5})

	_, err := service.FetchWithContext(context.Background(), "https://example.test/error", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	entries := service.Log().Entries()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Error == "" {
		t.Fatal("expected error message in log entry")
	}
	if entry.Status != http.StatusOK {
		t.Fatalf("entry.Status = %d, want %d", entry.Status, http.StatusOK)
	}
	if entry.Bytes != 0 {
		t.Fatalf("entry.Bytes = %d, want 0 for failed read", entry.Bytes)
	}
}

func TestServiceFetchWithContextBodySizeDoesNotAffectCache(t *testing.T) {
	transportCalls := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		transportCalls++
		resp := newTestResponse(req, http.StatusOK, "small body")
		resp.Header.Set("Cache-Control", "max-age=60")
		return resp, nil
	})}
	cache := NewHTTPCache(t.TempDir(), false)
	service := NewService(ServiceOptions{Client: client, Cache: cache, MaxBodySize: 100})

	// First fetch should succeed and populate cache
	body, err := service.Fetch("https://example.test/cached")
	if err != nil {
		t.Fatalf("first Fetch returned error: %v", err)
	}
	if body != "small body" {
		t.Fatalf("first body = %q", body)
	}

	// Second fetch should hit cache (not transport)
	body, err = service.Fetch("https://example.test/cached")
	if err != nil {
		t.Fatalf("second Fetch returned error: %v", err)
	}
	if body != "small body" {
		t.Fatalf("second body = %q", body)
	}
	if transportCalls != 1 {
		t.Fatalf("transport calls = %d, want 1", transportCalls)
	}
}

func TestServiceFetchWithContextBodySizeUnderLimitWithBinaryData(t *testing.T) {
	bodyBytes := make([]byte, 100)
	for i := range bodyBytes {
		bodyBytes[i] = byte(i)
	}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return newTestResponse(req, http.StatusOK, string(bodyBytes)), nil
	})}
	service := NewService(ServiceOptions{Client: client, MaxBodySize: 200})

	body, err := service.FetchWithContext(context.Background(), "https://example.test/binary", nil)
	if err != nil {
		t.Fatalf("FetchWithContext returned error: %v", err)
	}
	if len(body) != 100 {
		t.Fatalf("body length = %d, want 100", len(body))
	}
	for i := 0; i < 100; i++ {
		if body[i] != byte(i) {
			t.Fatalf("body[%d] = %d, want %d", i, body[i], i)
		}
	}
}

func TestServiceFetchWithContextBodySizeOneByteOverLimit(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return newTestResponse(req, http.StatusOK, strings.Repeat("x", 11)), nil
	})}
	service := NewService(ServiceOptions{Client: client, MaxBodySize: 10})

	_, err := service.FetchWithContext(context.Background(), "https://example.test/overflow", nil)
	if err == nil {
		t.Fatal("expected error for body 1 byte over limit")
	}
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("error = %v, want ErrBodyTooLarge", err)
	}
}

func TestServiceFetchWithContextBodySizeZeroLimitWithErrorResponse(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return newTestResponse(req, http.StatusInternalServerError, "error page body"), nil
	})}
	service := NewService(ServiceOptions{Client: client, MaxBodySize: 0})

	body, err := service.FetchWithContext(context.Background(), "https://example.test/error", nil)
	if err != nil {
		t.Fatalf("FetchWithContext returned error: %v", err)
	}
	// Non-empty error body is returned as-is (not wrapped in error page)
	if body != "error page body" {
		t.Fatalf("body = %q, want %q", body, "error page body")
	}
}

// --- limitedContextReader benchmarks ---

func BenchmarkLimitedContextReader(b *testing.B) {
	body := strings.Repeat("hello-world-", 1000) // ~12KB
	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reader := &limitedContextReader{
			ctx:    ctx,
			reader: strings.NewReader(body),
			limit:  0,
		}
		_, err := io.ReadAll(reader)
		if err != nil {
			b.Fatalf("ReadAll failed: %v", err)
		}
	}
}

func BenchmarkLimitedContextReaderWithLimit(b *testing.B) {
	body := strings.Repeat("hello-world-", 1000)
	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reader := &limitedContextReader{
			ctx:    ctx,
			reader: strings.NewReader(body),
			limit:  10000, // smaller than body
		}
		_, err := io.ReadAll(reader)
		if err == nil {
			b.Fatal("expected ErrBodyTooLarge")
		}
	}
}

func BenchmarkLimitedContextReaderCancelled(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	body := strings.Repeat("hello-world-", 1000)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reader := &limitedContextReader{
			ctx:    ctx,
			reader: strings.NewReader(body),
			limit:  0,
		}
		_, err := io.ReadAll(reader)
		if err == nil {
			b.Fatal("expected error for cancelled context")
		}
	}
}

func TestServiceRedirectedResponseDoesNotPopulateOriginalURLCache(t *testing.T) {
	const rawURL = "https://example.test/start"
	finalReq, err := http.NewRequest(http.MethodGet, "https://example.test/final", nil)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		resp := newTestResponse(finalReq, http.StatusOK, "redirected body")
		resp.Header.Set("Cache-Control", "max-age=60")
		return resp, nil
	})}
	service := NewService(ServiceOptions{Client: client, Cache: NewHTTPCache(t.TempDir(), false)})

	for i := 0; i < 2; i++ {
		body, err := service.Fetch(rawURL)
		if err != nil {
			t.Fatalf("Fetch %d returned error: %v", i+1, err)
		}
		if body != "redirected body" {
			t.Fatalf("Fetch %d body = %q", i+1, body)
		}
	}
	if calls != 2 {
		t.Fatalf("transport calls = %d, want 2", calls)
	}
	if summary := service.Security(); summary.URL != finalReq.URL.String() {
		t.Fatalf("security URL = %q, want final URL %q", summary.URL, finalReq.URL)
	}
	entries := service.Log().Entries()
	if entries[0].CacheHit || entries[1].CacheHit {
		t.Fatalf("redirected request log contained cache hit: %#v", entries)
	}
}

// --- FetchStream tests (M1.3) ---

func TestFetchStreamReturnsBodyReader(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusOK, "<html><body>streaming</body></html>")
		resp.Header.Set("Content-Type", "text/html")
		return resp, nil
	})}
	service := NewService(ServiceOptions{Client: client})

	body, meta, err := service.FetchStream(context.Background(), "https://example.test/page")
	if err != nil {
		t.Fatalf("FetchStream returned error: %v", err)
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll on stream returned error: %v", err)
	}
	if string(data) != "<html><body>streaming</body></html>" {
		t.Fatalf("body = %q", string(data))
	}
	if meta.Status != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", meta.Status, http.StatusOK)
	}
	if meta.ContentType != "text/html" {
		t.Errorf("ContentType = %q, want text/html", meta.ContentType)
	}
}

func TestFetchStreamContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service := NewService(ServiceOptions{})
	_, _, err := service.FetchStream(ctx, "https://example.test/page")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestFetchStreamBodyTooLarge(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return newTestResponse(req, http.StatusOK, "body that is too large for streaming"), nil
	})}
	service := NewService(ServiceOptions{Client: client, MaxBodySize: 10})

	body, _, err := service.FetchStream(context.Background(), "https://example.test/page")
	if err != nil {
		t.Fatalf("FetchStream returned error: %v", err)
	}
	defer body.Close()

	_, readErr := io.ReadAll(body)
	if readErr == nil {
		t.Fatal("expected error when reading body that exceeds limit")
	}
	if !errors.Is(readErr, ErrBodyTooLarge) {
		t.Fatalf("read error = %v, want ErrBodyTooLarge", readErr)
	}
}

func TestFetchStreamErrorResponse(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return newTestResponse(req, http.StatusNotFound, "not found"), nil
	})}
	service := NewService(ServiceOptions{Client: client})

	body, meta, err := service.FetchStream(context.Background(), "https://example.test/missing")
	if err != nil {
		t.Fatalf("FetchStream returned error: %v", err)
	}
	defer body.Close()

	if meta.Status != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", meta.Status, http.StatusNotFound)
	}
	data, _ := io.ReadAll(body)
	if string(data) != "not found" {
		t.Fatalf("body = %q, want %q", string(data), "not found")
	}
}

func TestFetchStreamPreservesMetadata(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusOK, "body")
		resp.Header.Set("Content-Type", "text/html; charset=utf-8")
		resp.Header.Set("X-Custom", "value")
		resp.ContentLength = 4
		return resp, nil
	})}
	service := NewService(ServiceOptions{Client: client})

	body, meta, err := service.FetchStream(context.Background(), "https://example.test/meta")
	if err != nil {
		t.Fatalf("FetchStream returned error: %v", err)
	}
	defer body.Close()

	if meta.Status != http.StatusOK {
		t.Errorf("StatusCode = %d", meta.Status)
	}
	if meta.ContentLength != 4 {
		t.Errorf("ContentLength = %d, want 4", meta.ContentLength)
	}
	if meta.Header.Get("X-Custom") != "value" {
		t.Errorf("X-Custom = %q, want value", meta.Header.Get("X-Custom"))
	}
}

func TestFetchStreamCancellationDuringRead(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("partial "))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	service := NewService(ServiceOptions{Client: &http.Client{}})
	ctx, cancel := context.WithCancel(context.Background())

	body, _, err := service.FetchStream(ctx, server.URL)
	if err != nil {
		t.Fatalf("FetchStream returned error: %v", err)
	}
	defer body.Close()

	<-started
	cancel()

	_, readErr := io.ReadAll(body)
	if readErr == nil {
		t.Fatal("expected error after cancel during stream read")
	}
}

func TestFetchStreamLogsEntry(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusOK, "logged body")
		resp.Header.Set("Content-Type", "text/plain")
		return resp, nil
	})}
	service := NewService(ServiceOptions{Client: client})

	body, _, err := service.FetchStream(context.Background(), "https://example.test/log")
	if err != nil {
		t.Fatalf("FetchStream returned error: %v", err)
	}
	body.Close()

	entries := service.Log().Entries()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	if entries[0].Status != http.StatusOK {
		t.Errorf("Status = %d, want %d", entries[0].Status, http.StatusOK)
	}
	if entries[0].URL != "https://example.test/log" {
		t.Errorf("URL = %q", entries[0].URL)
	}
}

func BenchmarkFetchStreamVsBuffered(b *testing.B) {
	htmlBody := strings.Repeat("<p>Hello world paragraph content for streaming benchmark.</p>", 100)
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return newTestResponse(req, http.StatusOK, htmlBody), nil
	})}

	b.Run("FetchStream", func(b *testing.B) {
		service := NewService(ServiceOptions{Client: client})
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			body, _, err := service.FetchStream(context.Background(), "https://example.test/bench")
			if err != nil {
				b.Fatal(err)
			}
			_, _ = io.ReadAll(body)
			body.Close()
		}
	})

	b.Run("FetchWithMeta", func(b *testing.B) {
		service := NewService(ServiceOptions{Client: client})
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _, err := service.FetchWithMeta(context.Background(), "https://example.test/bench", nil)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
