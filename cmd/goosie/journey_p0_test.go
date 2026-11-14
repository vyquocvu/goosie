package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gonet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// qc:BR-001
func TestJourneyHTTPSNavigation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><title>Secure Origin</title></head><body><h1 id="secure-heading">HTTPS Success</h1><p id="desc">Secure content</p></body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	browser.f.client = gonet.DefaultClientWithTLS(server.Client().Transport.(*http.Transport).TLSClientConfig)

	targetURL := server.URL + "/secure"
	browser.navigateViaAddressBar(t, targetURL)

	browser.waitFor(t, "https page loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "secure-heading")
		return ok && browser.activeURL() == targetURL
	})

	if !strings.HasPrefix(browser.activeURL(), "https://") {
		t.Errorf("active URL = %q, want https:// scheme", browser.activeURL())
	}
	addrNode, ok := findAXNode(browser.window.AccessibilityTree(), "goosie.address")
	if !ok || addrNode.Value != targetURL {
		t.Errorf("address bar AX node value = %q, want %q (found: %v)", addrNode.Value, targetURL, ok)
	}
	if tab := browser.f.tabMgr.Active(); tab.Error() != "" {
		t.Errorf("tab error = %q, want empty", tab.Error())
	}
	if tab := browser.f.tabMgr.Active(); tab.Title() != "Secure Origin" {
		t.Errorf("tab title = %q, want 'Secure Origin'", tab.Title())
	}
}

// qc:BR-002
func TestJourneyHTTPNavigationAndRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/source":
			http.Redirect(w, r, "/destination", http.StatusFound)
		case "/destination":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, `<!doctype html><html><head><title>Destination</title></head><body><h1 id="dest-head">Destination Reached</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	sourceURL := server.URL + "/source"
	destURL := server.URL + "/destination"

	browser.navigateViaAddressBar(t, sourceURL)

	browser.waitFor(t, "redirect destination loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "dest-head")
		return ok && browser.activeURL() == destURL
	})

	if !strings.HasPrefix(browser.activeURL(), "http://") {
		t.Errorf("active URL = %q, want http:// scheme", browser.activeURL())
	}
	addrNode, ok := findAXNode(browser.window.AccessibilityTree(), "goosie.address")
	if !ok || addrNode.Value != destURL {
		t.Errorf("address bar AX node value = %q, want %q", addrNode.Value, destURL)
	}
	if !axContainsLabel(browser.window.AccessibilityTree(), "Destination Reached") {
		t.Errorf("AX tree missing destination heading text; %s", browser.diagnostics())
	}
}

// qc:BR-003
func TestJourneyBareHostnameNormalization(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><body><h1 id="bare-doc">Bare Host Reached</h1></body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	browser.f.client = gonet.DefaultClientWithTLS(server.Client().Transport.(*http.Transport).TLSClientConfig)

	bareHost := strings.TrimPrefix(server.URL, "https://")
	bareInput := bareHost + "/bare"
	expectedURL := "https://" + bareInput

	browser.navigateViaAddressBar(t, bareInput)

	browser.waitFor(t, "bare hostname normalized and loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "bare-doc")
		return ok && browser.activeURL() == expectedURL
	})

	addrNode, ok := findAXNode(browser.window.AccessibilityTree(), "goosie.address")
	if !ok || addrNode.Value != expectedURL {
		t.Errorf("address bar AX node value = %q, want %q", addrNode.Value, expectedURL)
	}
}

// qc:BR-014
func TestJourneyStaleNavigationDoesNotOverwriteCurrent(t *testing.T) {
	slowEntered := make(chan struct{})
	releaseSlow := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/slow":
			close(slowEntered)
			<-releaseSlow
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="slow-doc">Slow Document</h1></body></html>`)
		case "/fast":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="fast-doc">Fast Document</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	slowURL := server.URL + "/slow"
	fastURL := server.URL + "/fast"

	browser.navigateViaAddressBar(t, slowURL)

	select {
	case <-slowEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for slow request to reach server")
	}

	browser.navigateViaAddressBar(t, fastURL)

	browser.waitFor(t, "fast document loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "fast-doc")
		return ok && browser.activeURL() == fastURL
	})

	close(releaseSlow)

	browser.waitFor(t, "stale slow result is ignored and fast doc remains", func() bool {
		_, fastPresent := findAXNode(browser.window.AccessibilityTree(), "fast-doc")
		_, slowPresent := findAXNode(browser.window.AccessibilityTree(), "slow-doc")
		return fastPresent && !slowPresent && browser.activeURL() == fastURL
	})

	if browser.activeURL() != fastURL {
		t.Errorf("active URL = %q, want %q (slow navigation overwrote fast!)", browser.activeURL(), fastURL)
	}
	addrNode, ok := findAXNode(browser.window.AccessibilityTree(), "goosie.address")
	if !ok || addrNode.Value != fastURL {
		t.Errorf("address bar AX node value = %q, want %q", addrNode.Value, fastURL)
	}
}

