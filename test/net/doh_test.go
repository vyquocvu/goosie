package net_test

import (
	"context"
	"encoding/json"
	stdnet "net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	transport "github.com/vyquocvu/goosie/internal/net"
)

func TestDoHResolverSuccessAndCache(t *testing.T) {
	var queryCount int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&queryCount, 1)

		if r.Header.Get("Accept") != "application/dns-json" {
			t.Errorf("expected Accept: application/dns-json header")
		}

		name := r.URL.Query().Get("name")
		if name != "example.com" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		resp := map[string]interface{}{
			"Status": 0,
			"Answer": []map[string]interface{}{
				{
					"name": "example.com",
					"type": 1,
					"TTL":  60,
					"data": "93.184.216.34",
				},
			},
		}
		w.Header().Set("Content-Type", "application/dns-json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	resolver := transport.NewDoHResolver(ts.URL)
	ctx := context.Background()

	// 1. Initial resolution
	ips, err := resolver.LookupIP(ctx, "example.com")
	if err != nil {
		t.Fatalf("LookupIP failed: %v", err)
	}
	if len(ips) != 1 || ips[0].String() != "93.184.216.34" {
		t.Fatalf("LookupIP = %v, want [93.184.216.34]", ips)
	}

	if count := atomic.LoadInt32(&queryCount); count != 1 {
		t.Fatalf("queryCount = %d, want 1", count)
	}

	// 2. Second resolution should hit cache and NOT issue HTTP query
	ipsCached, err := resolver.LookupIP(ctx, "example.com")
	if err != nil {
		t.Fatalf("LookupIP cached failed: %v", err)
	}
	if len(ipsCached) != 1 || ipsCached[0].String() != "93.184.216.34" {
		t.Fatalf("LookupIP cached = %v, want [93.184.216.34]", ipsCached)
	}

	if count := atomic.LoadInt32(&queryCount); count != 1 {
		t.Errorf("second lookup must be served from cache; queryCount = %d, want 1", count)
	}
}

func TestDoHLiteralIP(t *testing.T) {
	resolver := transport.NewDoHResolver("http://invalid.endpoint")
	ctx := context.Background()

	// Literal IPv4
	ips, err := resolver.LookupIP(ctx, "127.0.0.1")
	if err != nil || len(ips) != 1 || ips[0].String() != "127.0.0.1" {
		t.Errorf("literal IPv4 lookup failed: %v, %v", ips, err)
	}

	// Literal IPv6
	ips6, err := resolver.LookupIP(ctx, "::1")
	if err != nil || len(ips6) != 1 || ips6[0].String() != "::1" {
		t.Errorf("literal IPv6 lookup failed: %v, %v", ips6, err)
	}
}

func TestDoHNXDOMAIN(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"Status": 3, // NXDOMAIN
		}
		w.Header().Set("Content-Type", "application/dns-json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	resolver := transport.NewDoHResolver(ts.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := resolver.LookupIP(ctx, "nonexistent.example")
	if err == nil {
		t.Errorf("expected error on NXDOMAIN, got nil")
	}
}

func TestDoHAsAddressLookup(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"Status": 0,
			"Answer": []map[string]interface{}{
				{"name": "test.com", "type": 1, "TTL": 120, "data": "10.0.0.1"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	resolver := transport.NewDoHResolver(ts.URL)
	lookup := resolver.AsAddressLookup()

	ips, err := lookup(context.Background(), "test.com")
	if err != nil {
		t.Fatalf("AddressLookup failed: %v", err)
	}
	if len(ips) != 1 || !ips[0].Equal(stdnet.ParseIP("10.0.0.1")) {
		t.Errorf("got %v, want 10.0.0.1", ips)
	}
}
