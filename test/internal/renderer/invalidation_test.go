package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestNewInvalidationTracker(t *testing.T) {
	it := renderer.NewInvalidationTracker()

	if it == nil {
		t.Fatal("NewInvalidationTracker returned nil")
	}
	if it.DirtyNodes() == nil {
		t.Error("dirtyNodes map not initialized")
	}
}

func TestMarkDirty(t *testing.T) {
	it := renderer.NewInvalidationTracker()
	nodeID := int64(1)

	it.MarkDirty(nodeID, renderer.DirtyLayout)

	if !it.IsDirty(nodeID) {
		t.Error("Node should be marked as dirty")
	}

	flags := it.GetDirtyFlags(nodeID)
	if flags != renderer.DirtyLayout {
		t.Errorf("Expected DirtyLayout flag, got %v", flags)
	}
}

func TestMarkDirtyMultipleFlags(t *testing.T) {
	it := renderer.NewInvalidationTracker()
	nodeID := int64(1)

	it.MarkDirty(nodeID, renderer.DirtyLayout)
	it.MarkDirty(nodeID, renderer.DirtyPaint)

	flags := it.GetDirtyFlags(nodeID)
	if flags&renderer.DirtyLayout == 0 {
		t.Error("DirtyLayout flag should be set")
	}
	if flags&renderer.DirtyPaint == 0 {
		t.Error("DirtyPaint flag should be set")
	}
}

func TestClearDirty(t *testing.T) {
	it := renderer.NewInvalidationTracker()
	nodeID := int64(1)

	it.MarkDirty(nodeID, renderer.DirtyLayout)

	if !it.IsDirty(nodeID) {
		t.Error("Node should be dirty before clear")
	}

	it.ClearDirty(nodeID)

	if it.IsDirty(nodeID) {
		t.Error("Node should not be dirty after clear")
	}
}

func TestGetDirtyNodes(t *testing.T) {
	it := renderer.NewInvalidationTracker()

	it.MarkDirty(1, renderer.DirtyLayout)
	it.MarkDirty(2, renderer.DirtyPaint)
	it.MarkDirty(3, renderer.DirtyStyle)

	dirtyNodes := it.GetDirtyNodes()

	if len(dirtyNodes) != 3 {
		t.Errorf("Expected 3 dirty nodes, got %d", len(dirtyNodes))
	}

	// Check that all marked nodes are in the list
	nodeMap := make(map[int64]bool)
	for _, id := range dirtyNodes {
		nodeMap[id] = true
	}

	if !nodeMap[1] || !nodeMap[2] || !nodeMap[3] {
		t.Error("Not all dirty nodes returned")
	}
}

func TestClearAll(t *testing.T) {
	it := renderer.NewInvalidationTracker()

	it.MarkDirty(1, renderer.DirtyLayout)
	it.MarkDirty(2, renderer.DirtyPaint)

	it.ClearAll()

	if it.IsDirty(1) || it.IsDirty(2) {
		t.Error("Nodes should not be dirty after ClearAll")
	}

	dirtyNodes := it.GetDirtyNodes()
	if len(dirtyNodes) != 0 {
		t.Errorf("Expected 0 dirty nodes after ClearAll, got %d", len(dirtyNodes))
	}
}

func TestPropagateInvalidation(t *testing.T) {
	it := renderer.NewInvalidationTracker()

	// Create a tree: parent -> child
	parent := renderer.NewRenderNode(renderer.NodeTypeElement)
	parent.TagName = "div"

	child := renderer.NewRenderNode(renderer.NodeTypeElement)
	child.TagName = "p"
	parent.AddChild(child)

	// Invalidate child with layout flag
	it.PropagateInvalidation(child, renderer.DirtyLayout)

	// Both child and parent should be dirty
	if !it.IsDirty(child.ID) {
		t.Error("Child should be dirty")
	}
	if !it.IsDirty(parent.ID) {
		t.Error("Parent should be dirty (layout propagates up)")
	}
}

