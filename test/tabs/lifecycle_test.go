package tabs_test

import (
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/tabs"
)

// qc:BR-030
func TestCloseActiveTabSelectsNearest(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()
	t3 := mgr.NewTab()

	mgr.SwitchTo(t2.ID())
	if mgr.Active() != t2 {
		t.Fatal("t2 should be active")
	}

	mgr.CloseTab(t2.ID())
	if mgr.Active() != t3 {
		t.Fatalf("active = %d, want t3 (%d) after closing middle tab", mgr.Active().ID(), t3.ID())
	}
	if mgr.Count() != 2 {
		t.Fatalf("count = %d, want 2", mgr.Count())
	}

	mgr.CloseTab(t3.ID())
	if mgr.Active() != t1 {
		t.Fatalf("active = %d, want t1 (%d) after closing last tab", mgr.Active().ID(), t1.ID())
	}

	_ = t1
}

func TestCloseFirstTabKeepsIndexAtZero(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()
	_ = mgr.NewTab()

	mgr.SwitchTo(t2.ID())
	if mgr.ActiveIndex() != 1 {
		t.Fatalf("active index = %d, want 1", mgr.ActiveIndex())
	}

	mgr.CloseTab(t1.ID())
	if mgr.Active() != t2 {
		t.Fatalf("active = %d, want t2 (%d)", mgr.Active().ID(), t2.ID())
	}
	if mgr.ActiveIndex() != 0 {
		t.Fatalf("active index = %d, want 0", mgr.ActiveIndex())
	}
}

// qc:BR-029
func TestCloseBackgroundTabKeepsActiveState(t *testing.T) {
	mgr := tabs.NewManager(nil)
	background := mgr.NewTab()
	active := mgr.NewTab()
	mgr.SwitchTo(active.ID())
	if mgr.Active() != active {
		t.Fatal("active tab should be the one switched to")
	}

	active.SetURL("https://active.example/keep")
	active.History().Push("https://active.example/keep")
	active.History().Push("https://active.example/deep")
	background.SetURL("https://background.example/")
	background.History().Push("https://background.example/")

	mgr.CloseTab(background.ID())

	if mgr.Count() != 1 {
		t.Fatalf("count = %d after closing background tab, want 1", mgr.Count())
	}
	if mgr.Active() != active {
		t.Fatalf("active = %v after closing background tab, want the same tab", mgr.Active())
	}
	if got := active.URL(); got != "https://active.example/keep" {
		t.Fatalf("active URL = %q after closing background tab, want unchanged", got)
	}
	if got := active.History().Current(); got != "https://active.example/deep" {
		t.Fatalf("active history current = %q after closing background tab, want unchanged", got)
	}
}

func TestTabByIDReturnsCorrectTab(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()
	t3 := mgr.NewTab()

	for _, tc := range []*tabs.Tab{t1, t2, t3} {
		got := mgr.TabByID(tc.ID())
		if got != tc {
			t.Errorf("TabByID(%d) = %v, want %v", tc.ID(), got, tc)
		}
	}
}

func TestTabByIDReturnsNilForUnknown(t *testing.T) {
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	if got := mgr.TabByID(99999); got != nil {
		t.Errorf("TabByID(99999) = %v, want nil", got)
	}
}

func TestTabByIDAfterClose(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	mgr.NewTab()

	mgr.CloseTab(t1.ID())
	if got := mgr.TabByID(t1.ID()); got != nil {
		t.Errorf("TabByID(%d) = %v after close, want nil", t1.ID(), got)
	}
}

func TestTabScrollPositionIsIndependent(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()

	t1.SetScrollY(500)
	t2.SetScrollY(1200)

	mgr.SwitchTo(t2.ID())
	if t1.ScrollY() != 500 {
		t.Errorf("t1.ScrollY = %d, want 500: switching away changed it", t1.ScrollY())
	}
	if t2.ScrollY() != 1200 {
		t.Errorf("t2.ScrollY = %d, want 1200", t2.ScrollY())
	}

	mgr.SwitchTo(t1.ID())
	if t1.ScrollY() != 500 {
		t.Errorf("t1.ScrollY = %d after switching back, want 500: scroll was not preserved", t1.ScrollY())
	}
}

func TestTabHistoryIsolation(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()

	t1.History().Push("https://a.example/")
	t1.History().Push("https://a.example/page2")

	if t2.History().Current() != "" {
		t.Errorf("t2 history has %q, want empty: histories are shared", t2.History().Current())
	}
	if t1.History().Current() != "https://a.example/page2" {
		t.Errorf("t1 history current = %q, want the last pushed URL", t1.History().Current())
	}
}

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

func TestSwitchToNonexistentTabIsNoop(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()

	mgr.SwitchTo(99999)
	if mgr.Active() != t1 {
		t.Errorf("active changed after switching to bogus ID")
	}
}

// qc:BR-038
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
				_ = mgr.TabByID(tab.ID())
				_ = mgr.Active()
				_ = mgr.Count()
				_ = mgr.Tabs()
				mgr.CloseTab(tab.ID())
			}
		}()
	}
	wg.Wait()

	if mgr.Count() != 0 {
		t.Errorf("count = %d after concurrent create/close, want 0", mgr.Count())
	}
}

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

	mgr.CloseTab(t2.ID())
	if changes != 3 {
		t.Fatalf("changes = %d after CloseTab, want 3", changes)
	}

	_ = t1
}

func TestTabsReturnsACopy(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	mgr.NewTab()

	snapshot := mgr.Tabs()
	snapshot[0] = nil
	snapshot[1] = nil

	real := mgr.Tabs()
	if real[0] != t1 || real[0] == nil {
		t.Errorf("Tabs() returned a reference, not a copy: mutating the slice changed the manager")
	}
}
