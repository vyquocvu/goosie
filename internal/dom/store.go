// Package dom provides a compact DOM store for the Goosie engine.
//
// M2.3: Compact DOM Store
//
// The compact store replaces pointer-heavy *html.Node trees with index-based
// storage using stable NodeID handles. Nodes are stored in contiguous slices
// to reduce GC pressure and improve cache locality.
//
// Design:
//   - NodeID (uint32) is an index into the nodes slice
//   - Contiguous []nodeRecord slice using first-child/next-sibling links
//   - Packed []Attr slice indexed by AttrStart/AttrCount per node
//   - Separate textData []byte store for text node content
//   - Zero-allocation traversal iterators
//   - Stale handle detection via Kind field (Kind == 0 means freed)
//
// This is foundation infrastructure for M2.4 (streaming tree construction)
// and M2.5 (compatibility adapter).
package dom

import (
	"errors"
	"sync"

	"github.com/vyquocvu/goosie/internal/dom/atom"
)

// NodeID is a stable handle to a node in the compact store.
// It is an index into the nodes slice. Zero is the invalid/nil node.
// Stale handles are detected by checking the Kind field (Kind == 0 means freed).
type NodeID uint32

const (
	// NodeNone is the invalid/nil node ID.
	NodeNone NodeID = 0
)

// NodeKind represents the type of DOM node.
type NodeKind uint8

const (
	// NodeKindElement is an element node (div, p, a, etc.).
	NodeKindElement NodeKind = iota + 1
	// NodeKindText is a text node.
	NodeKindText
	// NodeKindComment is a comment node.
	NodeKindComment
	// NodeKindDocument is the document root node.
	NodeKindDocument
	// NodeKindDoctype is a doctype declaration node.
	NodeKindDoctype
)

// NodeFlags are bit flags for node state.
type NodeFlags uint8

const (
	// NodeFlagNone is the default state.
	NodeFlagNone NodeFlags = 0
	// NodeFlagDirty indicates the node needs style/layout recalculation.
	NodeFlagDirty NodeFlags = 1 << iota
	// NodeFlagHasRareData indicates the node has rare metadata (namespace, etc.).
	NodeFlagHasRareData
)

// Attr is a packed attribute stored in the global attribute slice.
// Each attribute is 8 bytes (2 × uint32 atoms).
type Attr struct {
	Name  atom.Atom
	Value atom.Atom
}

// nodeRecord is the internal node storage. 32 bytes per node on 64-bit.
type nodeRecord struct {
	Parent      NodeID
	FirstChild  NodeID
	LastChild   NodeID
	PrevSibling NodeID
	NextSibling NodeID
	Name        atom.Atom // Tag name for elements, empty for text/comment
	AttrStart   uint32    // Index into Store.attrs
	AttrCount   uint16    // Number of attributes
	Kind        NodeKind  // Zero means freed/invalid
	Flags       NodeFlags
	TextStart   uint32 // Index into Store.textData (for text/comment nodes)
	TextLen     uint32 // Length of text content
}

// rareData holds optional node metadata (namespace, etc.).
type rareData struct {
	Namespace atom.Atom
}

// Store is a compact DOM store that holds nodes in contiguous slices.
// It is safe for single-owner use (one goroutine mutates at a time).
type Store struct {
	mu        sync.RWMutex
	nodes     []nodeRecord
	attrs     []Attr
	textData  []byte
	rare      map[NodeID]rareData
	freeIDs   []NodeID
	nodeCount int
	attrCount int
	textBytes int
}

// NewStore creates a new compact DOM store with the given initial capacity.
// A zero capacity uses a default of 256 nodes.
func NewStore(capacity int) *Store {
	if capacity <= 0 {
		capacity = 256
	}
	return &Store{
		nodes:    make([]nodeRecord, 1, capacity+1), // Index 0 is reserved for NodeNone
		attrs:    make([]Attr, 0, 1024),
		textData: make([]byte, 0, 4096),
		rare:     make(map[NodeID]rareData),
		freeIDs:  make([]NodeID, 0, 64),
	}
}

// ErrInvalidNodeID is returned when an operation references an invalid or stale node.
var ErrInvalidNodeID = errors.New("invalid or stale NodeID")

// ErrInvalidParent is returned when a parent node is invalid or not an element/document.
var ErrInvalidParent = errors.New("invalid parent node")

