package main

import (
	"context"
	"fmt"
	"io"
	stdnet "net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/raster"
)

type recordingClient struct {
	mu    sync.Mutex
	ctxes map[string]context.Context
	docs  map[string]string
}

func (c *recordingClient) Get(ctx context.Context, u string) (*net.Response, error) {
	body, ok := c.docs[u]
	if !ok {
		return nil, fmt.Errorf("recordingClient: no fixture for %q", u)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctxes == nil {
		c.ctxes = map[string]context.Context{}
	}
	c.ctxes[u] = ctx
	contentType := "text/html"
	if strings.HasSuffix(u, ".css") {
		contentType = "text/css"
	}
	return &net.Response{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": contentType},
		Body:       []byte(body),
		URL:        u,
	}, nil
}

func (c *recordingClient) Post(ctx context.Context, u, contentType string, body io.Reader) (*net.Response, error) {
	return nil, fmt.Errorf("recordingClient: Post(%s) is not part of a page load", u)
}

func (c *recordingClient) SetTimeout(d time.Duration)  {}
func (c *recordingClient) Close() error                { return nil }
func (c *recordingClient) Jar() *net.CookieJar         { return nil }
func (c *recordingClient) NetworkLog() *net.NetworkLog { return nil }

const docWithSheet = `<!DOCTYPE html><html><head><link rel="stylesheet" href="/style.css"></head><body>hello</body></html>`

func loadFixture(t *testing.T, docURL, sheetURL string, docs map[string]string) *recordingClient {
	t.Helper()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	c := &recordingClient{docs: docs}
	if _, _, _, _, _, err := loadURLCtx(context.Background(), c, fonts, docURL, 800, 600, 1, t.TempDir(), false, false); err != nil {
		t.Fatalf("loadURLCtx(%s): %v", docURL, err)
	}
	if _, ok := c.ctxes[sheetURL]; !ok {
		t.Fatalf("%s was never fetched, so this load did not exercise a subresource at all", sheetURL)
	}
	return c
}

func TestPublicPageSubresourcesAreAddressGuarded(t *testing.T) {
	const doc = "http://public.example/index.html"
	const sheet = "http://public.example/style.css"
	c := loadFixture(t, doc, sheet, map[string]string{
		doc:   docWithSheet,
		sheet: "body { color: red }",
	})

	guard, guarded := net.AddressGuardFromContext(c.ctxes[doc])
	if !guarded {
		t.Fatal("the public document fetch was unguarded, so a redirect could reach a loopback, LAN, or metadata address")
	}
	if guard.Blocked == nil {
		t.Fatal("the document guard carries no predicate")
	}
	for ip, want := range map[string]bool{
		"169.254.169.254": true,
		"127.0.0.1":       true,
		"10.0.0.5":        true,
		"93.184.216.34":   false,
		"8.8.8.8":         false,
	} {
		if got := guard.Blocked(stdnet.ParseIP(ip)); got != want {
			t.Errorf("the document guard.Blocked(%s) = %v, want %v", ip, got, want)
		}
	}

	guard, guarded = net.AddressGuardFromContext(c.ctxes[sheet])
	if !guarded {
		t.Fatal("a public page's stylesheet was fetched unguarded, so a rebinding host name in an href reaches the viewer's machine")
	}
	if guard.Blocked == nil {
		t.Fatal("the guard carries no predicate")
	}
	for ip, want := range map[string]bool{
		"169.254.169.254": true,
		"127.0.0.1":       true,
		"10.0.0.5":        true,
		"93.184.216.34":   false,
		"8.8.8.8":         false,
	} {
		if got := guard.Blocked(stdnet.ParseIP(ip)); got != want {
			t.Errorf("the wired guard.Blocked(%s) = %v, want %v", ip, got, want)
		}
	}
	if guard.Lookup != nil {
		t.Error("the wired guard overrides the resolver, which is a test seam and not a browser setting")
	}
}

func TestLocalPageSubresourcesAreNotGuarded(t *testing.T) {
	const doc = "http://127.0.0.1:8080/index.html"
	const sheet = "http://127.0.0.1:8080/style.css"
	c := loadFixture(t, doc, sheet, map[string]string{
		doc:   docWithSheet,
		sheet: "body { color: red }",
	})
	if _, guarded := net.AddressGuardFromContext(c.ctxes[sheet]); guarded {
		t.Error("a local page's loopback stylesheet was guarded, so it would refuse to load its own CSS")
	}
}
