package js

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// CustomElementRegistry implements the Custom Elements API.
type CustomElementRegistry struct {
	mu                  sync.Mutex
	definitions         map[string]*customElementDefinition
	whenDefined         map[string]*promiseObj
	defining            bool
	isScoped            bool
	document            *dom.Document
	object              *goja.Object
	scopedDocs          map[*dom.Document]struct{}
	scopedDocumentOrder []*dom.Document
}

type customElementDefinition struct {
	name                     string
	constructor              goja.Constructor
	constructorRealm         *Runtime
	value                    goja.Value
	prototype                *goja.Object
	lifecycleCallbacks       map[string]goja.Value
	attributeChangedCallback goja.Value
	observed                 []string
	extends                  string
	formAssociated           bool
	disableShadow            bool
	disableInternals         bool
}

type customElementReactionBatch struct {
	queues     map[*dom.Node][]func()
	connected  map[*dom.Node]int
	order      []*dom.Node
	processing map[*dom.Node]bool
	current    *dom.Node
}

func newCustomElementReactionBatch() *customElementReactionBatch {
	return &customElementReactionBatch{queues: make(map[*dom.Node][]func()), connected: make(map[*dom.Node]int), processing: make(map[*dom.Node]bool)}
}

func (b *customElementReactionBatch) enqueue(node *dom.Node, reaction func()) {
	if b == nil || node == nil || reaction == nil {
		return
	}
	if len(b.queues[node]) == 0 {
		b.order = append(b.order, node)
	}
	b.queues[node] = append(b.queues[node], reaction)
}

func (b *customElementReactionBatch) enqueueConnected(node *dom.Node, reaction func()) bool {
	if b == nil || node == nil || reaction == nil || b.connected[node] > 0 {
		return false
	}
	b.connected[node]++
	b.enqueue(node, func() {
		b.connected[node]--
		reaction()
	})
	return true
}

func (b *customElementReactionBatch) process(node *dom.Node) {
	if b == nil || node == nil || b.processing[node] {
		return
	}
	b.processing[node] = true
	previousCurrent := b.current
	defer func() {
		b.processing[node] = false
		b.current = previousCurrent
	}()
	for len(b.queues[node]) > 0 {
		queue := b.queues[node]
		reaction := queue[0]
		b.queues[node] = queue[1:]
		b.current = node
		reaction()
	}
}

func (b *customElementReactionBatch) processAll() {
	for i := 0; i < len(b.order); i++ {
		b.process(b.order[i])
	}
}

func (r *Runtime) beginCustomElementReactionBatch() (*customElementReactionBatch, bool) {
	if r.customElementReactionBatch != nil {
		return r.customElementReactionBatch, false
	}
	batch := newCustomElementReactionBatch()
	r.customElementReactionBatch = batch
	return batch, true
}

func (r *Runtime) finishCustomElementReactionBatch(batch *customElementReactionBatch, owns bool, touched []*dom.Node) {
	if batch == nil {
		return
	}
	if owns {
		batch.processAll()
		for node := range batch.queues {
			if node != nil && !node.IsConnected() {
				r.mu.Lock()
				delete(r.customElementConnected, node)
				r.mu.Unlock()
			}
		}
		r.customElementReactionBatch = nil
		return
	}
	for _, node := range touched {
		batch.process(node)
	}
}

// setupCustomElements installs the Custom Elements API on window.
func (r *Runtime) setupCustomElements() {
	r.customElementRegistryObjects = make(map[*goja.Object]*CustomElementRegistry)
	r.customElementNodeRegistries = make(map[*dom.Node]*CustomElementRegistry)
	r.customElementNullRegistryLocks = make(map[*dom.Node]bool)
	r.customElementInitialRegistryDocs = make(map[*dom.Document]struct{})
	r.customElementDocumentRegistries = make(map[*dom.Document]*CustomElementRegistry)
	r.customElementShadowRegistries = make(map[*dom.ShadowRoot]*CustomElementRegistry)
	var interfaceObject *goja.Object
	registryInterface := func(call goja.ConstructorCall) *goja.Object {
		registry, registryObj := r.newCustomElementRegistry(interfaceObject)
		registry.isScoped = true
		return registryObj
	}
	interfaceObject = r.vm.ToValue(registryInterface).(*goja.Object)
	if prototype, ok := interfaceObject.Get("prototype").(*goja.Object); ok {
		// Instance methods are installed on each registry object below so they
		// close over that registry's own definition map. These prototype
		// methods preserve the Web IDL interface shape and dispatch to the
		// default registry when called with its branded object.
		_ = prototype.Set("define", func(call goja.FunctionCall) goja.Value {
			return r.jsCustomElementsDefine(call, r.customElementRegistry)
		})
		_ = prototype.Set("get", func(call goja.FunctionCall) goja.Value {
			return r.jsCustomElementsGet(call, r.customElementRegistry)
		})
		_ = prototype.Set("whenDefined", func(call goja.FunctionCall) goja.Value {
			return r.jsCustomElementsWhenDefined(call, r.customElementRegistry)
		})
		_ = prototype.Set("getName", func(call goja.FunctionCall) goja.Value {
			return r.jsCustomElementsGetName(call, r.customElementRegistry)
		})
		_ = prototype.Set("upgrade", func(call goja.FunctionCall) goja.Value {
			return r.jsCustomElementsUpgrade(call, r.customElementRegistry)
		})
		_ = prototype.Set("initialize", func(call goja.FunctionCall) goja.Value {
			return r.jsCustomElementsInitialize(call, r.customElementRegistry)
		})
		_ = prototype.Set("constructor", interfaceObject)
	}
	_ = interfaceObject.Set("name", "CustomElementRegistry")
	_ = r.vm.Set("CustomElementRegistry", interfaceObject)
	r.setupElementInternals()

	registry, registryObj := r.newCustomElementRegistry(interfaceObject)
	registry.document = r.domDoc
	r.customElementRegistry = registry

	_ = r.vm.Set("customElements", registryObj)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("customElements", registryObj)
		}
	}
	r.associateInitialNullRegistryAttributes()

}

// associateInitialNullRegistryAttributes captures the parser-only effect of
// the customelementregistry attribute before page scripts can mutate it. The
// attribute selects a null registry for the element and its descendants only
// when the parser creates that subtree; adding it later does not change an
// existing element's registry.
func (r *Runtime) associateInitialNullRegistryAttributes() {
	r.associateNullRegistryAttributesInDocument(r.domDoc)
}

func (r *Runtime) associateNullRegistryAttributesInDocument(doc *dom.Document) {
	if doc == nil {
		return
	}
	if r.customElementInitialRegistryDocs == nil {
		r.customElementInitialRegistryDocs = make(map[*dom.Document]struct{})
	}
	if _, scanned := r.customElementInitialRegistryDocs[doc]; scanned {
		return
	}
	r.customElementInitialRegistryDocs[doc] = struct{}{}
	if r.customElementNodeRegistries == nil {
		r.customElementNodeRegistries = make(map[*dom.Node]*CustomElementRegistry)
	}
	var visit func(*dom.Node, bool)
	visit = func(node *dom.Node, inheritedNull bool) {
		if node == nil {
			return
		}
		if node.Element() {
			inheritedNull = inheritedNull || node.HasAttribute("customelementregistry")
			if inheritedNull {
				r.customElementNodeRegistries[node] = nil
				r.customElementNullRegistryLocks[node] = true
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child, inheritedNull)
		}
	}
	visit(&doc.Node, false)
}

func (r *Runtime) newCustomElementRegistry(interfaceObject *goja.Object) (*CustomElementRegistry, *goja.Object) {
	registry := &CustomElementRegistry{
		definitions: make(map[string]*customElementDefinition),
		whenDefined: make(map[string]*promiseObj),
		scopedDocs:  make(map[*dom.Document]struct{}),
	}
	registryObj := r.vm.NewObject()
	registry.object = registryObj
	r.customElementRegistryObjects[registryObj] = registry
	if prototype, ok := interfaceObject.Get("prototype").(*goja.Object); ok {
		_ = registryObj.SetPrototype(prototype)
	}
	_ = registryObj.Set("define", func(call goja.FunctionCall) goja.Value {
		return r.jsCustomElementsDefine(call, registry)
	})
	_ = registryObj.Set("get", func(call goja.FunctionCall) goja.Value {
		return r.jsCustomElementsGet(call, registry)
	})
	_ = registryObj.Set("whenDefined", func(call goja.FunctionCall) goja.Value {
		return r.jsCustomElementsWhenDefined(call, registry)
	})
	_ = registryObj.Set("getName", func(call goja.FunctionCall) goja.Value {
		return r.jsCustomElementsGetName(call, registry)
	})
	_ = registryObj.Set("upgrade", func(call goja.FunctionCall) goja.Value {
		return r.jsCustomElementsUpgrade(call, registry)
	})
	_ = registryObj.Set("initialize", func(call goja.FunctionCall) goja.Value {
		return r.jsCustomElementsInitialize(call, registry)
	})
	return registry, registryObj
}