// ErrNodeNotFound is returned when a node is not found in the store.
var ErrNodeNotFound = errors.New("node not found")

// Allocate creates a new node and returns its NodeID.
// The node is uninitialized; callers must set Kind, Name, etc.
func (s *Store) Allocate() (NodeID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var id NodeID
	if len(s.freeIDs) > 0 {
		// Reuse a freed ID.
		id = s.freeIDs[len(s.freeIDs)-1]
		s.freeIDs = s.freeIDs[:len(s.freeIDs)-1]
		rec := &s.nodes[id]
		rec.Flags = NodeFlagNone
	} else {
		// Allocate a new ID.
		id = NodeID(len(s.nodes))
		if id == NodeNone {
			id = 1 // Skip 0
		}
		s.nodes = append(s.nodes, nodeRecord{})
	}

	s.nodeCount++
	return id, nil
}

// Get returns a read-only view of the node record for id.
// Returns ErrInvalidNodeID if id is invalid or stale.
func (s *Store) Get(id NodeID) (*nodeRecord, error) {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return nil, ErrInvalidNodeID
	}
	rec := &s.nodes[id]
	if rec.Kind == 0 {
		return nil, ErrInvalidNodeID
	}
	return rec, nil
}

// MustGet returns the node record for id, panicking if invalid.
func (s *Store) MustGet(id NodeID) *nodeRecord {
	rec, err := s.Get(id)
	if err != nil {
		panic(err)
	}
	return rec
}

// IsValid reports whether id is a valid, non-stale node handle.
func (s *Store) IsValid(id NodeID) bool {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return false
	}
	rec := &s.nodes[id]
	return rec.Kind != 0
}

// Parent returns the parent of id, or NodeNone if id is the root or invalid.
func (s *Store) Parent(id NodeID) NodeID {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return NodeNone
	}
	return s.nodes[id].Parent
}

// FirstChild returns the first child of id, or NodeNone if id has no children or is invalid.
func (s *Store) FirstChild(id NodeID) NodeID {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return NodeNone
	}
	return s.nodes[id].FirstChild
}

// LastChild returns the last child of id, or NodeNone if id has no children or is invalid.
func (s *Store) LastChild(id NodeID) NodeID {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return NodeNone
	}
	return s.nodes[id].LastChild
}

// NextSibling returns the next sibling of id, or NodeNone if id is the last child or invalid.
func (s *Store) NextSibling(id NodeID) NodeID {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return NodeNone
	}
	return s.nodes[id].NextSibling
}

// PrevSibling returns the previous sibling of id, or NodeNone if id is the first child or invalid.
func (s *Store) PrevSibling(id NodeID) NodeID {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return NodeNone
	}
	return s.nodes[id].PrevSibling
}

// Kind returns the node kind (element, text, comment, etc.).
func (s *Store) Kind(id NodeID) NodeKind {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return 0
	}
	return s.nodes[id].Kind
}

// Name returns the tag name atom for element nodes.
func (s *Store) Name(id NodeID) atom.Atom {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return atom.AtomNone
	}
	return s.nodes[id].Name
}

// Attrs returns the attributes for element node id.
// The returned slice is valid only until the next mutation.
func (s *Store) Attrs(id NodeID) ([]Attr, error) {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return nil, ErrInvalidNodeID
	}
	rec := &s.nodes[id]
	if rec.AttrCount == 0 {
		return nil, nil
	}
	start := rec.AttrStart
	end := start + uint32(rec.AttrCount)
	if end > uint32(len(s.attrs)) {
		return nil, ErrInvalidNodeID
	}
	return s.attrs[start:end], nil
}

// SetAttrs sets the attributes for element node id.
// The attrs slice is copied into the store's packed attribute storage.
func (s *Store) SetAttrs(id NodeID, attrs []Attr) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id == NodeNone || int(id) >= len(s.nodes) {
		return ErrInvalidNodeID
	}
	rec := &s.nodes[id]

	// Remove old attributes if any.
	if rec.AttrCount > 0 {
		s.removeAttrsLocked(rec.AttrStart, uint32(rec.AttrCount))
	}

	if len(attrs) == 0 {
		rec.AttrStart = 0
		rec.AttrCount = 0
		return nil
	}

	// Append new attributes.
	start := uint32(len(s.attrs))
	s.attrs = append(s.attrs, attrs...)
	rec.AttrStart = start
	rec.AttrCount = uint16(len(attrs))
	s.attrCount += len(attrs)
	return nil
}

