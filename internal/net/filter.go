package net

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
)

type shieldsDisabledKey struct{}

// WithShieldsDisabled returns a copy of parent context with the shields disabled flag set.
func WithShieldsDisabled(ctx context.Context, disabled bool) context.Context {
	return context.WithValue(ctx, shieldsDisabledKey{}, disabled)
}

// IsShieldsDisabled reports whether ad/tracker shields are disabled in ctx.
func IsShieldsDisabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, ok := ctx.Value(shieldsDisabledKey{}).(bool)
	return ok && v
}

// ErrBlockedByFilter reports that a request was refused by the ad/tracker filter.
var ErrBlockedByFilter = errors.New("net: request blocked by ad/tracker filter")

// FilterEngine matches URLs against domain, pattern, and exception rules
// in standard ad blocker formats (e.g., EasyList).
type FilterEngine struct {
	mu           sync.RWMutex
	enabled      bool
	domains      map[string]bool
	exceptions   map[string]bool
	patterns     []string
	blockedCount int64
}

// DefaultBlockedDomains contains high-volume analytics, tracker, and ad domains.
var defaultBlockedDomains = []string{
	"google-analytics.com",
	"googletagmanager.com",
	"doubleclick.net",
	"adservice.google.com",
	"pagead2.googlesyndication.com",
	"adnxs.com",
	"ads-twitter.com",
	"analytics.twitter.com",
	"connect.facebook.net",
	"criteo.com",
	"criteo.net",
	"outbrain.com",
	"taboola.com",
	"scorecardresearch.com",
	"hotjar.com",
	"mixpanel.com",
	"segment.io",
	"quantserve.com",
	"chartbeat.com",
	"moatads.com",
	"amazon-adsystem.com",
	"adcolony.com",
	"unityads.unity3d.com",
	"applovin.com",
}

// NewFilterEngine returns a filter engine pre-loaded with the default privacy rules.
func NewFilterEngine() *FilterEngine {
	fe := NewEmptyFilterEngine()
	fe.enabled = true
	for _, d := range defaultBlockedDomains {
		fe.AddDomain(d)
	}
	return fe
}

// NewEmptyFilterEngine returns an empty filter engine.
func NewEmptyFilterEngine() *FilterEngine {
	return &FilterEngine{
		enabled:    true,
		domains:    make(map[string]bool),
		exceptions: make(map[string]bool),
		patterns:   make([]string, 0),
	}
}

// SetEnabled enables or disables the filter.
func (f *FilterEngine) SetEnabled(enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enabled = enabled
}

// IsEnabled reports whether the filter is active.
func (f *FilterEngine) IsEnabled() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.enabled
}

// BlockedCount returns the total number of blocked requests.
func (f *FilterEngine) BlockedCount() int64 {
	return atomic.LoadInt64(&f.blockedCount)
}

// AddDomain adds a domain (and its subdomains) to the block list.
func (f *FilterEngine) AddDomain(domain string) {
	domain = cleanDomain(domain)
	if domain == "" {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.domains[domain] = true
}

// AddWhitelistDomain unblocks a domain (and its subdomains).
func (f *FilterEngine) AddWhitelistDomain(domain string) {
	domain = cleanDomain(domain)
	if domain == "" {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exceptions[domain] = true
}

// AddRule parses and registers a rule in standard Adblock Plus / EasyList format.
// Supported syntax:
//   - "||example.com^" : domain anchor
//   - "@@||example.com^" : exception/whitelist rule
//   - "example.com" : plain domain match
//   - "/ads/banner" : URL substring pattern
func (f *FilterEngine) AddRule(rule string) {
	rule = strings.TrimSpace(rule)
	if rule == "" || strings.HasPrefix(rule, "!") || strings.HasPrefix(rule, "#") {
		return // comment or cosmetic rule
	}

	// Exception rule
	if strings.HasPrefix(rule, "@@") {
		content := rule[2:]
		if strings.HasPrefix(content, "||") {
			domain := strings.TrimSuffix(strings.TrimPrefix(content, "||"), "^")
			f.AddWhitelistDomain(domain)
			return
		}
		f.AddWhitelistDomain(content)
		return
	}

	// Domain anchor rule: ||domain.com^
	if strings.HasPrefix(rule, "||") {
		domain := strings.TrimSuffix(strings.TrimPrefix(rule, "||"), "^")
		f.AddDomain(domain)
		return
	}

	// Domain without anchor or scheme
	if !strings.Contains(rule, "/") && !strings.Contains(rule, "*") {
		f.AddDomain(strings.TrimSuffix(rule, "^"))
		return
	}

	// Path or substring pattern
	pattern := strings.Trim(rule, "*")
	if pattern != "" {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.patterns = append(f.patterns, strings.ToLower(pattern))
	}
}

// ShouldBlock evaluates rawURL against the filter rules.
// If blocked, it returns true and the matching rule or domain, and increments BlockedCount.
func (f *FilterEngine) ShouldBlock(rawURL string) (bool, string) {
	if f == nil {
		return false, ""
	}
	f.mu.RLock()
	if !f.enabled {
		f.mu.RUnlock()
		return false, ""
	}
	f.mu.RUnlock()

	u, err := url.Parse(rawURL)
	if err != nil {
		return false, ""
	}

	host := strings.ToLower(u.Hostname())
	urlLower := strings.ToLower(rawURL)

	f.mu.RLock()
	defer f.mu.RUnlock()

	// 1. Check exceptions (whitelist) first
	if f.isDomainMatched(host, f.exceptions) {
		return false, ""
	}

	// 2. Check domain block rules
	if matchedDomain := f.findMatchingDomain(host, f.domains); matchedDomain != "" {
		atomic.AddInt64(&f.blockedCount, 1)
		return true, matchedDomain
	}

	// 3. Check substring/path patterns
	for _, pat := range f.patterns {
		if strings.Contains(urlLower, pat) {
			atomic.AddInt64(&f.blockedCount, 1)
			return true, pat
		}
	}

	return false, ""
}

func (f *FilterEngine) isDomainMatched(host string, set map[string]bool) bool {
	return f.findMatchingDomain(host, set) != ""
}

func (f *FilterEngine) findMatchingDomain(host string, set map[string]bool) string {
	curr := host
	for {
		if set[curr] {
			return curr
		}
		dot := strings.IndexByte(curr, '.')
		if dot < 0 {
			break
		}
		curr = curr[dot+1:]
	}
	return ""
}

func cleanDomain(d string) string {
	d = strings.TrimSpace(strings.ToLower(d))
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "||")
	d = strings.TrimSuffix(d, "^")
	if slash := strings.IndexByte(d, '/'); slash >= 0 {
		d = d[:slash]
	}
	if colon := strings.IndexByte(d, ':'); colon >= 0 {
		d = d[:colon]
	}
	return d
}
