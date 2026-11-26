package net_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	transport "github.com/vyquocvu/goosie/internal/net"
)

func TestCookiePolicyControlsBufferedAndStreamingRequests(t *testing.T) {
	for _, mode := range []string{"buffered", "streaming"} {
		t.Run(mode, func(t *testing.T) {
			var mu sync.Mutex
			headers := map[string]string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				headers[r.URL.Path] = r.Header.Get("Cookie")
				mu.Unlock()
				w.Header().Set("Content-Type", "text/plain")
				switch r.URL.Path {
				case "/seed":
					w.Header().Set("Set-Cookie", "existing=one; Path=/")
				case "/denied":
					w.Header().Set("Set-Cookie", "denied=one; Path=/")
				case "/allowed":
					w.Header().Set("Set-Cookie", "allowed=one; Path=/")
				}
				_, _ = io.WriteString(w, r.URL.Path)
			}))
			defer server.Close()
			client := transport.DefaultClient()
			defer client.Close()
			if _, err := client.Get(context.Background(), server.URL+"/seed"); err != nil {
				t.Fatal(err)
			}

			denied := transport.WithCookiePolicy(context.Background(), func(*url.URL) bool { return false })
			if mode == "streaming" {
				resp, err := client.(transport.StreamingHTTP).GetStream(denied, server.URL+"/denied")
				if err != nil {
					t.Fatal(err)
				}
				if err := resp.Close(); err != nil {
					t.Fatal(err)
				}
			} else if _, err := client.Get(denied, server.URL+"/denied"); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			gotDenied := headers["/denied"]
			mu.Unlock()
			if gotDenied != "" {
				t.Fatalf("denied %s request sent Cookie %q", mode, gotDenied)
			}
			if got := cookieValues(client.Jar().CookiesForHost(serverURLHostname(t, server.URL))); strings.Contains(got, "denied=one") {
				t.Fatalf("denied %s response stored a cookie: %s", mode, got)
			}

			allowed := transport.WithCookiePolicy(context.Background(), func(*url.URL) bool { return true })
			if mode == "streaming" {
				resp, err := client.(transport.StreamingHTTP).GetStream(allowed, server.URL+"/allowed")
				if err != nil {
					t.Fatal(err)
				}
				if err := resp.Close(); err != nil {
					t.Fatal(err)
				}
			} else if _, err := client.Get(allowed, server.URL+"/allowed"); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			gotAllowed := headers["/allowed"]
			mu.Unlock()
			if gotAllowed != "existing=one" {
				t.Fatalf("allowed %s request sent Cookie %q, want existing=one", mode, gotAllowed)
			}
			got := cookieValues(client.Jar().CookiesForHost(serverURLHostname(t, server.URL)))
			if !strings.Contains(got, "existing=one") || !strings.Contains(got, "allowed=one") {
				t.Fatalf("allowed %s response did not preserve and store cookies: %s", mode, got)
			}
		})
	}
}

func TestCookiePolicyBypassesResponseCache(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Cache-Control", "max-age=60")
		w.Header().Set("Set-Cookie", "fresh=one; Path=/")
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	client := transport.DefaultClient()
	defer client.Close()

	allowed := transport.WithCookiePolicy(context.Background(), func(*url.URL) bool { return true })
	if _, err := client.Get(allowed, server.URL); err != nil {
		t.Fatal(err)
	}
	denied := transport.WithCookiePolicy(context.Background(), func(*url.URL) bool { return false })
	if _, err := client.Get(denied, server.URL); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(allowed, server.URL); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("cookie-policy requests made %d network requests, want 3; cache reuse lost Set-Cookie processing", got)
	}
	if got := cookieValues(client.Jar().CookiesForHost(serverURLHostname(t, server.URL))); got != "fresh=one" {
		t.Fatalf("cookies after deny/reset = %q, want fresh=one", got)
	}
}

