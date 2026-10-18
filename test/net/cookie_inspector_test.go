package net_test

import (
	"net/url"
	"testing"
	"time"

	transport "github.com/vyquocvu/goosie/internal/net"
)

func TestCookieJarInspection(t *testing.T) {
	jar := transport.NewCookieJar()

	u1, _ := url.Parse("https://example.com/app")
	jar.Store(u1, []string{
		"session_id=xyz123; Path=/; Secure; HttpOnly",
		"theme=dark; Path=/app",
	})

	u2, _ := url.Parse("https://sub.other.org/")
	jar.Store(u2, []string{
		"pref=1; Domain=other.org; Path=/",
		"expired=old; Max-Age=0",
	})

	all := jar.AllCookies()
	if len(all) != 3 {
		t.Fatalf("expected 3 active cookies, got %d", len(all))
	}

	// Test host filtering
	exampleCookies := jar.CookiesForHost("example.com")
	if len(exampleCookies) != 2 {
		t.Fatalf("expected 2 cookies for example.com, got %d", len(exampleCookies))
	}

	otherCookies := jar.CookiesForHost("sub.other.org")
	if len(otherCookies) != 1 {
		t.Fatalf("expected 1 cookie for sub.other.org (domain matched), got %d", len(otherCookies))
	}
	if otherCookies[0].Name != "pref" {
		t.Errorf("expected cookie pref, got %s", otherCookies[0].Name)
	}

	unmatchedCookies := jar.CookiesForHost("nomatch.com")
	if len(unmatchedCookies) != 0 {
		t.Fatalf("expected 0 cookies for nomatch.com, got %d", len(unmatchedCookies))
	}

	// Test Clear
	jar.Clear()
	if len(jar.AllCookies()) != 0 {
		t.Fatalf("expected 0 cookies after Clear(), got %d", len(jar.AllCookies()))
	}
}

func TestCookieJarExpiryExclusion(t *testing.T) {
	jar := transport.NewCookieJar()
	u, _ := url.Parse("https://example.com/")
	jar.Store(u, []string{
		"future=ok; Expires=" + time.Now().Add(1*time.Hour).Format(time.RFC1123),
	})

	all := jar.AllCookies()
	if len(all) != 1 {
		t.Fatalf("expected 1 future cookie, got %d", len(all))
	}
}