func TestPropagateInvalidationSubtree(t *testing.T) {
	it := renderer.NewInvalidationTracker()

	// Create a tree: parent -> child1, child2
	parent := renderer.NewRenderNode(renderer.NodeTypeElement)
	parent.TagName = "div"

	child1 := renderer.NewRenderNode(renderer.NodeTypeElement)
	child1.TagName = "p"
	parent.AddChild(child1)

	child2 := renderer.NewRenderNode(renderer.NodeTypeElement)
	child2.TagName = "p"
	parent.AddChild(child2)

	// Invalidate parent with subtree flag
	it.PropagateInvalidation(parent, renderer.DirtySubtree)

	// Parent and all children should be dirty
	if !it.IsDirty(parent.ID) {
		t.Error("Parent should be dirty")
	}
	if !it.IsDirty(child1.ID) {
		t.Error("Child1 should be dirty")
	}
	if !it.IsDirty(child2.ID) {
		t.Error("Child2 should be dirty")
	}
}

func TestIncrementalLayoutEngine(t *testing.T) {
	ile := renderer.NewIncrementalLayoutEngine(800, 600)

	if ile == nil {
		t.Fatal("NewIncrementalLayoutEngine returned nil")
	}
	if ile.LayoutEngine == nil {
		t.Error("LayoutEngine not initialized")
	}
	if ile.Invalidation() == nil {
		t.Error("Invalidation tracker not initialized")
	}
}

func TestInvalidateNode(t *testing.T) {
	ile := renderer.NewIncrementalLayoutEngine(800, 600)

	node := renderer.NewRenderNode(renderer.NodeTypeElement)
	node.TagName = "div"

	ile.InvalidateNode(node, renderer.DirtyLayout)

	if !ile.IsNodeDirty(node.ID) {
		t.Error("Node should be dirty after invalidation")
	}
}

func TestComputeIncrementalLayoutNoDirty(t *testing.T) {
	ile := renderer.NewIncrementalLayoutEngine(800, 600)

	// Create a render tree
	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "div"

	text := renderer.NewRenderNode(renderer.NodeTypeText)
	text.Text = "Test"
	root.AddChild(text)

	// First layout
	layout1 := ile.ComputeIncrementalLayout(root, nil)

	if layout1 == nil {
		t.Fatal("First layout returned nil")
	}

	// Second layout without any changes
	layout2 := ile.ComputeIncrementalLayout(root, layout1)

	// Should return the previous layout since nothing is dirty
	if layout2 == nil {
		t.Error("Second layout should not be nil")
	}
}

func TestComputeIncrementalLayoutWithDirty(t *testing.T) {
	ile := renderer.NewIncrementalLayoutEngine(800, 600)

	// Create a render tree
	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "div"

	text := renderer.NewRenderNode(renderer.NodeTypeText)
	text.Text = "Test"
	root.AddChild(text)

	// First layout
	layout1 := ile.ComputeIncrementalLayout(root, nil)

	// Mark node as dirty
	ile.InvalidateNode(text, renderer.DirtyLayout)

	// Second layout should recompute
	layout2 := ile.ComputeIncrementalLayout(root, layout1)

	if layout2 == nil {
		t.Error("Second layout should not be nil")
	}

	// After layout, node should not be dirty
	if ile.IsNodeDirty(text.ID) {
		t.Error("Node should not be dirty after layout")
	}
}

func TestDirtyFlagCombinations(t *testing.T) {
	it := renderer.NewInvalidationTracker()
	nodeID := int64(1)

	tests := []struct {
		name  string
		flags renderer.DirtyFlag
	}{
		{"layout", renderer.DirtyLayout},
		{"paint", renderer.DirtyPaint},
		{"style", renderer.DirtyStyle},
		{"subtree", renderer.DirtySubtree},
		{"layout+paint", renderer.DirtyLayout | renderer.DirtyPaint},
		{"all", renderer.DirtyLayout | renderer.DirtyPaint | renderer.DirtyStyle | renderer.DirtySubtree},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it.ClearDirty(nodeID)
			it.MarkDirty(nodeID, tt.flags)

			flags := it.GetDirtyFlags(nodeID)
			if flags != tt.flags {
				t.Errorf("Expected flags %v, got %v", tt.flags, flags)
			}
		})
	}
}

