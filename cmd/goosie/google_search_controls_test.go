package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func journeyTextField(nodes []ax.Node, label string) (ax.Node, bool) {
	for _, node := range nodes {
		if node.Role == ax.RoleTextField && !node.AddressBar && node.Label == label {
			return node, true
		}
		if found, ok := journeyTextField(node.Children, label); ok {
			return found, true
		}
	}
	return ax.Node{}, false
}

func TestJourneyNamedFormAutofocusRoutesTypingToContent(t *testing.T) {
	var mu sync.Mutex
	var received url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/search" {
			mu.Lock()
			received = r.URL.Query()
			mu.Unlock()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="done">Results</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body>
<form name="f" action="/search" method="GET"><input name="q" title="Search"><button type="submit">Search</button></form>
<script>document.body.onload = function () { document.f.q.focus(); };</script>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)
	pageURL := server.URL + "/"
	browser.navigateViaAddressBar(t, pageURL)
	browser.waitFor(t, "named form query is named, focused, and owns keyboard routing", func() bool {
		field, ok := journeyTextField(browser.window.AccessibilityTree(), "Search")
		return ok && field.Focused && browser.f.toolbar.GetFocus() == toolbar.FocusNone && browser.activeURL() == pageURL
	})
	for _, key := range "goosie browser" {
		browser.pressKey(t, key)
	}
	browser.waitFor(t, "typing reaches the autofocused query control", func() bool {
		field, ok := journeyTextField(browser.window.AccessibilityTree(), "Search")
		return ok && field.Value == "goosie browser"
	})
	browser.pressKey(t, '\r')
	browser.waitFor(t, "submitting the autofocused form reaches the server", func() bool {
		mu.Lock()
		defer mu.Unlock()
		_, done := findAXNode(browser.window.AccessibilityTree(), "done")
		return done && received.Get("q") == "goosie browser" && strings.Contains(browser.activeURL(), "/search?q=goosie+browser")
	})
}