// jsCustomElementsDefine implements customElements.define(name, constructor, options).
func (r *Runtime) jsCustomElementsDefine(call goja.FunctionCall, registry *CustomElementRegistry) goja.Value {
	if len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'define' on 'CustomElementRegistry': 2 arguments required"))
	}

	name := call.Arguments[0].String()
	constructorVal := call.Arguments[1]
	constructor, isConstructor := goja.AssertConstructor(constructorVal)
	if !isConstructor {
		panic(r.vm.NewTypeError("Custom element constructor is not a constructor"))
	}

	// Validate name
	if !isValidCustomElementName(name) {
		panic(r.customElementNameException("SyntaxError", fmt.Sprintf("'%s' is not a valid custom element name", name)))
	}

	// The registry checks the name and constructor before inspecting user
	// supplied constructor properties. The definition-running flag is set
	// before those property reads so reentrant define() calls fail in order.
	registry.mu.Lock()
	if _, exists := registry.definitions[name]; exists {
		registry.mu.Unlock()
		panic(r.customElementDefinitionException("NotSupportedError", fmt.Sprintf("'%s' has already been defined", name)))
	}
	for _, existing := range registry.definitions {
		if constructorVal == existing.value {
			registry.mu.Unlock()
			panic(r.customElementDefinitionException("NotSupportedError", "The constructor is already registered."))
		}
	}
	if registry.defining {
		registry.mu.Unlock()
		panic(r.customElementDefinitionException("NotSupportedError", "A custom element definition is already running."))
	}
	registry.defining = true
	registry.mu.Unlock()
	running := true
	defer func() {
		if running {
			registry.mu.Lock()
			registry.defining = false
			registry.mu.Unlock()
		}
	}()

	constructorObject, ok := constructorVal.(*goja.Object)
	if !ok {
		panic(r.vm.NewTypeError("Custom element constructor must have an object prototype"))
	}
	prototypeValue := constructorObject.Get("prototype")
	prototype, ok := prototypeValue.(*goja.Object)
	if !ok {
		panic(r.vm.NewTypeError("Custom element constructor prototype must be an object"))
	}
	callbacks := make(map[string]goja.Value)
	for _, callbackName := range []string{"connectedCallback", "disconnectedCallback", "connectedMoveCallback", "adoptedCallback", "attributeChangedCallback"} {
		callback := prototype.Get(callbackName)
		if callback == nil || goja.IsUndefined(callback) {
			callbacks[callbackName] = goja.Undefined()
			continue
		}
		if _, callable := goja.AssertFunction(callback); !callable {
			panic(r.vm.NewTypeError(fmt.Sprintf("%s must be callable", callbackName)))
		}
		callbacks[callbackName] = callback
	}

	// Parse options.
	var extends string
	hasExtends := false
	if len(call.Arguments) >= 3 {
		if !goja.IsNull(call.Arguments[2]) && !goja.IsUndefined(call.Arguments[2]) {
			opts := call.Arguments[2].ToObject(r.vm)
			if ext := opts.Get("extends"); ext != nil && !goja.IsUndefined(ext) {
				hasExtends = true
				extends = ext.String()
			}
		}
	}
	if hasExtends && (isValidCustomElementName(extends) || isHTMLUnknownElementName(extends)) {
		panic(r.customElementDefinitionException("NotSupportedError", fmt.Sprintf("'%s' is not a supported built-in element name", extends)))
	}
	if registry.isScoped && hasExtends {
		panic(r.customElementDefinitionException("NotSupportedError", "Scoped custom element registries cannot define customized built-in elements"))
	}

	// observedAttributes is read from the constructor only when the prototype
	// supplies attributeChangedCallback. Thenable/iterable conversion is done
	// through Array.from so iterator getters and conversion exceptions propagate.
	var observed []string
	if _, hasAttributeChanged := callbacks["attributeChangedCallback"]; hasAttributeChanged && !goja.IsUndefined(callbacks["attributeChangedCallback"]) {
		if obs := constructorObject.Get("observedAttributes"); obs != nil && !goja.IsUndefined(obs) {
			observed = r.customElementStringSequence(obs)
		}
	}
	// Web IDL reads these static properties in this order after the prototype
	// callbacks and observedAttributes have been converted.
	disableShadow := false
	disableInternals := false
	disabledFeatures := constructorObject.Get("disabledFeatures")
	if disabledFeatures != nil && !goja.IsUndefined(disabledFeatures) {
		for _, feature := range r.customElementStringSequence(disabledFeatures) {
			if feature == "shadow" {
				disableShadow = true
			}
			if feature == "internals" {
				disableInternals = true
			}
		}
	}
	formAssociated := constructorObject.Get("formAssociated")
	isFormAssociated := formAssociated != nil && formAssociated.ToBoolean()
	if isFormAssociated {
		for _, callbackName := range []string{"formAssociatedCallback", "formResetCallback", "formDisabledCallback", "formStateRestoreCallback"} {
			callback := prototype.Get(callbackName)
			if callback == nil || goja.IsUndefined(callback) {
				callbacks[callbackName] = goja.Undefined()
				continue
			}
			if _, callable := goja.AssertFunction(callback); !callable {
				panic(r.vm.NewTypeError(fmt.Sprintf("%s must be callable", callbackName)))
			}
			callbacks[callbackName] = callback
		}
	}
	constructorRealm := r.functionRealm(constructorObject, r)

	def := &customElementDefinition{
		name:                     name,
		constructor:              constructor,
		constructorRealm:         constructorRealm,
		value:                    constructorVal,
		prototype:                prototype,
		lifecycleCallbacks:       callbacks,
		attributeChangedCallback: callbacks["attributeChangedCallback"],
		observed:                 observed,
		extends:                  extends,
		formAssociated:           isFormAssociated,
		disableShadow:            disableShadow,
		disableInternals:         disableInternals,
	}
	r.installProxyCustomElementHasInstance(constructorObject)

	registry.mu.Lock()
	registry.definitions[name] = def
	registry.defining = false
	registry.mu.Unlock()
	running = false

	// Scoped registries upgrade only elements in documents associated with
	// that registry. The global registry upgrades candidates in its document.
	if registry.isScoped {
		registry.mu.Lock()
		documents := append([]*dom.Document(nil), registry.scopedDocumentOrder...)
		if len(documents) == 0 {
			for doc := range registry.scopedDocs {
				documents = append(documents, doc)
			}
		}
		registry.mu.Unlock()
		for _, doc := range documents {
			r.upgradeCustomElements(name, registry, doc)
		}
	} else {
		doc := registry.document
		if doc == nil {
			doc = r.domDoc
		}
		r.upgradeCustomElements(name, registry, doc)
	}

	// Resolve the stable pending promise, if whenDefined() requested one.
	registry.mu.Lock()
	pending := registry.whenDefined[name]
	delete(registry.whenDefined, name)
	registry.mu.Unlock()
	if pending != nil {
		r.resolvePromise(pending, constructorVal)
	}

	return goja.Undefined()
}

// installProxyCustomElementHasInstance supplies OrdinaryHasInstance behavior
// for Proxy-wrapped custom-element constructors. goja's default Function
// @@hasInstance path currently rejects callable Proxy receivers, even though
// the HTML constructor and newTarget handling support them.
func (r *Runtime) installProxyCustomElementHasInstance(constructor *goja.Object) {
	if constructor == nil {
		return
	}
	proxy, ok := constructor.Export().(goja.Proxy)
	if !ok {
		return
	}
	target := proxy.Target()
	if target == nil {
		return
	}
	function, ok := r.vm.Get("Function").(*goja.Object)
	if !ok {
		return
	}
	functionPrototype, ok := function.Get("prototype").(*goja.Object)
	if !ok {
		return
	}
	defaultHasInstance := functionPrototype.GetSymbol(goja.SymHasInstance)
	if defaultHasInstance == nil || !target.GetSymbol(goja.SymHasInstance).StrictEquals(defaultHasInstance) {
		return
	}
	hasInstance := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		constructor, ok := call.This.(*goja.Object)
		if !ok {
			return r.vm.ToValue(false)
		}
		prototype, ok := constructor.Get("prototype").(*goja.Object)
		if !ok {
			panic(r.vm.NewTypeError("Function has non-object prototype in instanceof check"))
		}
		instance, ok := call.Argument(0).(*goja.Object)
		if !ok {
			return r.vm.ToValue(false)
		}
		for current := instance.Prototype(); current != nil; current = current.Prototype() {
			if current == prototype {
				return r.vm.ToValue(true)
			}
		}
		return r.vm.ToValue(false)
	})
	_ = target.DefineDataPropertySymbol(goja.SymHasInstance, hasInstance, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
}

// isHTMLUnknownElementName reports names whose HTML-namespace element
// interface is HTMLUnknownElement. A few legacy names are still recognized
// by the parser's atom table, but the HTML standard no longer defines an
// element interface for them.
func isHTMLUnknownElementName(name string) bool {
	switch name {
	case "bgsound", "blink", "isindex", "multicol", "nextid", "spacer":
		return true
	default:
		if isHTMLStandardElementName(name) {
			return false
		}
		if _, hasInterface := htmlTagInterfaces[name]; hasInterface {
			return false
		}
		return name != strings.ToLower(name) || dom.Lookup(name) == dom.AtomUnknown && !strings.Contains(name, "-")
	}
}

const htmlStandardElementNames = " a abbr address area article aside audio b base bdi bdo blockquote body br button canvas caption cite code col colgroup data datalist dd del details dfn dialog div dl dt em embed fieldset figcaption figure footer form h1 h2 h3 h4 h5 h6 head header hgroup hr html i iframe img input ins kbd label legend li link main map mark menu meta meter nav noscript object ol optgroup option output p picture pre progress q rp rt ruby s samp script search section select slot small source span strong style sub summary sup table tbody td template textarea tfoot th thead time title tr track u ul var video wbr "

func isHTMLStandardElementName(name string) bool {
	return name != "" && strings.Contains(htmlStandardElementNames, " "+name+" ")
}

func (r *Runtime) customElementStringSequence(value goja.Value) []string {
	if goja.IsNull(value) || goja.IsUndefined(value) {
		panic(r.vm.NewTypeError("Value is not an iterable sequence"))
	}
	iteratorSymbol, ok := r.vm.Get("Symbol").(*goja.Object).Get("iterator").(*goja.Symbol)
	if !ok {
		panic(r.vm.NewTypeError("Symbol.iterator is unavailable"))
	}
	object := value.ToObject(r.vm)
	iteratorMethod := object.GetSymbol(iteratorSymbol)
	iteratorFunction, ok := goja.AssertFunction(iteratorMethod)
	if !ok {
		panic(r.vm.NewTypeError("Value is not iterable"))
	}
	iteratorValue, err := iteratorFunction(object)
	if err != nil {
		panic(err)
	}
	iterator, ok := iteratorValue.(*goja.Object)
	if !ok {
		panic(r.vm.NewTypeError("Iterator is not an object"))
	}
	next, ok := goja.AssertFunction(iterator.Get("next"))
	if !ok {
		panic(r.vm.NewTypeError("Iterator next is not callable"))
	}
	values := make([]string, 0)
	for range 100000 {
		stepValue, err := next(iterator)
		if err != nil {
			panic(err)
		}
		step, ok := stepValue.(*goja.Object)
		if !ok {
			panic(r.vm.NewTypeError("Iterator result is not an object"))
		}
		if step.Get("done").ToBoolean() {
			return values
		}
		values = append(values, step.Get("value").String())
	}
	panic(r.vm.NewTypeError("Sequence conversion exceeded the element limit"))
}

// jsCustomElementsGet implements customElements.get(name).
func (r *Runtime) jsCustomElementsGet(call goja.FunctionCall, registry *CustomElementRegistry) goja.Value {
	if len(call.Arguments) < 1 {
		return goja.Undefined()
	}
	name := call.Arguments[0].String()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if def, ok := registry.definitions[name]; ok {
		return def.value
	}
	return goja.Undefined()
}

// jsCustomElementsGetName implements CustomElementRegistry.getName().
func (r *Runtime) jsCustomElementsGetName(call goja.FunctionCall, registry *CustomElementRegistry) goja.Value {
	if len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'getName' on 'CustomElementRegistry': 1 argument required"))
	}
	constructor := call.Arguments[0]
	if _, ok := goja.AssertConstructor(constructor); !ok {
		panic(r.vm.NewTypeError("The provided value is not a constructor"))
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for name, def := range registry.definitions {
		if def.value.StrictEquals(constructor) {
			return r.vm.ToValue(name)
		}
	}
	return goja.Null()
}

