package js

import (
	"net/url"
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// jsImportNode and jsAdoptNode implement the Document tree transfer methods.
// They are shared by the active document and detached documents created by
// DOMParser/createHTMLDocument, whose target owner document differs.
func (r *Runtime) jsImportNode(call goja.FunctionCall, target *dom.Document) goja.Value {
	if len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'importNode': 1 argument required"))
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'importNode': parameter 1 is not of type 'Node'"))
	}
	if node.Type == dom.NodeDocument {
		panic(r.domError("NotSupportedError", "The node to be imported is a document"))
	}
	if owner := r.runtimeForDocument(target); owner != nil && owner != r {
		arguments := make([]goja.Value, len(call.Arguments))
		for i, argument := range call.Arguments {
			arguments[i] = owner.importParentValue(r, argument)
		}
		// An unresolved customized built-in stores its `is` value in the
		// source runtime until a matching definition is registered. Transfer
		// that candidate before cloning into the target document's runtime.
		if candidate, exists := r.customElementCandidates[node]; exists {
			if owner.customElementCandidates == nil {
				owner.customElementCandidates = make(map[*dom.Node]string)
			}
			owner.customElementCandidates[node] = candidate
		}
		imported := owner.jsImportNode(goja.FunctionCall{Arguments: arguments}, target)
		return r.importChildValue(owner, imported, make(map[*goja.Object]*goja.Object), 0)
	}
	deep := len(call.Arguments) > 1 && call.Arguments[1].ToBoolean()
	registryFallback := r.customElementRegistryForDocument(target)
	if len(call.Arguments) > 1 {
		if options, ok := call.Arguments[1].(*goja.Object); ok && options != nil {
			deep = true
			if selfOnly := options.Get("selfOnly"); selfOnly != nil && !goja.IsUndefined(selfOnly) {
				deep = !selfOnly.ToBoolean()
			}
			if value := options.Get("customElementRegistry"); value != nil && !goja.IsUndefined(value) {
				registryObject, ok := value.(*goja.Object)
				if !ok || r.customElementRegistryObjects[registryObject] == nil {
					panic(r.vm.NewTypeError("The customElementRegistry option is not a CustomElementRegistry"))
				}
				registryFallback = r.customElementRegistryObjects[registryObject]
			}
		}
	}
	targetRegistry := r.customElementRegistryForDocument(target)
	clone := target.ImportNode(node, deep)
	r.associateImportedCustomElementRegistries(node, clone, registryFallback, target, targetRegistry)
	return r.wrapNode(clone)
}

// associateImportedCustomElementRegistries copies existing non-null registry
// associations and applies the import fallback to nodes whose source registry
// is null. Explicitly null declarative shadow roots remain registry boundaries.
func (r *Runtime) associateImportedCustomElementRegistries(source, imported *dom.Node, fallback *CustomElementRegistry, target *dom.Document, targetRegistry *CustomElementRegistry) {
	if source == nil || imported == nil {
		return
	}
	if r.customElementNodeRegistries == nil {
		r.customElementNodeRegistries = make(map[*dom.Node]*CustomElementRegistry)
	}
	registry := r.customElementRegistryForNode(source)
	if registry != nil && !registry.isScoped && registry.document != nil && registry.document != target {
		registry = targetRegistry
	}
	if registry == nil {
		registry = fallback
	}
	if imported.Element() {
		r.customElementNodeRegistries[imported] = registry
		if candidate, exists := r.customElementCandidates[source]; exists {
			if r.customElementCandidates == nil {
				r.customElementCandidates = make(map[*dom.Node]string)
			}
			r.customElementCandidates[imported] = candidate
		}
		if definition := r.customElementDefinitionForNodeInRegistry(imported, registry); definition != nil {
			r.upgradeElementInRegistry(imported, definition, false, false, registry)
		}
	}
	for sourceChild, importedChild := source.FirstChild, imported.FirstChild; sourceChild != nil && importedChild != nil; sourceChild, importedChild = sourceChild.NextSibling, importedChild.NextSibling {
		r.associateImportedCustomElementRegistries(sourceChild, importedChild, fallback, target, targetRegistry)
	}
	if source.ShadowRoot != nil && imported.ShadowRoot != nil {
		shadowRegistry := r.customElementRegistryForShadowRoot(source.ShadowRoot)
		if shadowRegistry != nil && !shadowRegistry.isScoped && shadowRegistry.document != nil && shadowRegistry.document != target {
			shadowRegistry = targetRegistry
		}
		if shadowRegistry == nil && !source.ShadowRoot.CustomElementRegistryNull {
			shadowRegistry = fallback
		}
		imported.ShadowRoot.CustomElementRegistryNull = source.ShadowRoot.CustomElementRegistryNull
		if r.customElementShadowRegistries == nil {
			r.customElementShadowRegistries = make(map[*dom.ShadowRoot]*CustomElementRegistry)
		}
		r.customElementShadowRegistries[imported.ShadowRoot] = shadowRegistry
		for sourceChild, importedChild := source.ShadowRoot.FirstChild, imported.ShadowRoot.FirstChild; sourceChild != nil && importedChild != nil; sourceChild, importedChild = sourceChild.NextSibling, importedChild.NextSibling {
			r.associateImportedCustomElementRegistries(sourceChild, importedChild, shadowRegistry, target, targetRegistry)
		}
	}
}

func (r *Runtime) jsAdoptNode(call goja.FunctionCall, target *dom.Document) goja.Value {
	if len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'adoptNode': 1 argument required"))
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'adoptNode': parameter 1 is not of type 'Node'"))
	}
	if node.Type == dom.NodeDocument {
		panic(r.domError("NotSupportedError", "The node to be adopted is a document"))
	}
	oldDocuments := captureOwnerDocuments([]*dom.Node{node})
	wasConnected := captureConnectedState([]*dom.Node{node})
	if node.Parent != nil {
		node.Parent.RemoveChild(node)
	}
	r.markRadioGroupsDirtyForNodes([]*dom.Node{node})
	r.notifyDisconnected([]*dom.Node{node}, wasConnected)
	node.AdoptInto(target)
	r.notifyAdopted([]*dom.Node{node}, oldDocuments)
	r.notifyMutation()
	return r.wrapNode(node)
}

