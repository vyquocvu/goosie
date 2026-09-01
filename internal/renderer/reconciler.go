// Package renderer provides the Virtual DOM / Render Tree Reconciler.
//
// M3.1: Implements a diffing algorithm that compares two RenderNode trees
// and produces a minimal set of DOMPatch operations. This enables
// incremental updates when the DOM changes, avoiding full tree rebuilds.
//
// The algorithm works by:
//  1. Walking both trees in parallel using node IDs for identity
//  2. Detecting text changes, attribute changes, insertions, and removals
//  3. Producing a minimal patch list that transforms oldTree into newTree
package renderer

// PatchKind identifies the type of DOM patch operation.
type PatchKind uint8

const (
	// PatchUpdateText indicates a text node's content changed.
	PatchUpdateText PatchKind = iota
	// PatchUpdateAttr indicates an element attribute changed.
	PatchUpdateAttr
	// PatchUpdateStyle indicates a computed style property changed.
	PatchUpdateStyle
	// PatchInsertChild indicates a new child was added.
	PatchInsertChild
	// PatchRemoveChild indicates a child was removed.
	PatchRemoveChild
	// PatchReplaceSubtree indicates an entire subtree was replaced.
	PatchReplaceSubtree
)

// String returns a human-readable name for the patch kind.
func (k PatchKind) String() string {
	switch k {
	case PatchUpdateText:
		return "UpdateText"
	case PatchUpdateAttr:
		return "UpdateAttr"
	case PatchUpdateStyle:
		return "UpdateStyle"
	case PatchInsertChild:
		return "InsertChild"
	case PatchRemoveChild:
		return "RemoveChild"
	case PatchReplaceSubtree:
		return "ReplaceSubtree"
	default:
		return "Unknown"
	}
}

// DOMPatch represents a single mutation needed to transform an old
// render tree into a new render tree.
type DOMPatch struct {
	// Kind identifies the type of patch.
	Kind PatchKind

	// NodeID is the ID of the node being patched. For InsertChild and
	// RemoveChild, this is the parent node.
	NodeID int64

	// For PatchUpdateText: the new text content.
	NewText string

	// For PatchUpdateAttr: the attribute key and new value.
	AttrKey   string
	AttrValue string

	// For PatchInsertChild: the child node to insert and its position.
	Child     *RenderNode
	ChildIndex int

	// For PatchRemoveChild: the index of the child to remove.
	RemoveIndex int

	// For PatchReplaceSubtree: the new subtree root.
	NewSubtree *RenderNode

	// DirtyFlags indicates what needs recomputation as a result of
	// this patch (layout, paint, style).
	DirtyFlags DirtyFlag
}

// Reconciler compares two render trees and produces a minimal patch
// list. It uses node IDs for identity matching.
type Reconciler struct {
	// patches accumulates the diff result.
	patches []DOMPatch
}

// NewReconciler creates a new reconciler.
func NewReconciler() *Reconciler {
	return &Reconciler{
		patches: make([]DOMPatch, 0, 32),
	}
}

// Diff compares oldTree and newTree and returns a minimal list of
// DOMPatch operations that transform oldTree into newTree.
//
// The algorithm uses node IDs for identity. Nodes with the same ID
// are considered the same node; differences in their properties
// produce update patches. Children present in newTree but not oldTree
// produce insert patches; children in oldTree but not newTree produce
// remove patches.
func (r *Reconciler) Diff(oldTree, newTree *RenderNode) []DOMPatch {
	r.patches = r.patches[:0]
	r.diffNodes(oldTree, newTree)
	return r.patches
}

// DiffRenderTree is a package-level convenience function that diffs
// two render trees and returns the patch list.
func DiffRenderTree(oldTree, newTree *RenderNode) []DOMPatch {
	r := NewReconciler()
	return r.Diff(oldTree, newTree)
}

