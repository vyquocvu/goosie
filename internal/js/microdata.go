package js

import (
	"sort"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// HTML Microdata (HTML Living Standard §5.2) is the structured-data
// vocabulary built into HTML itself: itemscope marks an item, itemtype and
// itemid name it, and itemprop marks its properties. The DOM side is the five
// attribute reflections, the read-only properties collection, and
// document.getItems.
//
// This runtime has no shared HTMLElement prototype — every element wrapper
// carries its own accessors — so the reflections are installed per wrapper by
// defineMicrodataAccessors, called from wrapNode like defineStyleAccessor and
// the rest. document.getItems is installed once on the document object here.

// setupMicrodata installs document.getItems on the document object. It runs
// in New's registration chain; the method body re-reads r.domDoc per call, so
// a runtime whose DOM arrives later via SetDOM is served the same way.
func (r *Runtime) setupMicrodata() {
	docVal := r.vm.Get("document")
	doc, ok := docVal.(*goja.Object)
	if !ok {
		return
	}
	_ = doc.Set("getItems", func(call goja.FunctionCall) goja.Value {
		return r.jsGetItems(call)
	})
}

// jsGetItems implements document.getItems(type). It returns the document's
// top-level microdata items: elements with itemscope that are not themselves
// a property (itemprop) of another item. With a space-separated type list,
// an item qualifies only when its itemtype tokens include every requested
// type; without an argument every top-level item qualifies.
func (r *Runtime) jsGetItems(call goja.FunctionCall) goja.Value {
	if r.domDoc == nil {
		return r.newHTMLCollection(nil)
	}
	var want []string
	if len(call.Arguments) > 0 && call.Arguments[0] != nil &&
		!goja.IsUndefined(call.Arguments[0]) && !goja.IsNull(call.Arguments[0]) {
		want = microdataTokens(call.Arguments[0].String())
	}
	var items []*dom.Node
	r.domDoc.Node.Walk(func(n *dom.Node) bool {
		if n.Element() && n.HasAttribute("itemscope") && !n.HasAttribute("itemprop") &&
			itemtypeContains(n.GetAttribute("itemtype"), want) {
			items = append(items, n)
		}
		return true
	})
	return r.newHTMLCollection(items)
}

// itemtypeContains reports whether the space-separated tokens of itemtype
// include every type in want. An empty want matches any item, including one
// with no itemtype at all.
func itemtypeContains(itemtype string, want []string) bool {
	if len(want) == 0 {
		return true
	}
	have := microdataTokens(itemtype)
	for _, w := range want {
		found := false
		for _, h := range have {
			if h == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func microdataTokens(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '\r', '\f':
			return true
		default:
			return false
		}
	})
}

// microdataProperties returns the properties found by the HTML microdata
// algorithm. itemref adds referenced elements in the item's tree scope to the
// traversal queue; nested items are included as properties but their children
// are left to their own properties collection.
func (r *Runtime) microdataProperties(item *dom.Node) []*dom.Node {
	if item == nil || !item.Element() {
		return nil
	}
	root := r.microdataTreeScopeRoot(item)
	if root == nil {
		return nil
	}
	pending := make([]*dom.Node, 0)
	for child := item.FirstChild; child != nil; child = child.NextSibling {
		if child.Element() {
			pending = append(pending, child)
		}
	}
	for _, id := range microdataTokens(item.GetAttribute("itemref")) {
		if target := findElementByIDInTree(root, id); target != nil {
			pending = append(pending, target)
		}
	}

	seen := map[*dom.Node]struct{}{item: {}}
	var out []*dom.Node
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		if current == nil || !current.Element() {
			continue
		}
		if _, exists := seen[current]; exists {
			continue
		}
		seen[current] = struct{}{}
		if !current.HasAttribute("itemscope") {
			for child := current.FirstChild; child != nil; child = child.NextSibling {
				if child.Element() {
					pending = append(pending, child)
				}
			}
		}
		if current.HasAttribute("itemprop") && len(microdataTokens(current.GetAttribute("itemprop"))) > 0 {
			out = append(out, current)
		}
	}

	order := make(map[*dom.Node]int)
	index := 0
	var walkTree func(*dom.Node)
	walkTree = func(node *dom.Node) {
		for current := node; current != nil; current = current.NextSibling {
			order[current] = index
			index++
			walkTree(current.FirstChild)
		}
	}
	walkTree(root)
	sort.SliceStable(out, func(i, j int) bool { return order[out[i]] < order[out[j]] })
	return out
}

