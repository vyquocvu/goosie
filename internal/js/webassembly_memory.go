package js

import (
	"fmt"

	"github.com/dop251/goja"
)

const (
	wasmPageSize       = 64 * 1024
	maxWasmMemoryPages = 1024
)

type wasmMemoryState struct {
	object  *goja.Object
	buffer  goja.ArrayBuffer
	shared  bool
	maximum int
	hasMax  bool
}

func (r *Runtime) setupWebAssemblyMemory() {
	wasm, ok := r.vm.Get("WebAssembly").(*goja.Object)
	if !ok {
		wasm = r.vm.NewObject()
		_ = r.vm.Set("WebAssembly", wasm)
	}
	proto := r.vm.NewObject()
	ctor := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("WebAssembly.Memory requires a descriptor"))
		}
		descriptor, ok := call.Argument(0).(*goja.Object)
		if !ok {
			panic(r.vm.NewTypeError("WebAssembly.Memory descriptor must be an object"))
		}
		initial, err := wasmPageCount(descriptor.Get("initial"), true)
		if err != nil {
			panic(r.vm.NewTypeError("WebAssembly.Memory initial must be a non-negative integer"))
		}
		maximumValue := descriptor.Get("maximum")
		hasMaximum := maximumValue != nil && !goja.IsUndefined(maximumValue)
		maximum := maxWasmMemoryPages
		if hasMaximum {
			maximum, err = wasmPageCount(maximumValue, true)
			if err != nil || maximum < initial {
				panic(r.wasmMemoryRangeError("WebAssembly.Memory maximum is invalid"))
			}
		}
		sharedValue := descriptor.Get("shared")
		shared := sharedValue != nil && !goja.IsUndefined(sharedValue) && sharedValue.ToBoolean()
		if shared && !hasMaximum {
			panic(r.vm.NewTypeError("Shared WebAssembly.Memory requires a maximum"))
		}
		if initial > maxWasmMemoryPages || maximum > maxWasmMemoryPages {
			panic(r.wasmMemoryRangeError("WebAssembly.Memory exceeds the Goosie memory limit"))
		}
		byteLength := initial * wasmPageSize
		data := make([]byte, byteLength)
		var buffer goja.ArrayBuffer
		if shared {
			buffer = r.vm.NewGrowableSharedArrayBuffer(data, maximum*wasmPageSize)
		} else {
			buffer = r.vm.NewArrayBuffer(data)
		}
		object := r.vm.NewObject()
		_ = object.SetPrototype(proto)
		if r.wasmMemories == nil {
			r.wasmMemories = make(map[*goja.Object]*wasmMemoryState)
		}
		state := &wasmMemoryState{object: object, buffer: buffer, shared: shared, maximum: maximum, hasMax: hasMaximum}
		r.wasmMemories[object] = state
		return object
	})
	ctorObject := ctor.ToObject(r.vm)
	_ = ctorObject.Set("prototype", proto)
	_ = ctorObject.Set("name", "Memory")
	_ = ctorObject.Set("length", 1)
	_ = proto.Set("constructor", ctor)
	_ = proto.DefineAccessorProperty("buffer", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		state := r.wasmMemoryFromReceiver(call.This)
		return r.vm.ToValue(state.buffer)
	}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = proto.Set("grow", func(call goja.FunctionCall) goja.Value {
		state := r.wasmMemoryFromReceiver(call.This)
		delta := 0
		if len(call.Arguments) != 0 {
			var err error
			delta, err = wasmPageCount(call.Argument(0), true)
			if err != nil {
				panic(r.wasmMemoryRangeError("WebAssembly.Memory grow delta is invalid"))
			}
		}
		oldPages := len(state.buffer.Bytes()) / wasmPageSize
		newPages := oldPages + delta
		limit := maxWasmMemoryPages
		if state.hasMax {
			limit = state.maximum
		}
		if newPages < oldPages || newPages > limit || newPages > maxWasmMemoryPages {
			panic(r.wasmMemoryRangeError("WebAssembly.Memory cannot grow beyond its maximum"))
		}
		if state.shared {
			bufferObject := r.vm.ToValue(state.buffer)
			grow, ok := goja.AssertFunction(bufferObject.ToObject(r.vm).Get("grow"))
			if !ok {
				panic(r.vm.NewTypeError("SharedArrayBuffer is not growable"))
			}
			if _, err := grow(bufferObject, r.vm.ToValue(newPages*wasmPageSize)); err != nil {
				panic(err)
			}
		} else if delta != 0 {
			old := state.buffer
			oldData := append([]byte(nil), old.Bytes()...)
			newData := make([]byte, newPages*wasmPageSize)
			copy(newData, oldData)
			state.buffer = r.vm.NewArrayBuffer(newData)
			old.Detach()
		}
		return r.vm.ToValue(oldPages)
	})
	_ = proto.Set("toString", func(call goja.FunctionCall) goja.Value {
		r.wasmMemoryFromReceiver(call.This)
		return r.vm.ToValue("[object WebAssembly.Memory]")
	})
	_ = wasm.Set("Memory", ctor)
}

func (r *Runtime) wasmMemoryRangeError(message string) goja.Value {
	constructor, ok := goja.AssertConstructor(r.vm.Get("RangeError"))
	if ok {
		if value, err := constructor(nil, r.vm.ToValue(message)); err == nil {
			return value
		}
	}
	return r.vm.NewTypeError(message)
}

func wasmPageCount(value goja.Value, required bool) (int, error) {
	if value == nil || goja.IsUndefined(value) {
		if required {
			return 0, fmt.Errorf("missing page count")
		}
		return 0, nil
	}
	n := value.ToInteger()
	if n < 0 || n > maxWasmMemoryPages || int64(int(n)) != n {
		return 0, fmt.Errorf("invalid page count")
	}
	return int(n), nil
}

func (r *Runtime) wasmMemoryFromReceiver(value goja.Value) *wasmMemoryState {
	object, ok := value.(*goja.Object)
	if !ok || r.wasmMemories[object] == nil {
		panic(r.vm.NewTypeError("WebAssembly.Memory method called on incompatible receiver"))
	}
	return r.wasmMemories[object]
}
