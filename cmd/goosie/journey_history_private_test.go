package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/preferences"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func newStateTestApp(t *testing.T, private bool) *browserE2E {
	t.Helper()
	home := t.TempDir()
	return newStateTestAppAt(t, private, home)
}

func newStateTestAppAt(t *testing.T, private bool, home string) *browserE2E {
	t.Helper()
	t.Setenv("HOME", home)
	args := []string{"-backend", "headless", "-width", "640", "-height", "480", "-dpr", "1"}
	if private {
		args = append(args, "-private")
	}
	c, err := parse(args)
	if err != nil {
		t.Fatalf("parse state test config: %v", err)
	}
	f, err := build(c)
	if err != nil {
		t.Fatalf("build state test app: %v", err)
	}
	b := &browserE2E{f: f}
	t.Cleanup(func() { b.close(t) })
	return b
}

func getTestResponse(t *testing.T, b *browserE2E, url string) {
	t.Helper()
	resp, err := b.f.client.Get(context.Background(), url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	if len(resp.Body) > 1024 {
		t.Fatalf("response from %s exceeded test body bound: %d bytes", url, len(resp.Body))
	}
}

// qc:BR-043
func TestJourneyHistoryRecordsDisplayedVisitsAndOmitsFailedLoad(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "<!doctype html><title>Title %s</title><h1>Page %s</h1>", strings.TrimPrefix(r.URL.Path, "/"), r.URL.Path)
	}))
	defer server.Close()

	b := newBrowserE2E(t)
	urls := []string{server.URL + "/a", server.URL + "/b", server.URL + "/c"}
	for i, url := range urls {
		b.navigateViaAddressBar(t, url)
		wantTitle := "Title " + strings.TrimPrefix(strings.TrimPrefix(url, server.URL), "/")
		b.waitFor(t, "successful titled navigation", func() bool {
			entries := b.f.history.Entries()
			return b.activeURL() == url && b.f.tabMgr.Active().Title() == wantTitle && len(entries) == i+1
		})
		entries := b.f.history.Entries()
		if len(entries) != i+1 || entries[i].URL != url || entries[i].Title != wantTitle {
			t.Fatalf("history after %s = %+v, want %d displayed visits ending in %s/%s", url, entries, i+1, url, wantTitle)
		}
	}
	failedURL := "http://127.0.0.1:1/unavailable"
	b.navigateViaAddressBar(t, failedURL)
	b.waitFor(t, "failed navigation completion", func() bool {
		tab := b.f.tabMgr.Active()
		tab.Nav.Mu.Lock()
		loading := tab.Nav.Loading
		tab.Nav.Mu.Unlock()
		return !loading && tab.Error() != ""
	})
	entries := b.f.history.Entries()
	if len(entries) != 3 {
		t.Fatalf("history after failed load has %d entries, want 3 successful visits: %+v", len(entries), entries)
	}
	for i, entry := range entries {
		if entry.URL != urls[i] {
			t.Errorf("history[%d].URL = %q, want %q", i, entry.URL, urls[i])
		}
	}
}

// qc:BR-045
func TestJourneyReloadDoesNotAddHistoryVisits(t *testing.T) {
	var requestsMu sync.Mutex
	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsMu.Lock()
		requests[r.URL.Path]++
		requestsMu.Unlock()
		fmt.Fprintf(w, "<!doctype html><title>%s</title><h1>%s</h1>", r.URL.Path, r.URL.Path)
	}))
	defer server.Close()
	b := newBrowserE2E(t)
	a, bb := server.URL+"/a", server.URL+"/b"
	b.navigateViaAddressBar(t, a)
	b.waitFor(t, "page A", func() bool { return b.activeURL() == a && b.f.tabMgr.Active().Title() == "/a" })
	b.navigateViaAddressBar(t, bb)
	b.waitFor(t, "page B", func() bool { return b.activeURL() == bb && b.f.tabMgr.Active().Title() == "/b" })
	beforeReload := len(b.f.history.Entries())
	requestsMu.Lock()
	bRequestsBeforeReload := requests["/b"]
	requestsMu.Unlock()
	b.clickToolbarButton(t, toolbar.ButtonReload)
	b.waitFor(t, "reloaded page B", func() bool {
		tab := b.f.tabMgr.Active()
		tab.Nav.Mu.Lock()
		loading := tab.Nav.Loading
		tab.Nav.Mu.Unlock()
		requestsMu.Lock()
		loaded := requests["/b"] > bRequestsBeforeReload
		requestsMu.Unlock()
		return !loading && loaded && b.activeURL() == bb && tab.Title() == "/b"
	})
	if got := len(b.f.history.Entries()); got != beforeReload {
		t.Fatalf("history grew from %d to %d after reload", beforeReload, got)
	}
	// Pin the same consecutive-URL rule directly too: reload must not create a
	// duplicate even when the history store receives the displayed URL again.
	b.f.history.Record(bb, "/b")
	if got := len(b.f.history.Entries()); got != beforeReload {
		t.Fatalf("recording the displayed URL again grew history from %d to %d", beforeReload, got)
	}
	b.clickToolbarButton(t, toolbar.ButtonBack)
	b.waitFor(t, "back to page A", func() bool { return b.activeURL() == a && b.f.tabMgr.Active().Title() == "/a" })
	beforeBackReload := len(b.f.history.Entries())
	requestsMu.Lock()
	aRequestsBeforeReload := requests["/a"]
	requestsMu.Unlock()
	b.clickToolbarButton(t, toolbar.ButtonReload)
	b.waitFor(t, "reloaded page A after Back", func() bool {
		tab := b.f.tabMgr.Active()
		tab.Nav.Mu.Lock()
		loading := tab.Nav.Loading
		tab.Nav.Mu.Unlock()
		requestsMu.Lock()
		loaded := requests["/a"] > aRequestsBeforeReload
		requestsMu.Unlock()
		return !loading && loaded && b.activeURL() == a && tab.Title() == "/a"
	})
	if got := len(b.f.history.Entries()); got != beforeBackReload {
		t.Fatalf("history grew from %d to %d after reload following Back", beforeBackReload, got)
	}
}

