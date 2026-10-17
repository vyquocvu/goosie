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

func TestReopenTabStateAndStack(t *testing.T) {
	mgr := NewManager(nil)
	tab1 := mgr.NewTab()
	tab1.SetURL("https://example.com/1")
	tab1.SetTitle("Page 1")
	tab1.History().Push("https://example.com/1")
	tab1.History().Push("https://example.com/1b")
	tab1.SetScrollY(150)

	tab2 := mgr.NewTab()
	tab2.SetURL("https://example.com/2")
	tab2.SetTitle("Page 2")
	tab2.History().Push("https://example.com/2")
	tab2.SetScrollY(300)

	tab1ID := tab1.ID()
	tab2ID := tab2.ID()

	mgr.CloseTab(tab1ID)
	if mgr.ClosedTabCount() != 1 {
		t.Fatalf("ClosedTabCount() = %d, want 1", mgr.ClosedTabCount())
	}
	if !mgr.CanReopenTab() {
		t.Fatal("CanReopenTab() want true")
	}

	mgr.CloseTab(tab2ID)
	if mgr.ClosedTabCount() != 2 {
		t.Fatalf("ClosedTabCount() = %d, want 2", mgr.ClosedTabCount())
	}

	// First reopen restores tab2 (LIFO)
	restored2, ok := mgr.ReopenTab()
	if !ok || restored2 == nil {
		t.Fatalf("ReopenTab() failed for tab2: ok=%v", ok)
	}
	if restored2.URL() != "https://example.com/2" {
		t.Errorf("restored2 URL = %q, want %q", restored2.URL(), "https://example.com/2")
	}
	if restored2.Title() != "Page 2" {
		t.Errorf("restored2 Title = %q, want %q", restored2.Title(), "Page 2")
	}
	if restored2.ScrollY() != 300 {
		t.Errorf("restored2 ScrollY = %d, want 300", restored2.ScrollY())
	}
	if restored2.ID() == tab2ID {
		t.Errorf("restored2 ID should be newly allocated, got same %d", tab2ID)
	}

	// Second reopen restores tab1
	restored1, ok := mgr.ReopenTab()
	if !ok || restored1 == nil {
		t.Fatalf("ReopenTab() failed for tab1: ok=%v", ok)
	}
	if restored1.URL() != "https://example.com/1" {
		t.Errorf("restored1 URL = %q, want %q", restored1.URL(), "https://example.com/1")
	}
	if restored1.Title() != "Page 1" {
		t.Errorf("restored1 Title = %q, want %q", restored1.Title(), "Page 1")
	}
	if restored1.ScrollY() != 150 {
		t.Errorf("restored1 ScrollY = %d, want 150", restored1.ScrollY())
	}
	entries, idx := restored1.History().Entries()
	if len(entries) != 2 || entries[0] != "https://example.com/1" || entries[1] != "https://example.com/1b" || idx != 1 {
		t.Errorf("restored1 history mismatch: entries=%v, idx=%d", entries, idx)
	}

	// Third reopen has nothing left
	empty, ok := mgr.ReopenTab()
	if ok || empty != nil {
		t.Fatalf("ReopenTab() on empty stack = (%v, %v), want (nil, false)", empty, ok)
	}
	if mgr.CanReopenTab() {
		t.Fatal("CanReopenTab() should be false when stack is empty")
	}
}

func TestReopenTabIgnoresBlankTabs(t *testing.T) {
	mgr := NewManager(nil)
	tab := mgr.NewTab() // Blank tab with no URL and no history entries
	mgr.CloseTab(tab.ID())

	if mgr.CanReopenTab() {
		t.Fatal("CanReopenTab() should be false for blank tab")
	}
	if count := mgr.ClosedTabCount(); count != 0 {
		t.Fatalf("ClosedTabCount() = %d, want 0", count)
	}
}

func TestReopenTabStackCapacityBounded(t *testing.T) {
	mgr := NewManager(nil)
	for i := 0; i < 35; i++ {
		tab := mgr.NewTab()
		tab.SetURL("https://example.com/test")
		mgr.CloseTab(tab.ID())
	}
	if count := mgr.ClosedTabCount(); count != maxClosedTabs {
		t.Fatalf("ClosedTabCount() = %d, want %d", count, maxClosedTabs)
	}
}
