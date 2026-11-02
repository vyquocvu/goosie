package main

import (
	"strconv"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// menuItemsByAction indexes the actionable entries so a test can name the one
// it cares about instead of counting separators.
func menuItemsByAction(items []ContextMenuItem) map[string]ContextMenuItem {
	byAction := make(map[string]ContextMenuItem, len(items))
	for _, it := range items {
		if it.Action != "" {
			byAction[it.Action] = it
		}
	}
	return byAction
}

// menuItemPoint returns a point inside the band painted for action, so a test
// clicks the item the pixels show rather than a hardcoded row.
func menuItemPoint(cm *ContextMenu, action string, scale int32) (frame.Point, bool) {
	y := cm.Y + int32(menuPadding)*scale
	for _, it := range cm.Items {
		h := int32(menuItemHeight) * scale
		if it.Separator {
			h = 6 * scale
		}
		if it.Action == action {
			return frame.Point{X: cm.X + 20*scale, Y: y + h/2}, true
		}
		y += h
	}
	return frame.Point{}, false
}

func TestBuildTabContextMenuItems(t *testing.T) {
	mgr := tabs.NewManager(nil)
	first := mgr.NewTab()
	second := mgr.NewTab()
	third := mgr.NewTab()
	first.SetURL("https://example.com/")
	first.SetPinned(true)

	items := BuildTabContextMenuItems(first, 0, 3, true)
	byAction := menuItemsByAction(items)
	for _, action := range []string{
		"tab_new", "tab_reload", "tab_duplicate", "tab_toggle_pin",
		"tab_close", "tab_close_others", "tab_close_right", "tab_reopen",
	} {
		if _, ok := byAction[action]; !ok {
			t.Errorf("menu missing action %q", action)
		}
	}
	if len(byAction) != 8 {
		t.Errorf("menu has %d actionable items, want 8", len(byAction))
	}

	wantID := strconv.FormatUint(first.ID(), 10)
	for action, it := range byAction {
		if action == "tab_new" {
			continue
		}
		if it.Data != wantID {
			t.Errorf("%s data = %q, want the clicked tab's id %q", action, it.Data, wantID)
		}
	}
	if got := byAction["tab_toggle_pin"].Label; got != "Unpin Tab" {
		t.Errorf("pinned tab's toggle label = %q, want Unpin Tab", got)
	}
	if byAction["tab_reopen"].Disabled {
		t.Error("Reopen Closed Tab disabled with a closed tab available")
	}
	if byAction["tab_close_right"].Disabled {
		t.Error("Close Tabs to the Right disabled for the leftmost of three tabs")
	}
	if byAction["tab_reload"].Disabled {
		t.Error("Reload disabled for a tab with a URL")
	}

	unpinned := menuItemsByAction(BuildTabContextMenuItems(second, 2, 3, true))
	if got := unpinned["tab_toggle_pin"].Label; got != "Pin Tab" {
		t.Errorf("unpinned tab's toggle label = %q, want Pin Tab", got)
	}
	if !unpinned["tab_close_right"].Disabled {
		t.Error("Close Tabs to the Right enabled for the rightmost tab")
	}
	if unpinned["tab_reopen"].Disabled {
		t.Error("Reopen Closed Tab disabled with a closed tab available")
	}
	if !unpinned["tab_reload"].Disabled {
		t.Error("Reload enabled for a tab with no URL")
	}

	single := menuItemsByAction(BuildTabContextMenuItems(third, 0, 1, false))
	if !single["tab_close_others"].Disabled {
		t.Error("Close Other Tabs enabled with a single tab")
	}
	if !single["tab_reopen"].Disabled {
		t.Error("Reopen Closed Tab enabled with no closed tab available")
	}
}

func TestTabContextMenuRightClickTargetsTab(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("no fonts: %v", err)
	}
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	second := mgr.NewTab()
	tb := toolbar.NewState(800, fonts)
	cw := newChromeWindow(&capWin{}, tb, mgr,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fonts, 1)

	var gotAction, gotData string
	cw.onTabMenuAction = func(action, data string) {
		gotAction, gotData = action, data
	}
	var contentAction string
	cw.onContextMenuAction = func(action, data string) {
		contentAction = action
	}

	rect := tabs.TabRect(1, 0, 2, 800)
	pos := frame.Point{X: (rect.X0 + rect.X1) / 2, Y: (rect.Y0 + rect.Y1) / 2}
	if !cw.intercept(surface.Event{Kind: surface.EvPointer, Action: surface.PointerPress, Button: surface.ButtonRight, Pos: pos}) {
		t.Fatal("right press on the tab strip was not consumed")
	}
	if !cw.contextMenu.Visible {
		t.Fatal("right press on a tab did not open a context menu")
	}
	if got := cw.contextMenu.Items; len(got) == 0 || got[0].Action != "tab_new" {
		t.Fatalf("menu items = %+v, want the tab actions menu", got)
	}
	if got := menuItemsByAction(cw.contextMenu.Items)["tab_close"].Data; got != strconv.FormatUint(second.ID(), 10) {
		t.Fatalf("Close Tab targets %q, want the right-clicked tab %d", got, second.ID())
	}

	closePt, ok := menuItemPoint(cw.contextMenu, "tab_close", 1)
	if !ok {
		t.Fatal("menu has no Close Tab item to click")
	}
	if !cw.intercept(surface.Event{Kind: surface.EvPointer, Action: surface.PointerPress, Button: surface.ButtonLeft, Pos: closePt}) {
		t.Fatal("click on a tab menu item was not consumed")
	}
	if gotAction != "tab_close" || gotData != strconv.FormatUint(second.ID(), 10) {
		t.Fatalf("dispatched %q/%q, want tab_close/%d", gotAction, gotData, second.ID())
	}
	if cw.contextMenu.Visible {
		t.Error("context menu stayed open after choosing an item")
	}
	if contentAction != "" {
		t.Errorf("tab action %q was also dispatched as a content action", contentAction)
	}

	// A press on empty strip space targets nothing and must not open the menu.
	if !cw.intercept(surface.Event{Kind: surface.EvPointer, Action: surface.PointerPress, Button: surface.ButtonRight,
		Pos: frame.Point{X: 700, Y: 18}}) {
		t.Fatal("right press on the strip was not consumed")
	}
	if cw.contextMenu.Visible {
		t.Fatal("right press on empty strip space opened a tab menu")
	}

	cw.showTabMenuAt(second.ID(), pos)
	cw.intercept(surface.Event{Kind: surface.EvKey, Key: 0x1B})
	if cw.contextMenu.Visible {
		t.Error("Escape did not close the tab menu")
	}
}

