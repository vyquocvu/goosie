package js

import (
	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// setupShadowDOM installs the Shadow DOM API on element prototypes.
func (r *Runtime) setupShadowDOM() {
	if r.nodeProto == nil {
		return
	}

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
		panic(r.vm.NewTypeError("Shadow root already exists"))
	}

	// Parse options
	mode := "open"
	if len(call.Arguments) >= 1 {
		if opts, ok := call.Arguments[0].Export().(map[string]interface{}); ok {
			if m, ok := opts["mode"]; ok {
				mode = toString(m)
			}
		}
	}

	// Create shadow root
	sr := &dom.ShadowRoot{
		Host: node,
		Mode: mode,
	}
	node.ShadowRoot = sr

	// Create JS wrapper for shadow root
	srObj := r.vm.NewObject()
	_ = srObj.Set("host", call.This)
	_ = srObj.Set("mode", mode)

	// Add appendChild method
	_ = srObj.Set("appendChild", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		childNode := r.getNode(call.Arguments[0])
		if childNode == nil {
			return goja.Undefined()
		}
		sr.AppendChild(childNode)
		r.notifyMutation()
		// Invoke connectedCallback if child is a custom element
		if childNode.Type == dom.NodeElement {
			r.invokeLifecycleCallback(childNode, "connectedCallback")
		}
		return call.Arguments[0]
	})

	// Store shadow root wrapper
	r.mu.Lock()
	nodeID := 0
	for id, n := range r.nodeRegistry {
		if n == node {
			nodeID = id
			break
		}
	}
	if nodeID > 0 {
		r.shadowRoots[nodeID] = srObj
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

	r.mu.Lock()
	defer r.mu.Unlock()

	if srObj, ok := r.shadowRoots[nodeID]; ok {
		return srObj
	}
	return goja.Null()
}

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		return ""
	}
}
