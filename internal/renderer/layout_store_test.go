package renderer

import (
	"testing"
)

// --- LayoutID tests ---

func TestLayoutIDNone(t *testing.T) {
	if LayoutNone != 0 {
		t.Errorf("LayoutNone should be 0, got %d", LayoutNone)
	}
}

func TestLayoutIDValid(t *testing.T) {
	id := LayoutID(1)
	if !id.Valid() {
		t.Error("LayoutID(1) should be valid")
	}
	if LayoutNone.Valid() {
		t.Error("LayoutNone should not be valid")
	}
}

// --- LayoutStore construction ---

func TestNewLayoutStore(t *testing.T) {
	store := NewLayoutStore(0)
	if store == nil {
		t.Fatal("NewLayoutStore returned nil")
	}
	if store.ObjectCount() != 0 {
		t.Errorf("expected 0 objects, got %d", store.ObjectCount())
	}
}

func TestNewLayoutStoreWithCapacity(t *testing.T) {
	store := NewLayoutStore(256)
	if store == nil {
		t.Fatal("NewLayoutStore returned nil")
	}
}

// --- Allocate ---

func TestAllocate(t *testing.T) {
	store := NewLayoutStore(0)
	id, err := store.Allocate()
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if !id.Valid() {
		t.Error("allocated ID should be valid")
	}
	if store.ObjectCount() != 1 {
		t.Errorf("expected 1 object, got %d", store.ObjectCount())
	}
}

func TestAllocateMultiple(t *testing.T) {
	store := NewLayoutStore(0)
	ids := make([]LayoutID, 10)
	for i := range ids {
		id, err := store.Allocate()
		if err != nil {
			t.Fatalf("Allocate %d failed: %v", i, err)
		}
		ids[i] = id
	}
	// All IDs should be unique
	seen := make(map[LayoutID]bool)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate LayoutID: %d", id)
		}
		seen[id] = true
	}
	if store.ObjectCount() != 10 {
		t.Errorf("expected 10 objects, got %d", store.ObjectCount())
	}
}

// --- Get / Set ---

func TestGetSetLayoutObject(t *testing.T) {
	store := NewLayoutStore(0)
	id, _ := store.Allocate()

	obj := store.Get(id)
	if obj == nil {
		t.Fatal("Get returned nil for allocated ID")
	}
	if obj.ID != id {
		t.Errorf("expected ID %d, got %d", id, obj.ID)
	}

	// Set properties
	store.SetDisplay(id, "block")
	store.SetBox(id, Rect{X: 10, Y: 20, Width: 100, Height: 50})

	obj = store.Get(id)
	if obj.Display != "block" {
		t.Errorf("expected display=block, got %s", obj.Display)
	}
	if obj.Box.X != 10 || obj.Box.Width != 100 {
		t.Errorf("box not set correctly: %+v", obj.Box)
	}
}

func TestGetInvalidID(t *testing.T) {
	store := NewLayoutStore(0)
	obj := store.Get(LayoutNone)
	if obj != nil {
		t.Error("Get(LayoutNone) should return nil")
	}
	obj = store.Get(LayoutID(999))
	if obj != nil {
		t.Error("Get(out of range) should return nil")
	}
}

// --- display:none handling ---

func TestDisplayNoneNoAllocation(t *testing.T) {
	store := NewLayoutStore(0)

	// Allocate a parent
	parentID, _ := store.Allocate()
	store.SetDisplay(parentID, "block")

	// A child with display:none should not get a layout object
	// The store should track this via the DOM-to-layout mapping returning LayoutNone
	childDOMID := int64(42)
	store.SetDOMToLayout(childDOMID, LayoutNone)

	mapped := store.DOMToLayout(childDOMID)
	if mapped.Valid() {
		t.Error("display:none element should map to LayoutNone")
	}
}

