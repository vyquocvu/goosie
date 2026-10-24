package engine

import (
	"fmt"
	"sync"

	"github.com/vyquocvu/goosie/internal/dom"
)

// docNodeBudget is the engine's js.MutationBudget: it gates every script-driven
// DOM insertion against this document's admission caps (MaxDocumentNodes,
// MaxDocumentDepth, MaxAttributes, MaxAttributeBytes) BEFORE the mutation lands,
// so a hostile page cannot grow the tree past the caps and have the overrun
// noticed only later by validateArena/walkDocument — after the allocation.
//
// It keeps an incremental live-node counter rather than re-walking the document
// per insertion, so the appendChild-in-a-loop hot path stays O(inserted subtree).
// Reserve charges the size of each inserted subtree that is not already live (a
// move within the document costs nothing); Release credits a subtree the script
// just detached, but only for nodes the caller confirms were live, so the
// counter tracks the document instead of drifting.
//
// The counter is guarded by mu because the fetch/XHR goroutines share the
// process with the VM thread; insertions themselves all run on the single VM
// thread, but the budget does not assume that.
type docNodeBudget struct {
	mu   sync.Mutex
	root *dom.Node
	live int
}

// newDocNodeBudget seeds a budget for doc, charging the nodes the parser already
// produced so the live counter matches walkDocument's baseline (which counts the
// document node itself). The parsed tree is within caps by construction
// (ParseBounded enforced them), so measureSubtree cannot error here; the size is
// taken regardless.
func newDocNodeBudget(doc *dom.Document) *docNodeBudget {
	b := &docNodeBudget{root: &doc.Node}
	size, _, _ := measureSubtree(b.root)
	b.live = size
	return b
}

// Reserve reports whether children may be inserted under parent without
// breaching the document caps, charging the budget on success and returning a
// non-nil error otherwise. It is the fallible half of js.admitInsert: on error
// the caller throws and the document is left untouched.
//
// A detached parent (createElement'd, not yet grafted into the document) does
// not change the live set — the children become reachable from parent, not from
// the document root — so the insertion is neither charged nor depth-checked; the
// eventual attach of parent charges the whole subtree and checks depth then.
func (b *docNodeBudget) Reserve(parent *dom.Node, children []*dom.Node) error {
	if b == nil || parent == nil {
		return nil
	}
	pd, parentLive := b.depthOf(parent)
	if !parentLive {
		return nil
	}
	add := 0
	for _, c := range children {
		if c == nil {
			continue
		}
		size, height, err := measureSubtree(c)
		if err != nil {
			return err
		}
		// The inserted child sits at pd+1; its deepest descendant at pd+1+height.
		if pd+1+height > MaxDocumentDepth {
			return fmt.Errorf("insertion would exceed the document depth limit (%d)", MaxDocumentDepth)
		}
		// A child already live is a move: its subtree is counted, so charge only
		// genuinely new nodes. Non-liveness is inherited downward, so a non-live
		// child's whole subtree is new and charged in full.
		if !b.isLive(c) {
			add += size
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.live+add > MaxDocumentNodes {
		return fmt.Errorf("insertion would exceed the document node limit (%d)", MaxDocumentNodes)
	}
	b.live += add
	return nil
}

// Release credits subtrees the caller has just detached from the document. The
// caller guarantees each child was live before detaching (it consulted IsLive),
// so the subtree that left is exactly the subtree that was charged; crediting it
// keeps a remove-then-append loop from monotonically exhausting the budget. The
// counter is floored at zero so a stray credit can never drive it negative.
func (b *docNodeBudget) Release(children []*dom.Node) {
	if b == nil {
		return
	}
	credit := 0
	for _, c := range children {
		if c == nil {
			continue
		}
		size, _, err := measureSubtree(c)
		if err != nil {
			// A malformed subtree: do not credit it, keeping the charge in place
			// rather than risking an under-count on a tree we cannot measure.
			return
		}
		credit += size
	}
	if credit == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.live -= credit
	if b.live < 0 {
		b.live = 0
	}
}

// IsLive reports whether n is reachable from the document root. Removal paths
// call it before detaching so Release credits only nodes that were charged.
func (b *docNodeBudget) IsLive(n *dom.Node) bool {
	_, live := b.depthOf(n)
	return live
}

// liveCount returns the current live-node tally under the lock.
func (b *docNodeBudget) liveCount() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.live
}

// isLive is the unexported convenience used inside Reserve.
func (b *docNodeBudget) isLive(n *dom.Node) bool {
	_, live := b.depthOf(n)
	return live
}

// depthOf returns n's depth below the document root (the root itself is 0) and
// whether n is live. The Parent walk is bounded by MaxDocumentDepth+2 steps: a
// live node can be no deeper than MaxDocumentDepth, so any walk that runs longer
// is a detached chain or a script-forged Parent cycle (appendChild can create one
// between two detached nodes, which do no cycle check). Neither is reachable from
// the root, so terminating as "not live" is both safe and correct.
func (b *docNodeBudget) depthOf(n *dom.Node) (int, bool) {
	if b == nil || n == nil {
		return 0, false
	}
	depth := 0
	for cur := n; cur != nil; cur = cur.Parent {
		if cur == b.root {
			return depth, true
		}
		depth++
		if depth > MaxDocumentDepth+2 {
			return 0, false
		}
	}
	return 0, false
}

// measureSubtree walks the subtree rooted at n iteratively and returns its node
// count (including n) and its height (edges from n to its deepest descendant).
// It mirrors walkDocument's defenses — cycle detection, the node and depth
// guards, the attribute count and byte caps, and the sibling back-link check —
// so a malformed or hostile subtree is rejected here, before it is grafted into
// the document, rather than after. The walk is O(subtree), never O(document),
// which is what keeps the insertion hot path proportional to what the script
// actually inserts.
func measureSubtree(n *dom.Node) (size int, height int, err error) {
	if n == nil {
		return 0, 0, nil
	}
	type entry struct {
		node  *dom.Node
		depth int
	}
	stack := []entry{{n, 0}}
	seen := make(map[*dom.Node]bool)
	for len(stack) > 0 {
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[e.node] {
			return size, height, fmt.Errorf("document node cycle")
		}
		seen[e.node] = true
		size++
		if size > MaxDocumentNodes {
			return size, height, fmt.Errorf("HTML node limit exceeded (%d)", MaxDocumentNodes)
		}
		if e.depth > MaxDocumentDepth {
			return size, height, fmt.Errorf("HTML tree depth limit exceeded (%d)", MaxDocumentDepth)
		}
		if e.depth > height {
			height = e.depth
		}
		if len(e.node.Attr) > MaxAttributes {
			return size, height, fmt.Errorf("HTML attribute count limit exceeded (%d)", MaxAttributes)
		}
		for _, a := range e.node.Attr {
			if len(a.Name)+len(a.Value) > MaxAttributeBytes {
				return size, height, fmt.Errorf("HTML attribute byte limit exceeded (%d)", MaxAttributeBytes)
			}
		}
		siblings := 0
		for c := e.node.FirstChild; c != nil; c = c.NextSibling {
			siblings++
			if siblings > MaxDocumentNodes || c.Parent != e.node {
				return size, height, fmt.Errorf("invalid document node links")
			}
		}
		for c := e.node.LastChild; c != nil; c = c.PrevSibling {
			stack = append(stack, entry{c, e.depth + 1})
		}
	}
	return size, height, nil
}
