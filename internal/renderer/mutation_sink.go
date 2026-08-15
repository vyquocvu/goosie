package renderer

import "github.com/vyquocvu/goosie/internal/js"

// MutationSink is the typed seam that consumes JS DOM mutation batches and
// routes them into the renderer's invalidation tracker without re-serializing
// the document. When an adapter is supplied, the sink also requests a
// present after invalidating paint chunks so dirty pixels reach the Fyne
// canvas in a single chain.
type MutationSink struct {
	r       *Renderer
	store   *NodeIDLookup
	adapter *FyneAdapter
	present func()
}

func NewMutationSink(r *Renderer, store *NodeIDLookup, present func()) *MutationSink {
	if present == nil {
		present = func() {}
	}
	return &MutationSink{r: r, store: store, present: present}
}

// NewMutationSinkWithAdapter wires the sink to a FyneAdapter so the
// present path can drive IncrementalPainter + FyneAdapter.PresentFrame
// without any caller-provided closure.
func NewMutationSinkWithAdapter(r *Renderer, store *NodeIDLookup, adapter *FyneAdapter) *MutationSink {
	sink := &MutationSink{r: r, store: store, adapter: adapter}
	sink.present = func() { sink.r.PresentFromMutationBatch(adapter) }
	return sink
}

func (s *MutationSink) Handle(batch []js.DOMMutation) {
	if s == nil || s.r == nil || len(batch) == 0 {
		return
	}
	invalidations := make([]MutationInvalidation, 0, len(batch))
	paintIDs := make([]int64, 0, len(batch))
	for _, mutation := range batch {
		id := s.lookup(mutation.TargetID)
		if id == 0 {
			id = s.lookup(mutation.ParentID)
		}
		if id == 0 {
			// Stale NodeID (superseded document or unknown element):
			// reject safely — nothing to invalidate or sync.
			continue
		}
		// Sync the mutation's value into the render tree before
		// invalidation so the subsequent layout/present reads fresh
		// content instead of repainting the stale cached tree.
		s.r.ApplyTypedMutationValue(id, mutation.Kind, mutation.Attribute, mutation.NewValue)
		flags := DirtyStyle | DirtyPaint
		switch mutation.Kind {
		case js.MutationInsert, js.MutationRemove, js.MutationReplace:
			flags = DirtyLayout | DirtySubtree | DirtyPaint
		case js.MutationSetText:
			flags = DirtyLayout | DirtyStyle | DirtyPaint
		case js.MutationSetAttribute:
			flags = DirtyStyle | DirtyPaint
		}
		invalidations = append(invalidations, MutationInvalidation{NodeID: id, Flags: flags})
		paintIDs = append(paintIDs, id)
	}
	if len(invalidations) == 0 {
		return
	}
	applied := s.r.ApplyMutationBatch(invalidations)
	if applied > 0 {
		s.r.RecordCoalescedMutations(len(batch))
		s.r.InvalidatePaintChunks(paintIDs)
		s.present()
	}
}

// ApplyTypedMutationValue syncs the value carried by a typed JS DOM
// mutation into the render tree under treeMu, so the next layout and
// present read fresh content. set-text updates the node's (or its first
// text child's) Text; set-attribute updates the Attrs map. Structural
// mutations have no render-side node to update here — they fall back to
// the full reparse path. Stale NodeIDs resolve to nothing and are
// rejected safely (returns false).
func (r *Renderer) ApplyTypedMutationValue(renderID int64, kind js.MutationKind, attr, newValue string) bool {
	if renderID == 0 {
		return false
	}
	r.treeMu.Lock()
	defer r.treeMu.Unlock()
	if r.currentRenderTree == nil {
		return false
	}
	node := findRenderNodeByIDRoot(r.currentRenderTree, renderID)
	if node == nil {
		return false
	}
	switch kind {
	case js.MutationSetText:
		setRenderText(node, newValue)
		return true
	case js.MutationSetAttribute:
		if node.Attrs == nil {
			node.Attrs = make(map[string]string)
		}
		node.Attrs[attr] = newValue
		return true
	default:
		return false
	}
}

// setRenderText updates the text content of a render node. JS
// element.textContent replaces the element's children with a single text
// node, so an element target updates its first text child; a text target
// is itself. When an element has no text child yet, one is appended so
// the relayout can render the new content.
func setRenderText(node *RenderNode, text string) {
	if node.Type == NodeTypeText {
		node.Text = text
		return
	}
	for _, c := range node.Children {
		if c.Type == NodeTypeText {
			c.Text = text
			return
		}
	}
	child := NewRenderNode(NodeTypeText)
	child.Text = text
	node.AddChild(child)
}

func (s *MutationSink) lookup(goosieID string) int64 {
	if s.store == nil || goosieID == "" {
		return 0
	}
	return s.store.Lookup(goosieID)
}