func (r *Runtime) microdataTreeScopeRoot(item *dom.Node) *dom.Node {
	if shadow := r.shadowRootForNode(item); shadow != nil {
		return shadow.FirstChild
	}
	root := item
	for root.Parent != nil {
		root = root.Parent
	}
	return root
}

// newHTMLCollection wraps nodes as an HTMLCollection-like object: length,
// item(index) and 0-based indexed access in document order. It is built fresh
// on every read of the property that returns it, so callers that re-read see
// the tree as it stands; a held reference is the snapshot it was built from.
func (r *Runtime) newHTMLCollection(nodes []*dom.Node) *goja.Object {
	obj := r.vm.NewObject()
	_ = obj.Set("length", len(nodes))
	for i, n := range nodes {
		_ = obj.Set(strconv.Itoa(i), r.wrapNode(n))
	}
	_ = obj.Set("item", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || call.Arguments[0] == nil {
			return goja.Null()
		}
		i := int(call.Arguments[0].ToInteger())
		if i < 0 || i >= len(nodes) {
			return goja.Null()
		}
		return r.wrapNode(nodes[i])
	})
	return obj
}

// newPropertiesCollection builds the HTMLPropertiesCollection for item: the
// length/item surface of a collection plus names, the distinct itemprop
// tokens across the item's properties in first-encounter order.
func (r *Runtime) newPropertiesCollection(item *dom.Node) *goja.Object {
	props := r.microdataProperties(item)
	obj := r.newHTMLCollection(props)
	counted := make(map[string]bool)
	var names []goja.Value
	for _, p := range props {
		for _, name := range microdataTokens(p.GetAttribute("itemprop")) {
			if counted[name] {
				continue
			}
			counted[name] = true
			names = append(names, r.vm.ToValue(name))
		}
	}
	_ = obj.Set("names", newJSArray(r.vm, names))
	return obj
}

// defineMicrodataAccessors installs the microdata IDL attributes on one
// element wrapper: the itemscope boolean reflection, the itemtype/itemid/
// itemprop/itemref string reflections, and the read-only properties
// collection. itemprop/itemref stay plain strings (the DOMTokenList form can
// come later); properties is a getter-only accessor, so a sloppy-mode write
// to it is ignored the way browsers ignore it.
//
// Each IDL name is reachable in two spellings — the content-attribute form
// (itemscope, itemprop) and the spec's camelCase IDL form (itemScope,
// itemProp) — with one accessor pair shared between them.
func (r *Runtime) defineMicrodataAccessors(obj *goja.Object, node *dom.Node) {
	if node == nil || !node.Element() {
		return
	}
	define := func(names []string, get, set goja.Value) {
		for _, name := range names {
			_ = obj.DefineAccessorProperty(name, get, set, goja.FLAG_TRUE, goja.FLAG_TRUE)
		}
	}

	isGet := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(node.HasAttribute("itemscope"))
	})
	isSet := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		want := len(call.Arguments) > 0 && call.Arguments[0] != nil && call.Arguments[0].ToBoolean()
		if want == node.HasAttribute("itemscope") {
			return goja.Undefined()
		}
		if want {
			node.SetAttribute("itemscope", "")
		} else {
			node.RemoveAttribute("itemscope")
		}
		r.notifyMutation()
		return goja.Undefined()
	})
	define([]string{"itemscope", "itemScope"}, isGet, isSet)

	str := func(attr string, names ...string) {
		get := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return r.vm.ToValue(node.GetAttribute(attr))
		})
		set := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			var v string
			if len(call.Arguments) > 0 && call.Arguments[0] != nil {
				v = call.Arguments[0].String()
			}
			before, had := node.GetAttribute(attr), node.HasAttribute(attr)
			node.SetAttribute(attr, v)
			if !had || before != v {
				r.notifyMutation()
			}
			return goja.Undefined()
		})
		define(names, get, set)
	}
	str("itemtype", "itemtype", "itemType")
	str("itemid", "itemid", "itemId")
	str("itemprop", "itemprop", "itemProp")
	str("itemref", "itemref", "itemRef")

	pGet := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.newPropertiesCollection(node)
	})
	_ = obj.DefineAccessorProperty("properties", pGet, goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
}
