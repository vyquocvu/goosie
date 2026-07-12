package renderer

import (
	"fmt"
	"image/color"
	"math"
	"testing"
)

// --- DirtyRegion basic tests ---

func TestDirtyRegionNew(t *testing.T) {
	dr := NewDirtyRegion(64)
	if dr.Len() != 0 {
		t.Errorf("new region Len() = %d, want 0", dr.Len())
	}
	if dr.TotalArea() != 0 {
		t.Errorf("new region TotalArea() = %g, want 0", dr.TotalArea())
	}
}

func TestDirtyRegionAdd(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{X: 0, Y: 0, W: 10, H: 10})
	if dr.Len() != 1 {
		t.Fatalf("after Add, Len() = %d, want 1", dr.Len())
	}
	r := dr.Rects()[0]
	if r.X != 0 || r.Y != 0 || r.W != 10 || r.H != 10 {
		t.Errorf("Rects()[0] = %+v, want {0,0,10,10}", r)
	}
}

func TestDirtyRegionAddZeroRect(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{}) // zero-size rect should be ignored
	if dr.Len() != 0 {
		t.Errorf("adding zero rect: Len() = %d, want 0", dr.Len())
	}
}

func TestDirtyRegionAddNegativeSize(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{X: 10, Y: 10, W: -5, H: -5})
	if dr.Len() != 0 {
		t.Errorf("adding negative-size rect: Len() = %d, want 0", dr.Len())
	}
}

func TestDirtyRegionClear(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{X: 0, Y: 0, W: 10, H: 10})
	dr.Add(RectF{X: 20, Y: 20, W: 30, H: 30})
	dr.Clear()
	if dr.Len() != 0 {
		t.Errorf("after Clear, Len() = %d, want 0", dr.Len())
	}
	if dr.TotalArea() != 0 {
		t.Errorf("after Clear, TotalArea() = %g, want 0", dr.TotalArea())
	}
}

func TestDirtyRegionTotalArea(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{X: 0, Y: 0, W: 10, H: 10})     // area = 100
	dr.Add(RectF{X: 100, Y: 100, W: 20, H: 20}) // area = 400, no overlap
	if got := dr.TotalArea(); got != 500 {
		t.Errorf("TotalArea() = %g, want 500", got)
	}
}

func TestDirtyRegionTotalAreaOverlapping(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{X: 0, Y: 0, W: 10, H: 10}) // area = 100
	dr.Add(RectF{X: 5, Y: 5, W: 10, H: 10}) // overlaps, union area = 175 (10*10 + 10*10 - 5*5)
	if got := dr.TotalArea(); got != 175 {
		t.Errorf("TotalArea() with overlap = %g, want 175", got)
	}
}

func TestDirtyRegionExpand(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{X: 10, Y: 10, W: 20, H: 20})
	dr.Expand(2)
	r := dr.Rects()[0]
	// Should expand by 2 on each side
	if r.X != 8 || r.Y != 8 || r.W != 24 || r.H != 24 {
		t.Errorf("after Expand(2): %+v, want {8,8,24,24}", r)
	}
}

func TestDirtyRegionExpandClipsToViewport(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{X: 0, Y: 0, W: 100, H: 100})
	viewport := RectF{X: 0, Y: 0, W: 50, H: 50}
	dr.ExpandClipped(5, viewport)
	r := dr.Rects()[0]
	// Expanded would be {-5,-5,110,110} but clipped to viewport {0,0,50,50}
	if r.X != 0 || r.Y != 0 || r.W != 50 || r.H != 50 {
		t.Errorf("after ExpandClipped: %+v, want {0,0,50,50}", r)
	}
}

func TestDirtyRegionMerge(t *testing.T) {
	dr1 := NewDirtyRegion(64)
	dr1.Add(RectF{X: 0, Y: 0, W: 10, H: 10})

	dr2 := NewDirtyRegion(64)
	dr2.Add(RectF{X: 100, Y: 100, W: 20, H: 20})

	dr1.Merge(dr2)
	if dr1.Len() != 2 {
		t.Fatalf("after Merge, Len() = %d, want 2", dr1.Len())
	}
}

