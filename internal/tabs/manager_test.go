package tabs

import "testing"

func TestTabByID(t *testing.T) {
	mgr := NewManager(nil)
	tab1 := mgr.NewTab()
	tab2 := mgr.NewTab()

	found := mgr.TabByID(tab1.ID())
	if found != tab1 {
		t.Fatal("TabByID returned wrong tab")
	}

	found = mgr.TabByID(tab2.ID())
	if found != tab2 {
		t.Fatal("TabByID returned wrong tab")
	}

	found = mgr.TabByID(999)
	if found != nil {
		t.Fatal("TabByID found non-existent tab")
	}
}

// TestTabScrollClampsToStripRange pins the shared scroll ownership: the
// offset the wheel, the draw path and the hit test all read is one mutex-held
// value clamped to the strip's legal range, so an over-scroll or a resize
// can never park the strip past its content.
func TestTabScrollClampsToStripRange(t *testing.T) {
	mgr := NewManager(nil)
	for i := 0; i < 40; i++ {
		mgr.NewTab()
	}
	n := int32(mgr.Count())
	mgr.SetTabScroll(99999, n, 1000)
	if got := mgr.TabScroll(); got != 2016 {
		t.Fatalf("TabScroll() = %d, want 2016 (clamped to max)", got)
	}
	mgr.SetTabScroll(-5, n, 1000)
	if got := mgr.TabScroll(); got != 0 {
		t.Fatalf("TabScroll() = %d, want 0 (clamped from below)", got)
	}
	mgr.SetTabScroll(100, 3, 1000)
	if got := mgr.TabScroll(); got != 0 {
		t.Fatalf("TabScroll() = %d, want 0 (fitting strip never scrolls)", got)
	}
}