// removeAttrsLocked removes attrs in [start, start+count).
// Caller must hold s.mu.
func (s *Store) removeAttrsLocked(start, count uint32) {
	end := start + count
	if end > uint32(len(s.attrs)) {
		return
	}
	// Shift remaining attrs down.
	copy(s.attrs[start:], s.attrs[end:])
	s.attrs = s.attrs[:len(s.attrs)-int(count)]

	// Update AttrStart for all nodes that reference attrs after the removed range.
	for i := range s.nodes {
		rec := &s.nodes[i]
		if rec.AttrStart > start {
			rec.AttrStart -= count
		}
	}
	s.attrCount -= int(count)
}

// Text returns the text content for text/comment nodes.
func (s *Store) Text(id NodeID) (string, error) {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return "", ErrInvalidNodeID
	}
	rec := &s.nodes[id]
	if rec.TextLen == 0 {
		return "", nil
	}
	start := rec.TextStart
	end := start + rec.TextLen
	if end > uint32(len(s.textData)) {
		return "", ErrInvalidNodeID
	}
	return string(s.textData[start:end]), nil
}

// SetText sets the text content for text/comment nodes.
func (s *Store) SetText(id NodeID, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id == NodeNone || int(id) >= len(s.nodes) {
		return ErrInvalidNodeID
	}
	rec := &s.nodes[id]

	// Remove old text if any.
	if rec.TextLen > 0 {
		s.removeTextLocked(rec.TextStart, rec.TextLen)
	}

	if text == "" {
		rec.TextStart = 0
		rec.TextLen = 0
		return nil
	}

	// Append new text.
	start := uint32(len(s.textData))
	s.textData = append(s.textData, text...)
	rec.TextStart = start
	rec.TextLen = uint32(len(text))
	s.textBytes += len(text)
	return nil
}

// removeTextLocked removes text in [start, start+length).
// Caller must hold s.mu.
func (s *Store) removeTextLocked(start, length uint32) {
	end := start + length
	if end > uint32(len(s.textData)) {
		return
	}
	// Shift remaining text down.
	copy(s.textData[start:], s.textData[end:])
	s.textData = s.textData[:len(s.textData)-int(length)]

	// Update TextStart for all nodes that reference text after the removed range.
	for i := range s.nodes {
		rec := &s.nodes[i]
		if rec.TextStart > start {
			rec.TextStart -= length
		}
	}
	s.textBytes -= int(length)
}

// SetKind sets the node kind.
func (s *Store) SetKind(id NodeID, kind NodeKind) error {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return ErrInvalidNodeID
	}
	s.nodes[id].Kind = kind
	return nil
}

// SetName sets the tag name atom for element nodes.
func (s *Store) SetName(id NodeID, name atom.Atom) error {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return ErrInvalidNodeID
	}
	s.nodes[id].Name = name
	return nil
}

// AppendChild adds a child node as the last child of parent.
// The child is removed from its current parent if any.
func (s *Store) AppendChild(parent, child NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if parent == NodeNone || int(parent) >= len(s.nodes) {
		return ErrInvalidParent
	}
	if child == NodeNone || int(child) >= len(s.nodes) {
		return ErrInvalidNodeID
	}

	pRec := &s.nodes[parent]
	cRec := &s.nodes[child]

	// Remove child from current parent if any.
	if cRec.Parent != NodeNone {
		s.removeChildLocked(cRec.Parent, child)
	}

	// Add as last child of parent.
	cRec.Parent = parent
	cRec.PrevSibling = pRec.LastChild
	cRec.NextSibling = NodeNone

	if pRec.LastChild != NodeNone {
		s.nodes[pRec.LastChild].NextSibling = child
	} else {
		pRec.FirstChild = child
	}
	pRec.LastChild = child

	return nil
}

