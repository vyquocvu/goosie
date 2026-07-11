package renderer

import (
	"testing"
)

// --- ReflowReason tests ---

func TestReflowReasonConstants(t *testing.T) {
	// Verify reflow reason constants are defined
	if ReflowNone != 0 {
		t.Errorf("ReflowNone should be 0, got %d", ReflowNone)
	}
	if ReflowGeometry == 0 {
		t.Error("ReflowGeometry should be non-zero")
	}
	if ReflowIntrinsic == 0 {
		t.Error("ReflowIntrinsic should be non-zero")
	}
	if ReflowText == 0 {
		t.Error("ReflowText should be non-zero")
	}
	if ReflowChildren == 0 {
		t.Error("ReflowChildren should be non-zero")
	}
	if ReflowViewport == 0 {
		t.Error("ReflowViewport should be non-zero")
	}
	if ReflowFont == 0 {
		t.Error("ReflowFont should be non-zero")
	}
	if ReflowStyle == 0 {
		t.Error("ReflowStyle should be non-zero")
	}
}

func TestReflowReasonCombine(t *testing.T) {
	reasons := ReflowGeometry | ReflowText
	if reasons&ReflowGeometry == 0 {
		t.Error("combined reasons should include ReflowGeometry")
	}
	if reasons&ReflowText == 0 {
		t.Error("combined reasons should include ReflowText")
	}
	if reasons&ReflowStyle != 0 {
		t.Error("combined reasons should not include ReflowStyle")
	}
}

func TestReflowReasonHas(t *testing.T) {
	reasons := ReflowReasons(ReflowGeometry | ReflowText)
	if !reasons.Has(ReflowGeometry) {
		t.Error("should have ReflowGeometry")
	}
	if !reasons.Has(ReflowText) {
		t.Error("should have ReflowText")
	}
	if reasons.Has(ReflowStyle) {
		t.Error("should not have ReflowStyle")
	}
}

func TestReflowReasonClear(t *testing.T) {
	reasons := ReflowReasons(ReflowGeometry | ReflowText | ReflowStyle)
	reasons = reasons.Clear(ReflowText)
	if reasons.Has(ReflowText) {
		t.Error("ReflowText should be cleared")
	}
	if !reasons.Has(ReflowGeometry) {
		t.Error("ReflowGeometry should still be set")
	}
	if !reasons.Has(ReflowStyle) {
		t.Error("ReflowStyle should still be set")
	}
}

// --- ReflowTracker tests ---

func TestNewReflowTracker(t *testing.T) {
	engine := NewReflowTracker()
	if engine == nil {
		t.Fatal("NewReflowTracker returned nil")
	}
}

func TestReflowTrackerMarkDirty(t *testing.T) {
	engine := NewReflowTracker()
	layoutID := LayoutID(1)

	engine.MarkDirty(layoutID, ReflowGeometry)

	if !engine.IsDirty(layoutID) {
		t.Error("layout should be marked dirty")
	}
}

func TestMarkDirtyMultipleReasons(t *testing.T) {
	engine := NewReflowTracker()
	layoutID := LayoutID(1)

	engine.MarkDirty(layoutID, ReflowGeometry)
	engine.MarkDirty(layoutID, ReflowText)

	reasons := engine.DirtyReasons(layoutID)
	if !reasons.Has(ReflowGeometry) {
		t.Error("should have ReflowGeometry")
	}
	if !reasons.Has(ReflowText) {
		t.Error("should have ReflowText")
	}
}

func TestIsDirty(t *testing.T) {
	engine := NewReflowTracker()
	layoutID := LayoutID(1)

	if engine.IsDirty(layoutID) {
		t.Error("clean layout should not be dirty")
	}

	engine.MarkDirty(layoutID, ReflowGeometry)

	if !engine.IsDirty(layoutID) {
		t.Error("marked layout should be dirty")
	}
}

func TestReflowTrackerClearDirty(t *testing.T) {
	engine := NewReflowTracker()
	layoutID := LayoutID(1)

	engine.MarkDirty(layoutID, ReflowGeometry)
	engine.ClearDirty(layoutID)

	if engine.IsDirty(layoutID) {
		t.Error("layout should be clean after ClearDirty")
	}
}

func TestDirtyReasons(t *testing.T) {
	engine := NewReflowTracker()
	layoutID := LayoutID(1)

	engine.MarkDirty(layoutID, ReflowGeometry|ReflowText)

	reasons := engine.DirtyReasons(layoutID)
	if !reasons.Has(ReflowGeometry) {
		t.Error("should have ReflowGeometry")
	}
	if !reasons.Has(ReflowText) {
		t.Error("should have ReflowText")
	}
}

