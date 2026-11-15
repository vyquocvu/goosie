package js

import (
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

func (r *Runtime) cssomCollectionProto(name string) *goja.Object {
	if r.cssomCollectionProtos == nil {
		r.cssomCollectionProtos = map[string]*goja.Object{}
	}
	if proto, ok := r.cssomCollectionProtos[name]; ok {
		return proto
	}
	proto := r.vm.NewObject()
	_ = proto.SetPrototype(r.vm.Get("Object").ToObject(r.vm).Get("prototype").ToObject(r.vm))
	_ = proto.DefineAccessorProperty("length", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return cssomProperty(r.cssomCollectionReceiver(call.This, name), "length")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	if name == "MediaList" {
		_ = proto.DefineAccessorProperty("mediaText", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			return cssomProperty(r.cssomCollectionReceiver(call.This, name), "mediaText")
		}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			obj := r.cssomCollectionReceiver(call.This, "MediaList")
			if state := r.mediaStateOf(obj); state != nil {
				if _, isSymbol := call.Argument(0).(*goja.Symbol); isSymbol {
					panic(r.vm.NewTypeError("Cannot convert a Symbol value to a string"))
				}
				state.setMediaText(call.Argument(0).String())
			}
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
		_ = proto.DefineDataProperty("appendMedium", r.cssomMethod("appendMedium", true, func(call goja.FunctionCall) goja.Value {
			obj := r.cssomCollectionReceiver(call.This, "MediaList")
			medium := r.cssomMediumArgument(call, "appendMedium")
			if state := r.mediaStateOf(obj); state != nil {
				state.appendMedium(medium)
			}
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE)
		_ = proto.DefineDataProperty("deleteMedium", r.cssomMethod("deleteMedium", true, func(call goja.FunctionCall) goja.Value {
			obj := r.cssomCollectionReceiver(call.This, "MediaList")
			medium := r.cssomMediumArgument(call, "deleteMedium")
			if state := r.mediaStateOf(obj); state != nil && medium != "" && !state.deleteMedium(medium) {
				panic(r.domError("NotFoundError", "The medium provided ("+medium+") was not found."))
			}
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	_ = proto.DefineDataProperty("item", r.cssomMethod("item", true, r.cssomItemFor(name)), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE)
	if name == "MediaList" {
		_ = proto.DefineDataProperty("toString", r.cssomMethod("toString", false, func(call goja.FunctionCall) goja.Value {
			text := cssomProperty(r.cssomCollectionReceiver(call.This, name), "mediaText")
			if goja.IsUndefined(text) || goja.IsNull(text) {
				panic(r.vm.NewTypeError("Illegal invocation"))
			}
			return r.vm.ToValue(text.String())
		}), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	r.defineConstructor(name, proto)
	iterator, err := r.vm.RunString("Array.prototype[Symbol.iterator]")
	if err == nil {
		_ = proto.DefineDataPropertySymbol(goja.SymIterator, iterator, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	}
	r.cssomCollectionProtos[name] = proto
	return proto
}

func (r *Runtime) cssomItemFor(name string) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		obj := r.cssomCollectionReceiver(call.This, name)
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to execute 'item' on '" + name + "': 1 argument required, but only 0 present."))
		}
		if _, isSymbol := call.Argument(0).(*goja.Symbol); isSymbol {
			panic(r.vm.NewTypeError("Failed to execute 'item' on '" + name + "': Cannot convert a Symbol value to a number"))
		}
		var count int64
		switch length := cssomProperty(obj, "length").Export().(type) {
		case int64:
			count = length
		case int:
			count = int64(length)
		case float64:
			count = int64(length)
		default:
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		index := call.Argument(0).ToInteger()
		if index < 0 || index >= count {
			return goja.Null()
		}
		value := cssomProperty(obj, strconv.Itoa(int(index)))
		if goja.IsUndefined(value) || goja.IsNull(value) {
			return goja.Null()
		}
		return value
	}
}

func (r *Runtime) cssomMediumArgument(call goja.FunctionCall, method string) string {
	if len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute '" + method + "' on 'MediaList': 1 argument required, but only 0 present."))
	}
	if _, isSymbol := call.Argument(0).(*goja.Symbol); isSymbol {
		panic(r.vm.NewTypeError("Cannot convert a Symbol value to a string"))
	}
	return call.Argument(0).String()
}

func (r *Runtime) cssomCollectionReceiver(this goja.Value, name string) *goja.Object {
	obj, ok := this.(*goja.Object)
	if !ok || obj == nil {
		panic(r.vm.NewTypeError("Illegal invocation"))
	}
	if owned, live := r.cssomCollections[obj]; !live || owned != name {
		panic(r.vm.NewTypeError("Illegal invocation"))
	}
	return obj
}

func (r *Runtime) cssomCollection(value goja.Value, name string) goja.Value {
	if r.cssomCollections == nil {
		r.cssomCollections = map[*goja.Object]string{}
	}
	if obj, ok := value.(*goja.Object); ok {
		r.cssomCollections[obj] = name
	}
	return value
}

func cssomProperty(obj *goja.Object, name string) goja.Value {
	if value := obj.Get(name); value != nil {
		return value
	}
	return goja.Undefined()
}

func (r *Runtime) cssomMethod(name string, takesIndex bool, impl func(goja.FunctionCall) goja.Value) *goja.Object {
	source := `(function (impl) {
		function NAME() { return impl.apply(this, arguments); }
		Object.defineProperty(NAME, 'name', { value: 'NAME', configurable: true });
		return NAME;
	})(__goosieCSSOMMethod__)`
	if takesIndex {
		source = `(function (impl) {
		function NAME(index) { return impl.apply(this, arguments); }
		Object.defineProperty(NAME, 'name', { value: 'NAME', configurable: true });
		return NAME;
	})(__goosieCSSOMMethod__)`
	}
	source = strings.ReplaceAll(source, "NAME", name)
	_ = r.vm.Set("__goosieCSSOMMethod__", r.vm.ToValue(impl))
	value, err := r.vm.RunString(source)
	_ = r.vm.Set("__goosieCSSOMMethod__", nil)
	if err != nil || value == nil {
		return r.vm.ToValue(impl).(*goja.Object)
	}
	return value.ToObject(r.vm)
}

func (r *Runtime) cssRuleList(owner *dom.Node) goja.Value {
	text := r.sheetCSSText(owner)
	if cached, ok := r.cssRuleListObjects[owner]; ok && cached.text == text {
		return cached.value
	}
	if r.cssRuleListObjects == nil {
		r.cssRuleListObjects = map[*dom.Node]cachedCSSRuleList{}
	}
	value := r.newCSSRuleList(r.sheetRuleView(owner))
	r.cssRuleListObjects[owner] = cachedCSSRuleList{text: text, value: value}
	return value
}

type cachedCSSRuleList struct {
	text  string
	value goja.Value
}

func (r *Runtime) newCSSRuleList(view *cachedCSSRules) goja.Value {
	items := func() []*goja.Object { return view.rules }
	target := r.vm.NewObject()
	_ = target.SetPrototype(r.cssomCollectionProto("CSSRuleList"))
	return r.cssomCollection(r.vm.ToValue(r.vm.NewProxy(target, &goja.ProxyTrapConfig{
		Get: func(proxied *goja.Object, property string, _ goja.Value) goja.Value {
			current := items()
			if property == "length" {
				return r.vm.ToValue(len(current))
			}
			if isCanonicalIndex(property) {
				index, _ := strconv.Atoi(property)
				if index >= 0 && index < len(current) {
					return current[index]
				}
				return goja.Undefined()
			}
			return proxied.Get(property)
		},
		GetIdx: func(_ *goja.Object, index int, _ goja.Value) goja.Value {
			current := items()
			if index >= 0 && index < len(current) {
				return current[index]
			}
			return goja.Undefined()
		},
		HasIdx: func(_ *goja.Object, index int) bool { return index >= 0 && index < len(items()) },
		GetOwnPropertyDescriptorIdx: func(_ *goja.Object, index int) goja.PropertyDescriptor {
			current := items()
			if index < 0 || index >= len(current) {
				return goja.PropertyDescriptor{}
			}
			return goja.PropertyDescriptor{Value: current[index], Writable: goja.FLAG_FALSE, Enumerable: goja.FLAG_TRUE, Configurable: goja.FLAG_TRUE}
		},
		OwnKeys: func(proxied *goja.Object) *goja.Object {
			current := items()
			keys := make([]interface{}, 0, len(current)+len(proxied.Keys()))
			for index := range current {
				keys = append(keys, strconv.Itoa(index))
			}
			for _, key := range proxied.Keys() {
				keys = append(keys, key)
			}
			return r.vm.NewArray(keys...)
		},
	})), "CSSRuleList")
}