func tabMenuTestPath(t *testing.T) *framePath {
	t.Helper()
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	return f
}

func tabActionID(tab *tabs.Tab) string {
	return strconv.FormatUint(tab.ID(), 10)
}

func TestTabContextMenuCloseOtherTabs(t *testing.T) {
	f := tabMenuTestPath(t)
	keep := f.tabMgr.NewTab()
	pinned := f.tabMgr.NewTab()
	pinned.SetPinned(true)
	f.tabMgr.SwitchTo(keep.ID())

	f.handleTabContextMenuAction("tab_close_others", tabActionID(keep))

	if got := f.tabMgr.Count(); got != 2 {
		t.Fatalf("tab count = %d, want 2 (kept tab plus pinned tab)", got)
	}
	if f.tabMgr.TabByID(keep.ID()) == nil {
		t.Error("Close Other Tabs closed the tab it was asked to keep")
	}
	if f.tabMgr.TabByID(pinned.ID()) == nil {
		t.Error("Close Other Tabs closed a pinned tab")
	}
	if got := f.tabMgr.Active(); got != keep {
		t.Errorf("active tab = %v, want the kept tab %d", got, keep.ID())
	}
}

func TestTabContextMenuCloseTabsToRight(t *testing.T) {
	f := tabMenuTestPath(t)
	first := f.tabMgr.Active()
	right := f.tabMgr.NewTab()
	pinnedRight := f.tabMgr.NewTab()
	pinnedRight.SetPinned(true)

	f.handleTabContextMenuAction("tab_close_right", tabActionID(first))

	if got := f.tabMgr.Count(); got != 2 {
		t.Fatalf("tab count = %d, want 2 (tab plus pinned tab to its right)", got)
	}
	if f.tabMgr.TabByID(right.ID()) != nil {
		t.Error("Close Tabs to the Right closed the unpinned tab beside it")
	}
	if f.tabMgr.TabByID(pinnedRight.ID()) == nil {
		t.Error("Close Tabs to the Right closed a pinned tab")
	}
}

func TestTabContextMenuTogglePin(t *testing.T) {
	f := tabMenuTestPath(t)
	tab := f.tabMgr.Active()

	f.handleTabContextMenuAction("tab_toggle_pin", tabActionID(tab))
	if !tab.Pinned() {
		t.Fatal("toggle pin did not pin the tab")
	}
	f.handleTabContextMenuAction("tab_toggle_pin", tabActionID(tab))
	if tab.Pinned() {
		t.Fatal("toggle pin did not unpin the tab")
	}
}

func TestTabContextMenuCloseAndReopen(t *testing.T) {
	f := tabMenuTestPath(t)
	tab := f.tabMgr.Active()
	tab.History().Push("https://example.com/")

	f.handleTabContextMenuAction("tab_close", tabActionID(tab))
	if got := f.tabMgr.Count(); got != 0 {
		t.Fatalf("tab count after close = %d, want 0", got)
	}

	f.handleTabContextMenuAction("tab_reopen", "")
	if got := f.tabMgr.Count(); got != 1 {
		t.Fatalf("tab count after reopen = %d, want 1", got)
	}
	entries, _ := f.tabMgr.Active().History().Entries()
	if len(entries) != 1 || entries[0] != "https://example.com/" {
		t.Errorf("reopened history = %v, want the closed tab's entry", entries)
	}
}

func TestTabContextMenuDuplicateTab(t *testing.T) {
	f := tabMenuTestPath(t)
	source := f.tabMgr.Active()

	f.handleTabContextMenuAction("tab_duplicate", tabActionID(source))

	if got := f.tabMgr.Count(); got != 2 {
		t.Fatalf("tab count after duplicate = %d, want 2", got)
	}
	if f.tabMgr.Active() == source {
		t.Error("duplicate did not activate the new tab")
	}
}

func TestTabContextMenuIgnoresBadTarget(t *testing.T) {
	f := tabMenuTestPath(t)
	before := f.tabMgr.Count()

	f.handleTabContextMenuAction("tab_close", "not-a-number")

	if got := f.tabMgr.Count(); got != before {
		t.Fatalf("tab count = %d, want %d; a malformed target changed the strip", got, before)
	}
}