func TestDisplayNoneSkippedInTree(t *testing.T) {
	store := NewLayoutStore(0)

	root, _ := store.Allocate()
	store.SetDisplay(root, "block")

	child1, _ := store.Allocate()
	store.SetDisplay(child1, "block")
	store.SetDOMToLayout(1, child1)

	// child2 has display:none — no allocation
	store.SetDOMToLayout(2, LayoutNone)

	child3, _ := store.Allocate()
	store.SetDisplay(child3, "block")
	store.SetDOMToLayout(3, child3)

	store.AppendChild(root, child1)
	store.AppendChild(root, child3)

	// Only 3 layout objects (root, child1, child3)
	if store.ObjectCount() != 3 {
		t.Errorf("expected 3 objects (display:none skipped), got %d", store.ObjectCount())
	}
}

// --- Generated content ---

func TestGeneratedContentLayout(t *testing.T) {
	store := NewLayoutStore(0)

	// Generated content creates a layout object without a DOM node
	genID, _ := store.Allocate()
	store.SetDisplay(genID, "block")
	store.SetGenerated(genID, true)

	obj := store.Get(genID)
	if !obj.IsGenerated {
		t.Error("generated content flag should be set")
	}

	// Generated content has no DOM mapping
	if store.LayoutToDOM(genID) != 0 {
		t.Error("generated content should have no DOM mapping")
	}
}

// --- DOM-to-Layout mapping ---

func TestDOMToLayoutMapping(t *testing.T) {
	store := NewLayoutStore(0)
	id, _ := store.Allocate()

	domID := int64(100)
	store.SetDOMToLayout(domID, id)

	mapped := store.DOMToLayout(domID)
	if mapped != id {
		t.Errorf("expected layout ID %d, got %d", id, mapped)
	}
}

func TestLayoutToDOMMapping(t *testing.T) {
	store := NewLayoutStore(0)
	id, _ := store.Allocate()

	domID := int64(200)
	store.SetDOMToLayout(domID, id)

	mapped := store.LayoutToDOM(id)
	if mapped != domID {
		t.Errorf("expected DOM ID %d, got %d", domID, mapped)
	}
}

func TestDOMToLayoutUnmapped(t *testing.T) {
	store := NewLayoutStore(0)
	mapped := store.DOMToLayout(999)
	if mapped != LayoutNone {
		t.Errorf("unmapped DOM should return LayoutNone, got %d", mapped)
	}
}

// --- Tree operations ---

func TestAppendChild(t *testing.T) {
	store := NewLayoutStore(0)
	parent, _ := store.Allocate()
	child, _ := store.Allocate()

	store.AppendChild(parent, child)

	if store.FirstChild(parent) != child {
		t.Error("child not appended correctly")
	}

	childObj := store.Get(child)
	if childObj.Parent != parent {
		t.Error("parent not set on child")
	}
}

func TestChildCount(t *testing.T) {
	store := NewLayoutStore(0)
	parent, _ := store.Allocate()

	for i := 0; i < 5; i++ {
		child, _ := store.Allocate()
		store.AppendChild(parent, child)
	}

	if store.ChildCount(parent) != 5 {
		t.Errorf("expected 5 children, got %d", store.ChildCount(parent))
	}
}

func TestFirstChild(t *testing.T) {
	store := NewLayoutStore(0)
	parent, _ := store.Allocate()
	child1, _ := store.Allocate()
	child2, _ := store.Allocate()

	store.AppendChild(parent, child1)
	store.AppendChild(parent, child2)

	if store.FirstChild(parent) != child1 {
		t.Errorf("expected first child %d, got %d", child1, store.FirstChild(parent))
	}
}

func TestNextSibling(t *testing.T) {
	store := NewLayoutStore(0)
	parent, _ := store.Allocate()
	child1, _ := store.Allocate()
	child2, _ := store.Allocate()
	child3, _ := store.Allocate()

	store.AppendChild(parent, child1)
	store.AppendChild(parent, child2)
	store.AppendChild(parent, child3)

	if store.NextSibling(child1) != child2 {
		t.Errorf("expected next sibling %d, got %d", child2, store.NextSibling(child1))
	}
	if store.NextSibling(child2) != child3 {
		t.Errorf("expected next sibling %d, got %d", child3, store.NextSibling(child2))
	}
	if store.NextSibling(child3) != LayoutNone {
		t.Error("last child should have no next sibling")
	}
}