func TestDirtyRegionMergeNil(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{X: 0, Y: 0, W: 10, H: 10})
	dr.Merge(nil) // should not panic
	if dr.Len() != 1 {
		t.Errorf("after Merge(nil), Len() = %d, want 1", dr.Len())
	}
}

func TestDirtyRegionBoundingRect(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{X: 10, Y: 20, W: 30, H: 40})
	dr.Add(RectF{X: 100, Y: 200, W: 50, H: 60})

	br := dr.BoundingRect()
	// Union of {10,20,30,40} and {100,200,50,60} = {10,20,140,240}
	if br.X != 10 || br.Y != 20 || br.W != 140 || br.H != 240 {
		t.Errorf("BoundingRect() = %+v, want {10,20,140,240}", br)
	}
}

func TestDirtyRegionBoundingRectEmpty(t *testing.T) {
	dr := NewDirtyRegion(64)
	br := dr.BoundingRect()
	if br.W != 0 || br.H != 0 {
		t.Errorf("empty BoundingRect() = %+v, want zero", br)
	}
}

// --- DirtyRegion bounded merge tests ---

func TestDirtyRegionBoundedMerge(t *testing.T) {
	// When maxRects is small, adding many rects should trigger merging
	dr := NewDirtyRegion(4)
	for i := 0; i < 20; i++ {
		dr.Add(RectF{X: float32(i), Y: 0, W: 10, H: 10})
	}
	// Should have merged to stay within or near the limit
	if dr.Len() > 8 { // some slack for the merge algorithm
		t.Errorf("bounded merge: Len() = %d, expected <= 8", dr.Len())
	}
	// All area should still be covered
	if dr.TotalArea() == 0 {
		t.Error("bounded merge: TotalArea() = 0, expected > 0")
	}
}

func TestDirtyRegionMergeOverlappingRects(t *testing.T) {
	dr := NewDirtyRegion(64)
	// Add overlapping rects that should be merged
	dr.Add(RectF{X: 0, Y: 0, W: 50, H: 50})
	dr.Add(RectF{X: 25, Y: 25, W: 50, H: 50})
	// These overlap, so merge should combine them
	dr.MergeOverlapping()
	if dr.Len() != 1 {
		t.Fatalf("after MergeOverlapping, Len() = %d, want 1", dr.Len())
	}
	r := dr.Rects()[0]
	if r.X != 0 || r.Y != 0 || r.W != 75 || r.H != 75 {
		t.Errorf("merged rect = %+v, want {0,0,75,75}", r)
	}
}

func TestDirtyRegionMergeNonOverlapping(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{X: 0, Y: 0, W: 10, H: 10})
	dr.Add(RectF{X: 100, Y: 100, W: 10, H: 10})
	dr.MergeOverlapping()
	if dr.Len() != 2 {
		t.Errorf("non-overlapping rects should not merge: Len() = %d, want 2", dr.Len())
	}
}

// --- DirtyRegionTracker tests ---

func TestDirtyRegionTrackerNew(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	if tr == nil {
		t.Fatal("NewDirtyRegionTracker returned nil")
	}
	region := tr.Finalize()
	if region.Len() != 0 {
		t.Errorf("empty tracker Finalize().Len() = %d, want 0", region.Len())
	}
}

func TestDirtyRegionTrackerInvalidateRect(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	tr.InvalidateRect(RectF{X: 10, Y: 20, W: 100, H: 50})
	region := tr.Finalize()
	if region.Len() != 1 {
		t.Fatalf("Finalize().Len() = %d, want 1", region.Len())
	}
	r := region.Rects()[0]
	if r.X != 10 || r.Y != 20 || r.W != 100 || r.H != 50 {
		t.Errorf("Finalize().Rects()[0] = %+v, want {10,20,100,50}", r)
	}
}