// jsCustomElementsUpgrade upgrades unresolved custom elements in the
// shadow-including subtree rooted at the supplied node. Template contents use
// their inert owner document, so they do not resolve definitions from the
// active document's registry.
func (r *Runtime) jsCustomElementsUpgrade(call goja.FunctionCall, registry *CustomElementRegistry) goja.Value {
	if len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'upgrade' on 'CustomElementRegistry': 1 argument required"))
	}
	root := r.getNode(call.Arguments[0])
	host, shadowRoot := r.shadowRootFromValue(call.Arguments[0])
	if root == nil && shadowRoot == nil {
		panic(r.vm.NewTypeError("Failed to execute 'upgrade' on 'CustomElementRegistry': parameter 1 is not of type 'Node'"))
	}
	rootDocument := (*dom.Document)(nil)
	if root != nil {
		rootDocument = root.Doc
	} else if host != nil {
		rootDocument = host.Doc
	}
	if rootDocument == nil {
		return goja.Undefined()
	}
	if shadowRoot != nil && r.customElementRegistryForShadowRoot(shadowRoot) != registry {
		return goja.Undefined()
	}
	if root != nil && root.Type == dom.NodeElement && r.customElementRegistryForNode(root) != registry {
		return goja.Undefined()
	}
	if registry.isScoped {
		registry.mu.Lock()
		_, associated := registry.scopedDocs[rootDocument]
		registry.mu.Unlock()
		if !associated {
			return goja.Undefined()
		}
	} else if r.customElementRegistryForDocument(rootDocument) != registry {
		return goja.Undefined()
	}
	definitions := make(map[string]*customElementDefinition)
	registry.mu.Lock()
	for name, def := range registry.definitions {
		definitions[name] = def
	}
	registry.mu.Unlock()
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Type == dom.NodeElement {
			if def := r.customElementDefinitionFromSet(node, definitions); def != nil && r.customElementRegistryForNode(node) == registry && !r.isCustomElementInstance(node, def) && !r.isFailedCustomElement(node) {
				wasConnected := node.IsConnected()
				r.upgradeElementInRegistry(node, def, false, false, registry)
				if wasConnected && r.isCustomElementInstance(node, def) {
					r.invokeConnectedCallback(node)
				}
			}
		}
		if node.ShadowRoot != nil {
			for child := node.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
				visit(child)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
		// Deliberately skip TemplateContentNode. It belongs to an inert
		// template-contents document and is not part of the shadow-including
		// tree rooted at the template element.
	}
	if root != nil {
		visit(root)
	} else if shadowRoot != nil {
		for child := shadowRoot.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	return goja.Undefined()
}

