package main

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
)

func TestReopenTabShortcut(t *testing.T) {
	mgr := tabs.NewManager(nil)
	mgr.NewTab()

	var newTabCalls int
	var reopenTabCalls int
	var closeTabCalls int

	cw := &chromeWindow{
		tabMgr:      mgr,
		onNewTab:    func() { newTabCalls++ },
		onReopenTab: func() { reopenTabCalls++ },
		onCloseTab:  func(uint64) { closeTabCalls++ },
	}

	// Cmd+T -> onNewTab
	handled := cw.handleTabShortcut('t', surface.ModCommand)
	if !handled || newTabCalls != 1 || reopenTabCalls != 0 {
		t.Fatalf("Cmd+T: handled=%v, newTabs=%d, reopenTabs=%d", handled, newTabCalls, reopenTabCalls)
	}

	// Cmd+Shift+T -> onReopenTab
	handled = cw.handleTabShortcut('t', surface.ModCommand|surface.ModShift)
	if !handled || newTabCalls != 1 || reopenTabCalls != 1 {
		t.Fatalf("Cmd+Shift+T ('t'): handled=%v, newTabs=%d, reopenTabs=%d", handled, newTabCalls, reopenTabCalls)
	}

	// Cmd+Shift+T (uppercase 'T') -> onReopenTab
	handled = cw.handleTabShortcut('T', surface.ModCommand|surface.ModShift)
	if !handled || newTabCalls != 1 || reopenTabCalls != 2 {
		t.Fatalf("Cmd+Shift+T ('T'): handled=%v, newTabs=%d, reopenTabs=%d", handled, newTabCalls, reopenTabCalls)
	}

	// Cmd+W -> onCloseTab
	handled = cw.handleTabShortcut('w', surface.ModCommand)
	if !handled || closeTabCalls != 1 {
		t.Fatalf("Cmd+W: handled=%v, closeTabs=%d", handled, closeTabCalls)
	}
}

func TestFramePathReopenTab(t *testing.T) {
	f := newNavTestPath(t)
	// Create a second tab so we can close one without emptying the manager
	tab2 := f.tabMgr.NewTab()
	tab2.SetURL("https://example.com/page2")
	tab2.SetTitle("Page Two")
	tab2.History().Push("https://example.com/page2")
	tab2.SetScrollY(240)

	f.tabMgr.SwitchTo(tab2.ID())
	if f.tabMgr.Active() != tab2 {
		t.Fatal("Active tab should be tab2")
	}

	// Close tab2
	f.closeTab(tab2.ID())
	if f.tabMgr.Count() != 1 {
		t.Fatalf("Tab count after close = %d, want 1", f.tabMgr.Count())
	}
	if f.tabMgr.ClosedTabCount() != 1 {
		t.Fatalf("ClosedTabCount() = %d, want 1", f.tabMgr.ClosedTabCount())
	}

	// Call reopenTab
	f.reopenTab()
	if f.tabMgr.Count() != 2 {
		t.Fatalf("Tab count after reopen = %d, want 2", f.tabMgr.Count())
	}

	reopened := f.tabMgr.Active()
	if reopened == nil {
		t.Fatal("Active tab after reopen is nil")
	}
	if reopened.URL() != "https://example.com/page2" {
		t.Errorf("Reopened URL = %q, want %q", reopened.URL(), "https://example.com/page2")
	}
	if reopened.Title() != "Page Two" {
		t.Errorf("Reopened Title = %q, want %q", reopened.Title(), "Page Two")
	}
	if reopened.ScrollY() != 240 {
		t.Errorf("Reopened ScrollY = %d, want 240", reopened.ScrollY())
	}
	if f.tabMgr.ClosedTabCount() != 0 {
		t.Errorf("ClosedTabCount() after reopen = %d, want 0", f.tabMgr.ClosedTabCount())
	}
}
