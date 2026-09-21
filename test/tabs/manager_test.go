package tabs_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/tabs"
)

func TestNewTabHasCorrectInitialState(t *testing.T) {
	mgr := tabs.NewManager(nil)
	tab := mgr.NewTab()

	if tab.ID == 0 {
		t.Fatal("new tab should have non-zero ID")
	}
	if tab.URL != "" {
		t.Fatalf("new tab URL should be empty, got %q", tab.URL)
	}
	if tab.Title != "New Tab" {
		t.Fatalf("new tab title should be 'New Tab', got %q", tab.Title)
	}
	if tab.Loading {
		t.Fatal("new tab should not be loading")
	}
	if tab.ScrollY != 0 {
		t.Fatalf("new tab scroll should be 0, got %d", tab.ScrollY)
	}
	if tab.History == nil {
		t.Fatal("new tab should have a history")
	}
}

func TestTabIDsAreUniqueAndMonotonic(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()
	t3 := mgr.NewTab()

	if t1.ID >= t2.ID || t2.ID >= t3.ID {
		t.Fatalf("IDs should be monotonically increasing: %d, %d, %d", t1.ID, t2.ID, t3.ID)
	}
}

func TestActiveReturnsCorrectTab(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	_ = mgr.NewTab()

	if mgr.Active() != t1 {
		t.Fatal("first tab should be active after creation")
	}
}

func TestNewTabStartsWithOwnHistory(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()

	if t1.History == t2.History {
		t.Fatal("each tab should have its own history instance")
	}
	t1.History.Push("https://example.com")
	if t2.History.Current() != "" {
		t.Fatal("pushing to t1 history should not affect t2")
	}
}

func TestCloseTabRemovesTab(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	_ = mgr.NewTab()

	mgr.CloseTab(t1.ID)

	if mgr.Count() != 1 {
		t.Fatalf("expected 1 tab, got %d", mgr.Count())
	}
	if mgr.Active().ID == t1.ID {
		t.Fatal("closed tab should not be active")
	}
}

func TestCloseLastTabCallsOnCloseLast(t *testing.T) {
	done := make(chan struct{})
	mgr := tabs.NewManager(func() { close(done) })
	t1 := mgr.NewTab()

	mgr.CloseTab(t1.ID)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("onCloseLast should be called when last tab closes")
	}
	if mgr.Count() != 0 {
		t.Fatal("tab count should be 0")
	}
}

func TestSwitchToChangesActiveTab(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()

	if mgr.Active() != t1 {
		t.Fatal("t1 should be active initially")
	}

	mgr.SwitchTo(t2.ID)
	if mgr.Active() != t2 {
		t.Fatal("t2 should be active after SwitchTo")
	}

	mgr.SwitchTo(t1.ID)
	if mgr.Active() != t1 {
		t.Fatal("t1 should be active after switching back")
	}
}

func TestSwitchToFiresOnChange(t *testing.T) {
	changes := 0
	mgr := tabs.NewManager(nil)
	mgr.OnChange = func() { changes++ }
	t1 := mgr.NewTab()
	_ = mgr.NewTab()

	before := changes
	mgr.SwitchTo(t1.ID)
	if changes != before {
		t.Fatal("switching to already-active tab should not fire OnChange")
	}

	t2ID := mgr.Tabs()[1].ID
	mgr.SwitchTo(t2ID)
	if changes != before+1 {
		t.Fatal("switching to different tab should fire OnChange once")
	}
}

func TestCount(t *testing.T) {
	mgr := tabs.NewManager(nil)
	if mgr.Count() != 0 {
		t.Fatal("empty manager should have count 0")
	}
	mgr.NewTab()
	mgr.NewTab()
	if mgr.Count() != 2 {
		t.Fatalf("expected 2, got %d", mgr.Count())
	}
}

func TestTabsReturnsAllTabs(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()

	all := mgr.Tabs()
	if len(all) != 2 {
		t.Fatalf("expected 2 tabs, got %d", len(all))
	}
	if all[0] != t1 || all[1] != t2 {
		t.Fatal("Tabs should return tabs in order")
	}
}
