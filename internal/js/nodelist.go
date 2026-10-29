package js

import (
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

func (r *Runtime) ensureCollectionProto() *goja.Object {
	if r.collectionProto != nil {
		return r.collectionProto
	}
	val, err := r.vm.RunString(`
		(function() {
			var proto = {};
			proto[Symbol.iterator] = Array.prototype[Symbol.iterator];
			proto.keys = Array.prototype.keys;
			proto.values = Array.prototype.values;
			proto.entries = Array.prototype.entries;
			proto.forEach = Array.prototype.forEach;
			return proto;
		})()
	`)
	if err != nil {
		r.collectionProto = r.vm.NewObject()
		return r.collectionProto
	}
	obj, ok := val.(*goja.Object)
	if !ok {
		r.collectionProto = r.vm.NewObject()
		return r.collectionProto
	}
	r.collectionProto = obj
	return obj
}

func (r *Runtime) newLiveNodeList(node *dom.Node) goja.Value {
	if node != nil {
		if cached := r.childNodeLists[node]; cached != nil {
			return cached
		}
	}
	matches := func() []*dom.Node {
		if node == nil {
			return nil
		}
		children := make([]*dom.Node, 0)
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			children = append(children, child)
		}
		return children
	}
	list := r.newLiveNodeListMatching(matches)
	if node != nil {
		r.childNodeLists[node] = list
	}
	return list
}

// newLiveNodeListMatching returns a live NodeList backed by an arbitrary
// DOM relationship. Numeric properties are own indexed properties so that
// Web IDL array-like checks, including hasOwnProperty(), observe the list.
func (r *Runtime) newLiveNodeListMatching(matches func() []*dom.Node) goja.Value {
	proto := r.ensureCollectionProto()
	target := r.newLiveCollectionMatching(matches, proto).(*goja.Object)
	proxy := r.vm.NewProxy(target, &goja.ProxyTrapConfig{
		Get: func(target *goja.Object, property string, _ goja.Value) goja.Value {
			if isCanonicalIndex(property) {
				index, _ := strconv.Atoi(property)
				nodes := matches()
				if index >= 0 && index < len(nodes) {
					return r.wrapNode(nodes[index])
				}
				return goja.Undefined()
			}
			if value := target.Get(property); value != nil && !goja.IsUndefined(value) {
				return value
			}
			return goja.Undefined()
		},
		GetIdx: func(_ *goja.Object, index int, _ goja.Value) goja.Value {
			nodes := matches()
			if index >= 0 && index < len(nodes) {
				return r.wrapNode(nodes[index])
			}
			return goja.Undefined()
		},
		Has: func(target *goja.Object, property string) bool {
			if isCanonicalIndex(property) {
				index, _ := strconv.Atoi(property)
				return index >= 0 && index < len(matches())
			}
			return target.Get(property) != nil && !goja.IsUndefined(target.Get(property))
		},
		HasIdx: func(_ *goja.Object, index int) bool { return index >= 0 && index < len(matches()) },
		GetOwnPropertyDescriptorIdx: func(_ *goja.Object, index int) goja.PropertyDescriptor {
			nodes := matches()
			if index < 0 || index >= len(nodes) {
				return goja.PropertyDescriptor{}
			}
			return goja.PropertyDescriptor{Value: r.wrapNode(nodes[index]), Writable: goja.FLAG_FALSE, Enumerable: goja.FLAG_TRUE, Configurable: goja.FLAG_TRUE}
		},
		OwnKeys: func(target *goja.Object) *goja.Object {
			keys := make([]interface{}, 0, len(matches())+len(target.Keys()))
			for index := range matches() {
				keys = append(keys, strconv.Itoa(index))
			}
			for _, key := range target.Keys() {
				keys = append(keys, key)
			}
			return r.vm.NewArray(keys...)
		},
	})
	return r.vm.ToValue(proxy)
}

func (r *Runtime) newLiveHTMLCollection(root *dom.Node, tag, class string) goja.Value {
	matches := func() []*dom.Node {
		if root == nil {
			return nil
		}
		if root.Type == dom.NodeDocument && root.Doc != nil {
			if tag != "" {
				return root.Doc.ElementsByTagName(tag)
			}
			if class != "" {
				return root.Doc.ElementsByClassName(class)
			}
			return nil
		}
		if tag != "" {
			return collectByTagInSubtree(root, tag)
		}
		if class != "" {
			return collectByClassInSubtree(root, class)
		}
		return nil
	}
	return r.newLiveHTMLCollectionMatching(matches)
}

