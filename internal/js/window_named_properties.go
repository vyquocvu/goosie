package js

import (
	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// installInitialWindowNamedProperties installs the legacy Window named
// properties contributed by ID-bearing elements that are in the parsed
// document. The accessors resolve against the live document, so removing or
// replacing an element changes the value observed through the property.
func (r *Runtime) installInitialWindowNamedProperties() {
	if r.domDoc == nil {
		return
	}
	r.domDoc.Node.Walk(func(n *dom.Node) bool {
		if n.Element() {
			if id := n.GetAttribute("id"); id != "" {
				r.installWindowNamedProperty(id)
			}
			if n.Data == "iframe" {
				if name := n.GetAttribute("name"); name != "" {
					r.installWindowNamedProperty(name)
				}
			}
		}
		return true
	})
}

func (r *Runtime) installWindowNamedProperties(nodes []*dom.Node) {
	for _, root := range nodes {
		if root == nil {
			continue
		}
		root.Walk(func(node *dom.Node) bool {
			if node.Element() {
				if id := node.GetAttribute("id"); id != "" {
					r.installWindowNamedProperty(id)
				}
				if node.Data == "iframe" {
					if name := node.GetAttribute("name"); name != "" {
						r.installWindowNamedProperty(name)
					}
				}
			}
			return true
		})
	}
}

func (r *Runtime) installWindowNamedProperty(name string) {
	if name == "" || r.vm == nil {
		return
	}
	if r.windowNamedProperties == nil {
		r.windowNamedProperties = make(map[string]bool)
	}
	if r.windowNamedProperties[name] {
		return
	}
	win := r.vm.GlobalObject()
	// Existing global bindings take precedence over legacy named properties.
	if existing := win.Get(name); existing != nil && !goja.IsUndefined(existing) {
		return
	}
	getter := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if r.domDoc == nil {
			return goja.Undefined()
		}
		for _, frame := range r.windowFrameNodes() {
			if frame.GetAttribute("name") == name {
				return r.iframeWindow(frame)
			}
		}
		node := r.domDoc.ElementByID(name)
		if node == nil {
			return goja.Undefined()
		}
		return r.wrapNode(node)
	})
	setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		var value goja.Value = goja.Undefined()
		if len(call.Arguments) > 0 {
			value = call.Arguments[0]
		}
		// A declared global or explicit Window assignment shadows the legacy
		// named property, as it does on a browser Window.
		_ = win.DefineDataProperty(name, value, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE)
		return goja.Undefined()
	})
	if err := win.DefineAccessorProperty(name, getter, setter, goja.FLAG_TRUE, goja.FLAG_FALSE); err == nil {
		r.windowNamedProperties[name] = true
	}
}