// PrependChild adds a child node as the first child of parent.
// The child is removed from its current parent if any.
func (s *Store) PrependChild(parent, child NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if parent == NodeNone || int(parent) >= len(s.nodes) {
		return ErrInvalidParent
	}
	if child == NodeNone || int(child) >= len(s.nodes) {
		return ErrInvalidNodeID
	}

	pRec := &s.nodes[parent]
	cRec := &s.nodes[child]

	// Remove child from current parent if any.
	if cRec.Parent != NodeNone {
		s.removeChildLocked(cRec.Parent, child)
	}

	// Add as first child of parent.
	cRec.Parent = parent
	cRec.NextSibling = pRec.FirstChild
	cRec.PrevSibling = NodeNone

	if pRec.FirstChild != NodeNone {
		s.nodes[pRec.FirstChild].PrevSibling = child
	} else {
		pRec.LastChild = child
	}
	pRec.FirstChild = child

	return nil
}

// InsertBefore inserts child before the reference node ref under parent.
// If ref is NodeNone, child is appended as the last child.
func (s *Store) InsertBefore(parent, child, ref NodeID) error {
	if ref == NodeNone {
		return s.AppendChild(parent, child)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if parent == NodeNone || int(parent) >= len(s.nodes) {
		return ErrInvalidParent
	}
	if child == NodeNone || int(child) >= len(s.nodes) {
		return ErrInvalidNodeID
	}
	if ref == NodeNone || int(ref) >= len(s.nodes) {
		return ErrInvalidNodeID
	}

	cRec := &s.nodes[child]
	refRec := &s.nodes[ref]

	// Verify ref is a child of parent.
	if refRec.Parent != parent {
		return ErrNodeNotFound
	}

	// Remove child from current parent if any.
	if cRec.Parent != NodeNone {
		s.removeChildLocked(cRec.Parent, child)
	}

	// Insert before ref.
	cRec.Parent = parent
	cRec.NextSibling = ref
	cRec.PrevSibling = refRec.PrevSibling

	if refRec.PrevSibling != NodeNone {
		s.nodes[refRec.PrevSibling].NextSibling = child
	} else {
		s.nodes[parent].FirstChild = child
	}
	refRec.PrevSibling = child

	return nil
}

// RemoveChild removes child from parent's child list.
func (s *Store) RemoveChild(parent, child NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if parent == NodeNone || int(parent) >= len(s.nodes) {
		return ErrInvalidParent
	}
	if child == NodeNone || int(child) >= len(s.nodes) {
		return ErrInvalidNodeID
	}

	return s.removeChildLocked(parent, child)
}

// removeChildLocked removes child from parent's child list.
// Caller must hold s.mu.
func (s *Store) removeChildLocked(parent, child NodeID) error {
	cRec := &s.nodes[child]
	pRec := &s.nodes[parent]

	if cRec.Parent != parent {
		return ErrNodeNotFound
	}

	// Update sibling links.
	if cRec.PrevSibling != NodeNone {
		s.nodes[cRec.PrevSibling].NextSibling = cRec.NextSibling
	} else {
		pRec.FirstChild = cRec.NextSibling
	}
	if cRec.NextSibling != NodeNone {
		s.nodes[cRec.NextSibling].PrevSibling = cRec.PrevSibling
	} else {
		pRec.LastChild = cRec.PrevSibling
	}

	// Clear child's parent and sibling links.
	cRec.Parent = NodeNone
	cRec.PrevSibling = NodeNone
	cRec.NextSibling = NodeNone

	return nil
}

// Remove removes a node and all its descendants from the store.
// The node is detached from its parent, then the entire subtree is freed.
func (s *Store) Remove(id NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id == NodeNone || int(id) >= len(s.nodes) {
		return ErrInvalidNodeID
	}

	rec := &s.nodes[id]

	// Detach from parent.
	if rec.Parent != NodeNone {
		s.removeChildLocked(rec.Parent, id)
	}

	// Free the entire subtree.
	s.freeSubtreeLocked(id)

	return nil
}

// freeSubtreeLocked frees a node and all its descendants.
// Caller must hold s.mu.
func (s *Store) freeSubtreeLocked(id NodeID) {
	rec := &s.nodes[id]

	// Free children first.
	child := rec.FirstChild
	for child != NodeNone {
		next := s.nodes[child].NextSibling
		s.freeSubtreeLocked(child)
		child = next
	}

	// Free attributes.
	if rec.AttrCount > 0 {
		s.removeAttrsLocked(rec.AttrStart, uint32(rec.AttrCount))
	}

	// Free text.
	if rec.TextLen > 0 {
		s.removeTextLocked(rec.TextStart, rec.TextLen)
	}

	// Free rare data.
	delete(s.rare, id)

	// Mark as free.
	rec.Kind = 0
	rec.Flags = NodeFlagNone
	rec.Name = atom.AtomNone
	rec.AttrStart = 0
	rec.AttrCount = 0
	rec.TextStart = 0
	rec.TextLen = 0
	rec.Parent = NodeNone
	rec.FirstChild = NodeNone
	rec.LastChild = NodeNone
	rec.PrevSibling = NodeNone
	rec.NextSibling = NodeNone

	// Add to free list.
	s.freeIDs = append(s.freeIDs, id)
	s.nodeCount--
}

// Replace replaces old node with new node in the tree.
// The old node is removed and freed. The new node takes its place.
func (s *Store) Replace(old, new NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if old == NodeNone || int(old) >= len(s.nodes) {
		return ErrInvalidNodeID
	}
	if new == NodeNone || int(new) >= len(s.nodes) {
		return ErrInvalidNodeID
	}

	oldRec := &s.nodes[old]
	newRec := &s.nodes[new]

	parent := oldRec.Parent
	if parent == NodeNone {
		return ErrInvalidParent
	}

	// Remove new from its current parent if any.
	if newRec.Parent != NodeNone {
		s.removeChildLocked(newRec.Parent, new)
	}

	// Copy old's links to new.
	newRec.Parent = parent
	newRec.PrevSibling = oldRec.PrevSibling
	newRec.NextSibling = oldRec.NextSibling

	// Update sibling links.
	if oldRec.PrevSibling != NodeNone {
		s.nodes[oldRec.PrevSibling].NextSibling = new
	} else {
		s.nodes[parent].FirstChild = new
	}
	if oldRec.NextSibling != NodeNone {
		s.nodes[oldRec.NextSibling].PrevSibling = new
	} else {
		s.nodes[parent].LastChild = new
	}

	// Free old node.
	s.freeSubtreeLocked(old)

	return nil
}

// ChildCount returns the number of direct children of id.
func (s *Store) ChildCount(id NodeID) int {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return 0
	}
	count := 0
	for child := s.nodes[id].FirstChild; child != NodeNone; child = s.nodes[child].NextSibling {
		count++
	}
	return count
}

