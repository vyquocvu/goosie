// Package renderer provides a compact layout store for the Goosie engine.
//
// M4.1: Separate layout objects from DOM nodes
//
// The LayoutStore replaces pointer-heavy *LayoutBox trees with index-based
// storage using stable LayoutID handles. Layout objects are created only
// for rendered nodes — display:none elements receive no layout allocation.
//
// Design:
//   - LayoutID (uint32) is an index into a contiguous []LayoutObject slice
//   - LayoutNone (0) is the invalid/nil layout handle
//   - Contiguous storage with first-child/next-sibling links
//   - Bidirectional DOM-to-layout and layout-to-DOM mappings
//   - Generated content can create layout objects without DOM nodes
//   - display:none elements map to LayoutNone (no allocation)
//
// This is additive infrastructure. The existing LayoutBox/LayoutEngine
// continues to work. The LayoutStore provides the foundation for M4.2
// (fragment storage) and M4.4 (incremental layout).

package renderer

import "sync"

// LayoutID is a stable handle to a layout object in the store.
// It is an index into the objects slice. Zero is the invalid/nil layout.
type LayoutID uint32

const (
	// LayoutNone is the invalid/nil layout ID.
	LayoutNone LayoutID = 0
)

// Valid reports whether this is a non-zero layout ID.
func (id LayoutID) Valid() bool {
	return id != LayoutNone
}

// LayoutObject is a compact layout node stored in the LayoutStore.
// It uses ID references instead of pointers for cache-friendly storage.
type LayoutObject struct {
	ID          LayoutID // This object's ID
	Parent      LayoutID // Parent layout object (LayoutNone for root)
	FirstChild  LayoutID // First child in the layout tree
	LastChild   LayoutID // Last child in the layout tree
	PrevSibling LayoutID // Previous sibling
	NextSibling LayoutID // Next sibling

	// Box dimensions and position
	Box Rect

	// Display type: "block", "inline", "flex", "grid", "none"
	Display string

	// DOM node ID this layout object corresponds to (0 if generated)
	DOMNodeID int64

	// IsGenerated is true for layout objects created by ::before/::after
	// or other generated content that has no corresponding DOM node.
	IsGenerated bool

	// Flags for dirty tracking (used by M4.4 incremental layout)
	DirtyFlags uint8
}

// LayoutStore is a compact store for layout objects using contiguous slices.
// It is safe for single-owner use (one goroutine mutates at a time).
type LayoutStore struct {
	mu      sync.RWMutex
	objects []LayoutObject
	freeIDs []LayoutID
	count   int

	// Bidirectional DOM-to-layout mappings
	domToLayout map[int64]LayoutID // DOM node ID -> LayoutID
	layoutToDOM map[LayoutID]int64 // LayoutID -> DOM node ID
}

// NewLayoutStore creates a new layout store with the given initial capacity.
// A zero capacity uses a default of 256.
func NewLayoutStore(capacity int) *LayoutStore {
	if capacity <= 0 {
		capacity = 256
	}
	return &LayoutStore{
		objects:     make([]LayoutObject, 1, capacity+1), // Index 0 reserved for LayoutNone
		freeIDs:     make([]LayoutID, 0, 64),
		domToLayout: make(map[int64]LayoutID),
		layoutToDOM: make(map[LayoutID]int64),
	}
}

// Allocate creates a new layout object and returns its LayoutID.
func (s *LayoutStore) Allocate() (LayoutID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var id LayoutID
	if len(s.freeIDs) > 0 {
		id = s.freeIDs[len(s.freeIDs)-1]
		s.freeIDs = s.freeIDs[:len(s.freeIDs)-1]
		s.objects[id] = LayoutObject{ID: id}
	} else {
		id = LayoutID(len(s.objects))
		s.objects = append(s.objects, LayoutObject{ID: id})
	}

	s.count++
	return id, nil
}

// Get returns a read-only view of the layout object for id.
// Returns nil if id is invalid.
func (s *LayoutStore) Get(id LayoutID) *LayoutObject {
	if !id.Valid() || int(id) >= len(s.objects) {
		return nil
	}
	return &s.objects[id]
}

// ObjectCount returns the number of active layout objects.
func (s *LayoutStore) ObjectCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.count
}

// --- Property setters ---

// SetDisplay sets the display type for a layout object.
func (s *LayoutStore) SetDisplay(id LayoutID, display string) {
	if !id.Valid() || int(id) >= len(s.objects) {
		return
	}
	s.objects[id].Display = display
}

// SetBox sets the box dimensions for a layout object.
func (s *LayoutStore) SetBox(id LayoutID, box Rect) {
	if !id.Valid() || int(id) >= len(s.objects) {
		return
	}
	s.objects[id].Box = box
}

// SetGenerated marks a layout object as generated content.
func (s *LayoutStore) SetGenerated(id LayoutID, generated bool) {
	if !id.Valid() || int(id) >= len(s.objects) {
		return
	}
	s.objects[id].IsGenerated = generated
}

// --- DOM-to-Layout mapping ---