func (r *Runtime) isCustomElementInstance(node *dom.Node, def *customElementDefinition) bool {
	if node == nil || def == nil || r.isFailedCustomElement(node) {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.customElementInstances[node] == def {
		return true
	}
	nid, ok := r.wrapCache[node]
	if !ok {
		return false
	}
	instance := r.nodeWrappers[nid]
	return instance != nil && instance.Prototype() == def.prototype
}

func (r *Runtime) isFailedCustomElement(node *dom.Node) bool {
	return node != nil && r.customElementFailures[node]
}

func (r *Runtime) markCustomElementInstance(node *dom.Node, def *customElementDefinition) {
	if node == nil || def == nil {
		return
	}
	r.mu.Lock()
	if r.customElementInstances == nil {
		r.customElementInstances = make(map[*dom.Node]*customElementDefinition)
	}
	r.customElementInstances[node] = def
	r.mu.Unlock()
	wasUndefined := node.CustomElementUndefined
	node.CustomElementUndefined = false
	if wasUndefined {
		r.notifyMutation()
	}
	node.FormAssociatedCustomElement = def.formAssociated
	if def.formAssociated {
		node.CustomElementValid = true
	}
}

func (r *Runtime) formAssociatedDisabled(node *dom.Node) bool {
	return node != nil && r.isFormAssociatedCustomElement(node) && dom.IsFormControlDisabled(node)
}

func (r *Runtime) initializeFormDisabledState(node *dom.Node) {
	if node == nil || !r.isFormAssociatedCustomElement(node) {
		return
	}
	if r.customElementDisabledStates == nil {
		r.customElementDisabledStates = make(map[*dom.Node]bool)
	}
	disabled := r.formAssociatedDisabled(node)
	r.customElementDisabledStates[node] = disabled
	if disabled {
		r.invokeFormDisabledCallback(node, true)
	}
}

func (r *Runtime) refreshFormDisabledState(node *dom.Node) {
	if node == nil || !r.isFormAssociatedCustomElement(node) {
		return
	}
	if r.customElementDisabledStates == nil {
		r.customElementDisabledStates = make(map[*dom.Node]bool)
	}
	next := r.formAssociatedDisabled(node)
	previous, initialized := r.customElementDisabledStates[node]
	r.customElementDisabledStates[node] = next
	if initialized && previous == next || !initialized && !next {
		return
	}
	r.invokeFormDisabledCallback(node, next)
}

func (r *Runtime) invokeFormDisabledCallback(node *dom.Node, disabled bool) {
	if node == nil {
		return
	}
	r.mu.Lock()
	def := r.customElementInstances[node]
	nid := r.wrapCache[node]
	wrapper := r.nodeWrappers[nid]
	r.mu.Unlock()
	if def == nil || wrapper == nil {
		return
	}
	fn, ok := goja.AssertFunction(def.lifecycleCallbacks["formDisabledCallback"])
	if !ok {
		return
	}
	_, _ = fn(wrapper, r.vm.ToValue(disabled))
}

func (r *Runtime) initializeFormAssociatedState(node *dom.Node) {
	if node == nil || !r.isFormAssociatedCustomElement(node) {
		return
	}
	if r.customElementFormOwners == nil {
		r.customElementFormOwners = make(map[*dom.Node]*dom.Node)
	}
	owner := r.customFormOwner(node)
	r.customElementFormOwners[node] = owner
	if owner != nil {
		r.invokeFormAssociatedCallback(node, owner)
	}
}

func (r *Runtime) refreshFormAssociatedState(node *dom.Node) {
	if node == nil || !r.isFormAssociatedCustomElement(node) {
		return
	}
	if r.customElementFormOwners == nil {
		r.customElementFormOwners = make(map[*dom.Node]*dom.Node)
	}
	owner := r.customFormOwner(node)
	previous, initialized := r.customElementFormOwners[node]
	r.customElementFormOwners[node] = owner
	if initialized && previous == owner || !initialized && owner == nil {
		return
	}
	r.invokeFormAssociatedCallback(node, owner)
}

func (r *Runtime) refreshFormAssociatedStates(doc *dom.Document) {
	if doc == nil {
		return
	}
	for _, node := range r.nodeRegistry {
		if node != nil && node.Doc == doc && node.Element() && r.isFormAssociatedCustomElement(node) {
			r.refreshFormAssociatedState(node)
		}
	}
}

func (r *Runtime) refreshFormAssociatedStateSubtree(root *dom.Node) {
	if root == nil {
		return
	}
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Element() {
			r.refreshFormAssociatedState(node)
			if node.ShadowRoot != nil {
				for child := node.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
					visit(child)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
}

func subtreeContainsFormElement(root *dom.Node) bool {
	if root == nil {
		return false
	}
	found := false
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil || found {
			return
		}
		if node.Element() {
			if node.Namespace == dom.NSHTML && node.Data == "form" {
				found = true
				return
			}
			if node.ShadowRoot != nil {
				visit(node.ShadowRoot.FirstChild)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	return found
}

func (r *Runtime) invokeFormAssociatedCallback(node, owner *dom.Node) {
	if node == nil {
		return
	}
	r.mu.Lock()
	def := r.customElementInstances[node]
	nid := r.wrapCache[node]
	wrapper := r.nodeWrappers[nid]
	r.mu.Unlock()
	if def == nil || wrapper == nil {
		return
	}
	fn, ok := goja.AssertFunction(def.lifecycleCallbacks["formAssociatedCallback"])
	if !ok {
		return
	}
	var formValue goja.Value = goja.Null()
	if owner != nil {
		formValue = r.wrapNode(owner)
	}
	_, _ = fn(wrapper, formValue)
}

func (r *Runtime) customElementDefinitionForNode(node *dom.Node) *customElementDefinition {
	if node == nil || node.Doc == nil {
		return nil
	}
	return r.customElementDefinitionForNodeInRegistry(node, r.customElementRegistryForNode(node))
}

func (r *Runtime) customElementRegistryForNode(node *dom.Node) *CustomElementRegistry {
	if node == nil {
		return nil
	}
	if node.Doc != nil && node.Doc.IsTemplateContentsOwnerDocument() {
		return nil
	}
	if registry, associated := r.customElementNodeRegistries[node]; associated {
		return registry
	}
	if root := r.shadowRootForNode(node); root != nil {
		return r.customElementRegistryForShadowRoot(root)
	}
	// A newly created element is detached until an insertion algorithm places
	// it in a tree. Resolve its document registry directly: walking every
	// wrapped shadow host to prove that the detached node is not inside one
	// makes repeated custom-element construction quadratic in document size.
	if node.Parent == nil {
		for _, registry := range r.iframeRegistries {
			if registry != nil && registry.document == node.Doc {
				return registry
			}
		}
		if node.Doc == r.domDoc {
			return r.customElementRegistry
		}
		if r.customElementRegistry != nil && r.customElementRegistry.document == node.Doc {
			return r.customElementRegistry
		}
		return nil
	}
	for _, host := range r.nodeRegistry {
		if host != nil && host.ShadowRoot != nil && shadowRootContains(host.ShadowRoot, node) {
			return r.customElementRegistryForShadowRoot(host.ShadowRoot)
		}
	}
	if node.Doc != nil {
		var findShadowRoot func(*dom.Node) *dom.ShadowRoot
		findShadowRoot = func(current *dom.Node) *dom.ShadowRoot {
			for child := current; child != nil; child = child.NextSibling {
				if child.ShadowRoot != nil {
					if shadowRootContains(child.ShadowRoot, node) {
						return child.ShadowRoot
					}
					if found := findShadowRoot(child.ShadowRoot.FirstChild); found != nil {
						return found
					}
				}
				if found := findShadowRoot(child.FirstChild); found != nil {
					return found
				}
			}
			return nil
		}
		if root := findShadowRoot(node.Doc.Node.FirstChild); root != nil {
			return r.customElementRegistryForShadowRoot(root)
		}
	}
	for current := node.Parent; current != nil; current = current.Parent {
		if registry, associated := r.customElementNodeRegistries[current]; associated {
			return registry
		}
	}
	for _, registry := range r.iframeRegistries {
		if registry != nil && registry.document == node.Doc {
			return registry
		}
	}
	if node.Doc == r.domDoc {
		return r.customElementRegistry
	}
	if r.customElementRegistry != nil && r.customElementRegistry.document == node.Doc {
		return r.customElementRegistry
	}
	return nil
}

func (r *Runtime) preserveCustomElementRegistryForDetachedNode(node *dom.Node) {
	if node == nil || !node.Element() {
		return
	}
	if _, associated := r.customElementNodeRegistries[node]; associated {
		return
	}
	registry := r.customElementRegistryForNode(node)
	if r.customElementNodeRegistries == nil {
		r.customElementNodeRegistries = make(map[*dom.Node]*CustomElementRegistry)
	}
	r.customElementNodeRegistries[node] = registry
	if registry == nil {
		if r.customElementNullRegistryLocks == nil {
			r.customElementNullRegistryLocks = make(map[*dom.Node]bool)
		}
		r.customElementNullRegistryLocks[node] = true
	}
}

func (r *Runtime) customElementRegistryForShadowRoot(root *dom.ShadowRoot) *CustomElementRegistry {
	if root == nil {
		return nil
	}
	if registry, associated := r.customElementShadowRegistries[root]; associated {
		return registry
	}
	if root.CustomElementRegistryNull {
		return nil
	}
	if root.Host != nil {
		return r.customElementRegistryForDocument(root.Host.Doc)
	}
	return nil
}

func (r *Runtime) updateCustomElementRegistryOnAdoption(node *dom.Node, target *dom.Document) {
	if node == nil || !node.Element() || target == nil {
		return
	}
	if node.ShadowRoot != nil {
		root := node.ShadowRoot
		if root.CustomElementRegistryNull && !root.CustomElementRegistryNullDeclarative {
			root.CustomElementRegistryNull = false
			if registry, associated := r.customElementShadowRegistries[root]; associated && registry == nil {
				r.customElementShadowRegistries[root] = r.customElementRegistryForDocument(target)
			}
		}
		if registry, associated := r.customElementShadowRegistries[root]; associated && registry != nil && !registry.isScoped && registry.document != nil && registry.document != target {
			// A shadow root initialized with the source document's global
			// registry follows its host to the destination document. Scoped
			// registries and explicitly null roots retain their association.
			r.customElementShadowRegistries[root] = r.customElementRegistryForDocument(target)
		}
		if registry, associated := r.customElementShadowRegistries[root]; associated && registry != nil && registry.isScoped {
			r.associateScopedRegistryWithDocument(registry, target)
		}
	}
	shadowRoot := r.shadowRootForNode(node)
	if shadowRoot != nil && shadowRoot.CustomElementRegistryNullDeclarative && r.customElementRegistryForShadowRoot(shadowRoot) == nil {
		registry, associated := r.customElementNodeRegistries[node]
		if !associated || registry == nil && r.customElementNullRegistryLocks[node] {
			// The node inherits the declarative root's null association, or has
			// an explicit null association. A non-null per-node association is
			// independent and still follows its own adoption rules.
			return
		}
	}
	if r.customElementNullRegistryLocks[node] {
		// A null association inherited inside a scoped tree remains null. In
		// other contexts, cross-document adoption recomputes the effective
		// registry from the destination document.
		var parentRegistry *CustomElementRegistry
		if shadowRoot != nil {
			parentRegistry = r.customElementRegistryForShadowRoot(shadowRoot)
		}
		if parentRegistry == nil {
			parentRegistry = r.customElementRegistryForNode(node.Parent)
		}
		if parentRegistry != nil && parentRegistry.isScoped {
			return
		}
		delete(r.customElementNullRegistryLocks, node)
	}
	if registry, associated := r.customElementNodeRegistries[node]; associated && registry != nil && registry.isScoped {
		r.associateScopedRegistryWithDocument(registry, target)
		return
	}
	if r.customElementNodeRegistries == nil {
		r.customElementNodeRegistries = make(map[*dom.Node]*CustomElementRegistry)
	}
	newRegistry := r.customElementRegistryForDocument(target)
	r.customElementNodeRegistries[node] = newRegistry
}

func (r *Runtime) customElementRegistryObject(registry *CustomElementRegistry) goja.Value {
	if registry == nil || registry.object == nil {
		return goja.Null()
	}
	return registry.object
}

func (r *Runtime) customElementRegistryForDocument(doc *dom.Document) *CustomElementRegistry {
	if doc == nil {
		return nil
	}
	// The active Document always uses this browsing context's current global
	// registry. Associations in customElementDocumentRegistries preserve the
	// registry of replaced Documents after iframe navigation.
	if doc == r.domDoc {
		return r.customElementRegistry
	}
	if registry, associated := r.customElementDocumentRegistries[doc]; associated {
		return registry
	}
	if frame := r.iframeForDocument(doc); frame != nil && r.iframeRegistries != nil {
		return r.iframeRegistries[frame]
	}
	return nil
}

func (r *Runtime) associateScopedRegistryWithDocument(registry *CustomElementRegistry, doc *dom.Document) {
	if registry == nil || !registry.isScoped || doc == nil {
		return
	}
	registry.mu.Lock()
	if registry.scopedDocs == nil {
		registry.scopedDocs = make(map[*dom.Document]struct{})
	}
	if _, exists := registry.scopedDocs[doc]; !exists {
		registry.scopedDocs[doc] = struct{}{}
		registry.scopedDocumentOrder = append(registry.scopedDocumentOrder, doc)
	}
	registry.mu.Unlock()
}

func (r *Runtime) associateParsedSubtreeWithRegistry(root *dom.Node, registry *CustomElementRegistry, preserveNull bool) {
	if root == nil {
		return
	}
	if r.customElementNodeRegistries == nil {
		r.customElementNodeRegistries = make(map[*dom.Node]*CustomElementRegistry)
	}
	var visit func(*dom.Node, *CustomElementRegistry, bool, bool)
	visit = func(node *dom.Node, currentRegistry *CustomElementRegistry, currentNull, lockedNull bool) {
		if node == nil {
			return
		}
		if node.Element() {
			if node.HasAttribute("customelementregistry") {
				currentRegistry = nil
				currentNull = true
				lockedNull = true
			}
			if currentRegistry != nil || currentNull {
				r.customElementNodeRegistries[node] = currentRegistry
			}
			if lockedNull {
				r.customElementNullRegistryLocks[node] = true
			}
			if definition := r.customElementDefinitionForNodeInRegistry(node, currentRegistry); definition != nil {
				r.upgradeElementInRegistry(node, definition, false, false, currentRegistry)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child, currentRegistry, currentNull, lockedNull)
		}
	}
	visit(root, registry, preserveNull, false)
}

// jsCustomElementsInitialize associates a registry with a root and its
// inclusive descendants whose registry is still null.
func (r *Runtime) jsCustomElementsInitialize(call goja.FunctionCall, registry *CustomElementRegistry) goja.Value {
	if len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'initialize' on 'CustomElementRegistry': 1 argument required"))
	}
	value := call.Argument(0)
	rootNode := r.getNode(value)
	host, shadowRoot := r.shadowRootFromValue(value)
	isDocument := rootNode != nil && rootNode.Type == dom.NodeDocument
	if rootNode == nil && shadowRoot == nil {
		panic(r.vm.NewTypeError("Failed to execute 'initialize' on 'CustomElementRegistry': parameter 1 is not of type 'Node'"))
	}
	rootDocument := (*dom.Document)(nil)
	if rootNode != nil {
		rootDocument = rootNode.Doc
	} else if host != nil {
		rootDocument = host.Doc
	}
	if !registry.isScoped && (isDocument || r.customElementRegistryForDocument(rootDocument) != registry) {
		panic(r.domError("NotSupportedError", "This registry cannot initialize the supplied root"))
	}
	if isDocument {
		if r.customElementRegistryForDocument(rootDocument) == nil {
			r.customElementDocumentRegistries[rootDocument] = registry
		}
	} else if shadowRoot != nil {
		if r.customElementRegistryForShadowRoot(shadowRoot) == nil {
			r.customElementShadowRegistries[shadowRoot] = registry
		}
	} else if rootNode.Element() && r.customElementRegistryForNode(rootNode) == nil {
		r.customElementNodeRegistries[rootNode] = registry
	}
	r.associateScopedRegistryWithDocument(registry, rootDocument)

	var initializeNode func(*dom.Node)
	initializeNode = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Element() {
			current := r.customElementRegistryForNode(node)
			if current == nil {
				r.customElementNodeRegistries[node] = registry
				delete(r.customElementNullRegistryLocks, node)
			} else if _, associated := r.customElementNodeRegistries[node]; !associated {
				// Materialize an inherited association so it survives later moves
				// out of the root that supplied it.
				r.customElementNodeRegistries[node] = current
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			initializeNode(child)
		}
	}
	if rootNode != nil {
		initializeNode(rootNode)
	}
	if shadowRoot != nil {
		for child := shadowRoot.FirstChild; child != nil; child = child.NextSibling {
			initializeNode(child)
		}
	}
	r.jsCustomElementsUpgrade(goja.FunctionCall{Arguments: []goja.Value{value}}, registry)
	return goja.Undefined()
}

func (r *Runtime) upgradeElementInRegistry(node *dom.Node, definition *customElementDefinition, synchronous, unknownFallback bool, registry *CustomElementRegistry) *dom.Node {
	previous := r.customElementConstructionRegistry
	r.customElementConstructionRegistry = registry
	defer func() { r.customElementConstructionRegistry = previous }()
	return r.upgradeElement(node, definition, synchronous, unknownFallback, false, nil)
}

func (r *Runtime) upgradeParserElementInRegistry(node *dom.Node, definition *customElementDefinition, registry *CustomElementRegistry, ownerDocument *dom.Document) *dom.Node {
	previous := r.customElementConstructionRegistry
	r.customElementConstructionRegistry = registry
	defer func() { r.customElementConstructionRegistry = previous }()
	return r.upgradeElement(node, definition, false, true, true, ownerDocument)
}

func (r *Runtime) customElementDefinitionForNodeInRegistry(node *dom.Node, registry *CustomElementRegistry) *customElementDefinition {
	if node == nil || registry == nil || node.Namespace != dom.NSHTML {
		return nil
	}
	name := node.GetAttribute("is")
	if candidate, exists := r.customElementCandidates[node]; exists {
		name = candidate
	}
	if def := r.customElementDefinitionIn(registry, name); def != nil && name != "" && def.extends == node.Data {
		return def
	}
	if def := r.customElementDefinitionIn(registry, node.Data); def != nil && def.extends == "" {
		return def
	}
	return nil
}

func (r *Runtime) customElementDefinitionFromSet(node *dom.Node, definitions map[string]*customElementDefinition) *customElementDefinition {
	if node == nil || node.Namespace != dom.NSHTML {
		return nil
	}
	name := node.GetAttribute("is")
	if candidate, exists := r.customElementCandidates[node]; exists {
		name = candidate
	}
	if def := definitions[name]; def != nil && name != "" && def.extends == node.Data {
		return def
	}
	if def := definitions[node.Data]; def != nil && def.extends == "" {
		return def
	}
	return nil
}

func (r *Runtime) documentHasBrowsingContext(doc *dom.Document) bool {
	if doc == nil {
		return false
	}
	if doc == r.domDoc {
		return true
	}
	for frame, registry := range r.iframeRegistries {
		if registry != nil && registry.document == doc && frame.IsConnected() {
			return true
		}
	}
	return false
}

func (r *Runtime) notifyCustomElementAttributeChanged(node *dom.Node, name, oldValue string, hadOld bool, newValue string, hadNew bool, namespace string) {
	r.markRadioGroupAttributeMutation(node, name, namespace)
	r.reconcileDirtyRadioGroups()
	for _, constructing := range r.customConstructionNodes {
		if constructing == node {
			return
		}
	}
	if namespace == "" && node != nil {
		if node.FormAssociatedCustomElement && strings.EqualFold(name, "form") {
			r.refreshFormAssociatedState(node)
		} else if node.Data == "form" && strings.EqualFold(name, "id") {
			r.refreshFormAssociatedStates(node.Doc)
		}
	}
	if namespace == "" && strings.EqualFold(name, "disabled") && node != nil {
		if node.Data == "fieldset" {
			r.refreshFormDisabledStateSubtree(node)
		} else if node.FormAssociatedCustomElement {
			r.refreshFormDisabledState(node)
		}
	}
	r.mu.Lock()
	def := r.customElementInstances[node]
	r.mu.Unlock()
	if def == nil {
		def = r.customElementDefinitionForNode(node)
	}
	if def == nil || !containsCustomAttribute(def.observed, name) || !r.isCustomElementInstance(node, def) {
		return
	}
	callback, ok := goja.AssertFunction(def.attributeChangedCallback)
	if !ok {
		return
	}
	oldJS, newJS, namespaceJS := goja.Null(), goja.Null(), goja.Null()
	if hadOld {
		oldJS = r.vm.ToValue(oldValue)
	}
	if hadNew {
		newJS = r.vm.ToValue(newValue)
	}
	if namespace != "" {
		namespaceJS = r.vm.ToValue(namespace)
	}
	reaction := func() {
		_, _ = callback(r.wrapNode(node), r.vm.ToValue(name), oldJS, newJS, namespaceJS)
	}
	if batch := r.customElementReactionBatch; batch != nil {
		batch.enqueue(node, reaction)
		if batch.current != node {
			batch.process(node)
		}
		return
	}
	reaction()
}

// jsCustomElementsWhenDefined implements customElements.whenDefined(name).
func (r *Runtime) jsCustomElementsWhenDefined(call goja.FunctionCall, registry *CustomElementRegistry) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'whenDefined' on 'CustomElementRegistry': 1 argument required"))
	}
	name := call.Arguments[0].String()
	promise := r.newPromiseObj()
	if !isValidCustomElementName(name) {
		r.rejectPromise(promise, r.customElementNameException("SyntaxError", "The name provided is not a valid custom element name."))
		return promise.obj
	}

	registry.mu.Lock()
	if def, exists := registry.definitions[name]; exists {
		registry.mu.Unlock()
		r.resolvePromise(promise, def.value)
		return promise.obj
	}
	if pending := registry.whenDefined[name]; pending != nil {
		registry.mu.Unlock()
		return pending.obj
	}
	registry.whenDefined[name] = promise
	registry.mu.Unlock()
	return promise.obj
}

func (r *Runtime) customElementNameException(name, message string) goja.Value {
	if constructor, ok := goja.AssertConstructor(r.vm.Get("DOMException")); ok {
		if exception, err := constructor(nil, r.vm.ToValue(message), r.vm.ToValue(name)); err == nil {
			return exception
		}
	}
	return namedError(r, name, message)
}

func (r *Runtime) customElementDefinitionException(name, message string) goja.Value {
	return r.customElementNameException(name, message)
}

// upgradeCustomElements upgrades existing elements with the given tag name.
func (r *Runtime) upgradeCustomElements(name string, registry *CustomElementRegistry, doc *dom.Document) {
	if doc == nil {
		return
	}
	def := r.customElementDefinitionIn(registry, name)
	if def == nil {
		return
	}

	// Walk the DOM and upgrade matching elements, including customized built-in
	// candidates created before their definition was registered.
	var upgrade func(n *dom.Node)
	upgrade = func(n *dom.Node) {
		if n.Type == dom.NodeElement && r.customElementDefinitionFromSet(n, map[string]*customElementDefinition{name: def}) == def && r.customElementRegistryForNode(n) == registry {
			if !r.isCustomElementInstance(n, def) && !r.isFailedCustomElement(n) {
				wasConnected := n.IsConnected()
				r.upgradeElementInRegistry(n, def, false, false, registry)
				if wasConnected && r.isCustomElementInstance(n, def) {
					r.invokeConnectedCallback(n)
				}
			}
		}
		if n.ShadowRoot != nil {
			for child := n.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
				upgrade(child)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			upgrade(c)
		}
	}

	// Start from document's HTML element
	if doc.HTML != nil {
		upgrade(doc.HTML)
	}
}

func (r *Runtime) customElementDefinition(name string) *customElementDefinition {
	return r.customElementDefinitionIn(r.customElementRegistry, name)
}

func (r *Runtime) customElementDefinitionIn(registry *CustomElementRegistry, name string) *customElementDefinition {
	if registry == nil {
		return nil
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return registry.definitions[name]
}

// allocateDirectCustomElement backs `new MyElement()` when the derived
// constructor reaches HTMLElement's base constructor outside an upgrade.
// The instance's prototype identifies the registered custom element.
func (r *Runtime) customElementDefinitionForConstructor(constructor *goja.Object) (*customElementDefinition, *CustomElementRegistry) {
	if constructor == nil {
		return nil, nil
	}
	seen := make(map[*goja.Object]bool)
	for r.iframeImportedFunctionAliases[constructor] != nil && !seen[constructor] {
		seen[constructor] = true
		constructor = r.iframeImportedFunctionAliases[constructor]
	}
	registries := []*CustomElementRegistry{r.customElementConstructionRegistry, r.customElementRegistry}
	for _, registry := range r.iframeRegistries {
		registries = append(registries, registry)
	}
	for _, registry := range registries {
		if registry == nil {
			continue
		}
		registry.mu.Lock()
		for _, candidate := range registry.definitions {
			if candidate.value != nil && candidate.value.StrictEquals(constructor) {
				registry.mu.Unlock()
				return candidate, registry
			}
		}
		registry.mu.Unlock()
	}
	return nil, nil
}

func (r *Runtime) isRegisteredCustomElementPrototype(prototype *goja.Object) bool {
	if prototype == nil {
		return false
	}
	registries := []*CustomElementRegistry{r.customElementRegistry}
	for _, registry := range r.iframeRegistries {
		registries = append(registries, registry)
	}
	for _, registry := range registries {
		if registry == nil {
			continue
		}
		registry.mu.Lock()
		for _, candidate := range registry.definitions {
			if candidate.prototype == prototype {
				registry.mu.Unlock()
				return true
			}
		}
		registry.mu.Unlock()
	}
	return false
}

func (r *Runtime) customElementConstructorMatchesInterface(def *customElementDefinition, interfaceName string) (matches, mismatch bool) {
	if def == nil || def.prototype == nil {
		return false, false
	}
	expected := r.htmlElementProto
	if def.extends != "" {
		expectedInterface := htmlTagInterfaces[def.extends]
		if expectedInterface == "" {
			expectedInterface = "HTMLElement"
		}
		if expectedInterface != interfaceName {
			return false, true
		}
		r.protoForTag(def.extends)
		if ctor := r.ifaceCtors[expectedInterface]; ctor != nil {
			if prototype, ok := ctor.Get("prototype").(*goja.Object); ok {
				expected = prototype
			}
		}
	} else if interfaceName != "HTMLElement" {
		return false, true
	}
	for prototype := def.prototype.Prototype(); prototype != nil; prototype = prototype.Prototype() {
		if prototype == expected {
			return true, false
		}
		if r.isRegisteredCustomElementPrototype(prototype) {
			continue
		}
		if prototype == r.htmlElementProto || prototype == r.unknownElementProto {
			return false, true
		}
		for _, native := range r.tagProtos {
			if prototype == native {
				return false, true
			}
		}
	}
	return false, false
}

func (r *Runtime) functionRealm(newTarget *goja.Object, fallback *Runtime) *Runtime {
	if newTarget == nil {
		if fallback != nil {
			return fallback
		}
		return r
	}
	// WindowProxy wrappers can be layered (for example, when a constructor is
	// imported through more than one iframe). A Proxy's GetFunctionRealm is the
	// realm of its target, so follow imported aliases and prefer the innermost
	// source realm over the wrapper runtime's own realm.
	seen := make(map[*goja.Object]bool)
	explicitRealm := false
	for current := newTarget; current != nil && !seen[current]; {
		seen[current] = true
		if source, ok := r.iframeParentObjects[current].(*goja.Object); ok && r.iframeParent != nil {
			fallback = r.iframeParent.functionRealm(source, r.iframeParent)
			explicitRealm = true
		}
		if realm := r.iframeImportedFunctionRealms[current]; realm != nil {
			fallback = realm
			explicitRealm = true
		}
		next := r.iframeImportedFunctionAliases[current]
		if next == nil {
			if proxy, ok := current.Export().(goja.Proxy); ok {
				next = proxy.Target()
			}
		}
		if next != nil && !explicitRealm && r.iframeImportedFunctionRealms[next] == nil {
			if realm := r.runtimeForFunctionObject(next); realm != nil {
				fallback = realm
			}
		}
		current = next
	}
	if fallback == nil {
		fallback = r
	}
	// Proxy [[GetPrototypeOf]] would identify the proxy target's Function
	// prototype only in a single Goja runtime. Across iframe runtimes, imported
	// callable wrappers retain their source realm in the map above; proxies over
	// those wrappers inherit the registered constructor's realm as fallback.
	return fallback
}

func (r *Runtime) runtimeForFunctionObject(function *goja.Object) *Runtime {
	if function == nil {
		return nil
	}
	functionPrototype := function.Prototype()
	if functionPrototype == nil {
		return nil
	}
	root := r
	for root.iframeParent != nil {
		root = root.iframeParent
	}
	seen := make(map[*Runtime]bool)
	stack := []*Runtime{root}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == nil || seen[current] {
			continue
		}
		seen[current] = true
		if constructor, ok := current.vm.Get("Function").(*goja.Object); ok {
			if prototype, ok := constructor.Get("prototype").(*goja.Object); ok && functionPrototype == prototype {
				return current
			}
		}
		for _, child := range current.iframeChildRuntimes {
			stack = append(stack, child)
		}
	}
	return nil
}

func (r *Runtime) applyHTMLConstructorFallbackPrototype(instance *goja.Object, def *customElementDefinition, realm *Runtime, primitivePrototype bool) {
	if instance == nil || def == nil {
		return
	}
	if realm == nil {
		realm = r
	}
	if !primitivePrototype {
		objectCtor, ok := r.vm.Get("Object").(*goja.Object)
		if !ok {
			return
		}
		objectPrototype, ok := objectCtor.Get("prototype").(*goja.Object)
		if !ok || instance.Prototype() != objectPrototype {
			return
		}
	}
	fallback := realm.htmlElementProto
	if def.extends != "" {
		fallback = realm.protoForTag(def.extends)
	}
	if fallback != nil {
		fallbackPrototype := fallback
		if realm != r {
			imported, ok := r.importParentValue(realm, fallback).(*goja.Object)
			if !ok {
				return
			}
			fallbackPrototype = imported
		}
		_ = instance.SetPrototype(fallbackPrototype)
	}
}

func (r *Runtime) allocateDirectCustomElement(instance *goja.Object, def *customElementDefinition, ownerRegistry *CustomElementRegistry) bool {
	if instance == nil || def == nil {
		return false
	}
	var doc *dom.Document
	if ownerRegistry != nil {
		doc = ownerRegistry.document
	}
	if doc == nil {
		doc = r.domDoc
	}
	if doc == nil {
		return false
	}
	tagName := def.name
	if def.extends != "" {
		tagName = def.extends
	}
	node := doc.NewElement(tagName)
	node.Namespace = dom.NSHTML
	node.NamespaceURI = "http://www.w3.org/1999/xhtml"
	if def.extends != "" {
		node.SetAttribute("is", def.name)
	}
	r.mu.Lock()
	nid := r.nextNID
	r.nextNID++
	r.nodeRegistry[nid] = node
	r.nodeWrappers[nid] = instance
	r.wrapCache[node] = nid
	_ = instance.Set("__nid__", nid)
	r.mu.Unlock()
	r.markCustomElementInstance(node, def)
	return true
}

// UpgradeParserElement applies an already-registered definition before the
// tree builder inserts a parser-created element.
func (r *Runtime) UpgradeParserElement(n *dom.Node) {
	r.UpgradeParserElementInDocument(n, nDocument(n))
}

func nDocument(node *dom.Node) *dom.Document {
	if node == nil {
		return nil
	}
	return node.Doc
}

// UpgradeParserElementInDocument upgrades a parser-created node while
// accounting synchronous reactions against its eventual owner document. Open
// document.write reparses into a temporary Document before adopting the tree.
func (r *Runtime) UpgradeParserElementInDocument(n *dom.Node, ownerDocument *dom.Document) *dom.Node {
	if n == nil || n.Type != dom.NodeElement {
		return n
	}
	if ownerDocument != nil && n.Doc != ownerDocument {
		// Fragment and document.write parsers build tokens in a temporary
		// Document. The HTML create-an-element-for-the-token algorithm still
		// constructs the element for its eventual owner document, so establish
		// that owner before the constructor runs.
		n.AdoptInto(ownerDocument)
	}
	if n.HasAttribute("customelementregistry") {
		r.associateParserElementRegistry(n)
	}
	if id := n.GetAttribute("id"); id != "" {
		r.installWindowNamedProperty(id)
	}
	for _, attr := range n.Attr {
		if attr.Namespace == dom.NSHTML && attr.NamespaceURI == "" {
			r.syncEventHandlerContentAttribute(n, attr.Name, attr.Value, true)
		}
	}
	if def := r.customElementDefinitionForNode(n); def != nil {
		if r.isCustomElementInstance(n, def) {
			return n
		}
		// HTML's create-an-element-for-the-token algorithm checkpoints before
		// construction only when the JavaScript stack is empty. A synchronous
		// document.write() from script still has an active stack and must leave
		// queued promise and mutation-observer jobs for its caller's checkpoint.
		if r.scriptExecutionDepth == 0 {
			// A reported exception from a queued job does not abort the parser's
			// create-an-element-for-the-token algorithm. The checkpoint runs
			// before construction; it is not part of the constructor's result.
			_ = r.drainMicrotasks()
		}
		replacement := n
		replacement = r.upgradeParserElementInRegistry(n, def, r.customElementRegistryForNode(n), ownerDocument)
		return replacement
	}
	return n
}

// UpgradeParserElementWithAttributes makes the token's customized built-in
// candidate available before construction while leaving the parsed attributes
// off the element until the tree builder's attribute-processing step.
func (r *Runtime) UpgradeParserElementWithAttributes(n *dom.Node, attrs []dom.Attribute) *dom.Node {
	return r.UpgradeParserElementWithAttributesInDocument(n, attrs, nDocument(n))
}

func (r *Runtime) UpgradeParserElementWithAttributesInDocument(n *dom.Node, attrs []dom.Attribute, ownerDocument *dom.Document) *dom.Node {
	if n == nil {
		return nil
	}
	for _, attr := range attrs {
		if attr.Namespace == dom.NSHTML && attr.NamespaceURI == "" && strings.EqualFold(attr.Name, "is") {
			if r.customElementCandidates == nil {
				r.customElementCandidates = make(map[*dom.Node]string)
			}
			r.customElementCandidates[n] = attr.Value
			break
		}
	}
	return r.UpgradeParserElementInDocument(n, ownerDocument)
}

// ParserElementAttributeChanged queues the parser-time attribute reaction
// after the parser has constructed the element and attached the token's
// attribute. The element may still be waiting for its insertion step.
func (r *Runtime) ParserElementAttributeChanged(node *dom.Node, attr dom.Attribute) {
	r.ParserElementAttributeChangedInDocument(node, attr, nDocument(node))
}

func (r *Runtime) ParserElementAttributeChangedInDocument(node *dom.Node, attr dom.Attribute, ownerDocument *dom.Document) {
	if node == nil {
		return
	}
	r.withDynamicMarkupInsertionCounter(ownerDocument, func() {
		if attr.Namespace == dom.NSHTML && attr.NamespaceURI == "" {
			switch strings.ToLower(attr.Name) {
			case "id":
				if attr.Value != "" {
					r.installWindowNamedProperty(attr.Value)
				}
			case "name":
				if node.Element() && node.Data == "iframe" && attr.Value != "" {
					r.installWindowNamedProperty(attr.Value)
				}
			case "customelementregistry":
				r.associateParserElementRegistry(node)
			}
			if strings.HasPrefix(strings.ToLower(attr.Name), "on") {
				r.syncEventHandlerContentAttribute(node, attr.Name, attr.Value, true)
			}
		}
		r.notifyCustomElementAttributeChanged(node, domAttributeLocalName(attr.Name), "", false, attr.Value, true, attr.NamespaceURI)
	})
}

func (r *Runtime) withDynamicMarkupInsertionCounter(doc *dom.Document, run func()) {
	if run == nil {
		return
	}
	if doc == nil {
		run()
		return
	}
	if r.dynamicMarkupInsertionCounters == nil {
		r.dynamicMarkupInsertionCounters = make(map[*dom.Document]int)
	}
	r.dynamicMarkupInsertionCounters[doc]++
	defer func() {
		r.dynamicMarkupInsertionCounters[doc]--
		if r.dynamicMarkupInsertionCounters[doc] == 0 {
			delete(r.dynamicMarkupInsertionCounters, doc)
		}
	}()
	run()
}

// ParserElementInserted runs the connected reaction after tree insertion.
func (r *Runtime) ParserElementInserted(n *dom.Node) {
	r.parserElementInserted(n, nDocument(n), false)
}

// ParserNodeInserted records the parser's insertion step for MutationObserver
// and then applies custom-element connection behavior for element nodes.
func (r *Runtime) ParserNodeInserted(n *dom.Node) {
	r.parserNodeInserted(n, nDocument(n), false)
}

func (r *Runtime) ParserNodeInsertedInDocument(n *dom.Node, ownerDocument *dom.Document) {
	r.parserNodeInserted(n, ownerDocument, true)
}

func (r *Runtime) parserNodeInserted(n *dom.Node, ownerDocument *dom.Document, useOwnerRegistry bool) {
	if n == nil || n.Parent == nil {
		return
	}
	r.queueMutationRecord(MutationRecord{
		Type:            "childList",
		Target:          n.Parent,
		AddedNodes:      []*dom.Node{n},
		PreviousSibling: n.PrevSibling,
		NextSibling:     n.NextSibling,
	})
	if n.Element() {
		if useOwnerRegistry {
			r.ParserElementInsertedInDocument(n, ownerDocument)
		} else {
			r.ParserElementInserted(n)
		}
	}
}

func (r *Runtime) ParserElementInsertedInDocument(n *dom.Node, ownerDocument *dom.Document) {
	r.parserElementInserted(n, ownerDocument, true)
}

func (r *Runtime) parserElementInserted(n *dom.Node, ownerDocument *dom.Document, useOwnerRegistry bool) {
	if n == nil {
		return
	}
	r.queueInitialDetailsToggleForNode(n)
	r.associateParserElementRegistry(n)
	if useOwnerRegistry && !n.HasAttribute("customelementregistry") && !nodeInDeclarativeShadowTemplate(n) {
		if r.customElementNodeRegistries == nil {
			r.customElementNodeRegistries = make(map[*dom.Node]*CustomElementRegistry)
		}
		r.customElementNodeRegistries[n] = r.customElementRegistryForDocument(ownerDocument)
	}
	def := r.customElementDefinitionForNode(n)
	if def != nil && r.isCustomElementInstance(n, def) && r.isInDocument(n) {
		r.withDynamicMarkupInsertionCounter(ownerDocument, func() { r.invokeConnectedCallback(n) })
	}
	if n.Data == "form" {
		r.refreshFormAssociatedStates(n.Doc)
	}
}

func (r *Runtime) associateParserElementRegistry(node *dom.Node) {
	if node == nil || !node.Element() {
		return
	}
	if r.customElementNodeRegistries == nil {
		r.customElementNodeRegistries = make(map[*dom.Node]*CustomElementRegistry)
	}
	if node.HasAttribute("customelementregistry") {
		r.customElementNodeRegistries[node] = nil
		r.customElementNullRegistryLocks[node] = true
		return
	}
	if nodeInDeclarativeShadowTemplate(node) {
		// These nodes are parsed in the template's inert contents first, then
		// moved into the declarative ShadowRoot when its end tag is processed.
		// Do not freeze the temporary null association from the inert fragment;
		// once moved, their effective registry comes from the ShadowRoot.
		return
	}
	if node.Parent == nil {
		return
	}
	if registry, associated := r.customElementNodeRegistries[node.Parent]; associated {
		r.customElementNodeRegistries[node] = registry
		if r.customElementNullRegistryLocks[node.Parent] {
			r.customElementNullRegistryLocks[node] = true
		}
		return
	}
	r.customElementNodeRegistries[node] = r.customElementRegistryForNode(node.Parent)
}

func nodeInDeclarativeShadowTemplate(node *dom.Node) bool {
	for current := node; current != nil; current = current.Parent {
		var template *dom.Node
		if current.IsHTMLTemplate() {
			template = current
		} else if current.Type == dom.NodeDocumentFragment {
			template = current.TemplateHost
		}
		if template != nil && template.IsHTMLTemplate() && (template.HasAttribute("shadowrootmode") || template.HasAttribute("shadowroot")) {
			return true
		}
	}
	return false
}

// upgradeElement constructs one custom element and delivers parser-time
// attributeChangedCallback reactions before insertion.
func (r *Runtime) upgradeElement(n *dom.Node, def *customElementDefinition, synchronous, unknownFallback, parserCreated bool, parserOwnerDocument *dom.Document) *dom.Node {
	if n == nil || def == nil || r.isFailedCustomElement(n) {
		return n
	}
	// A failed construction leaves the pre-created element with its native
	// fallback interface from the element's own realm. The custom constructor's
	// prototype can belong to a different Window when a definition is adopted
	// into another document, and must not become the failed element's prototype.
	fallbackPrototype := r.unknownElementProto
	fallbackInterface := "HTMLUnknownElement"
	if def.extends != "" {
		fallbackPrototype = r.protoForTag(def.extends)
		if name := htmlTagInterfaces[def.extends]; name != "" {
			fallbackInterface = name
		}
	}
	if framePrototype := r.iframeInterfacePrototype(n.Doc, fallbackInterface); framePrototype != nil {
		fallbackPrototype = framePrototype
	}
	if def.disableShadow && n.ShadowRoot != nil {
		wrapped, _ := r.wrapNode(n).(*goja.Object)
		r.failCustomElementConstruction(n, wrapped, r.customElementDefinitionException("NotSupportedError", "This custom element definition disables shadow roots"), r.protoForNode(n))
		return n
	}
	ownerDocument := n.Doc
	if parserOwnerDocument != nil {
		ownerDocument = parserOwnerDocument
	}
	initialAttributes := append([]dom.Attribute(nil), n.Attr...)
	wrapped := r.wrapNode(n)
	old, ok := wrapped.(*goja.Object)
	if !ok {
		return n
	}
	nid := int(old.Get("__nid__").ToInteger())
	r.customHTMLElementConstruction++
	r.customConstructionNodes = append(r.customConstructionNodes, n)
	r.customConstructionAlready = append(r.customConstructionAlready, false)
	r.customConstructionSynchronous = append(r.customConstructionSynchronous, synchronous)
	r.customConstructionCrossRealmException = append(r.customConstructionCrossRealmException, false)
	r.customConstructionCrossRealmErrors = append(r.customConstructionCrossRealmErrors, nil)
	r.customConstructionCrossRealmErrorRuntimes = append(r.customConstructionCrossRealmErrorRuntimes, nil)
	r.customConstructionAllowAdditional = append(r.customConstructionAllowAdditional, synchronous || parserCreated)
	var instance *goja.Object
	var err error
	construct := func() { instance, err = def.constructor(nil) }
	if parserCreated {
		r.withDynamicMarkupInsertionCounter(ownerDocument, construct)
	} else {
		construct()
	}
	constructorCalledSuper := len(r.customConstructionAlready) > 0 && r.customConstructionAlready[len(r.customConstructionAlready)-1]
	constructorExceptionReported := len(r.customConstructionCrossRealmException) > 0 && r.customConstructionCrossRealmException[len(r.customConstructionCrossRealmException)-1]
	var deferredCrossRealmError error
	var deferredCrossRealmRuntime *Runtime
	if len(r.customConstructionCrossRealmErrors) > 0 {
		deferredCrossRealmError = r.customConstructionCrossRealmErrors[len(r.customConstructionCrossRealmErrors)-1]
	}
	if len(r.customConstructionCrossRealmErrorRuntimes) > 0 {
		deferredCrossRealmRuntime = r.customConstructionCrossRealmErrorRuntimes[len(r.customConstructionCrossRealmErrorRuntimes)-1]
	}
	r.customConstructionNodes = r.customConstructionNodes[:len(r.customConstructionNodes)-1]
	r.customConstructionAlready = r.customConstructionAlready[:len(r.customConstructionAlready)-1]
	r.customConstructionSynchronous = r.customConstructionSynchronous[:len(r.customConstructionSynchronous)-1]
	r.customConstructionCrossRealmException = r.customConstructionCrossRealmException[:len(r.customConstructionCrossRealmException)-1]
	r.customConstructionCrossRealmErrors = r.customConstructionCrossRealmErrors[:len(r.customConstructionCrossRealmErrors)-1]
	r.customConstructionCrossRealmErrorRuntimes = r.customConstructionCrossRealmErrorRuntimes[:len(r.customConstructionCrossRealmErrorRuntimes)-1]
	r.customConstructionAllowAdditional = r.customConstructionAllowAdditional[:len(r.customConstructionAllowAdditional)-1]
	r.customHTMLElementConstruction--
	// Parser construction does not use the synchronous-custom-elements checks
	// from createElement(). Still, the parser performs a microtask checkpoint
	// immediately after construction when the JS stack is empty. Preserve
	// constructor mutations, but detect mutations made by those jobs.
	constructorAttributes := append([]dom.Attribute(nil), n.Attr...)
	if r.scriptExecutionDepth == 0 {
		// A parser-triggered constructor can run with an empty JavaScript
		// stack. In that case, the HTML construction algorithm checkpoints
		// immediately after the constructor and before validating its result.
		_ = r.drainMicrotasks()
	}
	if err != nil {
		var errorValue goja.Value = r.vm.ToValue(err)
		if exception, ok := err.(*goja.Exception); ok {
			errorValue = exception.Value()
		}
		if constructorCalledSuper && (parserCreated || !synchronous) {
			// An upgrade starts with an existing element. Keep the custom
			// prototype installed by the construction stack when its constructor
			// throws; only synchronous createElement() uses the native fallback.
			fallbackPrototype = old.Prototype()
		}
		if constructorExceptionReported {
			r.failCustomElementConstruction(n, old, nil, fallbackPrototype)
			if deferredCrossRealmError != nil && deferredCrossRealmRuntime != nil {
				deferredCrossRealmRuntime.reportWindowException(deferredCrossRealmError)
			}
		} else {
			r.failCustomElementConstructionInRealm(n, old, errorValue, fallbackPrototype, def.constructorRealm)
		}
		return n
	}
	if instance == nil {
		r.failCustomElementConstructionInRealm(n, old, r.vm.NewTypeError("Custom element constructor returned no element"), fallbackPrototype, def.constructorRealm)
		return n
	}
	instanceIDValue := instance.Get("__nid__")
	if instanceIDValue == nil || goja.IsUndefined(instanceIDValue) || goja.IsNull(instanceIDValue) {
		if constructorExceptionReported {
			r.failCustomElementConstruction(n, old, nil, fallbackPrototype)
			if deferredCrossRealmError != nil && deferredCrossRealmRuntime != nil {
				deferredCrossRealmRuntime.reportWindowException(deferredCrossRealmError)
			}
		} else {
			r.failCustomElementConstructionInRealm(n, old, r.vm.NewTypeError("Custom element constructor did not return the element created by HTMLElement"), fallbackPrototype, def.constructorRealm)
		}
		return n
	}
	instanceNodeID := int(instanceIDValue.ToInteger())
	if r.nodeRegistry[instanceNodeID] != n {
		if returnedNode := r.nodeRegistry[instanceNodeID]; returnedNode != nil && returnedNode.Type == dom.NodeElement {
			if parserCreated && r.isCustomElementInstance(returnedNode, def) {
				return returnedNode
			}
			if synchronous {
				r.failCustomElementConstructionInRealm(n, old, r.customElementDefinitionException("NotSupportedError", "The custom element constructor returned a different element"), fallbackPrototype, def.constructorRealm)
			} else {
				r.failCustomElementConstructionInRealm(n, old, r.vm.NewTypeError("Custom element constructor returned a different element"), fallbackPrototype, def.constructorRealm)
			}
		} else {
			r.failCustomElementConstructionInRealm(n, old, r.vm.NewTypeError("Custom element constructor did not return the element created by HTMLElement"), fallbackPrototype, def.constructorRealm)
		}
		return n
	}
	attributesModified := len(n.Attr) != 0
	if def.extends != "" {
		attributesModified = len(n.Attr) > 1 || len(n.Attr) == 1 && n.GetAttribute("is") != def.name
	}
	parserCheckpointModified := parserCreated && !reflect.DeepEqual(n.Attr, constructorAttributes)
	if parserCheckpointModified {
		n.Attr = append(n.Attr[:0], constructorAttributes...)
		n.Doc.MarkQueryDirty()
	}
	if (synchronous && (n.Doc != ownerDocument || attributesModified || n.FirstChild != nil || n.Parent != nil)) || parserCheckpointModified {
		r.failCustomElementConstructionInRealm(n, old, r.customElementDefinitionException("NotSupportedError", "The custom element constructor modified its element"), fallbackPrototype, def.constructorRealm)
		return n
	}
	r.mu.Lock()
	r.nodeWrappers[nid] = instance
	r.wrapCache[n] = nid
	r.mu.Unlock()
	r.markCustomElementInstance(n, def)
	for _, attr := range initialAttributes {
		if !containsCustomAttribute(def.observed, attr.Name) {
			continue
		}
		if fn, ok := goja.AssertFunction(def.attributeChangedCallback); ok {
			_, _ = fn(instance, r.vm.ToValue(attr.Name), goja.Null(), r.vm.ToValue(attr.Value), goja.Null())
		}
	}
	r.initializeFormDisabledState(n)
	r.initializeFormAssociatedState(n)
	return n
}

func (r *Runtime) failCustomElementConstructionInRealm(node *dom.Node, fallback *goja.Object, errorValue goja.Value, fallbackPrototype *goja.Object, realm *Runtime) {
	if realm == nil || realm == r {
		r.failCustomElementConstruction(node, fallback, errorValue, fallbackPrototype)
		return
	}
	message := "Custom element construction failed"
	if object, ok := errorValue.(*goja.Object); ok {
		if value := object.Get("message"); value != nil && !goja.IsUndefined(value) {
			message = value.String()
		}
	}
	r.failCustomElementConstruction(node, fallback, nil, fallbackPrototype)
	realm.reportWindowException(fmt.Errorf("%s", message))
}

func (r *Runtime) failCustomElementConstruction(node *dom.Node, fallback *goja.Object, errorValue goja.Value, fallbackPrototype *goja.Object) {
	wasUndefined := node != nil && node.CustomElementUndefined
	if node != nil && fallback != nil {
		nid := int(fallback.Get("__nid__").ToInteger())
		if fallbackPrototype == nil {
			fallbackPrototype = r.protoForNode(node)
		}
		_ = fallback.SetPrototype(fallbackPrototype)
		r.mu.Lock()
		r.nodeWrappers[nid] = fallback
		r.wrapCache[node] = nid
		delete(r.customElementInstances, node)
		if r.customElementFailures == nil {
			r.customElementFailures = make(map[*dom.Node]bool)
		}
		r.customElementFailures[node] = true
		r.mu.Unlock()
		node.CustomElementUndefined = false
		if wasUndefined {
			r.notifyMutation()
		}
	}
	if errorValue == nil || r.domDoc == nil {
		return
	}
	message := ""
	if object, ok := errorValue.(*goja.Object); ok {
		if value := object.Get("message"); value != nil && !goja.IsUndefined(value) {
			message = value.String()
		}
	}
	event := dom.NewErrorEvent("error", false, false, message, "", 0, 0, errorValue)
	r.dispatchWithWindowStep(&r.domDoc.Node, &event.Event)
}

func containsCustomAttribute(attributes []string, name string) bool {
	for _, attribute := range attributes {
		if attribute == name {
			return true
		}
	}
	return false
}

// invokeLifecycleCallback invokes a lifecycle callback on a custom element.
func (r *Runtime) invokeLifecycleCallback(n *dom.Node, callback string) {
	// Definitions retain callbacks captured at define() time. Re-reading from
	// the element's prototype here would lose the reaction if script deletes
	// or replaces that property after registration.
	r.mu.Lock()
	def := r.customElementInstances[n]
	var nodeID int
	if id, ok := r.wrapCache[n]; ok {
		nodeID = id
	}
	wrapper := r.nodeWrappers[nodeID]
	r.mu.Unlock()
	if def == nil || wrapper == nil {
		return
	}
	fn, ok := goja.AssertFunction(def.lifecycleCallbacks[callback])
	if !ok {
		return
	}
	_, _ = fn(wrapper)
}

// invokeAdoptedCallbacks runs adopted reactions for a node and its
// shadow-including descendants after their owner document has changed.
func (r *Runtime) invokeAdoptedCallbacks(root *dom.Node, oldDoc, newDoc *dom.Document) {
	if root == nil || oldDoc == nil || newDoc == nil || oldDoc == newDoc {
		return
	}
	oldDocument := r.wrapNode(&oldDoc.Node)
	newDocument := r.wrapNode(&newDoc.Node)
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Type == dom.NodeElement {
			r.updateCustomElementRegistryOnAdoption(node, newDoc)
			r.mu.Lock()
			def := r.customElementInstances[node]
			nid := r.wrapCache[node]
			wrapper := r.nodeWrappers[nid]
			r.mu.Unlock()
			if def != nil && wrapper != nil {
				if fn, ok := goja.AssertFunction(def.lifecycleCallbacks["adoptedCallback"]); ok {
					_, _ = fn(wrapper, oldDocument, newDocument)
				}
			}
			if node.ShadowRoot != nil {
				for child := node.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
					visit(child)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
}

// captureOwnerDocuments snapshots the root document of each inserted subtree.
// DOM insertion adopts a node before the JS binding can dispatch the custom
// element adopted reactions, so callers retain this pre-insertion value.
func captureOwnerDocuments(nodes []*dom.Node) map[*dom.Node]*dom.Document {
	documents := make(map[*dom.Node]*dom.Document, len(nodes))
	for _, node := range nodes {
		if node != nil {
			documents[node] = node.Doc
		}
	}
	return documents
}

func captureConnectedState(nodes []*dom.Node) map[*dom.Node]bool {
	connected := make(map[*dom.Node]bool, len(nodes))
	for _, node := range nodes {
		if node != nil {
			connected[node] = node.IsConnected()
		}
	}
	return connected
}

func (r *Runtime) notifyDisconnected(nodes []*dom.Node, previouslyConnected map[*dom.Node]bool) {
	for _, node := range nodes {
		if node != nil && previouslyConnected[node] {
			r.closePopoverSubtree(node)
		}
	}
	batch, owns := r.beginCustomElementReactionBatch()
	touched := make([]*dom.Node, 0, len(nodes))
	var visit func(*dom.Node, bool)
	visit = func(node *dom.Node, rootWasConnected bool) {
		if node == nil {
			return
		}
		if node.Element() {
			r.mu.Lock()
			connected := r.customElementConnected[node]
			def := r.customElementInstances[node]
			r.mu.Unlock()
			wasConnected := rootWasConnected || connected
			if wasConnected {
				r.mu.Lock()
				if r.customElementConnected == nil {
					r.customElementConnected = make(map[*dom.Node]bool)
				}
				delete(r.customElementConnected, node)
				r.mu.Unlock()
				if def == nil {
					def = r.customElementDefinitionForNode(node)
				}
				if def != nil && hasLifecycleCallback(def, "disconnectedCallback") {
					disconnectedNode := node
					batch.enqueue(node, func() { r.invokeLifecycleCallback(disconnectedNode, "disconnectedCallback") })
					touched = append(touched, node)
				}
			}
			if node.ShadowRoot != nil {
				for child := node.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
					visit(child, rootWasConnected)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child, rootWasConnected)
		}
	}
	for _, node := range nodes {
		if node != nil {
			visit(node, previouslyConnected[node])
		}
	}
	r.finishCustomElementReactionBatch(batch, owns, touched)
	for _, node := range nodes {
		r.refreshFormDisabledStateSubtree(node)
		if node != nil {
			r.refreshFormAssociatedStateSubtree(node)
			if subtreeContainsFormElement(node) {
				r.refreshFormAssociatedStates(node.Doc)
			}
			if node.Data == "legend" {
				r.refreshFormDisabledStates(node.Doc)
			}
		}
	}
}

func (r *Runtime) refreshFormDisabledStates(doc *dom.Document) {
	if doc == nil {
		return
	}
	for _, node := range r.nodeRegistry {
		if node != nil && node.Doc == doc && node.Element() {
			r.refreshFormDisabledState(node)
		}
	}
}

func (r *Runtime) refreshFormDisabledStateSubtree(root *dom.Node) {
	if root == nil {
		return
	}
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Element() {
			r.refreshFormDisabledState(node)
			if node.ShadowRoot != nil {
				for child := node.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
					visit(child)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
}

func (r *Runtime) notifyAdopted(nodes []*dom.Node, previous map[*dom.Node]*dom.Document) {
	batch, owns := r.beginCustomElementReactionBatch()
	touched := make([]*dom.Node, 0, len(nodes))
	var visit func(*dom.Node, *dom.Document)
	visit = func(node *dom.Node, oldDoc *dom.Document) {
		if node == nil {
			return
		}
		if oldDoc == nil {
			oldDoc = previousOwnerDocument(node, nodes, previous)
		}
		if node.Element() {
			if oldDoc != nil && oldDoc != node.Doc {
				r.updateCustomElementRegistryOnAdoption(node, node.Doc)
			}
			if reaction := r.adoptedReaction(node, oldDoc, node.Doc); reaction != nil {
				batch.enqueue(node, reaction)
				touched = append(touched, node)
			}
			if node.ShadowRoot != nil {
				for child := node.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
					visit(child, oldDoc)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child, oldDoc)
		}
	}
	for _, node := range nodes {
		if node != nil {
			visit(node, previous[node])
		}
	}
	r.finishCustomElementReactionBatch(batch, owns, touched)
}

func hasLifecycleCallback(def *customElementDefinition, name string) bool {
	if def == nil {
		return false
	}
	_, ok := goja.AssertFunction(def.lifecycleCallbacks[name])
	return ok
}

func (r *Runtime) adoptedReaction(node *dom.Node, oldDoc, newDoc *dom.Document) func() {
	if node == nil || oldDoc == nil || newDoc == nil || oldDoc == newDoc {
		return nil
	}
	r.mu.Lock()
	def := r.customElementInstances[node]
	nid := r.wrapCache[node]
	wrapper := r.nodeWrappers[nid]
	r.mu.Unlock()
	if def == nil || wrapper == nil {
		return nil
	}
	fn, ok := goja.AssertFunction(def.lifecycleCallbacks["adoptedCallback"])
	if !ok {
		return nil
	}
	oldDocument := r.wrapNode(&oldDoc.Node)
	newDocument := r.wrapNode(&newDoc.Node)
	return func() { _, _ = fn(wrapper, oldDocument, newDocument) }
}

// notifyMovedSubtrees queues adoption and connection reactions by element.
// A nested attribute mutation can then drain the target element's already
// pending reactions before the outer element's queue continues.
func (r *Runtime) notifyMovedSubtrees(nodes []*dom.Node, previous map[*dom.Node]*dom.Document) {
	batch, owns := r.beginCustomElementReactionBatch()
	touched := make([]*dom.Node, 0, len(nodes))
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Type == dom.NodeElement {
			oldDoc := previous[node]
			if oldDoc == nil {
				oldDoc = previousOwnerDocument(node, nodes, previous)
			}
			if oldDoc != nil && oldDoc != node.Doc {
				r.updateCustomElementRegistryOnAdoption(node, node.Doc)
			}
			r.mu.Lock()
			def := r.customElementInstances[node]
			r.mu.Unlock()
			wasCustomElement := def != nil && r.isCustomElementInstance(node, def)
			if def == nil {
				def = r.customElementDefinitionForNode(node)
			}
			if def != nil && !r.isCustomElementInstance(node, def) && !r.isFailedCustomElement(node) {
				r.upgradeElementInRegistry(node, def, false, false, r.customElementRegistryForNode(node))
			}
			if def != nil && r.isCustomElementInstance(node, def) {
				oldDoc := previous[node]
				if oldDoc == nil {
					oldDoc = previousOwnerDocument(node, nodes, previous)
				}
				if wasCustomElement {
					if reaction := r.adoptedReaction(node, oldDoc, node.Doc); reaction != nil {
						batch.enqueue(node, reaction)
						touched = append(touched, node)
					}
				}
				if node.IsConnected() {
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
			if node.ShadowRoot != nil {
				for child := node.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
					visit(child)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	for _, node := range nodes {
		visit(node)
	}
	r.finishCustomElementReactionBatch(batch, owns, touched)
}

func previousOwnerDocument(node *dom.Node, roots []*dom.Node, previous map[*dom.Node]*dom.Document) *dom.Document {
	for _, root := range roots {
		if shadowIncludingContains(root, node) {
			return previous[root]
		}
	}
	return nil
}

func shadowIncludingContains(root, target *dom.Node) bool {
	if root == nil || target == nil {
		return false
	}
	if root == target {
		return true
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if shadowIncludingContains(child, target) {
			return true
		}
	}
	if root.ShadowRoot != nil {
		for child := root.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
			if shadowIncludingContains(child, target) {
				return true
			}
		}
	}
	return false
}

// invokeConnectedCallback records delivery so recursive connection traversal
// cannot invoke the same reaction twice when a callback moves its children.
func (r *Runtime) invokeConnectedCallback(node *dom.Node) {
	if node == nil {
		return
	}
	r.mu.Lock()
	if r.customElementConnected == nil {
		r.customElementConnected = make(map[*dom.Node]bool)
	}
	if r.customElementConnected[node] {
		r.mu.Unlock()
		return
	}
	r.customElementConnected[node] = true
	r.mu.Unlock()
	r.invokeLifecycleCallback(node, "connectedCallback")
}

func (r *Runtime) invokeDisconnectedCallback(node *dom.Node) {
	if node == nil {
		return
	}
	r.mu.Lock()
	connected := r.customElementConnected[node]
	delete(r.customElementConnected, node)
	r.mu.Unlock()
	if connected {
		r.invokeLifecycleCallback(node, "disconnectedCallback")
	}
}

// invokeDisconnectedCallbacks runs disconnected reactions for a removed
// subtree, including custom elements in shadow trees.
func (r *Runtime) invokeDisconnectedCallbacks(root *dom.Node) {
	if root == nil {
		return
	}
	r.notifyDisconnected([]*dom.Node{root}, map[*dom.Node]bool{root: true})
}

// isInDocument checks if a node is in the document tree.
func (r *Runtime) isInDocument(n *dom.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == dom.NodeDocument {
			return true
		}
	}
	return false
}

// isValidCustomElementName checks if a name is valid for a custom element.
func isValidCustomElementName(name string) bool {
	return dom.IsValidCustomElementName(name)
}
