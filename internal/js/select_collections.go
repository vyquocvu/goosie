package js

import (
	"strconv"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

const maxSelectOptionsCollectionLength = 100000

func (r *Runtime) setupSelectCollectionProto() {
	if r.htmlOptionsCollectionProto != nil {
		return
	}
	r.setupHTMLFormCollectionInterfaces()
	r.htmlOptionsCollectionProto = r.newChildProto(r.htmlCollectionProto)
	r.defineInterfaceToStringTag("HTMLOptionsCollection", r.htmlOptionsCollectionProto)
	r.defineConstructor("HTMLOptionsCollection", r.htmlOptionsCollectionProto)
}

// newHTMLSelectElementProxy supplies the indexed properties exposed directly
// by HTMLSelectElement (select[0]), backed by its live options list.
func (r *Runtime) newHTMLSelectElementProxy(target *goja.Object, selectNode *dom.Node) *goja.Object {
	item := func(index int) goja.Value {
		options := dom.SelectOptions(selectNode)
		if index >= 0 && index < len(options) {
			return r.wrapNode(options[index])
		}
		return goja.Undefined()
	}
	proxy := r.vm.NewProxy(target, &goja.ProxyTrapConfig{
		Get: func(target *goja.Object, property string, _ goja.Value) goja.Value {
			if isCanonicalIndex(property) {
				index, _ := strconv.Atoi(property)
				return item(index)
			}
			return target.Get(property)
		},
		GetIdx: func(_ *goja.Object, index int, _ goja.Value) goja.Value { return item(index) },
		Has: func(target *goja.Object, property string) bool {
			if isCanonicalIndex(property) {
				index, _ := strconv.Atoi(property)
				return index >= 0 && index < len(dom.SelectOptions(selectNode))
			}
			return target.Get(property) != nil && !goja.IsUndefined(target.Get(property))
		},
		HasIdx: func(_ *goja.Object, index int) bool { return index >= 0 && index < len(dom.SelectOptions(selectNode)) },
		GetOwnPropertyDescriptorIdx: func(_ *goja.Object, index int) goja.PropertyDescriptor {
			options := dom.SelectOptions(selectNode)
			if index < 0 || index >= len(options) {
				return goja.PropertyDescriptor{}
			}
			return goja.PropertyDescriptor{Value: r.wrapNode(options[index]), Writable: goja.FLAG_FALSE, Enumerable: goja.FLAG_TRUE, Configurable: goja.FLAG_TRUE}
		},
		Set: func(target *goja.Object, property string, value goja.Value, _ goja.Value) bool {
			if isCanonicalIndex(property) {
				index, _ := strconv.Atoi(property)
				r.setSelectOptionAt(selectNode, index, value)
				return true
			}
			return target.Set(property, value) == nil
		},
		OwnKeys: func(target *goja.Object) *goja.Object {
			options := dom.SelectOptions(selectNode)
			keys := make([]interface{}, 0, len(options)+len(target.Keys()))
			for index := range options {
				keys = append(keys, strconv.Itoa(index))
			}
			for _, key := range target.Keys() {
				keys = append(keys, key)
			}
			return r.vm.NewArray(keys...)
		},
	})
	return r.vm.ToValue(proxy).(*goja.Object)
}

func (r *Runtime) selectOptionsCollection(selectNode *dom.Node, selectedOnly bool) *goja.Object {
	r.setupSelectCollectionProto()
	cache := &r.selectOptionsCollections
	proto := r.htmlOptionsCollectionProto
	if selectedOnly {
		cache = &r.selectSelectedCollections
		proto = r.htmlCollectionProto
	}
	if *cache == nil {
		*cache = make(map[*dom.Node]*goja.Object)
	}
	if collection := (*cache)[selectNode]; collection != nil {
		return collection
	}
	matches := func() []*dom.Node {
		options := dom.SelectOptions(selectNode)
		if !selectedOnly {
			return options
		}
		dom.NormalizeSelectSelectedness(selectNode)
		selected := make([]*dom.Node, 0, len(options))
		for _, option := range options {
			if option.OptionSelected() {
				selected = append(selected, option)
			}
		}
		return selected
	}
	target := r.vm.NewObject()
	_ = target.SetPrototype(proto)
	setter := goja.Undefined()
	if !selectedOnly {
		setter = r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			r.setSelectOptionsLength(selectNode, uint32(call.Argument(0).ToInteger()))
			return goja.Undefined()
		})
	}
	_ = target.DefineAccessorProperty("length", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(len(matches()))
	}), setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
	if !selectedOnly {
		_ = target.DefineAccessorProperty("selectedIndex", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			dom.NormalizeSelectSelectedness(selectNode)
			for index, option := range dom.SelectOptions(selectNode) {
				if option.OptionSelected() {
					return r.vm.ToValue(index)
				}
			}
			return r.vm.ToValue(-1)
		}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			index := call.Argument(0).ToInteger()
			options := dom.SelectOptions(selectNode)
			for i, option := range options {
				option.SetOptionSelected(int64(i) == index, int64(i) == index)
			}
			selectNode.SelectNoSelection = index < 0 || index >= int64(len(options))
			r.updateSelectSelectedContent(selectNode)
			r.notifyMutation()
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	_ = target.Set("item", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		index := int(call.Argument(0).ToInteger())
		nodes := matches()
		if index < 0 || index >= len(nodes) {
			return goja.Null()
		}
		return r.wrapNode(nodes[index])
	}))
	_ = target.Set("namedItem", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		if node := findNamedCollectionItem(matches(), call.Argument(0).String()); node != nil {
			return r.wrapNode(node)
		}
		return goja.Null()
	}))
	if !selectedOnly {
		_ = target.Set("add", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			r.addSelectOption(selectNode, call.Arguments)
			return goja.Undefined()
		}))
		_ = target.Set("remove", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			index := int64(0)
			if len(call.Arguments) > 0 {
				index = call.Argument(0).ToInteger()
			}
			options := dom.SelectOptions(selectNode)
			if index >= 0 && index < int64(len(options)) && options[index].Parent != nil {
				r.jsRemoveChild(goja.FunctionCall{This: r.wrapNode(options[index].Parent), Arguments: []goja.Value{r.wrapNode(options[index])}})
			}
			return goja.Undefined()
		}))
	}
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
			if node := findNamedCollectionItem(matches(), property); node != nil {
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
				index, _ := strconv.Atoi(property)
				return index >= 0 && index < len(matches())
			}
			value := target.Get(property)
			return value != nil && !goja.IsUndefined(value) || findNamedCollectionItem(matches(), property) != nil
		},
		HasIdx: func(_ *goja.Object, index int) bool { return index >= 0 && index < len(matches()) },
		GetOwnPropertyDescriptorIdx: func(_ *goja.Object, index int) goja.PropertyDescriptor {
			nodes := matches()
			if index < 0 || index >= len(nodes) {
				return goja.PropertyDescriptor{}
			}
			writable := goja.FLAG_FALSE
			if !selectedOnly {
				writable = goja.FLAG_TRUE
			}
			return goja.PropertyDescriptor{Value: r.wrapNode(nodes[index]), Writable: writable, Enumerable: goja.FLAG_TRUE, Configurable: goja.FLAG_TRUE}
		},
		OwnKeys: func(target *goja.Object) *goja.Object {
			nodes := matches()
			keys := make([]interface{}, 0, len(nodes)+len(target.Keys()))
			for index := range nodes {
				keys = append(keys, strconv.Itoa(index))
			}
			for _, key := range target.Keys() {
				keys = append(keys, key)
			}
			return r.vm.NewArray(keys...)
		},
		Set: func(target *goja.Object, property string, value goja.Value, _ goja.Value) bool {
			if !selectedOnly && isCanonicalIndex(property) {
				index, _ := strconv.Atoi(property)
				r.setSelectOptionAt(selectNode, index, value)
				return true
			}
			return target.Set(property, value) == nil
		},
	})
	result := r.vm.ToValue(proxy).(*goja.Object)
	(*cache)[selectNode] = result
	return result
}

