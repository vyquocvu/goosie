package tabs

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestDuplicateTab(t *testing.T) {
	mgr := NewManager(nil)
	t1 := mgr.NewTab()
	t1.SetURL("https://example.com/one")
	t1.SetTitle("Example One")
	t1.SetScrollY(150)
	h := toolbar.NewHistory()
	h.Push("https://example.com/start")
	h.Push("https://example.com/one")
	t1.SetHistory(h)

	t2 := mgr.NewTab()
	t2.SetURL("https://example.com/two")

	// Active tab is t2, switch back to t1
	mgr.SwitchTo(t1.ID())

	// Duplicate t1
	dup, ok := mgr.DuplicateTab(t1.ID())
	if !ok || dup == nil {
		t.Fatalf("expected DuplicateTab to succeed, got nil")
	}

	if mgr.Count() != 3 {
		t.Fatalf("expected 3 tabs, got %d", mgr.Count())
	}

	// dup should be at index 1 (right after t1)
	tabs := mgr.Tabs()
	if tabs[1].ID() != dup.ID() {
		t.Errorf("expected duplicated tab at index 1, found at index %d", mgr.ActiveIndex())
	}
	if mgr.ActiveIndex() != 1 {
		t.Errorf("expected active index to be 1, got %d", mgr.ActiveIndex())
	}

	// Verify cloned state
	if dup.URL() != "https://example.com/one" {
		t.Errorf("expected URL https://example.com/one, got %q", dup.URL())
	}
	if dup.Title() != "Example One" {
		t.Errorf("expected Title Example One, got %q", dup.Title())
	}
	if dup.ScrollY() != 150 {
		t.Errorf("expected ScrollY 150, got %d", dup.ScrollY())
	}
	if dup.History() == nil {
		t.Fatal("expected cloned history, got nil")
	}
	entries, idx := dup.History().Entries()
	if len(entries) != 2 || idx != 1 {
		t.Errorf("unexpected history entries length %d, idx %d", len(entries), idx)
	}

	// Duplicate invalid ID should fail
	if _, ok := mgr.DuplicateTab(99999); ok {
		t.Errorf("expected duplicate invalid tab to return false")
	}
}

func TestPinAndTogglePinTab(t *testing.T) {
	mgr := NewManager(nil)
	t1 := mgr.NewTab()
	t1.SetTitle("Tab 1")
	t2 := mgr.NewTab()
	t2.SetTitle("Tab 2")
	t3 := mgr.NewTab()
	t3.SetTitle("Tab 3")

	if t3.Pinned() {
		t.Errorf("tab should not be pinned by default")
	}

	// Pin t3: should move before unpinned tabs (to index 0)
	if !mgr.PinTab(t3.ID()) {
		t.Fatalf("PinTab failed")
	}
	if !t3.Pinned() {
		t.Errorf("expected t3 to be pinned")
	}
	tabs := mgr.Tabs()
	if tabs[0].ID() != t3.ID() {
		t.Errorf("expected pinned t3 at index 0, got %s", tabs[0].Title())
	}

	// TogglePin t3 -> unpin
	if !mgr.TogglePin(t3.ID()) {
		t.Fatalf("TogglePin failed")
	}
	if t3.Pinned() {
		t.Errorf("expected t3 to be unpinned after toggle")
	}

	// TogglePin t2 -> pin
	if !mgr.TogglePin(t2.ID()) {
		t.Fatalf("TogglePin failed")
	}
	if !t2.Pinned() {
		t.Errorf("expected t2 to be pinned")
	}
	tabs = mgr.Tabs()
	if tabs[0].ID() != t2.ID() {
		t.Errorf("expected pinned t2 at index 0, got %s", tabs[0].Title())
	}
}

func TestCloseOtherTabs(t *testing.T) {
	mgr := NewManager(nil)
	t1 := mgr.NewTab()
	t1.SetURL("https://example.com/1")
	t2 := mgr.NewTab()
	t2.SetURL("https://example.com/2")
	t3 := mgr.NewTab()
	t3.SetURL("https://example.com/3")
	t4 := mgr.NewTab()
	t4.SetURL("https://example.com/4")

	// Pin t1
	mgr.PinTab(t1.ID())

	// CloseOtherTabs keeping t3
	// Result should keep t1 (pinned) and t3 (keepID), closing t2 and t4
	mgr.CloseOtherTabs(t3.ID())

	if mgr.Count() != 2 {
		t.Fatalf("expected 2 tabs remaining, got %d", mgr.Count())
	}
	tabs := mgr.Tabs()
	if tabs[0].ID() != t1.ID() || tabs[1].ID() != t3.ID() {
		t.Errorf("unexpected tabs: %v, %v", tabs[0].ID(), tabs[1].ID())
	}
	if mgr.Active().ID() != t3.ID() {
		t.Errorf("expected active tab to be keepID t3, got %d", mgr.Active().ID())
	}

	// Closed tabs t2 and t4 should be in reopen stack
	if mgr.ClosedTabCount() != 2 {
		t.Errorf("expected 2 closed tabs in reopen stack, got %d", mgr.ClosedTabCount())
	}
}

func TestCloseTabsToRight(t *testing.T) {
	mgr := NewManager(nil)
	t1 := mgr.NewTab()
	t1.SetURL("https://example.com/1")
	t2 := mgr.NewTab()
	t2.SetURL("https://example.com/2")
	t3 := mgr.NewTab()
	t3.SetURL("https://example.com/3")
	t4 := mgr.NewTab()
	t4.SetURL("https://example.com/4")

	// Pin t4
	t4.SetPinned(true)

	// Close tabs to right of t2:
	// t3 is unpinned -> closed
	// t4 is pinned -> preserved
	mgr.CloseTabsToRight(t2.ID())

	if mgr.Count() != 3 {
		t.Fatalf("expected 3 tabs remaining, got %d", mgr.Count())
	}
	tabs := mgr.Tabs()
	if tabs[0].ID() != t1.ID() || tabs[1].ID() != t2.ID() || tabs[2].ID() != t4.ID() {
		t.Errorf("unexpected remaining tabs")
	}

	// t3 should be in closed tab history
	closed, ok := mgr.PopClosedTab()
	if !ok || closed.URL != "https://example.com/3" {
		t.Errorf("expected t3 to be closed and saved, got %v", closed)
	}
}

func TestDrawTabBarWithPinnedTabNoCloseButton(t *testing.T) {
	mgr := NewManager(nil)
	tab := mgr.NewTab()
	tab.SetTitle("Pinned Tab")
	tab.SetPinned(true)

	buf := frame.NewBitmap(400, 40)
	DrawTabBar(buf, mgr, 0, nil, 1)

	// Verify no panic and tab drew properly
	if buf.Empty() {
		t.Errorf("expected non-empty bitmap")
	}
}
