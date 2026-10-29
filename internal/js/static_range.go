package js

import (
	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

func (r *Runtime) installStaticRangeConstructor() {
	proto := r.vm.NewObject()
	ctor := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) == 0 || goja.IsNull(call.Arguments[0]) || goja.IsUndefined(call.Arguments[0]) {
			panic(r.vm.NewTypeError("Failed to construct 'StaticRange': 1 argument required"))
		}
		init := call.Arguments[0].ToObject(r.vm)
		startNode, startOK := r.staticRangeNode(init.Get("startContainer"))
		endNode, endOK := r.staticRangeNode(init.Get("endContainer"))
		if !startOK || !endOK {
			panic(r.vm.NewTypeError("Failed to construct 'StaticRange': required Node member is missing or invalid"))
		}
		if startNode.Type == dom.NodeDoctype || endNode.Type == dom.NodeDoctype {
			panic(r.domError("InvalidNodeTypeError", "A DocumentType node cannot be used as a StaticRange boundary"))
		}
		startOffset, startOffsetOK := staticRangeOffset(init.Get("startOffset"))
		endOffset, endOffsetOK := staticRangeOffset(init.Get("endOffset"))
		if !startOffsetOK || !endOffsetOK {
			panic(r.vm.NewTypeError("Failed to construct 'StaticRange': required offset member is missing"))
		}
		return r.wrapStaticRange(dom.StaticRange{
			StartContainer: startNode,
			StartOffset:    startOffset,
			EndContainer:   endNode,
			EndOffset:      endOffset,
		})
	}).(*goja.Object)
	_ = ctor.Set("prototype", proto)
	_ = proto.Set("constructor", ctor)
	for property, getter := range map[string]func(*dom.StaticRange) any{
		"startContainer": func(sr *dom.StaticRange) any { return r.wrapNode(sr.StartContainer) },
		"startOffset":    func(sr *dom.StaticRange) any { return sr.StartOffset },
		"endContainer":   func(sr *dom.StaticRange) any { return r.wrapNode(sr.EndContainer) },
		"endOffset":      func(sr *dom.StaticRange) any { return sr.EndOffset },
		"collapsed": func(sr *dom.StaticRange) any {
			return sr.StartContainer == sr.EndContainer && sr.StartOffset == sr.EndOffset
		},
	} {
		prop := property
		read := getter
		_ = proto.DefineAccessorProperty(prop, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			sr := getStaticRange(call.This.ToObject(r.vm))
			if sr == nil {
				panic(r.vm.NewTypeError("Illegal invocation"))
			}
			return r.vm.ToValue(read(sr))
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	_ = r.vm.Set("StaticRange", ctor)
	r.defineInterfaceToStringTag("StaticRange", proto)
}

func (r *Runtime) staticRangeNode(value goja.Value) (*dom.Node, bool) {
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		return nil, false
	}
	nidValue := obj.Get("__nid__")
	if nidValue == nil || goja.IsUndefined(nidValue) || goja.IsNull(nidValue) {
		return nil, false
	}
	nid := int(nidValue.ToInteger())
	node := r.nodeRegistry[nid]
	return node, node != nil
}

func staticRangeOffset(value goja.Value) (int, bool) {
	if value == nil || goja.IsUndefined(value) {
		return 0, false
	}
	return webIDLUnsignedLong(value), true
}

func (r *Runtime) wrapStaticRange(sr dom.StaticRange) *goja.Object {
	obj := r.vm.NewObject()
	if ctor, ok := r.vm.Get("StaticRange").(*goja.Object); ok {
		if proto, ok := ctor.Get("prototype").(*goja.Object); ok {
			_ = obj.SetPrototype(proto)
		}
	}
	_ = obj.Set("__staticRange__", &sr)
	return obj
}

func getStaticRange(obj *goja.Object) *dom.StaticRange {
	if obj == nil {
		return nil
	}
	value := obj.Get("__staticRange__")
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil
	}
	sr, ok := value.Export().(*dom.StaticRange)
	if !ok {
		return nil
	}
	return sr
}

func (r *Runtime) staticRangeArray(ranges []dom.StaticRange) *goja.Object {
	values := make([]interface{}, 0, len(ranges))
	for _, sr := range ranges {
		values = append(values, r.wrapStaticRange(sr))
	}
	return r.vm.NewArray(values...)
}