// newLiveHTMLCollectionMatching creates a live indexed collection for a
// standards-defined element relationship such as table.rows or row.cells.
func (r *Runtime) newLiveHTMLCollectionMatching(matches func() []*dom.Node) goja.Value {
	proto := r.htmlCollectionProto
	if proto == nil {
		r.setupHTMLFormCollectionInterfaces()
		proto = r.htmlCollectionProto
	}
	target := r.newLiveCollectionMatching(matches, proto).(*goja.Object)
	_ = target.Set("namedItem", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		if node := findNamedCollectionItem(matches(), call.Arguments[0].String()); node != nil {
			return r.wrapNode(node)
		}
		return goja.Null()
	}))
	var proxyValue goja.Value
	proxy := r.vm.NewProxy(target, &goja.ProxyTrapConfig{
		Get: func(target *goja.Object, property string, receiver goja.Value) goja.Value {
			if property == "length" {
				if proxyValue == nil || !receiver.StrictEquals(proxyValue) {
					panic(r.vm.NewTypeError("Illegal invocation"))
				}
				return r.vm.ToValue(len(matches()))
			}
			if isCanonicalIndex(property) {
				index, _ := strconv.Atoi(property)
				nodes := matches()
				if index >= 0 && index < len(nodes) {
					return r.wrapNode(nodes[index])
				}
				return goja.Undefined()
			}
			if value := target.Get(property); value != nil && !goja.IsUndefined(value) {
				return value
			}
			if node := findNamedCollectionItem(matches(), property); node != nil {
				return r.wrapNode(node)
			}
			return goja.Undefined()
		},
		GetIdx: func(_ *goja.Object, index int, _ goja.Value) goja.Value {
			nodes := matches()
			if index >= 0 && uint64(index) < 1<<32-1 && index < len(nodes) {
				return r.wrapNode(nodes[index])
			}
			if (index < 0 || uint64(index) >= 1<<32-1) && findNamedCollectionItem(nodes, strconv.Itoa(index)) != nil {
				node := findNamedCollectionItem(nodes, strconv.Itoa(index))
				return r.wrapNode(node)
			}
			return goja.Undefined()
		},
		Has: func(target *goja.Object, property string) bool {
			if isCanonicalIndex(property) {
				index, _ := strconv.Atoi(property)
				return index >= 0 && index < len(matches())
			}
			value := target.Get(property)
			return value != nil && !goja.IsUndefined(value) || findNamedCollectionItem(matches(), property) != nil
		},
		GetOwnPropertyDescriptor: func(target *goja.Object, property string) goja.PropertyDescriptor {
			if property == "length" || property == "item" || property == "namedItem" {
				return goja.PropertyDescriptor{}
			}
			if descriptor := r.ownPropertyDescriptor(target, property); !descriptor.Empty() {
				return descriptor
			}
			if node := findNamedCollectionItem(matches(), property); node != nil {
				return goja.PropertyDescriptor{Value: r.wrapNode(node), Writable: goja.FLAG_FALSE, Enumerable: goja.FLAG_FALSE, Configurable: goja.FLAG_TRUE}
			}
			return goja.PropertyDescriptor{}
		},
		HasIdx: func(_ *goja.Object, index int) bool { return index >= 0 && index < len(matches()) },
		GetOwnPropertyDescriptorIdx: func(_ *goja.Object, index int) goja.PropertyDescriptor {
			nodes := matches()
			if index < 0 || index >= len(nodes) {
				return goja.PropertyDescriptor{}
			}
			return goja.PropertyDescriptor{Value: r.wrapNode(nodes[index]), Writable: goja.FLAG_FALSE, Enumerable: goja.FLAG_TRUE, Configurable: goja.FLAG_TRUE}
		},
		OwnKeys: func(target *goja.Object) *goja.Object {
			nodes := matches()
			keys := make([]interface{}, 0, len(nodes)+len(supportedHTMLCollectionPropertyNames(nodes))+len(target.GetOwnPropertyNames()))
			for index := range nodes {
				keys = append(keys, strconv.Itoa(index))
			}
			seen := make(map[string]bool)
			for _, key := range supportedHTMLCollectionPropertyNames(nodes) {
				keys = append(keys, key)
				seen[key] = true
			}
			for _, key := range target.GetOwnPropertyNames() {
				if key == "length" || key == "item" || key == "namedItem" || seen[key] {
					continue
				}
				keys = append(keys, key)
				seen[key] = true
			}
			return r.vm.NewArray(keys...)
		},
		Set: func(target *goja.Object, property string, value, receiver goja.Value) bool {
			ownDescriptor := r.ownPropertyDescriptor(target, property)
			if ownDescriptor.Empty() && findNamedCollectionItem(matches(), property) != nil {
				// Named properties act as inherited properties when the collection
				// is a prototype. Define the expando on the actual receiver rather
				// than re-entering this proxy's Set trap.
				if object, ok := receiver.(*goja.Object); ok {
					return object.DefineDataProperty(property, value, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE) == nil
				}
				return false
			}
			// goja passes its internal proxy receiver in a representation that is
			// not StrictEquals to the public proxy value. Re-entering Set on that
			// receiver therefore invokes this trap again. The collection target is
			// the ordinary object that owns expandos, so write there directly.
			return target.Set(property, value) == nil
		},
		SetIdx: func(_ *goja.Object, _ int, _ goja.Value, _ goja.Value) bool { return false },
		DeleteProperty: func(target *goja.Object, property string) bool {
			ownDescriptor := r.ownPropertyDescriptor(target, property)
			if ownDescriptor.Empty() && findNamedCollectionItem(matches(), property) != nil {
				return false
			}
			return target.Delete(property) == nil
		},
		DeletePropertyIdx: func(_ *goja.Object, index int) bool {
			return index < 0 || index >= len(matches())
		},
		DefineProperty: func(target *goja.Object, property string, descriptor goja.PropertyDescriptor) bool {
			if isCanonicalIndex(property) {
				return false
			}
			ownDescriptor := r.ownPropertyDescriptor(target, property)
			if ownDescriptor.Empty() && findNamedCollectionItem(matches(), property) != nil {
				return false
			}
			if descriptor.IsAccessor() {
				return target.DefineAccessorProperty(property, descriptor.Getter, descriptor.Setter, defaultFalseFlag(descriptor.Configurable), defaultFalseFlag(descriptor.Enumerable)) == nil
			}
			value := descriptor.Value
			if value == nil {
				value = goja.Undefined()
			}
			return target.DefineDataProperty(property, value, defaultFalseFlag(descriptor.Writable), defaultFalseFlag(descriptor.Configurable), defaultFalseFlag(descriptor.Enumerable)) == nil
		},
	})
	proxyValue = r.vm.ToValue(proxy)
	return proxyValue
}

