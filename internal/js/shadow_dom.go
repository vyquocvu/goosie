package js

import (
	"strconv"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// setupShadowDOM installs the Shadow DOM API on element prototypes.
func (r *Runtime) setupShadowDOM() {
	if r.nodeProto == nil {
		return
	}
	if r.shadowRootProto == nil {
		ctor := r.vm.ToValue(func(goja.ConstructorCall) *goja.Object { panic(r.vm.NewTypeError("Illegal constructor")) }).(*goja.Object)
		r.shadowRootProto = ctor.Get("prototype").(*goja.Object)
		_ = r.shadowRootProto.Set("constructor", ctor)
		_ = r.shadowRootProto.SetPrototype(r.documentFragmentProto)
		_ = r.vm.Set("ShadowRoot", ctor)
		if win, ok := r.vm.Get("window").(*goja.Object); ok {
			_ = win.Set("ShadowRoot", ctor)
		}
	}
	_ = r.shadowRootProto.DefineAccessorProperty("customElementRegistry", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		_, root := r.shadowRootFromValue(call.This)
		if root == nil {
			return goja.Null()
		}
		return r.customElementRegistryObject(r.customElementRegistryForShadowRoot(root))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.shadowRootProto.DefineAccessorProperty("delegatesFocus", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		_, root := r.shadowRootFromValue(call.This)
		if root == nil {
			return r.vm.ToValue(false)
		}
		return r.vm.ToValue(root.DelegatesFocus)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.shadowRootProto.DefineAccessorProperty("ownerDocument", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		host, root := r.shadowRootFromValue(call.This)
		if host == nil || root == nil || host.Doc == nil {
			return goja.Null()
		}
		if host.Doc == r.domDoc {
			return r.wrapNode(&host.Doc.Node)
		}
		return r.wrapDetachedDocument(host.Doc)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.shadowRootProto.DefineAccessorProperty("activeElement", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		host, root := r.shadowRootFromValue(call.This)
		if host == nil || root == nil || r.opts.ActiveElement == nil {
			return goja.Null()
		}
		active := r.opts.ActiveElement()
		if active == nil || !active.IsConnected() {
			return goja.Null()
		}
		return r.wrapNode(retargetShadowActiveElement(active, host, root))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.shadowRootProto.Set("getElementById", func(call goja.FunctionCall) goja.Value {
		rootNode, root := r.shadowRootFromValue(call.This)
		if rootNode == nil || root == nil || len(call.Arguments) == 0 {
			return goja.Null()
		}
		wanted := call.Arguments[0].String()
		var found *dom.Node
		for child := root.FirstChild; child != nil && found == nil; child = child.NextSibling {
			child.Walk(func(node *dom.Node) bool {
				if node.Element() && node.GetAttribute("id") == wanted {
					found = node
					return false
				}
				return true
			})
		}
		if found == nil {
			return goja.Null()
		}
		return r.wrapNode(found)
	})
	_ = r.shadowRootProto.Set("appendChild", func(call goja.FunctionCall) goja.Value {
		_, sr := r.shadowRootFromValue(call.This)
		if sr == nil || len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		child := r.getNode(call.Arguments[0])
		if child == nil {
			return goja.Undefined()
		}
		oldDocuments := captureOwnerDocuments([]*dom.Node{child})
		wasConnected := captureConnectedState([]*dom.Node{child})
		sr.AppendChild(child)
		r.markRadioGroupsDirtyForNodes([]*dom.Node{child})
		r.notifyMutation()
		r.notifyDisconnected([]*dom.Node{child}, wasConnected)
		r.notifyAdopted([]*dom.Node{child}, oldDocuments)
		r.activateDOMInsertedSubtree(child)
		return call.Arguments[0]
	})
	_ = r.shadowRootProto.Set("removeChild", func(call goja.FunctionCall) goja.Value {
		host, sr := r.shadowRootFromValue(call.This)
		child := r.getNode(call.Argument(0))
		if host == nil || sr == nil || child == nil || child.Parent != host || !shadowRootContains(sr, child) {
			panic(r.domError("NotFoundError", "The node is not a child of this shadow root"))
		}
		wasConnected := child.IsConnected()
		r.preserveCustomElementRegistryForDetachedNode(child)
		unlinkShadowRootChild(sr, child)
		r.markRadioGroupsDirtyForNodes([]*dom.Node{child})
		if wasConnected {
			r.invokeDisconnectedCallbacks(child)
		}
		r.notifyMutation()
		return r.wrapNode(child)
	})
	_ = r.shadowRootProto.Set("append", func(call goja.FunctionCall) goja.Value {
		_, sr := r.shadowRootFromValue(call.This)
		if sr == nil {
			return goja.Undefined()
		}
		var inserted []*dom.Node
		for _, arg := range call.Arguments {
			if child := r.getNode(arg); child != nil {
				inserted = append(inserted, child)
			} else if host, _ := r.shadowRootFromValue(call.This); host != nil && host.Doc != nil {
				inserted = append(inserted, host.Doc.NewText(arg.String()))
			}
		}
		oldDocuments := captureOwnerDocuments(inserted)
		wasConnected := captureConnectedState(inserted)
		for _, child := range inserted {
			sr.AppendChild(child)
		}
		r.markRadioGroupsDirtyForNodes(inserted)
		r.notifyMutation()
		r.notifyDisconnected(inserted, wasConnected)
		r.notifyAdopted(inserted, oldDocuments)
		for _, child := range inserted {
			r.activateDOMInsertedSubtree(child)
		}
		return goja.Undefined()
	})

	// Add attachShadow to nodeProto
	_ = r.nodeProto.Set("attachShadow", func(call goja.FunctionCall) goja.Value {
		return r.jsAttachShadow(call)
	})

	// Add shadowRoot getter to nodeProto
	_ = r.vm.Set("__nodeProto__", r.nodeProto)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__nodeProto__, 'shadowRoot', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return null;
				return document.__getShadowRoot__(nid);
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__nodeProto__", nil)

	// Install __getShadowRoot__ on document
	if doc := r.vm.Get("document"); doc != nil {
		if docObj, ok := doc.(*goja.Object); ok {
			_ = docObj.Set("__getShadowRoot__", func(call goja.FunctionCall) goja.Value {
				return r.jsGetShadowRoot(call)
			})
		}
	}
}

// jsAttachShadow implements element.attachShadow(options).
func (r *Runtime) jsAttachShadow(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil {
		return goja.Null()
	}

	// Check if already has shadow root
	if node.ShadowRoot != nil {
		panic(r.domError("NotSupportedError", "Shadow root already exists"))
	}

	// Parse options
	mode := "open"
	slotAssignment := "named"
	clonable, delegatesFocus := false, false
	var registry *CustomElementRegistry
	registrySpecified := false
	if len(call.Arguments) >= 1 {
		if opts, ok := call.Argument(0).(*goja.Object); ok {
			if value := opts.Get("mode"); value != nil && !goja.IsUndefined(value) {
				mode = value.String()
			}
			if value := opts.Get("slotAssignment"); value != nil && !goja.IsUndefined(value) && value.String() == "manual" {
				slotAssignment = "manual"
			}
			if value := opts.Get("clonable"); value != nil && !goja.IsUndefined(value) {
				clonable = value.ToBoolean()
			}
			if value := opts.Get("delegatesFocus"); value != nil && !goja.IsUndefined(value) {
				delegatesFocus = value.ToBoolean()
			}
			if value := opts.Get("customElementRegistry"); value != nil && !goja.IsUndefined(value) {
				registrySpecified = true
				if !goja.IsNull(value) {
					registryObject, ok := value.(*goja.Object)
					if !ok || r.customElementRegistryObjects[registryObject] == nil {
						panic(r.vm.NewTypeError("The customElementRegistry option is not a CustomElementRegistry"))
					}
					registry = r.customElementRegistryObjects[registryObject]
				}
			}
		}
	}

	// Create shadow root
	sr := &dom.ShadowRoot{
		Host:                      node,
		Mode:                      mode,
		DelegatesFocus:            delegatesFocus,
		SlotAssignment:            slotAssignment,
		Clonable:                  clonable,
		CustomElementRegistryNull: registrySpecified && registry == nil,
	}
	if registrySpecified {
		if r.customElementShadowRegistries == nil {
			r.customElementShadowRegistries = make(map[*dom.ShadowRoot]*CustomElementRegistry)
		}
		r.customElementShadowRegistries[sr] = registry
	}
	r.associateScopedRegistryWithDocument(registry, node.Doc)
	node.ShadowRoot = sr

	// Create JS wrapper for shadow root
	srObj := r.vm.NewObject()
	_ = srObj.SetPrototype(r.shadowRootProto)
	_ = srObj.Set("host", call.This)
	_ = srObj.Set("mode", mode)

	r.installShadowRootTreeMethods(srObj, node, sr)

	// Store shadow root wrapper
	r.mu.Lock()
	nodeID := r.wrapCache[node]
	if nodeID > 0 {
		r.shadowRoots[nodeID] = srObj
	}
	for _, constructing := range r.customConstructionNodes {
		if constructing == node {
			if r.customElementShadowRootsDuringConstruction == nil {
				r.customElementShadowRootsDuringConstruction = make(map[*dom.Node]goja.Value)
			}
			r.customElementShadowRootsDuringConstruction[node] = srObj
			if state := r.elementInternals[node]; state != nil {
				state.shadowRoot = srObj
			}
			break
		}
	}
	r.mu.Unlock()

	return srObj
}

// jsGetShadowRoot implements document.__getShadowRoot__(nodeID).
func (r *Runtime) jsGetShadowRoot(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return goja.Null()
	}
	nodeID := int(call.Arguments[0].ToInteger())
	allowClosed := len(call.Arguments) > 1 && call.Argument(1).ToBoolean()

	r.mu.Lock()
	defer r.mu.Unlock()

	if node, ok := r.nodeRegistry[nodeID]; ok && node.ShadowRoot != nil && node.ShadowRoot.Mode == "closed" && !allowClosed {
		return goja.Null()
	}
	if srObj, ok := r.shadowRoots[nodeID]; ok {
		return srObj
	}

	// Check if the underlying DOM node has a shadow root (e.g. Declarative Shadow DOM)
	if node, ok := r.nodeRegistry[nodeID]; ok && node.ShadowRoot != nil {
		if node.ShadowRoot.Mode == "closed" && !allowClosed {
			return goja.Null()
		}
		srObj := r.vm.NewObject()
		_ = srObj.SetPrototype(r.shadowRootProto)
		_ = srObj.Set("mode", node.ShadowRoot.Mode)
		if hostVal := r.wrapNode(node); hostVal != nil {
			_ = srObj.Set("host", hostVal)
		}
		sr := node.ShadowRoot
		r.installShadowRootTreeMethods(srObj, node, sr)
		r.shadowRoots[nodeID] = srObj
		return srObj
	}

	return goja.Null()
}

func shadowRootContains(sr *dom.ShadowRoot, target *dom.Node) bool {
	var contains func(*dom.Node) bool
	contains = func(node *dom.Node) bool {
		for child := node; child != nil; child = child.NextSibling {
			if child == target {
				return true
			}
			if contains(child.FirstChild) {
				return true
			}
		}
		return false
	}
	return sr != nil && contains(sr.FirstChild)
}

func retargetDocumentActiveElement(active *dom.Node) *dom.Node {
	current := active
	for active != nil && active.Doc != nil {
		host := shadowHostContaining(active.Doc.Node.FirstChild, current)
		if host == nil || host == current {
			break
		}
		current = host
	}
	return current
}

func shadowHostContaining(nodes, target *dom.Node) *dom.Node {
	for node := nodes; node != nil; node = node.NextSibling {
		if node.ShadowRoot != nil && shadowRootContains(node.ShadowRoot, target) {
			if nested := shadowHostContaining(node.ShadowRoot.FirstChild, target); nested != nil {
				return nested
			}
			return node
		}
		if nested := shadowHostContaining(node.FirstChild, target); nested != nil {
			return nested
		}
	}
	return nil
}

func (r *Runtime) shadowRootContainingNode(target *dom.Node) *dom.ShadowRoot {
	if r == nil || r.domDoc == nil || target == nil {
		return nil
	}
	var find func(*dom.Node) *dom.ShadowRoot
	find = func(node *dom.Node) *dom.ShadowRoot {
		for current := node; current != nil; current = current.NextSibling {
			if current.ShadowRoot != nil {
				if nested := find(current.ShadowRoot.FirstChild); nested != nil {
					return nested
				}
				if shadowRootContains(current.ShadowRoot, target) {
					return current.ShadowRoot
				}
			}
			if nested := find(current.FirstChild); nested != nil {
				return nested
			}
		}
		return nil
	}
	return find(r.domDoc.Node.FirstChild)
}

func (r *Runtime) fireShadowRootEvent(sr *dom.ShadowRoot, ev *dom.Event) {
	if r == nil || sr == nil || sr.Host == nil || ev == nil || !ev.Bubbles() {
		return
	}
	nodeID := r.wrapCache[sr.Host]
	root := r.shadowRoots[nodeID]
	if root == nil {
		return
	}
	listeners := append([]*genericEventListener(nil), r.genericEventTargets[root]...)
	event := r.wrapEvent(ev).(*goja.Object)
	_ = event.Set("currentTarget", root)
	_ = event.Set("eventPhase", int(dom.BubblingPhase))
	for _, listener := range listeners {
		if listener.typ != ev.Type || !r.genericListenerPresent(root, listener) {
			continue
		}
		if listener.once {
			r.removeGenericListener(root, listener)
		}
		ev.SetInPassiveListener(listener.passive)
		_ = event.Set("__passiveListener", listener.passive)
		r.withCurrentEvent(event, func() {
			if listener.fn != nil {
				_, _ = listener.fn(root, event)
			} else if object, ok := listener.value.(*goja.Object); ok {
				if fn, ok := goja.AssertFunction(object.Get("handleEvent")); ok {
					_, _ = fn(listener.value, event)
				}
			}
		})
		ev.SetInPassiveListener(false)
		_ = event.Set("__passiveListener", false)
		if ev.PropagationStopped() {
			break
		}
	}
}

func retargetShadowActiveElement(active, host *dom.Node, root *dom.ShadowRoot) *dom.Node {
	if active == nil || host == nil || root == nil || !shadowRootContains(root, active) {
		return nil
	}
	current := active
	for parent := currentParent(current); parent != nil && parent != host; parent = currentParent(parent) {
		if parent.ShadowRoot != nil && shadowRootContains(parent.ShadowRoot, current) {
			current = parent
		}
	}
	return current
}

func currentParent(node *dom.Node) *dom.Node {
	if node == nil {
		return nil
	}
	return node.Parent
}

func (r *Runtime) shadowRootFromValue(value goja.Value) (*dom.Node, *dom.ShadowRoot) {
	obj, ok := value.(*goja.Object)
	if !ok {
		return nil, nil
	}
	if idValue := obj.Get("__shadowHostNid__"); idValue != nil && !goja.IsUndefined(idValue) {
		id := int(idValue.ToInteger())
		r.mu.Lock()
		host := r.nodeRegistry[id]
		registered := r.shadowRoots[id] == obj
		r.mu.Unlock()
		if registered && host != nil && host.ShadowRoot != nil {
			return host, host.ShadowRoot
		}
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, wrapper := range r.shadowRoots {
		if wrapper == obj {
			host := r.nodeRegistry[id]
			if host != nil {
				return host, host.ShadowRoot
			}
		}
	}
	return nil, nil
}

func unlinkShadowRootChild(sr *dom.ShadowRoot, child *dom.Node) {
	sr.RemoveChild(child)
}

func (r *Runtime) installShadowRootTreeMethods(root *goja.Object, host *dom.Node, sr *dom.ShadowRoot) {
	if r.genericEventTargets == nil {
		r.genericEventTargets = make(map[*goja.Object][]*genericEventListener)
	}
	r.genericEventTargets[root] = nil
	_ = root.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		r.addGenericEventListener(call)
		return goja.Undefined()
	})
	_ = root.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		r.jsRemoveGenericEventListener(call)
		return goja.Undefined()
	})
	_ = root.Set("dispatchEvent", func(call goja.FunctionCall) goja.Value {
		return r.dispatchGenericEvent(call)
	})
	if nodeID, ok := r.wrapCache[host]; ok {
		_ = root.Set("__shadowHostNid__", nodeID)
	}
	adoptedGetter := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		values := r.adoptedStyleSheets[sr]
		arrayValues := make([]interface{}, len(values))
		for i, value := range values {
			arrayValues[i] = value
		}
		array := r.vm.NewArray(arrayValues...)
		_ = array.Set("push", func(call goja.FunctionCall) goja.Value {
			if r.adoptedStyleSheets == nil {
				r.adoptedStyleSheets = make(map[*dom.ShadowRoot][]goja.Value)
			}
			values := append([]goja.Value(nil), r.adoptedStyleSheets[sr]...)
			for _, value := range call.Arguments {
				if _, ok := value.(*goja.Object); !ok {
					panic(r.vm.NewTypeError("adoptedStyleSheets entries must be CSSStyleSheet objects"))
				}
				values = append(values, value)
			}
			r.adoptedStyleSheets[sr] = values
			sr.AdoptedStyleSheetTexts = sr.AdoptedStyleSheetTexts[:0]
			for _, value := range values {
				if object, ok := value.(*goja.Object); ok {
					if text := object.Get("__constructedCSS"); text != nil && !goja.IsUndefined(text) {
						sr.AdoptedStyleSheetTexts = append(sr.AdoptedStyleSheetTexts, text.String())
					}
				}
			}
			r.notifyMutation()
			return r.vm.ToValue(len(values))
		})
		return array
	})
	adoptedSetter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		var values []goja.Value
		if len(call.Arguments) > 0 && !goja.IsNull(call.Arguments[0]) && !goja.IsUndefined(call.Arguments[0]) {
			array, ok := call.Arguments[0].(*goja.Object)
			if !ok {
				panic(r.vm.NewTypeError("adoptedStyleSheets must be a sequence"))
			}
			length := array.Get("length").ToInteger()
			for i := int64(0); i < length; i++ {
				values = append(values, array.Get(strconv.FormatInt(i, 10)))
			}
		}
		if r.adoptedStyleSheets == nil {
			r.adoptedStyleSheets = make(map[*dom.ShadowRoot][]goja.Value)
		}
		r.adoptedStyleSheets[sr] = values
		sr.AdoptedStyleSheetTexts = sr.AdoptedStyleSheetTexts[:0]
		for _, value := range values {
			if object, ok := value.(*goja.Object); ok {
				if text := object.Get("__constructedCSS"); text != nil && !goja.IsUndefined(text) {
					sr.AdoptedStyleSheetTexts = append(sr.AdoptedStyleSheetTexts, text.String())
				}
			}
		}
		r.notifyMutation()
		return goja.Undefined()
	})
	_ = root.DefineAccessorProperty("adoptedStyleSheets", adoptedGetter, adoptedSetter, goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = root.DefineAccessorProperty("firstChild", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.wrapNode(sr.FirstChild)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = root.DefineAccessorProperty("lastChild", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.wrapNode(sr.LastChild)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = root.DefineAccessorProperty("firstElementChild", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		for child := sr.FirstChild; child != nil; child = child.NextSibling {
			if child.Element() {
				return r.wrapNode(child)
			}
		}
		return goja.Null()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = root.DefineAccessorProperty("lastElementChild", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		for child := sr.LastChild; child != nil; child = child.PrevSibling {
			if child.Element() {
				return r.wrapNode(child)
			}
		}
		return goja.Null()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	queryRoot := func() *dom.Node {
		return &dom.Node{Type: dom.NodeDocumentFragment, Doc: host.Doc, FirstChild: sr.FirstChild, LastChild: sr.LastChild}
	}
	_ = root.Set("querySelector", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("querySelector requires a selector"))
		}
		n := queryRoot()
		p := r.selectorPlanFor(call.Arguments[0].String())
		if p.syntaxErr != "" {
			r.throwSelectorSyntaxError("querySelector", n, p)
		}
		if match := r.querySelectorPlan(n, p); match != nil {
			return r.wrapNode(match)
		}
		return goja.Null()
	})
	_ = root.Set("querySelectorAll", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("querySelectorAll requires a selector"))
		}
		n := queryRoot()
		p := r.selectorPlanFor(call.Arguments[0].String())
		if p.syntaxErr != "" {
			r.throwSelectorSyntaxError("querySelectorAll", n, p)
		}
		matches := r.querySelectorAllPlan(n, p)
		values := make([]goja.Value, len(matches))
		for i, match := range matches {
			values[i] = r.wrapNode(match)
		}
		return newJSArray(r.vm, values)
	})
	getter := r.vm.ToValue(func(goja.FunctionCall) goja.Value { return r.vm.ToValue(r.serializeChildren(queryRoot())) })
	setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		html := ""
		if len(call.Arguments) > 0 && call.Arguments[0] != nil {
			html = call.Arguments[0].String()
		}
		r.setShadowRootInnerHTML(sr, host, html)
		return goja.Undefined()
	})
	_ = root.DefineAccessorProperty("innerHTML", getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = root.Set("setHTMLUnsafe", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to execute 'setHTMLUnsafe' on 'ShadowRoot': 1 argument required"))
		}
		r.setShadowRootInnerHTML(sr, host, call.Arguments[0].String())
		return goja.Undefined()
	})
}