func TestCookiePolicyIsReevaluatedOnRedirects(t *testing.T) {
	for _, mode := range []string{"buffered", "streaming"} {
		t.Run(mode, func(t *testing.T) {
			var targetMu sync.Mutex
			var targetCookies []string
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/denied" || r.URL.Path == "/allowed" {
					targetMu.Lock()
					targetCookies = append(targetCookies, r.Header.Get("Cookie"))
					targetMu.Unlock()
				}
				switch r.URL.Path {
				case "/seed":
					w.Header().Set("Set-Cookie", "seed=one; Path=/")
				case "/denied":
					w.Header().Set("Set-Cookie", "denied=one; Path=/")
				case "/allowed":
					w.Header().Set("Set-Cookie", "allowed=one; Path=/")
				}
				_, _ = io.WriteString(w, "target")
			}))
			defer target.Close()
			targetURL := strings.Replace(target.URL, "127.0.0.1", "localhost", 1)
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Set-Cookie", "origin=one; Path=/")
				http.Redirect(w, r, targetURL+r.URL.Path, http.StatusFound)
			}))
			defer origin.Close()
			client := transport.DefaultClient()
			defer client.Close()
			if _, err := client.Get(context.Background(), targetURL+"/seed"); err != nil {
				t.Fatal(err)
			}

			var allowTarget atomic.Bool
			ctx := transport.WithCookiePolicy(context.Background(), func(u *url.URL) bool {
				return u.Host != serverURLHost(t, targetURL) || allowTarget.Load()
			})
			request := func(path string) {
				if mode == "streaming" {
					resp, err := client.(transport.StreamingHTTP).GetStream(ctx, origin.URL+path)
					if err != nil {
						t.Fatal(err)
					}
					if err := resp.Close(); err != nil {
						t.Fatal(err)
					}
					return
				}
				if _, err := client.Get(ctx, origin.URL+path); err != nil {
					t.Fatal(err)
				}
			}
			request("/denied")
			allowTarget.Store(true)
			request("/allowed")

			targetMu.Lock()
			got := append([]string(nil), targetCookies...)
			targetMu.Unlock()
			if len(got) != 2 || got[0] != "" || got[1] != "seed=one" {
				t.Fatalf("redirect target cookies = %q, want [\"\" \"seed=one\"]", got)
			}
			cookies := cookieValues(client.Jar().CookiesForHost(serverURLHostname(t, targetURL)))
			if !strings.Contains(cookies, "seed=one") || !strings.Contains(cookies, "allowed=one") || strings.Contains(cookies, "denied=one") {
				t.Fatalf("redirect cookie storage = %q", cookies)
			}
		})
	}
}

func TestResponsePreservesMultipleCSPHeaderValues(t *testing.T) {
	for _, mode := range []string{"buffered", "streaming"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Add("Content-Security-Policy", "script-src 'self'")
				w.Header().Add("Content-Security-Policy", "script-src https://scripts.example")
				w.Header().Add("Content-Security-Policy-Report-Only", "default-src 'none'")
				_, _ = io.WriteString(w, "ok")
			}))
			defer server.Close()
			client := transport.DefaultClient()
			defer client.Close()
			var headers map[string][]string
			if mode == "streaming" {
				resp, err := client.(transport.StreamingHTTP).GetStream(context.Background(), server.URL)
				if err != nil {
					t.Fatal(err)
				}
				headers = resp.HeaderValues
				if err := resp.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				resp, err := client.Get(context.Background(), server.URL)
				if err != nil {
					t.Fatal(err)
				}
				headers = resp.HeaderValues
			}
			values := headers["Content-Security-Policy"]
			if len(values) != 2 || values[0] != "script-src 'self'" || values[1] != "script-src https://scripts.example" {
				t.Fatalf(" enforcing CSP values = %q", values)
			}
			if values := headers["Content-Security-Policy-Report-Only"]; len(values) != 1 {
				t.Fatalf("report-only CSP values = %q", values)
			}
		})
	}
}

func cookieValues(cookies []transport.Cookie) string {
	values := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		values = append(values, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(values, "; ")
}

func serverURLHost(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

func serverURLHostname(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Hostname()
}
