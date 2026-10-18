package net

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdnet "net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrDoHLookupFailed indicates a DNS-over-HTTPS query failure.
var ErrDoHLookupFailed = errors.New("net: DoH lookup failed")

// CloudflareDoHEndpoint is Cloudflare's default 1.1.1.1 DoH JSON query endpoint.
const CloudflareDoHEndpoint = "https://cloudflare-dns.com/dns-query"

// GoogleDoHEndpoint is Google's default 8.8.8.8 DoH JSON query endpoint.
const GoogleDoHEndpoint = "https://dns.google/resolve"

// dohResponse mirrors the JSON schema used by Cloudflare and Google DoH endpoints.
type dohResponse struct {
	Status int `json:"Status"` // 0 = NOERROR, 3 = NXDOMAIN
	Answer []struct {
		Name string `json:"name"`
		Type int    `json:"type"` // 1 = A, 28 = AAAA
		TTL  int    `json:"TTL"`
		Data string `json:"data"`
	} `json:"Answer"`
}

type dohCacheEntry struct {
	ips       []stdnet.IP
	expiresAt time.Time
}

// DoHResolver performs DNS resolution over encrypted HTTPS (RFC 8484 JSON API).
type DoHResolver struct {
	endpoint string
	client   *http.Client
	mu       sync.RWMutex
	cache    map[string]dohCacheEntry
}

// NewDoHResolver constructs a DoH resolver querying the given endpoint URL.
func NewDoHResolver(endpoint string) *DoHResolver {
	return &DoHResolver{
		endpoint: endpoint,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: make(map[string]dohCacheEntry),
	}
}

// DefaultDoHResolver constructs a DoH resolver using Cloudflare's 1.1.1.1 endpoint.
func DefaultDoHResolver() *DoHResolver {
	return NewDoHResolver(CloudflareDoHEndpoint)
}

// AsAddressLookup adapts the DoHResolver into an AddressLookup function
// suitable for use with AddressGuard.
func (r *DoHResolver) AsAddressLookup() AddressLookup {
	return func(ctx context.Context, host string) ([]stdnet.IP, error) {
		return r.LookupIP(ctx, host)
	}
}

// LookupIP resolves host to IP addresses over HTTPS with TTL caching.
func (r *DoHResolver) LookupIP(ctx context.Context, host string) ([]stdnet.IP, error) {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return nil, errors.New("net: empty host for DoH lookup")
	}

	// Literal IP address check
	if ip := stdnet.ParseIP(host); ip != nil {
		return []stdnet.IP{ip}, nil
	}

	// Cache check
	now := time.Now()
	r.mu.RLock()
	entry, found := r.cache[host]
	r.mu.RUnlock()
	if found && now.Before(entry.expiresAt) {
		return entry.ips, nil
	}

	// Query IPv4 (A) record
	ips, ttl, err := r.queryRecord(ctx, host, "A")
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		// Attempt IPv6 (AAAA) record
		v6IPs, v6TTL, v6Err := r.queryRecord(ctx, host, "AAAA")
		if v6Err == nil && len(v6IPs) > 0 {
			ips = v6IPs
			ttl = v6TTL
		}
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("%w: no A/AAAA records found for %s", ErrDoHLookupFailed, host)
	}

	// Cache answer (clamp TTL between 10s and 1 hour)
	if ttl < 10 {
		ttl = 10
	} else if ttl > 3600 {
		ttl = 3600
	}

	r.mu.Lock()
	r.cache[host] = dohCacheEntry{
		ips:       ips,
		expiresAt: now.Add(time.Duration(ttl) * time.Second),
	}
	r.mu.Unlock()

	return ips, nil
}

func (r *DoHResolver) queryRecord(ctx context.Context, host, recordType string) ([]stdnet.IP, int, error) {
	u, err := url.Parse(r.endpoint)
	if err != nil {
		return nil, 0, err
	}
	q := u.Query()
	q.Set("name", host)
	q.Set("type", recordType)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/dns-json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: query %s: %w", ErrDoHLookupFailed, host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("%w: status %d from %s", ErrDoHLookupFailed, resp.StatusCode, r.endpoint)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, 0, err
	}

	var dResp dohResponse
	if err := json.Unmarshal(body, &dResp); err != nil {
		return nil, 0, fmt.Errorf("%w: invalid JSON response: %w", ErrDoHLookupFailed, err)
	}

	if dResp.Status != 0 {
		return nil, 0, fmt.Errorf("%w: DNS error status %d", ErrDoHLookupFailed, dResp.Status)
	}

	var result []stdnet.IP
	minTTL := 300
	for _, ans := range dResp.Answer {
		if ip := stdnet.ParseIP(strings.TrimSpace(ans.Data)); ip != nil {
			result = append(result, ip)
			if ans.TTL > 0 && ans.TTL < minTTL {
				minTTL = ans.TTL
			}
		}
	}

	return result, minTTL, nil
}
