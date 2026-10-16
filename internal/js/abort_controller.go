package js

import (
	"github.com/dop251/goja"
)

// AbortController and AbortSignal API implementation.

// setupAbortController installs AbortController and AbortSignal constructors.
func (r *Runtime) setupAbortController() {
	// AbortController constructor.
	controllerCtor := func(call goja.ConstructorCall) *goja.Object {
		obj := r.vm.NewObject()

		// Create the signal object.
		signalObj := r.vm.NewObject()
		_ = signalObj.Set("aborted", false)
		_ = signalObj.Set("reason", goja.Undefined())
		_ = signalObj.Set("onabort", goja.Null())

		_ = signalObj.Set("throwIfAborted", func(call goja.FunctionCall) goja.Value {
			aborted := signalObj.Get("aborted")
			if aborted != nil && aborted.ToBoolean() {
				reason := signalObj.Get("reason")
				if reason == nil || goja.IsUndefined(reason) {
					panic(r.vm.ToValue("AbortError"))
				}
				panic(reason)
			}
			return goja.Undefined()
		})

		_ = obj.Set("signal", signalObj)

		// abort(reason) method.
		_ = obj.Set("abort", func(call goja.FunctionCall) goja.Value {
			aborted := signalObj.Get("aborted")
			if aborted != nil && aborted.ToBoolean() {
				return goja.Undefined()
			}

			_ = signalObj.Set("aborted", true)

			if len(call.Arguments) >= 1 {
				_ = signalObj.Set("reason", call.Arguments[0])
			} else {
				_ = signalObj.Set("reason", r.vm.ToValue("AbortError"))
			}

			// Fire onabort handler.
			handler := signalObj.Get("onabort")
			if handler != nil && !goja.IsNull(handler) && !goja.IsUndefined(handler) {
				if fn, ok := goja.AssertFunction(handler); ok {
					eventObj := r.vm.NewObject()
					_ = eventObj.Set("type", "abort")
					_, _ = fn(signalObj, eventObj)
				}
			}

			return goja.Undefined()
		})

		return obj
	}

	// Install AbortController.
	controllerVal := r.vm.ToValue(controllerCtor)
	_ = r.vm.Set("AbortController", controllerVal)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("AbortController", controllerVal)
		}
	}

	// Install AbortSignal as a plain object with static methods.
	signalObj := r.vm.NewObject()

	// AbortSignal.abort(reason) static method.
	_ = signalObj.Set("abort", func(call goja.FunctionCall) goja.Value {
		sig := r.vm.NewObject()
		_ = sig.Set("aborted", true)
		if len(call.Arguments) >= 1 {
			_ = sig.Set("reason", call.Arguments[0])
		} else {
			_ = sig.Set("reason", r.vm.ToValue("AbortError"))
		}
		_ = sig.Set("onabort", goja.Null())
		return sig
	})

	// AbortSignal.timeout(ms) static method.
	_ = signalObj.Set("timeout", func(call goja.FunctionCall) goja.Value {
		sig := r.vm.NewObject()
		_ = sig.Set("aborted", false)
		_ = sig.Set("reason", r.vm.ToValue("TimeoutError"))
		_ = sig.Set("onabort", goja.Null())
		return sig
	})

	_ = r.vm.Set("AbortSignal", signalObj)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("AbortSignal", signalObj)
		}
	}
}
