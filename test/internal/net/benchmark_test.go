package net_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"github.com/vyquocvu/goosie/internal/net"
)

func BenchmarkFetchSimple(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, benchmark!"))
	}))
	defer ts.Close()

	svc := net.NewService(net.ServiceOptions{})
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := svc.FetchWithContext(ctx, ts.URL, nil)
		if err != nil {
			b.Fatalf("Fetch failed: %v", err)
		}
	}
}

func BenchmarkFetchCached(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Cached response"))
	}))
	defer ts.Close()

	tempDir, err := os.MkdirTemp("", "goosie_net_bench_")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cache := net.NewHTTPCache(tempDir, false)
	svc := net.NewService(net.ServiceOptions{Cache: cache})
	ctx := context.Background()

	// Initial fetch to populate cache
	_, err = svc.FetchWithContext(ctx, ts.URL, nil)
	if err != nil {
		b.Fatalf("Initial fetch failed: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := svc.FetchWithContext(ctx, ts.URL, nil)
		if err != nil {
			b.Fatalf("Cached fetch failed: %v", err)
		}
	}
}
