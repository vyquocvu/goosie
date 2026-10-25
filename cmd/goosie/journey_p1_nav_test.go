package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// qc:BR-004
func TestJourneyURLWithPathQueryFragment(t *testing.T) {
	var receivedPath, receivedQuery string
	var reqMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqMu.Lock()
		receivedPath = r.URL.Path
		receivedQuery = r.URL.RawQuery
		reqMu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><body>
<div style="height: 1000px;">Spacing</div>
<h1 id="target-frag">Target Content</h1>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	targetURL := server.URL + "/search/items?category=books&sort=desc#target-frag"
	browser.navigateViaAddressBar(t, targetURL)

	browser.waitFor(t, "url with path query and fragment loaded", func() bool {
		reqMu.Lock()
		path, query := receivedPath, receivedQuery
		reqMu.Unlock()
		_, ok := findAXNode(browser.window.AccessibilityTree(), "target-frag")
		return ok && browser.activeURL() == targetURL && path == "/search/items" && query == "category=books&sort=desc"
	})

	// Post-condition assertions:
	// 1. HTTP request sent to server preserved path and query, but fragment was not sent over wire
	reqMu.Lock()
	gotPath, gotQuery := receivedPath, receivedQuery
	reqMu.Unlock()
	if gotPath != "/search/items" {
		t.Errorf("request path = %q, want /search/items", gotPath)
	}
	if gotQuery != "category=books&sort=desc" {
		t.Errorf("request query = %q, want category=books&sort=desc", gotQuery)
	}

	// 2. Active URL and address bar in browser retain the full URL with fragment
	if browser.activeURL() != targetURL {
		t.Errorf("activeURL = %q, want %q", browser.activeURL(), targetURL)
	}
	addrNode, ok := findAXNode(browser.window.AccessibilityTree(), "goosie.address")
	if !ok || addrNode.Value != targetURL {
		t.Errorf("address bar value = %q, want %q (found: %v)", addrNode.Value, targetURL, ok)
	}

	// 3. Document retained fragment and target element exists in AX tree
	if _, ok := findAXNode(browser.window.AccessibilityTree(), "target-frag"); !ok {
		t.Errorf("AX node 'target-frag' missing; %s", browser.diagnostics())
	}
}

// qc:BR-008
func TestJourneyRedirectAndBack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/start":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="start-heading">Initial Origin</h1></body></html>`)
		case "/redirect":
			http.Redirect(w, r, "/destination", http.StatusFound)
		case "/destination":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="dest-heading">Destination Page</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	startURL := server.URL + "/start"
	redirectURL := server.URL + "/redirect"
	destURL := server.URL + "/destination"

	// 1. Navigate to initial page
	browser.navigateViaAddressBar(t, startURL)
	browser.waitFor(t, "initial page loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "start-heading")
		return ok && browser.activeURL() == startURL
	})

	// 2. Navigate to redirecting page
	browser.navigateViaAddressBar(t, redirectURL)
	browser.waitFor(t, "redirect followed to destination", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "dest-heading")
		return ok && browser.activeURL() == destURL
	})

	// Assert address bar displays final destination URL, not requested redirect URL
	addrNode, ok := findAXNode(browser.window.AccessibilityTree(), "goosie.address")
	if !ok || addrNode.Value != destURL {
		t.Errorf("address bar = %q, want %q", addrNode.Value, destURL)
	}

	// 3. User clicks Back
	browser.clickToolbarButton(t, toolbar.ButtonBack)

	// 4. Back reaches start page without looping back to redirect
	browser.waitFor(t, "back returns to initial page without loop", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil {
			return false
		}
		_, ok := findAXNode(browser.window.AccessibilityTree(), "start-heading")
		return ok && browser.activeURL() == startURL && !active.History().CanBack() && active.History().CanForward()
	})
}

