package js

import (
	"errors"

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

		// abort(reason) method. An absent or undefined reason installs the
		// default AbortError DOMException; any value the page passed - null
		// included - is kept verbatim, as scripts/verify-br123-fetch-abort.js
		// measured in Chromium.
		_ = obj.Set("abort", func(call goja.FunctionCall) goja.Value {
			reason := r.resolveAbortReason(call.Arguments)
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
		return r.newAbortSignal(true, r.resolveAbortReason(call.Arguments))
	})

	// AbortSignal.timeout(ms) static method. Chromium reports signal.reason as
	// undefined until the timer fires; the timer itself is not implemented yet
	// (GOOSIE-SIGNALTIMEOUT-001), so the reason starts out undefined rather
	// than claiming a TimeoutError that never arrives.
	_ = signalObj.Set("timeout", func(call goja.FunctionCall) goja.Value {
		return r.newAbortSignal(false, goja.Undefined())
	})

	_ = r.vm.Set("AbortSignal", signalObj)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("AbortSignal", signalObj)
		}
	}
}

// resolveAbortReason turns abort()'s argument list into the reason value the
// signal carries. An absent or undefined argument means the default abort,
// which Chromium reports as
// "AbortError: signal is aborted without reason"; every other value - null
// included - is kept as the page wrote it.
func (r *Runtime) resolveAbortReason(args []goja.Value) goja.Value {
	if len(args) >= 1 && args[0] != nil && !goja.IsUndefined(args[0]) {
		return args[0]
	}
	return r.newAbortErrorDOMException()
}

// newAbortErrorDOMException builds the default abort reason: a DOMException
// named AbortError with the message Chromium's own signal.reason carries.
func (r *Runtime) newAbortErrorDOMException() goja.Value {
	return r.newDOMExceptionValue("AbortError", "signal is aborted without reason")
}

// newDOMExceptionValue constructs an exception through the page's DOMException
// constructor, so `err instanceof DOMException` holds for engine-raised values
// the same way it does for ones the script built itself.
func (r *Runtime) newDOMExceptionValue(name, message string) goja.Value {
	if constructor, ok := goja.AssertConstructor(r.vm.Get("DOMException")); ok {
		if exception, err := constructor(nil, r.vm.ToValue(message), r.vm.ToValue(name)); err == nil {
			return exception
		}
	}
	exception := r.vm.NewGoError(errors.New(message))
	_ = exception.Set("name", name)
	return exception
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