func nodesEqual(a, b *dom.Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Type != b.Type {
		return false
	}
	switch a.Type {
	case dom.NodeElement:
		if elementNamespaceURI(a) != elementNamespaceURI(b) || a.Prefix != b.Prefix ||
			elementLocalNameForEquality(a) != elementLocalNameForEquality(b) || len(a.Attr) != len(b.Attr) {
			return false
		}
	case dom.NodeText, dom.NodeComment, dom.NodeCDATASection:
		if a.DataContent != b.DataContent || len(a.DataUTF16) != len(b.DataUTF16) {
			return false
		}
	case dom.NodeProcessingInstruction:
		if a.Data != b.Data || a.DataContent != b.DataContent || len(a.DataUTF16) != len(b.DataUTF16) {
			return false
		}
	case dom.NodeDoctype:
		if a.DoctypeName != b.DoctypeName || a.PublicID != b.PublicID || a.SystemID != b.SystemID {
			return false
		}
	}
	for i := range a.DataUTF16 {
		if a.DataUTF16[i] != b.DataUTF16[i] {
			return false
		}
	}
	for _, attr := range a.Attr {
		found := false
		for _, other := range b.Attr {
			if attr.NamespaceURI == other.NamespaceURI && attributeLocalNameForEquality(attr.Name) == attributeLocalNameForEquality(other.Name) && attr.Value == other.Value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for ac, bc := a.FirstChild, b.FirstChild; ; ac, bc = ac.NextSibling, bc.NextSibling {
		if ac == nil || bc == nil {
			return ac == nil && bc == nil
		}
		if !nodesEqual(ac, bc) {
			return false
		}
	}
}

func elementLocalNameForEquality(node *dom.Node) string {
	if node == nil {
		return ""
	}
	return attributeLocalNameForEquality(node.Data)
}

func attributeLocalNameForEquality(qualifiedName string) string {
	if colon := strings.LastIndexByte(qualifiedName, ':'); colon >= 0 {
		return qualifiedName[colon+1:]
	}
	return qualifiedName
}

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

// installNodeTreeMethods adds the child-list mutation API to Node.prototype,
// so every wrapped node - element, text, comment and document fragment alike -
// answers them. Called from setupDOM, next to the prototype's other methods;
// wrappers inherit these through [[Prototype]] (see element_interfaces.go).
func (r *Runtime) installNodeTreeMethods() {
	moveBefore := func(call goja.FunctionCall) goja.Value { return r.jsMoveBefore(call) }
	remove := func(call goja.FunctionCall) goja.Value { return r.jsRemove(call) }
	after := func(call goja.FunctionCall) goja.Value { return r.jsAfter(call) }
	before := func(call goja.FunctionCall) goja.Value { return r.jsBefore(call) }
	replaceWith := func(call goja.FunctionCall) goja.Value { return r.jsReplaceWith(call) }
	for _, prototype := range []*goja.Object{r.elementProto, r.characterDataProto, r.documentTypeProto} {
		if prototype != nil {
			_ = prototype.Set("remove", remove)
			_ = prototype.Set("before", before)
			_ = prototype.Set("after", after)
			_ = prototype.Set("replaceWith", replaceWith)
		}
	}
	for _, prototype := range []*goja.Object{r.elementProto, r.documentProto, r.documentFragmentProto} {
		if prototype != nil {
			_ = prototype.Set("moveBefore", moveBefore)
		}
	}
	insertBefore := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.jsInsertBefore(call)
	}).ToObject(r.vm)
	_ = insertBefore.DefineDataProperty("length", r.vm.ToValue(2), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.nodeProto.Set("insertBefore", insertBefore)
	replaceChild := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.jsReplaceChild(call)
	}).ToObject(r.vm)
	_ = replaceChild.DefineDataProperty("length", r.vm.ToValue(2), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.nodeProto.Set("replaceChild", replaceChild)
	_ = r.nodeProto.Set("cloneNode", func(call goja.FunctionCall) goja.Value {
		return r.jsCloneNode(call)
	})
	_ = r.nodeProto.Set("normalize", func(call goja.FunctionCall) goja.Value {
		if node := r.getNode(call.This); node != nil {
			r.normalizeNode(node)
		}
		return goja.Undefined()
	})
	_ = r.nodeProto.Set("isEqualNode", func(call goja.FunctionCall) goja.Value {
		this := r.getNode(call.This)
		if this == nil || len(call.Arguments) == 0 {
			return r.vm.ToValue(this == nil && (len(call.Arguments) == 0 || goja.IsNull(call.Arguments[0])))
		}
		return r.vm.ToValue(nodesEqual(this, r.getNode(call.Arguments[0])))
	})
	_ = r.nodeProto.Set("isSameNode", func(call goja.FunctionCall) goja.Value {
		this := r.getNode(call.This)
		return r.vm.ToValue(this != nil && len(call.Arguments) > 0 && this == r.getNode(call.Arguments[0]))
	})
	_ = r.nodeProto.Set("contains", func(call goja.FunctionCall) goja.Value {
		return r.jsContains(call)
	})
	_ = r.nodeProto.Set("hasChildNodes", func(call goja.FunctionCall) goja.Value {
		this := r.getNode(call.This)
		return r.vm.ToValue(this != nil && this.FirstChild != nil)
	})
	_ = r.nodeProto.Set("compareDocumentPosition", func(call goja.FunctionCall) goja.Value {
		this := r.getNode(call.This)
		if this == nil || len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to execute 'compareDocumentPosition': 1 argument required"))
		}
		other := r.getNode(call.Arguments[0])
		if other == nil {
			panic(r.vm.NewTypeError("Failed to execute 'compareDocumentPosition': parameter 1 is not of type 'Node'"))
		}
		return r.vm.ToValue(r.compareDocumentPosition(this, other))
	})
}

