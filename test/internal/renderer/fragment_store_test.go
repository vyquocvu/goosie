package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"testing"
)

// --- FragmentID tests ---

func TestFragmentIDNone(t *testing.T) {
	if renderer.FragmentNone != 0 {
		t.Errorf("FragmentNone should be 0, got %d", renderer.FragmentNone)
	}
}

func TestFragmentIDValid(t *testing.T) {
	id := renderer.FragmentID(1)
	if !id.Valid() {
		t.Error("FragmentID(1) should be valid")
	}
	if renderer.FragmentNone.Valid() {
		t.Error("FragmentNone should not be valid")
	}
}

// --- FragmentStore construction ---

func TestNewFragmentStore(t *testing.T) {
	store := renderer.NewFragmentStore(0)
	if store == nil {
		t.Fatal("NewFragmentStore returned nil")
	}
	if store.FragmentCount() != 0 {
		t.Errorf("expected 0 fragments, got %d", store.FragmentCount())
	}
}

func TestNewFragmentStoreWithCapacity(t *testing.T) {
	store := renderer.NewFragmentStore(256)
	if store == nil {
		t.Fatal("NewFragmentStore returned nil")
	}
}

// --- Allocate ---

func TestAllocateFragment(t *testing.T) {
	store := renderer.NewFragmentStore(0)
	id, err := store.Allocate()
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if !id.Valid() {
		t.Error("allocated ID should be valid")
	}
	if store.FragmentCount() != 1 {
		t.Errorf("expected 1 fragment, got %d", store.FragmentCount())
	}
}

func TestAllocateMultipleFragments(t *testing.T) {
	store := renderer.NewFragmentStore(0)
	ids := make([]renderer.FragmentID, 10)
	for i := range ids {
		id, err := store.Allocate()
		if err != nil {
			t.Fatalf("Allocate %d failed: %v", i, err)
		}
		ids[i] = id
	}
	// All IDs should be unique
	seen := make(map[renderer.FragmentID]bool)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate FragmentID: %d", id)
		}
		seen[id] = true
	}
	if store.FragmentCount() != 10 {
		t.Errorf("expected 10 fragments, got %d", store.FragmentCount())
	}
}

// --- Get / Set ---

func TestGetSetFragment(t *testing.T) {
	store := renderer.NewFragmentStore(0)
	id, _ := store.Allocate()

	frag := store.Get(id)
	if frag == nil {
		t.Fatal("Get returned nil for allocated ID")
	}
	if frag.ID != id {
		t.Errorf("expected ID %d, got %d", id, frag.ID)
	}

	// Set properties
	store.SetType(id, renderer.FragmentLine)
	store.SetBox(id, renderer.Rect{X: 10, Y: 20, Width: 100, Height: 50})

	frag = store.Get(id)
	if frag.Type != renderer.FragmentLine {
		t.Errorf("expected type FragmentLine, got %d", frag.Type)
	}
	if frag.Box.X != 10 || frag.Box.Width != 100 {
		t.Errorf("box not set correctly: %+v", frag.Box)
	}
}

func TestGetInvalidFragmentID(t *testing.T) {
	store := renderer.NewFragmentStore(0)
	frag := store.Get(renderer.FragmentNone)
	if frag != nil {
		t.Error("Get(FragmentNone) should return nil")
	}
	frag = store.Get(renderer.FragmentID(999))
	if frag != nil {
		t.Error("Get(out of range) should return nil")
	}
}

// --- Fragment types ---

func TestFragmentTypeLine(t *testing.T) {
	store := renderer.NewFragmentStore(0)
	id, _ := store.Allocate()
	store.SetType(id, renderer.FragmentLine)

	frag := store.Get(id)
	if frag.Type != renderer.FragmentLine {
		t.Errorf("expected FragmentLine, got %d", frag.Type)
	}
}

func TestFragmentTypeTextRun(t *testing.T) {
	store := renderer.NewFragmentStore(0)
	id, _ := store.Allocate()
	store.SetType(id, renderer.FragmentTextRun)
	store.SetText(id, "Hello, World!")
	store.SetTextRange(id, 0, 13)

	frag := store.Get(id)
	if frag.Type != renderer.FragmentTextRun {
		t.Errorf("expected FragmentTextRun, got %d", frag.Type)
	}
	if frag.Text != "Hello, World!" {
		t.Errorf("expected text 'Hello, World!', got '%s'", frag.Text)
	}
	if frag.TextStart != 0 || frag.TextEnd != 13 {
		t.Errorf("expected text range [0,13), got [%d,%d)", frag.TextStart, frag.TextEnd)
	}
}

func TestFragmentTypeBox(t *testing.T) {
	store := renderer.NewFragmentStore(0)
	id, _ := store.Allocate()
	store.SetType(id, renderer.FragmentBox)
	store.SetLayoutID(id, renderer.LayoutID(42))

	frag := store.Get(id)
	if frag.Type != renderer.FragmentBox {
		t.Errorf("expected FragmentBox, got %d", frag.Type)
	}
	if frag.LayoutID != renderer.LayoutID(42) {
		t.Errorf("expected LayoutID 42, got %d", frag.LayoutID)
	}
}

func TestFragmentTypeReplacedElement(t *testing.T) {
	store := renderer.NewFragmentStore(0)
	id, _ := store.Allocate()
	store.SetType(id, renderer.FragmentReplaced)
	store.SetIntrinsicSize(id, 100, 50)

	frag := store.Get(id)
	if frag.Type != renderer.FragmentReplaced {
		t.Errorf("expected FragmentReplaced, got %d", frag.Type)
	}
	if frag.IntrinsicWidth != 100 || frag.IntrinsicHeight != 50 {
		t.Errorf("expected intrinsic size 100x50, got %.0fx%.0f", frag.IntrinsicWidth, frag.IntrinsicHeight)
	}
}

