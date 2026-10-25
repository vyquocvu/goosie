package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/surface"
)

// qc:BR-023
func TestJourneyInitialWindowUsableTab(t *testing.T) {
	browser := newBrowserE2E(t)
	defer browser.close(t)

	// Check tab manager state on clean startup
	if count := browser.f.tabMgr.Count(); count != 1 {
		t.Fatalf("startup tab count = %d, want 1; %s", count, browser.diagnostics())
	}
	active := browser.f.tabMgr.Active()
	if active == nil {
		t.Fatalf("startup active tab is nil; %s", browser.diagnostics())
	}

	// Verify accessibility tree reflects exactly one tab and it is Selected
	browser.waitFor(t, "startup accessibility tree has single selected tab", func() bool {
		tree := browser.window.AccessibilityTree()
		tabGroup, ok := findAXNode(tree, "goosie.tabs")
		if !ok {
			return false
		}
		var tabNodes []ax.Node
		for _, child := range tabGroup.Children {
			if child.Role == ax.RoleTab {
				tabNodes = append(tabNodes, child)
			}
		}
		if len(tabNodes) != 1 {
			return false
		}
		return tabNodes[0].Selected
	})

	// Verify address bar is present in accessibility tree
	addrNode, ok := findAXNode(browser.window.AccessibilityTree(), "goosie.address")
	if !ok {
		t.Fatalf("startup address bar accessibility node missing; %s", browser.diagnostics())
	}
	if !addrNode.AddressBar {
		t.Errorf("address node AddressBar = false, want true")
	}

	// Verify the tab is immediately usable: navigate to a local server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><body><h1 id="started">Started Tab</h1></body></html>`)
	}))
	defer server.Close()

	browser.navigateViaAddressBar(t, server.URL)
	browser.waitFor(t, "initial tab navigation completion", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "started")
		return ok && browser.activeURL() == server.URL
	})
}

