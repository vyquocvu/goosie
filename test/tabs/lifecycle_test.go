package tabs_test

import (
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/tabs"
)

// Tab lifecycle tests cover the state management properties that the
// existing manager tests do not reach: active-index adjustment when a
// tab is closed, TabByID lookup, concurrent access safety, and the
// scroll-position preservation contract that keeps a user's place when
// they switch away and back.

// TestCloseActiveTabSelectsNearest verifies that closing the active tab
// moves the selection to the adjacent tab rather than leaving the active
// index dangling. When the active tab is the last in the list, the
// selection moves left; otherwise it stays at the same index, which
// means the next tab slides into focus.
func TestCloseActiveTabSelectsNearest(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()
	t3 := mgr.NewTab()

	// Switch to the middle tab.
	mgr.SwitchTo(t2.ID)
	if mgr.Active() != t2 {
		t.Fatal("t2 should be active")
	}

	// Closing the middle tab should leave the active index at 1, which
	// now points to t3.
	mgr.CloseTab(t2.ID)
	if mgr.Active() != t3 {
		t.Fatalf("active = %d, want t3 (%d) after closing middle tab", mgr.Active().ID, t3.ID)
	}
	if mgr.Count() != 2 {
		t.Fatalf("count = %d, want 2", mgr.Count())
	}

	// Now the active tab is the last one; closing it should move left.
	mgr.CloseTab(t3.ID)
	if mgr.Active() != t1 {
		t.Fatalf("active = %d, want t1 (%d) after closing last tab", mgr.Active().ID, t1.ID)
	}

	_ = t1
}

// TestCloseFirstTabKeepsIndexAtZero verifies that closing the tab before
// the active one shifts the active index down so the same tab stays
// active, just at a new position.
func TestCloseFirstTabKeepsIndexAtZero(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()
	_ = mgr.NewTab()

	// Active is t1 (index 0). Switch to t2 (index 1).
	mgr.SwitchTo(t2.ID)
	if mgr.ActiveIndex() != 1 {
		t.Fatalf("active index = %d, want 1", mgr.ActiveIndex())
	}

	// Close t1 (before active). Active index should drop to 0, still t2.
	mgr.CloseTab(t1.ID)
	if mgr.Active() != t2 {
		t.Fatalf("active = %d, want t2 (%d)", mgr.Active().ID, t2.ID)
	}
	if mgr.ActiveIndex() != 0 {
		t.Fatalf("active index = %d, want 0", mgr.ActiveIndex())
	}
}

// TestTabByIDReturnsCorrectTab verifies the lookup path the surface uses
// to map a tab identifier back to its state.
func TestTabByIDReturnsCorrectTab(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()
	t3 := mgr.NewTab()

	for _, tc := range []*tabs.Tab{t1, t2, t3} {
		got := mgr.TabByID(tc.ID)
		if got != tc {
			t.Errorf("TabByID(%d) = %v, want %v", tc.ID, got, tc)
		}
	}
}

// TestTabByIDReturnsNilForUnknown verifies that a bogus ID yields nil
// rather than a stale pointer.
func TestTabByIDReturnsNilForUnknown(t *testing.T) {
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	if got := mgr.TabByID(99999); got != nil {
		t.Errorf("TabByID(99999) = %v, want nil", got)
	}
}

// TestTabByIDAfterClose verifies that a closed tab is no longer
// findable, preventing the surface from operating on a dead tab.
func TestTabByIDAfterClose(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	mgr.NewTab()

	mgr.CloseTab(t1.ID)
	if got := mgr.TabByID(t1.ID); got != nil {
		t.Errorf("TabByID(%d) = %v after close, want nil", t1.ID, got)
	}
}

