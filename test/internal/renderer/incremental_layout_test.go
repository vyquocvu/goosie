package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"testing"
)

// --- ReflowReason tests ---

func TestReflowReasonConstants(t *testing.T) {
	// Verify reflow reason constants are defined
	if renderer.ReflowNone != 0 {
		t.Errorf("ReflowNone should be 0, got %d", renderer.ReflowNone)
	}
	if renderer.ReflowGeometry == 0 {
		t.Error("ReflowGeometry should be non-zero")
	}
	if renderer.ReflowIntrinsic == 0 {
		t.Error("ReflowIntrinsic should be non-zero")
	}
	if renderer.ReflowText == 0 {
		t.Error("ReflowText should be non-zero")
	}
	if renderer.ReflowChildren == 0 {
		t.Error("ReflowChildren should be non-zero")
	}
	if renderer.ReflowViewport == 0 {
		t.Error("ReflowViewport should be non-zero")
	}
	if renderer.ReflowFont == 0 {
		t.Error("ReflowFont should be non-zero")
	}
	if renderer.ReflowStyle == 0 {
		t.Error("ReflowStyle should be non-zero")
	}
}

func TestReflowReasonCombine(t *testing.T) {
	reasons := renderer.ReflowGeometry | renderer.ReflowText
	if reasons&renderer.ReflowGeometry == 0 {
		t.Error("combined reasons should include ReflowGeometry")
	}
	if reasons&renderer.ReflowText == 0 {
		t.Error("combined reasons should include ReflowText")
	}
	if reasons&renderer.ReflowStyle != 0 {
		t.Error("combined reasons should not include ReflowStyle")
	}
}

func TestReflowReasonHas(t *testing.T) {
	reasons := renderer.ReflowReasons(renderer.ReflowGeometry | renderer.ReflowText)
	if !reasons.Has(renderer.ReflowGeometry) {
		t.Error("should have ReflowGeometry")
	}
	if !reasons.Has(renderer.ReflowText) {
		t.Error("should have ReflowText")
	}
	if reasons.Has(renderer.ReflowStyle) {
		t.Error("should not have ReflowStyle")
	}
}

func TestReflowReasonClear(t *testing.T) {
	reasons := renderer.ReflowReasons(renderer.ReflowGeometry | renderer.ReflowText | renderer.ReflowStyle)
	reasons = reasons.Clear(renderer.ReflowText)
	if reasons.Has(renderer.ReflowText) {
		t.Error("ReflowText should be cleared")
	}
	if !reasons.Has(renderer.ReflowGeometry) {
		t.Error("ReflowGeometry should still be set")
	}
	if !reasons.Has(renderer.ReflowStyle) {
		t.Error("ReflowStyle should still be set")
	}
}

// --- ReflowTracker tests ---

func TestNewReflowTracker(t *testing.T) {
	engine := renderer.NewReflowTracker()
	if engine == nil {
		t.Fatal("NewReflowTracker returned nil")
	}
}

func TestReflowTrackerMarkDirty(t *testing.T) {
	engine := renderer.NewReflowTracker()
	layoutID := renderer.LayoutID(1)

	engine.MarkDirty(layoutID, renderer.ReflowGeometry)

	if !engine.IsDirty(layoutID) {
		t.Error("layout should be marked dirty")
	}
}

func TestMarkDirtyMultipleReasons(t *testing.T) {
	engine := renderer.NewReflowTracker()
	layoutID := renderer.LayoutID(1)

	engine.MarkDirty(layoutID, renderer.ReflowGeometry)
	engine.MarkDirty(layoutID, renderer.ReflowText)

	reasons := engine.DirtyReasons(layoutID)
	if !reasons.Has(renderer.ReflowGeometry) {
		t.Error("should have ReflowGeometry")
	}
	if !reasons.Has(renderer.ReflowText) {
		t.Error("should have ReflowText")
	}
}

func TestIsDirty(t *testing.T) {
	engine := renderer.NewReflowTracker()
	layoutID := renderer.LayoutID(1)

	if engine.IsDirty(layoutID) {
		t.Error("clean layout should not be dirty")
	}

	engine.MarkDirty(layoutID, renderer.ReflowGeometry)

	if !engine.IsDirty(layoutID) {
		t.Error("marked layout should be dirty")
	}
}

func TestReflowTrackerClearDirty(t *testing.T) {
	engine := renderer.NewReflowTracker()
	layoutID := renderer.LayoutID(1)

	engine.MarkDirty(layoutID, renderer.ReflowGeometry)
	engine.ClearDirty(layoutID)

	if engine.IsDirty(layoutID) {
		t.Error("layout should be clean after ClearDirty")
	}
}

func TestDirtyReasons(t *testing.T) {
	engine := renderer.NewReflowTracker()
	layoutID := renderer.LayoutID(1)

	engine.MarkDirty(layoutID, renderer.ReflowGeometry|renderer.ReflowText)

	reasons := engine.DirtyReasons(layoutID)
	if !reasons.Has(renderer.ReflowGeometry) {
		t.Error("should have ReflowGeometry")
	}
	if !reasons.Has(renderer.ReflowText) {
		t.Error("should have ReflowText")
	}
}