// qc:BR-026
func TestJourneySwitchBetweenThreeTabs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/tab1":
			fmt.Fprint(w, `<!doctype html><html><head><title>Title One</title></head><body><h1 id="doc1">Content One</h1></body></html>`)
		case "/tab2":
			fmt.Fprint(w, `<!doctype html><html><head><title>Title Two</title></head><body><h1 id="doc2">Content Two</h1></body></html>`)
		case "/tab3":
			fmt.Fprint(w, `<!doctype html><html><head><title>Title Three</title></head><body><h1 id="doc3">Content Three</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	url1 := server.URL + "/tab1"
	url2 := server.URL + "/tab2"
	url3 := server.URL + "/tab3"

	// 1. Navigate first tab to url1
	browser.navigateViaAddressBar(t, url1)
	browser.waitFor(t, "tab 1 loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "doc1")
		return ok && browser.activeURL() == url1
	})
	tab1ID := browser.f.tabMgr.Active().ID()

	// 2. Open second tab via Cmd+T
	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 't', Mods: surface.ModCommand})
	browser.waitFor(t, "second tab created and focused", func() bool {
		return browser.f.tabMgr.Count() == 2 && browser.f.tabMgr.Active().ID() != tab1ID
	})
	tab2ID := browser.f.tabMgr.Active().ID()
	if tab2ID == tab1ID {
		t.Fatalf("second tab ID = %d, expected different from tab 1 ID = %d", tab2ID, tab1ID)
	}

	browser.navigateViaAddressBar(t, url2)
	browser.waitFor(t, "tab 2 loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "doc2")
		return ok && browser.activeURL() == url2
	})

	// 3. Open third tab via New Tab button click in AX tree
	browser.clickNode(t, "goosie.new-tab")
	browser.waitFor(t, "third tab created and focused", func() bool {
		id := browser.f.tabMgr.Active().ID()
		return browser.f.tabMgr.Count() == 3 && id != tab1ID && id != tab2ID
	})
	tab3ID := browser.f.tabMgr.Active().ID()
	if tab3ID == tab1ID || tab3ID == tab2ID {
		t.Fatalf("third tab ID = %d must be distinct", tab3ID)
	}

	browser.navigateViaAddressBar(t, url3)
	browser.waitFor(t, "tab 3 loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "doc3")
		return ok && browser.activeURL() == url3
	})

	id1Str := strconv.FormatUint(tab1ID, 10)
	id2Str := strconv.FormatUint(tab2ID, 10)
	id3Str := strconv.FormatUint(tab3ID, 10)

	// 4. Switch to Tab 1 by clicking its AX tab node
	browser.clickNode(t, "goosie.tab."+id1Str)
	browser.waitFor(t, "switched to tab 1", func() bool {
		_, doc1 := findAXNode(browser.window.AccessibilityTree(), "doc1")
		t1Node, _ := findAXNode(browser.window.AccessibilityTree(), "goosie.tab."+id1Str)
		return doc1 && t1Node.Selected && browser.activeURL() == url1
	})
	if active := browser.f.tabMgr.Active(); active.Title() != "Title One" {
		t.Errorf("tab 1 title = %q, want %q", active.Title(), "Title One")
	}

	// 5. Switch to Tab 2
	browser.clickNode(t, "goosie.tab."+id2Str)
	browser.waitFor(t, "switched to tab 2", func() bool {
		_, doc2 := findAXNode(browser.window.AccessibilityTree(), "doc2")
		t2Node, _ := findAXNode(browser.window.AccessibilityTree(), "goosie.tab."+id2Str)
		return doc2 && t2Node.Selected && browser.activeURL() == url2
	})
	if active := browser.f.tabMgr.Active(); active.Title() != "Title Two" {
		t.Errorf("tab 2 title = %q, want %q", active.Title(), "Title Two")
	}

	// 6. Switch back to Tab 3
	browser.clickNode(t, "goosie.tab."+id3Str)
	browser.waitFor(t, "switched to tab 3", func() bool {
		_, doc3 := findAXNode(browser.window.AccessibilityTree(), "doc3")
		t3Node, _ := findAXNode(browser.window.AccessibilityTree(), "goosie.tab."+id3Str)
		return doc3 && t3Node.Selected && browser.activeURL() == url3
	})
	if active := browser.f.tabMgr.Active(); active.Title() != "Title Three" {
		t.Errorf("tab 3 title = %q, want %q", active.Title(), "Title Three")
	}
}

// qc:BR-030
func TestJourneyCloseActiveTabSelectsNext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/first":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="first-doc">First Tab</h1></body></html>`)
		case "/second":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="second-doc">Second Tab</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	firstURL := server.URL + "/first"
	secondURL := server.URL + "/second"

	// Set up first tab
	browser.navigateViaAddressBar(t, firstURL)
	browser.waitFor(t, "first tab loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "first-doc")
		return ok && browser.activeURL() == firstURL
	})
	firstID := browser.f.tabMgr.Active().ID()

	// Open second tab
	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 't', Mods: surface.ModCommand})
	browser.waitFor(t, "second tab created and focused", func() bool {
		return browser.f.tabMgr.Count() == 2 && browser.f.tabMgr.Active().ID() != firstID
	})
	secondID := browser.f.tabMgr.Active().ID()
	if secondID == firstID {
		t.Fatalf("second tab ID = %d must be distinct from first tab ID = %d", secondID, firstID)
	}

	browser.navigateViaAddressBar(t, secondURL)
	browser.waitFor(t, "second tab loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "second-doc")
		return ok && browser.activeURL() == secondURL
	})

	// Close the active second tab using Cmd+W
	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 'w', Mods: surface.ModCommand})

	// Wait for the close to select the next tab: count drops to 1 and the
	// remaining tab is the one that was open before.
	browser.waitFor(t, "closed tab hands focus to the remaining tab", func() bool {
		active := browser.f.tabMgr.Active()
		return browser.f.tabMgr.Count() == 1 && active != nil && active.ID() == firstID
	})

	active := browser.f.tabMgr.Active()
	if active == nil {
		t.Fatalf("active tab is nil after closing active tab; %s", browser.diagnostics())
	}
	if active.ID() != firstID {
		t.Fatalf("active tab ID = %d, want firstID = %d", active.ID(), firstID)
	}

	firstIDStr := strconv.FormatUint(firstID, 10)
	browser.waitFor(t, "remaining tab becomes selected in AX tree", func() bool {
		_, doc := findAXNode(browser.window.AccessibilityTree(), "first-doc")
		tabNode, ok := findAXNode(browser.window.AccessibilityTree(), "goosie.tab."+firstIDStr)
		return doc && ok && tabNode.Selected && browser.activeURL() == firstURL
	})
}