// qc:BR-017
func TestJourneyBackForwardFiveNavigations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		step := strings.TrimPrefix(r.URL.Path, "/page")
		fmt.Fprintf(w, `<!doctype html><html><head><title>Page %s</title></head><body><h1 id="page-%s">Page %s Heading</h1></body></html>`, step, step, step)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	urls := make([]string, 5)
	for i := 0; i < 5; i++ {
		urls[i] = fmt.Sprintf("%s/page%d", server.URL, i+1)
	}

	for i, u := range urls {
		pageID := fmt.Sprintf("page-%d", i+1)
		browser.navigateViaAddressBar(t, u)
		browser.waitFor(t, fmt.Sprintf("page %d loaded", i+1), func() bool {
			_, ok := findAXNode(browser.window.AccessibilityTree(), pageID)
			return ok && browser.activeURL() == u
		})
	}

	entries, current := browser.f.tabMgr.Active().History().Entries()
	if len(entries) != 5 || current != 4 {
		t.Fatalf("history entries = %v (current = %d), want 5 entries at index 4", entries, current)
	}

	for step := 4; step >= 1; step-- {
		browser.clickToolbarButton(t, toolbar.ButtonBack)
		wantURL := urls[step-1]
		pageID := fmt.Sprintf("page-%d", step)
		browser.waitFor(t, fmt.Sprintf("back navigation to page %d", step), func() bool {
			_, ok := findAXNode(browser.window.AccessibilityTree(), pageID)
			return ok && browser.activeURL() == wantURL
		})
	}

	if browser.f.tabMgr.Active().History().CanBack() {
		t.Errorf("at page 1, CanBack = true, want false")
	}

	for step := 2; step <= 5; step++ {
		browser.clickToolbarButton(t, toolbar.ButtonForward)
		wantURL := urls[step-1]
		pageID := fmt.Sprintf("page-%d", step)
		browser.waitFor(t, fmt.Sprintf("forward navigation to page %d", step), func() bool {
			_, ok := findAXNode(browser.window.AccessibilityTree(), pageID)
			return ok && browser.activeURL() == wantURL
		})
	}

	if browser.f.tabMgr.Active().History().CanForward() {
		t.Errorf("at page 5, CanForward = true, want false")
	}
}

// qc:BR-125
func TestJourneyCmdLFocusesAddressBar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/first":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="first">First</h1></body></html>`)
		case "/second":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="second">Second</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	firstURL := server.URL + "/first"
	secondURL := server.URL + "/second"

	browser.navigateViaAddressBar(t, firstURL)
	browser.waitFor(t, "first page loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "first")
		return ok && browser.activeURL() == firstURL
	})

	browser.pressKey(t, 0x1b)
	browser.waitFor(t, "address bar loses focus after Escape", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "goosie.address")
		return ok && !node.Focused
	})

	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 'l', Mods: surface.ModCommand})

	browser.waitFor(t, "address bar focused after Cmd+L", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "goosie.address")
		return ok && node.Focused && node.AddressBar
	})

	browser.pressKey(t, 'x')
	browser.waitFor(t, "typing after Cmd+L replaces the selected URL", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "goosie.address")
		return ok && node.Focused && node.Value == "x"
	})

	browser.navigateViaAddressBar(t, secondURL)
	browser.waitFor(t, "second page loaded after address bar replacement", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "second")
		return ok && browser.activeURL() == secondURL
	})
}
