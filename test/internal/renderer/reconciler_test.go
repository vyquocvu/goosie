package renderer_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer"
)

func TestDiff_TextChange(t *testing.T) {
	old := &renderer.RenderNode{
		ID:   1,
		Type: renderer.NodeTypeText,
		Text: "hello",
	}
	newNode := &renderer.RenderNode{
		ID:   1,
		Type: renderer.NodeTypeText,
		Text: "world",
	}

	patches := renderer.DiffRenderTree(old, newNode)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].Kind != renderer.PatchUpdateText {
		t.Errorf("expected PatchUpdateText, got %v", patches[0].Kind)
	}
	if patches[0].NewText != "world" {
		t.Errorf("expected 'world', got %q", patches[0].NewText)
	}
	if patches[0].NodeID != 1 {
		t.Errorf("expected NodeID=1, got %d", patches[0].NodeID)
	}
}

func TestDiff_TextNoChange(t *testing.T) {
	old := &renderer.RenderNode{
		ID:   1,
		Type: renderer.NodeTypeText,
		Text: "same",
	}
	newNode := &renderer.RenderNode{
		ID:   1,
		Type: renderer.NodeTypeText,
		Text: "same",
	}

	patches := renderer.DiffRenderTree(old, newNode)
	if len(patches) != 0 {
		t.Fatalf("expected 0 patches for unchanged text, got %d", len(patches))
	}
}

func TestDiff_ClassChange(t *testing.T) {
	old := &renderer.RenderNode{
		ID:      1,
		Type:    renderer.NodeTypeElement,
		TagName: "div",
		Attrs:   map[string]string{"class": "old"},
	}
	newNode := &renderer.RenderNode{
		ID:      1,
		Type:    renderer.NodeTypeElement,
		TagName: "div",
		Attrs:   map[string]string{"class": "new"},
	}

	patches := renderer.DiffRenderTree(old, newNode)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].Kind != renderer.PatchUpdateAttr {
		t.Errorf("expected PatchUpdateAttr, got %v", patches[0].Kind)
	}
	if patches[0].AttrKey != "class" {
		t.Errorf("expected attr key 'class', got %q", patches[0].AttrKey)
	}
	if patches[0].AttrValue != "new" {
		t.Errorf("expected attr value 'new', got %q", patches[0].AttrValue)
	}
}

func TestDiff_AttrAdded(t *testing.T) {
	old := &renderer.RenderNode{
		ID:      1,
		Type:    renderer.NodeTypeElement,
		TagName: "div",
		Attrs:   map[string]string{},
	}
	newNode := &renderer.RenderNode{
		ID:      1,
		Type:    renderer.NodeTypeElement,
		TagName: "div",
		Attrs:   map[string]string{"id": "test"},
	}

	patches := renderer.DiffRenderTree(old, newNode)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].Kind != renderer.PatchUpdateAttr {
		t.Errorf("expected PatchUpdateAttr, got %v", patches[0].Kind)
	}
}

func TestDiff_AttrRemoved(t *testing.T) {
	old := &renderer.RenderNode{
		ID:      1,
		Type:    renderer.NodeTypeElement,
		TagName: "div",
		Attrs:   map[string]string{"id": "test"},
	}
	newNode := &renderer.RenderNode{
		ID:      1,
		Type:    renderer.NodeTypeElement,
		TagName: "div",
		Attrs:   map[string]string{},
	}

	patches := renderer.DiffRenderTree(old, newNode)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].Kind != renderer.PatchUpdateAttr {
		t.Errorf("expected PatchUpdateAttr, got %v", patches[0].Kind)
	}
	if patches[0].AttrValue != "" {
		t.Errorf("expected empty attr value for removal, got %q", patches[0].AttrValue)
	}
}

func TestDiff_SubtreeInsert(t *testing.T) {
	parent := &renderer.RenderNode{
		ID:       1,
		Type:     renderer.NodeTypeElement,
		TagName:  "div",
		Attrs:    map[string]string{},
		Children: []*renderer.RenderNode{},
	}

	newChild := &renderer.RenderNode{
		ID:      2,
		Type:    renderer.NodeTypeElement,
		TagName: "p",
		Attrs:   map[string]string{},
	}
	newParent := &renderer.RenderNode{
		ID:       1,
		Type:     renderer.NodeTypeElement,
		TagName:  "div",
		Attrs:    map[string]string{},
		Children: []*renderer.RenderNode{newChild},
	}

	patches := renderer.DiffRenderTree(parent, newParent)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].Kind != renderer.PatchInsertChild {
		t.Errorf("expected PatchInsertChild, got %v", patches[0].Kind)
	}
	if patches[0].ChildIndex != 0 {
		t.Errorf("expected ChildIndex=0, got %d", patches[0].ChildIndex)
	}
}

