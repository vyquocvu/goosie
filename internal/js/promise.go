package js

import (
	"fmt"
	"sync"

	"github.com/dop251/goja"
)

// Promise state constants per the ECMAScript specification.
const (
	promisePending   = "pending"
	promiseFulfilled = "fulfilled"
	promiseRejected  = "rejected"
)

// Hidden property key to store the internal promiseObj on the JS object.
const promiseInternalKey = "__promise__"

// promiseReaction is a callback waiting for a promise to settle.
type promiseReaction struct {
	promise     *promiseObj
	onFulfilled goja.Value
	onRejected  goja.Value
}

// promiseObj holds the internal state of a Promise.
type promiseObj struct {
	r         *Runtime
	obj       *goja.Object
	mu        sync.Mutex
	state     string
	result    goja.Value
	reactions []promiseReaction
}

// setupPromise installs the Promise constructor and related APIs on the
// global scope. This provides a standards-compliant Promise implementation
// that works with async/await patterns common in modern JavaScript.
func (r *Runtime) setupPromise() {
	promiseConstructor := func(call goja.ConstructorCall) *goja.Object {
		return r.constructPromise(call)
	}

	promiseCtorObj := r.vm.ToValue(promiseConstructor).(*goja.Object)

	_ = promiseCtorObj.Set("resolve", func(call goja.FunctionCall) goja.Value {
		return r.promiseResolveValue(call)
	})

	_ = promiseCtorObj.Set("reject", func(call goja.FunctionCall) goja.Value {
		return r.promiseRejectValue(call)
	})

	_ = promiseCtorObj.Set("all", func(call goja.FunctionCall) goja.Value {
		return r.promiseAll(call)
	})

	_ = promiseCtorObj.Set("race", func(call goja.FunctionCall) goja.Value {
		return r.promiseRace(call)
	})

	_ = r.vm.Set("Promise", promiseCtorObj)
}

// constructPromise implements the Promise(executor) constructor.
func (r *Runtime) constructPromise(call goja.ConstructorCall) *goja.Object {
	p := &promiseObj{
		r:     r,
		state: promisePending,
	}

	obj := call.This
	p.obj = obj

	// Store the internal promiseObj on the JS object for later retrieval.
	_ = obj.Set(promiseInternalKey, p)

	_ = obj.Set("then", func(call goja.FunctionCall) goja.Value {
		return r.promiseThen(call, p)
	})

	_ = obj.Set("catch", func(call goja.FunctionCall) goja.Value {
		return r.promiseCatch(call, p)
	})

	_ = obj.Set("finally", func(call goja.FunctionCall) goja.Value {
		return r.promiseFinally(call, p)
	})

	if len(call.Arguments) < 1 {
		return obj
	}

	executor, ok := goja.AssertFunction(call.Arguments[0])
	if !ok {
		return obj
	}

	resolve := func(call goja.FunctionCall) goja.Value {
		var val goja.Value
		if len(call.Arguments) > 0 {
			val = call.Arguments[0]
		} else {
			val = goja.Undefined()
		}
		r.resolvePromise(p, val)
		return goja.Undefined()
	}

	reject := func(call goja.FunctionCall) goja.Value {
		var val goja.Value
		if len(call.Arguments) > 0 {
			val = call.Arguments[0]
		} else {
			val = goja.Undefined()
		}
		r.rejectPromise(p, val)
		return goja.Undefined()
	}

	_, _ = executor(nil, r.vm.ToValue(resolve), r.vm.ToValue(reject))

	return obj
}

// resolvePromise settles a promise as fulfilled.
func (r *Runtime) resolvePromise(p *promiseObj, val goja.Value) {
	p.mu.Lock()
	if p.state != promisePending {
		p.mu.Unlock()
		return
	}

	// If the value is a thenable (promise-like), adopt its state.
	if obj, ok := val.(*goja.Object); ok {
		if thenFn := obj.Get("then"); thenFn != nil {
			if f, ok := goja.AssertFunction(thenFn); ok {
				p.mu.Unlock()
				_, _ = f(obj, r.vm.ToValue(func(v goja.Value) {
					r.resolvePromise(p, v)
				}), r.vm.ToValue(func(v goja.Value) {
					r.rejectPromise(p, v)
				}))
				return
			}
		}
	}

	p.state = promiseFulfilled
	p.result = val
	reactions := p.reactions
	p.reactions = nil
	p.mu.Unlock()

	for _, reaction := range reactions {
		r.triggerReaction(reaction, false, val)
	}
}

// rejectPromise settles a promise as rejected.
func (r *Runtime) rejectPromise(p *promiseObj, val goja.Value) {
	p.mu.Lock()
	if p.state != promisePending {
		p.mu.Unlock()
		return
	}

	p.state = promiseRejected
	p.result = val
	reactions := p.reactions
	p.reactions = nil
	p.mu.Unlock()

	for _, reaction := range reactions {
		r.triggerReaction(reaction, true, val)
	}
}

