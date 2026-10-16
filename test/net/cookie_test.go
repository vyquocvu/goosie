package net_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/net"
)

// capture requests echo back the Cookie header they received, so tests assert
// on what the client actually sent.
func captureServer(t *testing.T, setCookies ...string) (*httptest.Server, *[]string) {
	t.Helper()
	got := &[]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = append(*got, r.Header.Get("Cookie"))
		for _, sc := range setCookies {
			w.Header().Add("Set-Cookie", sc)
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html></html>")
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestCookieStoredAndSentOnNextRequest(t *testing.T) {
	srv, got := captureServer(t, "session=abc123; Path=/")
	ctx := context.Background()
	client := net.DefaultClient()
	defer client.Close()

	u, _ := url.Parse(srv.URL)
	if _, err := client.Get(ctx, srv.URL+"/"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(ctx, srv.URL+"/page"); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 2 {
		t.Fatalf("got %d requests, want 2", len(*got))
	}
	if (*got)[0] != "" {
		t.Fatalf("first request already had a cookie: %q", (*got)[0])
	}
	if (*got)[1] != "session=abc123" {
		t.Fatalf("second request Cookie = %q, want session=abc123", (*got)[1])
	}
	_ = u
}

func TestCookieDomainAttributeMustMatchHost(t *testing.T) {
	// A host claiming a Domain it does not own (or a bare TLD) must not have
	// that cookie stored; requests to any path on the same host then carry no
	// cookie at all.
	srv, got := captureServer(t, "bad=tossed; Domain=example.org; Path=/", "tld=wild; Domain=com; Path=/")
	ctx := context.Background()
	client := net.DefaultClient()
	defer client.Close()

	if _, err := client.Get(ctx, srv.URL+"/"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(ctx, srv.URL+"/again"); err != nil {
		t.Fatal(err)
	}
	if (*got)[1] != "" {
		t.Fatalf("cookie with foreign Domain attribute was sent: %q", (*got)[1])
	}
}

func TestSecureCookieNotSentOverHTTP(t *testing.T) {
	srv, got := captureServer(t, "secret=1; Secure; Path=/")
	ctx := context.Background()
	client := net.DefaultClient()
	defer client.Close()

	if _, err := client.Get(ctx, srv.URL+"/"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(ctx, srv.URL+"/again"); err != nil {
		t.Fatal(err)
	}
	if (*got)[1] != "" {
		t.Fatalf("Secure cookie sent over plain HTTP: %q", (*got)[1])
	}
}

func TestHostOnlyCookieNotSentToSubdomain(t *testing.T) {
	// The jar is queried directly: no test server can easily serve two hosts.
	jar := net.NewCookieJar()
	u, _ := url.Parse("https://example.com/")
	jar.Store(u, []string{"a=1"})
	sub, _ := url.Parse("https://api.example.com/")
	if got := jar.HeaderFor(sub); got != "" {
		t.Fatalf("host-only cookie sent to subdomain: %q", got)
	}
	same, _ := url.Parse("https://example.com/deep/path")
	if got := jar.HeaderFor(same); got != "a=1" {
		t.Fatalf("host-only cookie missing on same host: %q", got)
	}
}

func TestDomainCookieSentToSubdomain(t *testing.T) {
	jar := net.NewCookieJar()
	u, _ := url.Parse("https://example.com/")
	jar.Store(u, []string{"b=2; Domain=example.com"})
	sub, _ := url.Parse("https://api.example.com/")
	if got := jar.HeaderFor(sub); got != "b=2" {
		t.Fatalf("Domain cookie missing on subdomain: %q", got)
	}
}

func TestCookiePathMatching(t *testing.T) {
	jar := net.NewCookieJar()
	u, _ := url.Parse("https://example.com/docs/page")
	jar.Store(u, []string{"p=1"})
	if got := jar.HeaderFor(mustURL(t, "https://example.com/docs/more")); got != "p=1" {
		t.Fatalf("cookie missing for sibling path: %q", got)
	}
	if got := jar.HeaderFor(mustURL(t, "https://example.com/docsx")); got != "" {
		t.Fatalf("cookie sent for prefix-but-not-path: %q", got)
	}
	if got := jar.HeaderFor(mustURL(t, "https://example.com/other")); got != "" {
		t.Fatalf("cookie sent outside its path: %q", got)
	}
}

func TestMaxAgeZeroDeletesCookie(t *testing.T) {
	jar := net.NewCookieJar()
	u, _ := url.Parse("https://example.com/")
	jar.Store(u, []string{"gone=1"})
	jar.Store(u, []string{"gone=; Max-Age=0"})
	if got := jar.HeaderFor(u); got != "" {
		t.Fatalf("cookie survived Max-Age=0 deletion: %q", got)
	}
}

func TestExpiredCookieNotSent(t *testing.T) {
	jar := net.NewCookieJar()
	u, _ := url.Parse("https://example.com/")
	jar.Store(u, []string{"old=1; Expires=Wed, 21 Oct 2015 07:28:00 GMT"})
	if got := jar.HeaderFor(u); got != "" {
		t.Fatalf("expired cookie sent: %q", got)
	}
}

func TestClearEmptiesJar(t *testing.T) {
	jar := net.NewCookieJar()
	u, _ := url.Parse("https://example.com/")
	jar.Store(u, []string{"a=1; Path=/", "b=2; Path=/"})
	jar.Clear()
	if got := jar.HeaderFor(u); got != "" {
		t.Fatalf("jar still serves cookies after Clear: %q", got)
	}
}

func TestPerHostCookieLimit(t *testing.T) {
	jar := net.NewCookieJar()
	u, _ := url.Parse("https://example.com/")
	for i := 0; i < net.MaxCookiesPerHost+20; i++ {
		jar.Store(u, []string{fmt.Sprintf("k%03d=%d; Path=/", i, i)})
	}
	got := jar.HeaderFor(u)
	count := len(strings.Split(got, "; "))
	if count > net.MaxCookiesPerHost {
		t.Fatalf("jar holds %d cookies for one host, cap is %d", count, net.MaxCookiesPerHost)
	}
	// The oldest cookies are the ones evicted; the newest set must survive.
	if !strings.Contains(got, fmt.Sprintf("k%03d=%d", net.MaxCookiesPerHost+19, net.MaxCookiesPerHost+19)) {
		t.Fatalf("most recently set cookie was evicted: %q", got)
	}
}

func TestPrivateClientIsolatesAndClears(t *testing.T) {
	srv, _ := captureServer(t, "sess=xyz; Path=/")
	ctx := context.Background()

	private := net.DefaultPrivateClient()
	if _, err := private.Get(ctx, srv.URL+"/"); err != nil {
		t.Fatal(err)
	}

	// A separate default client never sees the private session's cookie.
	otherSrv, otherGot := captureServer(t)
	otherClient := net.DefaultClient()
	defer otherClient.Close()
	if _, err := otherClient.Get(ctx, otherSrv.URL+"/"); err != nil {
		t.Fatal(err)
	}
	if len(*otherGot) != 1 || (*otherGot)[0] != "" {
		t.Fatalf("cookie leaked between clients: %q", (*otherGot)[0])
	}

	if err := private.Close(); err != nil {
		t.Fatal(err)
	}
	// After Close the private jar is empty: a fresh request carries no cookie.
	srv2, got2 := captureServer(t)
	if _, err := private.Get(ctx, srv2.URL+"/"); err != nil {
		t.Fatal(err)
	}
	if (*got2)[0] != "" {
		t.Fatalf("private client still sent a cookie after Close: %q", (*got2)[0])
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// TestLoadEnforcesJarLimits pins migration safety: a hostile or legacy
// cookie file past the jar invariants must load bounded exactly as if the
// cookies had arrived live (per-host and total eviction), never unbounded.
func TestLoadEnforcesJarLimits(t *testing.T) {
	type persisted struct {
		Name    string    `json:"name"`
		Value   string    `json:"value"`
		Domain  string    `json:"domain"`
		Path    string    `json:"path"`
		Expires time.Time `json:"expires"`
	}
	exp := time.Now().Add(24 * time.Hour)
	var list []persisted
	for i := 0; i < 60; i++ {
		list = append(list, persisted{Name: fmt.Sprintf("a%d", i), Value: "v", Domain: "a.example", Path: "/", Expires: exp})
	}
	for h := 0; h < 62; h++ {
		for i := 0; i < 50; i++ {
			list = append(list, persisted{Name: fmt.Sprintf("n%d", i), Value: "v", Domain: fmt.Sprintf("h%d.example", h), Path: "/", Expires: exp})
		}
	}
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "cookies.json")
	if err := os.WriteFile(in, data, 0o644); err != nil {
		t.Fatal(err)
	}
	jar := net.NewCookieJar()
	if err := jar.Load(in); err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Count through a Save round-trip: session cookies persist here (future
	// expiry), so the saved file reflects exactly what the jar holds.
	out := filepath.Join(dir, "out.json")
	if err := jar.Save(out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	saved, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var kept []persisted
	if err := json.Unmarshal(saved, &kept); err != nil {
		t.Fatalf("saved jar is not valid JSON: %v", err)
	}
	if len(kept) > 3000 {
		t.Fatalf("jar holds %d cookies after Load, want at most 3000 total", len(kept))
	}
	var hostA int
	for _, c := range kept {
		if c.Domain == "a.example" {
			hostA++
		}
	}
	if hostA > 50 {
		t.Fatalf("host a.example holds %d cookies, want at most 50", hostA)
	}
}
