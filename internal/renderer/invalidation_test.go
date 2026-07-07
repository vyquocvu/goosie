package renderer

import (
	"testing"
)

func TestNewInvalidationTracker(t *testing.T) {
	it := NewInvalidationTracker()

	if it == nil {
		t.Fatal("NewInvalidationTracker returned nil")
	}
	if it.dirtyNodes == nil {
		t.Error("dirtyNodes map not initialized")
	}
}

func TestMarkDirty(t *testing.T) {
	it := NewInvalidationTracker()
	nodeID := int64(1)

	it.MarkDirty(nodeID, DirtyLayout)

	if !it.IsDirty(nodeID) {
		t.Error("Node should be marked as dirty")
	}

	flags := it.GetDirtyFlags(nodeID)
	if flags != DirtyLayout {
		t.Errorf("Expected DirtyLayout flag, got %v", flags)
	}
}

func TestMarkDirtyMultipleFlags(t *testing.T) {
	it := NewInvalidationTracker()
	nodeID := int64(1)

	it.MarkDirty(nodeID, DirtyLayout)
	it.MarkDirty(nodeID, DirtyPaint)

	flags := it.GetDirtyFlags(nodeID)
	if flags&DirtyLayout == 0 {
		t.Error("DirtyLayout flag should be set")
	}
	if flags&DirtyPaint == 0 {
		t.Error("DirtyPaint flag should be set")
	}
}

func TestClearDirty(t *testing.T) {
	it := NewInvalidationTracker()
	nodeID := int64(1)

	it.MarkDirty(nodeID, DirtyLayout)

	if !it.IsDirty(nodeID) {
		t.Error("Node should be dirty before clear")
	}

	it.ClearDirty(nodeID)

	if it.IsDirty(nodeID) {
		t.Error("Node should not be dirty after clear")
	}
}

func TestGetDirtyNodes(t *testing.T) {
	it := NewInvalidationTracker()

	it.MarkDirty(1, DirtyLayout)
	it.MarkDirty(2, DirtyPaint)
	it.MarkDirty(3, DirtyStyle)

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
	it := NewInvalidationTracker()

	it.MarkDirty(1, DirtyLayout)
	it.MarkDirty(2, DirtyPaint)

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
	it := NewInvalidationTracker()

	// Create a tree: parent -> child
	parent := NewRenderNode(NodeTypeElement)
	parent.TagName = "div"

	child := NewRenderNode(NodeTypeElement)
	child.TagName = "p"
	parent.AddChild(child)

	// Invalidate child with layout flag
	it.PropagateInvalidation(child, DirtyLayout)

	// Both child and parent should be dirty
	if !it.IsDirty(child.ID) {
		t.Error("Child should be dirty")
	}
	if !it.IsDirty(parent.ID) {
		t.Error("Parent should be dirty (layout propagates up)")
	}
}

func TestPropagateInvalidationSubtree(t *testing.T) {
	it := NewInvalidationTracker()

	// Create a tree: parent -> child1, child2
	parent := NewRenderNode(NodeTypeElement)
	parent.TagName = "div"

	child1 := NewRenderNode(NodeTypeElement)
	child1.TagName = "p"
	parent.AddChild(child1)

	child2 := NewRenderNode(NodeTypeElement)
	child2.TagName = "p"
	parent.AddChild(child2)

	// Invalidate parent with subtree flag
	it.PropagateInvalidation(parent, DirtySubtree)

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
	ile := NewIncrementalLayoutEngine(800, 600)

	if ile == nil {
		t.Fatal("NewIncrementalLayoutEngine returned nil")
	}
	if ile.LayoutEngine == nil {
		t.Error("LayoutEngine not initialized")
	}
	if ile.invalidation == nil {
		t.Error("Invalidation tracker not initialized")
	}
}

func TestInvalidateNode(t *testing.T) {
	ile := NewIncrementalLayoutEngine(800, 600)

	node := NewRenderNode(NodeTypeElement)
	node.TagName = "div"

	ile.InvalidateNode(node, DirtyLayout)

	if !ile.IsNodeDirty(node.ID) {
		t.Error("Node should be dirty after invalidation")
	}
}

func TestComputeIncrementalLayoutNoDirty(t *testing.T) {
	ile := NewIncrementalLayoutEngine(800, 600)

	// Create a render tree
	root := NewRenderNode(NodeTypeElement)
	root.TagName = "div"

	text := NewRenderNode(NodeTypeText)
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
	ile := NewIncrementalLayoutEngine(800, 600)

	// Create a render tree
	root := NewRenderNode(NodeTypeElement)
	root.TagName = "div"

	text := NewRenderNode(NodeTypeText)
	text.Text = "Test"
	root.AddChild(text)

	// First layout
	layout1 := ile.ComputeIncrementalLayout(root, nil)

	// Mark node as dirty
	ile.InvalidateNode(text, DirtyLayout)

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
	it := NewInvalidationTracker()
	nodeID := int64(1)

	tests := []struct {
		name  string
		flags DirtyFlag
	}{
		{"layout", DirtyLayout},
		{"paint", DirtyPaint},
		{"style", DirtyStyle},
		{"subtree", DirtySubtree},
		{"layout+paint", DirtyLayout | DirtyPaint},
		{"all", DirtyLayout | DirtyPaint | DirtyStyle | DirtySubtree},
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
