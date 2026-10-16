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

// TestMoveTabReordersAndKeepsActive pins drag-reorder ownership: moving a tab
// reorders by final index, clamps out-of-range targets, ignores unknown IDs,
// and keeps the ACTIVE tab (by identity, not position) across the move.
func TestMoveTabReordersAndKeepsActive(t *testing.T) {
	mgr := NewManager(nil)
	a := mgr.NewTab()
	b := mgr.NewTab()
	c := mgr.NewTab()
	mgr.SwitchTo(b.ID())
	if !mgr.MoveTab(a.ID(), 2) {
		t.Fatal("MoveTab(first, last) reported no move")
	}
	ids := tabIDs(mgr)
	if len(ids) != 3 || ids[0] != b.ID() || ids[1] != c.ID() || ids[2] != a.ID() {
		t.Fatalf("order = %v, want [b c a]", ids)
	}
	if got := mgr.Active(); got != b {
		t.Fatal("active tab changed across a reorder of another tab")
	}
	mgr.MoveTab(c.ID(), -99)
	if ids := tabIDs(mgr); ids[0] != c.ID() {
		t.Fatalf("order = %v, want c clamped to front", ids)
	}
	mgr.MoveTab(c.ID(), 99)
	if ids := tabIDs(mgr); ids[2] != c.ID() {
		t.Fatalf("order = %v, want c clamped to back", ids)
	}
	if mgr.MoveTab(9999, 0) {
		t.Fatal("MoveTab(unknown) reported a move")
	}
	if mgr.MoveTab(b.ID(), 0) {
		t.Fatal("MoveTab to the same index reported a move")
	}
	if got := mgr.Active(); got != b {
		t.Fatal("active tab changed across clamping moves")
	}
}

func tabIDs(mgr *TabManager) []uint64 {
	var out []uint64
	for _, tab := range mgr.Tabs() {
		out = append(out, tab.ID())
	}
	return out
}

// TestDragStateRoundtrip pins the shared drag handoff between hit-testing
// and drawing: one mutex-held value, cleared on end.
func TestDragStateRoundtrip(t *testing.T) {
	mgr := NewManager(nil)
	tab := mgr.NewTab()
	if _, _, ok := mgr.Dragging(); ok {
		t.Fatal("Dragging() set with no drag")
	}
	mgr.StartDrag(tab.ID(), 120)
	id, x, ok := mgr.Dragging()
	if !ok || id != tab.ID() || x != 120 {
		t.Fatalf("Dragging() = (%d, %d, %v), want (%d, 120, true)", id, x, ok, tab.ID())
	}
	mgr.MoveDrag(200)
	if _, x, _ := mgr.Dragging(); x != 200 {
		t.Fatalf("MoveDrag x = %d, want 200", x)
	}
	mgr.EndDrag()
	if _, _, ok := mgr.Dragging(); ok {
		t.Fatal("Dragging() still set after EndDrag")
	}
}