// --- Remove ---

// TestLayoutStoreRemoveChild tests removing a child from its
// parent in the layout tree store (a separate, renderer-internal
// store used for layout bookkeeping, not the DOM tree itself).
func TestLayoutStoreRemoveChild(t *testing.T) {
	store := NewLayoutStore(0)
	parent, _ := store.Allocate()
	child, _ := store.Allocate()

	store.AppendChild(parent, child)
	store.RemoveChild(parent, child)

	if store.FirstChild(parent) != LayoutNone {
		t.Error("child should be removed")
	}
	if store.ChildCount(parent) != 0 {
		t.Error("child count should be 0 after removal")
	}
}

// --- Reset ---

func TestReset(t *testing.T) {
	store := NewLayoutStore(0)
	for i := 0; i < 10; i++ {
		store.Allocate()
	}
	store.SetDOMToLayout(1, LayoutID(1))

	store.Reset()

	if store.ObjectCount() != 0 {
		t.Errorf("expected 0 objects after reset, got %d", store.ObjectCount())
	}
	if store.DOMToLayout(1) != LayoutNone {
		t.Error("mappings should be cleared after reset")
	}
}

// --- HasLayout ---

func TestHasLayout(t *testing.T) {
	store := NewLayoutStore(0)
	id, _ := store.Allocate()
	domID := int64(1)
	store.SetDOMToLayout(domID, id)

	if !store.HasLayout(domID) {
		t.Error("HasLayout should return true for mapped DOM node")
	}
	if store.HasLayout(999) {
		t.Error("HasLayout should return false for unmapped DOM node")
	}
}

// --- Edge cases ---

func TestAppendChildToNone(t *testing.T) {
	store := NewLayoutStore(0)
	child, _ := store.Allocate()
	err := store.AppendChild(LayoutNone, child)
	if err == nil {
		t.Error("appending to LayoutNone should fail")
	}
}

func TestSetDisplayOnNone(t *testing.T) {
	store := NewLayoutStore(0)
	// Should not panic
	store.SetDisplay(LayoutNone, "block")
}

func TestChildCountNone(t *testing.T) {
	store := NewLayoutStore(0)
	if store.ChildCount(LayoutNone) != 0 {
		t.Error("ChildCount of LayoutNone should be 0")
	}
}

// --- helpers ---

func newTestLayoutStore() *LayoutStore {
	return NewLayoutStore(64)
}

// --- Benchmarks ---

func BenchmarkLayoutStoreAllocate(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		store := NewLayoutStore(1024)
		for j := 0; j < 100; j++ {
			store.Allocate()
		}
	}
}

func BenchmarkLayoutStoreAppendChild(b *testing.B) {
	store := NewLayoutStore(1024)
	parent, _ := store.Allocate()
	children := make([]LayoutID, 100)
	for i := range children {
		children[i], _ = store.Allocate()
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, c := range children {
			store.AppendChild(parent, c)
		}
		// Reset for next iteration
		for _, c := range children {
			store.RemoveChild(parent, c)
		}
	}
}

func BenchmarkLayoutStoreDOMMapping(b *testing.B) {
	store := NewLayoutStore(1024)
	ids := make([]LayoutID, 100)
	for i := range ids {
		ids[i], _ = store.Allocate()
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for j, id := range ids {
			store.SetDOMToLayout(int64(j+1), id)
		}
		for j := range ids {
			store.DOMToLayout(int64(j + 1))
		}
	}
}

func BenchmarkLayoutStoreChildCount(b *testing.B) {
	store := NewLayoutStore(1024)
	parent, _ := store.Allocate()
	for j := 0; j < 100; j++ {
		child, _ := store.Allocate()
		store.AppendChild(parent, child)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		store.ChildCount(parent)
	}
}