func (r *Runtime) compareDocumentPosition(node, other *dom.Node) int {
	if node == other {
		return 0
	}
	nodeRoot, otherRoot := nodeRoot(node), nodeRoot(other)
	if nodeRoot != otherRoot {
		if r.documentPositionOrder == nil {
			r.documentPositionOrder = make(map[*dom.Node]uint64)
		}
		order := func(root *dom.Node) uint64 {
			if value, ok := r.documentPositionOrder[root]; ok {
				return value
			}
			r.nextDocumentPositionOrder++
			r.documentPositionOrder[root] = r.nextDocumentPositionOrder
			return r.nextDocumentPositionOrder
		}
		position := 1 | 32
		if order(otherRoot) < order(nodeRoot) {
			position |= 2
		} else {
			position |= 4
		}
		return position
	}
	if isAncestorNode(node, other) {
		return 4 | 16
	}
	if isAncestorNode(other, node) {
		return 2 | 8
	}
	ancestor := commonAncestor(node, other)
	if ancestor == nil {
		return 1 | 32 | 4
	}
	if childIndex(ancestor, childBelowAncestor(ancestor, node)) < childIndex(ancestor, childBelowAncestor(ancestor, other)) {
		return 4
	}
	return 2
}

// jsMoveBefore implements ParentNode.moveBefore for elements. It preserves
// connectedness and queues the custom-element connectedMoveCallback for the
// moved shadow-including subtree; unlike insertBefore, it cannot adopt a node
// from another tree or connect/disconnect it.
func (r *Runtime) jsMoveBefore(call goja.FunctionCall) goja.Value {
	parent := r.getNode(call.This)
	if parent == nil || len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'moveBefore' on 'Element': a node is required"))
	}
	moved := r.getNode(call.Arguments[0])
	if moved == nil {
		panic(r.vm.NewTypeError("Failed to execute 'moveBefore' on 'Element': parameter 1 is not of type 'Node'"))
	}
	var reference *dom.Node
	if len(call.Arguments) > 1 && !goja.IsNull(call.Arguments[1]) && !goja.IsUndefined(call.Arguments[1]) {
		reference = r.getNode(call.Arguments[1])
		if reference == nil {
			panic(r.vm.NewTypeError("Failed to execute 'moveBefore' on 'Element': parameter 2 is not of type 'Node'"))
		}
	}
	parentConnected := parent.IsConnected()
	if moved.Parent == nil || nodeTreeRoot(parent) != nodeTreeRoot(moved) {
		panic(r.customElementNameException("HierarchyRequestError", "The node cannot be moved into a different tree."))
	}
	if parent.Type == dom.NodeDocument && (moved.Type == dom.NodeText || moved.Type == dom.NodeCDATASection) {
		panic(r.customElementNameException("HierarchyRequestError", "Text nodes cannot be moved into a document."))
	}
	if reference != nil && reference.Parent != parent {
		panic(r.customElementNameException("NotFoundError", "The reference node is not a child of this element."))
	}
	if moved.Type != dom.NodeElement && moved.Type != dom.NodeText && moved.Type != dom.NodeCDATASection && moved.Type != dom.NodeComment {
		panic(r.customElementNameException("HierarchyRequestError", "Only elements and character data nodes can be moved."))
	}
	for ancestor := parent; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor == moved {
			panic(r.customElementNameException("HierarchyRequestError", "The node cannot be moved into its descendant."))
		}
	}
	if reference == moved {
		reference = moved.NextSibling
	}
	if moved.Parent == parent && moved == reference {
		return goja.Undefined()
	}
	wasConnected := parentConnected
	parent.InsertBefore(moved, reference)
	r.installWindowNamedProperties([]*dom.Node{moved})
	r.markRadioGroupsDirtyForNodes([]*dom.Node{moved})
	r.notifyMutation()
	if wasConnected {
		var walk func(*dom.Node)
		walk = func(node *dom.Node) {
			if node.Type == dom.NodeElement && r.customElementDefinition(node.Data) != nil {
				r.invokeLifecycleCallback(node, "connectedMoveCallback")
			}
			if node.ShadowRoot != nil {
				for child := node.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
					walk(child)
				}
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
		walk(moved)
	}
	return goja.Undefined()
}

type nodeTreeRootKey struct {
	node       *dom.Node
	shadowHost *dom.Node
}

func nodeTreeRoot(node *dom.Node) nodeTreeRootKey {
	for current := node; current != nil; current = current.Parent {
		parent := current.Parent
		if parent != nil && parent.ShadowRoot != nil {
			for child := parent.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
				if child == current {
					return nodeTreeRootKey{shadowHost: parent}
				}
			}
		}
		if current.Parent == nil {
			return nodeTreeRootKey{node: current}
		}
	}
	return nodeTreeRootKey{}
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

