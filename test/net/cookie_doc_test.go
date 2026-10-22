package net_test

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/net"
)

func TestCookieJarDocumentAccess(t *testing.T) {
	jar := net.NewCookieJar()
	docURL := "https://example.com/blog/posts"

	// 1. Initially empty
	if c := jar.CookiesForDocument(docURL); c != "" {
		t.Fatalf("expected empty cookies, got %q", c)
	}

	// 2. Set cookie from document
	jar.SetCookieFromDocument(docURL, "theme=dark; path=/")
	jar.SetCookieFromDocument(docURL, "lang=vi; path=/blog")

	got := jar.CookiesForDocument(docURL)
	if !strings.Contains(got, "theme=dark") || !strings.Contains(got, "lang=vi") {
		t.Fatalf("expected theme=dark and lang=vi, got %q", got)
	}

	// 3. Different path check
	otherURL := "https://example.com/other"
	otherCookies := jar.CookiesForDocument(otherURL)
	if !strings.Contains(otherCookies, "theme=dark") {
		t.Errorf("expected theme=dark at /other, got %q", otherCookies)
	}
	if strings.Contains(otherCookies, "lang=vi") {
		t.Errorf("lang=vi should not be visible at /other, got %q", otherCookies)
	}

	// 4. HttpOnly cookie stored via HTTP response must NOT be visible to document
	u, _ := url.Parse("https://example.com/")
	jar.Store(u, []string{"session_token=secret123; Path=/; HttpOnly; Secure"})

	docCookies := jar.CookiesForDocument(docURL)
	if strings.Contains(docCookies, "session_token") {
		t.Errorf("document.cookie MUST NOT see HttpOnly cookies, got %q", docCookies)
	}

	// But HTTP request header MUST include session_token
	reqURL, _ := url.Parse(docURL)
	header := jar.HeaderFor(reqURL)
	if !strings.Contains(header, "session_token=secret123") {
		t.Errorf("HTTP request header MUST include HttpOnly session_token, got %q", header)
	}

	// 5. Script cannot overwrite existing HttpOnly cookie
	jar.SetCookieFromDocument(docURL, "session_token=tampered; Path=/")
	header = jar.HeaderFor(reqURL)
	if !strings.Contains(header, "session_token=secret123") || strings.Contains(header, "tampered") {
		t.Errorf("script must not overwrite HttpOnly cookie, header is %q", header)
	}

	// 6. Script deleting cookie via Max-Age=0
	jar.SetCookieFromDocument(docURL, "lang=vi; path=/blog; max-age=0")
	gotAfterDelete := jar.CookiesForDocument(docURL)
	if strings.Contains(gotAfterDelete, "lang=vi") {
		t.Errorf("expected lang=vi to be deleted by max-age=0, got %q", gotAfterDelete)
	}

	// 7. Script deleting cookie via expired Date
	past := time.Now().Add(-24 * time.Hour).Format(time.RFC1123)
	jar.SetCookieFromDocument(docURL, "theme=dark; path=/; expires="+past)
	gotAfterExpire := jar.CookiesForDocument(docURL)
	if strings.Contains(gotAfterExpire, "theme=dark") {
		t.Errorf("expected theme=dark to be deleted by expires in past, got %q", gotAfterExpire)
	}
}