func TestDiff_SubtreeRemove(t *testing.T) {
	child := &renderer.RenderNode{
		ID:      2,
		Type:    renderer.NodeTypeElement,
		TagName: "p",
		Attrs:   map[string]string{},
	}
	oldParent := &renderer.RenderNode{
		ID:       1,
		Type:     renderer.NodeTypeElement,
		TagName:  "div",
		Attrs:    map[string]string{},
		Children: []*renderer.RenderNode{child},
	}
	newParent := &renderer.RenderNode{
		ID:       1,
		Type:     renderer.NodeTypeElement,
		TagName:  "div",
		Attrs:    map[string]string{},
		Children: []*renderer.RenderNode{},
	}

	patches := renderer.DiffRenderTree(oldParent, newParent)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].Kind != renderer.PatchRemoveChild {
		t.Errorf("expected PatchRemoveChild, got %v", patches[0].Kind)
	}
	if patches[0].RemoveIndex != 0 {
		t.Errorf("expected RemoveIndex=0, got %d", patches[0].RemoveIndex)
	}
}

func TestDiff_MultipleChanges(t *testing.T) {
	child1 := &renderer.RenderNode{
		ID:   2,
		Type: renderer.NodeTypeText,
		Text: "old text",
	}
	child2 := &renderer.RenderNode{
		ID:      3,
		Type:    renderer.NodeTypeElement,
		TagName: "span",
		Attrs:   map[string]string{},
	}

	old := &renderer.RenderNode{
		ID:       1,
		Type:     renderer.NodeTypeElement,
		TagName:  "div",
		Attrs:    map[string]string{"class": "old"},
		Children: []*renderer.RenderNode{child1, child2},
	}

	newChild1 := &renderer.RenderNode{
		ID:   2,
		Type: renderer.NodeTypeText,
		Text: "new text",
	}
	newChild3 := &renderer.RenderNode{
		ID:      4,
		Type:    renderer.NodeTypeElement,
		TagName: "em",
		Attrs:   map[string]string{},
	}

	newNode := &renderer.RenderNode{
		ID:       1,
		Type:     renderer.NodeTypeElement,
		TagName:  "div",
		Attrs:    map[string]string{"class": "new"},
		Children: []*renderer.RenderNode{newChild1, newChild3},
	}

	patches := renderer.DiffRenderTree(old, newNode)

	// Should have: attr change on div, text change on child1, remove child2, insert newChild3
	if len(patches) < 3 {
		t.Fatalf("expected at least 3 patches, got %d", len(patches))
	}

	// Verify we have the expected patch kinds
	kinds := make(map[renderer.PatchKind]int)
	for _, p := range patches {
		kinds[p.Kind]++
	}
	if kinds[renderer.PatchUpdateAttr] < 1 {
		t.Error("expected at least 1 PatchUpdateAttr")
	}
	if kinds[renderer.PatchUpdateText] < 1 {
		t.Error("expected at least 1 PatchUpdateText")
	}
	if kinds[renderer.PatchRemoveChild] < 1 {
		t.Error("expected at least 1 PatchRemoveChild")
	}
	if kinds[renderer.PatchInsertChild] < 1 {
		t.Error("expected at least 1 PatchInsertChild")
	}
}

func TestDiff_NilTrees(t *testing.T) {
	patches := renderer.DiffRenderTree(nil, nil)
	if len(patches) != 0 {
		t.Errorf("expected 0 patches for nil trees, got %d", len(patches))
	}
}

func TestApplyPatches_TextUpdate(t *testing.T) {
	root := &renderer.RenderNode{
		ID:   1,
		Type: renderer.NodeTypeText,
		Text: "old",
	}

	patches := []renderer.DOMPatch{
		{Kind: renderer.PatchUpdateText, NodeID: 1, NewText: "new"},
	}

	applied := renderer.ApplyPatches(root, patches)
	if applied != 1 {
		t.Errorf("expected 1 applied, got %d", applied)
	}
	if root.Text != "new" {
		t.Errorf("expected 'new', got %q", root.Text)
	}
}