// qc:BR-013
func TestJourneySlowPageStopKeepsDocument(t *testing.T) {
	reqStarted := make(chan struct{}, 1)
	unblockSlow := make(chan struct{})
	defer close(unblockSlow)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/initial":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="init-heading">Initial Page</h1></body></html>`)
		case "/slow":
			select {
			case reqStarted <- struct{}{}:
			default:
			}
			select {
			case <-r.Context().Done():
			case <-unblockSlow:
				fmt.Fprint(w, `<!doctype html><html><body><h1 id="slow-heading">Slow Page Loaded</h1></body></html>`)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	initialURL := server.URL + "/initial"
	slowURL := server.URL + "/slow"

	// 1. Load initial page
	browser.navigateViaAddressBar(t, initialURL)
	browser.waitFor(t, "initial page loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "init-heading")
		return ok && browser.activeURL() == initialURL
	})

	// 2. Navigate to slow page
	browser.navigateViaAddressBar(t, slowURL)

	// Wait until the slow request starts in the background and tab is loading
	browser.waitFor(t, "slow navigation in flight", func() bool {
		select {
		case <-reqStarted:
			return true
		default:
			return false
		}
	})

	// 3. User clicks Stop button (which is ButtonReload while loading)
	browser.clickToolbarButton(t, toolbar.ButtonReload)

	// 4. Assert tab stops loading and currently displayed document is kept
	browser.waitFor(t, "stop completes and keeps displayed document", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil || active.Loading() {
			return false
		}
		_, ok := findAXNode(browser.window.AccessibilityTree(), "init-heading")
		return ok && browser.activeURL() == initialURL && active.Error() == ""
	})
}

// qc:BR-015
func TestJourneyLinkNavigationInSameTab(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/link-start":
			fmt.Fprint(w, `<!doctype html><html><body><a id="nav-link" href="/link-dest">Click to Advance</a></body></html>`)
		case "/link-dest":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="dest-head">Destination Landed</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	startURL := server.URL + "/link-start"
	destURL := server.URL + "/link-dest"

	browser.navigateViaAddressBar(t, startURL)
	browser.waitFor(t, "start page loaded with link", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "nav-link")
		return ok && browser.activeURL() == startURL
	})

	// Click link to advance
	browser.clickNode(t, "nav-link")

	// Post-condition: URL, document, and history synchronize exactly once, with no duplicate or spurious entries
	browser.waitFor(t, "destination loaded and history synchronized", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil {
			return false
		}
		entries, idx := active.History().Entries()
		_, ok := findAXNode(browser.window.AccessibilityTree(), "dest-head")
		return ok && browser.activeURL() == destURL && len(entries) == 2 && idx == 1 && entries[0] == startURL && entries[1] == destURL
	})

	active := browser.f.tabMgr.Active()
	if !active.History().CanBack() {
		t.Errorf("CanBack() = false, want true after link navigation")
	}
	if active.History().CanForward() {
		t.Errorf("CanForward() = true, want false at history head")
	}
}

// qc:BR-016
func TestJourneyAnchorNavigation(t *testing.T) {
	var pageURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/page":
			fmt.Fprint(w, `<!doctype html><html><body style="margin:0;padding:0;">
<a id="same-anchor" href="#bottom-target">Jump Down</a>
<div style="height: 1500px;">Spacer Content</div>
<h2 id="bottom-target">Bottom Target Content</h2>
</body></html>`)
		case "/other":
			fmt.Fprintf(w, `<!doctype html><html><body style="margin:0;padding:0;">
<a id="cross-anchor" href="%s/page#bottom-target">Jump to Target from Other</a>
</body></html>`, pageURL)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	pageURL = server.URL

	browser := newBrowserE2E(t)
	defer browser.close(t)

	docURL := server.URL + "/page"
	otherURL := server.URL + "/other"
	targetURL := docURL + "#bottom-target"

	// 1. Same-page anchor jump
	browser.navigateViaAddressBar(t, docURL)
	browser.waitFor(t, "same-page anchor document loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "same-anchor")
		return ok && browser.activeURL() == docURL
	})

	browser.clickNode(t, "same-anchor")
	browser.waitFor(t, "same-page anchor jump updates fragment and scrolls viewport", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil {
			return false
		}
		return browser.activeURL() == targetURL && active.ScrollY() > 0
	})

	// 2. Cross-page anchor jump from another document
	browser.navigateViaAddressBar(t, otherURL)
	browser.waitFor(t, "other document loaded with cross anchor link", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "cross-anchor")
		return ok && browser.activeURL() == otherURL
	})

	browser.clickNode(t, "cross-anchor")
	browser.waitFor(t, "cross-page anchor jump loads page, sets fragment and scrolls viewport", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil {
			return false
		}
		_, ok := findAXNode(browser.window.AccessibilityTree(), "bottom-target")
		return ok && browser.activeURL() == targetURL && active.ScrollY() > 0
	})
}