func TestDirtyRegionTrackerInvalidateLayoutID(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	bounds := RectF{X: 0, Y: 0, W: 50, H: 50}
	tr.UpdateBounds(LayoutID(1), bounds)
	tr.InvalidateLayoutID(LayoutID(1))

	region := tr.Finalize()
	if region.Len() != 1 {
		t.Fatalf("Finalize().Len() = %d, want 1", region.Len())
	}
	r := region.Rects()[0]
	if r.X != 0 || r.Y != 0 || r.W != 50 || r.H != 50 {
		t.Errorf("dirty rect = %+v, want {0,0,50,50}", r)
	}
}

func TestDirtyRegionTrackerInvalidateMove(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	oldBounds := RectF{X: 0, Y: 0, W: 50, H: 50}
	newBounds := RectF{X: 100, Y: 100, W: 50, H: 50}

	tr.InvalidateMove(LayoutID(1), oldBounds, newBounds)

	region := tr.Finalize()
	// Should have both old and new bounds as dirty
	if region.Len() != 2 {
		t.Fatalf("after move, Finalize().Len() = %d, want 2", region.Len())
	}
	// Check that both regions are present
	found := make(map[bool]bool)
	for _, r := range region.Rects() {
		if r.X == 0 && r.Y == 0 {
			found[true] = true // old
		}
		if r.X == 100 && r.Y == 100 {
			found[false] = true // new
		}
	}
	if !found[true] || !found[false] {
		t.Error("move should dirty both old and new bounds")
	}
}

func TestDirtyRegionTrackerInvalidateMoveUpdatesStoredBounds(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	oldBounds := RectF{X: 0, Y: 0, W: 50, H: 50}
	newBounds := RectF{X: 100, Y: 100, W: 50, H: 50}

	tr.InvalidateMove(LayoutID(1), oldBounds, newBounds)

	// After move, stored bounds should be newBounds
	bounds, ok := tr.Bounds(LayoutID(1))
	if !ok {
		t.Fatal("Bounds(1) should exist after InvalidateMove")
	}
	if bounds != newBounds {
		t.Errorf("stored bounds = %+v, want %+v", bounds, newBounds)
	}
}

func TestDirtyRegionTrackerUpdateBounds(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	tr.UpdateBounds(LayoutID(1), RectF{X: 10, Y: 20, W: 30, H: 40})

	bounds, ok := tr.Bounds(LayoutID(1))
	if !ok {
		t.Fatal("Bounds should exist after UpdateBounds")
	}
	if bounds.X != 10 || bounds.Y != 20 || bounds.W != 30 || bounds.H != 40 {
		t.Errorf("Bounds() = %+v, want {10,20,30,40}", bounds)
	}
}

func TestDirtyRegionTrackerBoundsMissing(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	_, ok := tr.Bounds(LayoutID(99))
	if ok {
		t.Error("Bounds for unknown ID should return false")
	}
}

func TestDirtyRegionTrackerBoundsLayoutNone(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	tr.UpdateBounds(LayoutNone, RectF{X: 0, Y: 0, W: 10, H: 10})
	_, ok := tr.Bounds(LayoutNone)
	if ok {
		t.Error("Bounds for LayoutNone should return false")
	}
}

func TestDirtyRegionTrackerRemoveLayout(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	tr.UpdateBounds(LayoutID(1), RectF{X: 0, Y: 0, W: 50, H: 50})
	tr.RemoveLayout(LayoutID(1))
	_, ok := tr.Bounds(LayoutID(1))
	if ok {
		t.Error("Bounds should not exist after RemoveLayout")
	}
}

func TestDirtyRegionTrackerRemoveLayoutInvalidates(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	tr.UpdateBounds(LayoutID(1), RectF{X: 0, Y: 0, W: 50, H: 50})
	tr.RemoveLayout(LayoutID(1))

	region := tr.Finalize()
	if region.Len() != 1 {
		t.Fatalf("removing layout should dirty old bounds: Len() = %d, want 1", region.Len())
	}
}

func TestDirtyRegionTrackerFinalizeResetsDirty(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	tr.InvalidateRect(RectF{X: 0, Y: 0, W: 10, H: 10})
	tr.Finalize()

	// Second finalize should be empty (no new invalidations)
	region := tr.Finalize()
	if region.Len() != 0 {
		t.Errorf("second Finalize().Len() = %d, want 0", region.Len())
	}
}