func TestApplyPatches_AttrUpdate(t *testing.T) {
	root := &renderer.RenderNode{
		ID:      1,
		Type:    renderer.NodeTypeElement,
		TagName: "div",
		Attrs:   map[string]string{"class": "old"},
	}

	patches := []renderer.DOMPatch{
		{Kind: renderer.PatchUpdateAttr, NodeID: 1, AttrKey: "class", AttrValue: "new"},
	}

	applied := renderer.ApplyPatches(root, patches)
	if applied != 1 {
		t.Errorf("expected 1 applied, got %d", applied)
	}
	if root.Attrs["class"] != "new" {
		t.Errorf("expected 'new', got %q", root.Attrs["class"])
	}
}

func TestApplyPatches_InsertChild(t *testing.T) {
	root := &renderer.RenderNode{
		ID:       1,
		Type:     renderer.NodeTypeElement,
		TagName:  "div",
		Attrs:    map[string]string{},
		Children: []*renderer.RenderNode{},
	}

	child := &renderer.RenderNode{
		ID:      2,
		Type:    renderer.NodeTypeElement,
		TagName: "p",
		Attrs:   map[string]string{},
	}

	patches := []renderer.DOMPatch{
		{Kind: renderer.PatchInsertChild, NodeID: 1, Child: child, ChildIndex: 0},
	}

	applied := renderer.ApplyPatches(root, patches)
	if applied != 1 {
		t.Errorf("expected 1 applied, got %d", applied)
	}
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}
	if root.Children[0].ID != 2 {
		t.Errorf("expected child ID=2, got %d", root.Children[0].ID)
	}
}

func TestApplyPatches_RemoveChild(t *testing.T) {
	child := &renderer.RenderNode{
		ID:      2,
		Type:    renderer.NodeTypeElement,
		TagName: "p",
		Attrs:   map[string]string{},
	}
	root := &renderer.RenderNode{
		ID:       1,
		Type:     renderer.NodeTypeElement,
		TagName:  "div",
		Attrs:    map[string]string{},
		Children: []*renderer.RenderNode{child},
	}

	patches := []renderer.DOMPatch{
		{Kind: renderer.PatchRemoveChild, NodeID: 1, RemoveIndex: 0},
	}

	applied := renderer.ApplyPatches(root, patches)
	if applied != 1 {
		t.Errorf("expected 1 applied, got %d", applied)
	}
	if len(root.Children) != 0 {
		t.Errorf("expected 0 children, got %d", len(root.Children))
	}
}

func TestComputeDirtyFlags(t *testing.T) {
	patches := []renderer.DOMPatch{
		{Kind: renderer.PatchUpdateText, DirtyFlags: renderer.DirtyLayout | renderer.DirtyPaint},
		{Kind: renderer.PatchUpdateAttr, DirtyFlags: renderer.DirtyPaint},
	}

	flags := renderer.ComputeDirtyFlags(patches)
	if flags&renderer.DirtyLayout == 0 {
		t.Error("expected DirtyLayout to be set")
	}
	if flags&renderer.DirtyPaint == 0 {
		t.Error("expected DirtyPaint to be set")
	}
}

func TestNeedsRelayout(t *testing.T) {
	patches := []renderer.DOMPatch{
		{Kind: renderer.PatchUpdateText, DirtyFlags: renderer.DirtyLayout | renderer.DirtyPaint},
	}
	if !renderer.NeedsRelayout(patches) {
		t.Error("expected NeedsRelayout to be true")
	}

	patches2 := []renderer.DOMPatch{
		{Kind: renderer.PatchUpdateAttr, DirtyFlags: renderer.DirtyPaint},
	}
	if renderer.NeedsRelayout(patches2) {
		t.Error("expected NeedsRelayout to be false for paint-only patches")
	}
}

func BenchmarkDiff_LargeTree(b *testing.B) {
	// Build a tree with ~5000 nodes
	var buildTree func(id, breadth, depth int) *renderer.RenderNode
	buildTree = func(id, breadth, depth int) *renderer.RenderNode {
		node := &renderer.RenderNode{
			ID:       int64(id),
			Type:     renderer.NodeTypeElement,
			TagName:  "div",
			Attrs:    map[string]string{"class": "node"},
			Children: make([]*renderer.RenderNode, 0, breadth),
		}
		if depth <= 0 {
			return node
		}
		for i := 0; i < breadth; i++ {
			child := buildTree(id*breadth+i+1, breadth, depth-1)
			child.Parent = node
			node.Children = append(node.Children, child)
		}
		return node
	}

	old := buildTree(1, 5, 5) // 5^5 = 3125+ nodes
	newTree := buildTree(1, 5, 5)
	// Change one leaf
	newTree.Children[0].Children[0].Children[0].Children[0].Attrs["class"] = "changed"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = renderer.DiffRenderTree(old, newTree)
	}
}
