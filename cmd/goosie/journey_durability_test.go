package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/surface"
)

type durableTab struct {
	id  uint64
	url string
	doc string
}

// qc:BR-169
func TestJourneyTwentyURLsAcrossTenTabsSwitchAndCloseHalf(t *testing.T) {
	const (
		tabCount   = 10
		urlsPerTab = 2
		keepEvery  = 2
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/p/")
		if path == "" || strings.Contains(path, "/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><html><head><title>Page %s</title></head><body><h1 id="doc-%s">content %s</h1></body></html>`, path, path, path)
	}))
	defer server.Close()

	b := newBrowserE2E(t)
	defer b.close(t)

	states := make([]durableTab, 0, tabCount)
	for i := 0; i < tabCount; i++ {
		if i > 0 {
			previous := b.f.tabMgr.Active()
			previousID := uint64(0)
			if previous != nil {
				previousID = previous.ID()
			}
			b.inject(t, surface.Event{Kind: surface.EvKey, Key: 't', Mods: surface.ModCommand})
			want := i + 1
			b.waitFor(t, fmt.Sprintf("tab %d created and active", want), func() bool {
				active := b.f.tabMgr.Active()
				return b.f.tabMgr.Count() == want && active != nil && active.ID() != previousID
			})
		}
		tab := b.f.tabMgr.Active()
		if tab == nil {
			t.Fatalf("tab %d has no active tab; %s", i+1, b.diagnostics())
		}
		id := tab.ID()
		for j := 0; j < urlsPerTab; j++ {
			n := i*urlsPerTab + j
			url := fmt.Sprintf("%s/p/%d", server.URL, n)
			doc := fmt.Sprintf("doc-%d", n)
			b.navigateViaAddressBar(t, url)
			b.waitFor(t, fmt.Sprintf("tab %d loads page %d", i+1, n), func() bool {
				active := b.f.tabMgr.Active()
				if active == nil || active.ID() != id || b.activeURL() != url || active.Session() == nil {
					return false
				}
				return active.Session().Doc.ElementByID(doc) != nil
			})
		}
		last := i*urlsPerTab + urlsPerTab - 1
		states = append(states, durableTab{
			id:  id,
			url: fmt.Sprintf("%s/p/%d", server.URL, last),
			doc: fmt.Sprintf("doc-%d", last),
		})
	}

	if got := b.f.tabMgr.Count(); got != tabCount {
		t.Fatalf("tab count after %d documents = %d, want %d; %s", tabCount*urlsPerTab, got, tabCount, b.diagnostics())
	}

	for i, st := range states {
		st := st
		b.f.switchTab(st.id)
		b.waitFor(t, fmt.Sprintf("switch back to tab %d", i+1), func() bool {
			active := b.f.tabMgr.Active()
			return active != nil && active.ID() == st.id && b.activeURL() == st.url
		})
		assertDurableTab(t, b, st, fmt.Sprintf("after switching to tab %d", i+1))
	}

	remaining := 0
	for i := 1; i < len(states); i += keepEvery {
		st := states[i]
		b.f.switchTab(st.id)
		b.waitFor(t, fmt.Sprintf("close candidate tab %d is active", i+1), func() bool {
			active := b.f.tabMgr.Active()
			return active != nil && active.ID() == st.id
		})
		b.inject(t, surface.Event{Kind: surface.EvKey, Key: 'w', Mods: surface.ModCommand})
		remaining++
		want := tabCount - remaining
		b.waitFor(t, fmt.Sprintf("tab %d closes", i+1), func() bool {
			return b.f.tabMgr.Count() == want && b.f.tabMgr.TabByID(st.id) == nil
		})
	}

	if got := b.f.tabMgr.Count(); got != tabCount/keepEvery {
		t.Fatalf("tab count after closing half = %d, want %d; %s", got, tabCount/keepEvery, b.diagnostics())
	}
	if active := b.f.tabMgr.Active(); active == nil {
		t.Fatal("no active tab after closing half the strip")
	}

	for i := 0; i < len(states); i += keepEvery {
		st := states[i]
		tab := b.f.tabMgr.TabByID(st.id)
		if tab == nil {
			t.Fatalf("tab %d was closed but it was not one of the half selected for closing", i+1)
		}
		assertDurableTab(t, b, st, fmt.Sprintf("after closing half, tab %d", i+1))
		b.f.switchTab(st.id)
		b.waitFor(t, fmt.Sprintf("remaining tab %d becomes active", i+1), func() bool {
			active := b.f.tabMgr.Active()
			return active != nil && active.ID() == st.id && b.activeURL() == st.url
		})
		assertDurableTab(t, b, st, fmt.Sprintf("remaining tab %d while active", i+1))
	}
}

func assertDurableTab(t *testing.T, b *browserE2E, st durableTab, when string) {
	t.Helper()
	tab := b.f.tabMgr.TabByID(st.id)
	if tab == nil {
		t.Fatalf("%s: tab %d is missing", when, st.id)
	}
	if got := tab.URL(); got != st.url {
		t.Fatalf("%s: URL = %q, want %q", when, got, st.url)
	}
	sess := tab.Session()
	if sess == nil {
		t.Fatalf("%s: tab %d lost its document", when, st.id)
	}
	if sess.Doc.ElementByID(st.doc) == nil {
		t.Fatalf("%s: tab %d does not hold %q any more", when, st.id, st.doc)
	}
	if tab.Layer() == nil {
		t.Fatalf("%s: tab %d lost its rendered layer", when, st.id)
	}
}