func TestDirtyRegionTrackerReset(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	tr.UpdateBounds(LayoutID(1), RectF{X: 0, Y: 0, W: 50, H: 50})
	tr.InvalidateLayoutID(LayoutID(1))
	tr.Reset()

	region := tr.Finalize()
	if region.Len() != 0 {
		t.Errorf("after Reset, Finalize().Len() = %d, want 0", region.Len())
	}
	_, ok := tr.Bounds(LayoutID(1))
	if ok {
		t.Error("after Reset, Bounds should not exist")
	}
}

func TestDirtyRegionTrackerInvalidateLayoutNone(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	tr.InvalidateLayoutID(LayoutNone) // should be a no-op
	region := tr.Finalize()
	if region.Len() != 0 {
		t.Errorf("invalidate LayoutNone: Len() = %d, want 0", region.Len())
	}
}

func TestDirtyRegionTrackerMultipleInvalidations(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	tr.UpdateBounds(LayoutID(1), RectF{X: 0, Y: 0, W: 10, H: 10})
	tr.UpdateBounds(LayoutID(2), RectF{X: 100, Y: 100, W: 20, H: 20})

	tr.InvalidateLayoutID(LayoutID(1))
	tr.InvalidateLayoutID(LayoutID(2))
	tr.InvalidateRect(RectF{X: 200, Y: 200, W: 30, H: 30})

	region := tr.Finalize()
	if region.Len() != 3 {
		t.Errorf("multiple invalidations: Len() = %d, want 3", region.Len())
	}
}

func TestDirtyRegionTrackerInvalidateWithoutPriorBounds(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	// Invalidating a layout ID with no stored bounds should be a no-op
	tr.InvalidateLayoutID(LayoutID(42))
	region := tr.Finalize()
	if region.Len() != 0 {
		t.Errorf("invalidate without bounds: Len() = %d, want 0", region.Len())
	}
}

// --- Expansion for visual effects tests ---

func TestExpandForEffects_Shadow(t *testing.T) {
	r := RectF{X: 10, Y: 10, W: 100, H: 50}
	expanded := ExpandForEffects(r, EffectParams{ShadowBlur: 5})
	// Should expand by shadow blur on all sides
	if expanded.X != 5 || expanded.Y != 5 || expanded.W != 110 || expanded.H != 60 {
		t.Errorf("shadow expand = %+v, want {5,5,110,60}", expanded)
	}
}

func TestExpandForEffects_Border(t *testing.T) {
	r := RectF{X: 10, Y: 10, W: 100, H: 50}
	expanded := ExpandForEffects(r, EffectParams{BorderWidth: 3})
	if expanded.X != 7 || expanded.Y != 7 || expanded.W != 106 || expanded.H != 56 {
		t.Errorf("border expand = %+v, want {7,7,106,56}", expanded)
	}
}

func TestExpandForEffects_Antialiasing(t *testing.T) {
	r := RectF{X: 10, Y: 10, W: 100, H: 50}
	expanded := ExpandForEffects(r, EffectParams{AAMargin: 1})
	if expanded.X != 9 || expanded.Y != 9 || expanded.W != 102 || expanded.H != 52 {
		t.Errorf("AA expand = %+v, want {9,9,102,52}", expanded)
	}
}

func TestExpandForEffects_Combined(t *testing.T) {
	r := RectF{X: 10, Y: 10, W: 100, H: 50}
	expanded := ExpandForEffects(r, EffectParams{
		ShadowBlur:  5,
		BorderWidth: 3,
		AAMargin:    1,
	})
	// Total expansion per side = 5 + 3 + 1 = 9
	if expanded.X != 1 || expanded.Y != 1 || expanded.W != 118 || expanded.H != 68 {
		t.Errorf("combined expand = %+v, want {1,1,118,68}", expanded)
	}
}

