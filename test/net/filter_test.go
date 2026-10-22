package net_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	transport "github.com/vyquocvu/goosie/internal/net"
)

func TestFilterEngineDefaultRules(t *testing.T) {
	fe := transport.NewFilterEngine()

	blockedURLs := []string{
		"https://www.google-analytics.com/analytics.js",
		"https://region.google-analytics.com/collect?v=2",
		"https://adservice.google.com/adsid/google/ui",
		"https://securepubads.g.doubleclick.net/gampad/ads",
		"https://connect.facebook.net/en_US/fbevents.js",
		"https://static.hotjar.com/c/hotjar-12345.js",
	}

	for _, u := range blockedURLs {
		blocked, reason := fe.ShouldBlock(u)
		if !blocked {
			t.Errorf("ShouldBlock(%q) = false, want true (reason %q)", u, reason)
		}
	}

	allowedURLs := []string{
		"https://example.com/index.html",
		"https://en.wikipedia.org/wiki/Go_(programming_language)",
		"https://news.ycombinator.com/",
		"https://github.com/vyquocvu/goosie",
	}

	for _, u := range allowedURLs {
		blocked, _ := fe.ShouldBlock(u)
		if blocked {
			t.Errorf("ShouldBlock(%q) = true, want false", u)
		}
	}
}

func TestFilterEngineEasyListSyntaxAndExceptions(t *testing.T) {
	fe := transport.NewEmptyFilterEngine()

	// Add domain anchor rule
	fe.AddRule("||bad-domain.com^")
	// Add pattern rule
	fe.AddRule("*/banner-ads/*")
	// Add exception rule
	fe.AddRule("@@||safe.bad-domain.com^")

	// Base domain and subdomains should be blocked
	if blocked, _ := fe.ShouldBlock("https://bad-domain.com/tracker"); !blocked {
		t.Errorf("bad-domain.com should be blocked")
	}
	if blocked, _ := fe.ShouldBlock("https://sub.bad-domain.com/tracker"); !blocked {
		t.Errorf("sub.bad-domain.com should be blocked")
	}

	// Exception subdomain must be permitted
	if blocked, _ := fe.ShouldBlock("https://safe.bad-domain.com/content"); blocked {
		t.Errorf("safe.bad-domain.com should be allowed by exception rule")
	}

	// Pattern match
	if blocked, _ := fe.ShouldBlock("https://other.com/assets/banner-ads/top.png"); !blocked {
		t.Errorf("URL with pattern /banner-ads/ should be blocked")
	}
}

func TestFilterEngineEnableDisable(t *testing.T) {
	fe := transport.NewFilterEngine()
	target := "https://doubleclick.net/ad"

	if blocked, _ := fe.ShouldBlock(target); !blocked {
		t.Errorf("expected doubleclick to be blocked when enabled")
	}

	fe.SetEnabled(false)
	if fe.IsEnabled() {
		t.Errorf("expected filter to be disabled")
	}
	if blocked, _ := fe.ShouldBlock(target); blocked {
		t.Errorf("expected doubleclick NOT to be blocked when filter is disabled")
	}

	fe.SetEnabled(true)
	if blocked, _ := fe.ShouldBlock(target); !blocked {
		t.Errorf("expected doubleclick to be blocked again when re-enabled")
	}
}

func TestHTTPClientFilterIntegration(t *testing.T) {
	client := transport.DefaultClient()
	ctx := context.Background()

	// Attempt to get a blocked URL
	resp, err := client.Get(ctx, "https://www.google-analytics.com/analytics.js")
	if resp != nil {
		t.Errorf("expected nil response for blocked URL")
	}
	if err == nil || !errors.Is(err, transport.ErrBlockedByFilter) {
		t.Fatalf("expected ErrBlockedByFilter, got: %v", err)
	}

	// Check blocked count
	if fClient, ok := client.(interface {
		Filter() *transport.FilterEngine
	}); ok {
		fe := fClient.Filter()
		if fe == nil || fe.BlockedCount() < 1 {
			t.Errorf("expected BlockedCount >= 1, got %v", fe)
		}
	}
}

func TestFilterEngineConcurrency(t *testing.T) {
	fe := transport.NewFilterEngine()
	fe.AddRule("||concurrent-tracker.com^")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				fe.ShouldBlock("https://concurrent-tracker.com/pixel")
			} else {
				fe.ShouldBlock("https://legitimate-site.org/home")
			}
		}(i)
	}
	wg.Wait()

	if fe.BlockedCount() != 25 {
		t.Errorf("BlockedCount = %d, want 25", fe.BlockedCount())
	}
}
