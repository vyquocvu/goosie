package renderer

import "github.com/vyquocvu/goosie/internal/js"

// MutationSink is the typed seam that consumes JS DOM mutation batches and
// routes them into the renderer's invalidation tracker without re-serializing
// the document.
type MutationSink struct {
	r       *Renderer
	store   *NodeIDLookup
	present func()
}

func NewMutationSink(r *Renderer, store *NodeIDLookup, present func()) *MutationSink {
	if present == nil {
		present = func() {}
	}
	return &MutationSink{r: r, store: store, present: present}
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
			continue
		}
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
		s.r.InvalidatePaintChunks(paintIDs)
		s.present()
	}
}

func (s *MutationSink) lookup(goosieID string) int64 {
	if s.store == nil || goosieID == "" {
		return 0
	}
	return s.store.Lookup(goosieID)
}