// qc:BR-054
func TestPrivateBuildDoesNotTouchProfileStores(t *testing.T) {
	b := newStateTestApp(t, true)
	if b.f.client == nil || b.f.bookmarks != nil || b.f.history != nil {
		t.Fatalf("private app stores/client = %T/%v/%v; want client and no profile stores", b.f.client, b.f.bookmarks, b.f.history)
	}
	b.f.client.Jar().SetCookieFromDocument("http://example.test/", "private=ephemeral")
	if len(b.f.client.Jar().AllCookies()) != 1 {
		t.Fatal("private client did not accept its in-memory cookie")
	}
	b.close(t)
	for _, path := range []string{b.f.config.cookieFile, b.f.config.historyFile, b.f.config.bookmarkFile} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("private startup touched profile file %s: stat err=%v", path, err)
		}
	}
}

// qc:BR-055
func TestJourneyRegularPrivateRegularCookieIsolation(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Path] = r.Header.Get("Cookie")
		mu.Unlock()
		switch r.URL.Path {
		case "/set-x":
			http.SetCookie(w, &http.Cookie{Name: "X", Value: "regular", Path: "/", Expires: time.Now().Add(24 * time.Hour)})
		case "/set-y":
			http.SetCookie(w, &http.Cookie{Name: "Y", Value: "private", Path: "/", Expires: time.Now().Add(24 * time.Hour)})
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	home := t.TempDir()
	regular := newStateTestAppAt(t, false, home)
	getTestResponse(t, regular, server.URL+"/set-x")
	cookieFile := regular.f.config.cookieFile
	regular.close(t)
	if data, err := os.ReadFile(cookieFile); err != nil || !strings.Contains(string(data), "regular") || !strings.Contains(string(data), "X") {
		t.Fatalf("regular cookie file = %q, %v; want cookie X", data, err)
	}

	private := newStateTestAppAt(t, true, home)
	getTestResponse(t, private, server.URL+"/check-private")
	getTestResponse(t, private, server.URL+"/set-y")
	private.close(t)
	if got := private.f.client.Jar().AllCookies(); len(got) != 0 {
		t.Fatalf("private jar after Close = %+v, want cleared", got)
	}
	mu.Lock()
	privateCookie := seen["/check-private"]
	mu.Unlock()
	if strings.Contains(privateCookie, "X=") {
		t.Fatalf("private request received regular cookie: %q", privateCookie)
	}
	if data, err := os.ReadFile(cookieFile); err != nil || strings.Contains(string(data), "private") || strings.Contains(string(data), "Y") {
		t.Fatalf("profile cookie file after private close = %q, %v; must contain X but never Y", data, err)
	}

	regularAgain := newStateTestAppAt(t, false, home)
	getTestResponse(t, regularAgain, server.URL+"/check-regular-again")
	mu.Lock()
	regularCookie := seen["/check-regular-again"]
	mu.Unlock()
	if !strings.Contains(regularCookie, "X=regular") || strings.Contains(regularCookie, "Y=") {
		t.Fatalf("regular request cookies = %q, want X and no Y", regularCookie)
	}
}

// qc:BR-056
func TestPrivateRunDoesNotJournalOrRestorePrivateTabs(t *testing.T) {
	home := t.TempDir()
	private := newStateTestAppAt(t, true, home)
	secretURL := "https://private.example/secret"
	tab := private.f.tabMgr.Active()
	tab.SetURL(secretURL)
	tab.History().Push(secretURL)
	private.f.saveActiveSession()
	if shouldRestoreSession(true, preferences.StartupRestore) {
		t.Fatal("shouldRestoreSession(true, restore) = true")
	}
	private.close(t)
	for _, path := range []string{private.f.config.sessionFile, private.f.config.sessionActiveFile} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("private session file %s exists after close (stat err=%v)", path, err)
		}
	}

	regular := newStateTestAppAt(t, false, home)
	if regular.f.crashedSession != nil {
		t.Fatalf("regular startup found a private crash journal: %+v", regular.f.crashedSession)
	}
	for _, restored := range regular.f.tabMgr.Tabs() {
		if restored.URL() == secretURL {
			t.Fatalf("private URL %q appeared in restored tabs", secretURL)
		}
	}
}
