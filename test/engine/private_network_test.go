package engine_test

import (
	"sync/atomic"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// localAuthorities are the spellings of "an address that only exists on the
// viewer's machine or network". A public page that names one is not asking for a
// stylesheet or a picture: it is asking the browser to reach into a network the
// viewer happens to be attached to, which is where cloud metadata services
// (169.254.169.254), routers (192.168.x.x, printer.local) and development
// servers (127.0.0.1:8080) live.
//
// The base for every case is a plain http:// public origin on purpose. Under an
// https:// base, an http:// subresource is already refused as mixed content, and a
// test that passed for that reason would not be testing this rule.
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

// TestLocalPageCanReachLocalNetwork covers the other half of the rule, and the
// reason the rule is a policy about the initiator rather than a blocklist of
// addresses: a page that is itself served from the machine or the private network
// is entitled to use that network. Blocking 127.0.0.1 outright would break local
// development and every test that drives the fetcher with a loopback server.
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

// TestPublicPageCanReachPublicNetwork is the non-vacuity check for the whole
// rule: an over-broad classifier that refused every address would fail here
// rather than turning the browser into a page that loads nothing.
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