func TestFindReflowRoot(t *testing.T) {
	engine := renderer.NewReflowTracker()

	// Create a simple tree: root -> child -> grandchild
	root := renderer.LayoutID(1)
	child := renderer.LayoutID(2)
	grandchild := renderer.LayoutID(3)

	engine.SetParent(child, root)
	engine.SetParent(grandchild, child)

	// Mark grandchild dirty
	engine.MarkDirty(grandchild, renderer.ReflowText)

	// Find reflow root - should be the smallest valid root
	reflowRoot := engine.FindReflowRoot(grandchild)
	if reflowRoot != grandchild {
		t.Errorf("expected reflow root to be grandchild %d, got %d", grandchild, reflowRoot)
	}
}

func TestFindReflowRootWithDirtyParent(t *testing.T) {
	engine := renderer.NewReflowTracker()

	root := renderer.LayoutID(1)
	child := renderer.LayoutID(2)

	engine.SetParent(child, root)

	// Mark both parent and child dirty
	engine.MarkDirty(root, renderer.ReflowChildren)
	engine.MarkDirty(child, renderer.ReflowText)

	// Find reflow root for child - should be root since parent is dirty with ReflowChildren
	reflowRoot := engine.FindReflowRoot(child)
	if reflowRoot != root {
		t.Errorf("expected reflow root to be root %d, got %d", root, reflowRoot)
	}
}

func TestCollectDirtyRects(t *testing.T) {
	engine := renderer.NewReflowTracker()

	engine.MarkDirty(renderer.LayoutID(1), renderer.ReflowGeometry)
	engine.MarkDirty(renderer.LayoutID(2), renderer.ReflowText)
	engine.MarkDirty(renderer.LayoutID(3), renderer.ReflowStyle)

	dirtyRects := engine.CollectDirtyRects()
	if len(dirtyRects) != 3 {
		t.Errorf("expected 3 dirty rects, got %d", len(dirtyRects))
	}
}

func TestClearAllDirty(t *testing.T) {
	engine := renderer.NewReflowTracker()

	engine.MarkDirty(renderer.LayoutID(1), renderer.ReflowGeometry)
	engine.MarkDirty(renderer.LayoutID(2), renderer.ReflowText)

	engine.ClearAllDirty()

	if engine.IsDirty(renderer.LayoutID(1)) || engine.IsDirty(renderer.LayoutID(2)) {
		t.Error("all dirty flags should be cleared")
	}
}

// --- Intrinsic size cache tests ---

func TestIntrinsicSizeCache(t *testing.T) {
	engine := renderer.NewReflowTracker()
	layoutID := renderer.LayoutID(1)

	// Set intrinsic size
	engine.SetIntrinsicSize(layoutID, 100, 50)

	// Get intrinsic size
	width, height := engine.IntrinsicSize(layoutID)
	if width != 100 || height != 50 {
		t.Errorf("expected intrinsic size 100x50, got %.0fx%.0f", width, height)
	}
}

func TestIntrinsicSizeCacheMiss(t *testing.T) {
	engine := renderer.NewReflowTracker()
	layoutID := renderer.LayoutID(999)

	width, height := engine.IntrinsicSize(layoutID)
	if width != 0 || height != 0 {
		t.Errorf("expected 0x0 for cache miss, got %.0fx%.0f", width, height)
	}
}

func TestIntrinsicSizeCacheInvalidate(t *testing.T) {
	engine := renderer.NewReflowTracker()
	layoutID := renderer.LayoutID(1)

	engine.SetIntrinsicSize(layoutID, 100, 50)
	engine.InvalidateIntrinsicSize(layoutID)

	width, height := engine.IntrinsicSize(layoutID)
	if width != 0 || height != 0 {
		t.Errorf("expected 0x0 after invalidation, got %.0fx%.0f", width, height)
	}
}

// --- Benchmarks ---

func BenchmarkIncrementalMarkDirty(b *testing.B) {
	engine := renderer.NewReflowTracker()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		engine.MarkDirty(renderer.LayoutID(uint32(i%1000)), renderer.ReflowGeometry)
	}
}

func BenchmarkIncrementalIsDirty(b *testing.B) {
	engine := renderer.NewReflowTracker()
	engine.MarkDirty(renderer.LayoutID(1), renderer.ReflowGeometry)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		engine.IsDirty(renderer.LayoutID(1))
	}
}

func BenchmarkIncrementalFindReflowRoot(b *testing.B) {
	engine := renderer.NewReflowTracker()

	// Create a chain of 100 nodes
	for i := 1; i < 100; i++ {
		engine.SetParent(renderer.LayoutID(uint32(i+1)), renderer.LayoutID(uint32(i)))
	}
	engine.MarkDirty(renderer.LayoutID(100), renderer.ReflowText)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		engine.FindReflowRoot(renderer.LayoutID(100))
	}
}

func BenchmarkIntrinsicSizeSet(b *testing.B) {
	engine := renderer.NewReflowTracker()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		engine.SetIntrinsicSize(renderer.LayoutID(uint32(i%1000)), 100, 50)
	}
}

func BenchmarkIntrinsicSizeGet(b *testing.B) {
	engine := renderer.NewReflowTracker()
	engine.SetIntrinsicSize(renderer.LayoutID(1), 100, 50)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		engine.IntrinsicSize(renderer.LayoutID(1))
	}
}
