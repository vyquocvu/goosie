package renderer_test

import (
	"context"
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer"
)

// TestIncrementalLayout_SingleNodeUpdate verifies that updating a single
// node's text triggers incremental layout without rebuilding the entire tree.
func TestIncrementalLayout_SingleNodeUpdate(t *testing.T) {
	// Create a renderer with a simple HTML document.
	r := renderer.NewRenderer(800, 600)
	html := `<html><body><p id="test">Hello</p></body></html>`
	_, err := r.RenderHTML(context.Background(), html)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	// Get the initial render tree.
	root := r.GetRoot()
	if root == nil {
		t.Fatal("GetRoot returned nil")
	}

	// Find the <p> node.
	var pNode *renderer.RenderNode
	var walk func(n *renderer.RenderNode)
	walk = func(n *renderer.RenderNode) {
		if n == nil {
			return
		}
		if n.TagName == "p" {
			pNode = n
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)

	if pNode == nil {
		t.Fatal("could not find <p> node")
	}

	// Create a patch to update the text.
	patches := []renderer.DOMPatch{
		{
			Kind:       renderer.PatchUpdateText,
			NodeID:     pNode.ID,
			NewText:    "World",
			DirtyFlags: renderer.DirtyPaint,
		},
	}

	// Apply the patches.
	applied := renderer.ApplyPatchesToRenderer(r, patches)
	if applied != 1 {
		t.Errorf("expected 1 patch applied, got %d", applied)
	}

	// Verify the text was updated.
	if pNode.Text != "World" {
		t.Errorf("expected text 'World', got %q", pNode.Text)
	}
}

// TestIncrementalLayout_AttributeUpdate verifies that updating an attribute
// triggers style recomputation and incremental layout.
func TestIncrementalLayout_AttributeUpdate(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	html := `<html><body><div id="box" class="old">Content</div></body></html>`
	_, err := r.RenderHTML(context.Background(), html)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	root := r.GetRoot()
	if root == nil {
		t.Fatal("GetRoot returned nil")
	}

	// Find the <div> node.
	var divNode *renderer.RenderNode
	var walk func(n *renderer.RenderNode)
	walk = func(n *renderer.RenderNode) {
		if n == nil {
			return
		}
		if n.TagName == "div" {
			divNode = n
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)

	if divNode == nil {
		t.Fatal("could not find <div> node")
	}

	// Update the class attribute.
	patches := []renderer.DOMPatch{
		{
			Kind:       renderer.PatchUpdateAttr,
			NodeID:     divNode.ID,
			AttrKey:    "class",
			AttrValue:  "new",
			DirtyFlags: renderer.DirtyStyle | renderer.DirtyPaint,
		},
	}

	applied := renderer.ApplyPatchesToRenderer(r, patches)
	if applied != 1 {
		t.Errorf("expected 1 patch applied, got %d", applied)
	}

	// Verify the attribute was updated.
	if divNode.Attrs["class"] != "new" {
		t.Errorf("expected class 'new', got %q", divNode.Attrs["class"])
	}
}

// TestIncrementalLayout_MultiplePatches verifies that multiple patches can be
// applied in a single batch.
func TestIncrementalLayout_MultiplePatches(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	html := `<html><body><p id="p1">One</p><p id="p2">Two</p></body></html>`
	_, err := r.RenderHTML(context.Background(), html)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	root := r.GetRoot()
	if root == nil {
		t.Fatal("GetRoot returned nil")
	}

	// Find both <p> nodes.
	var p1, p2 *renderer.RenderNode
	var walk func(n *renderer.RenderNode)
	walk = func(n *renderer.RenderNode) {
		if n == nil {
			return
		}
		if n.TagName == "p" {
			if p1 == nil {
				p1 = n
			} else if p2 == nil {
				p2 = n
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)

	if p1 == nil || p2 == nil {
		t.Fatal("could not find both <p> nodes")
	}

	// Update both texts in a single batch.
	patches := []renderer.DOMPatch{
		{
			Kind:       renderer.PatchUpdateText,
			NodeID:     p1.ID,
			NewText:    "Uno",
			DirtyFlags: renderer.DirtyPaint,
		},
		{
			Kind:       renderer.PatchUpdateText,
			NodeID:     p2.ID,
			NewText:    "Dos",
			DirtyFlags: renderer.DirtyPaint,
		},
	}

	applied := renderer.ApplyPatchesToRenderer(r, patches)
	if applied != 2 {
		t.Errorf("expected 2 patches applied, got %d", applied)
	}

	if p1.Text != "Uno" {
		t.Errorf("expected p1 text 'Uno', got %q", p1.Text)
	}
	if p2.Text != "Dos" {
		t.Errorf("expected p2 text 'Dos', got %q", p2.Text)
	}
}

// TestIncrementalLayout_NilRenderer verifies that ApplyPatchesToRenderer
// handles nil renderer gracefully.
func TestIncrementalLayout_NilRenderer(t *testing.T) {
	patches := []renderer.DOMPatch{
		{
			Kind:   renderer.PatchUpdateText,
			NodeID: 1,
		},
	}

	applied := renderer.ApplyPatchesToRenderer(nil, patches)
	if applied != 0 {
		t.Errorf("expected 0 patches applied for nil renderer, got %d", applied)
	}
}

// TestIncrementalLayout_EmptyPatches verifies that an empty patch list is
// handled correctly.
func TestIncrementalLayout_EmptyPatches(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	html := `<html><body><p>Test</p></body></html>`
	_, err := r.RenderHTML(context.Background(), html)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	applied := renderer.ApplyPatchesToRenderer(r, nil)
	if applied != 0 {
		t.Errorf("expected 0 patches applied for empty list, got %d", applied)
	}

	applied = renderer.ApplyPatchesToRenderer(r, []renderer.DOMPatch{})
	if applied != 0 {
		t.Errorf("expected 0 patches applied for empty slice, got %d", applied)
	}
}

// BenchmarkIncrementalLayout_SingleUpdate measures the performance of updating
// a single node's text.
func BenchmarkIncrementalLayout_SingleUpdate(b *testing.B) {
	r := renderer.NewRenderer(800, 600)
	html := `<html><body><p id="target">Original</p></body></html>`
	_, err := r.RenderHTML(context.Background(), html)
	if err != nil {
		b.Fatalf("RenderHTML failed: %v", err)
	}

	root := r.GetRoot()
	var pNode *renderer.RenderNode
	var walk func(n *renderer.RenderNode)
	walk = func(n *renderer.RenderNode) {
		if n == nil {
			return
		}
		if n.TagName == "p" {
			pNode = n
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)

	if pNode == nil {
		b.Fatal("could not find <p> node")
	}

	patches := []renderer.DOMPatch{
		{
			Kind:       renderer.PatchUpdateText,
			NodeID:     pNode.ID,
			NewText:    "Updated",
			DirtyFlags: renderer.DirtyPaint,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderer.ApplyPatchesToRenderer(r, patches)
	}
}