func defaultFalseFlag(flag goja.Flag) goja.Flag {
	if flag == goja.FLAG_NOT_SET {
		return goja.FLAG_FALSE
	}
	return flag
}

func supportedHTMLCollectionPropertyNames(nodes []*dom.Node) []string {
	names := make([]string, 0)
	seen := make(map[string]bool)
	for _, node := range nodes {
		if node == nil || !node.Element() || node.Namespace != dom.NSHTML {
			continue
		}
		for _, name := range []string{node.GetAttribute("id"), node.GetAttribute("name")} {
			if name != "" && !seen[name] {
				names = append(names, name)
				seen[name] = true
			}
		}
	}
	return names
}

func (r *Runtime) newLiveFormControlsCollection(matches func() []*dom.Node) goja.Value {
	if r.htmlFormControlsCollectionProto == nil {
		r.setupHTMLFormCollectionInterfaces()
	}
	target := r.newLiveCollectionMatching(matches, r.htmlFormControlsCollectionProto).(*goja.Object)
	proxy := r.vm.NewProxy(target, &goja.ProxyTrapConfig{
		Get: func(target *goja.Object, property string, _ goja.Value) goja.Value {
			if isCanonicalIndex(property) {
				idx, _ := strconv.Atoi(property)
				nodes := matches()
				if idx >= 0 && idx < len(nodes) {
					return r.wrapNode(nodes[idx])
				}
				return goja.Undefined()
			}
			if value := target.Get(property); value != nil && !goja.IsUndefined(value) {
				return value
			}
			if node := findNamedCollectionItem(matches(), property); node != nil {
				items := findNamedCollectionItems(matches(), property)
				if len(items) > 1 {
					return r.newLiveRadioNodeList(func() []*dom.Node { return findNamedCollectionItems(matches(), property) })
				}
				return r.wrapNode(node)
			}
			return goja.Undefined()
		},
		GetIdx: func(_ *goja.Object, index int, _ goja.Value) goja.Value {
			nodes := matches()
			if index >= 0 && index < len(nodes) {
				return r.wrapNode(nodes[index])
			}
			return goja.Undefined()
		},
		Has: func(target *goja.Object, property string) bool {
			if isCanonicalIndex(property) {
				idx, _ := strconv.Atoi(property)
				return idx >= 0 && idx < len(matches())
			}
			if value := target.Get(property); value != nil && !goja.IsUndefined(value) {
				return true
			}
			return findNamedCollectionItem(matches(), property) != nil
		},
		HasIdx: func(_ *goja.Object, index int) bool {
			return index >= 0 && index < len(matches())
		},
		GetOwnPropertyDescriptorIdx: func(_ *goja.Object, index int) goja.PropertyDescriptor {
			nodes := matches()
			if index < 0 || index >= len(nodes) {
				return goja.PropertyDescriptor{}
			}
			return goja.PropertyDescriptor{Value: r.wrapNode(nodes[index]), Writable: goja.FLAG_FALSE, Enumerable: goja.FLAG_TRUE, Configurable: goja.FLAG_TRUE}
		},
		OwnKeys: func(target *goja.Object) *goja.Object {
			keys := make([]interface{}, 0, len(matches())+len(target.Keys()))
			for index := range matches() {
				keys = append(keys, strconv.Itoa(index))
			}
			for _, key := range target.Keys() {
				keys = append(keys, key)
			}
			return r.vm.NewArray(keys...)
		},
	})
	_ = target.Set("namedItem", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		name := call.Arguments[0].String()
		items := findNamedCollectionItems(matches(), name)
		if len(items) == 0 {
			return goja.Null()
		}
		if len(items) > 1 {
			return r.newLiveRadioNodeList(func() []*dom.Node { return findNamedCollectionItems(matches(), name) })
		}
		return r.wrapNode(items[0])
	}))
	return r.vm.ToValue(proxy)
}