// NodeCount returns the total number of active nodes in the store.
func (s *Store) NodeCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nodeCount
}

// AttrCount returns the total number of attributes across all nodes.
func (s *Store) AttrCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.attrCount
}

// TextBytes returns the total bytes of text content in the store.
func (s *Store) TextBytes() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.textBytes
}

// Reset clears all nodes and resets the store to empty.
func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nodes = s.nodes[:1] // Keep index 0 reserved
	s.attrs = s.attrs[:0]
	s.textData = s.textData[:0]
	s.rare = make(map[NodeID]rareData)
	s.freeIDs = s.freeIDs[:0]
	s.nodeCount = 0
	s.attrCount = 0
	s.textBytes = 0
}

// SetRareData sets rare metadata for a node.
func (s *Store) SetRareData(id NodeID, data rareData) error {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return ErrInvalidNodeID
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.rare[id] = data
	s.nodes[id].Flags |= NodeFlagHasRareData
	return nil
}

// GetRareData returns the rare metadata for a node, or false if not present.
func (s *Store) GetRareData(id NodeID) (rareData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if id == NodeNone || int(id) >= len(s.nodes) {
		return rareData{}, false
	}
	data, ok := s.rare[id]
	return data, ok
}

// SetFlag sets a flag on a node.
func (s *Store) SetFlag(id NodeID, flag NodeFlags) error {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return ErrInvalidNodeID
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nodes[id].Flags |= flag
	return nil
}

// ClearFlag clears a flag on a node.
func (s *Store) ClearFlag(id NodeID, flag NodeFlags) error {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return ErrInvalidNodeID
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nodes[id].Flags &^= flag
	return nil
}

// HasFlag reports whether a node has a flag set.
func (s *Store) HasFlag(id NodeID, flag NodeFlags) bool {
	if id == NodeNone || int(id) >= len(s.nodes) {
		return false
	}
	return s.nodes[id].Flags&flag != 0
}
