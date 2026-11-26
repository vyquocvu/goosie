package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/session"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func historyAPIProbe(t *testing.T, b *browserE2E) string {
	t.Helper()
	tab := b.f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return ""
	}
	got, err := tab.Session().EvaluateJS(`[location.href, history.length, JSON.stringify(history.state), window.events.join(",")].join("|")`)
	if err != nil {
		return ""
	}
	return got
}

func historyAPIRequests(countsMu *sync.Mutex, counts map[string]int, path string) int {
	countsMu.Lock()
	defer countsMu.Unlock()
	return counts[path]
}

func TestJourneyHistoryAPIStaysInTheCurrentDocument(t *testing.T) {
	var countsMu sync.Mutex
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		countsMu.Lock()
		counts[r.URL.Path]++
		countsMu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><title>SPA</title><script>
			window.events = [];
			window.addEventListener("popstate", function (event) {
				window.events.push(event.state && event.state.page);
			});
			history.pushState({page: 2}, "", "/spa?page=2");
			history.pushState({page: 3}, "", "/spa?page=3");
		</script>`)
	}))
	defer server.Close()

	b := newBrowserE2E(t)
	start := server.URL + "/spa"
	b.navigateViaAddressBar(t, start)
	pushed := start + "?page=3|3|{\"page\":3}|"
	b.waitFor(t, "pushState commits without navigation", func() bool {
		tab := b.f.tabMgr.Active()
		return tab != nil && tab.URL() == start+"?page=3" && historyAPIProbe(t, b) == pushed
	})
	if got := historyAPIRequests(&countsMu, counts, "/spa"); got != 1 {
		t.Fatalf("/spa requests after two pushState calls = %d, want 1", got)
	}
	b.f.saveActiveSession()
	st, err := session.Load(b.f.config.sessionActiveFile)
	if err != nil {
		t.Fatalf("load saved session: %v", err)
	}
	if len(st.Tabs) != 1 || len(st.Tabs[0].History) != 3 || len(st.Tabs[0].HistoryStates) != 3 {
		t.Fatalf("saved history state = %+v", st.Tabs)
	}
	if st.Tabs[0].HistoryIndex != 2 || string(st.Tabs[0].HistoryStates[1]) == "" || !st.Tabs[0].HistorySameDocument[2] {
		t.Fatalf("saved history metadata = %+v", st.Tabs[0])
	}

	b.clickToolbarButton(t, toolbar.ButtonBack)
	back := start + "?page=2|3|{\"page\":2}|2"
	b.waitFor(t, "Back restores state and fires popstate", func() bool {
		tab := b.f.tabMgr.Active()
		return tab != nil && tab.URL() == start+"?page=2" && historyAPIProbe(t, b) == back
	})
	if got := historyAPIRequests(&countsMu, counts, "/spa"); got != 1 {
		t.Fatalf("/spa requests after Back = %d, want 1", got)
	}

	b.clickToolbarButton(t, toolbar.ButtonForward)
	forward := pushed + "2,3"
	b.waitFor(t, "Forward restores state and fires popstate", func() bool {
		tab := b.f.tabMgr.Active()
		return tab != nil && tab.URL() == start+"?page=3" && historyAPIProbe(t, b) == forward
	})
	if got := historyAPIRequests(&countsMu, counts, "/spa"); got != 1 {
		t.Fatalf("/spa requests after Forward = %d, want 1", got)
	}
}

func TestJourneyHistoryAPICrossesDocumentsWithState(t *testing.T) {
	var countsMu sync.Mutex
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		countsMu.Lock()
		counts[r.URL.Path]++
		countsMu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/destination" {
			fmt.Fprint(w, `<!doctype html><title>Destination</title><script>window.events = [];</script>`)
			return
		}
		push := ""
		if r.URL.Query().Get("state") == "" {
			push = `history.pushState({page: 2}, "", "/spa?state=2");`
		}
		fmt.Fprintf(w, `<!doctype html><title>SPA</title><script>
			window.events = [];
			window.addEventListener("popstate", function (event) {
				window.events.push(event.state && event.state.page);
			});
			%s
		</script>`, push)
	}))
	defer server.Close()

	b := newBrowserE2E(t)
	start := server.URL + "/spa"
	destination := server.URL + "/destination"
	b.navigateViaAddressBar(t, start)
	pushed := start + "?state=2|2|{\"page\":2}|"
	b.waitFor(t, "cross-document source pushes state", func() bool {
		tab := b.f.tabMgr.Active()
		return tab != nil && tab.URL() == start+"?state=2" && historyAPIProbe(t, b) == pushed
	})
	b.navigateViaAddressBar(t, destination)
	b.waitFor(t, "destination document loads", func() bool {
		tab := b.f.tabMgr.Active()
		return tab != nil && tab.URL() == destination && tab.Session() != nil && !tab.Loading()
	})
	b.clickToolbarButton(t, toolbar.ButtonBack)
	restored := start + "?state=2|3|{\"page\":2}|2"
	b.waitFor(t, "Back reloads the source document with its state", func() bool {
		tab := b.f.tabMgr.Active()
		return tab != nil && tab.URL() == start+"?state=2" && historyAPIProbe(t, b) == restored
	})
	if got := historyAPIRequests(&countsMu, counts, "/spa"); got < 2 {
		t.Fatalf("/spa requests after cross-document Back = %d, want at least 2", got)
	}
	b.clickToolbarButton(t, toolbar.ButtonForward)
	b.waitFor(t, "Forward returns to the destination document", func() bool {
		tab := b.f.tabMgr.Active()
		return tab != nil && tab.URL() == destination && tab.Session() != nil && !tab.Loading()
	})
}

func TestPrivateHistoryStateIsNotPersisted(t *testing.T) {
	home := t.TempDir()
	b := newStateTestAppAt(t, true, home)
	tab := b.f.tabMgr.Active()
	tab.SetURL("https://private.example/start")
	tab.History().PushState("https://private.example/route", toolbar.Request{}, []byte(`{"secret":true}`), 0, true)
	b.f.saveActiveSession()
	for _, path := range []string{b.f.config.sessionFile, b.f.config.sessionActiveFile} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("private session file %s exists after history state (stat err=%v)", path, err)
		}
	}
}
