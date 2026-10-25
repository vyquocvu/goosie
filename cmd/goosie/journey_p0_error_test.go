package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
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

	// 1. Load initial valid page
	browser.navigateViaAddressBar(t, initialURL)
	browser.waitFor(t, "initial page loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "initial-doc")
		return ok && browser.activeURL() == initialURL
	})

	// 2. Navigate to malformed URL (port > 65535 is invalid syntax)
	browser.navigateViaAddressBar(t, malformedURL)

	// 3. Wait for error page:
	// - Old document must NOT be retained as if load succeeded
	// - Tab title reports problem loading page
	// - Tab error state is non-empty
	// - Accessibility tree shows error page content ("Try Again" link or error code)
	browser.waitFor(t, "error page for malformed URL", func() bool {
		tree := browser.window.AccessibilityTree()
		_, initialStillPresent := findAXNode(tree, "initial-doc")
		active := browser.f.tabMgr.Active()
		return !initialStillPresent && active != nil && active.Error() != "" &&
			active.Title() == "Problem loading page" &&
			(axContainsLabel(tree, "Try Again") ||
				axContainsLabel(tree, "This Site Can’t Be Reached"))
	})

	// 4. Recover: navigate to a subsequent valid URL
	browser.navigateViaAddressBar(t, recoveredURL)
	browser.waitFor(t, "recovered page loaded successfully", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "recovered-doc")
		active := browser.f.tabMgr.Active()
		return ok && browser.activeURL() == recoveredURL && active != nil && active.Error() == ""
	})
}

// qc:BR-012
func TestJourneyServerDownShowsRecognizableErrorAndAcceptsNextNav(t *testing.T) {
	// Allocate a port and close immediately so the host is guaranteed dialable-down (connection refused)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	deadAddr := listener.Addr().String()
	_ = listener.Close()

	deadURL := "http://" + deadAddr + "/unreachable"

	browser := newBrowserE2E(t)
	defer browser.close(t)

	// 1. Navigate to unreachable server
	browser.navigateViaAddressBar(t, deadURL)

	// 2. Wait for error state:
	// - Error page is rendered
	// - Accessibility tree contains the failed host/address or recognizable error heading/code
	// - Active tab reflects the failure
	browser.waitFor(t, "error page for unreachable server", func() bool {
		tree := browser.window.AccessibilityTree()
		active := browser.f.tabMgr.Active()
		if active == nil || active.Error() == "" {
			return false
		}
		// The dead host is itself a label in the address bar, because it is the
		// URL that was just typed, so matching it can pass before any error page
		// exists. Require a label only the error page body carries, plus the
		// title the next lines read.
		return active.Title() == "Problem loading page" &&
			(axContainsLabel(tree, "Unable to Connect") ||
				axContainsLabel(tree, "ERR_CONNECTION_REFUSED"))
	})

	// 3. Verify app is fully responsive and accepts the next navigation
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
