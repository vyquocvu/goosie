package js

import (
	"errors"
	"time"

	"github.com/dop251/goja"
)

func (r *Runtime) setupAbortController() {
	controllerCtor := func(call goja.ConstructorCall) *goja.Object {
		obj := r.vm.NewObject()
		signalObj := r.newAbortSignal(false, goja.Undefined())

		_ = obj.Set("signal", signalObj)

		_ = obj.Set("abort", func(call goja.FunctionCall) goja.Value {
			reason := r.resolveAbortReason(call.Arguments)
			if fn, ok := goja.AssertFunction(signalObj.Get("__abort__")); ok {
				_, _ = fn(signalObj, reason)
			}
			return goja.Undefined()
		})

		return obj
	}

	controllerVal := r.vm.ToValue(controllerCtor)
	_ = r.vm.Set("AbortController", controllerVal)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("AbortController", controllerVal)
		}
	}

	signalObj := r.vm.NewObject()

	_ = signalObj.Set("abort", func(call goja.FunctionCall) goja.Value {
		return r.newAbortSignal(true, r.resolveAbortReason(call.Arguments))
	})

	_ = signalObj.Set("timeout", func(call goja.FunctionCall) goja.Value {
		ms := int64(0)
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Arguments[0]) && !goja.IsNull(call.Arguments[0]) {
			ms = call.Arguments[0].ToInteger()
		}
		if ms < 0 {
			ms = 0
		}
		signal := r.newAbortSignal(false, goja.Undefined())
		r.nextTimerID++
		id := r.nextTimerID
		callback := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			timeoutErr := r.newDOMExceptionValue("TimeoutError", "signal timed out")
			if fn, ok := goja.AssertFunction(signal.Get("__abort__")); ok {
				_, _ = fn(signal, timeoutErr)
			}
			return goja.Undefined()
		})
		r.timers[id] = &timer{
			id:       id,
			callback: callback,
			nextFire: time.Now().Add(time.Duration(ms) * time.Millisecond),
		}
		return signal
	})

	_ = signalObj.Set("any", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to execute 'any' on 'AbortSignal': 1 argument required, but only 0 present."))
		}
		out := r.newAbortSignal(false, goja.Undefined())
		outAbort, hasOutAbort := goja.AssertFunction(out.Get("__abort__"))
		if !hasOutAbort {
			panic(r.vm.NewTypeError("Failed to execute 'any' on 'AbortSignal': signal has no abort algorithm"))
		}
		forward := r.vm.ToValue(func(fnCall goja.FunctionCall) goja.Value {
			source, isObject := fnCall.This.(*goja.Object)
			if !isObject {
				return goja.Undefined()
			}
			_, _ = outAbort(out, source.Get("reason"))
			return goja.Undefined()
		})
		seen := make(map[*goja.Object]bool)
		var failure error
		r.vm.ForOf(call.Arguments[0], func(item goja.Value) bool {
			object, isObject := item.(*goja.Object)
			if !isObject {
				panic(r.vm.NewTypeError("Failed to execute 'any' on 'AbortSignal': Parameter 1 is not of type 'AbortSignal'."))
			}
			register, ok := goja.AssertFunction(object.Get("__registerAbortAlgorithm__"))
			if !ok {
				panic(r.vm.NewTypeError("Failed to execute 'any' on 'AbortSignal': Parameter 1 is not of type 'AbortSignal'."))
			}
			if seen[object] {
				return true
			}
			seen[object] = true
			if _, err := register(object, forward); err != nil {
				failure = err
				return false
			}
			return true
		})
		if failure != nil {
			panic(failure)
		}
		return out
	})

	_ = r.vm.Set("AbortSignal", signalObj)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("AbortSignal", signalObj)
		}
	}
}

func (r *Runtime) resolveAbortReason(args []goja.Value) goja.Value {
	if len(args) >= 1 && args[0] != nil && !goja.IsUndefined(args[0]) {
		return args[0]
	}
	return r.newAbortErrorDOMException()
}

func (r *Runtime) newAbortErrorDOMException() goja.Value {
	return r.newDOMExceptionValue("AbortError", "signal is aborted without reason")
}

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
		eventObj := r.jsCreateLegacyEvent("Event")
		if evt, ok := eventObj.(*goja.Object); ok {
			_ = evt.Set("type", "abort")
			_ = evt.Set("__initialized", true)
			_ = evt.Set("isTrusted", true)
			handler := signal.Get("onabort")
			if fn, ok := goja.AssertFunction(handler); ok {
				_, _ = fn(signal, evt)
			}
			r.dispatchGenericEvent(goja.FunctionCall{
				This:      signal,
				Arguments: []goja.Value{evt},
			})
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
