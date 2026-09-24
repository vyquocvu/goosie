package net_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	transport "github.com/vyquocvu/goosie/internal/net"
)

// hitCounter serves one fixed body and records how many requests reached it.
func hitCounter(t *testing.T, headers map[string]string) (*httptest.Server, *int64) {
	t.Helper()
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, "posted")
			return
		}
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		_, _ = io.WriteString(w, "payload")
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestCacheServesFreshResponse(t *testing.T) {
	srv, hits := hitCounter(t, map[string]string{"Cache-Control": "public, max-age=60"})
	client := transport.DefaultClient()
	defer client.Close()
	first, err := client.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt64(hits); n != 1 {
		t.Fatalf("server hits = %d, want 1 (fresh response should be served from cache)", n)
	}
	if string(second.Body) != string(first.Body) || second.StatusCode != first.StatusCode {
		t.Fatalf("cached response differs: %d %q vs %d %q", second.StatusCode, second.Body, first.StatusCode, first.Body)
	}
	if second.URL != first.URL {
		t.Fatalf("cached URL = %q, want %q", second.URL, first.URL)
	}
}

func TestCacheSkipsNoStore(t *testing.T) {
	srv, hits := hitCounter(t, map[string]string{"Cache-Control": "no-store"})
	client := transport.DefaultClient()
	defer client.Close()
	for i := 0; i < 2; i++ {
		if _, err := client.Get(context.Background(), srv.URL); err != nil {
			t.Fatal(err)
		}
	}
	if n := atomic.LoadInt64(hits); n != 2 {
		t.Fatalf("server hits = %d, want 2 (no-store must never be cached)", n)
	}
}

func TestCacheSkipsPrivateAndSetCookie(t *testing.T) {
	cases := map[string]map[string]string{
		"private":  {"Cache-Control": "private, max-age=60"},
		"cookie":   {"Cache-Control": "public, max-age=60", "Set-Cookie": "a=1"},
		"noCache":  {"Cache-Control": "no-cache"},
		"maxAge0":  {"Cache-Control": "public, max-age=0"},
	}
	for name, headers := range cases {
		t.Run(name, func(t *testing.T) {
			srv, hits := hitCounter(t, headers)
			client := transport.DefaultClient()
			defer client.Close()
			for i := 0; i < 2; i++ {
				if _, err := client.Get(context.Background(), srv.URL); err != nil {
					t.Fatal(err)
				}
			}
			if n := atomic.LoadInt64(hits); n != 2 {
				t.Fatalf("server hits = %d, want 2 (%s must not be cached)", n, name)
			}
		})
	}
}

func TestCacheEntryExpires(t *testing.T) {
	srv, hits := hitCounter(t, map[string]string{"Cache-Control": "public, max-age=1"})
	client := transport.DefaultClient()
	defer client.Close()
	if _, err := client.Get(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	if _, err := client.Get(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt64(hits); n != 2 {
		t.Fatalf("server hits = %d, want 2 (entry expired after max-age=1)", n)
	}
}

func TestCachePostNotCached(t *testing.T) {
	srv, hits := hitCounter(t, map[string]string{"Cache-Control": "public, max-age=60"})
	client := transport.DefaultClient()
	defer client.Close()
	for i := 0; i < 2; i++ {
		if _, err := client.Post(context.Background(), srv.URL, "text/plain", strings.NewReader("x")); err != nil {
			t.Fatal(err)
		}
	}
	if n := atomic.LoadInt64(hits); n != 2 {
		t.Fatalf("server hits = %d, want 2 (POST responses must not be cached)", n)
	}
}

func TestCacheBoundedByBytes(t *testing.T) {
	body := strings.Repeat("a", 64<<10)
	var n int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := atomic.AddInt64(&n, 1)
		w.Header().Set("Cache-Control", "public, max-age=600")
		_, _ = fmt.Fprintf(w, "%s-%d", body, i)
	}))
	defer srv.Close()
	client := transport.DefaultClient()
	defer client.Close()
	// Fill past the bound with distinct URLs; the client must stay correct
	// (a re-get of any stored URL either hits the server or the cache, never
	// returns a wrong body).
	for i := 0; i < 2000; i++ {
		if _, err := client.Get(context.Background(), fmt.Sprintf("%s/%d", srv.URL, i)); err != nil {
			t.Fatal(err)
		}
	}
	resp, err := client.Get(context.Background(), srv.URL+"/0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(resp.Body), body) {
		t.Fatal("wrong body after eviction churn")
	}
}
