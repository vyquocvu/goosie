package js

// TreeWalker and NodeIterator implement the DOM Traversal API.
//
// document.createTreeWalker(root, whatToShow, filter) returns a TreeWalker
// that navigates the DOM tree relative to a mutable currentNode using
// directional methods (parentNode, firstChild, lastChild, previousSibling,
// nextSibling) and linear methods (nextNode, previousNode) that walk in
// document order.
//
// document.createNodeIterator(root, whatToShow, filter) returns a
// NodeIterator whose currentNode advances through the subtree in pre-order
// depth-first document order via nextNode and previousNode.
//
// Both accept a whatToShow bitmask (NodeFilter.SHOW_* constants) and an
// optional NodeFilter callback. TreeWalker distinguishes FILTER_SKIP from
// FILTER_REJECT so skipped nodes expose their descendants while rejected
// subtrees are pruned. NodeIterator treats both results as omitted nodes.

import (
	"errors"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// ---------------------------------------------------------------------------
// Node-type to whatToShow-bit translation
// ---------------------------------------------------------------------------

// whatToShowBit returns the NodeFilter.SHOW_* bit for n's type.
func whatToShowBit(n *dom.Node) uint32 {
	if n == nil {
		return 0
	}
	switch n.Type {
	case dom.NodeElement:
		return 0x1
	case dom.NodeText:
		return 0x4
	case dom.NodeCDATASection:
		return 0x8
	case dom.NodeComment:
		return 0x80
	case dom.NodeProcessingInstruction:
		return 0x40
	case dom.NodeDocument:
		return 0x100
	case dom.NodeDoctype:
		return 0x200
	case dom.NodeDocumentFragment:
		return 0x400
	}
	return 0
}

// ---------------------------------------------------------------------------
// Filter evaluation
// ---------------------------------------------------------------------------

const (
	filterAccept uint16 = 1
	filterReject uint16 = 2
	filterSkip   uint16 = 3
)

func (r *Runtime) traversalFilterResult(n *dom.Node, whatToShow uint32, filter goja.Value) uint16 {
	if n == nil {
		return filterReject
	}
	if whatToShow&whatToShowBit(n) == 0 {
		return filterSkip
	}
	if filter == nil || goja.IsUndefined(filter) || goja.IsNull(filter) {
		return filterAccept
	}
	if child, source, ok := r.childRealmTraversalValue(filter); ok {
		return r.traversalFilterResultInChild(child, n, source)
	}
	callback := filter
	if _, ok := goja.AssertFunction(callback); !ok {
		if object, ok := filter.(*goja.Object); ok {
			callback = object.Get("acceptNode")
		} else {
			panic(r.vm.NewTypeError("NodeFilter.acceptNode is not callable"))
		}
	}
	fn, ok := goja.AssertFunction(callback)
	if !ok {
		panic(r.vm.NewTypeError("NodeFilter.acceptNode is not callable"))
	}
	wrapped := r.wrapNode(n)
	result, err := fn(filter, wrapped)
	if err != nil {
		var exception *goja.Exception
		if errors.As(err, &exception) {
			panic(exception.Value())
		}
		panic(r.vm.NewGoError(err))
	}
	return uint16(result.ToInteger())
}

func (r *Runtime) childRealmTraversalValue(value goja.Value) (*Runtime, goja.Value, bool) {
	object, ok := value.(*goja.Object)
	if !ok {
		return nil, nil, false
	}
	if _, callable := goja.AssertFunction(value); callable {
		child := r.iframeImportedFunctionRealms[object]
		if child == nil {
			return nil, nil, false
		}
		if source := child.iframeImportedParentValues[object]; source != nil {
			return child, source, true
		}
		return nil, nil, false
	}
	for _, child := range r.iframeChildRuntimes {
		if source := child.iframeImportedParentValues[object]; source != nil {
			return child, source, true
		}
	}
	return nil, nil, false
}

func (r *Runtime) traversalFilterResultInChild(child *Runtime, node *dom.Node, filter goja.Value) (result uint16) {
	defer func() {
		if recovered := recover(); recovered != nil {
			switch exception := recovered.(type) {
			case *goja.Exception:
				panic(r.importChildValue(child, exception.Value(), make(map[*goja.Object]*goja.Object), 0))
			case goja.Value:
				panic(r.importChildValue(child, exception, make(map[*goja.Object]*goja.Object), 0))
			default:
				panic(recovered)
			}
		}
	}()
	callback := filter
	if _, callable := goja.AssertFunction(callback); !callable {
		if object, ok := filter.(*goja.Object); ok {
			callback = object.Get("acceptNode")
		} else {
			panic(child.vm.NewTypeError("NodeFilter.acceptNode is not callable"))
		}
	}
	fn, callable := goja.AssertFunction(callback)
	if !callable {
		panic(child.vm.NewTypeError("NodeFilter.acceptNode is not callable"))
	}
	wrapped := child.importParentValue(r, r.wrapNode(node))
	value, err := fn(filter, wrapped)
	if err != nil {
		var exception *goja.Exception
		if errors.As(err, &exception) {
			panic(exception)
		}
		panic(child.vm.NewGoError(err))
	}
	return uint16(value.ToInteger())
}

// ---------------------------------------------------------------------------
// TreeWalker
// ---------------------------------------------------------------------------

// treeWalker holds the Go-side state of one TreeWalker. The JS-facing object
// carries __tw__ as an own property; methods and accessors read it from there.
type treeWalker struct {
	root       *dom.Node
	current    *dom.Node
	whatToShow uint32
	filter     goja.Value
	filtering  bool
}

// installTreeWalker creates the JS TreeWalker object and returns it.
func (r *Runtime) installTreeWalker(root *dom.Node, whatToShow uint32, filter goja.Value) *goja.Object {
	tw := &treeWalker{root: root, current: root, whatToShow: whatToShow, filter: filter}
	obj := r.vm.NewObject()
	_ = obj.Set("__tw__", tw)
	_ = obj.Set("toString", func(goja.FunctionCall) goja.Value { return r.vm.ToValue("[object TreeWalker]") })
	readonly := func(name string, getter func() goja.Value) {
		_ = obj.DefineAccessorProperty(name, r.vm.ToValue(func(goja.FunctionCall) goja.Value { return getter() }), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	readonly("root", func() goja.Value { return r.wrapNode(tw.root) })
	readonly("whatToShow", func() goja.Value { return r.vm.ToValue(tw.whatToShow) })
	readonly("filter", func() goja.Value {
		if tw.filter == nil {
			return goja.Null()
		}
		return tw.filter
	})

	// currentNode: get/set accessor that reads/writes tw.current through
	// the wrapper cache so identity is preserved across repeated reads.
	_ = obj.Set("__getCurrent__", func(goja.FunctionCall) goja.Value {
		t := obj.Get("__tw__").Export().(*treeWalker)
		return r.wrapNode(t.current)
	})
	_ = obj.Set("__setCurrent__", func(call goja.FunctionCall) goja.Value {
		t := obj.Get("__tw__").Export().(*treeWalker)
		if len(call.Arguments) > 0 {
			node := r.getNode(call.Arguments[0])
			if node == nil {
				panic(r.vm.NewTypeError("TreeWalker.currentNode must be a Node"))
			}
			t.current = node
		}
		return goja.Undefined()
	})
	_ = obj.DefineAccessorProperty("currentNode",
		obj.Get("__getCurrent__"), obj.Get("__setCurrent__"),
		goja.FLAG_TRUE, goja.FLAG_TRUE)

	_ = obj.Set("parentNode", func(call goja.FunctionCall) goja.Value {
		return r.twParent(call.This.ToObject(r.vm).Get("__tw__").Export().(*treeWalker))
	})
	_ = obj.Set("firstChild", func(call goja.FunctionCall) goja.Value {
		return r.twChild(call.This.ToObject(r.vm).Get("__tw__").Export().(*treeWalker), true)
	})
	_ = obj.Set("lastChild", func(call goja.FunctionCall) goja.Value {
		return r.twChild(call.This.ToObject(r.vm).Get("__tw__").Export().(*treeWalker), false)
	})
	_ = obj.Set("previousSibling", func(call goja.FunctionCall) goja.Value {
		return r.twSibling(call.This.ToObject(r.vm).Get("__tw__").Export().(*treeWalker), false)
	})
	_ = obj.Set("nextSibling", func(call goja.FunctionCall) goja.Value {
		return r.twSibling(call.This.ToObject(r.vm).Get("__tw__").Export().(*treeWalker), true)
	})

	// Linear traversal: nextNode/previousNode walk in document order,
	// stepping over nodes that fail the filter without stopping.
	_ = obj.Set("nextNode", func(call goja.FunctionCall) goja.Value {
		t := call.This.ToObject(r.vm).Get("__tw__").Export().(*treeWalker)
		return r.twNext(t)
	})
	_ = obj.Set("previousNode", func(call goja.FunctionCall) goja.Value {
		t := call.This.ToObject(r.vm).Get("__tw__").Export().(*treeWalker)
		return r.twPrev(t)
	})

	return obj
}

func (r *Runtime) twParent(t *treeWalker) goja.Value {
	r.assertTreeWalkerInactive(t)
	if t.current == nil || t.current == t.root {
		return goja.Null()
	}
	for node := t.current.Parent; node != nil; node = node.Parent {
		if node == t.root {
			return goja.Null()
		}
		switch r.treeWalkerFilterResult(t, node) {
		case filterAccept:
			t.current = node
			return r.wrapNode(node)
		}
	}
	return goja.Null()
}

func (r *Runtime) treeWalkerFilterResult(t *treeWalker, node *dom.Node) uint16 {
	if t.filter != nil && !goja.IsNull(t.filter) && !goja.IsUndefined(t.filter) {
		if t.filtering {
			panic(r.domError("InvalidStateError", "The TreeWalker filter is already being evaluated."))
		}
		t.filtering = true
		defer func() { t.filtering = false }()
	}
	return r.traversalFilterResult(node, t.whatToShow, t.filter)
}

func (r *Runtime) assertTreeWalkerInactive(t *treeWalker) {
	if t.filtering {
		panic(r.domError("InvalidStateError", "The TreeWalker filter is already being evaluated."))
	}
}

func (r *Runtime) twChild(t *treeWalker, forward bool) goja.Value {
	r.assertTreeWalkerInactive(t)
	if t.current == nil {
		return goja.Null()
	}
	node := t.current.FirstChild
	if !forward {
		node = t.current.LastChild
	}
	if found := r.twSearchChild(t, node, forward); found != nil {
		t.current = found
		return r.wrapNode(found)
	}
	return goja.Null()
}

func (r *Runtime) twSearchChild(t *treeWalker, node *dom.Node, forward bool) *dom.Node {
	for node != nil {
		result := r.treeWalkerFilterResult(t, node)
		if result == filterAccept {
			return node
		}
		if result == filterSkip {
			child := node.FirstChild
			if !forward {
				child = node.LastChild
			}
			if child != nil {
				if found := r.twSearchChild(t, child, forward); found != nil {
					return found
				}
			}
		}
		node = traversalSibling(node, forward)
	}
	return nil
}

func traversalSibling(node *dom.Node, forward bool) *dom.Node {
	if forward {
		return node.NextSibling
	}
	return node.PrevSibling
}

func (r *Runtime) twSibling(t *treeWalker, forward bool) goja.Value {
	r.assertTreeWalkerInactive(t)
	if t.current == nil || t.current == t.root {
		return goja.Null()
	}
	for current := t.current; current != nil && current != t.root; {
		sibling := current.NextSibling
		if !forward {
			sibling = current.PrevSibling
		}
		for sibling != nil {
			result := r.treeWalkerFilterResult(t, sibling)
			if result == filterAccept {
				t.current = sibling
				return r.wrapNode(sibling)
			}
			if result != filterReject {
				child := sibling.FirstChild
				if !forward {
					child = sibling.LastChild
				}
				if child != nil {
					if found := r.twSearchSibling(t, child, forward); found != nil {
						t.current = found
						return r.wrapNode(found)
					}
				}
			}
			sibling = traversalSibling(sibling, forward)
		}
		current = current.Parent
		if current == nil || current == t.root {
			return goja.Null()
		}
		if r.treeWalkerFilterResult(t, current) == filterAccept {
			return goja.Null()
		}
	}
	return goja.Null()
}

// twSearchSibling searches the descendants of a sibling candidate using the
// TreeWalker sibling traversal rules. Unlike firstChild/lastChild traversal,
// a filter result other than FILTER_REJECT descends into the candidate.
func (r *Runtime) twSearchSibling(t *treeWalker, node *dom.Node, forward bool) *dom.Node {
	for node != nil {
		result := r.treeWalkerFilterResult(t, node)
		if result == filterAccept {
			return node
		}
		if result != filterReject {
			child := node.FirstChild
			if !forward {
				child = node.LastChild
			}
			if child != nil {
				if found := r.twSearchSibling(t, child, forward); found != nil {
					return found
				}
			}
		}
		node = traversalSibling(node, forward)
	}
	return nil
}

// twNext returns the next node in pre-order depth-first order after current
// that passes the filter, advancing current. The search stays within root.
func (r *Runtime) twNext(t *treeWalker) goja.Value {
	r.assertTreeWalkerInactive(t)
	n := t.current
	if n == nil {
		return goja.Null()
	}
	next := r.twNextInTree(n, t.root)
	for next != nil {
		result := r.treeWalkerFilterResult(t, next)
		if result == filterAccept {
			t.current = next
			return r.wrapNode(next)
		}
		next = r.twNextInTree(next, t.root, result == filterSkip)
	}
	return goja.Null()
}

// twNextInTree returns the next node in pre-order depth-first order after n,
// bounded by root. Returns nil when n is the last reachable node.
func (r *Runtime) twNextInTree(n, root *dom.Node, descend ...bool) *dom.Node {
	if (len(descend) == 0 || descend[0]) && n.FirstChild != nil {
		return n.FirstChild
	}
	for n != nil && n != root {
		if n.NextSibling != nil {
			return n.NextSibling
		}
		n = n.Parent
	}
	return nil
}

// twPrev returns the previous node in document order before current that
// passes the filter, moving current backwards.
func (r *Runtime) twPrev(t *treeWalker) goja.Value {
	r.assertTreeWalkerInactive(t)
	n := t.current
	if n == nil || n == t.root {
		return goja.Null()
	}
	prev := r.twPrevInTree(n, t.root)
	for prev != nil {
		pruned := false
		for ancestor := prev.Parent; ancestor != nil && ancestor != t.root; ancestor = ancestor.Parent {
			if r.treeWalkerFilterResult(t, ancestor) == filterReject {
				prev = r.twPreviousBeforeSubtree(ancestor, t.root)
				pruned = true
				break
			}
		}
		if pruned {
			continue
		}
		result := r.treeWalkerFilterResult(t, prev)
		if result == filterAccept {
			t.current = prev
			return r.wrapNode(prev)
		}
		if result != filterSkip {
			prev = r.twPreviousBeforeSubtree(prev, t.root)
		} else {
			prev = r.twPrevInTree(prev, t.root)
		}
	}
	return goja.Null()
}

func (r *Runtime) twPreviousBeforeSubtree(n, root *dom.Node) *dom.Node {
	for n != nil && n != root {
		if n.PrevSibling != nil {
			previous := n.PrevSibling
			for previous.LastChild != nil {
				previous = previous.LastChild
			}
			return previous
		}
		if n.Parent == root {
			return nil
		}
		return n.Parent
	}
	return nil
}

// twPrevInTree returns the previous node in pre-order depth-first order
// before n, bounded by root.
func (r *Runtime) twPrevInTree(n, root *dom.Node) *dom.Node {
	if n.PrevSibling != nil {
		c := n.PrevSibling
		for c.LastChild != nil {
			c = c.LastChild
		}
		return c
	}
	if n.Parent != nil && n.Parent != root {
		return n.Parent
	}
	return nil
}

// ---------------------------------------------------------------------------
// NodeIterator
// ---------------------------------------------------------------------------

// nodeIterator holds the Go-side state of one NodeIterator.
type nodeIterator struct {
	root             *dom.Node
	current          *dom.Node
	whatToShow       uint32
	filter           goja.Value
	before           bool
	filtering        bool
	filterRetargeted bool
	reportedCurrent  *dom.Node
	reportedBefore   bool
}

func (r *Runtime) observeNodeRemoval(doc *dom.Document) {
	if doc == nil {
		return
	}
	if r.nodeRemovalObservers == nil {
		r.nodeRemovalObservers = make(map[*dom.Document]func())
	}
	if r.nodeRemovalObservers[doc] == nil {
		r.nodeRemovalObservers[doc] = doc.RegisterBeforeNodeRemovalCallback(func(_, child *dom.Node) {
			r.retargetNodeIteratorsBeforeRemoval(child)
		})
	}
}

// installNodeIterator creates the JS NodeIterator object and returns it.
func (r *Runtime) installNodeIterator(root *dom.Node, whatToShow uint32, filter goja.Value) *goja.Object {
	r.observeNodeRemoval(root.Doc)
	ni := &nodeIterator{root: root, current: root, whatToShow: whatToShow, filter: filter, before: true}
	r.nodeIterators = append(r.nodeIterators, ni)
	obj := r.vm.NewObject()
	_ = obj.Set("__ni__", ni)
	_ = obj.Set("toString", func(goja.FunctionCall) goja.Value { return r.vm.ToValue("[object NodeIterator]") })
	readonly := func(name string, getter func() goja.Value) {
		_ = obj.DefineAccessorProperty(name, r.vm.ToValue(func(goja.FunctionCall) goja.Value { return getter() }), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	readonly("root", func() goja.Value { return r.wrapNode(ni.root) })
	readonly("whatToShow", func() goja.Value { return r.vm.ToValue(ni.whatToShow) })
	readonly("filter", func() goja.Value {
		if ni.filter == nil {
			return goja.Null()
		}
		return ni.filter
	})
	readonly("referenceNode", func() goja.Value {
		if ni.filtering {
			return r.wrapNode(ni.reportedCurrent)
		}
		return r.wrapNode(ni.current)
	})
	readonly("pointerBeforeReferenceNode", func() goja.Value {
		if ni.filtering {
			return r.vm.ToValue(ni.reportedBefore)
		}
		return r.vm.ToValue(ni.before)
	})

	_ = obj.Set("nextNode", func(call goja.FunctionCall) goja.Value {
		it := call.This.ToObject(r.vm).Get("__ni__").Export().(*nodeIterator)
		return r.niNext(it)
	})
	_ = obj.Set("previousNode", func(call goja.FunctionCall) goja.Value {
		it := call.This.ToObject(r.vm).Get("__ni__").Export().(*nodeIterator)
		return r.niPrev(it)
	})
	// detach() is a deprecated no-op kept for web compatibility.
	_ = obj.Set("detach", func(goja.FunctionCall) goja.Value { return goja.Undefined() })

	return obj
}

// niNext advances current to the next node in pre-order depth-first order
// that passes the filter, staying within the root subtree.
func (r *Runtime) niNext(it *nodeIterator) goja.Value {
	r.assertNodeIteratorFilterContext(it)
	n := it.current
	before := it.before
	if n == nil {
		return goja.Null()
	}
	for {
		next := n
		if !before {
			next = it.nextInOrder(n)
		}
		if next == nil {
			return goja.Null()
		}
		before = false
		result, retargeted := r.evaluateNodeIteratorCandidate(it, next, before)
		if result == filterAccept {
			if !retargeted {
				it.current = next
				it.before = before
			}
			return r.wrapNode(next)
		}
		if retargeted {
			n, before = it.current, it.before
			continue
		}
		n = next
	}
}

// niPrev moves current to the previous node in document order that passes
// the filter.
func (r *Runtime) niPrev(it *nodeIterator) goja.Value {
	r.assertNodeIteratorFilterContext(it)
	n := it.current
	before := it.before
	if n == nil {
		return goja.Null()
	}
	for {
		prev := n
		if before {
			prev = it.prevInOrder(n)
		}
		if prev == nil {
			return goja.Null()
		}
		before = true
		result, retargeted := r.evaluateNodeIteratorCandidate(it, prev, before)
		if result == filterAccept {
			if !retargeted {
				it.current = prev
				it.before = before
			}
			return r.wrapNode(prev)
		}
		if retargeted {
			n, before = it.current, it.before
			continue
		}
		n = prev
	}
}

func (r *Runtime) assertNodeIteratorFilterContext(it *nodeIterator) {
	if it == nil {
		return
	}
	filter, ok := it.filter.(*goja.Object)
	if !ok {
		return
	}
	child := r.iframeImportedFunctionRealms[filter]
	if child != nil && child.iframeParentFrame != nil && !child.iframeParentFrame.IsConnected() {
		panic(r.vm.NewTypeError("The NodeIterator filter's browsing context is no longer active"))
	}
}

func (r *Runtime) evaluateNodeIteratorCandidate(it *nodeIterator, node *dom.Node, candidateBefore bool) (result uint16, retargeted bool) {
	oldCurrent, oldBefore := it.current, it.before
	it.reportedCurrent, it.reportedBefore = oldCurrent, oldBefore
	it.current, it.before = node, candidateBefore
	it.filterRetargeted = false
	if it.filter != nil && !goja.IsNull(it.filter) && !goja.IsUndefined(it.filter) {
		if it.filtering {
			panic(r.domError("InvalidStateError", "The NodeIterator filter is already being evaluated."))
		}
		it.filtering = true
		defer func() {
			it.filtering = false
			retargeted = it.filterRetargeted
			if !retargeted {
				it.current, it.before = oldCurrent, oldBefore
			}
		}()
		result = r.traversalFilterResult(node, it.whatToShow, it.filter)
		return result, false
	}
	result = r.traversalFilterResult(node, it.whatToShow, it.filter)
	it.current, it.before = oldCurrent, oldBefore
	return result, false
}

func (r *Runtime) retargetNodeIteratorsBeforeRemoval(removed *dom.Node) {
	if removed == nil {
		return
	}
	for _, it := range r.nodeIterators {
		if it == nil || it.root == nil || it.current == nil || removed.Contains(it.root) || !removed.Contains(it.current) {
			continue
		}
		if it.before {
			if next := it.nextAfterSubtree(removed); next != nil {
				it.current = next
				it.before = true
			} else {
				it.current = it.prevInOrder(removed)
				it.before = false
			}
		} else {
			it.current = it.prevInOrder(removed)
			it.before = false
		}
		if it.filtering {
			it.filterRetargeted = true
		}
	}
}

func (it *nodeIterator) nextAfterSubtree(node *dom.Node) *dom.Node {
	for current := node; current != nil && current != it.root; current = current.Parent {
		if current.NextSibling != nil {
			return current.NextSibling
		}
	}
	return nil
}

// nextInOrder returns the next node in pre-order depth-first order after n,
// bounded by the iterator's root. Returns nil when n is the last reachable
// node in the subtree.
func (it *nodeIterator) nextInOrder(n *dom.Node) *dom.Node {
	if n.FirstChild != nil {
		return n.FirstChild
	}
	for n != nil && n != it.root {
		if n.NextSibling != nil {
			return n.NextSibling
		}
		n = n.Parent
	}
	return nil
}

// prevInOrder returns the previous node in pre-order depth-first order
// before n, bounded by the iterator's root.
func (it *nodeIterator) prevInOrder(n *dom.Node) *dom.Node {
	if n == it.root {
		return nil
	}
	if n.PrevSibling != nil {
		c := n.PrevSibling
		for c.LastChild != nil {
			c = c.LastChild
		}
		return c
	}
	if n.Parent != nil {
		return n.Parent
	}
	return nil
}

// ---------------------------------------------------------------------------
// Document methods and NodeFilter constants
// ---------------------------------------------------------------------------

// installTreeWalkerAPI adds document.createTreeWalker,
// document.createNodeIterator, and the global NodeFilter constants.
func (r *Runtime) installTreeWalkerAPI(doc *goja.Object) {
	// document.createTreeWalker(root, whatToShow, filter)
	_ = doc.Set("createTreeWalker", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("createTreeWalker requires a root Node"))
		}
		if r.domDoc == nil {
			return goja.Null()
		}
		root := r.getNode(call.Arguments[0])
		if root == nil {
			panic(r.vm.NewTypeError("createTreeWalker root must be a Node"))
		}
		whatToShow := uint32(0xFFFFFFFF) // SHOW_ALL
		if len(call.Arguments) >= 2 && !goja.IsUndefined(call.Arguments[1]) {
			whatToShow = uint32(call.Arguments[1].ToInteger())
		}
		var filter goja.Value
		if len(call.Arguments) >= 3 && !goja.IsUndefined(call.Arguments[2]) && !goja.IsNull(call.Arguments[2]) {
			filter = call.Arguments[2]
		}
		return r.installTreeWalker(root, whatToShow, filter)
	})

	// document.createNodeIterator(root, whatToShow, filter)
	_ = doc.Set("createNodeIterator", func(call goja.FunctionCall) goja.Value {
		if r.domDoc == nil || len(call.Arguments) < 1 {
			return goja.Null()
		}
		root := r.getNode(call.Arguments[0])
		if root == nil {
			return goja.Null()
		}
		whatToShow := uint32(0xFFFFFFFF)
		if len(call.Arguments) >= 2 && !goja.IsUndefined(call.Arguments[1]) {
			whatToShow = uint32(call.Arguments[1].ToInteger())
		}
		var filter goja.Value
		if len(call.Arguments) >= 3 && !goja.IsUndefined(call.Arguments[2]) && !goja.IsNull(call.Arguments[2]) {
			filter = call.Arguments[2]
		}
		return r.installNodeIterator(root, whatToShow, filter)
	})

	// NodeFilter constants: available as bare globals (NodeFilter.SHOW_ALL)
	// and on the window object (window.NodeFilter.SHOW_ALL) since window
	// IS the global object in this runtime.
	nf := r.vm.NewObject()
	constants := map[string]int64{
		"SHOW_ALL":                    0xFFFFFFFF,
		"SHOW_ELEMENT":                0x1,
		"SHOW_ATTRIBUTE":              0x2,
		"SHOW_TEXT":                   0x4,
		"SHOW_CDATA_SECTION":          0x8,
		"SHOW_ENTITY_REFERENCE":       0x10,
		"SHOW_ENTITY":                 0x20,
		"SHOW_PROCESSING_INSTRUCTION": 0x40,
		"SHOW_COMMENT":                0x80,
		"SHOW_DOCUMENT":               0x100,
		"SHOW_DOCUMENT_TYPE":          0x200,
		"SHOW_DOCUMENT_FRAGMENT":      0x400,
		"SHOW_NOTATION":               0x800,
		"FILTER_ACCEPT":               1,
		"FILTER_REJECT":               2,
		"FILTER_SKIP":                 3,
	}
	for name, val := range constants {
		_ = nf.Set(name, val)
		_ = r.vm.Set(name, val)
	}
	_ = r.vm.Set("NodeFilter", nf)
}