// SetDOMToLayout maps a DOM node ID to a layout object.
// Use LayoutNone to indicate no layout (e.g., display:none).
func (s *LayoutStore) SetDOMToLayout(domID int64, layoutID LayoutID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if layoutID.Valid() {
		s.domToLayout[domID] = layoutID
		s.layoutToDOM[layoutID] = domID
		s.objects[layoutID].DOMNodeID = domID
	} else {
		// Remove mapping
		if old, ok := s.domToLayout[domID]; ok {
			delete(s.layoutToDOM, old)
			delete(s.domToLayout, domID)
		}
	}
}

// DOMToLayout returns the layout ID for a DOM node, or LayoutNone if unmapped.
func (s *LayoutStore) DOMToLayout(domID int64) LayoutID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.domToLayout[domID]
	if !ok {
		return LayoutNone
	}
	return id
}

// LayoutToDOM returns the DOM node ID for a layout object, or 0 if generated.
func (s *LayoutStore) LayoutToDOM(layoutID LayoutID) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	domID, ok := s.layoutToDOM[layoutID]
	if !ok {
		return 0
	}
	return domID
}

// HasLayout reports whether a DOM node has a corresponding layout object.
func (s *LayoutStore) HasLayout(domID int64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.domToLayout[domID]
	return ok && id.Valid()
}

// --- Tree operations ---

// AppendChild adds a child layout object as the last child of parent.
func (s *LayoutStore) AppendChild(parent, child LayoutID) error {
	if !parent.Valid() || int(parent) >= len(s.objects) {
		return ErrInvalidLayoutID
	}
	if !child.Valid() || int(child) >= len(s.objects) {
		return ErrInvalidLayoutID
	}

	pObj := &s.objects[parent]
	cObj := &s.objects[child]

	// Detach from current parent if any
	if cObj.Parent.Valid() {
		s.removeChildInternal(cObj.Parent, child)
	}

	cObj.Parent = parent
	cObj.PrevSibling = pObj.LastChild
	cObj.NextSibling = LayoutNone

	if pObj.LastChild.Valid() {
		s.objects[pObj.LastChild].NextSibling = child
	} else {
		pObj.FirstChild = child
	}
	pObj.LastChild = child

	return nil
}

// RemoveChild removes a child layout object from its parent.
func (s *LayoutStore) RemoveChild(parent, child LayoutID) {
	if !parent.Valid() || int(parent) >= len(s.objects) {
		return
	}
	if !child.Valid() || int(child) >= len(s.objects) {
		return
	}
	s.removeChildInternal(parent, child)
}

// removeChildInternal removes child from parent's child list.
func (s *LayoutStore) removeChildInternal(parent, child LayoutID) {
	cObj := &s.objects[child]
	pObj := &s.objects[parent]

	// Update sibling links
	if cObj.PrevSibling.Valid() {
		s.objects[cObj.PrevSibling].NextSibling = cObj.NextSibling
	} else {
		pObj.FirstChild = cObj.NextSibling
	}
	if cObj.NextSibling.Valid() {
		s.objects[cObj.NextSibling].PrevSibling = cObj.PrevSibling
	} else {
		pObj.LastChild = cObj.PrevSibling
	}

	cObj.Parent = LayoutNone
	cObj.PrevSibling = LayoutNone
	cObj.NextSibling = LayoutNone
}

// ChildCount returns the number of direct children.
func (s *LayoutStore) ChildCount(id LayoutID) int {
	if !id.Valid() || int(id) >= len(s.objects) {
		return 0
	}
	count := 0
	for child := s.objects[id].FirstChild; child.Valid(); child = s.objects[child].NextSibling {
		count++
	}
	return count
}

// FirstChild returns the first child of id, or LayoutNone.
func (s *LayoutStore) FirstChild(id LayoutID) LayoutID {
	if !id.Valid() || int(id) >= len(s.objects) {
		return LayoutNone
	}
	return s.objects[id].FirstChild
}

// NextSibling returns the next sibling of id, or LayoutNone.
func (s *LayoutStore) NextSibling(id LayoutID) LayoutID {
	if !id.Valid() || int(id) >= len(s.objects) {
		return LayoutNone
	}
	return s.objects[id].NextSibling
}

// Parent returns the parent of id, or LayoutNone.
func (s *LayoutStore) Parent(id LayoutID) LayoutID {
	if !id.Valid() || int(id) >= len(s.objects) {
		return LayoutNone
	}
	return s.objects[id].Parent
}

// --- Reset ---

// Reset clears all layout objects and mappings.
func (s *LayoutStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.objects = s.objects[:1] // Keep index 0 reserved
	s.freeIDs = s.freeIDs[:0]
	s.count = 0
	s.domToLayout = make(map[int64]LayoutID)
	s.layoutToDOM = make(map[LayoutID]int64)
}

// --- Errors ---

// ErrInvalidLayoutID is returned for invalid layout operations.
var ErrInvalidLayoutID = errInvalidLayoutID{}

type errInvalidLayoutID struct{}

func (errInvalidLayoutID) Error() string { return "invalid LayoutID" }