func TestRendererApplyMutationBatchMarksExistingNode(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><body><div id="target">value</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	var target *renderer.RenderNode
	walkRenderNodes(r.GetRoot(), func(node *renderer.RenderNode) {
		if node != nil && node.Attrs["id"] == "target" {
			target = node
		}
	})
	if target == nil {
		t.Fatal("target node not found")
	}
	if applied := r.ApplyMutationBatch([]renderer.MutationInvalidation{{NodeID: target.ID, Flags: renderer.DirtyStyle | renderer.DirtyPaint}}); applied != 1 {
		t.Fatalf("applied = %d, want 1", applied)
	}
	if !r.Incremental().IsNodeDirty(target.ID) {
		t.Fatal("target should be invalidated")
	}
}

func TestRendererInvalidatePaintChunksMarksDirty(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><body><div id="a">a</div><div id="b">b</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	var target *renderer.RenderNode
	walkRenderNodes(r.GetRoot(), func(node *renderer.RenderNode) {
		if node != nil && node.Attrs["id"] == "a" {
			target = node
		}
	})
	if target == nil {
		t.Fatal("target node not found")
	}
	box := r.GetLayoutBox(target)
	if box == nil {
		t.Fatalf("layout box missing for node %d", target.ID)
	}
	r.ChunkedDisplay().Chunks().Add(renderer.PaintChunk{Owner: renderer.LayoutID(uint32(box.NodeID)), Start: 0, End: 1, Bounds: renderer.RectF{X: 0, Y: 0, W: 10, H: 10}})
	r.ChunkedDisplay().MarkAllClean()
	if invalidated := r.InvalidatePaintChunks([]int64{target.ID}); invalidated == 0 {
		t.Fatal("expected at least one chunk invalidated")
	}
	if r.ChunkedDisplay().DirtyChunkCount() == 0 {
		t.Fatal("dirty chunk count should be > 0")
	}
}

func TestRendererPresentFromMutationBatch(t *testing.T) {
	r := renderer.NewRenderer(200, 200)
	_, err := r.RenderHTML(context.Background(), `<html><body><div id="target">v</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	r.ChunkedDisplay().Chunks().Add(renderer.PaintChunk{Owner: renderer.LayoutID(1), Start: 0, End: 1, Bounds: renderer.RectF{X: 0, Y: 0, W: 50, H: 50}})
	r.ChunkedDisplay().Chunks().Add(renderer.PaintChunk{Owner: renderer.LayoutID(2), Start: 1, End: 2, Bounds: renderer.RectF{X: 60, Y: 70, W: 30, H: 30}})
	r.ChunkedDisplay().Commands().Add(renderer.DisplayCommand{Kind: renderer.CmdRect, Rect: renderer.RectCommand{Bounds: renderer.RectF{X: 0, Y: 0, W: 200, H: 200}}})
	adapter := renderer.NewFyneAdapter()
	if !r.PresentFromMutationBatch(adapter) {
		t.Fatal("expected PresentFromMutationBatch to return true")
	}
	if adapter.CurrentFrame() == nil {
		t.Fatal("expected adapter to receive a frame")
	}
	if r.ChunkedDisplay().DirtyChunkCount() != 0 {
		t.Fatal("expected chunks to be marked clean after present")
	}
}

func TestRendererPartialReflowPreservesLayout(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><body><div id="a">a</div><div id="b">b</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	original := r.GetLayoutBox(r.GetRoot())
	if original == nil {
		t.Fatal("expected initial layout")
	}
	var target *renderer.RenderNode
	walkRenderNodes(r.GetRoot(), func(node *renderer.RenderNode) {
		if node != nil && node.Attrs["id"] == "a" {
			target = node
		}
	})
	if target == nil {
		t.Fatal("target node not found")
	}
	if applied := r.ApplyMutationBatch([]renderer.MutationInvalidation{{NodeID: target.ID, Flags: renderer.DirtyLayout}}); applied != 1 {
		t.Fatalf("applied = %d, want 1", applied)
	}
	if r.GetLayoutBox(r.GetRoot()) == nil {
		t.Fatal("layout tree should still exist after partial reflow")
	}
}

func BenchmarkRendererApplyMutationBatch(b *testing.B) {
	r := renderer.NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><body><div id="target">value</div></body></html>`)
	if err != nil {
		b.Fatal(err)
	}
	var target *renderer.RenderNode
	walkRenderNodes(r.GetRoot(), func(node *renderer.RenderNode) {
		if node != nil && node.Attrs["id"] == "target" {
			target = node
		}
	})
	if target == nil {
		b.Fatal("target node not found")
	}
	mutation := []renderer.MutationInvalidation{{NodeID: target.ID, Flags: renderer.DirtyStyle | renderer.DirtyPaint}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.ApplyMutationBatch(mutation)
		r.Incremental().GetInvalidationTracker().ClearAll()
	}
}