// qc:BR-020
func TestJourneyReloadPageWithQueryFragment(t *testing.T) {
	var renderMu sync.Mutex
	renderCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		renderMu.Lock()
		renderCount++
		current := renderCount
		renderMu.Unlock()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><html><body>
<h1 id="count-head">Render Count: %d</h1>
<div id="frag-target">Target</div>
</body></html>`, current)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	targetURL := server.URL + "/report?mode=summary&sort=asc#frag-target"
	browser.navigateViaAddressBar(t, targetURL)

	browser.waitFor(t, "initial page with query and fragment loaded", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil {
			return false
		}
		entries, _ := active.History().Entries()
		return browser.activeURL() == targetURL &&
			axContainsLabel(browser.window.AccessibilityTree(), "Render Count: 1") &&
			len(entries) == 1
	})

	// User clicks reload
	browser.clickToolbarButton(t, toolbar.ButtonReload)

	// Post-condition: reloaded URL keeps query and fragment, history retains exactly 1 entry (no fake entry added)
	browser.waitFor(t, "reloaded page with query and fragment preserves history", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil {
			return false
		}
		entries, _ := active.History().Entries()
		return browser.activeURL() == targetURL &&
			axContainsLabel(browser.window.AccessibilityTree(), "Render Count: 2") &&
			len(entries) == 1
	})
}

// qc:BR-021
func TestJourneyRefreshWithPendingResponse(t *testing.T) {
	req1Started := make(chan struct{}, 1)
	req1Release := make(chan struct{})
	var releaseOnce sync.Once
	doRelease := func() {
		releaseOnce.Do(func() {
			close(req1Release)
		})
	}
	defer doRelease()

	var reqCount int
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqCount++
		num := reqCount
		mu.Unlock()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if num == 1 {
			select {
			case req1Started <- struct{}{}:
			default:
			}
			select {
			case <-r.Context().Done():
			case <-req1Release:
			}
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="resp1">First Stale Response</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body><h1 id="resp2">Second Fresh Response</h1></body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	targetURL := server.URL + "/supersede"

	// 1. Start first navigation
	browser.navigateViaAddressBar(t, targetURL)

	// Wait for request 1 to be in-flight
	browser.waitFor(t, "first request pending", func() bool {
		select {
		case <-req1Started:
			return true
		default:
			return false
		}
	})

	// 2. Trigger new navigation/refresh while request 1 is still pending
	browser.navigateViaAddressBar(t, targetURL)

	// Wait for second fresh navigation result to be applied
	browser.waitFor(t, "second fresh navigation result applied", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil || active.Loading() {
			return false
		}
		_, ok := findAXNode(browser.window.AccessibilityTree(), "resp2")
		return ok && browser.activeURL() == targetURL
	})

	// Release request 1 so its goroutine completes
	doRelease()

	// Post-condition: stale response 1 must never be applied
	if _, ok := findAXNode(browser.window.AccessibilityTree(), "resp1"); ok {
		t.Errorf("stale response 1 was applied over newer response 2; %s", browser.diagnostics())
	}
}

// qc:BR-022
func TestJourneyAddressBarFocusTypingDoesNotLeakToDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/first":
			fmt.Fprint(w, `<!doctype html><html><body>
<label for="doc-text">Notes</label>
<input id="doc-text" name="notes" placeholder="Type here">
</body></html>`)
		case "/second":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="second-page">Second Page Landed</h1></body></html>`)
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
	browser.waitFor(t, "first page loaded with input", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "doc-text")
		return ok && browser.activeURL() == firstURL
	})

	// 1. Focus the document input element
	browser.clickNode(t, "doc-text")
	browser.waitFor(t, "doc input focused", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "doc-text")
		return ok && node.Focused
	})

	// 2. Focus address bar via Cmd+L
	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 'l', Mods: surface.ModCommand})
	browser.waitFor(t, "address bar focused after Cmd+L", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "goosie.address")
		return ok && node.Focused
	})

	// 3. Type new URL by keyboard events
	for _, ch := range secondURL {
		browser.pressKey(t, ch)
	}
	browser.pressKey(t, '\r')

	// 4. Second page loads
	browser.waitFor(t, "second page loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "second-page")
		return ok && browser.activeURL() == secondURL
	})

	// 5. Navigate Back to first page and verify no typed characters leaked into doc input
	browser.clickToolbarButton(t, toolbar.ButtonBack)
	browser.waitFor(t, "returned to first page", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "doc-text")
		return ok && browser.activeURL() == firstURL && node.Value == ""
	})

	inputNode, ok := findAXNode(browser.window.AccessibilityTree(), "doc-text")
	if !ok || inputNode.Value != "" {
		t.Errorf("document input value leaked address bar typing: got %q, want empty", inputNode.Value)
	}
}