// jsReplaceChildren implements ParentNode.replaceChildren(). It converts the
// variadic Node-or-DOMString sequence before mutating, validates the complete
// resulting child list, then replaces the target's children as one operation.
func (r *Runtime) jsReplaceChildren(call goja.FunctionCall) goja.Value {
	parent := r.getNode(call.This)
	if parent == nil {
		return goja.Undefined()
	}
	r.requireContainerNode(parent, "replaceChildren")
	doc := parent.Doc
	if doc == nil {
		doc = r.domDoc
	}
	if doc == nil {
		return goja.Undefined()
	}

	// convert nodes into a node: strings become Text nodes, fragments
	// contribute their children, and repeating a Node moves it to the last
	// occurrence in the resulting sequence.
	children := r.childNodeArguments(doc, call.Arguments)
	unique := make([]*dom.Node, 0, len(children))
	for _, child := range children {
		for i, existing := range unique {
			if existing == child {
				unique = append(unique[:i], unique[i+1:]...)
				break
			}
		}
		unique = append(unique, child)
	}
	children = unique
	r.validateReplaceChildren(parent, children)
	r.requireNoAncestorInsertion(parent, children)

	removed := parent.ChildNodes()
	affected := append([]*dom.Node(nil), removed...)
	affectedSet := make(map[*dom.Node]struct{}, len(affected)+len(children))
	for _, child := range affected {
		affectedSet[child] = struct{}{}
	}
	for _, child := range children {
		if _, exists := affectedSet[child]; !exists {
			affected = append(affected, child)
			affectedSet[child] = struct{}{}
		}
	}
	affectedConnected := captureConnectedState(affected)
	oldDocuments := captureOwnerDocuments(children)
	var liveRemoved []*dom.Node
	inserted := make(map[*dom.Node]struct{}, len(children))
	for _, child := range children {
		inserted[child] = struct{}{}
	}
	for _, child := range removed {
		if r.nodeIsLive(child) {
			if _, survives := inserted[child]; !survives {
				liveRemoved = append(liveRemoved, child)
			}
		}
	}

	fragment := doc.NewDocumentFragment()
	r.admitInsert("replaceChildren", parent, children, func() {
		// Move the requested nodes into a fragment first. This leaves a stable
		// snapshot of the old children and handles nodes that are already under
		// the target without duplicating them.
		for _, child := range children {
			oldParent := child.Parent
			fragment.AppendChild(child)
			// Staging a node in the fragment runs the removal steps for its
			// original parent. Those are separate mutations (and must be queued
			// before replace-all's record for the target parent).
			if oldParent != nil && oldParent != parent {
				r.queueMutationRecord(MutationRecord{
					Type:         "childList",
					Target:       oldParent,
					RemovedNodes: []*dom.Node{child},
				})
			}
		}
		for _, child := range removed {
			if child.Parent == parent {
				parent.RemoveChild(child)
			}
		}
		parent.InsertNode(fragment, nil)
	})

	if b := r.mutationBudget(); b != nil {
		b.Release(liveRemoved)
	}
	r.installWindowNamedProperties(children)
	r.markRadioGroupsDirtyForNodes(append(removed, children...))
	r.queueMutationRecord(MutationRecord{
		Type:         "childList",
		Target:       parent,
		AddedNodes:   children,
		RemovedNodes: removed,
	})
	r.notifyDisconnected(affected, affectedConnected)
	r.notifyAdopted(children, oldDocuments)
	if parent.Type == dom.NodeDocument && parent.Doc != nil {
		parent.Doc.HTML = documentElementOf(parent.Doc)
		parent.Doc.Head = documentChildOf(parent.Doc, "head")
		parent.Doc.Body = documentBodyOf(parent.Doc)
		parent.Doc.DocType = documentDoctypeOf(parent.Doc)
		if owner := r.runtimeForDocument(parent.Doc); owner != nil {
			owner.RefreshDocumentTreeReferences()
		}
	}
	r.notifyMutation()
	for _, child := range children {
		r.activateDOMInsertedSubtree(child)
	}
	return goja.Undefined()
}

// validateReplaceChildren applies the hierarchy constraints that can be
// checked from the complete replacement sequence before any tree mutation.
func (r *Runtime) validateReplaceChildren(parent *dom.Node, children []*dom.Node) {
	if parent.Type != dom.NodeDocument {
		for _, child := range children {
			if child.Type == dom.NodeDocument || child.Type == dom.NodeDoctype {
				panic(r.domError("HierarchyRequestError", "The node cannot be inserted at this position."))
			}
		}
		return
	}
	var hasElement, hasDoctype bool
	for _, child := range children {
		switch child.Type {
		case dom.NodeElement:
			if hasElement {
				panic(r.domError("HierarchyRequestError", "A document cannot have more than one document element."))
			}
			hasElement = true
		case dom.NodeDoctype:
			if hasDoctype || hasElement {
				panic(r.domError("HierarchyRequestError", "A document type must occur once before the document element."))
			}
			hasDoctype = true
		case dom.NodeComment, dom.NodeProcessingInstruction:
			// These node types may appear around the document type and element.
		case dom.NodeText, dom.NodeCDATASection:
			panic(r.domError("HierarchyRequestError", "A document cannot contain text nodes."))
		default:
			panic(r.domError("HierarchyRequestError", "The node cannot be inserted at this position."))
		}
	}
}