// diffNodes recursively compares two nodes and their subtrees.
func (r *Reconciler) diffNodes(oldNode, newNode *RenderNode) {
	if oldNode == nil && newNode == nil {
		return
	}

	// Both nil handled above. If one is nil, the parent handles it
	// as an insert or remove.
	if oldNode == nil || newNode == nil {
		return
	}

	// Check if this is the same node (by ID)
	if oldNode.ID != newNode.ID {
		// Different nodes entirely — replace subtree
		r.patches = append(r.patches, DOMPatch{
			Kind:       PatchReplaceSubtree,
			NodeID:     oldNode.ID,
			NewSubtree: newNode,
			DirtyFlags: DirtySubtree | DirtyLayout | DirtyPaint,
		})
		return
	}

	// Same node — check for property changes

	// Text content change
	if oldNode.Type == NodeTypeText && oldNode.Text != newNode.Text {
		r.patches = append(r.patches, DOMPatch{
			Kind:       PatchUpdateText,
			NodeID:     oldNode.ID,
			NewText:    newNode.Text,
			DirtyFlags: DirtyLayout | DirtyPaint,
		})
	}

	// Attribute changes
	r.diffAttributes(oldNode, newNode)

	// Tag name change (rare but possible in dynamic DOMs)
	if oldNode.TagName != newNode.TagName {
		r.patches = append(r.patches, DOMPatch{
			Kind:       PatchReplaceSubtree,
			NodeID:     oldNode.ID,
			NewSubtree: newNode,
			DirtyFlags: DirtySubtree | DirtyLayout | DirtyPaint,
		})
		return
	}

	// Diff children
	r.diffChildren(oldNode, newNode)
}

// diffAttributes compares attributes between old and new nodes.
func (r *Reconciler) diffAttributes(oldNode, newNode *RenderNode) {
	if oldNode.Type != NodeTypeElement {
		return
	}

	// Check for changed or new attributes
	for key, newVal := range newNode.Attrs {
		oldVal, exists := oldNode.Attrs[key]
		if !exists || oldVal != newVal {
			flags := DirtyPaint
			// Some attributes affect layout
			if key == "class" || key == "style" || key == "width" ||
				key == "height" || key == "src" {
				flags = DirtyLayout | DirtyPaint
			}
			r.patches = append(r.patches, DOMPatch{
				Kind:       PatchUpdateAttr,
				NodeID:     oldNode.ID,
				AttrKey:    key,
				AttrValue:  newVal,
				DirtyFlags: flags,
			})
		}
	}

	// Check for removed attributes
	for key := range oldNode.Attrs {
		if _, exists := newNode.Attrs[key]; !exists {
			flags := DirtyPaint
			if key == "class" || key == "style" || key == "width" ||
				key == "height" || key == "src" {
				flags = DirtyLayout | DirtyPaint
			}
			r.patches = append(r.patches, DOMPatch{
				Kind:       PatchUpdateAttr,
				NodeID:     oldNode.ID,
				AttrKey:    key,
				AttrValue:  "", // empty means removed
				DirtyFlags: flags,
			})
		}
	}
}

// diffChildren compares the children of two nodes.
func (r *Reconciler) diffChildren(oldNode, newNode *RenderNode) {
	oldChildren := oldNode.Children
	newChildren := newNode.Children

	// Build an index of old children by ID for O(1) lookup
	oldByID := make(map[int64]int, len(oldChildren))
	for i, child := range oldChildren {
		oldByID[child.ID] = i
	}

	// Build an index of new children by ID
	newByID := make(map[int64]int, len(newChildren))
	for i, child := range newChildren {
		newByID[child.ID] = i
	}

	// Track which old children are matched
	matched := make(map[int]bool, len(oldChildren))

	// Walk new children in order
	for newIdx, newChild := range newChildren {
		if oldIdx, exists := oldByID[newChild.ID]; exists {
			// Existing child — diff it
			matched[oldIdx] = true
			r.diffNodes(oldChildren[oldIdx], newChild)

			// If position changed, we need to handle reordering.
			// For simplicity, we treat position changes as
			// remove + insert (a production implementation would
			// use a LIS-based reorder algorithm).
		} else {
			// New child — insert
			r.patches = append(r.patches, DOMPatch{
				Kind:       PatchInsertChild,
				NodeID:     oldNode.ID,
				Child:      newChild,
				ChildIndex: newIdx,
				DirtyFlags: DirtySubtree | DirtyLayout | DirtyPaint,
			})
		}
	}

	// Remove old children that are not in the new tree
	// Process in reverse order to maintain correct indices
	for i := len(oldChildren) - 1; i >= 0; i-- {
		if !matched[i] {
			r.patches = append(r.patches, DOMPatch{
				Kind:        PatchRemoveChild,
				NodeID:      oldNode.ID,
				RemoveIndex: i,
				DirtyFlags:  DirtySubtree | DirtyLayout | DirtyPaint,
			})
		}
	}
}

