package renderer_test

import (
	"context"
	"image"
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer"
)

// TestDirtyRegionFromPatches verifies that the dirty region is correctly
// computed from a list of DOM patches.
func TestDirtyRegionFromPatches(t *testing.T) {
	// Create a mock node with a box.
	node := &renderer.RenderNode{
		ID:      1,
		TagName: "p",
		Box: &renderer.Box{
			X:      10,
			Y:      20,
			Width:  100,
			Height: 50,
		},
	}

	// Create a patch for the node.
	patches := []renderer.DOMPatch{
		{
			Kind:       renderer.PatchUpdateText,
			NodeID:     node.ID,
			NewText:    "Updated",
			DirtyFlags: renderer.DirtyPaint,
		},
	}

	// Build node index.
	nodeIndex := map[int64]*renderer.RenderNode{
		node.ID: node,
	}

	// Compute dirty region.
	dirty := renderer.DirtyRegionFromPatches(patches, nodeIndex)

	// Verify the dirty region is not empty.
	if dirty.IsEmpty() {
		t.Error("dirty region should not be empty")
	}

	// Verify the dirty region matches the node's bounds.
	if dirty.X != 10 || dirty.Y != 20 || dirty.W != 100 || dirty.H != 50 {
		t.Errorf("dirty region (%v) should match node bounds (10, 20, 100, 50)", dirty)
	}
}

// TestDirtyRegionFromPatches_Empty verifies that an empty patch list produces
// an empty dirty region.
func TestDirtyRegionFromPatches_Empty(t *testing.T) {
	nodeIndex := make(map[int64]*renderer.RenderNode)
	patches := []renderer.DOMPatch{}

	dirty := renderer.DirtyRegionFromPatches(patches, nodeIndex)
	if !dirty.IsEmpty() {
		t.Error("empty patch list should produce empty dirty region")
	}
}

// TestDirtyRegionFromPatches_MultipleNodes verifies that the dirty region
// unions the bounds of multiple patched nodes.
func TestDirtyRegionFromPatches_MultipleNodes(t *testing.T) {
	// Create two mock nodes with boxes.
	node1 := &renderer.RenderNode{
		ID:      1,
		TagName: "p",
		Box: &renderer.Box{
			X:      10,
			Y:      10,
			Width:  50,
			Height: 50,
		},
	}
	node2 := &renderer.RenderNode{
		ID:      2,
		TagName: "p",
		Box: &renderer.Box{
			X:      200,
			Y:      200,
			Width:  50,
			Height: 50,
		},
	}

	// Create patches for both nodes.
	patches := []renderer.DOMPatch{
		{
			Kind:       renderer.PatchUpdateText,
			NodeID:     node1.ID,
			NewText:    "Uno",
			DirtyFlags: renderer.DirtyPaint,
		},
		{
			Kind:       renderer.PatchUpdateText,
			NodeID:     node2.ID,
			NewText:    "Dos",
			DirtyFlags: renderer.DirtyPaint,
		},
	}

	// Build node index.
	nodeIndex := map[int64]*renderer.RenderNode{
		node1.ID: node1,
		node2.ID: node2,
	}

	// Compute dirty region.
	dirty := renderer.DirtyRegionFromPatches(patches, nodeIndex)

	// Verify the dirty region spans both nodes.
	if dirty.IsEmpty() {
		t.Error("dirty region should not be empty")
	}

	// The dirty region should encompass both nodes' bounds.
	if dirty.W < 200 || dirty.H < 200 {
		t.Errorf("dirty region (%v) should span both nodes", dirty)
	}
}

// TestApplyPatchesWithPartialRepaint_NilRenderer verifies that nil renderer
// is handled gracefully.
func TestApplyPatchesWithPartialRepaint_NilRenderer(t *testing.T) {
	patches := []renderer.DOMPatch{
		{
			Kind:   renderer.PatchUpdateText,
			NodeID: 1,
		},
	}

	frame := image.NewRGBA(image.Rect(0, 0, 100, 100))
	result, err := renderer.ApplyPatchesWithPartialRepaint(nil, patches, frame)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != frame {
		t.Error("expected same frame to be returned for nil renderer")
	}
}

// TestApplyPatchesWithPartialRepaint_EmptyPatches verifies that empty patches
// are handled correctly.
func TestApplyPatchesWithPartialRepaint_EmptyPatches(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	html := `<html><body><p>Test</p></body></html>`
	_, err := r.RenderHTML(context.Background(), html)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	frame := image.NewRGBA(image.Rect(0, 0, 800, 600))
	result, err := renderer.ApplyPatchesWithPartialRepaint(r, nil, frame)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != frame {
		t.Error("expected same frame to be returned for empty patches")
	}
}

// BenchmarkDirtyRegionFromPatches measures the performance of computing dirty
// regions from patches.
func BenchmarkDirtyRegionFromPatches(b *testing.B) {
	node := &renderer.RenderNode{
		ID:      1,
		TagName: "p",
		Box: &renderer.Box{
			X:      10,
			Y:      20,
			Width:  100,
			Height: 50,
		},
	}

	patches := []renderer.DOMPatch{
		{
			Kind:       renderer.PatchUpdateText,
			NodeID:     node.ID,
			NewText:    "Updated",
			DirtyFlags: renderer.DirtyPaint,
		},
	}

	nodeIndex := map[int64]*renderer.RenderNode{
		node.ID: node,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderer.DirtyRegionFromPatches(patches, nodeIndex)
	}
}