// setSelectOptionAt implements the indexed setters on HTMLSelectElement and
// HTMLOptionsCollection. Null removes the option at the index; an option value
// inserts or replaces through the ordinary DOM mutation path so custom-element
// reactions and mutation bookkeeping run normally.
func (r *Runtime) setSelectOptionAt(selectNode *dom.Node, index int, value goja.Value) {
	if index < 0 || index >= maxSelectOptionsCollectionLength {
		if index >= maxSelectOptionsCollectionLength {
			panic(r.domError("QuotaExceededError", "The requested options collection index exceeds the document limit."))
		}
		return
	}
	options := dom.SelectOptions(selectNode)
	if goja.IsNull(value) {
		if index < len(options) && options[index].Parent != nil {
			old := options[index]
			r.jsRemoveChild(goja.FunctionCall{This: r.wrapNode(old.Parent), Arguments: []goja.Value{r.wrapNode(old)}})
		}
		return
	}
	option := r.getNode(value)
	if option == nil || option.Data != "option" {
		panic(r.vm.NewTypeError("The indexed value must be an HTMLOptionElement"))
	}
	if index >= len(options) {
		r.setSelectOptionsLength(selectNode, uint32(index+1))
		options = dom.SelectOptions(selectNode)
	}
	old := options[index]
	if old == option {
		return
	}
	if old.Parent == nil {
		return
	}
	r.jsReplaceChild(goja.FunctionCall{This: r.wrapNode(old.Parent), Arguments: []goja.Value{value, r.wrapNode(old)}})
}