func TestExpandForEffects_ZeroParams(t *testing.T) {
	r := RectF{X: 10, Y: 10, W: 100, H: 50}
	expanded := ExpandForEffects(r, EffectParams{})
	if expanded != r {
		t.Errorf("zero params: got %+v, want %+v", expanded, r)
	}
}

func TestExpandForEffects_ShadowOffset(t *testing.T) {
	r := RectF{X: 10, Y: 10, W: 100, H: 50}
	expanded := ExpandForEffects(r, EffectParams{
		ShadowBlur:    4,
		ShadowOffsetX: 3,
		ShadowOffsetY: 5,
	})
	// Left expansion = blur + max(0, -offsetX) = 4 + 0 = 4
	// Right expansion = blur + max(0, offsetX) = 4 + 3 = 7
	// Top expansion = blur + max(0, -offsetY) = 4 + 0 = 4
	// Bottom expansion = blur + max(0, offsetY) = 4 + 5 = 9
	if expanded.X != 6 || expanded.Y != 6 || expanded.W != 111 || expanded.H != 63 {
		t.Errorf("shadow offset expand = %+v, want {6,6,111,63}", expanded)
	}
}

// --- Debug visualization tests ---

func TestDebugDirtyRegionOverlay(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{X: 10, Y: 20, W: 100, H: 50})
	dr.Add(RectF{X: 200, Y: 300, W: 80, H: 40})

	dl := DebugDirtyRegionOverlay(dr, color.RGBA{R: 255, A: 64})
	// Should produce 2 rect commands (one per dirty rect)
	if dl.Len() != 2 {
		t.Fatalf("overlay Len() = %d, want 2", dl.Len())
	}
	cmd := dl.At(0)
	if cmd.Kind != CmdRect {
		t.Errorf("cmd[0].Kind = %v, want CmdRect", cmd.Kind)
	}
	if cmd.Rect.Bounds.X != 10 || cmd.Rect.Bounds.Y != 20 {
		t.Errorf("cmd[0].Bounds = %+v, want {10,20,100,50}", cmd.Rect.Bounds)
	}
}

func TestDebugDirtyRegionOverlayEmpty(t *testing.T) {
	dr := NewDirtyRegion(64)
	dl := DebugDirtyRegionOverlay(dr, color.RGBA{R: 255, A: 64})
	if dl.Len() != 0 {
		t.Errorf("empty overlay Len() = %d, want 0", dl.Len())
	}
}

func TestDebugDirtyRegionOverlayNil(t *testing.T) {
	dl := DebugDirtyRegionOverlay(nil, color.RGBA{R: 255, A: 64})
	if dl.Len() != 0 {
		t.Errorf("nil overlay Len() = %d, want 0", dl.Len())
	}
}

func TestDebugDirtyRegionOverlayDefaultColor(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{X: 0, Y: 0, W: 10, H: 10})
	dl := DebugDirtyRegionOverlay(dr, color.RGBA{})
	cmd := dl.At(0)
	// Should use a default color (semi-transparent red)
	r, g, b, a := cmd.Rect.Color.RGBA()
	if r == 0 && g == 0 && b == 0 && a == 0 {
		t.Error("default color should not be fully transparent")
	}
}

// --- Edge case tests ---

func TestDirtyRegionExpandEmpty(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Expand(5) // should not panic on empty region
	if dr.Len() != 0 {
		t.Errorf("expand empty: Len() = %d, want 0", dr.Len())
	}
}

func TestDirtyRegionLargeExpansion(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{X: 50, Y: 50, W: 10, H: 10})
	dr.Expand(1000)
	r := dr.Rects()[0]
	if r.X != -950 || r.Y != -950 || r.W != 2010 || r.H != 2010 {
		t.Errorf("large expansion: %+v, want {-950,-950,2010,2010}", r)
	}
}

func TestDirtyRegionTrackerInvalidateMoveSamePosition(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	bounds := RectF{X: 50, Y: 50, W: 30, H: 30}
	tr.InvalidateMove(LayoutID(1), bounds, bounds)

	region := tr.Finalize()
	// Even if old == new, both should be added (they overlap, so merge should handle it)
	if region.Len() == 0 {
		t.Error("move with same position should still produce dirty region")
	}
}