// validateParentNodeInsertion checks the complete child sequence ParentNode
// append/prepend would produce. Validate before moving any nodes so a rejected
// operation cannot leave a partially-mutated Document.
func (r *Runtime) validateParentNodeInsertion(parent *dom.Node, kids []*dom.Node, prepend bool) {
	if parent == nil {
		return
	}
	sequence := make([]*dom.Node, 0, len(kids)+4)
	inserted := make([]*dom.Node, 0, len(kids))
	for _, kid := range kids {
		if kid == nil {
			continue
		}
		if kid.Type == dom.NodeDocumentFragment {
			for child := kid.FirstChild; child != nil; child = child.NextSibling {
				inserted = append(inserted, child)
			}
		} else {
			inserted = append(inserted, kid)
		}
	}
	moveSet := make(map[*dom.Node]bool, len(inserted))
	for _, kid := range inserted {
		moveSet[kid] = true
	}
	for child := parent.FirstChild; child != nil; child = child.NextSibling {
		if !moveSet[child] {
			sequence = append(sequence, child)
		}
	}
	if prepend {
		sequence = append(inserted, sequence...)
	} else {
		sequence = append(sequence, inserted...)
	}
	r.validateReplaceChildren(parent, sequence)
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
	if len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'insertBefore' on 'Node': 2 arguments required"))
	}
	var ref *dom.Node
	if !goja.IsNull(call.Arguments[1]) && !goja.IsUndefined(call.Arguments[1]) {
		ref = r.getNode(call.Arguments[1])
		if ref == nil {
			panic(r.vm.NewTypeError("Failed to execute 'insertBefore' on 'Node': parameter 2 is not of type 'Node'"))
		}
	}
	r.requireContainerNode(parent, "insertBefore")
	r.requireNoAncestorInsertion(parent, insertCharge(newChild))
	if ref != nil {
		r.requireChild(parent, ref, "insertBefore")
	}
	inserted := insertCharge(newChild)
	for _, node := range inserted {
		switch node.Type {
		case dom.NodeDocumentFragment, dom.NodeDoctype, dom.NodeElement, dom.NodeText, dom.NodeCDATASection, dom.NodeProcessingInstruction, dom.NodeComment:
		default:
			panic(r.domError("HierarchyRequestError", "The node cannot be inserted at this position."))
		}
	}
	if parent.Type == dom.NodeDocument {
		moveSet := make(map[*dom.Node]struct{}, len(inserted))
		for _, node := range inserted {
			moveSet[node] = struct{}{}
		}
		effectiveReference := ref
		for effectiveReference != nil {
			if _, willMove := moveSet[effectiveReference]; !willMove {
				break
			}
			effectiveReference = effectiveReference.NextSibling
		}
		retained := make([]*dom.Node, 0, 4)
		insertIndex := 0
		for child := parent.FirstChild; child != nil; child = child.NextSibling {
			if child == effectiveReference {
				insertIndex = len(retained)
			}
			if _, willMove := moveSet[child]; willMove {
				continue
			}
			retained = append(retained, child)
		}
		if effectiveReference == nil {
			insertIndex = len(retained)
		}
		tail := append([]*dom.Node(nil), retained[insertIndex:]...)
		prospective := append(retained[:insertIndex], inserted...)
		prospective = append(prospective, tail...)
		r.validateReplaceChildren(parent, prospective)
	} else {
		for _, node := range inserted {
			if node.Type == dom.NodeDocument || node.Type == dom.NodeDoctype {
				panic(r.domError("HierarchyRequestError", "The node cannot be inserted at this position."))
			}
		}
	}
	oldDocuments := captureOwnerDocuments(inserted)
	wasConnected := captureConnectedState(inserted)
	removals := snapshotChildRemovals(inserted)
	r.admitInsert("insertBefore", parent, insertCharge(newChild), func() {
		parent.InsertNode(newChild, ref)
	})
	r.queueInsertionMutation(parent, inserted, removals)
	r.installWindowNamedProperties(inserted)
	r.markRadioGroupsDirtyForNodes(inserted)
	r.notifyMutation()
	r.notifyDisconnected(inserted, wasConnected)
	r.notifyAdopted(inserted, oldDocuments)
	for _, insertedNode := range inserted {
		r.activateDOMInsertedSubtree(insertedNode)
	}
	return call.Arguments[0]
}

// jsAfter implements ChildNode.after() by converting its arguments into a
// fragment and inserting that fragment immediately after the receiver. The
// admission and lifecycle hooks run once for the complete sequence, so a
// rejected insertion cannot leave only part of the requested nodes moved.
func (r *Runtime) jsAfter(call goja.FunctionCall) goja.Value {
	target := r.getNode(call.This)
	if target == nil || target.Parent == nil {
		return goja.Undefined()
	}
	parent := target.Parent
	doc := parent.Doc
	if doc == nil {
		doc = target.Doc
	}
	if doc == nil {
		return goja.Undefined()
	}

	var inserted []*dom.Node
	for _, value := range call.Arguments {
		if node := r.getNode(value); node != nil {
			if node.DocumentFragment() {
				inserted = append(inserted, node.ChildNodes()...)
			} else {
				inserted = append(inserted, node)
			}
			continue
		}
		inserted = append(inserted, doc.NewText(value.String()))
	}
	if len(inserted) == 0 {
		return goja.Undefined()
	}

	for _, node := range inserted {
		for ancestor := parent; ancestor != nil; ancestor = ancestor.Parent {
			if ancestor == node {
				panic(r.domError("HierarchyRequestError", "The node cannot be inserted at this position."))
			}
		}
	}

	fragment := doc.NewDocumentFragment()
	reference := target.NextSibling
	insertedSet := make(map[*dom.Node]struct{}, len(inserted))
	for _, node := range inserted {
		insertedSet[node] = struct{}{}
	}
	for reference != nil {
		if _, willMove := insertedSet[reference]; !willMove {
			break
		}
		reference = reference.NextSibling
	}
	oldDocuments := captureOwnerDocuments(inserted)
	wasConnected := captureConnectedState(inserted)
	r.admitInsert("after", parent, inserted, func() {
		for _, node := range inserted {
			fragment.AppendChild(node)
		}
		parent.InsertNode(fragment, reference)
	})
	r.installWindowNamedProperties(inserted)
	r.markRadioGroupsDirtyForNodes(inserted)
	r.notifyMutation()
	r.notifyDisconnected(inserted, wasConnected)
	r.notifyAdopted(inserted, oldDocuments)
	for _, node := range inserted {
		r.activateDOMInsertedSubtree(node)
	}
	return goja.Undefined()
}

// childNodeArguments converts the ChildNode variadic arguments to the node
// sequence used by before()/replaceWith(): Node arguments are retained,
// fragments contribute their children, and other values become text nodes.
func (r *Runtime) childNodeArguments(doc *dom.Document, args []goja.Value) []*dom.Node {
	var nodes []*dom.Node
	for _, value := range args {
		if node := r.getNode(value); node != nil {
			if node.DocumentFragment() {
				nodes = append(nodes, node.ChildNodes()...)
			} else {
				nodes = append(nodes, node)
			}
			continue
		}
		nodes = append(nodes, doc.NewText(value.String()))
	}
	return nodes
}

