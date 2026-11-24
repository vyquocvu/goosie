package js

import (
	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

func (r *Runtime) installDocumentNamedProperties(doc *goja.Object, docNID int) {
	namedValue := func(name string) goja.Value {
		forms := r.documentNamedForms(name)
		switch len(forms) {
		case 0:
			return goja.Undefined()
		case 1:
			return r.wrapNode(forms[0])
		default:
			return r.newLiveHTMLCollectionMatching(func() []*dom.Node {
				return r.documentNamedForms(name)
			})
		}
	}
	proxy := r.vm.NewProxy(doc, &goja.ProxyTrapConfig{Get: func(target *goja.Object, property string, _ goja.Value) goja.Value {
		if value := target.Get(property); value != nil && !goja.IsUndefined(value) {
			return value
		}
		return namedValue(property)
	},
		Has: func(target *goja.Object, property string) bool {
			if value := target.Get(property); value != nil && !goja.IsUndefined(value) {
				return true
			}
			return len(r.documentNamedForms(property)) != 0
		},
		GetOwnPropertyDescriptor: func(target *goja.Object, property string) goja.PropertyDescriptor {
			if descriptor := r.ownPropertyDescriptor(target, property); !descriptor.Empty() {
				return descriptor
			}
			forms := r.documentNamedForms(property)
			if len(forms) == 0 {
				return goja.PropertyDescriptor{}
			}
			return goja.PropertyDescriptor{Value: namedValue(property), Writable: goja.FLAG_FALSE, Enumerable: goja.FLAG_FALSE, Configurable: goja.FLAG_TRUE}
		},
		OwnKeys: func(target *goja.Object) *goja.Object {
			keys := make([]interface{}, 0, len(target.GetOwnPropertyNames()))
			seen := make(map[string]bool, len(keys))
			for _, key := range target.GetOwnPropertyNames() {
				keys = append(keys, key)
				seen[key] = true
			}
			if r.domDoc != nil {
				r.domDoc.Node.Walk(func(node *dom.Node) bool {
					if !node.Element() || node.Namespace != dom.NSHTML || node.Data != "form" {
						return true
					}
					for _, key := range []string{node.GetAttribute("id"), node.GetAttribute("name")} {
						if key != "" && !seen[key] {
							keys = append(keys, key)
							seen[key] = true
						}
					}
					return true
				})
			}
			return r.vm.NewArray(keys...)
		},
	})
	proxyObject := r.vm.ToValue(proxy).(*goja.Object)
	_ = r.vm.Set("document", proxyObject)
	r.nodeWrappers[docNID] = proxyObject
}

func (r *Runtime) documentNamedForms(name string) []*dom.Node {
	if name == "" || r.domDoc == nil {
		return nil
	}
	var forms []*dom.Node
	r.domDoc.Node.Walk(func(node *dom.Node) bool {
		if node.Element() && node.Namespace == dom.NSHTML && node.Data == "form" &&
			(node.GetAttribute("id") == name || node.GetAttribute("name") == name) {
			forms = append(forms, node)
		}
		return true
	})
	return forms
}
