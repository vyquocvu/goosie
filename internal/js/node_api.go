package js

import (
	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// This file is the Node-tree half of the DOM binding: the child-list mutation
// and traversal API a real content script cannot avoid.
//
// The set is not a judgement about which APIs look important. It is what the
// two pages this task measured actually call, plus what the library on those
// pages calls on the way:
//
//	gov.vn   jQuery 3.6.0 support block, at offset 37,880 of the 89,501-byte
//	           minified file and the first statement that touches the DOM at
//	           all: `ce = E.createDocumentFragment().appendChild(
//	           E.createElement("div"))`. It runs at library scope, before the
//	           `return jQuery` export, so it died before this file existed and
//	           `jQuery` was never defined at all on that page.
//	vnexpress (page copy 201,234 bytes): `window.addEventListener(...)` 6 sites,
//	           and 9 `parentNode.insertBefore(...)` sites - the
//	           `<script>`/`<link>` loader idiom. None passes a literal null, but
//	           two hand over `x.nextSibling` (offsets 113,261 and 195,744),
//	           which is null exactly when the node lands after the last child, so
//	           the append case is reachable on the page. jQuery proves it three
//	           ways: `prepend` passes `t.firstChild`, null for an empty parent
//	           (50,065); `after` passes `this.nextSibling`, null for a last child
//	           (50,297); and `wrapAll`'s path calls `t.insertBefore(this[0])`
//	           with the second argument absent (80,979). All three are the
//	           standard's append case and all are load-bearing for a reveal, so
//	           `insertBefore(x, null)` appends here too.
//
// Every name below was counted in that shipped jquery-3.6.0.min.js (89,501
// bytes, sha1 b82d238d…): createDocumentFragment 2, insertBefore 5 (4 call
// sites and the `insertBefore:"before"` alias-map key), replaceChild 1,
// cloneNode 5, childNodes 11, firstChild 12, lastChild 5, nextSibling 10,
// previousSibling 6, ownerDocument 24, parentNode 54, nodeValue 1, contains 13.
// Names a page does not reach are deliberately absent and listed in
// docs/tasks/TASK-047-DOM-JQUERY-APIS.md.

// installNodeTreeMethods adds the child-list mutation API to the shared node
// prototype, so every wrapped node - element, text, comment and document
// fragment alike - answers them. Called from setupDOM, next to the prototype's
// other methods, because goja cannot see a __proto__ chain: wrapNode copies
// these keys onto each instance.
func (r *Runtime) installNodeTreeMethods() {
	_ = r.nodeProto.Set("insertBefore", func(call goja.FunctionCall) goja.Value {
		return r.jsInsertBefore(call)
	})
	_ = r.nodeProto.Set("replaceChild", func(call goja.FunctionCall) goja.Value {
		return r.jsReplaceChild(call)
	})
	_ = r.nodeProto.Set("cloneNode", func(call goja.FunctionCall) goja.Value {
		return r.jsCloneNode(call)
	})
	_ = r.nodeProto.Set("contains", func(call goja.FunctionCall) goja.Value {
		return r.jsContains(call)
	})
	_ = r.nodeProto.Set("hasChildNodes", func(call goja.FunctionCall) goja.Value {
		this := r.getNode(call.This)
		return r.vm.ToValue(this != nil && this.FirstChild != nil)
	})
}

// insertCharge lists the nodes an insertion of newChild actually puts under its
// parent, which is what the mutation budget must be charged for. A
// DocumentFragment is the reason this is not just `[]*dom.Node{newChild}`: the
// fragment itself never lands in the tree, its children do, so charging the
// fragment would over-count by one node per jQuery `domManip` call and - worse -
// would depth-check a subtree that is not the one being inserted.
func insertCharge(newChild *dom.Node) []*dom.Node {
	if newChild == nil {
		return nil
	}
	if newChild.DocumentFragment() {
		return newChild.ChildNodes()
	}
	return []*dom.Node{newChild}
}

// jsInsertBefore implements `parent.insertBefore(newNode, referenceNode)`.
//
// Semantics pinned here, each because a measured page depends on it:
//   - referenceNode null/undefined/absent appends. vnexpress reaches it by
//     passing `a.nextSibling` (113,261, 195,744) and jQuery by passing
//     `t.firstChild`, `this.nextSibling` or nothing at all (50,065, 50,297,
//     80,979); this is the DOM Standard's step 1 and the reason the second
//     argument may even be missing.
//   - a DocumentFragment argument moves its children in order and is left
//     empty, so the same fragment can be refilled. jQuery's buildFragment
//     returns one and hands it to appendChild/insertBefore.
//   - a referenceNode that is not a child of this node throws NotFoundError,
//     like browsers, rather than silently appending, because the silent
//     version turns a page bug into content in the wrong place.
//   - newNode === referenceNode is a no-op (see dom.Node.InsertBefore: the
//     splice would otherwise create a sibling cycle).
func (r *Runtime) jsInsertBefore(call goja.FunctionCall) goja.Value {
	parent := r.getNode(call.This)
	if parent == nil {
		return goja.Undefined()
	}
	newChild := r.getNode(call.Argument(0))
	if newChild == nil {
		panic(r.vm.NewTypeError("Failed to execute 'insertBefore' on 'Node': parameter 1 is not of type 'Node'"))
	}
	var ref *dom.Node
	if len(call.Arguments) > 1 {
		ref = r.getNode(call.Arguments[1])
	}
	if ref != nil {
		r.requireChild(parent, ref, "insertBefore")
	}
	r.admitInsert("insertBefore", parent, insertCharge(newChild), func() {
		parent.InsertNode(newChild, ref)
	})
	r.notifyMutation()
	r.notifyConnected(newChild)
	return call.Arguments[0]
}

// jsReplaceChild implements `parent.replaceChild(newChild, oldChild)`.
// jQuery's `replaceWith` reaches it once, as `t.replaceChild(e, this)`.
func (r *Runtime) jsReplaceChild(call goja.FunctionCall) goja.Value {
	parent := r.getNode(call.This)
	if parent == nil {
		return goja.Undefined()
	}
	newChild := r.getNode(call.Argument(0))
	if newChild == nil {
		panic(r.vm.NewTypeError("Failed to execute 'replaceChild' on 'Node': parameter 1 is not of type 'Node'"))
	}
	oldChild := r.getNode(call.Argument(1))
	if oldChild == nil {
		panic(r.vm.NewTypeError("Failed to execute 'replaceChild' on 'Node': parameter 2 is not of type 'Node'"))
	}
	r.requireChild(parent, oldChild, "replaceChild")
	// The old subtree is credited on the same condition every other removal
	// path uses: only nodes reachable from the document were ever charged.
	oldWasLive := r.nodeIsLive(oldChild)
	r.admitInsert("replaceChild", parent, insertCharge(newChild), func() {
		parent.ReplaceNode(newChild, oldChild)
	})
	if oldWasLive {
		r.releaseNodes([]*dom.Node{oldChild})
	}
	r.notifyMutation()
	r.notifyConnected(newChild)
	return call.Arguments[1]
}

// requireChild throws the NotFoundError browsers throw when the reference node
// of an insert or replace is not a child of the node being mutated. goja turns
// the panic into a catchable exception at the script frame, so a page's own
// try/catch sees it and the rest of the script still runs.
func (r *Runtime) requireChild(parent, child *dom.Node, op string) {
	if child.Parent != parent {
		e := r.vm.NewTypeError("Failed to execute '" + op + "' on 'Node': The reference node is not a child of this node.")
		_ = e.Set("name", "NotFoundError")
		panic(e)
	}
}

// jsCloneNode implements `node.cloneNode(deep)`.
//
// `false` copies the node alone, `true` copies the whole subtree. jQuery needs
// both shapes in its own support tests, run back to back:
// `ce.cloneNode(!0).cloneNode(!0).lastChild.checked` - a deep clone of a clone,
// then a child-list read. A shallow clone there would return a node with no
// children and `lastChild` would be null, so `lastChild.checked` would throw
// inside the library's feature probe.
//
// The clone is detached, so it is deliberately not charged against the
// document's node budget: that is exactly how document.createElement already
// behaves, and the eventual append charges the whole subtree through
// measureSubtree. Deviations worth naming: internal/dom keeps form-control state
// in attributes (js.go's __getChecked__/__setValue__ read and write them), and
// CloneNode copies the attribute slice, so `checked` and a script-set `value`
// both come across - which is what makes jQuery's checkClone chain answer true.
// The cost is that there is no dirty-value flag: in a browser a script-set value
// clones as the *default* value, here it clones as the value, and `defaultValue`
// therefore has nothing to distinguish it. A cloned <script> keeps no listener
// registrations - the realm's listeners are keyed by *dom.Node, and the clone is
// a new one.
func (r *Runtime) jsCloneNode(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil {
		return goja.Undefined()
	}
	deep := len(call.Arguments) > 0 && call.Arguments[0].ToBoolean()
	clone := node.CloneNode(deep)
	if clone == nil {
		return goja.Null()
	}
	return r.wrapNode(clone)
}

// jsContains implements `node.contains(other)`: true for the node itself and
// for any descendant, false for null, foreign and detached nodes.
func (r *Runtime) jsContains(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	other := r.getNode(call.Argument(0))
	return r.vm.ToValue(node.Contains(other))
}

// jsCreateDocumentFragment implements `document.createDocumentFragment()`.
//
// The result is a real Node, so it carries the prototype methods like anything
// else: jQuery's very first statement is
// `E.createDocumentFragment().appendChild(E.createElement("div"))`, which only
// works if appendChild exists on the fragment and returns its argument (the div
// becomes the value of `ce`, the library's scratch container for the rest of
// the boot).
func (r *Runtime) jsCreateDocumentFragment(goja.FunctionCall) goja.Value {
	if r.domDoc == nil {
		return goja.Null()
	}
	return r.wrapNode(r.domDoc.NewDocumentFragment())
}

// defineNodeAccessors installs the child-list and tree-traversal reads a Node
// exposes, as live accessors on one wrapper.
//
// `childNodes` returns a fresh snapshot array rather than a live NodeList, and
// that is a decision, not an oversight: a live object needs an invalidation hook
// per mutation and a NodeList identity the spec promises, and internal/js has
// neither. Snapshot reads stay correct for the patterns the measured pages use -
// `for (var i = 0; i < el.childNodes.length; i++) el.removeChild(el.childNodes[i])`
// reads the array once per iteration, which is the loop jQuery's `cleanData`
// runs, and its `while (el.firstChild) el.removeChild(el.firstChild)` form
// re-reads every step, which is what a snapshot gives for free. Anything that
// genuinely needs a live list (MutationObserver's `previousSibling` records) is
// already served by the mutation recorder, not by this property.
func (r *Runtime) defineNodeAccessors(obj *goja.Object, node *dom.Node) {
	// childNodes: every node type, document order.
	_ = obj.DefineAccessorProperty("childNodes", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		kids := node.ChildNodes()
		list := make([]goja.Value, 0, len(kids))
		for _, c := range kids {
			list = append(list, r.wrapNode(c))
		}
		return r.vm.ToValue(list)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	_ = obj.DefineAccessorProperty("firstChild", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.wrapNode(node.FirstChild)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	_ = obj.DefineAccessorProperty("lastChild", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.wrapNode(node.LastChild)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	_ = obj.DefineAccessorProperty("nextSibling", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.wrapNode(node.NextSibling)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	_ = obj.DefineAccessorProperty("previousSibling", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.wrapNode(node.PrevSibling)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	// ownerDocument is the document object itself, not a second wrapper for
	// the same node: jQuery's Sizzle `setDocument` compares
	// `r != C` against window.document and re-points its whole internal
	// document cache at whatever it gets back, so a look-alike wrapper would
	// be adopted as the document and then fail on the first property only the
	// real one has. wrapNode(&doc.Node) returns the global document object for
	// exactly this reason, and the document's own ownerDocument is null.
	_ = obj.DefineAccessorProperty("ownerDocument", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if node.Doc == nil || node.Type == dom.NodeDocument {
			return goja.Null()
		}
		return r.wrapNode(&node.Doc.Node)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	// nodeValue: text and comment data for the leaf types, null for the types
	// that have no value (element, document, fragment), per the standard.
	_ = obj.DefineAccessorProperty("nodeValue", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		switch node.Type {
		case dom.NodeText, dom.NodeComment:
			return r.vm.ToValue(node.DataContent)
		default:
			return goja.Null()
		}
	}), r.vm.ToValue(func(setCall goja.FunctionCall) goja.Value {
		if node.Type != dom.NodeText && node.Type != dom.NodeComment {
			return goja.Undefined()
		}
		if len(setCall.Arguments) > 0 && !goja.IsNull(setCall.Arguments[0]) && !goja.IsUndefined(setCall.Arguments[0]) {
			node.DataContent = setCall.Arguments[0].String()
			if node.Doc != nil {
				node.Doc.MarkQueryDirty()
			}
			r.notifyMutation()
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_FALSE)

	_ = obj.DefineAccessorProperty("isConnected", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(node.IsConnected())
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
}

// nodeStaticName is the DOM Standard's fixed nodeName for the node types that
// have one. Elements are named by their tag, so they never reach here.
func nodeStaticName(node *dom.Node) string {
	if node == nil {
		return ""
	}
	switch node.Type {
	case dom.NodeText:
		return "#text"
	case dom.NodeComment:
		return "#comment"
	case dom.NodeDocument:
		return "#document"
	case dom.NodeDocumentFragment:
		return "#document-fragment"
	case dom.NodeDoctype:
		return "#doctype"
	}
	return ""
}

// notifyConnected runs custom-element connectedCallback for a node that was
// just grafted into the document. It walks a fragment's children because that
// is what actually landed. Appending through the parser never reaches here;
// this is only the script-driven path, matching how appendChild already
// behaved for a single element.
func (r *Runtime) notifyConnected(newChild *dom.Node) {
	if newChild == nil {
		return
	}
	if newChild.DocumentFragment() {
		for _, c := range newChild.ChildNodes() {
			r.notifyConnected(c)
		}
		return
	}
	if newChild.Element() {
		r.invokeLifecycleCallback(newChild, "connectedCallback")
	}
}