func BenchmarkRendererPartialReflow(b *testing.B) {
	r := renderer.NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><body>`+repeatingDivs(200)+`</body></html>`)
	if err != nil {
		b.Fatal(err)
	}
	var target *renderer.RenderNode
	walkRenderNodes(r.GetRoot(), func(node *renderer.RenderNode) {
		if node != nil && node.TagName == "div" && node.Attrs["id"] == "d100" {
			target = node
		}
	})
	if target == nil {
		b.Fatal("target node not found")
	}
	mutation := []renderer.MutationInvalidation{{NodeID: target.ID, Flags: renderer.DirtyLayout}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.ApplyMutationBatch(mutation)
		r.Incremental().GetInvalidationTracker().ClearAll()
		r.SetCurrentLayoutTree(r.LayoutEngine().ComputeLayout(r.GetRoot()))
	}
}

func BenchmarkRendererFullReflow(b *testing.B) {
	r := renderer.NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><body>`+repeatingDivs(200)+`</body></html>`)
	if err != nil {
		b.Fatal(err)
	}
	root := r.GetRoot()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.SetCurrentLayoutTree(r.LayoutEngine().ComputeLayout(root))
	}
}

func BenchmarkRendererInvalidatePaintChunks(b *testing.B) {
	r := renderer.NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><body><div id="a">a</div><div id="b">b</div></body></html>`)
	if err != nil {
		b.Fatal(err)
	}
	var target *renderer.RenderNode
	walkRenderNodes(r.GetRoot(), func(node *renderer.RenderNode) {
		if node != nil && node.Attrs["id"] == "a" {
			target = node
		}
	})
	if target == nil {
		b.Fatal("target node not found")
	}
	r.ChunkedDisplay().Chunks().Add(renderer.PaintChunk{Owner: renderer.LayoutID(uint32(target.ID)), Start: 0, End: 1, Bounds: renderer.RectF{X: 0, Y: 0, W: 10, H: 10}})
	r.ChunkedDisplay().MarkAllClean()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.InvalidatePaintChunks([]int64{target.ID})
		r.ChunkedDisplay().MarkAllClean()
	}
}

func repeatingDivs(n int) string {
	var out strings.Builder
	for i := 0; i < n; i++ {
		out.WriteString(`<div id="d`)
		out.WriteString(strconv.Itoa(i))
		out.WriteString(`">`)
		out.WriteString(strconv.Itoa(i))
		out.WriteString(`</div>`)
	}
	return out.String()
}

func walkRenderNodes(node *renderer.RenderNode, visit func(*renderer.RenderNode)) {
	if node == nil {
		return
	}
	visit(node)
	for _, child := range node.Children {
		walkRenderNodes(child, visit)
	}
}