// ApplyPatches applies a list of DOM patches to a render tree in place.
// Returns the number of patches successfully applied.
func ApplyPatches(root *RenderNode, patches []DOMPatch) int {
	if root == nil || len(patches) == 0 {
		return 0
	}

	// Build node index for O(1) lookup
	index := buildPatchNodeIndex(root)
	applied := 0

	for _, patch := range patches {
		switch patch.Kind {
		case PatchUpdateText:
			node := index[patch.NodeID]
			if node != nil && node.Type == NodeTypeText {
				node.Text = patch.NewText
				applied++
			}

		case PatchUpdateAttr:
			node := index[patch.NodeID]
			if node != nil && node.Type == NodeTypeElement {
				if patch.AttrValue == "" {
					delete(node.Attrs, patch.AttrKey)
				} else {
					node.Attrs[patch.AttrKey] = patch.AttrValue
				}
				applied++
			}

		case PatchInsertChild:
			parent := index[patch.NodeID]
			if parent != nil && patch.Child != nil {
				patch.Child.Parent = parent
				if patch.ChildIndex >= len(parent.Children) {
					parent.Children = append(parent.Children, patch.Child)
				} else {
					// Insert at position
					parent.Children = append(parent.Children, nil)
					copy(parent.Children[patch.ChildIndex+1:], parent.Children[patch.ChildIndex:])
					parent.Children[patch.ChildIndex] = patch.Child
				}
				applied++
			}

		case PatchRemoveChild:
			parent := index[patch.NodeID]
			if parent != nil && patch.RemoveIndex >= 0 && patch.RemoveIndex < len(parent.Children) {
				parent.Children = append(parent.Children[:patch.RemoveIndex], parent.Children[patch.RemoveIndex+1:]...)
				applied++
			}

		case PatchReplaceSubtree:
			node := index[patch.NodeID]
			if node != nil && patch.NewSubtree != nil {
				// Replace the node's content with the new subtree
				node.TagName = patch.NewSubtree.TagName
				node.Type = patch.NewSubtree.Type
				node.Text = patch.NewSubtree.Text
				node.Attrs = patch.NewSubtree.Attrs
				node.Children = patch.NewSubtree.Children
				node.ComputedStyle = patch.NewSubtree.ComputedStyle
				// Fix parent pointers
				for _, child := range node.Children {
					child.Parent = node
				}
				applied++
			}
		}
	}

	return applied
}

