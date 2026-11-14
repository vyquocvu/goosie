package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
)

// qc:BR-007
func TestJourneyMalformedURLRecoversWithoutRetainingOldDoc(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/initial":
			fmt.Fprint(w, `<!doctype html><html><head><title>Initial</title></head><body><h1 id="initial-doc">Initial Content</h1></body></html>`)
		case "/recovered":
			fmt.Fprint(w, `<!doctype html><html><head><title>Recovered</title></head><body><h1 id="recovered-doc">Recovered Content</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	initialURL := server.URL + "/initial"
	recoveredURL := server.URL + "/recovered"
	malformedURL := "http://127.0.0.1:99999/malformed"

	browser.navigateViaAddressBar(t, initialURL)
	browser.waitFor(t, "initial page loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "initial-doc")
		return ok && browser.activeURL() == initialURL
	})

	browser.navigateViaAddressBar(t, malformedURL)

	browser.waitFor(t, "error page for malformed URL", func() bool {
		tree := browser.window.AccessibilityTree()
		_, initialStillPresent := findAXNode(tree, "initial-doc")
		active := browser.f.tabMgr.Active()
		return !initialStillPresent && active != nil && active.Error() != "" &&
			active.Title() == "Problem loading page" &&
			(axContainsLabel(tree, "Try Again") ||
				axContainsLabel(tree, "This Site Can’t Be Reached"))
	})

	browser.navigateViaAddressBar(t, recoveredURL)
	browser.waitFor(t, "recovered page loaded successfully", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "recovered-doc")
		active := browser.f.tabMgr.Active()
		return ok && browser.activeURL() == recoveredURL && active != nil && active.Error() == ""
	})
}

// qc:BR-162
func TestJourneyOversizedDocumentShowsErrorAndRecovers(t *testing.T) {
	oversized := strings.Repeat("<p>", engine.MaxDocumentBytes/3+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/huge":
			fmt.Fprint(w, oversized)
		case "/recovered":
			fmt.Fprint(w, `<!doctype html><html><head><title>Recovered</title></head><body><h1 id="recovered-after-oversize">Recovered Content</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	hugeURL := server.URL + "/huge"
	browser.navigateViaAddressBar(t, hugeURL)
	browser.waitFor(t, "error page for oversized document", func() bool {
		tree := browser.window.AccessibilityTree()
		active := browser.f.tabMgr.Active()
		byteCap := axContainsLabel(tree, "response exceeds 16 MiB") ||
			axContainsLabel(tree, "HTML byte limit exceeded")
		return active != nil && active.Error() != "" &&
			active.Title() == "Problem loading page" && byteCap
	})

	recoveredURL := server.URL + "/recovered"
	browser.navigateViaAddressBar(t, recoveredURL)
	browser.waitFor(t, "navigation after oversized document", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "recovered-after-oversize")
		active := browser.f.tabMgr.Active()
		return ok && browser.activeURL() == recoveredURL && active != nil && active.Error() == ""
	})
}

// qc:BR-167
func TestJourneyUnresponsiveScriptLeavesChromeOperable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/slow":
			fmt.Fprint(w, `<!doctype html><html><head><title>Slow</title></head><body><h1 id="slow-doc">SLOW PAGE BODY</h1><script>while (true) {}</script></body></html>`)
		case "/after":
			fmt.Fprint(w, `<!doctype html><html><head><title>After</title></head><body><h1 id="after-doc">After Timeout</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	browser.f.scriptTimeout = 50 * time.Millisecond

	slowURL := server.URL + "/slow"
	browser.navigateViaAddressBar(t, slowURL)
	browser.waitFor(t, "unresponsive script prompt", func() bool {
		tree := browser.window.AccessibilityTree()
		active := browser.f.tabMgr.Active()
		_, interruptedStillShown := findAXNode(tree, "slow-doc")
		return active != nil && active.Title() == "Unresponsive Script" &&
			!interruptedStillShown &&
			axContainsLabel(tree, "Unresponsive Script") &&
			axContainsLabel(tree, "Reload without scripts")
	})

	afterURL := server.URL + "/after"
	browser.navigateViaAddressBar(t, afterURL)
	browser.waitFor(t, "navigation after unresponsive script", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "after-doc")
		active := browser.f.tabMgr.Active()
		return ok && browser.activeURL() == afterURL && active != nil && active.Error() == ""
	})
}

// qc:BR-012
func TestJourneyServerDownShowsRecognizableErrorAndAcceptsNextNav(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	deadAddr := listener.Addr().String()
	_ = listener.Close()

	deadURL := "http://" + deadAddr + "/unreachable"

	browser := newBrowserE2E(t)
	defer browser.close(t)

	browser.navigateViaAddressBar(t, deadURL)

	browser.waitFor(t, "error page for unreachable server", func() bool {
		tree := browser.window.AccessibilityTree()
		active := browser.f.tabMgr.Active()
		if active == nil || active.Error() == "" {
			return false
		}
		return active.Title() == "Problem loading page" &&
			(axContainsLabel(tree, "Unable to Connect") ||
				axContainsLabel(tree, "ERR_CONNECTION_REFUSED"))
	})

	liveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><body><h1 id="live-doc">Live Server</h1></body></html>`)
	}))
	defer liveServer.Close()

	liveURL := liveServer.URL + "/live"
	browser.navigateViaAddressBar(t, liveURL)

	browser.waitFor(t, "subsequent navigation to live server succeeds", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "live-doc")
		active := browser.f.tabMgr.Active()
		return ok && browser.activeURL() == liveURL && active != nil && active.Error() == ""
	})
}