func isCanonicalIndex(property string) bool {
	index, err := strconv.ParseUint(property, 10, 32)
	return err == nil && index < 1<<32-1 && strconv.FormatUint(index, 10) == property
}

func findNamedCollectionItem(nodes []*dom.Node, name string) *dom.Node {
	if name == "" {
		return nil
	}
	for _, node := range nodes {
		if node.Element() && node.Namespace == dom.NSHTML && (node.GetAttribute("id") == name || node.GetAttribute("name") == name) {
			return node
		}
	}
	return nil
}

func findNamedCollectionItems(nodes []*dom.Node, name string) []*dom.Node {
	var result []*dom.Node
	if name == "" {
		return result
	}
	for _, node := range nodes {
		if node.Element() && node.Namespace == dom.NSHTML && (node.GetAttribute("id") == name || node.GetAttribute("name") == name) {
			result = append(result, node)
		}
	}
	return result
}

func (r *Runtime) newLiveRadioNodeList(matches func() []*dom.Node) goja.Value {
	target := r.newLiveCollectionMatching(matches, r.radioNodeListProto).(*goja.Object)
	return r.vm.ToValue(r.vm.NewProxy(target, &goja.ProxyTrapConfig{
		Get: func(target *goja.Object, property string, _ goja.Value) goja.Value {
			if isCanonicalIndex(property) {
				index, _ := strconv.Atoi(property)
				nodes := matches()
				if index >= 0 && index < len(nodes) {
					return r.wrapNode(nodes[index])
				}
				return goja.Undefined()
			}
			return target.Get(property)
		},
		GetIdx: func(_ *goja.Object, index int, _ goja.Value) goja.Value {
			nodes := matches()
			if index >= 0 && index < len(nodes) {
				return r.wrapNode(nodes[index])
			}
			return goja.Undefined()
		},
		HasIdx: func(_ *goja.Object, index int) bool { return index >= 0 && index < len(matches()) },
		GetOwnPropertyDescriptorIdx: func(_ *goja.Object, index int) goja.PropertyDescriptor {
			nodes := matches()
			if index < 0 || index >= len(nodes) {
				return goja.PropertyDescriptor{}
			}
			return goja.PropertyDescriptor{Value: r.wrapNode(nodes[index]), Writable: goja.FLAG_FALSE, Enumerable: goja.FLAG_TRUE, Configurable: goja.FLAG_TRUE}
		},
		OwnKeys: func(target *goja.Object) *goja.Object {
			keys := make([]interface{}, 0, len(matches())+len(target.Keys()))
			for index := range matches() {
				keys = append(keys, strconv.Itoa(index))
			}
			for _, key := range target.Keys() {
				keys = append(keys, key)
			}
			return r.vm.NewArray(keys...)
		},
	}))
}