// jsBefore implements ChildNode.before() using one fragment insertion so node
// moves and custom-element reactions observe the complete requested sequence.
func (r *Runtime) jsBefore(call goja.FunctionCall) goja.Value {
	target := r.getNode(call.This)
	if target == nil || target.Parent == nil {
		return goja.Undefined()
	}
	parent := target.Parent
	doc := parent.Doc
	if doc == nil {
		doc = target.Doc
	}
	if doc == nil {
		return goja.Undefined()
	}
	nodes := r.childNodeArguments(doc, call.Arguments)
	inserted := nodes[:0]
	for _, node := range nodes {
		if node != target {
			inserted = append(inserted, node)
		}
	}
	if len(inserted) == 0 {
		return goja.Undefined()
	}
	fragment := doc.NewDocumentFragment()
	oldDocuments := captureOwnerDocuments(inserted)
	wasConnected := captureConnectedState(inserted)
	var previouslyLive []*dom.Node
	for _, node := range inserted {
		if r.nodeIsLive(node) {
			previouslyLive = append(previouslyLive, node)
		}
	}
	r.admitInsert("before", parent, inserted, func() {
		for _, node := range inserted {
			fragment.AppendChild(node)
		}
		// ChildNode.before() first converts its arguments into a fragment. If
		// one argument is an ancestor of the insertion point, converting the
		// sequence can disconnect that ancestor before pre-insert rejects the
		// fragment. The custom-element reaction must still run when the method
		// throws; validating before fragment construction loses that observable
		// disconnection.
		for _, node := range inserted {
			for ancestor := parent; ancestor != nil; ancestor = ancestor.Parent {
				if ancestor == node {
					r.releaseNodes(previouslyLive)
					r.markRadioGroupsDirtyForNodes(inserted)
					r.notifyMutation()
					r.notifyDisconnected(inserted, wasConnected)
					panic(r.domError("HierarchyRequestError", "The node cannot be inserted at this position."))
				}
			}
		}
		parent.InsertNode(fragment, target)
	})
	r.installWindowNamedProperties(inserted)
	r.markRadioGroupsDirtyForNodes(inserted)
	r.notifyMutation()
	r.notifyDisconnected(inserted, wasConnected)
	r.notifyAdopted(inserted, oldDocuments)
	for _, node := range inserted {
		r.activateDOMInsertedSubtree(node)
	}
	return goja.Undefined()
}

// jsReplaceWith implements ChildNode.replaceWith(). When the receiver is also
// an argument, converting multiple nodes moves it into the temporary fragment;
// in that case the fragment is inserted at the first following sibling not in
// the converted sequence, preserving the receiver at its argument position.
func (r *Runtime) jsReplaceWith(call goja.FunctionCall) goja.Value {
	target := r.getNode(call.This)
	if target == nil || target.Parent == nil {
		return goja.Undefined()
	}
	parent := target.Parent
	doc := parent.Doc
	if doc == nil {
		doc = target.Doc
	}
	if doc == nil {
		return goja.Undefined()
	}
	nodes := r.childNodeArguments(doc, call.Arguments)
	targetInNodes := false
	containsNode := func(candidate *dom.Node) bool {
		for _, node := range nodes {
			if node == candidate {
				return true
			}
		}
		return false
	}
	for _, node := range nodes {
		if node == target {
			targetInNodes = true
			break
		}
	}
	// A single-node conversion returns that node directly. Replacing a node
	// with itself is therefore a no-op; with multiple nodes, conversion creates
	// a fragment and moves the receiver along with the other arguments.
	if targetInNodes && len(nodes) == 1 {
		return goja.Undefined()
	}
	inserted := nodes
	r.requireNoAncestorInsertion(parent, inserted)
	viableNextSibling := target.NextSibling
	if targetInNodes {
		for viableNextSibling != nil && containsNode(viableNextSibling) {
			viableNextSibling = viableNextSibling.NextSibling
		}
	}
	oldWasLive := r.nodeIsLive(target)
	oldWasConnected := target.IsConnected()
	oldDocuments := captureOwnerDocuments(inserted)
	wasConnected := captureConnectedState(inserted)
	fragment := doc.NewDocumentFragment()
	r.admitInsert("replaceWith", parent, inserted, func() {
		for _, node := range inserted {
			fragment.AppendChild(node)
		}
		if targetInNodes {
			parent.InsertNode(fragment, viableNextSibling)
		} else {
			parent.ReplaceNode(fragment, target)
		}
	})
	r.installWindowNamedProperties(inserted)
	r.markRadioGroupsDirtyForNodes(append(inserted, target))
	if oldWasLive && !targetInNodes {
		r.releaseNodes([]*dom.Node{target})
	}
	r.notifyMutation()
	if oldWasConnected {
		r.invokeDisconnectedCallbacks(target)
	}
	r.notifyDisconnected(inserted, wasConnected)
	r.notifyAdopted(inserted, oldDocuments)
	for _, node := range inserted {
		r.activateDOMInsertedSubtree(node)
	}
	return goja.Undefined()
}

// jsReplaceChild implements `parent.replaceChild(newChild, oldChild)`.
// jQuery's `replaceWith` reaches it once, as `t.replaceChild(e, this)`.
func (r *Runtime) jsReplaceChild(call goja.FunctionCall) goja.Value {
	parent := r.getNode(call.This)
	if parent == nil {
		return goja.Undefined()
	}
	r.requireContainerNode(parent, "replaceChild")
	newChild := r.getNode(call.Argument(0))
	if newChild == nil {
		panic(r.vm.NewTypeError("Failed to execute 'replaceChild' on 'Node': parameter 1 is not of type 'Node'"))
	}
	oldChild := r.getNode(call.Argument(1))
	if oldChild == nil {
		panic(r.vm.NewTypeError("Failed to execute 'replaceChild' on 'Node': parameter 2 is not of type 'Node'"))
	}
	r.requireNoAncestorInsertion(parent, insertCharge(newChild))
	r.requireChild(parent, oldChild, "replaceChild")
	// The old subtree is credited on the same condition every other removal
	// path uses: only nodes reachable from the document were ever charged.
	oldWasLive := r.nodeIsLive(oldChild)
	oldWasConnected := oldChild.IsConnected()
	inserted := insertCharge(newChild)
	insertedRemovals := snapshotChildRemovals(inserted)
	if parent.Type == dom.NodeDocument {
		prospective := make([]*dom.Node, 0, len(inserted)+4)
		moveSet := make(map[*dom.Node]struct{}, len(inserted))
		for _, node := range inserted {
			moveSet[node] = struct{}{}
		}
		replacementIndex := 0
		beforeOldChild := true
		for child := parent.FirstChild; child != nil; child = child.NextSibling {
			if child == oldChild {
				beforeOldChild = false
				continue
			}
			if _, willMove := moveSet[child]; willMove {
				continue
			}
			if beforeOldChild {
				replacementIndex++
			}
			prospective = append(prospective, child)
		}
		tail := append([]*dom.Node(nil), prospective[replacementIndex:]...)
		prospective = append(prospective[:replacementIndex], inserted...)
		prospective = append(prospective, tail...)
		r.validateReplaceChildren(parent, prospective)
	} else {
		for _, node := range inserted {
			if node.Type == dom.NodeDocument || node.Type == dom.NodeDoctype {
				panic(r.domError("HierarchyRequestError", "The node cannot be inserted at this position."))
			}
		}
	}
	oldDocuments := captureOwnerDocuments(inserted)
	wasConnected := captureConnectedState(inserted)
	r.admitInsert("replaceChild", parent, insertCharge(newChild), func() {
		parent.ReplaceNode(newChild, oldChild)
	})
	for _, removal := range insertedRemovals {
		r.queueMutationRecord(MutationRecord{Type: "childList", Target: removal.parent, RemovedNodes: []*dom.Node{removal.node}, PreviousSibling: removal.previous, NextSibling: removal.next})
	}
	if newChild == oldChild {
		r.queueMutationRecord(MutationRecord{Type: "childList", Target: parent, AddedNodes: inserted})
	} else {
		r.queueMutationRecord(MutationRecord{Type: "childList", Target: parent, RemovedNodes: []*dom.Node{oldChild}, AddedNodes: inserted})
	}
	r.installWindowNamedProperties(inserted)
	r.markRadioGroupsDirtyForNodes(append(inserted, oldChild))
	if oldWasLive {
		r.releaseNodes([]*dom.Node{oldChild})
	}
	r.notifyMutation()
	if oldWasConnected {
		r.invokeDisconnectedCallbacks(oldChild)
	}
	r.notifyDisconnected(inserted, wasConnected)
	r.notifyAdopted(inserted, oldDocuments)
	for _, insertedNode := range inserted {
		r.activateDOMInsertedSubtree(insertedNode)
	}
	return call.Arguments[1]
}