// updateSelectSelectedContent mirrors the selected option's contents into the
// selectedcontent element used by customizable selects. The copy is made via
// the DOM clone and insertion paths so upgraded custom elements receive their
// construction and connection reactions.
func (r *Runtime) updateSelectSelectedContent(selectNode *dom.Node) {
	if selectNode == nil || selectNode.HasAttribute("multiple") {
		return
	}
	var selectedContents []*dom.Node
	var find func(*dom.Node)
	find = func(node *dom.Node) {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == dom.NodeElement && child.Data == "selectedcontent" {
				selectedContents = append(selectedContents, child)
				continue
			}
			find(child)
		}
	}
	find(selectNode)
	if len(selectedContents) == 0 {
		return
	}
	dom.NormalizeSelectSelectedness(selectNode)
	var selected *dom.Node
	for _, option := range dom.SelectOptions(selectNode) {
		if option.OptionSelected() {
			selected = option
			break
		}
	}
	for _, selectedContent := range selectedContents {
		if selectedContent.HasAttribute("disabled") {
			continue
		}
		for child := selectedContent.FirstChild; child != nil; {
			next := child.NextSibling
			r.jsRemoveChild(goja.FunctionCall{This: r.wrapNode(selectedContent), Arguments: []goja.Value{r.wrapNode(child)}})
			child = next
		}
		if selected == nil {
			continue
		}
		for child := selected.FirstChild; child != nil; child = child.NextSibling {
			clone := r.jsCloneNode(goja.FunctionCall{This: r.wrapNode(child), Arguments: []goja.Value{r.vm.ToValue(true)}})
			if !goja.IsNull(clone) && !goja.IsUndefined(clone) {
				r.jsAppendChild(goja.FunctionCall{This: r.wrapNode(selectedContent), Arguments: []goja.Value{clone}})
			}
		}
	}
}

func (r *Runtime) installSelectCollectionInterface(proto *goja.Object) {
	_ = proto.DefineAccessorProperty("options", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Null()
		}
		return r.selectOptionsCollection(node, false)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("selectedOptions", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Null()
		}
		return r.selectOptionsCollection(node, true)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("length", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		return r.vm.ToValue(len(dom.SelectOptions(node)))
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node != nil {
			r.setSelectOptionsLength(node, uint32(call.Argument(0).ToInteger()))
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = proto.Set("item", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || len(call.Arguments) == 0 {
			return goja.Null()
		}
		index := int(call.Argument(0).ToInteger())
		options := dom.SelectOptions(node)
		if index < 0 || index >= len(options) {
			return goja.Null()
		}
		return r.wrapNode(options[index])
	}))
	_ = proto.Set("add", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		r.addSelectOption(node, call.Arguments)
		return goja.Undefined()
	}))
	_ = proto.Set("remove", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		r.removeSelectOption(node, call.Arguments)
		return goja.Undefined()
	}))
}

func (r *Runtime) addSelectOption(selectNode *dom.Node, args []goja.Value) {
	if selectNode == nil || len(args) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'add' on 'HTMLSelectElement': 1 argument required"))
	}
	option := r.getNode(args[0])
	if option == nil || option.Data != "option" && option.Data != "optgroup" {
		panic(r.vm.NewTypeError("The provided value is not an HTMLOptionElement or HTMLOptGroupElement"))
	}
	var before *dom.Node
	if len(args) > 1 && !goja.IsNull(args[1]) && !goja.IsUndefined(args[1]) {
		if candidate := r.getNode(args[1]); candidate != nil {
			before = candidate
		} else {
			index := int(args[1].ToInteger())
			options := dom.SelectOptions(selectNode)
			if index >= 0 && index < len(options) {
				before = options[index]
			}
		}
	}
	parent := selectNode
	if before != nil {
		if before.Data != "option" && before.Data != "optgroup" || dom.NearestSelect(before) != selectNode {
			panic(r.domError("NotFoundError", "The reference option is not in this select element."))
		}
		parent = before.Parent
	}
	var ref goja.Value = goja.Null()
	if before != nil {
		ref = r.wrapNode(before)
	}
	r.jsInsertBefore(goja.FunctionCall{This: r.wrapNode(parent), Arguments: []goja.Value{args[0], ref}})
}

func (r *Runtime) removeSelectOption(selectNode *dom.Node, args []goja.Value) {
	if selectNode == nil {
		return
	}
	if len(args) == 0 {
		if selectNode.Parent != nil {
			r.jsRemoveChild(goja.FunctionCall{This: r.wrapNode(selectNode.Parent), Arguments: []goja.Value{r.wrapNode(selectNode)}})
		}
		return
	}
	index := int(args[0].ToInteger())
	options := dom.SelectOptions(selectNode)
	if index >= 0 && index < len(options) && options[index].Parent != nil {
		r.jsRemoveChild(goja.FunctionCall{This: r.wrapNode(options[index].Parent), Arguments: []goja.Value{r.wrapNode(options[index])}})
	}
}

func (r *Runtime) setSelectOptionsLength(selectNode *dom.Node, length uint32) {
	if length > maxSelectOptionsCollectionLength {
		panic(r.domError("QuotaExceededError", "The requested options collection length exceeds the document limit."))
	}
	for {
		options := dom.SelectOptions(selectNode)
		if len(options) <= int(length) {
			if len(options) < int(length) {
				option := selectNode.Doc.NewElement("option")
				r.jsAppendChild(goja.FunctionCall{This: r.wrapNode(selectNode), Arguments: []goja.Value{r.wrapNode(option)}})
				continue
			}
			return
		}
		last := options[len(options)-1]
		r.jsRemoveChild(goja.FunctionCall{This: r.wrapNode(last.Parent), Arguments: []goja.Value{r.wrapNode(last)}})
	}
}
