package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/preferences"
)

// TestJourneyAddressBarEncodesWithoutDialingASurprisingHost covers the two
// halves of an address-bar value that is not already a clean URL: surrounding
// whitespace is trimmed, and characters that cannot appear literally in the
// request path are encoded. The server's view of Host and RequestURI is the
// oracle - a page loaded from the typed host with an encoded path is
// normalization; a request that arrives somewhere else is the bug.
// qc:BR-006
func TestJourneyAddressBarEncodesWithoutDialingASurprisingHost(t *testing.T) {
	var mu sync.Mutex
	var requests []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Host+" "+r.URL.RequestURI())
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><title>Page</title></head><body><h1 id="page">Page</h1></body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	const rawPath = "/a b/đường"
	wantURL := server.URL + "/a%20b/%C4%91%C6%B0%E1%BB%9Dng"

	browser.navigateViaAddressBar(t, "   "+server.URL+rawPath+"   ")
	browser.waitFor(t, "trimmed and encoded URL loads from the typed host", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "page")
		return ok && browser.activeURL() == wantURL
	})

	mu.Lock()
	gotRequests := append([]string(nil), requests...)
	mu.Unlock()
	if len(gotRequests) != 1 {
		t.Fatalf("server saw %d requests, want 1: %v", len(gotRequests), gotRequests)
	}
	if want := u.Host + " /a%20b/%C4%91%C6%B0%E1%BB%9Dng"; gotRequests[0] != want {
		t.Fatalf("server saw %q, want %q", gotRequests[0], want)
	}

	// A percent sign that does not form an escape is a clear error, not a
	// request to a host or path the user did not mean.
	before := len(gotRequests)
	browser.navigateViaAddressBar(t, "   "+server.URL+"/100%   ")
	browser.waitFor(t, "invalid escape reports an error page", func() bool {
		active := browser.f.tabMgr.Active()
		return active != nil && active.Error() != "" && active.Title() == "Problem loading page"
	})
	mu.Lock()
	after := len(requests)
	mu.Unlock()
	if after != before {
		t.Fatalf("malformed escape produced %d additional requests: %v", after-before, requests[before:])
	}
}

// TestJourneyNonAllowlistedLinkSchemesSearchInsteadOfHandingOff is the click
// path for BR-168. Goosie has no native scheme handler to invoke; the danger is
// a link whose scheme the renderer would otherwise treat as a destination.
// Every one of them has to land in the browser's own HTTP search page, scoped
// by the profile template, with no request anywhere but that page.
// qc:BR-168
func TestJourneyNonAllowlistedLinkSchemesSearchInsteadOfHandingOff(t *testing.T) {
	var mu sync.Mutex
	var unexpected []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, `<!doctype html><html><body>
<h1 id="page">Links</h1>
<a id="mailto" href="mailto:someone@example.com">Mail</a>
<a id="javascript" href="javascript:document.body.setAttribute(&quot;data-pwned&quot;,&quot;1&quot;)">Script</a>
<a id="data" href="data:text/html,secret">Data</a>
<a id="ftp" href="ftp://files.example/private">FTP</a>
</body></html>`)
		case "/search":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="search">Search</h1></body></html>`)
		default:
			mu.Lock()
			unexpected = append(unexpected, r.Host+" "+r.URL.RequestURI())
			mu.Unlock()
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)
	browser.f.setPreferences(preferences.Settings{SearchURLTemplate: server.URL + "/search?q={query}"})

	pageURL := server.URL + "/page"
	browser.navigateViaAddressBar(t, pageURL)
	browser.waitFor(t, "link page", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "page")
		return ok && browser.activeURL() == pageURL
	})

	for _, link := range []struct{ id, href string }{
		{"mailto", "mailto:someone@example.com"},
		{"javascript", `javascript:document.body.setAttribute("data-pwned","1")`},
		{"data", "data:text/html,secret"},
		{"ftp", "ftp://files.example/private"},
	} {
		t.Run(link.id, func(t *testing.T) {
			browser.clickNode(t, link.id)
			want := server.URL + "/search?q=" + url.QueryEscape(link.href)
			browser.waitFor(t, "the scheme is searched, not handed to a handler", func() bool {
				_, ok := findAXNode(browser.window.AccessibilityTree(), "search")
				return ok && browser.activeURL() == want
			})
			if !strings.HasPrefix(browser.activeURL(), server.URL+"/search?") {
				t.Fatalf("active URL = %q, want the browser's own search page", browser.activeURL())
			}

			browser.navigateViaAddressBar(t, pageURL)
			browser.waitFor(t, "return to the link page", func() bool {
				_, ok := findAXNode(browser.window.AccessibilityTree(), "page")
				return ok && browser.activeURL() == pageURL
			})
		})
	}

	mu.Lock()
	defer mu.Unlock()
	if len(unexpected) != 0 {
		t.Fatalf("scheme navigation reached %v, want only the page and search endpoints", unexpected)
	}
}
