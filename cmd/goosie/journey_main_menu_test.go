package main

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/toolbar"
)

// TestJourneyMainMenuOpensManagers drives the chrome menu through the real
// window wiring: press the menu button, click Bookmarks, then History, and
// assert the active tab navigates to the internal managers (BR-052).
func TestJourneyMainMenuOpensManagers(t *testing.T) {
	srvURL := findJourneyPage(t, "<p>main menu journey</p>", "")
	b := newBrowserE2E(t)
	b.navigateViaAddressBar(t, srvURL)
	b.waitFor(t, "page loaded", func() bool { return b.activeURL() == srvURL })

	cw, ok := b.f.window.(*chromeWindow)
	if !ok {
		t.Fatalf("window type = %T, want *chromeWindow", b.f.window)
	}

	b.clickToolbarButton(t, toolbar.ButtonMenu)
	b.waitFor(t, "main menu visible", func() bool { return cw.mainMenu != nil && cw.mainMenu.Visible })

	pt, ok := menuItemPoint(cw.mainMenu, "bookmarks", cw.sc())
	if !ok {
		t.Fatal("main menu has no Bookmarks item")
	}
	b.click(t, pt.X, pt.Y)
	b.waitFor(t, "bookmarks opened", func() bool { return b.activeURL() == "goosie://bookmarks" })

	b.clickToolbarButton(t, toolbar.ButtonMenu)
	b.waitFor(t, "main menu visible again", func() bool { return cw.mainMenu.Visible })
	pt, ok = menuItemPoint(cw.mainMenu, "history", cw.sc())
	if !ok {
		t.Fatal("main menu has no History item")
	}
	b.click(t, pt.X, pt.Y)
	b.waitFor(t, "history opened", func() bool { return b.activeURL() == "goosie://history" })
}