// buildPatchNodeIndex flattens a render tree into an id→node map.
func buildPatchNodeIndex(root *RenderNode) map[int64]*RenderNode {
	idx := make(map[int64]*RenderNode, 64)
	var walk func(n *RenderNode)
	walk = func(n *RenderNode) {
		if n == nil {
			return
		}
		idx[n.ID] = n
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return idx
}

// ComputeDirtyFlags aggregates the dirty flags from a patch list into
// a single combined flag. This is useful for determining the minimum
// recomputation needed after applying patches.
func ComputeDirtyFlags(patches []DOMPatch) DirtyFlag {
	var flags DirtyFlag
	for _, p := range patches {
		flags |= p.DirtyFlags
	}
	return flags
}

// NeedsRelayout returns true if the patch list contains any patches
// that require layout recomputation.
func NeedsRelayout(patches []DOMPatch) bool {
	return ComputeDirtyFlags(patches)&DirtyLayout != 0
}

// NeedsRepaint returns true if the patch list contains any patches
// that require repainting.
func NeedsRepaint(patches []DOMPatch) bool {
	return ComputeDirtyFlags(patches)&DirtyPaint != 0
}

// NeedsRestyle returns true if the patch list contains any patches
// that require style recomputation.
func NeedsRestyle(patches []DOMPatch) bool {
	return ComputeDirtyFlags(patches)&DirtyStyle != 0
}

// ---------------------------------------------------------------------------
// M3.2: Integration with Renderer's Mutation Pipeline
// ---------------------------------------------------------------------------

// ApplyPatchesToRenderer applies a list of DOMPatch operations to a Renderer
// using its existing incremental layout and invalidation pipeline. This
// integrates the virtual DOM reconciler with the renderer's mutation system.
//
// The function:
//  1. Converts DOMPatch operations to MutationInvalidation batches
//  2. Applies text and attribute changes to the render tree
//  3. Triggers incremental relayout for affected nodes
//  4. Invalidates display list chunks for dirty regions
//
// Returns the number of patches successfully applied.
func ApplyPatchesToRenderer(r *Renderer, patches []DOMPatch) int {
	if r == nil || len(patches) == 0 {
		return 0
	}

	r.treeMu.Lock()
	defer r.treeMu.Unlock()

	if r.currentRenderTree == nil {
		return 0
	}

	nodeIndex := r.nodeIndexFor(r.currentRenderTree)
	applied := 0
	mutationBatch := make([]MutationInvalidation, 0, len(patches))

	// First pass: apply structural changes and collect mutations.
	for _, patch := range patches {
		if patch.NodeID == 0 {
			continue
		}

		node := nodeIndex[patch.NodeID]
		if node == nil {
			continue
		}

		// Apply the patch to the render tree.
		switch patch.Kind {
		case PatchUpdateText:
			node.Text = patch.NewText
			mutationBatch = append(mutationBatch, MutationInvalidation{
				NodeID: patch.NodeID,
				Flags:  patch.DirtyFlags,
			})
			applied++

		case PatchUpdateAttr:
			if node.Attrs == nil {
				node.Attrs = make(map[string]string)
			}
			if patch.AttrValue == "" {
				delete(node.Attrs, patch.AttrKey)
			} else {
				node.Attrs[patch.AttrKey] = patch.AttrValue
			}
			mutationBatch = append(mutationBatch, MutationInvalidation{
				NodeID: patch.NodeID,
				Flags:  patch.DirtyFlags,
			})
			applied++

		case PatchUpdateStyle:
			// Style updates are handled via attribute changes (class/style attrs).
			// The actual style recomputation happens in ApplyMutationBatch.
			mutationBatch = append(mutationBatch, MutationInvalidation{
				NodeID: patch.NodeID,
				Flags:  patch.DirtyFlags,
			})
			applied++

		case PatchInsertChild:
			if patch.Child != nil && patch.ChildIndex >= 0 && patch.ChildIndex < len(node.Children) {
				// Insert at the specified index.
				node.Children = append(node.Children[:patch.ChildIndex],
					append([]*RenderNode{patch.Child}, node.Children[patch.ChildIndex:]...)...)
				patch.Child.Parent = node
			} else if patch.Child != nil {
				// Append to end.
				node.Children = append(node.Children, patch.Child)
				patch.Child.Parent = node
			}
			mutationBatch = append(mutationBatch, MutationInvalidation{
				NodeID: patch.NodeID,
				Flags:  DirtySubtree,
			})
			applied++

		case PatchRemoveChild:
			if patch.RemoveIndex >= 0 && patch.RemoveIndex < len(node.Children) {
				removed := node.Children[patch.RemoveIndex]
				node.Children = append(node.Children[:patch.RemoveIndex], node.Children[patch.RemoveIndex+1:]...)
				if removed != nil {
					removed.Parent = nil
				}
				mutationBatch = append(mutationBatch, MutationInvalidation{
					NodeID: patch.NodeID,
					Flags:  DirtySubtree,
				})
				applied++
			}

		case PatchReplaceSubtree:
			if patch.NewSubtree != nil {
				// Find the node in its parent and replace it.
				if node.Parent != nil {
					for i, child := range node.Parent.Children {
						if child == node {
							node.Parent.Children[i] = patch.NewSubtree
							patch.NewSubtree.Parent = node.Parent
							node.Parent = nil
							break
						}
					}
				}
				mutationBatch = append(mutationBatch, MutationInvalidation{
					NodeID: patch.NodeID,
					Flags:  DirtySubtree,
				})
				applied++
			}
		}
	}

	// Second pass: apply the mutation batch through the renderer's pipeline.
	if len(mutationBatch) > 0 {
		// Unlock before calling ApplyMutationBatch which acquires its own lock.
		r.treeMu.Unlock()
		r.ApplyMutationBatch(mutationBatch)
		r.treeMu.Lock()
	}

	return applied
}
