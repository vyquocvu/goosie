package dom

// ChildIterator provides zero-allocation iteration over a node's children.
// Usage:
//
//	for it := store.Children(parent); it.Next(); {
//	    child := it.ID()
//	    // ...
//	}
type ChildIterator struct {
	store  *Store
	parent NodeID
	cur    NodeID
}

// Children returns an iterator over the direct children of parent.
func (s *Store) Children(parent NodeID) ChildIterator {
	return ChildIterator{
		store:  s,
		parent: parent,
		cur:    NodeNone,
	}
}

// Next advances the iterator to the next child. Returns false when exhausted.
func (it *ChildIterator) Next() bool {
	if it.cur == NodeNone {
		it.cur = it.store.FirstChild(it.parent)
	} else {
		it.cur = it.store.NextSibling(it.cur)
	}
	return it.cur != NodeNone
}

// ID returns the current child's NodeID.
func (it *ChildIterator) ID() NodeID {
	return it.cur
}

// SubtreeIterator provides zero-allocation pre-order depth-first traversal
// of a subtree.
// Usage:
//
//	for it := store.Subtree(root); it.Next(); {
//	    node := it.ID()
//	    // ...
//	}
type SubtreeIterator struct {
	store *Store
	root  NodeID
	cur   NodeID
	depth int
}

// Subtree returns an iterator that walks the subtree rooted at root in
// pre-order depth-first order. The root itself is the first node returned.
func (s *Store) Subtree(root NodeID) SubtreeIterator {
	return SubtreeIterator{
		store: s,
		root:  root,
		cur:   NodeNone,
	}
}

// Next advances the iterator to the next node in the subtree.
// Returns false when the entire subtree has been visited.
func (it *SubtreeIterator) Next() bool {
	if it.cur == NodeNone {
		// First call: start at root.
		it.cur = it.root
		return it.cur != NodeNone
	}

	// Try first child.
	child := it.store.FirstChild(it.cur)
	if child != NodeNone {
		it.depth++
		it.cur = child
		return true
	}

	// Try next sibling, walking up as needed.
	cur := it.cur
	for cur != it.root {
		sib := it.store.NextSibling(cur)
		if sib != NodeNone {
			it.cur = sib
			return true
		}
		cur = it.store.Parent(cur)
		it.depth--
	}

	// Exhausted.
	it.cur = NodeNone
	return false
}

// ID returns the current node's NodeID.
func (it *SubtreeIterator) ID() NodeID {
	return it.cur
}

// Depth returns the current depth relative to the subtree root (0 = root).
func (it *SubtreeIterator) Depth() int {
	return it.depth
}

// ReverseChildIterator provides zero-allocation iteration over a node's
// children in reverse order (last child first).
type ReverseChildIterator struct {
	store  *Store
	parent NodeID
	cur    NodeID
}

// ReverseChildren returns an iterator over the children of parent in
// reverse order.
func (s *Store) ReverseChildren(parent NodeID) ReverseChildIterator {
	return ReverseChildIterator{
		store:  s,
		parent: parent,
		cur:    NodeNone,
	}
}

// Next advances the iterator to the previous child. Returns false when exhausted.
func (it *ReverseChildIterator) Next() bool {
	if it.cur == NodeNone {
		it.cur = it.store.LastChild(it.parent)
	} else {
		it.cur = it.store.PrevSibling(it.cur)
	}
	return it.cur != NodeNone
}

// ID returns the current child's NodeID.
func (it *ReverseChildIterator) ID() NodeID {
	return it.cur
}

// AncestorIterator provides zero-allocation iteration from a node up to
// the document root.
type AncestorIterator struct {
	store   *Store
	initial NodeID
	cur     NodeID
	started bool
}

// Ancestors returns an iterator that walks from id up to the root.
// The first call to Next returns id itself.
func (s *Store) Ancestors(id NodeID) AncestorIterator {
	return AncestorIterator{
		store:   s,
		initial: id,
		cur:     id,
	}
}

// Next advances the iterator to the next ancestor. Returns false when exhausted.
func (it *AncestorIterator) Next() bool {
	if !it.started {
		it.started = true
		return it.initial != NodeNone
	}
	parent := it.store.Parent(it.cur)
	if parent == NodeNone {
		return false
	}
	it.cur = parent
	return true
}

// ID returns the current ancestor's NodeID.
func (it *AncestorIterator) ID() NodeID {
	return it.cur
}

// SiblingIterator provides zero-allocation iteration over a node's siblings
// (including the node itself on first call).
type SiblingIterator struct {
	store   *Store
	anchor  NodeID
	cur     NodeID
	started bool
}

// Siblings returns an iterator over all siblings of id, starting with id itself.
func (s *Store) Siblings(id NodeID) SiblingIterator {
	return SiblingIterator{
		store:  s,
		anchor: id,
		cur:    id,
	}
}

// Next advances the iterator to the next sibling. Returns false when exhausted.
func (it *SiblingIterator) Next() bool {
	if !it.started {
		it.started = true
		// Find the first sibling.
		cur := it.anchor
		for {
			prev := it.store.PrevSibling(cur)
			if prev == NodeNone {
				break
			}
			cur = prev
		}
		it.cur = cur
		return it.cur != NodeNone
	}
	it.cur = it.store.NextSibling(it.cur)
	return it.cur != NodeNone
}

// ID returns the current sibling's NodeID.
func (it *SiblingIterator) ID() NodeID {
	return it.cur
}