func (r *Runtime) setShadowRootInnerHTML(sr *dom.ShadowRoot, host *dom.Node, html string) {
	parsed, err := dom.ParseFragmentBounded(host, html, dom.ParseLimits{Nodes: 1024, Depth: 32, Attributes: 256, AttributeBytes: 4096})
	if err != nil {
		panic(r.vm.NewTypeError("ShadowRoot.innerHTML could not be parsed"))
	}
	var newChildren []*dom.Node
	for c := parsed.FirstChild; c != nil; c = c.NextSibling {
		newChildren = append(newChildren, c)
	}
	var old []*dom.Node
	for c := sr.FirstChild; c != nil; c = c.NextSibling {
		old = append(old, c)
	}
	wasConnected := captureConnectedState(old)
	wasLive := r.nodeIsLive(host)
	registry := r.customElementRegistryForShadowRoot(sr)
	r.associateScopedRegistryWithDocument(registry, host.Doc)
	for _, child := range newChildren {
		r.associateParsedSubtreeWithRegistry(child, registry, true)
	}
	r.admitInsert("innerHTML", host, newChildren, func() {
		for _, c := range old {
			sr.RemoveChild(c)
		}
		for _, c := range newChildren {
			parsed.RemoveChild(c)
			sr.AppendChild(c)
		}
	})
	if wasLive {
		r.releaseNodes(old)
	}
	r.markRadioGroupsDirtyForNodes(append(old, newChildren...))
	r.notifyDisconnected(old, wasConnected)
	for _, c := range newChildren {
		r.activateInsertedSubtree(c)
	}
	r.notifyMutation()
}

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		return ""
	}
}