// requireNoAncestorInsertion enforces the DOM pre-insert constraint that a
// node cannot be inserted into itself or one of its descendants. Without it,
// the parent/child links form a cycle and ordinary DOM walks recurse forever.
func (r *Runtime) requireNoAncestorInsertion(parent *dom.Node, inserted []*dom.Node) {
	for _, node := range inserted {
		for ancestor := parent; ancestor != nil; ancestor = ancestor.Parent {
			if ancestor == node {
				panic(r.domError("HierarchyRequestError", "The node cannot be inserted at this position."))
			}
		}
	}
}

// requireChild throws the NotFoundError browsers throw when the reference node
// of an insert or replace is not a child of the node being mutated. goja turns
// the panic into a catchable exception at the script frame, so a page's own
// try/catch sees it and the rest of the script still runs.
func (r *Runtime) requireChild(parent, child *dom.Node, op string) {
	if child.Parent != parent {
		panic(r.domError("NotFoundError", "Failed to execute '"+op+"' on 'Node': The reference node is not a child of this node."))
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
		if object, ok := call.This.(*goja.Object); ok {
			if attribute := r.attributeValues[object]; attribute != nil {
				clone := r.newJSAttribute(attribute.name, attribute.namespace, attribute.currentOwnerDocument()).(*goja.Object)
				r.attributeValues[clone].value = attribute.currentValue()
				return clone
			}
		}
		return goja.Undefined()
	}
	deep := len(call.Arguments) > 0 && call.Arguments[0].ToBoolean()
	clone := node.CloneNode(deep)
	if clone == nil {
		return goja.Null()
	}
	if node.Type == dom.NodeDocument {
		return r.wrapDetachedDocument(r.cloneDocumentWrapper(node, clone))
	}
	var lockNullRegistrySubtree func(*dom.Node)
	lockNullRegistrySubtree = func(current *dom.Node) {
		if current == nil {
			return
		}
		if current.Element() {
			if r.customElementNodeRegistries == nil {
				r.customElementNodeRegistries = make(map[*dom.Node]*CustomElementRegistry)
			}
			if r.customElementNullRegistryLocks == nil {
				r.customElementNullRegistryLocks = make(map[*dom.Node]bool)
			}
			r.customElementNodeRegistries[current] = nil
			r.customElementNullRegistryLocks[current] = true
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			lockNullRegistrySubtree(child)
		}
	}
	var copyCustomElementRegistries func(*dom.Node, *dom.Node)
	cloneDefinitions := make(map[*dom.Node]*customElementDefinition)
	copyCustomElementRegistries = func(source, target *dom.Node) {
		if source == nil || target == nil {
			return
		}
		if registry, associated := r.customElementNodeRegistries[source]; associated {
			if r.customElementNodeRegistries == nil {
				r.customElementNodeRegistries = make(map[*dom.Node]*CustomElementRegistry)
			}
			r.customElementNodeRegistries[target] = registry
		}
		if candidate, associated := r.customElementCandidates[source]; associated {
			if r.customElementCandidates == nil {
				r.customElementCandidates = make(map[*dom.Node]string)
			}
			r.customElementCandidates[target] = candidate
		}
		r.mu.Lock()
		definition := r.customElementInstances[source]
		r.mu.Unlock()
		if definition != nil {
			cloneDefinitions[target] = definition
		}
		if r.customElementNullRegistryLocks[source] {
			if r.customElementNullRegistryLocks == nil {
				r.customElementNullRegistryLocks = make(map[*dom.Node]bool)
			}
			r.customElementNullRegistryLocks[target] = true
		}
		sourceChild, targetChild := source.FirstChild, target.FirstChild
		for sourceChild != nil && targetChild != nil {
			copyCustomElementRegistries(sourceChild, targetChild)
			sourceChild, targetChild = sourceChild.NextSibling, targetChild.NextSibling
		}
		if source.ShadowRoot != nil && target.ShadowRoot != nil {
			for sourceChild, targetChild := source.ShadowRoot.FirstChild, target.ShadowRoot.FirstChild; sourceChild != nil && targetChild != nil; sourceChild, targetChild = sourceChild.NextSibling, targetChild.NextSibling {
				if source.ShadowRoot.CustomElementRegistryNull {
					lockNullRegistrySubtree(targetChild)
				} else {
					copyCustomElementRegistries(sourceChild, targetChild)
				}
			}
		}
	}
	copyCustomElementRegistries(node, clone)
	var upgradeClonedCustomElements func(*dom.Node, *CustomElementRegistry)
	upgradeClonedCustomElements = func(current *dom.Node, inherited *CustomElementRegistry) {
		if current == nil {
			return
		}
		registry := inherited
		if explicit, associated := r.customElementNodeRegistries[current]; associated {
			registry = explicit
		}
		if current.Type == dom.NodeElement {
			def := cloneDefinitions[current]
			if def == nil {
				def = r.customElementDefinitionForNodeInRegistry(current, registry)
			}
			if def != nil {
				r.upgradeElementInRegistry(current, def, false, false, registry)
			}
		}
		if current.ShadowRoot != nil {
			shadowRegistry := r.customElementRegistryForShadowRoot(current.ShadowRoot)
			for child := current.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
				upgradeClonedCustomElements(child, shadowRegistry)
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			upgradeClonedCustomElements(child, registry)
		}
	}
	upgradeClonedCustomElements(clone, r.customElementRegistryForNode(clone))
	return r.wrapNode(clone)
}

