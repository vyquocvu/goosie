// Package renderer provides incremental layout for the Goosie engine.
//
// M4.4: Implement incremental layout
//
// The IncrementalLayoutEngine tracks dirty layout objects and performs
// minimal reflow by finding the smallest valid reflow root. It caches
// intrinsic sizes and preserves unaffected layout fragments.
//
// Design:
//   - DirtyReason flags track why a layout object is dirty
//   - DirtyReasons is a bitmask for efficient combination and checking
//   - FindReflowRoot walks up the tree to find the smallest valid root
//   - Intrinsic size cache avoids redundant measurement
//   - CollectDirtyRects gathers all dirty regions for repainting
//
// This is additive infrastructure. The existing LayoutEngine continues
// to work. The IncrementalLayoutEngine provides the foundation for
// efficient partial reflow.

package renderer

import "sync"

// ReflowReason represents why a layout object needs reflow.
type ReflowReason uint8

const (
	// ReflowNone means the layout is clean.
	ReflowNone ReflowReason = 0
	// ReflowGeometry means position or size changed.
	ReflowGeometry ReflowReason = 1 << iota
	// ReflowIntrinsic means intrinsic size changed.
	ReflowIntrinsic
	// ReflowText means text content changed.
	ReflowText
	// ReflowChildren means children changed.
	ReflowChildren
	// ReflowViewport means viewport changed.
	ReflowViewport
	// ReflowFont means font changed.
	ReflowFont
	// ReflowStyle means style changed.
	ReflowStyle
)

// ReflowReasons is a bitmask of reflow reasons.
type ReflowReasons uint8

// Has reports whether the reasons include the given reason.
func (r ReflowReasons) Has(reason ReflowReason) bool {
	return r&ReflowReasons(reason) != 0
}

// Clear removes the given reason from the bitmask.
func (r ReflowReasons) Clear(reason ReflowReason) ReflowReasons {
	return r & ^ReflowReasons(reason)
}

// ReflowTracker tracks dirty layout and performs minimal reflow.
type ReflowTracker struct {
	mu sync.RWMutex

	// Dirty flags per layout object
	dirty map[LayoutID]ReflowReasons

	// Parent relationships for finding reflow roots
	parents map[LayoutID]LayoutID

	// Intrinsic size cache
	intrinsicCache *IntrinsicSizeCache
}

// NewReflowTracker creates a new reflow tracker.
func NewReflowTracker() *ReflowTracker {
	return &ReflowTracker{
		dirty:          make(map[LayoutID]ReflowReasons),
		parents:        make(map[LayoutID]LayoutID),
		intrinsicCache: NewIntrinsicSizeCache(4096),
	}
}

// MarkDirty marks a layout object as dirty with the given reason.
func (e *ReflowTracker) MarkDirty(id LayoutID, reason ReflowReason) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !id.Valid() {
		return
	}

	e.dirty[id] |= ReflowReasons(reason)
}

// IsDirty reports whether a layout object is dirty.
func (e *ReflowTracker) IsDirty(id LayoutID) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !id.Valid() {
		return false
	}

	reasons, ok := e.dirty[id]
	return ok && reasons != ReflowReasons(ReflowNone)
}

// DirtyReasons returns the dirty reasons for a layout object.
func (e *ReflowTracker) DirtyReasons(id LayoutID) ReflowReasons {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !id.Valid() {
		return ReflowReasons(ReflowNone)
	}

	return e.dirty[id]
}

// ClearDirty clears the dirty flags for a layout object.
func (e *ReflowTracker) ClearDirty(id LayoutID) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !id.Valid() {
		return
	}

	delete(e.dirty, id)
}

// ClearAllDirty clears all dirty flags.
func (e *ReflowTracker) ClearAllDirty() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.dirty = make(map[LayoutID]ReflowReasons)
}

// SetParent sets the parent of a layout object for reflow root finding.
func (e *ReflowTracker) SetParent(id, parent LayoutID) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !id.Valid() {
		return
	}

	if parent.Valid() {
		e.parents[id] = parent
	} else {
		delete(e.parents, id)
	}
}

// FindReflowRoot finds the smallest valid reflow root for a dirty layout object.
// It walks up the tree to find the highest dirty ancestor with ReflowChildren,
// or returns the highest dirty ancestor.
func (e *ReflowTracker) FindReflowRoot(id LayoutID) LayoutID {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !id.Valid() {
		return LayoutNone
	}

	// Walk up to find the highest dirty ancestor
	current := id
	reflowRoot := id // Default to the node itself

	// Check if the node itself is dirty
	if _, ok := e.dirty[id]; ok {
		reflowRoot = id
	}

	// Walk up the tree
	for {
		parent, hasParent := e.parents[current]
		if !hasParent || !parent.Valid() {
			break
		}

		reasons, ok := e.dirty[parent]
		if !ok {
			// Parent not dirty, stop walking
			break
		}

		// Parent is dirty, it becomes the reflow root
		reflowRoot = parent

		// If parent is dirty with ReflowChildren, we must reflow from here
		if reasons.Has(ReflowChildren) {
			return parent
		}

		current = parent
	}

	return reflowRoot
}

// CollectDirtyRects collects all dirty layout IDs.
// In a real implementation, this would return Rects for repainting.
func (e *ReflowTracker) CollectDirtyRects() []LayoutID {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]LayoutID, 0, len(e.dirty))
	for id, reasons := range e.dirty {
		if reasons != ReflowReasons(ReflowNone) {
			result = append(result, id)
		}
	}
	return result
}

// SetIntrinsicSize caches the intrinsic size for a layout object.
func (e *ReflowTracker) SetIntrinsicSize(id LayoutID, width, height float32) {
	if !id.Valid() {
		return
	}
	e.intrinsicCache.Put(id, width, height)
}

// IntrinsicSize returns the cached intrinsic size for a layout object.
// Returns (0, 0) if not cached.
func (e *ReflowTracker) IntrinsicSize(id LayoutID) (float32, float32) {
	if !id.Valid() {
		return 0, 0
	}
	w, h, _ := e.intrinsicCache.Get(id)
	return w, h
}

// InvalidateIntrinsicSize removes the cached intrinsic size for a layout object.
func (e *ReflowTracker) InvalidateIntrinsicSize(id LayoutID) {
	if !id.Valid() {
		return
	}
	e.intrinsicCache.Invalidate(id)
}
