package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
)

func isJourneyNodeFocused(nodes []ax.Node, id string) bool {
	node, ok := findAXNode(nodes, id)
	return ok && node.Focused
}

// qc:BR-105
func TestJourneyFormValidationBlocksRequestUntilValid(t *testing.T) {
	var reqMu sync.Mutex
	var pageRequests, searchRequests int
	var method, path, rawQuery string
	var payload url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqMu.Lock()
		defer reqMu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/form":
			pageRequests++
			fmt.Fprint(w, `<!doctype html><html><body>
<form id="search" action="/search" method="GET">
  <input type="text" id="q" name="q" required>
  <input type="text" id="other" name="other" value="intact">
  <button type="submit" id="submit" name="intent" value="send">Search</button>
</form>
</body></html>`)
		case "/search":
			searchRequests++
			method, path, rawQuery, payload = r.Method, r.URL.Path, r.URL.RawQuery, r.URL.Query()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="results">Results</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/form"
	browser.navigateViaAddressBar(t, pageURL)
	browser.waitFor(t, "form loaded from the test server", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "submit")
		reqMu.Lock()
		defer reqMu.Unlock()
		return ok && browser.activeURL() == pageURL && pageRequests == 1
	})

	// The first real pointer click must reach the engine's validation path, focus
	// the empty required control, and leave the loaded page as the only request.
	browser.clickElementByID(t, "submit")
	browser.waitFor(t, "invalid click is handled by validation or a navigation occurs", func() bool {
		tab := browser.f.tabMgr.Active()
		if tab != nil && tab.Session() != nil && tab.Session().Focused() != nil && isJourneyNodeFocused(browser.window.AccessibilityTree(), "q") {
			return true
		}
		reqMu.Lock()
		defer reqMu.Unlock()
		return pageRequests != 1 || searchRequests != 0 || browser.activeURL() != pageURL
	})
	reqMu.Lock()
	gotPageRequests, gotSearchRequests := pageRequests, searchRequests
	reqMu.Unlock()
	if gotPageRequests != 1 || gotSearchRequests != 0 {
		t.Fatalf("requests after invalid submit = page:%d search:%d, want page:1 search:0", gotPageRequests, gotSearchRequests)
	}
	if got := browser.activeURL(); got != pageURL {
		t.Fatalf("URL after invalid submit = %q, want unchanged %q", got, pageURL)
	}
	if got := browser.f.tabMgr.Active().Session().FocusedText(); got != "" {
		t.Fatalf("invalid required value = %q, want preserved empty value", got)
	}

	// Type into the focused control, then activate the same submitter again.
	for _, ch := range "ready" {
		browser.pressKey(t, ch)
	}
	browser.waitFor(t, "required value updated through keyboard input", func() bool {
		sess := browser.f.tabMgr.Active().Session()
		return sess.Focused() != nil && sess.FocusedText() == "ready"
	})
	browser.clickElementByID(t, "submit")
	browser.waitFor(t, "one valid form request reaches the server", func() bool {
		reqMu.Lock()
		defer reqMu.Unlock()
		_, ok := findAXNode(browser.window.AccessibilityTree(), "results")
		return ok && searchRequests == 1 && browser.activeURL() == server.URL+"/search?q=ready&other=intact&intent=send"
	})

	reqMu.Lock()
	gotPageRequests, gotSearchRequests = pageRequests, searchRequests
	gotMethod, gotPath, gotRawQuery := method, path, rawQuery
	gotPayload := payload
	reqMu.Unlock()
	if gotPageRequests != 1 || gotSearchRequests != 1 {
		t.Fatalf("requests after valid submit = page:%d search:%d, want page:1 search:1", gotPageRequests, gotSearchRequests)
	}
	if gotMethod != http.MethodGet || gotPath != "/search" {
		t.Errorf("valid request = %s %s, want GET /search", gotMethod, gotPath)
	}
	if gotRawQuery != "q=ready&other=intact&intent=send" {
		t.Errorf("valid request query = %q, want q=ready&other=intact&intent=send", gotRawQuery)
	}
	for key, want := range map[string]string{"q": "ready", "other": "intact", "intent": "send"} {
		if got := gotPayload.Get(key); got != want {
			t.Errorf("request parameter %q = %q, want %q", key, got, want)
		}
	}
}