// triggerReaction invokes a reaction callback with the promise's result.
func (r *Runtime) triggerReaction(reaction promiseReaction, isRejected bool, val goja.Value) {
	var callback goja.Value
	if isRejected {
		callback = reaction.onRejected
	} else {
		callback = reaction.onFulfilled
	}

	if callback == nil || goja.IsUndefined(callback) {
		if isRejected {
			r.rejectPromise(reaction.promise, val)
		} else {
			r.resolvePromise(reaction.promise, val)
		}
		return
	}

	f, ok := goja.AssertFunction(callback)
	if !ok {
		if isRejected {
			r.rejectPromise(reaction.promise, val)
		} else {
			r.resolvePromise(reaction.promise, val)
		}
		return
	}

	ret, err := f(nil, val)
	if err != nil {
		r.rejectPromise(reaction.promise, r.vm.ToValue(err.Error()))
		return
	}

	r.resolvePromise(reaction.promise, ret)
}

// promiseThen implements promise.then(onFulfilled, onRejected).
func (r *Runtime) promiseThen(call goja.FunctionCall, p *promiseObj) goja.Value {
	var onFulfilled, onRejected goja.Value
	if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Arguments[0]) {
		onFulfilled = call.Arguments[0]
	}
	if len(call.Arguments) >= 2 && !goja.IsUndefined(call.Arguments[1]) {
		onRejected = call.Arguments[1]
	}

	chained := r.newPromiseObj()

	reaction := promiseReaction{
		promise:     chained,
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
	}

	p.mu.Lock()
	if p.state == promisePending {
		p.reactions = append(p.reactions, reaction)
		p.mu.Unlock()
	} else {
		isRejected := p.state == promiseRejected
		result := p.result
		p.mu.Unlock()
		r.triggerReaction(reaction, isRejected, result)
	}

	return chained.obj
}

// promiseCatch implements promise.catch(onRejected).
func (r *Runtime) promiseCatch(call goja.FunctionCall, p *promiseObj) goja.Value {
	var onRejected goja.Value
	if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Arguments[0]) {
		onRejected = call.Arguments[0]
	}

	return r.promiseThen(goja.FunctionCall{
		This:      p.obj,
		Arguments: []goja.Value{goja.Undefined(), onRejected},
	}, p)
}

// promiseFinally implements promise.finally(onFinally).
func (r *Runtime) promiseFinally(call goja.FunctionCall, p *promiseObj) goja.Value {
	var onFinally goja.Value
	if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Arguments[0]) {
		onFinally = call.Arguments[0]
	}

	if onFinally == nil || goja.IsUndefined(onFinally) {
		return r.promiseThen(call, p)
	}

	finallyFn, ok := goja.AssertFunction(onFinally)
	if !ok {
		return r.promiseThen(call, p)
	}

	wrappedFulfilled := func(call goja.FunctionCall) goja.Value {
		_, _ = finallyFn(nil)
		if len(call.Arguments) > 0 {
			return call.Arguments[0]
		}
		return goja.Undefined()
	}

	wrappedRejected := func(call goja.FunctionCall) goja.Value {
		_, _ = finallyFn(nil)
		if len(call.Arguments) > 0 {
			return call.Arguments[0]
		}
		return goja.Undefined()
	}

	return r.promiseThen(goja.FunctionCall{
		This:      p.obj,
		Arguments: []goja.Value{r.vm.ToValue(wrappedFulfilled), r.vm.ToValue(wrappedRejected)},
	}, p)
}

// newPromiseObj creates a new pending promise object.
func (r *Runtime) newPromiseObj() *promiseObj {
	p := &promiseObj{
		r:     r,
		state: promisePending,
	}

	obj := r.vm.NewObject()
	p.obj = obj
	_ = obj.Set(promiseInternalKey, p)

	_ = obj.Set("then", func(call goja.FunctionCall) goja.Value {
		return r.promiseThen(call, p)
	})

	_ = obj.Set("catch", func(call goja.FunctionCall) goja.Value {
		return r.promiseCatch(call, p)
	})

	_ = obj.Set("finally", func(call goja.FunctionCall) goja.Value {
		return r.promiseFinally(call, p)
	})

	return p
}

// getInternalPromise retrieves the internal promiseObj from a JS object.
func getInternalPromise(obj *goja.Object) *promiseObj {
	v := obj.Get(promiseInternalKey)
	if v == nil || goja.IsUndefined(v) {
		return nil
	}
	p, ok := v.Export().(*promiseObj)
	if !ok {
		return nil
	}
	return p
}

// promiseResolveValue implements Promise.resolve(value).
func (r *Runtime) promiseResolveValue(call goja.FunctionCall) goja.Value {
	var val goja.Value
	if len(call.Arguments) > 0 {
		val = call.Arguments[0]
	} else {
		val = goja.Undefined()
	}

	// If the value is already a promise with our internal marker, return it.
	if obj, ok := val.(*goja.Object); ok {
		if getInternalPromise(obj) != nil {
			return val
		}
	}

	p := r.newPromiseObj()
	r.resolvePromise(p, val)
	return p.obj
}

