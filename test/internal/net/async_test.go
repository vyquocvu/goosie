package net_test

import (
	"context"
	stdnet "net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
	"github.com/vyquocvu/goosie/internal/net"
)

func requireLoopbackListener(t *testing.T) {
	t.Helper()
	ln, err := stdnet.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("loopback listener unavailable in this environment: %v", err)
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("close loopback listener: %v", err)
	}
}

// TestAsyncFetchCancellation verifies that fetch can be cancelled mid-flight
func TestAsyncFetchCancellation(t *testing.T) {
	requireLoopbackListener(t)
	// Create a server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("slow response"))
	}))
	defer server.Close()

	fetcher := net.NewFetcher()
	ctx, cancel := context.WithCancel(context.Background())

	// Start fetch in goroutine
	done := make(chan error, 1)
	go func() {
		_, err := fetcher.FetchWithContext(ctx, server.URL, nil)
		done <- err
	}()

	// Cancel after a short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Wait for result
	select {
	case err := <-done:
		if err == nil {
			t.Error("Expected error from cancelled fetch, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Error("Fetch did not complete within timeout")
	}
}

// TestConcurrentFetches verifies multiple concurrent fetches work correctly
func TestConcurrentFetches(t *testing.T) {
	requireLoopbackListener(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer server.Close()

	fetcher := net.NewFetcher()
	const numFetches = 10

	var wg sync.WaitGroup
	errors := make(chan error, numFetches)

	for i := 0; i < numFetches; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			_, err := fetcher.FetchWithContext(ctx, server.URL, nil)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent fetch failed: %v", err)
	}
}

// TestAsyncFetchWithTimeout verifies that timeout context works correctly
func TestAsyncFetchWithTimeout(t *testing.T) {
	requireLoopbackListener(t)
	// Create a server that never responds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second) // Very long delay
	}))
	defer server.Close()

	fetcher := net.NewFetcher()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := fetcher.FetchWithContext(ctx, server.URL, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	// Should complete within reasonable time after timeout
	if elapsed > 500*time.Millisecond {
		t.Errorf("Fetch took too long to timeout: %v", elapsed)
	}
}

// TestMultipleNavigationCancellation simulates user rapidly navigating between pages
func TestMultipleNavigationCancellation(t *testing.T) {
	requireLoopbackListener(t)
	// Create multiple test servers
	servers := make([]*httptest.Server, 3)
	for i := range servers {
		delay := time.Duration(i+1) * 100 * time.Millisecond
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(delay)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("response"))
		}))
		defer servers[i].Close()
	}

	fetcher := net.NewFetcher()

	// Simulate rapid navigation - each navigation cancels previous one
	var ctx context.Context
	var cancel context.CancelFunc

	for i, server := range servers {
		// Cancel previous request
		if cancel != nil {
			cancel()
		}

		// Start new request
		ctx, cancel = context.WithCancel(context.Background())

		if i < len(servers)-1 {
			// For all but last, start fetch and immediately move to next
			go func(url string, c context.Context) {
				fetcher.FetchWithContext(c, url, nil)
			}(server.URL, ctx)
		} else {
			// Last one should complete
			_, err := fetcher.FetchWithContext(ctx, server.URL, nil)
			if err != nil {
				t.Errorf("Final fetch should succeed, got error: %v", err)
			}
		}
	}

	if cancel != nil {
		cancel()
	}
}