func TestDirtyRegionTrackerInvalidateMoveLayoutNone(t *testing.T) {
	tr := NewDirtyRegionTracker(64)
	tr.InvalidateMove(LayoutNone,
		RectF{X: 0, Y: 0, W: 10, H: 10},
		RectF{X: 20, Y: 20, W: 10, H: 10})

	region := tr.Finalize()
	if region.Len() != 0 {
		t.Errorf("move with LayoutNone: Len() = %d, want 0", region.Len())
	}
}

func TestDirtyRegionRectsReturnsCopy(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{X: 0, Y: 0, W: 10, H: 10})
	rects := dr.Rects()
	rects[0] = RectF{X: 999, Y: 999, W: 1, H: 1}
	// Original should be unchanged
	orig := dr.Rects()[0]
	if orig.X != 0 || orig.Y != 0 {
		t.Error("Rects() should return a copy")
	}
}

func TestDirtyRegionContains(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{X: 0, Y: 0, W: 100, H: 100})
	dr.Add(RectF{X: 200, Y: 200, W: 50, H: 50})

	tests := []struct {
		x, y float32
		want bool
	}{
		{50, 50, true},    // inside first rect
		{225, 225, true},  // inside second rect
		{150, 150, false}, // between rects
		{-1, 0, false},    // outside
	}
	for _, tt := range tests {
		got := dr.Contains(tt.x, tt.y)
		if got != tt.want {
			t.Errorf("Contains(%g, %g) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}

func TestDirtyRegionIntersects(t *testing.T) {
	dr := NewDirtyRegion(64)
	dr.Add(RectF{X: 0, Y: 0, W: 50, H: 50})

	if !dr.Intersects(RectF{X: 25, Y: 25, W: 100, H: 100}) {
		t.Error("should intersect overlapping rect")
	}
	if dr.Intersects(RectF{X: 100, Y: 100, W: 50, H: 50}) {
		t.Error("should not intersect disjoint rect")
	}
}

// --- Benchmarks ---

func BenchmarkDirtyRegionAdd(b *testing.B) {
	dr := NewDirtyRegion(64)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dr.Add(RectF{X: float32(i % 100), Y: float32(i % 50), W: 10, H: 10})
		if dr.Len() > 60 {
			dr.Clear()
		}
	}
}

func BenchmarkDirtyRegionMergeOverlapping(b *testing.B) {
	sizes := []int{10, 100, 500}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			rects := make([]RectF, n)
			for i := range rects {
				rects[i] = RectF{X: float32(i % 20), Y: float32(i % 10), W: 50, H: 50}
			}
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				dr := NewDirtyRegion(64)
				for _, r := range rects {
					dr.Add(r)
				}
				dr.MergeOverlapping()
			}
		})
	}
}

func BenchmarkDirtyRegionTotalArea(b *testing.B) {
	dr := NewDirtyRegion(64)
	for i := 0; i < 100; i++ {
		dr.Add(RectF{X: float32(i), Y: 0, W: 10, H: 10})
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = dr.TotalArea()
	}
}

func BenchmarkDirtyRegionTrackerInvalidateMove(b *testing.B) {
	tr := NewDirtyRegionTracker(64)
	for i := 0; i < 100; i++ {
		tr.UpdateBounds(LayoutID(i+1), RectF{X: float32(i), Y: 0, W: 50, H: 50})
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		id := LayoutID(i%100 + 1)
		newBounds := RectF{X: float32(i%100) + 10, Y: 10, W: 50, H: 50}
		tr.InvalidateMove(id, tr.mustBounds(id), newBounds)
		tr.Finalize()
	}
}

func BenchmarkExpandForEffects(b *testing.B) {
	r := RectF{X: 10, Y: 10, W: 100, H: 50}
	params := EffectParams{ShadowBlur: 5, BorderWidth: 2, AAMargin: 1}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ExpandForEffects(r, params)
	}
}

