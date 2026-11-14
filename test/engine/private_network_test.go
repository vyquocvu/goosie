package engine_test

import (
	"sync/atomic"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

var localAuthorities = []string{
	"127.0.0.1",
	"127.0.0.1:8080",
	"0.0.0.0",
	"0.1.2.3",
	"10.1.2.3",
	"172.16.0.1",
	"192.168.0.1",
	"169.254.169.254",
	"100.64.0.1",
	"localhost",
	"localhost:3000",
	"db.internal",
	"printer.local",
	"home.home.arpa",
	"[::1]",
	"[fd00::1]",
	"[fe80::1]",
	"[ff12::1]",
	"[ff11::1]",
	"[::ffff:127.0.0.1]",
}

// qc:BR-159
func TestPublicPageCannotFetchLocalNetworkImage(t *testing.T) {
	for _, authority := range localAuthorities {
		for _, scheme := range []string{"http", "https"} {
			name := scheme + "://" + authority
			t.Run(name, func(t *testing.T) {
				var fetched int64
				fetcher := func(base, url string) ([]byte, error) {
					atomic.AddInt64(&fetched, 1)
					return nil, nil
				}
				html := `<img src="` + name + `/img.png">`
				sess, err := engine.NewSession(html, nil, 400,
					engine.WithImages("http://public.example.com/page", fetcher))
				if err != nil {
					t.Fatal(err)
				}
				if n := atomic.LoadInt64(&fetched); n != 0 {
					t.Fatalf("a public page fetched the local subresource %s %d times, want 0", name, n)
				}
				_ = sess
			})
		}
	}
}

// qc:BR-159
func TestPublicPageCannotFetchLocalNetworkStylesheet(t *testing.T) {
	for _, authority := range localAuthorities {
		t.Run(authority, func(t *testing.T) {
			var fetched int64
			linker := func(base, href string) (string, error) {
				atomic.AddInt64(&fetched, 1)
				return "", nil
			}
			html := `<link rel="stylesheet" href="http://` + authority + `/x.css">`
			sess, err := engine.NewSession(html, nil, 400,
				engine.WithLinkedCSS("http://public.example.com/page", linker))
			if err != nil {
				t.Fatal(err)
			}
			if n := atomic.LoadInt64(&fetched); n != 0 {
				t.Fatalf("a public page fetched the local stylesheet %s %d times, want 0", authority, n)
			}
			_ = sess
		})
	}
}

// qc:BR-159
func TestPublicPageCannotFetchLocalNetworkScript(t *testing.T) {
	for _, authority := range localAuthorities {
		t.Run(authority, func(t *testing.T) {
			var fetched int64
			fetcher := func(base, url string) ([]byte, error) {
				atomic.AddInt64(&fetched, 1)
				return []byte(`window.__localScriptRan = true;`), nil
			}
			html := `<script src="http://` + authority + `/evil.js"></script>`
			if _, err := engine.NewSession(html, nil, 400,
				engine.WithScripts("http://public.example.com/page", fetcher)); err != nil {
				t.Fatal(err)
			}
			if n := atomic.LoadInt64(&fetched); n != 0 {
				t.Fatalf("a public page fetched the local script %s %d times, want 0", authority, n)
			}
		})
	}
}

func TestLocalPageCanReachLocalNetwork(t *testing.T) {
	cases := []struct {
		name string
		base string
		src  string
	}{
		{"loopback page from loopback", "http://127.0.0.1:8080/page", "http://127.0.0.1:8080/img.png"},
		{"loopback page from private", "http://localhost:3000/", "http://10.1.2.3/img.png"},
		{"private page from loopback", "http://192.168.1.10/", "http://127.0.0.1:8080/img.png"},
		{"local file page from loopback", "file:///Users/me/notes/index.html", "http://127.0.0.1:8080/img.png"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var fetched int64
			fetcher := func(base, url string) ([]byte, error) {
				atomic.AddInt64(&fetched, 1)
				return nil, nil
			}
			html := `<img src="` + tc.src + `">`
			if _, err := engine.NewSession(html, nil, 400,
				engine.WithImages(tc.base, fetcher)); err != nil {
				t.Fatal(err)
			}
			if n := atomic.LoadInt64(&fetched); n != 1 {
				t.Fatalf("%s fetched %d times from base %s, want 1", tc.src, n, tc.base)
			}
		})
	}
}

func TestPublicPageCanReachPublicNetwork(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"host", "https://cdn.example.com/img.png"},
		{"bare domain", "https://example.org/img.png"},
		{"numeric public address", "https://93.184.216.34/img.png"},
		{"shared address space as a label", "https://cgnat100.example.com/img.png"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var fetched int64
			fetcher := func(base, url string) ([]byte, error) {
				atomic.AddInt64(&fetched, 1)
				return nil, nil
			}
			html := `<img src="` + tc.src + `">`
			if _, err := engine.NewSession(html, nil, 400,
				engine.WithImages("http://public.example.com/page", fetcher)); err != nil {
				t.Fatal(err)
			}
			if n := atomic.LoadInt64(&fetched); n != 1 {
				t.Fatalf("%s fetched %d times, want 1", tc.src, n)
			}
		})
	}
}
