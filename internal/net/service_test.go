package net

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"strings"
	"testing"
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
