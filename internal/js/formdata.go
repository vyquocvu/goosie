package js

import (
	"fmt"

	"github.com/dop251/goja"
)

// FormData API implementation.

// formDataEntry represents one entry in a FormData object.
type formDataEntry struct {
	name  string
	value string
}

// setupFormData installs the FormData constructor.
func (r *Runtime) setupFormData() {
	ctor := func(call goja.ConstructorCall) *goja.Object {
		obj := call.This
		entries := make([]formDataEntry, 0)

		// append(name, value)
		_ = obj.Set("append", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return goja.Undefined()
			}
			name := call.Arguments[0].String()
			value := call.Arguments[1].String()
			entries = append(entries, formDataEntry{name: name, value: value})
			return goja.Undefined()
		})

		// set(name, value)
		_ = obj.Set("set", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return goja.Undefined()
			}
			name := call.Arguments[0].String()
			value := call.Arguments[1].String()

			// Remove all existing entries with this name.
			filtered := make([]formDataEntry, 0, len(entries))
			for _, e := range entries {
				if e.name != name {
					filtered = append(filtered, e)
				}
			}
			filtered = append(filtered, formDataEntry{name: name, value: value})
			entries = filtered
			return goja.Undefined()
		})

		// get(name)
		_ = obj.Set("get", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return goja.Null()
			}
			name := call.Arguments[0].String()
			for _, e := range entries {
				if e.name == name {
					return r.vm.ToValue(e.value)
				}
			}
			return goja.Null()
		})

		// getAll(name)
		_ = obj.Set("getAll", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return r.vm.NewArray()
			}
			name := call.Arguments[0].String()
			arr := r.vm.NewArray()
			i := 0
			for _, e := range entries {
				if e.name == name {
					_ = arr.Set(fmt.Sprintf("%d", i), e.value)
					i++
				}
			}
			return arr
		})

		// has(name)
		_ = obj.Set("has", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return r.vm.ToValue(false)
			}
			name := call.Arguments[0].String()
			for _, e := range entries {
				if e.name == name {
					return r.vm.ToValue(true)
				}
			}
			return r.vm.ToValue(false)
		})

		// delete(name)
		_ = obj.Set("delete", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return goja.Undefined()
			}
			name := call.Arguments[0].String()
			filtered := make([]formDataEntry, 0, len(entries))
			for _, e := range entries {
				if e.name != name {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
			return goja.Undefined()
		})

		// forEach(callback)
		_ = obj.Set("forEach", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return goja.Undefined()
			}
			fn, ok := goja.AssertFunction(call.Arguments[0])
			if !ok {
				return goja.Undefined()
			}
			for _, e := range entries {
				_, _ = fn(goja.Undefined(), r.vm.ToValue(e.value), r.vm.ToValue(e.name), obj)
			}
			return goja.Undefined()
		})

		// entries()
		_ = obj.Set("entries", func(call goja.FunctionCall) goja.Value {
			arr := r.vm.NewArray()
			i := 0
			for _, e := range entries {
				entry := r.vm.NewArray()
				_ = entry.Set("0", e.name)
				_ = entry.Set("1", e.value)
				_ = arr.Set(fmt.Sprintf("%d", i), entry)
				i++
			}
			return arr
		})

		// keys()
		_ = obj.Set("keys", func(call goja.FunctionCall) goja.Value {
			arr := r.vm.NewArray()
			i := 0
			for _, e := range entries {
				_ = arr.Set(fmt.Sprintf("%d", i), e.name)
				i++
			}
			return arr
		})

		// values()
		_ = obj.Set("values", func(call goja.FunctionCall) goja.Value {
			arr := r.vm.NewArray()
			i := 0
			for _, e := range entries {
				_ = arr.Set(fmt.Sprintf("%d", i), e.value)
				i++
			}
			return arr
		})

		return nil
	}

	val := r.vm.ToValue(ctor)
	_ = r.vm.Set("FormData", val)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("FormData", val)
		}
	}
}