func (r *Runtime) cloneDocumentWrapper(source, cloneNode *dom.Node) *dom.Document {
	clone := dom.NewDocument()
	if source.Doc != nil {
		clone.ContentLanguage = source.Doc.ContentLanguage
		clone.CharacterSet = source.Doc.CharacterSet
		clone.ContentType = source.Doc.ContentType
		clone.QuirksMode = source.Doc.QuirksMode
		clone.LimitedQuirksMode = source.Doc.LimitedQuirksMode
		clone.DesignMode = source.Doc.DesignMode
		clone.IsXML = source.Doc.IsXML
		if documentURL := r.detachedDocumentURLs[source.Doc]; documentURL != nil {
			copyURL := *documentURL
			if r.detachedDocumentURLs == nil {
				r.detachedDocumentURLs = make(map[*dom.Document]*url.URL)
			}
			r.detachedDocumentURLs[clone] = &copyURL
		}
	}
	clone.Node = *cloneNode
	// CloneNode linked the copied children to cloneNode's address. This
	// Document embeds its Node at a different address, so repair the first
	// parent link or event paths stop at documentElement and omit Document.
	for child := clone.Node.FirstChild; child != nil; child = child.NextSibling {
		child.Parent = &clone.Node
	}
	var assignOwner func(*dom.Node, *dom.Document)
	assignOwner = func(node *dom.Node, owner *dom.Document) {
		if node == nil {
			return
		}
		node.Doc = owner
		node.ID = owner.AllocateNodeID()
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			assignOwner(child, owner)
		}
		if node.TemplateContentNode != nil {
			contentDoc := owner.TemplateContentsOwnerDocument()
			assignOwner(node.TemplateContentNode, contentDoc)
		}
	}
	assignOwner(&clone.Node, clone)
	clone.HTML = documentElementOf(clone)
	clone.Head = documentChildOf(clone, "head")
	clone.Body = documentBodyOf(clone)
	clone.DocType = documentDoctypeOf(clone)
	return clone
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

// nodeStaticName is the DOM Standard's fixed nodeName for the node types that
// have one. Elements are named by their tag, so they never reach here.
func nodeStaticName(node *dom.Node) string {
	if node == nil {
		return ""
	}
	switch node.Type {
	case dom.NodeText:
		return "#text"
	case dom.NodeCDATASection:
		return "#cdata-section"
	case dom.NodeComment:
		return "#comment"
	case dom.NodeDocument:
		return "#document"
	case dom.NodeDocumentFragment:
		return "#document-fragment"
	case dom.NodeDoctype:
		return "#doctype"
	case dom.NodeProcessingInstruction:
		return node.Data
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
	batch, owns := r.beginCustomElementReactionBatch()
	touched := make([]*dom.Node, 0, 4)
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.DocumentFragment() {
			for _, child := range node.ChildNodes() {
				visit(child)
			}
			return
		}
		connected := node.IsConnected()
		if connected && node.Element() && node.Namespace == dom.NSHTML {
			r.maybeLoadImageInput(node, false)
			if r.documentHasBrowsingContext(node.Doc) && node.Data == "script" && !node.ScriptAlreadyStarted {
				node.ScriptAlreadyStarted = true
				hasSrc := node.HasAttribute("src")
				typeName := strings.ToLower(strings.TrimSpace(node.GetAttribute("type")))
				hasType := node.HasAttribute("type")
				classic := !hasType || typeName == "" || typeName == "text/javascript" || typeName == "application/javascript"
				if !hasSrc && classic {
					_ = r.Run(node.TextContent(), "")
				}
			}
			r.mu.Lock()
			def := r.customElementInstances[node]
			r.mu.Unlock()
			if def == nil {
				def = r.customElementDefinitionForNode(node)
			}
			if def != nil && !r.isCustomElementInstance(node, def) && !r.isFailedCustomElement(node) {
				r.upgradeElementInRegistry(node, def, false, false, r.customElementRegistryForNode(node))
			}
			if def != nil && r.isCustomElementInstance(node, def) {
				r.mu.Lock()
				if r.customElementConnected == nil {
					r.customElementConnected = make(map[*dom.Node]bool)
				}
				wasConnected := r.customElementConnected[node]
				r.customElementConnected[node] = true
				r.mu.Unlock()
				if !wasConnected && hasLifecycleCallback(def, "connectedCallback") {
					connectedNode := node
					if batch.enqueueConnected(node, func() { r.invokeLifecycleCallback(connectedNode, "connectedCallback") }) {
						touched = append(touched, node)
					}
				}
			}
		}
		// Stage the whole shadow-including subtree before invoking the host's
		// callback. A callback can synchronously remove/adopt a descendant whose
		// own reaction queue already contains its pending connection reaction.
		if node.Element() && node.ShadowRoot != nil {
			for child := node.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
				visit(child)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(newChild)
	r.finishCustomElementReactionBatch(batch, owns, touched)
}