// TestTabScrollPositionIsIndependent verifies that each tab carries its
// own scroll offset and switching tabs does not clobber it. This is the
// contract that lets a user switch to another tab and come back to find
// their place preserved.
func TestTabScrollPositionIsIndependent(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()

	t1.ScrollY = 500
	t2.ScrollY = 1200

	mgr.SwitchTo(t2.ID)
	if t1.ScrollY != 500 {
		t.Errorf("t1.ScrollY = %d, want 500: switching away changed it", t1.ScrollY)
	}
	if t2.ScrollY != 1200 {
		t.Errorf("t2.ScrollY = %d, want 1200", t2.ScrollY)
	}

	mgr.SwitchTo(t1.ID)
	if t1.ScrollY != 500 {
		t.Errorf("t1.ScrollY = %d after switching back, want 500: scroll was not preserved", t1.ScrollY)
	}
}

// TestTabHistoryIsolation verifies that each tab's history is an
// independent instance. This is the isolation property that prevents a
// navigation in one tab from appearing in another's back/forward trail.
func TestTabHistoryIsolation(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()

	t1.History.Push("https://a.example/")
	t1.History.Push("https://a.example/page2")

	if t2.History.Current() != "" {
		t.Errorf("t2 history has %q, want empty: histories are shared", t2.History.Current())
	}
	if t1.History.Current() != "https://a.example/page2" {
		t.Errorf("t1 history current = %q, want the last pushed URL", t1.History.Current())
	}
}

// TestCloseNonexistentTabIsNoop verifies that closing an ID that does
// not exist does not panic or change the tab list.
func TestCloseNonexistentTabIsNoop(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()

	mgr.CloseTab(99999)
	if mgr.Count() != 1 {
		t.Errorf("count = %d after closing bogus ID, want 1", mgr.Count())
	}
	if mgr.Active() != t1 {
		t.Errorf("active changed after closing bogus ID")
	}
}

// TestSwitchToNonexistentTabIsNoop verifies that switching to a
// non-existent tab does not change the active tab or panic.
func TestSwitchToNonexistentTabIsNoop(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()

	mgr.SwitchTo(99999)
	if mgr.Active() != t1 {
		t.Errorf("active changed after switching to bogus ID")
	}
}

// TestConcurrentTabOperations verifies that the manager is safe under
// concurrent access. Run with -race: the value is in the report it does
// not make.
func TestConcurrentTabOperations(t *testing.T) {
	mgr := tabs.NewManager(nil)
	const goroutines = 8
	const opsPerGoroutine = 50

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				tab := mgr.NewTab()
				_ = mgr.TabByID(tab.ID)
				_ = mgr.Active()
				_ = mgr.Count()
				_ = mgr.Tabs()
				mgr.CloseTab(tab.ID)
			}
		}()
	}
	wg.Wait()

	// All tabs were created and closed, so the count should be zero.
	if mgr.Count() != 0 {
		t.Errorf("count = %d after concurrent create/close, want 0", mgr.Count())
	}
}

// TestOnChangeFiresForNewAndClose verifies that the OnChange callback
// fires for tab creation and closure, which is how the surface knows to
// redraw the tab bar.
func TestOnChangeFiresForNewAndClose(t *testing.T) {
	changes := 0
	mgr := tabs.NewManager(nil)
	mgr.OnChange = func() { changes++ }

	t1 := mgr.NewTab()
	if changes != 1 {
		t.Fatalf("changes = %d after NewTab, want 1", changes)
	}

	t2 := mgr.NewTab()
	if changes != 2 {
		t.Fatalf("changes = %d after second NewTab, want 2", changes)
	}

	mgr.CloseTab(t2.ID)
	if changes != 3 {
		t.Fatalf("changes = %d after CloseTab, want 3", changes)
	}

	_ = t1
}

// TestTabsReturnsACopy verifies that the slice returned by Tabs() cannot
// be used to mutate the manager's internal state.
func TestTabsReturnsACopy(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	mgr.NewTab()

	snapshot := mgr.Tabs()
	snapshot[0] = nil
	snapshot[1] = nil

	// The real tab list must be unaffected.
	real := mgr.Tabs()
	if real[0] != t1 || real[0] == nil {
		t.Errorf("Tabs() returned a reference, not a copy: mutating the slice changed the manager")
	}
}