// promiseRejectValue implements Promise.reject(reason).
func (r *Runtime) promiseRejectValue(call goja.FunctionCall) goja.Value {
	var val goja.Value
	if len(call.Arguments) > 0 {
		val = call.Arguments[0]
	} else {
		val = goja.Undefined()
	}

	p := r.newPromiseObj()
	r.rejectPromise(p, val)
	return p.obj
}

// promiseAll implements Promise.all(iterable).
func (r *Runtime) promiseAll(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		p := r.newPromiseObj()
		r.rejectPromise(p, r.vm.ToValue("Promise.all requires an iterable"))
		return p.obj
	}

	iterable := call.Arguments[0]
	arr, ok := iterable.(*goja.Object)
	if !ok {
		p := r.newPromiseObj()
		r.rejectPromise(p, r.vm.ToValue("Promise.all requires an iterable"))
		return p.obj
	}

	lengthVal := arr.Get("length")
	if lengthVal == nil || goja.IsUndefined(lengthVal) {
		lengthVal = r.vm.ToValue(0)
	}
	length := int(lengthVal.ToInteger())

	if length == 0 {
		p := r.newPromiseObj()
		r.resolvePromise(p, r.vm.NewArray())
		return p.obj
	}

	p := r.newPromiseObj()
	results := make([]goja.Value, length)
	remaining := length

	for i := 0; i < length; i++ {
		val := arr.Get(fmt.Sprintf("%d", i))
		if val == nil {
			val = goja.Undefined()
		}

		// Wrap value in a resolved promise if it's not already a promise.
		var promiseJSObj *goja.Object
		if obj, ok := val.(*goja.Object); ok && getInternalPromise(obj) != nil {
			promiseJSObj = obj
		} else {
			wrapped := r.promiseResolveValue(goja.FunctionCall{Arguments: []goja.Value{val}})
			promiseJSObj, _ = wrapped.(*goja.Object)
		}

		if promiseJSObj == nil {
			continue
		}

		innerP := getInternalPromise(promiseJSObj)
		if innerP == nil {
			continue
		}

		index := i
		thenFn := func(call goja.FunctionCall) goja.Value {
			results[index] = call.Arguments[0]
			remaining--
			if remaining == 0 {
				resultArr := r.vm.NewArray()
				for j, v := range results {
					_ = resultArr.Set(fmt.Sprintf("%d", j), v)
				}
				r.resolvePromise(p, resultArr)
			}
			return goja.Undefined()
		}

		catchFn := func(call goja.FunctionCall) goja.Value {
			r.rejectPromise(p, call.Arguments[0])
			return goja.Undefined()
		}

		_ = r.promiseThen(goja.FunctionCall{
			This:      promiseJSObj,
			Arguments: []goja.Value{r.vm.ToValue(thenFn), r.vm.ToValue(catchFn)},
		}, innerP)
	}

	return p.obj
}

// promiseRace implements Promise.race(iterable).
func (r *Runtime) promiseRace(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		p := r.newPromiseObj()
		r.rejectPromise(p, r.vm.ToValue("Promise.race requires an iterable"))
		return p.obj
	}

	iterable := call.Arguments[0]
	arr, ok := iterable.(*goja.Object)
	if !ok {
		p := r.newPromiseObj()
		r.rejectPromise(p, r.vm.ToValue("Promise.race requires an iterable"))
		return p.obj
	}

	lengthVal := arr.Get("length")
	if lengthVal == nil || goja.IsUndefined(lengthVal) {
		lengthVal = r.vm.ToValue(0)
	}
	length := int(lengthVal.ToInteger())

	if length == 0 {
		return r.newPromiseObj().obj
	}

	p := r.newPromiseObj()

	for i := 0; i < length; i++ {
		val := arr.Get(fmt.Sprintf("%d", i))
		if val == nil {
			val = goja.Undefined()
		}

		var promiseJSObj *goja.Object
		if obj, ok := val.(*goja.Object); ok && getInternalPromise(obj) != nil {
			promiseJSObj = obj
		} else {
			wrapped := r.promiseResolveValue(goja.FunctionCall{Arguments: []goja.Value{val}})
			promiseJSObj, _ = wrapped.(*goja.Object)
		}

		if promiseJSObj == nil {
			continue
		}

		innerP := getInternalPromise(promiseJSObj)
		if innerP == nil {
			continue
		}

		thenFn := func(call goja.FunctionCall) goja.Value {
			r.resolvePromise(p, call.Arguments[0])
			return goja.Undefined()
		}

		catchFn := func(call goja.FunctionCall) goja.Value {
			r.rejectPromise(p, call.Arguments[0])
			return goja.Undefined()
		}

		_ = r.promiseThen(goja.FunctionCall{
			This:      promiseJSObj,
			Arguments: []goja.Value{r.vm.ToValue(thenFn), r.vm.ToValue(catchFn)},
		}, innerP)
	}

	return p.obj
}