func (r *Runtime) newLiveCollectionMatching(matches func() []*dom.Node, proto *goja.Object) goja.Value {
	obj := r.vm.NewObject()
	_ = obj.SetPrototype(proto)
	_ = obj.DefineAccessorProperty("length", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(len(matches()))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = obj.Set("item", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Null()
		}
		idx := int(uint32(call.Arguments[0].ToInteger()))
		nodes := matches()
		if idx < 0 || idx >= len(nodes) {
			return goja.Null()
		}
		return r.wrapNode(nodes[idx])
	}))
	return obj
}

func collectByTagInSubtree(root *dom.Node, tag string) []*dom.Node {
	var result []*dom.Node
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		collectByTagRecur(c, tag, &result)
	}
	return result
}

func collectByTagRecur(n *dom.Node, tag string, result *[]*dom.Node) {
	if n.Element() {
		if matchesDOMTagName(n, tag) {
			*result = append(*result, n)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectByTagRecur(c, tag, result)
	}
}

func matchesDOMTagName(node *dom.Node, tag string) bool {
	if node == nil || !node.Element() {
		return false
	}
	if tag == "*" {
		return true
	}
	if node.Doc != nil && node.Doc.IsXML {
		return node.Data == tag
	}
	if node.Namespace == dom.NSHTML {
		return strings.EqualFold(node.Data, tag)
	}
	return node.Data == tag
}

func (r *Runtime) newLiveHTMLCollectionNS(root *dom.Node, namespace, localName string) goja.Value {
	return r.newLiveHTMLCollectionMatching(func() []*dom.Node {
		var result []*dom.Node
		if root == nil {
			return result
		}
		var walk func(*dom.Node)
		walk = func(parent *dom.Node) {
			for child := parent.FirstChild; child != nil; child = child.NextSibling {
				if child.Element() {
					nsMatches := namespace == "*" || elementNamespaceURI(child) == namespace
					nameMatches := localName == "*" || child.Data == localName
					if nsMatches && nameMatches {
						result = append(result, child)
					}
				}
				walk(child)
			}
		}
		walk(root)
		return result
	})
}

func elementNamespaceURI(node *dom.Node) string {
	if node == nil {
		return ""
	}
	if node.NamespaceURI != "" {
		return node.NamespaceURI
	}
	switch node.Namespace {
	case dom.NSHTML:
		return "http://www.w3.org/1999/xhtml"
	case dom.NSSVG:
		return "http://www.w3.org/2000/svg"
	case dom.NSMathML:
		return "http://www.w3.org/1998/Math/MathML"
	default:
		return ""
	}
}

func collectByClassInSubtree(root *dom.Node, class string) []*dom.Node {
	var result []*dom.Node
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		collectByClassRecur(c, class, &result)
	}
	return result
}

func collectByClassRecur(n *dom.Node, class string, result *[]*dom.Node) {
	if n.Element() {
		for _, c := range n.ClassList() {
			if c == class {
				*result = append(*result, n)
				break
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectByClassRecur(c, class, result)
	}
}

func (r *Runtime) installElementCollectionMethods() {
	_ = r.elementProto.Set("getElementsByTagName", func(call goja.FunctionCall) goja.Value {
		root := r.getNode(call.This)
		if root == nil || len(call.Arguments) < 1 {
			return r.newLiveHTMLCollection(nil, "", "")
		}
		return r.newLiveHTMLCollection(root, call.Arguments[0].String(), "")
	})
	_ = r.elementProto.Set("getElementsByClassName", func(call goja.FunctionCall) goja.Value {
		root := r.getNode(call.This)
		if root == nil || len(call.Arguments) < 1 {
			return r.newLiveHTMLCollection(nil, "", "")
		}
		return r.newLiveHTMLCollection(root, "", call.Arguments[0].String())
	})
	_ = r.elementProto.Set("getElementsByTagNameNS", func(call goja.FunctionCall) goja.Value {
		root := r.getNode(call.This)
		if root == nil || len(call.Arguments) < 2 {
			return r.newLiveHTMLCollectionNS(nil, "", "")
		}
		return r.newLiveHTMLCollectionNS(root, call.Arguments[0].String(), call.Arguments[1].String())
	})
}
