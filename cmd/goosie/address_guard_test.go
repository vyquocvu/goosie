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

// loadURLCtx is where the browser's two halves meet: internal/engine owns the
// rule about which addresses a page may reach, internal/net owns the dial that
// has to honour it, and neither can see the other (the import table keeps
// internal/engine from importing internal/net so the engine stays mockable). The
// join is a context value handed over here, per fetch, and it is the only thing
// that makes the resolution-time rule hold against a name that resolves
// somewhere other than where it says.
//
// These tests read that join off the context a fetch arrived with, because the
// alternative - a real DNS answer pointing a public name at loopback - is not
// something a test can arrange without editing /etc/hosts.

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

func (c *recordingClient) SetTimeout(d time.Duration) {}
func (c *recordingClient) Close() error               { return nil }

// docWithSheet is the smallest document whose load fetches a second URL: the
// linked sheet is the subresource, and its href is relative, so the engine - not
// this file - resolves it against the document it was found in.
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

	// The user asked for this document, so nothing refuses the address it names
	// or the address it resolves to: that is how a browser reaches its own router.
	if _, guarded := net.AddressGuardFromContext(c.ctxes[doc]); guarded {
		t.Error("the document fetch was address-guarded, which would refuse a typed URL that resolves to a LAN address")
	}

	guard, guarded := net.AddressGuardFromContext(c.ctxes[sheet])
	if !guarded {
		t.Fatal("a public page's stylesheet was fetched unguarded, so a rebinding host name in an href reaches the viewer's machine")
	}
	if guard.Blocked == nil {
		t.Fatal("the guard carries no predicate")
	}
	// The predicate has to be the engine's rule rather than any predicate: this is
	// the one place the two are joined, and a guard wired to a stub here would
	// pass every internal/net test while refusing nothing real.
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
	// The lookup stays nil so the guard resolves through the system resolver:
	// overriding it here would replace the answer real DNS gives with a fixture.
	if guard.Lookup != nil {
		t.Error("the wired guard overrides the resolver, which is a test seam and not a browser setting")
	}
}

// TestLocalPageSubresourcesAreNotGuarded is the case the policy exists to
// protect: a development server on loopback loading its own sheet from loopback.
// Guarding it would break the main thing this browser is pointed at locally,
// which is what the engine's initiator rule is for.
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