func TestFindReflowRoot(t *testing.T) {
	engine := NewReflowTracker()

	// Create a simple tree: root -> child -> grandchild
	root := LayoutID(1)
	child := LayoutID(2)
	grandchild := LayoutID(3)

	engine.SetParent(child, root)
	engine.SetParent(grandchild, child)

	// Mark grandchild dirty
	engine.MarkDirty(grandchild, ReflowText)

	// Find reflow root - should be the smallest valid root
	reflowRoot := engine.FindReflowRoot(grandchild)
	if reflowRoot != grandchild {
		t.Errorf("expected reflow root to be grandchild %d, got %d", grandchild, reflowRoot)
	}
}

func TestFindReflowRootWithDirtyParent(t *testing.T) {
	engine := NewReflowTracker()

	root := LayoutID(1)
	child := LayoutID(2)

	engine.SetParent(child, root)

	// Mark both parent and child dirty
	engine.MarkDirty(root, ReflowChildren)
	engine.MarkDirty(child, ReflowText)

	// Find reflow root for child - should be root since parent is dirty with ReflowChildren
	reflowRoot := engine.FindReflowRoot(child)
	if reflowRoot != root {
		t.Errorf("expected reflow root to be root %d, got %d", root, reflowRoot)
	}
}

func TestCollectDirtyRects(t *testing.T) {
	engine := NewReflowTracker()

	engine.MarkDirty(LayoutID(1), ReflowGeometry)
	engine.MarkDirty(LayoutID(2), ReflowText)
	engine.MarkDirty(LayoutID(3), ReflowStyle)

	dirtyRects := engine.CollectDirtyRects()
	if len(dirtyRects) != 3 {
		t.Errorf("expected 3 dirty rects, got %d", len(dirtyRects))
	}
}

func TestClearAllDirty(t *testing.T) {
	engine := NewReflowTracker()

	engine.MarkDirty(LayoutID(1), ReflowGeometry)
	engine.MarkDirty(LayoutID(2), ReflowText)

	engine.ClearAllDirty()

	if engine.IsDirty(LayoutID(1)) || engine.IsDirty(LayoutID(2)) {
		t.Error("all dirty flags should be cleared")
	}
}

// --- Intrinsic size cache tests ---

func TestIntrinsicSizeCache(t *testing.T) {
	engine := NewReflowTracker()
	layoutID := LayoutID(1)

	// Set intrinsic size
	engine.SetIntrinsicSize(layoutID, 100, 50)

	// Get intrinsic size
	width, height := engine.IntrinsicSize(layoutID)
	if width != 100 || height != 50 {
		t.Errorf("expected intrinsic size 100x50, got %.0fx%.0f", width, height)
	}
}

func TestIntrinsicSizeCacheMiss(t *testing.T) {
	engine := NewReflowTracker()
	layoutID := LayoutID(999)

	width, height := engine.IntrinsicSize(layoutID)
	if width != 0 || height != 0 {
		t.Errorf("expected 0x0 for cache miss, got %.0fx%.0f", width, height)
	}
}

func TestIntrinsicSizeCacheInvalidate(t *testing.T) {
	engine := NewReflowTracker()
	layoutID := LayoutID(1)

	engine.SetIntrinsicSize(layoutID, 100, 50)
	engine.InvalidateIntrinsicSize(layoutID)

	width, height := engine.IntrinsicSize(layoutID)
	if width != 0 || height != 0 {
		t.Errorf("expected 0x0 after invalidation, got %.0fx%.0f", width, height)
	}
}

// --- Benchmarks ---

func BenchmarkIncrementalMarkDirty(b *testing.B) {
	engine := NewReflowTracker()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		engine.MarkDirty(LayoutID(uint32(i%1000)), ReflowGeometry)
	}
}

func BenchmarkIncrementalIsDirty(b *testing.B) {
	engine := NewReflowTracker()
	engine.MarkDirty(LayoutID(1), ReflowGeometry)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		engine.IsDirty(LayoutID(1))
	}
}

func BenchmarkIncrementalFindReflowRoot(b *testing.B) {
	engine := NewReflowTracker()

	// Create a chain of 100 nodes
	for i := 1; i < 100; i++ {
		engine.SetParent(LayoutID(uint32(i+1)), LayoutID(uint32(i)))
	}
	engine.MarkDirty(LayoutID(100), ReflowText)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		engine.FindReflowRoot(LayoutID(100))
	}
}

func BenchmarkIntrinsicSizeSet(b *testing.B) {
	engine := NewReflowTracker()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		engine.SetIntrinsicSize(LayoutID(uint32(i%1000)), 100, 50)
	}
}

func BenchmarkIntrinsicSizeGet(b *testing.B) {
	engine := NewReflowTracker()
	engine.SetIntrinsicSize(LayoutID(1), 100, 50)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		engine.IntrinsicSize(LayoutID(1))
	}
}
