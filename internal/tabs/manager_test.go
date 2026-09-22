package tabs

import "testing"

func TestTabByID(t *testing.T) {
	mgr := NewManager(nil)
	tab1 := mgr.NewTab()
	tab2 := mgr.NewTab()

	found := mgr.TabByID(tab1.ID)
	if found != tab1 {
		t.Fatal("TabByID returned wrong tab")
	}

	found = mgr.TabByID(tab2.ID)
	if found != tab2 {
		t.Fatal("TabByID returned wrong tab")
	}

	found = mgr.TabByID(999)
	if found != nil {
		t.Fatal("TabByID found non-existent tab")
	}
}
