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
		signalObj := r.newAbortSignal(false, goja.Undefined())

		_ = obj.Set("signal", signalObj)

		// abort(reason) method.
		_ = obj.Set("abort", func(call goja.FunctionCall) goja.Value {
			reason := goja.Value(r.vm.ToValue("AbortError"))
			if len(call.Arguments) >= 1 {
				reason = call.Arguments[0]
			}
			if fn, ok := goja.AssertFunction(signalObj.Get("__abort__")); ok {
				_, _ = fn(signalObj, reason)
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
		var reason goja.Value
		if len(call.Arguments) >= 1 {
			reason = call.Arguments[0]
		} else {
			reason = r.vm.ToValue("AbortError")
		}
		return r.newAbortSignal(true, reason)
	})

	// AbortSignal.timeout(ms) static method.
	_ = signalObj.Set("timeout", func(call goja.FunctionCall) goja.Value {
		return r.newAbortSignal(false, r.vm.ToValue("TimeoutError"))
	})

	_ = r.vm.Set("AbortSignal", signalObj)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("AbortSignal", signalObj)
		}
	}
}

func (r *Runtime) newAbortSignal(aborted bool, reason goja.Value) *goja.Object {
	signal := r.vm.NewObject()
	r.initializeGenericEventTarget(signal)
	stateAborted, stateReason := aborted, reason
	_ = signal.DefineAccessorProperty("aborted", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(stateAborted)
	}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = signal.DefineAccessorProperty("reason", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return stateReason
	}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = signal.Set("onabort", goja.Null())
	algorithms := make([]goja.Value, 0)
	_ = signal.Set("__registerAbortAlgorithm__", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		if signal.Get("aborted").ToBoolean() {
			if fn, ok := goja.AssertFunction(call.Arguments[0]); ok {
				_, _ = fn(signal, goja.Undefined())
			}
			return goja.Undefined()
		}
		algorithms = append(algorithms, call.Arguments[0])
		return goja.Undefined()
	})
	_ = signal.Set("throwIfAborted", func(call goja.FunctionCall) goja.Value {
		if stateAborted {
			panic(stateReason)
		}
		return goja.Undefined()
	})
	_ = signal.Set("__abort__", func(call goja.FunctionCall) goja.Value {
		if stateAborted {
			return goja.Undefined()
		}
		stateAborted = true
		if len(call.Arguments) != 0 {
			stateReason = call.Arguments[0]
		}
		r.runAbortAlgorithms(signal)
		handler := signal.Get("onabort")
		if fn, ok := goja.AssertFunction(handler); ok {
			eventObj := r.vm.NewObject()
			_ = eventObj.Set("type", "abort")
			_, _ = fn(signal, eventObj)
		}
		return goja.Undefined()
	})
	_ = signal.Set("__runAbortAlgorithms__", func(call goja.FunctionCall) goja.Value {
		pending := algorithms
		algorithms = nil
		for _, algorithm := range pending {
			if fn, ok := goja.AssertFunction(algorithm); ok {
				_, _ = fn(signal, goja.Undefined())
			}
		}
		return goja.Undefined()
	})
	return signal
}

func (r *Runtime) runAbortAlgorithms(signal *goja.Object) {
	if signal == nil {
		return
	}
	if fn, ok := goja.AssertFunction(signal.Get("__runAbortAlgorithms__")); ok {
		_, _ = fn(signal, goja.Undefined())
	}
}
