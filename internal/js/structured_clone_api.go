package js

import (
	"github.com/dop251/goja"
)

func (r *Runtime) setupStructuredClone() {
	_ = r.installGlobal("structuredClone", r.vm.ToValue(r.structuredClone))
}

func (r *Runtime) structuredClone(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'structuredClone' on 'Window': 1 argument required, but only 0 present."))
	}
	var options goja.Value
	if len(call.Arguments) > 1 {
		options = call.Arguments[1]
	}
	cloned, err := cloneMessageValue(r, r, call.Arguments[0])
	if err != nil {
		panic(r.dataCloneError(err.Error()))
	}
	for _, item := range transferList(r.vm, options) {
		object, isObject := item.(*goja.Object)
		if !isObject {
			panic(r.dataCloneError("object is not a transferable"))
		}
		if !isArrayBufferObject(r.vm, object) {
			panic(r.dataCloneError("object is not a supported transferable"))
		}
		buffer, ok := object.Export().(goja.ArrayBuffer)
		if !ok || buffer.Detached() {
			panic(r.dataCloneError("ArrayBuffer is detached or invalid"))
		}
		if !buffer.Detach() {
			panic(r.dataCloneError("ArrayBuffer is detached or invalid"))
		}
	}
	return cloned
}