// --- Fragment chaining (one layout object -> multiple fragments) ---

func TestFragmentChain(t *testing.T) {
	store := renderer.NewFragmentStore(0)

	// Create a chain of 3 fragments for one layout object
	frag1, _ := store.Allocate()
	frag2, _ := store.Allocate()
	frag3, _ := store.Allocate()

	store.SetType(frag1, renderer.FragmentTextRun)
	store.SetType(frag2, renderer.FragmentTextRun)
	store.SetType(frag3, renderer.FragmentTextRun)

	// Link them
	store.SetNextFragment(frag1, frag2)
	store.SetNextFragment(frag2, frag3)

	// Verify chain
	if store.NextFragment(frag1) != frag2 {
		t.Errorf("expected frag2 after frag1")
	}
	if store.NextFragment(frag2) != frag3 {
		t.Errorf("expected frag3 after frag2")
	}
	if store.NextFragment(frag3) != renderer.FragmentNone {
		t.Errorf("expected FragmentNone after frag3")
	}
}

func TestLayoutObjectToFragments(t *testing.T) {
	store := renderer.NewFragmentStore(0)

	layoutID := renderer.LayoutID(10)

	// Create fragments for this layout object
	frag1, _ := store.Allocate()
	frag2, _ := store.Allocate()

	store.SetLayoutID(frag1, layoutID)
	store.SetLayoutID(frag2, layoutID)
	store.SetNextFragment(frag1, frag2)

	// First fragment should be accessible from layout ID
	store.SetFirstFragment(layoutID, frag1)

	firstFrag := store.FirstFragment(layoutID)
	if firstFrag != frag1 {
		t.Errorf("expected first fragment %d, got %d", frag1, firstFrag)
	}
}

// --- Scratch buffer pool ---

func TestScratchBufferPool(t *testing.T) {
	pool := renderer.NewScratchBufferPool(4)

	// Get a buffer
	buf := pool.Get()
	if buf == nil {
		t.Fatal("Get returned nil")
	}

	// Use it
	buf.Floats = append(buf.Floats, 1.0, 2.0, 3.0)
	buf.Ints = append(buf.Ints, 10, 20)

	// Return it
	pool.Put(buf)

	// Get another - should reuse
	buf2 := pool.Get()
	if buf2 == nil {
		t.Fatal("Get returned nil after Put")
	}
}

func TestScratchBufferPoolBounds(t *testing.T) {
	pool := renderer.NewScratchBufferPool(2)

	// Get more than capacity
	buf1 := pool.Get()
	buf2 := pool.Get()
	buf3 := pool.Get()

	// Return all
	pool.Put(buf1)
	pool.Put(buf2)
	pool.Put(buf3)

	// Should not panic
}

// --- Reset ---

func TestFragmentStoreReset(t *testing.T) {
	store := renderer.NewFragmentStore(0)
	for i := 0; i < 10; i++ {
		store.Allocate()
	}

	store.Reset()

	if store.FragmentCount() != 0 {
		t.Errorf("expected 0 fragments after reset, got %d", store.FragmentCount())
	}
}

// --- Edge cases ---

func TestSetTypeOnNone(t *testing.T) {
	store := renderer.NewFragmentStore(0)
	// Should not panic
	store.SetType(renderer.FragmentNone, renderer.FragmentLine)
}

func TestSetBoxOnNone(t *testing.T) {
	store := renderer.NewFragmentStore(0)
	// Should not panic
	store.SetBox(renderer.FragmentNone, renderer.Rect{})
}

func TestNextFragmentNone(t *testing.T) {
	store := renderer.NewFragmentStore(0)
	if store.NextFragment(renderer.FragmentNone) != renderer.FragmentNone {
		t.Error("NextFragment(FragmentNone) should return FragmentNone")
	}
}

func TestFirstFragmentNone(t *testing.T) {
	store := renderer.NewFragmentStore(0)
	if store.FirstFragment(renderer.LayoutNone) != renderer.FragmentNone {
		t.Error("FirstFragment(LayoutNone) should return FragmentNone")
	}
}

// --- helpers ---

func newTestFragmentStore() *renderer.FragmentStore {
	return renderer.NewFragmentStore(64)
}

// --- Benchmarks ---

func BenchmarkFragmentAllocate(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		store := renderer.NewFragmentStore(1024)
		for j := 0; j < 100; j++ {
			store.Allocate()
		}
	}
}

func BenchmarkFragmentSetGet(b *testing.B) {
	store := renderer.NewFragmentStore(1024)
	ids := make([]renderer.FragmentID, 100)
	for i := range ids {
		ids[i], _ = store.Allocate()
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, id := range ids {
			store.SetType(id, renderer.FragmentTextRun)
			store.SetText(id, "benchmark text")
			store.SetBox(id, renderer.Rect{X: 0, Y: 0, Width: 100, Height: 20})
			store.Get(id)
		}
	}
}

func BenchmarkFragmentChain(b *testing.B) {
	store := renderer.NewFragmentStore(1024)
	frags := make([]renderer.FragmentID, 100)
	for i := range frags {
		frags[i], _ = store.Allocate()
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Build chain
		for j := 0; j < len(frags)-1; j++ {
			store.SetNextFragment(frags[j], frags[j+1])
		}
		// Traverse chain
		for frag := frags[0]; frag.Valid(); frag = store.NextFragment(frag) {
			_ = store.Get(frag)
		}
	}
}

func BenchmarkScratchBufferPool(b *testing.B) {
	pool := renderer.NewScratchBufferPool(16)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := pool.Get()
		buf.Floats = append(buf.Floats, 1.0, 2.0, 3.0, 4.0, 5.0)
		pool.Put(buf)
	}
}
