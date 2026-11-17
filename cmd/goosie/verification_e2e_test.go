package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestE2EFindJourney(t *testing.T) {
	page := `<!doctype html><html><body style="margin:0;font-size:16px;">` +
		`<p id="first">needle alpha</p>` +
		strings.Repeat(`<p>filler filler filler filler filler</p>`, 60) +
		`<p id="second">needle omega</p></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}))
	t.Cleanup(server.Close)

	browser := newBrowserE2E(t)
	browser.navigateViaAddressBar(t, server.URL)
	browser.waitFor(t, "find fixture loaded at the top of the document", func() bool {
		tab := browser.f.tabMgr.Active()
		return browser.activeURL() == server.URL && tab != nil && tab.Session() != nil &&
			tab.Layer() != nil && tab.ScrollY() == 0 &&
			axContainsLabel(browser.window.AccessibilityTree(), "needle alpha")
	})
	tab := browser.f.tabMgr.Active()
	originalSession := tab.Session()
	originalText := originalSession.Doc.Node.TextContent()

	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 'f', Mods: surface.ModCommand})
	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 'f', Mods: surface.ModCommand, KeyAction: surface.KeyRelease})
	browser.waitFor(t, "Cmd+F opens and focuses find", func() bool {
		active, _, _ := browser.f.toolbar.FindCounts()
		return active && browser.f.toolbar.GetFocus() == toolbar.FocusAddress
	})
	for _, key := range "needle" {
		browser.pressKey(t, key)
	}
	browser.waitFor(t, "needle has two matches with the first selected at the top", func() bool {
		active, total, index := browser.f.toolbar.FindCounts()
		query, _, _, _, _, _, _ := browser.f.toolbar.AccessibilitySnapshot()
		return active && query == "needle" && total == 2 && index == 0 && tab.ScrollY() == 0
	})

	browser.pressKey(t, '\r')
	browser.waitFor(t, "Enter selects the second match", func() bool {
		active, total, index := browser.f.toolbar.FindCounts()
		return active && total == 2 && index == 1
	})
	browser.waitFor(t, "second match scrolls the document", func() bool {
		return tab.ScrollY() > 0
	})

	browser.pressKey(t, 0x1b)
	browser.waitFor(t, "Escape closes find and returns focus to the page", func() bool {
		active, _, _ := browser.f.toolbar.FindCounts()
		return !active && browser.f.toolbar.GetFocus() == toolbar.FocusNone
	})

	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 'f', Mods: surface.ModCommand})
	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 'f', Mods: surface.ModCommand, KeyAction: surface.KeyRelease})
	browser.waitFor(t, "Cmd+F reopens the previous query", func() bool {
		active, total, _ := browser.f.toolbar.FindCounts()
		query, focused, _, _, _, _, _ := browser.f.toolbar.AccessibilitySnapshot()
		return active && focused && query == "needle" && total == 2
	})
	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 'a', Mods: surface.ModCommand})
	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 'a', Mods: surface.ModCommand, KeyAction: surface.KeyRelease})
	for _, key := range "xyzzy" {
		browser.pressKey(t, key)
	}
	browser.waitFor(t, "replacement miss query reports zero matches", func() bool {
		active, total, index := browser.f.toolbar.FindCounts()
		query, _, _, _, _, _, _ := browser.f.toolbar.AccessibilitySnapshot()
		return active && query == "xyzzy" && total == 0 && index == 0
	})
	browser.waitFor(t, "zero-result find retains the document, URL, and rendered page", func() bool {
		return browser.f.tabMgr.Active() == tab && tab.URL() == server.URL &&
			tab.Session() == originalSession && originalSession.Doc.Node.TextContent() == originalText &&
			tab.Layer() != nil && axContainsLabel(browser.window.AccessibilityTree(), "needle alpha")
	})
	browser.pressKey(t, 0x1b)
	browser.waitFor(t, "Escape closes find after the miss", func() bool {
		active, _, _ := browser.f.toolbar.FindCounts()
		return !active && browser.f.toolbar.GetFocus() == toolbar.FocusNone
	})
}

func TestE2ETabIsolationJourney(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/a":
			_, _ = w.Write([]byte(`<!doctype html><html><head><title>Page A</title></head><body><p id="page-a">Tab A original document</p></body></html>`))
		case "/b":
			_, _ = w.Write([]byte(`<!doctype html><html><head><title>Page B</title></head><body><p id="page-b">Tab B separate document</p></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	browser := newBrowserE2E(t)
	urlA, urlB := server.URL+"/a", server.URL+"/b"
	browser.navigateViaAddressBar(t, urlA)
	browser.waitFor(t, "page A loads in the only tab", func() bool {
		tab := browser.f.tabMgr.Active()
		return browser.f.tabMgr.Count() == 1 && tab != nil && tab.Session() != nil &&
			tab.URL() == urlA && axContainsLabel(browser.window.AccessibilityTree(), "Tab A original document")
	})
	tabA := browser.f.tabMgr.Active()
	sessionA := tabA.Session()
	textA := sessionA.Doc.Node.TextContent()

	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 't', Mods: surface.ModCommand})
	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 't', Mods: surface.ModCommand, KeyAction: surface.KeyRelease})
	browser.waitFor(t, "Cmd+T creates and activates a separate blank tab", func() bool {
		tab := browser.f.tabMgr.Active()
		return browser.f.tabMgr.Count() == 2 && tab != nil && tab.ID() != tabA.ID() &&
			tab.URL() == "" && tab.Session() == nil
	})
	tabB := browser.f.tabMgr.Active()

	browser.navigateViaAddressBar(t, urlB)
	browser.waitFor(t, "page B loads in the new tab without page A content", func() bool {
		nodes := browser.window.AccessibilityTree()
		return browser.f.tabMgr.Count() == 2 && browser.f.tabMgr.Active() == tabB &&
			tabB.URL() == urlB && tabB.Session() != nil && tabB.Session() != sessionA &&
			axContainsLabel(nodes, "Tab B separate document") && !axContainsLabel(nodes, "Tab A original document")
	})
	browser.waitFor(t, "background tab A retains its URL and original document", func() bool {
		return tabA.URL() == urlA && tabA.Session() == sessionA && sessionA.Doc.Node.TextContent() == textA
	})

	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 'w', Mods: surface.ModCommand})
	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 'w', Mods: surface.ModCommand, KeyAction: surface.KeyRelease})
	browser.waitFor(t, "Cmd+W closes B and restores A with its document and URL intact", func() bool {
		nodes := browser.window.AccessibilityTree()
		return browser.f.tabMgr.Count() == 1 && browser.f.tabMgr.Active() == tabA &&
			tabA.URL() == urlA && tabA.Session() == sessionA && sessionA.Doc.Node.TextContent() == textA &&
			tabA.Layer() != nil && axContainsLabel(nodes, "Tab A original document") &&
			!axContainsLabel(nodes, "Tab B separate document")
	})
}