func BenchmarkDebugDirtyRegionOverlay(b *testing.B) {
	dr := NewDirtyRegion(64)
	for i := 0; i < 100; i++ {
		dr.Add(RectF{X: float32(i * 10), Y: 0, W: 8, H: 100})
	}
	clr := color.RGBA{R: 255, A: 64}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = DebugDirtyRegionOverlay(dr, clr)
	}
}

// mustBounds is a test helper that panics if bounds are not found.
func (tr *DirtyRegionTracker) mustBounds(id LayoutID) RectF {
	b, ok := tr.Bounds(id)
	if !ok {
		panic(fmt.Sprintf("no bounds for LayoutID %d", id))
	}
	return b
}

// --- RectF additional tests ---

func TestRectFUnion(t *testing.T) {
	a := RectF{X: 0, Y: 0, W: 10, H: 10}
	b := RectF{X: 20, Y: 30, W: 15, H: 5}
	u := RectUnion(a, b)
	if u.X != 0 || u.Y != 0 || u.W != 35 || u.H != 35 {
		t.Errorf("RectUnion = %+v, want {0,0,35,35}", u)
	}
}

func TestRectFIntersection(t *testing.T) {
	a := RectF{X: 0, Y: 0, W: 50, H: 50}
	b := RectF{X: 25, Y: 25, W: 50, H: 50}
	inter := RectIntersection(a, b)
	if inter.X != 25 || inter.Y != 25 || inter.W != 25 || inter.H != 25 {
		t.Errorf("RectIntersection = %+v, want {25,25,25,25}", inter)
	}
}

func TestRectFIntersectionDisjoint(t *testing.T) {
	a := RectF{X: 0, Y: 0, W: 10, H: 10}
	b := RectF{X: 100, Y: 100, W: 10, H: 10}
	inter := RectIntersection(a, b)
	if inter.W != 0 || inter.H != 0 {
		t.Errorf("disjoint intersection = %+v, want zero", inter)
	}
}

func TestRectFArea(t *testing.T) {
	r := RectF{X: 0, Y: 0, W: 10, H: 20}
	if got := r.Area(); got != 200 {
		t.Errorf("Area() = %g, want 200", got)
	}
}

func TestRectFAreaZero(t *testing.T) {
	r := RectF{}
	if got := r.Area(); got != 0 {
		t.Errorf("Area() = %g, want 0", got)
	}
}

func TestRectFIsEmpty(t *testing.T) {
	tests := []struct {
		r     RectF
		empty bool
	}{
		{RectF{X: 0, Y: 0, W: 10, H: 10}, false},
		{RectF{X: 0, Y: 0, W: 0, H: 0}, true},
		{RectF{X: 0, Y: 0, W: -1, H: 10}, true},
		{RectF{X: 0, Y: 0, W: 10, H: -1}, true},
	}
	for _, tt := range tests {
		if got := tt.r.IsEmpty(); got != tt.empty {
			t.Errorf("RectF%+v).IsEmpty() = %v, want %v", tt.r, got, tt.empty)
		}
	}
}

func TestRectFEquals(t *testing.T) {
	a := RectF{X: 1, Y: 2, W: 3, H: 4}
	b := RectF{X: 1, Y: 2, W: 3, H: 4}
	c := RectF{X: 1, Y: 2, W: 3, H: 5}
	if !a.Equal(b) {
		t.Error("equal rects should be Equal")
	}
	if a.Equal(c) {
		t.Error("different rects should not be Equal")
	}
}

func TestRectFNearlyEqual(t *testing.T) {
	a := RectF{X: 1.0, Y: 2.0, W: 3.0, H: 4.0}
	b := RectF{X: 1.001, Y: 2.001, W: 3.001, H: 4.001}
	if !a.NearlyEqual(b, 0.01) {
		t.Error("nearly equal rects should be NearlyEqual")
	}
	if a.NearlyEqual(b, 0.0001) {
		t.Error("should not be NearlyEqual with tight tolerance")
	}
}

// suppress unused import warnings
var _ = math.Abs
