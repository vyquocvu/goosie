package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// qc:BR-034
func TestJourneySwitchTabRestoresScroll(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/tab1":
			fmt.Fprint(w, `<!doctype html><html><body style="margin:0;padding:0;">
<h1 id="tab1-head">Tab 1 Heading</h1>
<div style="height: 2500px;">Tab 1 Tall Spacer</div>
<p id="tab1-foot">Tab 1 Bottom Footer</p>
</body></html>`)
		case "/tab2":
			fmt.Fprint(w, `<!doctype html><html><body style="margin:0;padding:0;">
<h1 id="tab2-head">Tab 2 Heading</h1>
<div style="height: 2500px;">Tab 2 Tall Spacer</div>
<p id="tab2-foot">Tab 2 Bottom Footer</p>
</body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	url1 := server.URL + "/tab1"
	url2 := server.URL + "/tab2"

	browser.navigateViaAddressBar(t, url1)
	browser.waitFor(t, "tab 1 initial document loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "tab1-head")
		return ok && browser.activeURL() == url1
	})
	tab1ID := browser.f.tabMgr.Active().ID()

	browser.pressKey(t, 0x1b)
	browser.waitFor(t, "address bar blurred", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "goosie.address")
		return ok && !node.Focused
	})

	browser.pressKey(t, keyPageDown)
	browser.waitFor(t, "tab 1 scrolled down", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil || active.ID() != tab1ID {
			return false
		}
		return active.ScrollY() > 0 && browser.f.sched.Viewport().Offset.Y > 0
	})
	tab1Scroll := browser.f.tabMgr.Active().ScrollY()

	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 't', Mods: surface.ModCommand})
	browser.waitFor(t, "tab 2 created and focused", func() bool {
		active := browser.f.tabMgr.Active()
		return active != nil && browser.f.tabMgr.Count() == 2 && active.ID() != tab1ID
	})
	tab2ID := browser.f.tabMgr.Active().ID()

	browser.navigateViaAddressBar(t, url2)
	browser.waitFor(t, "tab 2 loaded at top", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil || active.ID() != tab2ID {
			return false
		}
		_, ok := findAXNode(browser.window.AccessibilityTree(), "tab2-head")
		return ok && browser.activeURL() == url2 && active.ScrollY() == 0
	})

	browser.pressKey(t, 0x1b)
	browser.waitFor(t, "tab 2 address bar blurred", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "goosie.address")
		return ok && !node.Focused
	})

	browser.pressKey(t, keyPageDown)
	browser.waitFor(t, "tab 2 first page down registered", func() bool {
		active := browser.f.tabMgr.Active()
		return active != nil && active.ID() == tab2ID && active.ScrollY() > 0
	})
	browser.pressKey(t, keyPageDown)
	browser.waitFor(t, "tab 2 scrolled to distinct offset", func() bool {
		active := browser.f.tabMgr.Active()
		return active != nil && active.ID() == tab2ID && active.ScrollY() > tab1Scroll
	})
	tab2Scroll := browser.f.tabMgr.Active().ScrollY()

	tab1IDStr := strconv.FormatUint(tab1ID, 10)
	tab2IDStr := strconv.FormatUint(tab2ID, 10)

	browser.clickNode(t, "goosie.tab."+tab1IDStr)

	browser.waitFor(t, "switch to tab 1 restores document and scroll offset", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil || active.ID() != tab1ID {
			return false
		}
		_, ok := findAXNode(browser.window.AccessibilityTree(), "tab1-head")
		return ok && browser.activeURL() == url1 && active.ScrollY() == tab1Scroll && browser.f.sched.Viewport().Offset.Y == tab1Scroll
	})

	browser.clickNode(t, "goosie.tab."+tab2IDStr)
	browser.waitFor(t, "switch to tab 2 restores document and scroll offset", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil || active.ID() != tab2ID {
			return false
		}
		_, ok := findAXNode(browser.window.AccessibilityTree(), "tab2-head")
		return ok && browser.activeURL() == url2 && active.ScrollY() == tab2Scroll && browser.f.sched.Viewport().Offset.Y == tab2Scroll
	})
}

// qc:BR-035
func TestJourneyTabsIndependentHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/t1-first":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="t1-p1">Tab 1 Page 1</h1></body></html>`)
		case "/t1-second":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="t1-p2">Tab 1 Page 2</h1></body></html>`)
		case "/t2-alpha":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="t2-pa">Tab 2 Page A</h1></body></html>`)
		case "/t2-beta":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="t2-pb">Tab 2 Page B</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	t1URL1 := server.URL + "/t1-first"
	t1URL2 := server.URL + "/t1-second"
	t2URL1 := server.URL + "/t2-alpha"
	t2URL2 := server.URL + "/t2-beta"

	browser.navigateViaAddressBar(t, t1URL1)
	browser.waitFor(t, "tab 1 page 1 loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "t1-p1")
		return ok && browser.activeURL() == t1URL1
	})
	tab1ID := browser.f.tabMgr.Active().ID()

	browser.navigateViaAddressBar(t, t1URL2)
	browser.waitFor(t, "tab 1 page 2 loaded with 2 history entries", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil || active.ID() != tab1ID {
			return false
		}
		entries, idx := active.History().Entries()
		_, ok := findAXNode(browser.window.AccessibilityTree(), "t1-p2")
		return ok && browser.activeURL() == t1URL2 && len(entries) == 2 && idx == 1
	})

	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 't', Mods: surface.ModCommand})
	browser.waitFor(t, "tab 2 created and active", func() bool {
		active := browser.f.tabMgr.Active()
		return active != nil && browser.f.tabMgr.Count() == 2 && active.ID() != tab1ID
	})
	tab2ID := browser.f.tabMgr.Active().ID()

	browser.navigateViaAddressBar(t, t2URL1)
	browser.waitFor(t, "tab 2 page 1 loaded", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil || active.ID() != tab2ID {
			return false
		}
		_, ok := findAXNode(browser.window.AccessibilityTree(), "t2-pa")
		return ok && browser.activeURL() == t2URL1
	})

	browser.navigateViaAddressBar(t, t2URL2)
	browser.waitFor(t, "tab 2 page 2 loaded with 2 history entries", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil || active.ID() != tab2ID {
			return false
		}
		entries, idx := active.History().Entries()
		_, ok := findAXNode(browser.window.AccessibilityTree(), "t2-pb")
		return ok && browser.activeURL() == t2URL2 && len(entries) == 2 && idx == 1
	})

	browser.clickToolbarButton(t, toolbar.ButtonBack)
	browser.waitFor(t, "tab 2 navigated back to page A", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil || active.ID() != tab2ID {
			return false
		}
		_, ok := findAXNode(browser.window.AccessibilityTree(), "t2-pa")
		return ok && browser.activeURL() == t2URL1 && !active.History().CanBack() && active.History().CanForward()
	})

	tab1IDStr := strconv.FormatUint(tab1ID, 10)
	tab2IDStr := strconv.FormatUint(tab2ID, 10)

	browser.clickNode(t, "goosie.tab."+tab1IDStr)

	browser.waitFor(t, "tab 1 has preserved history unaffected by tab 2", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil || active.ID() != tab1ID {
			return false
		}
		entries, idx := active.History().Entries()
		_, ok := findAXNode(browser.window.AccessibilityTree(), "t1-p2")
		return ok && browser.activeURL() == t1URL2 &&
			len(entries) == 2 && idx == 1 &&
			entries[0] == t1URL1 && entries[1] == t1URL2 &&
			active.History().CanBack() && !active.History().CanForward()
	})

	browser.clickToolbarButton(t, toolbar.ButtonBack)
	browser.waitFor(t, "tab 1 navigates back to its own page 1", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil || active.ID() != tab1ID {
			return false
		}
		_, ok := findAXNode(browser.window.AccessibilityTree(), "t1-p1")
		return ok && browser.activeURL() == t1URL1 && !active.History().CanBack() && active.History().CanForward()
	})

	browser.clickNode(t, "goosie.tab."+tab2IDStr)
	browser.waitFor(t, "tab 2 still retains its own forward history", func() bool {
		active := browser.f.tabMgr.Active()
		if active == nil || active.ID() != tab2ID {
			return false
		}
		_, ok := findAXNode(browser.window.AccessibilityTree(), "t2-pa")
		return ok && browser.activeURL() == t2URL1 && !active.History().CanBack() && active.History().CanForward()
	})
}
