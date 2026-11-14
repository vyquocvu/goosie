package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/toolbar"
)

// qc:BR-017
func TestJourneyPostHistoryAsksBeforeResubmitting(t *testing.T) {
	var mu sync.Mutex
	var posts int
	var bodies []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/form":
			fmt.Fprint(w, `<!doctype html><html><body>
<h1 id="form-title">Form</h1>
<form action="/submit" method="POST">
  <input type="text" name="note" value="hello">
  <button type="submit" id="send">Send</button>
</form>
</body></html>`)
		case "/submit":
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			posts++
			bodies = append(bodies, string(body))
			count := posts
			mu.Unlock()
			if r.Method != http.MethodPost {
				t.Errorf("resubmission reached the server as %s, want POST", r.Method)
			}
			fmt.Fprintf(w, `<!doctype html><html><body><h1 id="result">Submitted %d</h1></body></html>`, count)
		case "/other":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="other">Other</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	postCount := func() int {
		mu.Lock()
		defer mu.Unlock()
		return posts
	}
	lastBody := func() string {
		mu.Lock()
		defer mu.Unlock()
		if len(bodies) == 0 {
			return ""
		}
		return bodies[len(bodies)-1]
	}
	browser := newBrowserE2E(t)
	defer browser.close(t)

	waitForPrompt := func(description string) {
		t.Helper()
		browser.waitFor(t, description, func() bool {
			_, ok := findAXNode(browser.window.AccessibilityTree(), "resubmit-confirm")
			return ok
		})
	}

	browser.navigateViaAddressBar(t, server.URL+"/form")
	browser.waitFor(t, "form page", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "send")
		return ok && browser.activeURL() == server.URL+"/form"
	})

	browser.clickElementByID(t, "send")
	browser.waitFor(t, "the first POST result", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "result")
		return ok && browser.activeURL() == server.URL+"/submit" && postCount() == 1
	})
	if got := lastBody(); !strings.Contains(got, "note=hello") {
		t.Fatalf("first POST body = %q, want note=hello", got)
	}

	browser.clickToolbarButton(t, toolbar.ButtonBack)
	browser.waitFor(t, "back to the form", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "send")
		return ok && browser.activeURL() == server.URL+"/form"
	})

	browser.clickToolbarButton(t, toolbar.ButtonForward)
	waitForPrompt("forward to the POST result prompts")
	if postCount() != 1 {
		t.Fatalf("the server saw %d POST(s) before confirmation, want 1", postCount())
	}

	browser.clickElementByID(t, "resubmit-cancel")
	browser.waitFor(t, "cancel restores the form", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "send")
		return ok && browser.activeURL() == server.URL+"/form"
	})
	if postCount() != 1 {
		t.Fatalf("cancel sent a POST: count = %d", postCount())
	}
	if !browser.f.tabMgr.Active().History().CanForward() {
		t.Fatal("cancel moved the history cursor past the POST entry")
	}

	browser.clickToolbarButton(t, toolbar.ButtonForward)
	waitForPrompt("forward prompts again after cancel")
	browser.clickElementByID(t, "resubmit-confirm")
	browser.waitFor(t, "confirmed resubmission", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "result")
		return ok && browser.activeURL() == server.URL+"/submit" && postCount() == 2
	})
	if got := lastBody(); got != "note=hello" {
		t.Fatalf("resubmitted body = %q, want note=hello", got)
	}

	browser.clickToolbarButton(t, toolbar.ButtonReload)
	waitForPrompt("reload of a POST result prompts")
	browser.clickElementByID(t, "resubmit-cancel")
	browser.waitFor(t, "cancel keeps the POST result", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "result")
		return ok && browser.activeURL() == server.URL+"/submit"
	})
	if postCount() != 2 {
		t.Fatalf("cancel during reload sent a POST: count = %d", postCount())
	}

	browser.clickToolbarButton(t, toolbar.ButtonReload)
	waitForPrompt("reload prompts again")
	browser.clickElementByID(t, "resubmit-confirm")
	browser.waitFor(t, "reload resubmitted", func() bool {
		return postCount() == 3
	})

	browser.navigateViaAddressBar(t, server.URL+"/other")
	browser.waitFor(t, "other page", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "other")
		return ok && browser.activeURL() == server.URL+"/other"
	})
	browser.clickToolbarButton(t, toolbar.ButtonBack)
	waitForPrompt("back to a POST entry prompts")
	if postCount() != 3 {
		t.Fatalf("the server saw %d POST(s) before the back confirmation, want 3", postCount())
	}

	browser.clickElementByID(t, "resubmit-cancel")
	browser.waitFor(t, "cancel keeps the other page", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "other")
		return ok && browser.activeURL() == server.URL+"/other"
	})

	browser.clickToolbarButton(t, toolbar.ButtonBack)
	waitForPrompt("back prompts again after cancel")
	browser.clickElementByID(t, "resubmit-confirm")
	browser.waitFor(t, "back resubmission", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "result")
		return ok && browser.activeURL() == server.URL+"/submit" && postCount() == 4
	})

	browser.clickToolbarButton(t, toolbar.ButtonForward)
	browser.waitFor(t, "forward to the GET entry needs no prompt", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "other")
		return ok && browser.activeURL() == server.URL+"/other" && postCount() == 4
	})
}
