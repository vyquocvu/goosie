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

	jar.Clear()
	if len(jar.AllCookies()) != 0 {
		t.Fatalf("expected 0 cookies after Clear(), got %d", len(jar.AllCookies()))
	}
}

// qc:BR-058
func TestCookieJarRemoveForDomainDeletesOnlyThatSite(t *testing.T) {
	jar := transport.NewCookieJar()
	example, _ := url.Parse("https://example.com/")
	jar.Store(example, []string{
		"a=1; Path=/; Max-Age=3600",
		"b=2; Path=/app; Max-Age=3600",
	})
	other, _ := url.Parse("https://other.org/")
	jar.Store(other, []string{"c=3; Path=/; Max-Age=3600"})
	sub, _ := url.Parse("https://sub.example.com/")
	jar.Store(sub, []string{"d=4; Path=/; Max-Age=3600"})

	if got := jar.RemoveForDomain("example.com"); got != 2 {
		t.Fatalf("RemoveForDomain(example.com) = %d, want 2", got)
	}
	remaining := jar.AllCookies()
	if len(remaining) != 2 {
		t.Fatalf("cookies after removing example.com = %+v, want other.org and sub.example.com", remaining)
	}
	domains := map[string]bool{}
	for _, c := range remaining {
		domains[c.Domain] = true
	}
	if !domains["other.org"] || !domains["sub.example.com"] || domains["example.com"] {
		t.Fatalf("remaining cookie domains = %v, want other.org and sub.example.com only", domains)
	}
	if got := jar.RemoveForDomain("example.com"); got != 0 {
		t.Fatalf("second RemoveForDomain(example.com) = %d, want 0", got)
	}
	if got := jar.RemoveForDomain(""); got != 0 {
		t.Fatalf("RemoveForDomain(\"\") = %d, want 0", got)
	}
	if got := jar.RemoveForDomain("other.org"); got != 1 {
		t.Fatalf("RemoveForDomain(other.org) = %d, want 1", got)
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
