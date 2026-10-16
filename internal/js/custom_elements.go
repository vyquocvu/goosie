package js

import (
	"fmt"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// CustomElementRegistry implements the Custom Elements API.
type CustomElementRegistry struct {
	mu          sync.Mutex
	definitions map[string]*customElementDefinition
	whenDefined map[string][]func()
}

type customElementDefinition struct {
	name        string
	constructor goja.ConstructorCall
	observed    []string
	extends     string
}

// setupCustomElements installs the Custom Elements API on window.
func (r *Runtime) setupCustomElements() {
	registry := &CustomElementRegistry{
		definitions: make(map[string]*customElementDefinition),
		whenDefined: make(map[string][]func()),
	}

	registryObj := r.vm.NewObject()

	_ = registryObj.Set("define", func(call goja.FunctionCall) goja.Value {
		return r.jsCustomElementsDefine(call, registry)
	})

	_ = registryObj.Set("get", func(call goja.FunctionCall) goja.Value {
		return r.jsCustomElementsGet(call, registry)
	})

	_ = registryObj.Set("whenDefined", func(call goja.FunctionCall) goja.Value {
		return r.jsCustomElementsWhenDefined(call, registry)
	})

	_ = r.vm.Set("customElements", registryObj)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("customElements", registryObj)
		}
	}

	r.customElementRegistry = registry
}

// jsCustomElementsDefine implements customElements.define(name, constructor, options).
func (r *Runtime) jsCustomElementsDefine(call goja.FunctionCall, registry *CustomElementRegistry) goja.Value {
	if len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'define' on 'CustomElementRegistry': 2 arguments required"))
	}

	name := call.Arguments[0].String()
	constructorVal := call.Arguments[1]

	// Validate name
	if !isValidCustomElementName(name) {
		panic(r.vm.NewTypeError(fmt.Sprintf("'%s' is not a valid custom element name", name)))
	}

	// Check if already defined
	registry.mu.Lock()
	if _, exists := registry.definitions[name]; exists {
		registry.mu.Unlock()
		panic(r.vm.NewTypeError(fmt.Sprintf("'%s' has already been defined", name)))
	}
	registry.mu.Unlock()

	// Parse options
	var extends string
	if len(call.Arguments) >= 3 {
		if opts, ok := call.Arguments[2].Export().(map[string]interface{}); ok {
			if ext, ok := opts["extends"]; ok {
				extends = fmt.Sprintf("%v", ext)
			}
		}
	}

	// Get observedAttributes from the constructor's prototype
	var observed []string
	if ctor, ok := constructorVal.(*goja.Object); ok {
		if proto := ctor.Get("prototype"); proto != nil {
			if protoObj, ok := proto.(*goja.Object); ok {
				if obs := protoObj.Get("observedAttributes"); obs != nil {
					if arr, ok := obs.Export().([]interface{}); ok {
						for _, v := range arr {
							observed = append(observed, fmt.Sprintf("%v", v))
						}
					}
				}
			}
		}
	}

	def := &customElementDefinition{
		name:     name,
		observed: observed,
		extends:  extends,
	}

	registry.mu.Lock()
	registry.definitions[name] = def
	registry.mu.Unlock()

	// Upgrade existing elements with this tag name
	r.upgradeCustomElements(name, constructorVal)

	// Resolve whenDefined promises
	registry.mu.Lock()
	callbacks := registry.whenDefined[name]
	delete(registry.whenDefined, name)
	registry.mu.Unlock()
	for _, cb := range callbacks {
		cb()
	}

	return goja.Undefined()
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
		_ = def
		// Return the constructor (we don't store it in def, so return undefined for now)
	}
	return goja.Undefined()
}

// jsCustomElementsWhenDefined implements customElements.whenDefined(name).
func (r *Runtime) jsCustomElementsWhenDefined(call goja.FunctionCall, registry *CustomElementRegistry) goja.Value {
	if len(call.Arguments) < 1 {
		return goja.Undefined()
	}
	name := call.Arguments[0].String()

	registry.mu.Lock()
	if _, exists := registry.definitions[name]; exists {
		registry.mu.Unlock()
		// Already defined, return undefined (simplified - should return resolved promise)
		return goja.Undefined()
	}

	// Not yet defined, return undefined (simplified - should return pending promise)
	registry.mu.Unlock()
	return goja.Undefined()
}

// upgradeCustomElements upgrades existing elements with the given tag name.
func (r *Runtime) upgradeCustomElements(name string, constructorVal goja.Value) {
	if r.domDoc == nil {
		return
	}

	// Walk the DOM and upgrade matching elements
	var upgrade func(n *dom.Node)
	upgrade = func(n *dom.Node) {
		if n.Type == dom.NodeElement && n.Data == name {
			r.upgradeElement(n, constructorVal)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			upgrade(c)
		}
	}

	// Start from document's HTML element
	if r.domDoc.HTML != nil {
		upgrade(r.domDoc.HTML)
	}
}

// upgradeElement upgrades a single element to a custom element.
func (r *Runtime) upgradeElement(n *dom.Node, constructorVal goja.Value) {
	// Invoke connectedCallback if element is in document
	if r.isInDocument(n) {
		r.invokeLifecycleCallback(n, "connectedCallback")
	}
}

// invokeLifecycleCallback invokes a lifecycle callback on a custom element.
func (r *Runtime) invokeLifecycleCallback(n *dom.Node, callback string) {
	// Find the node ID
	r.mu.Lock()
	var nodeID int
	for id, node := range r.nodeRegistry {
		if node == n {
			nodeID = id
			break
		}
	}
	r.mu.Unlock()

	if nodeID == 0 {
		return
	}

	// Get the JS wrapper for this node
	r.mu.Lock()
	wrapper := r.nodeWrappers[nodeID]
	r.mu.Unlock()

	if wrapper == nil {
		return
	}

	// Call the callback
	if fn := wrapper.Get(callback); fn != nil {
		if call, ok := goja.AssertFunction(fn); ok {
			_, _ = call(goja.Undefined())
		}
	}
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
	if len(name) == 0 {
		return false
	}
	// Must contain a hyphen
	if !strings.Contains(name, "-") {
		return false
	}
	// Must start with lowercase ASCII letter
	if name[0] < 'a' || name[0] > 'z' {
		return false
	}
	// Must not be any of the reserved names
	reserved := map[string]bool{
		"annotation-xml": true, "color-profile": true, "font-face": true,
		"font-face-src": true, "font-face-uri": true, "font-face-format": true,
		"font-face-name": true, "missing-glyph": true,
	}
	if reserved[name] {
		return false
	}
	return true
}
